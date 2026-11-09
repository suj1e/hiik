package core

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"hiik/mods/api"
)

// The core keeps a terminal's attention state in labels, so it survives a
// reconnect and an app restart.
const (
	labelSeen   = "herdr.seen"   // the last turn the user looked at
	labelPinned = "herdr.pinned" // "1" while the agent is pinned
)

// agentSeenTurn reads the seen label's turn; a missing or bad value is zero.
func agentSeenTurn(labels map[string]string) uint64 {
	return labelUint(labels, labelSeen)
}

// agentPinned reports whether a terminal carries the pin label. The older
// hiik.pinned name is still read so terminals labelled before this version
// keep their place.
func agentPinned(labels map[string]string) bool {
	return labelOn(labels, labelPinned) || labelOn(labels, "hiik.pinned")
}

// labelUint reads an unsigned decimal label; a missing or bad value is zero.
func labelUint(labels map[string]string, key string) uint64 {
	if labels == nil {
		return 0
	}
	n, err := strconv.ParseUint(strings.TrimSpace(labels[key]), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// labelOn reports whether a label is the core's true value.
func labelOn(labels map[string]string, key string) bool {
	switch strings.TrimSpace(labels[key]) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

// setAgentLabel writes labels on an agent's terminal. Without a core there
// is nowhere durable to write, so the model keeps its change. A core that
// refuses the label (a remote terminal whose connection dropped, for
// example) returns its message so the caller can show it.
func (a *App) setAgentLabel(address string, labels map[string]string) error {
	a.mu.RLock()
	c := a.core
	a.mu.RUnlock()
	if c == nil {
		return nil
	}
	ctx, cancel := a.callContext()
	defer cancel()
	if err := c.Label(ctx, address, labels); err != nil {
		return fmt.Errorf("label %s: %w", address, err)
	}
	return nil
}

// orderAgents groups sub-agents under the terminal that started them and
// sorts the result: pinned groups first, then blocked > done > working >
// idle. A parent's status rolls up from its children, so a blocked child
// marks its parent's group. The order is the list order agent.next and
// agent.prev follow.
func orderAgents(list []api.Agent) []api.Agent {
	if len(list) < 2 {
		return list
	}
	byAddr := make(map[string]int, len(list))
	for i := range list {
		byAddr[list[i].Address] = i
	}
	// A parent that is gone or forms a cycle cannot group anything.
	for i := range list {
		parent := strings.TrimSpace(list[i].Parent)
		if parent == list[i].Address {
			parent = ""
		}
		if parent != "" {
			if _, ok := byAddr[parent]; !ok {
				parent = ""
			}
		}
		if parent != "" && parentCycle(list, byAddr, i) {
			parent = ""
		}
		list[i].Parent = parent
	}

	children := make(map[string][]int)
	for i := range list {
		if parent := list[i].Parent; parent != "" {
			children[parent] = append(children[parent], i)
		}
	}

	// rolled[i] is the highest-priority status among i and its descendants,
	// with i's own status winning ties.
	rolled := make(map[int]string, len(list))
	var walk func(i int) (int, string)
	walk = func(i int) (int, string) {
		best, bestStatus := priority(list[i].Status), list[i].Status
		for _, child := range children[list[i].Address] {
			if p, status := walk(child); p > best {
				best, bestStatus = p, status
			}
		}
		rolled[i] = bestStatus
		return best, bestStatus
	}

	var roots []int
	for i := range list {
		if list[i].Parent == "" {
			roots = append(roots, i)
			walk(i)
		}
	}

	var groupPinned func(i int) bool
	groupPinned = func(i int) bool {
		if list[i].Pinned {
			return true
		}
		for _, child := range children[list[i].Address] {
			if groupPinned(child) {
				return true
			}
		}
		return false
	}

	sort.SliceStable(roots, func(x, y int) bool {
		a, b := roots[x], roots[y]
		if pa, pb := groupPinned(a), groupPinned(b); pa != pb {
			return pa
		}
		if pa, pb := priority(rolled[a]), priority(rolled[b]); pa != pb {
			return pa > pb
		}
		return list[a].Address < list[b].Address
	})

	out := make([]api.Agent, 0, len(list))
	var add func(i int)
	add = func(i int) {
		ag := list[i]
		ag.Status = rolled[i]
		out = append(out, ag)
		kids := append([]int(nil), children[list[i].Address]...)
		sort.SliceStable(kids, func(x, y int) bool {
			a, b := kids[x], kids[y]
			if list[a].Pinned != list[b].Pinned {
				return list[a].Pinned
			}
			if pa, pb := priority(rolled[a]), priority(rolled[b]); pa != pb {
				return pa > pb
			}
			return list[a].Address < list[b].Address
		})
		for _, child := range kids {
			add(child)
		}
	}
	for _, root := range roots {
		add(root)
	}
	return out
}

// parentCycle reports whether following start's parents returns to an
// address already seen.
func parentCycle(list []api.Agent, byAddr map[string]int, start int) bool {
	seen := map[int]bool{start: true}
	for parent := list[start].Parent; parent != ""; {
		i, ok := byAddr[parent]
		if !ok {
			return false
		}
		if seen[i] {
			return true
		}
		seen[i] = true
		parent = list[i].Parent
	}
	return false
}
