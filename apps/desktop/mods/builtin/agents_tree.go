package builtin

import (
	"sort"

	"hiik/mods/api"
)

// sideAgentNode is one agent and the agents it started. The forest keeps a
// child under its parent exactly once, so a row never appears twice.
type sideAgentNode struct {
	agent    api.Agent
	children []*sideAgentNode
	index    int // the agent's place in the app's list, for stable ordering
}

// sideAgentForest builds the parent/child forest. A parent outside the list,
// a self-parent, or a cycle leaves the agent as a root; the app's order is
// kept.
func sideAgentForest(agents []api.Agent) []*sideAgentNode {
	index := make(map[string]int, len(agents))
	unique := make([]api.Agent, 0, len(agents))
	for _, ag := range agents {
		if ag.Address == "" {
			continue
		}
		if _, ok := index[ag.Address]; ok {
			continue
		}
		index[ag.Address] = len(unique)
		unique = append(unique, ag)
	}
	nodes := make([]*sideAgentNode, len(unique))
	for i, ag := range unique {
		nodes[i] = &sideAgentNode{agent: ag, index: i}
	}
	var roots []*sideAgentNode
	for i, ag := range unique {
		if ag.Parent == "" || ag.Parent == ag.Address {
			roots = append(roots, nodes[i])
			continue
		}
		parent, ok := index[ag.Parent]
		if !ok {
			roots = append(roots, nodes[i])
			continue
		}
		nodes[parent].children = append(nodes[parent].children, nodes[i])
	}

	visited := make(map[string]bool, len(nodes))
	var walk func(*sideAgentNode)
	walk = func(n *sideAgentNode) {
		if visited[n.agent.Address] {
			return
		}
		visited[n.agent.Address] = true
		kept := n.children[:0]
		for _, child := range n.children {
			if visited[child.agent.Address] {
				continue // a second edge or a cycle
			}
			kept = append(kept, child)
			walk(child)
		}
		n.children = kept
	}
	for _, root := range roots {
		walk(root)
	}
	// A cycle has no root. Promote its first agent so every agent is drawn.
	for _, node := range nodes {
		if !visited[node.agent.Address] {
			roots = append(roots, node)
			walk(node)
		}
	}
	sort.SliceStable(roots, func(i, j int) bool { return roots[i].index < roots[j].index })
	return roots
}

// sideAgentVisibleDescendants counts the rows a fold would hide.
func sideAgentVisibleDescendants(node *sideAgentNode) int {
	if node == nil {
		return 0
	}
	count := 0
	for _, child := range node.children {
		if child.agent.Pinned {
			continue
		}
		count += 1 + sideAgentVisibleDescendants(child)
	}
	return count
}

// sideAgentFlatten lists every node once, roots before their children.
func sideAgentFlatten(roots []*sideAgentNode) []*sideAgentNode {
	out := make([]*sideAgentNode, 0, len(roots))
	var walk func(*sideAgentNode)
	walk = func(node *sideAgentNode) {
		out = append(out, node)
		for _, child := range node.children {
			walk(child)
		}
	}
	for _, root := range roots {
		walk(root)
	}
	return out
}

// sideAgentSection is one foldable group of top-level rows.
type sideAgentSection struct {
	id     string
	label  string
	status string // the status that colors the header, if any
	nodes  []*sideAgentNode
}

// sideAgentSort is how the panel orders its rows.
type sideAgentSort int

const (
	sideAgentSortPriority sideAgentSort = iota
	sideAgentSortGrouped
)

func (s sideAgentSort) next() sideAgentSort {
	if s == sideAgentSortPriority {
		return sideAgentSortGrouped
	}
	return sideAgentSortPriority
}

func (s sideAgentSort) label() string {
	if s == sideAgentSortGrouped {
		return "Grouped by space"
	}
	return "Priority"
}

// glyph is the button's text icon: a clock for priority, layers for spaces.
func (s sideAgentSort) glyph() string {
	if s == sideAgentSortGrouped {
		return "☰"
	}
	return "◷"
}

// sideAgentSections collects the roots into the Pinned section first, then
// either priority sections or one group per space.
func sideAgentSections(roots []*sideAgentNode, mode sideAgentSort, spaceNames map[string]string) []sideAgentSection {
	var pinned, rest []*sideAgentNode
	for _, node := range roots {
		if node.agent.Pinned {
			pinned = append(pinned, node)
			continue
		}
		rest = append(rest, node)
	}
	// A pinned sub-agent is drawn at the top too; it must not also stay
	// nested under its parent.
	for _, node := range roots {
		collectPinned(node, &pinned)
	}
	pinned = uniqueAgentNodes(pinned)
	sort.SliceStable(pinned, func(i, j int) bool { return pinned[i].index < pinned[j].index })

	out := make([]sideAgentSection, 0, 5)
	if len(pinned) > 0 {
		out = append(out, sideAgentSection{id: "pinned", label: "Pinned", nodes: pinned})
	}
	if mode == sideAgentSortGrouped {
		order := make([]string, 0, len(rest))
		groups := make(map[string]*sideAgentSection, len(rest))
		for _, node := range rest {
			id := node.agent.Space
			group := groups[id]
			if group == nil {
				group = &sideAgentSection{id: "space:" + id, label: sideAgentSpaceLabel(id, spaceNames)}
				groups[id] = group
				order = append(order, id)
			}
			group.nodes = append(group.nodes, node)
		}
		for _, id := range order {
			out = append(out, *groups[id])
		}
		return out
	}

	sections := []sideAgentSection{
		{id: "sec:blocked", label: "Needs you", status: "blocked"},
		{id: "sec:done", label: "Done", status: "done"},
		{id: "sec:working", label: "Working", status: "working"},
		{id: "sec:idle", label: "Idle", status: "idle"},
	}
	for _, node := range rest {
		index := 3
		switch sideAgentStatusRank(node.agent.Status) {
		case 4:
			index = 0
		case 3:
			index = 1
		case 2:
			index = 2
		}
		sections[index].nodes = append(sections[index].nodes, node)
	}
	for _, section := range sections {
		if len(section.nodes) > 0 {
			out = append(out, section)
		}
	}
	return out
}

func collectPinned(node *sideAgentNode, out *[]*sideAgentNode) {
	if node == nil {
		return
	}
	if node.agent.Pinned {
		*out = append(*out, node)
	}
	for _, child := range node.children {
		collectPinned(child, out)
	}
}

func uniqueAgentNodes(nodes []*sideAgentNode) []*sideAgentNode {
	if len(nodes) < 2 {
		return nodes
	}
	seen := make(map[string]bool, len(nodes))
	out := nodes[:0]
	for _, node := range nodes {
		if seen[node.agent.Address] {
			continue
		}
		seen[node.agent.Address] = true
		out = append(out, node)
	}
	return out
}

func sideAgentSpaceLabel(id string, spaceNames map[string]string) string {
	if id == "" {
		return "No space"
	}
	if name := spaceNames[id]; name != "" {
		return name
	}
	return id
}
