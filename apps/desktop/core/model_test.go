package core

import (
	"testing"

	"hiik/core/store"
	"hiik/mods/api"
)

func TestCreateSpacesUsesStableUniqueIDs(t *testing.T) {
	m := new(model)
	s1, t1, p1 := m.createSpace("api", "/tmp/api", "")
	s2, t2, p2 := m.createSpace("", "/src/site", "box")
	if s1 != "s1" || t1 != "t1" || p1 != "p1" {
		t.Fatalf("first IDs = %s %s %s", s1, t1, p1)
	}
	if s2 != "s2" || t2 != "t2" || p2 != "p2" {
		t.Fatalf("second IDs = %s %s %s", s2, t2, p2)
	}
	if m.activeSpace != s2 {
		t.Fatalf("active space = %q, want %q", m.activeSpace, s2)
	}
	if m.spaces[1].Name != "site" {
		t.Fatalf("derived name = %q", m.spaces[1].Name)
	}
	if tab := activeTab(&m.spaces[1]); tab == nil || tab.Root == nil || tab.Root.ID != p2 || tab.Focus != p2 {
		t.Fatalf("new space layout = %#v", m.spaces[1])
	}
}

func TestRenameSelectAndMoveSpaces(t *testing.T) {
	m := new(model)
	m.createSpace("", "/tmp/api", "")
	m.createSpace("db", "/tmp/db", "")
	m.createSpace("web", "/tmp/web", "")

	if err := m.renameSpace("s1", ""); err != nil {
		t.Fatal(err)
	}
	if m.spaces[0].Name != "api" {
		t.Fatalf("derived rename = %q", m.spaces[0].Name)
	}
	if err := m.renameSpace("s1", "work"); err != nil {
		t.Fatal(err)
	}
	if m.spaces[0].Name != "work" {
		t.Fatalf("rename = %q", m.spaces[0].Name)
	}

	// Move the active space up one and then to the front.
	if err := m.moveSpace("s3", -1, nil); err != nil {
		t.Fatal(err)
	}
	if got := spaceIDs(m); got != "s1,s3,s2" {
		t.Fatalf("after move = %s", got)
	}
	to := 0
	if err := m.moveSpace("s3", 0, &to); err != nil {
		t.Fatal(err)
	}
	if got := spaceIDs(m); got != "s3,s1,s2" {
		t.Fatalf("after move to 0 = %s", got)
	}
	if m.activeSpace != "s3" {
		t.Fatalf("move changed the active space: %q", m.activeSpace)
	}

	if err := m.selectSpaceDelta(1); err != nil {
		t.Fatal(err)
	}
	if m.activeSpace != "s1" {
		t.Fatalf("next space = %q", m.activeSpace)
	}
	if err := m.selectSpace("s2"); err != nil {
		t.Fatal(err)
	}
	if err := m.selectSpaceDelta(1); err != nil {
		t.Fatal(err)
	}
	if m.activeSpace != "s3" {
		t.Fatalf("wrapped space = %q", m.activeSpace)
	}

	terms, err := m.closeSpace("s3")
	if err != nil {
		t.Fatal(err)
	}
	if len(terms) != 0 {
		t.Fatalf("closing a terminal-less space killed %v", terms)
	}
	if got := spaceIDs(m); got != "s1,s2" {
		t.Fatalf("after close = %s", got)
	}
	if m.activeSpace != "s1" {
		t.Fatalf("active after closing the active space = %q", m.activeSpace)
	}
}

