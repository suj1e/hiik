package builtin

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"hiik/mods/api"
)

// agentsNotice shows the last label write the core refused, so a remote
// terminal that cannot be marked seen or pinned says so instead of failing
// silently.
var agentsNotice string

func init() {
	api.Register(api.Mod{
		ID:   "agents",
		Name: "Agents",
		Panels: []api.Panel{{
			ID:     api.PanelAgents,
			Title:  "Agents",
			Render: renderAgents,
		}},
		Commands: []api.Command{
			sideWrap(api.CmdAgentNext, "Next agent"),
			sideWrap(api.CmdAgentPrev, "Previous agent"),
			sideWrap(api.CmdAgentFocus, "Focus agent"),
			sideWrap(api.CmdAgentPin, "Pin agent"),
			sideWrap(api.CmdAgentPeek, "Peek at agent"),
		},
	})
}

// sideAgentView is what the panel keeps from frame to frame: the sort the
// user chose.
type sideAgentView struct {
	sort sideAgentSort
}

func newSideAgentView() *sideAgentView {
	return &sideAgentView{}
}

// sideAgentKey names a row of the agent outline: a section, or an agent.
type sideAgentKey struct {
	kind sideAgentEntryKind
	id   string
}

type sideAgentEntryKind uint8

const (
	sideAgentKindSection sideAgentEntryKind = iota
	sideAgentKindAgent
)

// sideAgentTreeState is the panel's tree: which sections and sub-agents
// show their children, and the rows seen so far, whose branches start
// open.
type sideAgentTreeState struct {
	outline ui.OutlineState[sideAgentKey]
	known   map[sideAgentKey]bool
}

func newSideAgentTreeState() *sideAgentTreeState {
	return &sideAgentTreeState{known: make(map[sideAgentKey]bool)}
}

// ensureOpen opens branches the first time they show, and forgets those
// that are gone, so a branch the user closed stays closed.
func (st *sideAgentTreeState) ensureOpen(branches []sideAgentKey) {
	if st == nil {
		return
	}
	if st.known == nil {
		st.known = make(map[sideAgentKey]bool)
	}
	live := make(map[sideAgentKey]bool, len(branches))
	for _, branch := range branches {
		live[branch] = true
		if !st.known[branch] {
			st.known[branch] = true
			st.outline.Open.Add(branch)
		}
	}
	for branch := range st.known {
		if !live[branch] {
			delete(st.known, branch)
			st.outline.Open.Remove(branch)
		}
	}
}

// sideAgentOutlineModel is the outline of the sections: their headers at
// the top, and under each the agents that section holds, with each
// agent's sub-agents under it.
type sideAgentOutlineModel struct {
	roots      []sideAgentKey
	sections   map[string]sideAgentSection
	nodes      map[string]*sideAgentNode
	sectionFor map[string]string
	children   func(sideAgentKey) []sideAgentKey
}

func sideAgentOutlineModelFor(sections []sideAgentSection, roots []*sideAgentNode) sideAgentOutlineModel {
	byID := make(map[string]sideAgentSection, len(sections))
	m := sideAgentOutlineModel{
		sections:   byID,
		nodes:      make(map[string]*sideAgentNode),
		sectionFor: make(map[string]string),
	}
	for _, section := range sections {
		byID[section.id] = section
		m.roots = append(m.roots, sideAgentKey{kind: sideAgentKindSection, id: section.id})
	}
	var collect func(*sideAgentNode)
	collect = func(node *sideAgentNode) {
		if node == nil {
			return
		}
		m.nodes[node.agent.Address] = node
		for _, child := range node.children {
			collect(child)
		}
	}
	for _, root := range roots {
		collect(root)
	}
	m.children = func(key sideAgentKey) []sideAgentKey {
		switch key.kind {
		case sideAgentKindSection:
			section, ok := byID[key.id]
			if !ok {
				return nil
			}
			out := make([]sideAgentKey, 0, len(section.nodes))
			for _, node := range section.nodes {
				out = append(out, sideAgentKey{kind: sideAgentKindAgent, id: node.agent.Address})
			}
			if len(out) == 0 {
				return nil
			}
			return out
		case sideAgentKindAgent:
			node := m.nodes[key.id]
			if node == nil {
				return nil
			}
			var out []sideAgentKey
			for _, child := range node.children {
				if child.agent.Pinned {
					continue
				}
				out = append(out, sideAgentKey{kind: sideAgentKindAgent, id: child.agent.Address})
			}
			if len(out) == 0 {
				return nil
			}
			return out
		default:
			return nil
		}
	}
	var tag func(section string, node *sideAgentNode, pinned bool)
	tag = func(section string, node *sideAgentNode, pinned bool) {
		if node == nil {
			return
		}
		if _, ok := m.sectionFor[node.agent.Address]; ok {
			return
		}
		m.sectionFor[node.agent.Address] = section
		for _, child := range node.children {
			if child.agent.Pinned && !pinned {
				continue
			}
			tag(section, child, pinned)
		}
	}
	for _, section := range sections {
		pinned := section.id == "pinned"
		for _, node := range section.nodes {
			tag(section.id, node, pinned)
		}
	}
	return m
}

