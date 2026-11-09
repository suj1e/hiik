package builtin

import (
	"testing"
	"time"

	"hiik/mods/api"
)

func sideTestFlatten(nodes []*sideAgentNode) []string {
	var out []string
	var walk func(*sideAgentNode)
	walk = func(n *sideAgentNode) {
		out = append(out, n.agent.Address)
		for _, child := range n.children {
			if child.agent.Pinned {
				continue
			}
			walk(child)
		}
	}
	for _, node := range nodes {
		walk(node)
	}
	return out
}

func sideTestForestRoots(roots []*sideAgentNode) []string {
	out := make([]string, 0, len(roots))
	for _, node := range roots {
		out = append(out, node.agent.Address)
	}
	return out
}

func TestSideAgentForest(t *testing.T) {
	agents := []api.Agent{
		{Address: "t1"},
		{Address: "t2", Parent: "t1"},
		{Address: "t3", Parent: "t1"},
		{Address: "t4", Parent: "t2"},
		{Address: "t5", Parent: "t9"}, // parent outside the list
		{Address: "t6", Parent: "t7"}, // a cycle
		{Address: "t7", Parent: "t6"},
		{Address: "t1", Title: "duplicate"}, // the same address twice
	}
	roots := sideAgentForest(agents)
	got := sideTestForestRoots(roots)
	want := []string{"t1", "t5", "t6"}
	if len(got) != len(want) {
		t.Fatalf("roots = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("roots = %v, want %v", got, want)
		}
	}
	if len(roots[0].children) != 2 || roots[0].children[0].agent.Address != "t2" || roots[0].children[1].agent.Address != "t3" {
		t.Fatalf("t1 children = %v", roots[0].children)
	}
	if len(roots[0].children[0].children) != 1 || roots[0].children[0].children[0].agent.Address != "t4" {
		t.Fatalf("t2 children = %v", roots[0].children[0].children)
	}
	all := sideTestFlatten(roots)
	seen := make(map[string]bool, len(all))
	for _, address := range all {
		if seen[address] {
			t.Errorf("%s is drawn twice: %v", address, all)
		}
		seen[address] = true
	}
	if len(all) != 7 {
		t.Errorf("drawn %d agents, want 7: %v", len(all), all)
	}
	flat := sideAgentFlatten(roots)
	if len(flat) != 7 {
		t.Errorf("flattened %d agents, want 7", len(flat))
	}
}

func TestSideAgentSections(t *testing.T) {
	agents := []api.Agent{
		{Address: "pin", Status: "blocked", Pinned: true},
		{Address: "blocked", Status: "blocked"},
		{Address: "error", Status: "error"},
		{Address: "done", Status: "done"},
		{Address: "working", Status: "working"},
		{Address: "idle", Status: "idle"},
	}
	sections := sideAgentSections(sideAgentForest(agents), sideAgentSortPriority, nil)
	want := []struct {
		label string
		n     int
	}{
		{"Pinned", 1},
		{"Needs you", 2},
		{"Done", 1},
		{"Working", 1},
		{"Idle", 1},
	}
	if len(sections) != len(want) {
		t.Fatalf("sections = %d, want %d: %#v", len(sections), len(want), sections)
	}
	for i, w := range want {
		if sections[i].label != w.label || len(sections[i].nodes) != w.n {
			t.Errorf("section %d = %q/%d, want %q/%d", i, sections[i].label, len(sections[i].nodes), w.label, w.n)
		}
	}
}

func TestSideAgentSectionsUseOwnStatus(t *testing.T) {
	agents := []api.Agent{
		{Address: "parent", Status: "done"},
		{Address: "child", Parent: "parent", Status: "working"},
	}
	sections := sideAgentSections(sideAgentForest(agents), sideAgentSortPriority, nil)
	if len(sections) != 1 || sections[0].label != "Done" {
		t.Fatalf("sections = %#v, want one Done section", sections)
	}
	if len(sections[0].nodes) != 1 || sections[0].nodes[0].agent.Address != "parent" {
		t.Fatalf("nodes = %#v, want the parent", sections[0].nodes)
	}
}

func TestSideAgentSectionsGroupedBySpace(t *testing.T) {
	agents := []api.Agent{
		{Address: "pin", Pinned: true, Space: "s1"},
		{Address: "a", Space: "s1"},
		{Address: "b", Space: "s2"},
		{Address: "c", Space: "s1"},
		{Address: "d"},
	}
	names := map[string]string{"s1": "One", "s2": "Two"}
	sections := sideAgentSections(sideAgentForest(agents), sideAgentSortGrouped, names)
	want := []struct {
		label   string
		members []string
	}{
		{"Pinned", []string{"pin"}},
		{"One", []string{"a", "c"}},
		{"Two", []string{"b"}},
		{"No space", []string{"d"}},
	}
	if len(sections) != len(want) {
		t.Fatalf("sections = %d, want %d: %#v", len(sections), len(want), sections)
	}
	for i, w := range want {
		if sections[i].label != w.label {
			t.Errorf("section %d label = %q, want %q", i, sections[i].label, w.label)
		}
		got := make([]string, 0, len(sections[i].nodes))
		for _, node := range sections[i].nodes {
			got = append(got, node.agent.Address)
		}
		if len(got) != len(w.members) {
			t.Fatalf("section %q = %v, want %v", w.label, got, w.members)
		}
		for j := range w.members {
			if got[j] != w.members[j] {
				t.Errorf("section %q = %v, want %v", w.label, got, w.members)
			}
		}
	}
}