func TestSplitAndClosePanes(t *testing.T) {
	m := new(model)
	m.createSpace("api", "/tmp/api", "")
	m.setTerminal("p1", "t1")

	newID, err := m.splitPane("p1", false)
	if err != nil {
		t.Fatal(err)
	}
	if newID != "p2" {
		t.Fatalf("new pane = %q", newID)
	}
	root := m.spaces[0].Tabs[0].Root
	if !isLeaf(root.A) || !isLeaf(root.B) {
		t.Fatalf("split children = %#v", root)
	}
	if root.Vertical || root.Ratio != 0.5 {
		t.Fatalf("split = %#v", root)
	}
	if root.A.ID != "p1" || root.B.ID != "p2" || root.ID != "p3" {
		t.Fatalf("split IDs = %#v", root)
	}
	if m.spaces[0].Tabs[0].Focus != "p2" || m.ui.Focus != "p2" {
		t.Fatalf("focus after split = %q / %q", m.spaces[0].Tabs[0].Focus, m.ui.Focus)
	}

	if _, err := m.splitPane("p2", true); err != nil {
		t.Fatal(err)
	}
	root = m.spaces[0].Tabs[0].Root
	if !root.B.Vertical {
		t.Fatalf("down split is not vertical: %#v", root.B)
	}
	m.setTerminal("p2", "t2")
	m.setTerminal("p4", "t3")

	terms, err := m.closePane("p2")
	if err != nil {
		t.Fatal(err)
	}
	if len(terms) != 1 || terms[0] != "t2" {
		t.Fatalf("closed terminals = %v", terms)
	}
	root = m.spaces[0].Tabs[0].Root
	if root.ID != "p3" || root.B.ID != "p4" {
		t.Fatalf("tree after close = %#v", root)
	}

	terms, err = m.closePane("p1")
	if err != nil {
		t.Fatal(err)
	}
	if len(terms) != 1 || terms[0] != "t1" {
		t.Fatalf("closed terminals = %v", terms)
	}
	if m.spaces[0].Tabs[0].Root.ID != "p4" {
		t.Fatalf("root after promoting the sibling = %#v", m.spaces[0].Tabs[0].Root)
	}

	terms, err = m.closePane("p4")
	if err != nil {
		t.Fatal(err)
	}
	if len(terms) != 1 || terms[0] != "t3" {
		t.Fatalf("closed terminals = %v", terms)
	}
	if len(m.spaces) != 0 {
		t.Fatalf("closing the last pane left the space: %#v", m.spaces)
	}
}

func TestZoomFocusGoAndSwap(t *testing.T) {
	m := new(model)
	m.createSpace("api", "/tmp/api", "")
	if _, err := m.splitPane("p1", false); err != nil { // p1 | p2
		t.Fatal(err)
	}
	if _, err := m.splitPane("p2", true); err != nil { // p1 | (p2 / p4)
		t.Fatal(err)
	}
	m.setTerminal("p1", "t1")
	m.setTerminal("p2", "t2")
	m.setTerminal("p4", "t4")

	if err := m.focusPane("p1"); err != nil {
		t.Fatal(err)
	}
	if m.ui.Focus != "p1" || m.spaces[0].Tabs[0].Focus != "p1" {
		t.Fatalf("focus = %q", m.ui.Focus)
	}

	next, err := m.goPane("p1", "right")
	if err != nil {
		t.Fatal(err)
	}
	if next != "p2" {
		t.Fatalf("right from p1 = %q", next)
	}
	next, err = m.goPane("p4", "up")
	if err != nil {
		t.Fatal(err)
	}
	if next != "p2" {
		t.Fatalf("up from p4 = %q", next)
	}
	if _, err := m.goPane("p1", "up"); err == nil {
		t.Fatal("going up from the top row should fail")
	}

	zoomed, err := m.zoomPane("p1")
	if err != nil {
		t.Fatal(err)
	}
	if !zoomed || m.ui.Zoomed != "p1" {
		t.Fatalf("zoom = %v %q", zoomed, m.ui.Zoomed)
	}
	zoomed, err = m.zoomPane("p1")
	if err != nil {
		t.Fatal(err)
	}
	if zoomed || m.ui.Zoomed != "" {
		t.Fatalf("unzoom = %v %q", zoomed, m.ui.Zoomed)
	}

	if err := m.swapPanes("p1", "p2"); err != nil {
		t.Fatal(err)
	}
	if node := findNode(m.spaces[0].Tabs[0].Root, "p1"); node.Terminal != "t2" {
		t.Fatalf("p1 terminal after swap = %q", node.Terminal)
	}
	if node := findNode(m.spaces[0].Tabs[0].Root, "p2"); node.Terminal != "t1" {
		t.Fatalf("p2 terminal after swap = %q", node.Terminal)
	}
	if m.ui.Focus != "p1" {
		t.Fatalf("swap moved focus to %q", m.ui.Focus)
	}

	if err := m.setRatio("p3", 0.25); err != nil {
		t.Fatal(err)
	}
	if got := findNode(m.spaces[0].Tabs[0].Root, "p3").Ratio; got != 0.25 {
		t.Fatalf("ratio = %v", got)
	}
	if err := m.setRatio("p1", 0.4); err != nil {
		t.Fatal(err)
	}
	if got := findNode(m.spaces[0].Tabs[0].Root, "p3").Ratio; got != 0.4 {
		t.Fatalf("ratio through a leaf = %v", got)
	}
	if err := m.setRatio("p1", 1.5); err == nil {
		t.Fatal("an out-of-range ratio should fail")
	}

	m.createSpace("solo", "/tmp/solo", "")
	if err := m.setRatio("p6", 0.3); err == nil {
		t.Fatal("a pane without a split should not resize")
	}
}