// branches lists every key with children, in the order the outline shows
// them with its branches open.
func (m sideAgentOutlineModel) branches() []sideAgentKey {
	var out []sideAgentKey
	var walk func(keys []sideAgentKey)
	walk = func(keys []sideAgentKey) {
		for _, key := range keys {
			children := m.children(key)
			if children == nil {
				continue
			}
			out = append(out, key)
			walk(children)
		}
	}
	walk(m.roots)
	return out
}

// rowOf returns the outline row showing key with the open branches, or -1.
func (m sideAgentOutlineModel) rowOf(state *ui.OutlineState[sideAgentKey], key sideAgentKey) int {
	row := 0
	var walk func(keys []sideAgentKey) bool
	walk = func(keys []sideAgentKey) bool {
		for _, k := range keys {
			if k == key {
				return true
			}
			row++
			children := m.children(k)
			if children == nil || state == nil || !state.Open.Has(k) {
				continue
			}
			if walk(children) {
				return true
			}
		}
		return false
	}
	if walk(m.roots) {
		return row
	}
	return -1
}

func renderAgents(rc *api.RenderContext) {
	if rc == nil || rc.C == nil || rc.App == nil {
		return
	}
	t := rc.C.Theme()
	agents := append([]api.Agent(nil), sideAgents(rc)...)
	spaces := sideSpaces(rc)
	spaceNames, tabNames := sideNames(spaces)
	shown := sideAgentPaneTerminals(spaces, sideActive(rc))
	compact := sideAgentCompact(rc)
	active := sideStr(rc.Props, "activeAgent")
	roots := sideAgentForest(agents)
	durations := sideAgentDurations(agents, time.Now())

	root := ui.Column(rc.C).Key("mods/agents/panel").Fill()
	view := ui.Local(root, "mods/agents/view", func() sideAgentView { return *newSideAgentView() })
	if view == nil {
		view = newSideAgentView()
	}
	tree := ui.Local(root, "mods/agents/tree", func() sideAgentTreeState { return *newSideAgentTreeState() })
	if tree == nil {
		tree = newSideAgentTreeState()
	}
	selected := ui.Local(root, "mods/agents/selected", func() int { return -1 })
	compactState := ui.Local(root, "mods/agents/compact-list", func() ui.ListState { return ui.ListState{} })
	compactSelected := ui.Local(root, "mods/agents/compact-selected", func() int { return -1 })

	root.Children(func() {
		if !compact {
			sideAgentHeader(rc, view)
		}
		if len(roots) == 0 {
			if !compact {
				ui.Text(rc.C, "No agent status yet. Agents show up here when they report their status.").
					FontSize(12).
					TextColor(t.TextMuted).
					Padding(12)
			}
			return
		}

		if compact {
			sideAgentCompactList(rc, compactState, compactSelected, roots, active, shown)
		} else {
			sideAgentOutline(rc, view, tree, selected, roots, active, spaceNames, tabNames, durations, shown)
		}
		if agentsNotice != "" {
			ui.Text(rc.C, agentsNotice).
				FontSize(t.FontSize-2).
				TextColor(t.Danger).
				Padding(4, 8).
				SingleLine().
				Tooltip(agentsNotice)
		}
	})
}

func sideAgentCompact(rc *api.RenderContext) bool {
	v, _ := rc.Props["compact"].(bool)
	return v
}

func sideAgentHeader(rc *api.RenderContext, view *sideAgentView) {
	t := rc.C.Theme()
	ui.Toolbar(rc.C, func() {
		ui.Text(rc.C, "Agents").FontSize(t.FontSize - 1).Bold().TextColor(t.TextMuted)
		ui.Spacer(rc.C)
		if ui.Button(rc.C, view.sort.glyph()).
			Key("mods/agents/sort").
			Height(22).
			Padding(0, 6).
			Radius(4).
			Label("Sort agents").
			Tooltip("Sort agents: " + view.sort.label()).
			Clicked() {
			view.sort = view.sort.next()
			rc.C.Invalidate()
		}
	}).Height(41).
		Padding(0, 12)
}