func TestSideAgentPinnedChildIsNotNested(t *testing.T) {
	agents := []api.Agent{
		{Address: "parent"},
		{Address: "child", Parent: "parent", Pinned: true},
		{Address: "sibling", Parent: "parent"},
	}
	roots := sideAgentForest(agents)
	sections := sideAgentSections(roots, sideAgentSortPriority, nil)
	var pinned []string
	for _, section := range sections {
		if section.id != "pinned" {
			continue
		}
		for _, node := range section.nodes {
			pinned = append(pinned, node.agent.Address)
		}
	}
	if len(pinned) != 1 || pinned[0] != "child" {
		t.Fatalf("pinned = %v, want [child]", pinned)
	}
	visible := 0
	for _, child := range roots[0].children {
		if !child.agent.Pinned {
			visible++
			if child.agent.Address != "sibling" {
				t.Errorf("parent keeps %s as a visible child", child.agent.Address)
			}
		}
	}
	if visible != 1 {
		t.Fatalf("parent has %d visible children, want sibling only", visible)
	}
	if n := sideAgentVisibleDescendants(roots[0]); n != 1 {
		t.Errorf("visible descendants = %d, want 1", n)
	}
}

func TestSideAgentDuration(t *testing.T) {
	cases := []struct {
		ms   int64
		want string
	}{
		{-time.Second.Milliseconds(), "<1m"},
		{30 * time.Second.Milliseconds(), "<1m"},
		{7 * time.Minute.Milliseconds(), "7m"},
		{59 * time.Minute.Milliseconds(), "59m"},
		{2 * time.Hour.Milliseconds(), "2h"},
		{(3 * 24 * time.Hour).Milliseconds(), "3d"},
	}
	for _, tc := range cases {
		if got := sideAgentDuration(tc.ms); got != tc.want {
			t.Errorf("duration(%d) = %q, want %q", tc.ms, got, tc.want)
		}
	}
}

func TestSideAgentStatusClockKeepsStampUntilStatusChanges(t *testing.T) {
	clock := newSideAgentStatusClock()
	start := time.Unix(1_700_000_000, 0)
	agents := []api.Agent{{Address: "t1", Status: "working"}}
	clock.durations(agents, start)

	later := start.Add(7 * time.Minute)
	if got := clock.durations(agents, later)["t1"]; got != 7*time.Minute.Milliseconds() {
		t.Fatalf("duration = %d, want 7m", got)
	}
	changed := []api.Agent{{Address: "t1", Status: "blocked"}}
	if got := clock.durations(changed, later)["t1"]; got != 0 {
		t.Fatalf("changed status duration = %d, want 0", got)
	}
	if got := clock.durations(changed, later.Add(2*time.Hour))["t1"]; got != 2*time.Hour.Milliseconds() {
		t.Fatalf("changed status duration = %d, want 2h", got)
	}
}

func TestSideAgentViewSort(t *testing.T) {
	v := newSideAgentView()
	if v.sort != sideAgentSortPriority {
		t.Fatalf("default sort = %v, want priority", v.sort)
	}
	if v.sort.next() != sideAgentSortGrouped || sideAgentSortGrouped.next() != sideAgentSortPriority {
		t.Error("sort does not cycle between priority and grouped")
	}
}

func TestSideAgentTreeStartsBranchesOpenAndKeepsClosed(t *testing.T) {
	agents := []api.Agent{
		{Address: "parent"},
		{Address: "child", Parent: "parent"},
	}
	roots := sideAgentForest(agents)
	sections := sideAgentSections(roots, sideAgentSortPriority, nil)
	model := sideAgentOutlineModelFor(sections, roots)
	branches := model.branches()
	if len(branches) != 2 {
		t.Fatalf("branches = %d, want the section and the parent: %v", len(branches), branches)
	}
	tree := newSideAgentTreeState()
	tree.ensureOpen(branches)
	for _, branch := range branches {
		if !tree.outline.Open.Has(branch) {
			t.Fatalf("branch %v did not start open", branch)
		}
	}
	tree.outline.Open.Remove(branches[1])
	tree.ensureOpen(branches)
	if tree.outline.Open.Has(branches[1]) {
		t.Fatal("a branch the user closed reopened")
	}
	if got := model.rowOf(&tree.outline, sideAgentKey{kind: sideAgentKindAgent, id: "child"}); got != -1 {
		t.Fatalf("folded child is at row %d, want hidden", got)
	}
}

func TestAgentsPanelRendersSubAgents(t *testing.T) {
	agentsNotice = ""
	app := &btPanelApp{agents: []api.Agent{
		{Address: "t1", Title: "Parent agent", Status: "blocked"},
		{Address: "t2", Title: "Child agent", Status: "working", Parent: "t1"},
	}}
	p, ok := api.Panels()[api.PanelAgents]
	if !ok || p.Render == nil {
		t.Fatalf("panel %s is not registered", api.PanelAgents)
	}
	btRenderPanel(t, p, app, nil, "Child agent")
}