func TestTabOps(t *testing.T) {
	m := new(model)
	m.createSpace("api", "/tmp/api", "")
	m.setTerminal("p1", "t1")

	tabID, paneID, err := m.createTab("s1", "logs")
	if err != nil {
		t.Fatal(err)
	}
	if tabID != "t2" || paneID != "p2" {
		t.Fatalf("new tab = %s %s", tabID, paneID)
	}
	sp := &m.spaces[0]
	if sp.Active != 1 || sp.Tabs[1].Name != "logs" {
		t.Fatalf("tab state = %#v", sp)
	}

	if err := m.renameTab("s1", "t2", ""); err != nil {
		t.Fatal(err)
	}
	if sp.Tabs[1].Name != "2" {
		t.Fatalf("derived tab name = %q", sp.Tabs[1].Name)
	}

	if err := m.selectTab("s1", "t1"); err != nil {
		t.Fatal(err)
	}
	if sp.Active != 0 || m.ui.Focus != "p1" {
		t.Fatalf("select tab = %d %q", sp.Active, m.ui.Focus)
	}
	if err := m.selectTabDelta("s1", 1); err != nil {
		t.Fatal(err)
	}
	if sp.Active != 1 {
		t.Fatalf("next tab = %d", sp.Active)
	}

	to := 1
	if err := m.moveTab("s1", "t1", 0, &to); err != nil {
		t.Fatal(err)
	}
	if sp.Tabs[0].ID != "t2" || sp.Tabs[1].ID != "t1" {
		t.Fatalf("tabs after move = %v", tabIDs(sp))
	}
	if sp.Active != 0 {
		t.Fatalf("active tab after reorder = %d", sp.Active)
	}

	terms, err := m.closeTab("s1", "t1")
	if err != nil {
		t.Fatal(err)
	}
	if len(terms) != 1 || terms[0] != "t1" {
		t.Fatalf("closed terminals = %v", terms)
	}
	if len(sp.Tabs) != 1 || sp.Active != 0 {
		t.Fatalf("after tab close = %#v", sp)
	}

	terms, err = m.closeTab("s1", "t2")
	if err != nil {
		t.Fatal(err)
	}
	if len(terms) != 0 {
		t.Fatalf("empty tab terminals = %v", terms)
	}
	if len(m.spaces) != 0 || m.activeSpace != "" {
		t.Fatalf("closing the last tab left the space: %#v", m.spaces)
	}
}