// sideAgentOutline draws the sections and their trees with the official
// outline: section headers pin while their rows scroll, and arrows fold
// sections and sub-agents.
func sideAgentOutline(rc *api.RenderContext, view *sideAgentView, tree *sideAgentTreeState, selected *int, roots []*sideAgentNode, active string, spaceNames, tabNames map[string]string, durations map[string]int64, shown map[string]bool) {
	if rc == nil || rc.C == nil {
		return
	}
	sections := sideAgentSections(roots, view.sort, spaceNames)
	model := sideAgentOutlineModelFor(sections, roots)
	if tree == nil {
		tree = newSideAgentTreeState()
	}
	if selected == nil {
		selected = new(int)
	}
	tree.ensureOpen(model.branches())
	tree.outline.List.Header = func(i int) bool {
		key := tree.outline.Item(i)
		return key.kind == sideAgentKindSection
	}
	*selected = model.rowOf(&tree.outline, sideAgentKey{kind: sideAgentKindAgent, id: active})
	tree.outline.List.Selected = selected

	ui.Outline(rc.C, &tree.outline, model.roots, model.children, func(key sideAgentKey) {
		sideAgentOutlineRow(rc, key, &model, tree, active, view.sort == sideAgentSortPriority, spaceNames, tabNames, durations, shown)
	}).Grow(1).
		MinHeight(0).
		KeepScrollbars(true)
}

// sideAgentOutlineRow draws one row of the outline: a section header, or
// an agent with its program, place, duration and status.
func sideAgentOutlineRow(rc *api.RenderContext, key sideAgentKey, model *sideAgentOutlineModel, tree *sideAgentTreeState, active string, statusCompact bool, spaceNames, tabNames map[string]string, durations map[string]int64, shown map[string]bool) {
	if model == nil || tree == nil {
		return
	}
	if key.kind == sideAgentKindSection {
		renderSideAgentSection(rc, key, model, tree)
		return
	}
	node := model.nodes[key.id]
	if node == nil {
		return
	}
	sideAgentRow(rc, key, node, model, tree, active == node.agent.Address, statusCompact && model.sectionFor[key.id] != "pinned", spaceNames, tabNames, durations, shown)
}

func renderSideAgentSection(rc *api.RenderContext, key sideAgentKey, model *sideAgentOutlineModel, tree *sideAgentTreeState) {
	section, ok := model.sections[key.id]
	if !ok {
		return
	}
	t := rc.C.Theme()
	color := t.TextMuted
	switch section.status {
	case "blocked":
		color = t.Danger
	case "done":
		color = t.Success
	case "working":
		color = t.Accent
	}
	head := ui.Row(rc.C).
		Key("mods/agents/section/"+section.id).
		FillWidth().
		Height(30).
		Padding(0, 8).
		Gap(6).
		AlignItems(ui.Center).
		Background(t.Background).
		OnClick(func() {
			if tree == nil {
				return
			}
			if tree.outline.Open.Has(key) {
				tree.outline.Open.Remove(key)
			} else {
				tree.outline.Open.Add(key)
			}
			rc.C.Invalidate()
		})
	head.Children(func() {
		ui.Text(rc.C, section.label).FontSize(11).Bold().TextColor(color).SingleLine()
		ui.Badge(rc.C, fmt.Sprintf("%d", len(section.nodes))).
			Tooltip(fmt.Sprintf("%d agents", len(section.nodes)))
		ui.Box(rc.C).Grow(1)
	})
}

func sideAgentRow(rc *api.RenderContext, key sideAgentKey, node *sideAgentNode, model *sideAgentOutlineModel, tree *sideAgentTreeState, selected bool, statusCompact bool, spaceNames, tabNames map[string]string, durations map[string]int64, shown map[string]bool) {
	t := rc.C.Theme()
	ag := node.agent
	row := ui.Row(rc.C.Key("mods/agents/"+ag.Address)).
		FillWidth().
		Height(28).
		Radius(8).
		Padding(0, 8).
		Gap(9).
		AlignItems(ui.Center).
		Cursor(ui.CursorPointer).
		OnClick(func() {
			if !shown[ag.Address] {
				sideAgentPeek(rc, ag)
				return
			}
			sideAgentFocus(rc, ag)
		}).
		ContextMenu(sideAgentMenu(rc, ag))
	hovered := row.Hovered()
	row.Children(func() {
		program, hasProgram := sideProgramFor(rc.App, ag, true)
		sideProgramBadge(rc.C, t, program, hasProgram, ag.Status)

		ui.Column(rc.C).Grow(1).Gap(1).Children(func() {
			ui.Row(rc.C).Gap(4).AlignItems(ui.Center).Children(func() {
				if ag.Pinned {
					color := t.Accent
					if selected {
						color = t.AccentText
					}
					ui.Text(rc.C, "★").FontSize(t.FontSize - 1).TextColor(color)
				}
				title := ui.Text(rc.C, sideAgentLabel(ag)).FontSize(t.FontSize).SingleLine()
				if selected {
					title.TextColor(t.AccentText)
				}
			})
			if meta := sideAgentMeta(ag, spaceNames, tabNames); meta != "" {
				color := t.TextMuted
				if selected {
					color = t.AccentText
				}
				ui.Text(rc.C, meta).
					FontSize(t.FontSize - 2).
					TextColor(color).
					SingleLine()
			}
		})

		if children := model.children(key); children != nil && !tree.outline.Open.Has(key) {
			if hidden := sideAgentVisibleDescendants(node); hidden > 0 {
				ui.Badge(rc.C, fmt.Sprintf("%d", hidden)).
					Tooltip("Fold sub-agents")
			}
		}

		if hovered {
			pinGlyph, pinLabel := "⚐", "Pin"
			if ag.Pinned {
				pinGlyph, pinLabel = "⚑", "Unpin"
			}
			ui.ButtonBase(rc.C).
				Key("mods/agents/pin/"+ag.Address).
				Height(18).
				Padding(0, 3).
				Radius(4).
				Label(pinLabel).
				Tooltip(pinLabel).
				OnClick(func() { sideAgentPin(rc, ag) }).
				Children(func() {
					color := t.TextMuted
					if ag.Pinned {
						color = t.Accent
					}
					ui.Text(rc.C, pinGlyph).FontSize(t.FontSize - 1).TextColor(color)
				})
		}

		if since, ok := durations[ag.Address]; ok {
			label := sideAgentDuration(since)
			color := t.TextMuted
			if selected {
				color = t.AccentText
			}
			ui.Text(rc.C, label).
				FontSize(10).
				TextColor(color).
				SingleLine().
				Tooltip("Status unchanged for " + label)
		}
		sideAgentStatusIndicator(rc, ag.Status, statusCompact, selected)
	})
}

// sideAgentCompactList draws the rail's rows: every agent once, pinned
// first, as a virtual list.
func sideAgentCompactList(rc *api.RenderContext, state *ui.ListState, selected *int, roots []*sideAgentNode, active string, shown map[string]bool) {
	t := rc.C.Theme()
	all := sideAgentFlatten(roots)
	ordered := make([]*sideAgentNode, 0, len(all))
	for _, node := range all {
		if node.agent.Pinned {
			ordered = append(ordered, node)
		}
	}
	for _, node := range all {
		if !node.agent.Pinned {
			ordered = append(ordered, node)
		}
	}
	if state == nil {
		state = &ui.ListState{}
	}
	if selected == nil {
		selected = new(int)
	}
	*selected = -1
	for i, node := range ordered {
		if active != "" && node.agent.Address == active {
			*selected = i
			break
		}
	}
	state.Key = func(i int) any { return ordered[i].agent.Address }
	state.Selected = selected

	ui.List(rc.C, state, len(ordered), func(i int) {
		node := ordered[i]
		ag := node.agent
		selected := active != "" && ag.Address == active
		row := ui.Row(rc.C.Key("mods/agents/compact/"+ag.Address)).
			FillWidth().
			Height(26).
			Radius(6).
			Padding(0, 8).
			Gap(4).
			AlignItems(ui.Center).
			Cursor(ui.CursorPointer).
			OnClick(func() {
				if !shown[ag.Address] {
					sideAgentPeek(rc, ag)
					return
				}
				sideAgentFocus(rc, ag)
			}).
			ContextMenu(sideAgentMenu(rc, ag))
		row.Children(func() {
			program, hasProgram := sideProgramFor(rc.App, ag, true)
			sideProgramBadge(rc.C, t, program, hasProgram, ag.Status)
			ui.Box(rc.C).Grow(1)
			color := t.TextMuted
			if selected {
				color = t.AccentText
			}
			ui.Text(rc.C, fmt.Sprintf("%d", i+1)).FontSize(10).TextColor(color)
			sideAgentStatusIndicator(rc, ag.Status, true, selected)
		})
	}).Grow(1).
		MinHeight(0).
		KeepScrollbars(true)
}