func TestModelFromStateRepairsIDsAndFocus(t *testing.T) {
	st := store.State{
		Version:     1,
		ActiveSpace: "missing",
		Spaces: []store.Space{
			{ID: "", Name: "", Folder: "/home/me/proj", Tabs: nil},
			{ID: "", Name: "second", Folder: "/x", ActiveTab: 9, Tabs: []store.Tab{
				{ID: "t1", Name: "", Focus: "nope", Root: &store.Node{
					ID: "p1", Vertical: true, Ratio: 4,
					A: &store.Node{ID: "p1", Terminal: "t9"},
					B: &store.Node{ID: "", Terminal: "t10"},
				}},
			}},
		},
	}
	m := modelFromState(st)
	if len(m.spaces) != 2 {
		t.Fatalf("spaces = %d", len(m.spaces))
	}
	if m.spaces[0].ID == "" || m.spaces[0].ID == m.spaces[1].ID {
		t.Fatalf("space IDs = %q %q", m.spaces[0].ID, m.spaces[1].ID)
	}
	if m.spaces[0].Name != "proj" {
		t.Fatalf("derived name = %q", m.spaces[0].Name)
	}
	if m.activeSpace != m.spaces[0].ID {
		t.Fatalf("active space = %q", m.activeSpace)
	}

	tab := &m.spaces[1].Tabs[0]
	if tab.Name != "1" {
		t.Fatalf("derived tab name = %q", tab.Name)
	}
	if tab.Focus == "" || !isLeaf(findNode(tab.Root, tab.Focus)) {
		t.Fatalf("focus = %q is not a leaf", tab.Focus)
	}
	seen := map[string]bool{}
	var dup bool
	walkNode(tab.Root, func(n *api.Node) {
		if seen[n.ID] {
			dup = true
		}
		seen[n.ID] = true
	})
	if dup {
		t.Fatalf("duplicate node IDs after normalize: %#v", tab.Root)
	}
	if tab.Root.Ratio != 0.5 {
		t.Fatalf("ratio = %v", tab.Root.Ratio)
	}
	if m.spaces[1].Active != 0 {
		t.Fatalf("active tab = %d", m.spaces[1].Active)
	}
	if m.ui.Focus == "" || !m.paneExists(m.ui.Focus) {
		t.Fatalf("ui focus = %q", m.ui.Focus)
	}
}

func TestAgentsOrderAndPin(t *testing.T) {
	m := new(model)
	m.setAgents([]api.Agent{
		{Address: "t9", Status: "done"},
		{Address: "t8", Pinned: true},
		{Address: "t7"},
	})
	if got := agentIDs(m); got != "t8,t9,t7" {
		t.Fatalf("sorted agents = %s", got)
	}
	ag, ok := m.moveAgent(1)
	if !ok || ag.Address != "t8" {
		t.Fatalf("first next = %#v", ag)
	}
	ag, _ = m.moveAgent(1)
	if ag.Address != "t9" {
		t.Fatalf("second next = %#v", ag)
	}

	pinned, err := m.pinAgent("t9")
	if err != nil {
		t.Fatal(err)
	}
	if !pinned {
		t.Fatal("pin did not set")
	}
	if got := agentIDs(m); got != "t9,t8,t7" {
		t.Fatalf("after pin = %s", got)
	}
	if m.activeAgent != "t9" {
		t.Fatalf("pin changed the selection: %q", m.activeAgent)
	}

	if err := m.focusAgent("t9"); err != nil {
		t.Fatal(err)
	}
	if m.agent("t9").Status != "idle" || !m.agent("t9").Seen {
		t.Fatalf("focused agent = %#v", m.agent("t9"))
	}
	if _, err := m.pinAgent("nope"); err == nil {
		t.Fatal("pinning an unknown agent should fail")
	}
}

func spaceIDs(m *model) string {
	out := ""
	for i, sp := range m.spaces {
		if i > 0 {
			out += ","
		}
		out += sp.ID
	}
	return out
}

func tabIDs(sp *api.Space) string {
	out := ""
	for i, tab := range sp.Tabs {
		if i > 0 {
			out += ","
		}
		out += tab.ID
	}
	return out
}

func agentIDs(m *model) string {
	out := ""
	for i, ag := range m.agents {
		if i > 0 {
			out += ","
		}
		out += ag.Address
	}
	return out
}