// sideAgentStatusIndicator is the right-side state, separate from the
// program icon: a spinner, a Needs you badge, a dot, or a ring.
func sideAgentStatusIndicator(rc *api.RenderContext, status string, compact bool, selected bool) {
	t := rc.C.Theme()
	accent := t.Accent
	if selected {
		accent = t.AccentText
	}
	switch sideAgentCanonicalStatus(status) {
	case "working":
		ui.Spinner(rc.C).FontSize(12).TextColor(accent).Tooltip("Working")
	case "blocked":
		text := "Needs you"
		if compact {
			text = "!"
		}
		ui.Badge(rc.C, text).
			Background(t.Danger.Mix(t.Background, 0.85)).
			TextColor(t.Danger).
			Tooltip("Needs you")
	case "done":
		ui.Text(rc.C, "●").FontSize(9).TextColor(accent).Tooltip("Done")
	default:
		color := t.TextMuted
		if selected {
			color = t.AccentText
		}
		ui.Text(rc.C, "○").FontSize(11).TextColor(color).Tooltip("Idle")
	}
}

// sideAgentPaneTerminals is the set of terminals shown in the active
// tab, so a row can focus one of them and peek at anything else.
func sideAgentPaneTerminals(spaces []api.Space, activeSpace string) map[string]bool {
	out := make(map[string]bool)
	for _, sp := range spaces {
		if sp.ID != activeSpace || len(sp.Tabs) == 0 {
			continue
		}
		i := sp.Active
		if i < 0 || i >= len(sp.Tabs) {
			i = 0
		}
		var walk func(*api.Node)
		walk = func(n *api.Node) {
			if n == nil {
				return
			}
			if n.Terminal != "" {
				out[n.Terminal] = true
			}
			walk(n.A)
			walk(n.B)
		}
		walk(sp.Tabs[i].Root)
	}
	return out
}

func sideAgentPeek(rc *api.RenderContext, ag api.Agent) {
	if err := rc.App.Run(api.CmdAgentPeek, map[string]any{"address": ag.Address}); err != nil {
		agentsNotice = err.Error()
		return
	}
	agentsNotice = ""
}

func sideAgentFocus(rc *api.RenderContext, ag api.Agent) {
	if err := rc.App.Run(api.CmdAgentFocus, map[string]any{"address": ag.Address}); err != nil {
		agentsNotice = err.Error()
		return
	}
	agentsNotice = ""
}

func sideAgentPin(rc *api.RenderContext, ag api.Agent) {
	if err := rc.App.Run(api.CmdAgentPin, map[string]any{"address": ag.Address}); err != nil {
		agentsNotice = err.Error()
		return
	}
	agentsNotice = ""
}

func sideAgentMenu(rc *api.RenderContext, ag api.Agent) func(*ui.Menu) {
	return func(m *ui.Menu) {
		pinLabel := "Pin"
		if ag.Pinned {
			pinLabel = "Unpin"
		}
		if m.Item(pinLabel).Checked(ag.Pinned).Chosen() {
			sideAgentPin(rc, ag)
		}
		if m.Item("Peek").Chosen() {
			if err := rc.App.Run(api.CmdAgentPeek, map[string]any{"address": ag.Address}); err != nil {
				agentsNotice = err.Error()
			} else {
				agentsNotice = ""
			}
		}
	}
}

// sideAgentMeta is the app and the space/tab an agent belongs to.
func sideAgentMeta(ag api.Agent, spaceNames, tabNames map[string]string) string {
	var parts []string
	if ag.App != "" && ag.App != sideAgentLabel(ag) {
		parts = append(parts, ag.App)
	}
	var where []string
	if ag.Space != "" {
		name := spaceNames[ag.Space]
		if name == "" {
			name = ag.Space
		}
		where = append(where, name)
	}
	if ag.Tab != "" {
		name := tabNames[ag.Tab]
		if name == "" {
			name = ag.Tab
		}
		where = append(where, name)
	}
	if len(where) > 0 {
		parts = append(parts, strings.Join(where, " / "))
	}
	return strings.Join(parts, " · ")
}
