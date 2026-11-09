package core

import (
	"strings"
	"testing"

	"hiik/mods/api"
)

func TestCommandRegistryIsUnique(t *testing.T) {
	a := testApp()
	seen := make(map[string]bool)
	for _, c := range a.Commands() {
		if c.ID == "" {
			t.Fatal("command without an ID")
		}
		if seen[c.ID] {
			t.Fatalf("duplicate command %q", c.ID)
		}
		if c.Title == "" {
			t.Fatalf("command %q has no title", c.ID)
		}
		if c.Run == nil {
			t.Fatalf("command %q has no Run", c.ID)
		}
		seen[c.ID] = true
	}
	for _, id := range []string{
		api.CmdSpaceNew, api.CmdSpaceClose, api.CmdSpaceRename,
		api.CmdSpaceMove, api.CmdSpaceSelect,
		api.CmdTabNew, api.CmdTabClose, api.CmdTabRename,
		api.CmdTabMove, api.CmdTabSelect,
		api.CmdPaneSplit, api.CmdPaneClose, api.CmdPaneZoom,
		api.CmdPaneFocus, api.CmdPaneGo, api.CmdPaneSwap, "pane.resize",
		"terminal.keep", "terminal.kill", "terminal.label", "terminal.peek",
		api.CmdAgentNext, api.CmdAgentPrev, api.CmdAgentFocus,
		api.CmdAgentPin, api.CmdAgentPeek,
		api.CmdPalette, api.CmdSettings, api.CmdAbout, "app.set",
		api.CmdRetryCore,
	} {
		if !seen[id] {
			t.Fatalf("registry is missing %q", id)
		}
	}
}

func TestUnknownCommand(t *testing.T) {
	a := testApp()
	err := a.Run("space.explode", nil)
	if err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("err = %v", err)
	}
}

func TestCommandArgumentValidation(t *testing.T) {
	a := testApp()
	m := &a.model
	m.createSpace("api", "/tmp/api", "")

	cases := []struct {
		name string
		args map[string]any
		want string
	}{
		{api.CmdSpaceSelect, map[string]any{"nope": 1}, "unknown argument"},
		{api.CmdSpaceRename, map[string]any{"name": 5}, "must be a string"},
		{api.CmdSpaceSelect, map[string]any{"delta": "1"}, "must be an integer"},
		{api.CmdTabNew, map[string]any{"argv": "bash"}, "must be a list of strings"},
		{api.CmdSpaceMove, map[string]any{}, "needs delta or to"},
		{api.CmdSpaceMove, map[string]any{"delta": 1, "to": 0}, "not both"},
		{api.CmdSpaceSelect, map[string]any{"id": "s1", "delta": 1}, "not both"},
		{api.CmdPaneGo, map[string]any{"direction": "sideways"}, "direction"},
		{api.CmdPaneSplit, map[string]any{"direction": "left"}, "right or down"},
	}
	for _, tc := range cases {
		err := a.Run(tc.name, tc.args)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s %v: err = %v, want %q", tc.name, tc.args, err, tc.want)
		}
	}
}

func TestModelOnlyCommandsWorkOffline(t *testing.T) {
	a := testApp()
	a.model.createSpace("api", "/tmp/api", "")
	a.model.createTab("s1", "logs")
	a.model.setTerminal("p1", "t1")

	if err := a.Run(api.CmdSpaceRename, map[string]any{"id": "s1", "name": "work"}); err != nil {
		t.Fatal(err)
	}
	if got := a.Spaces()[0].Name; got != "work" {
		t.Fatalf("name = %q", got)
	}
	if err := a.Run(api.CmdSpaceSelect, map[string]any{"space": "s1"}); err != nil {
		t.Fatal(err)
	}
	if err := a.Run(api.CmdSpaceMove, map[string]any{"space": "s1", "delta": 1}); err == nil {
		t.Fatal("moving the only space by one should be out of range")
	}
	if err := a.Run(api.CmdTabRename, map[string]any{"tab": "t2", "name": "build"}); err != nil {
		t.Fatal(err)
	}
	if err := a.Run(api.CmdTabSelect, map[string]any{"id": "t1"}); err != nil {
		t.Fatal(err)
	}
	if err := a.Run(api.CmdPaneZoom, map[string]any{"id": "p1"}); err != nil {
		t.Fatal(err)
	}
	if got := a.UIState().Zoomed; got != "p1" {
		t.Fatalf("zoomed = %q", got)
	}
	if err := a.Run(api.CmdPaneFocus, map[string]any{"node": "p2"}); err != nil {
		t.Fatal(err)
	}
	if got := a.UIState().Focus; got != "p2" {
		t.Fatalf("focus = %q", got)
	}
	if err := a.Run(api.CmdPaneClose, map[string]any{"id": "p2"}); err != nil {
		t.Fatal(err)
	}
	if len(a.Spaces()[0].Tabs) != 1 {
		t.Fatalf("tabs = %d", len(a.Spaces()[0].Tabs))
	}
}

func TestTerminalCommandsStartAndKill(t *testing.T) {
	a := testApp()
	fake := newFakeCore()
	a.SetCore(fake)

	if err := a.Run(api.CmdSpaceNew, map[string]any{
		"name": "api", "folder": "/tmp/api", "machine": "box",
		"argv": []string{"bash", "-l"},
	}); err != nil {
		t.Fatal(err)
	}
	runs := fake.runsSnapshot()
	if len(runs) != 1 {
		t.Fatalf("runs = %d", len(runs))
	}
	if got := runs[0].Labels["hiik.space"]; got != "s1" {
		t.Fatalf("space label = %q", got)
	}
	if got := runs[0].Labels["hiik.tab"]; got != "t1" {
		t.Fatalf("tab label = %q", got)
	}
	if runs[0].Cwd != "/tmp/api" || runs[0].On != "box" {
		t.Fatalf("run = %#v", runs[0])
	}
	if got := len(runs[0].Argv); got != 2 || runs[0].Argv[0] != "bash" {
		t.Fatalf("argv = %v", runs[0].Argv)
	}
	if got := a.Spaces()[0].Tabs[0].Root.Terminal; got != "t1" {
		t.Fatalf("pane terminal = %q", got)
	}

	// A new tab inherits the space's folder and gets its own labels.
	if err := a.Run(api.CmdTabNew, map[string]any{"space": "s1", "name": "logs"}); err != nil {
		t.Fatal(err)
	}
	runs = fake.runsSnapshot()
	if len(runs) != 2 || runs[1].Cwd != "/tmp/api" {
		t.Fatalf("tab run = %#v", runs)
	}
	if runs[1].Labels["hiik.tab"] != "t2" {
		t.Fatalf("tab label = %#v", runs[1].Labels)
	}

	// Split down from the first pane, using the shell's alias keys.
	if err := a.Run(api.CmdPaneSplit, map[string]any{"node": "p1", "dir": "down"}); err != nil {
		t.Fatal(err)
	}
	runs = fake.runsSnapshot()
	if len(runs) != 3 {
		t.Fatalf("runs = %d", len(runs))
	}
	if runs[2].Labels["hiik.tab"] != "t1" || runs[2].Labels["hiik.space"] != "s1" {
		t.Fatalf("split labels = %#v", runs[2].Labels)
	}
	root := a.Spaces()[0].Tabs[0].Root
	if !root.Vertical {
		t.Fatalf("split is not vertical: %#v", root)
	}
	if root.B.Terminal != "t3" {
		t.Fatalf("new pane terminal = %q", root.B.Terminal)
	}

	// Closing by terminal address kills it and removes its pane.
	if err := a.Run(api.CmdPaneClose, map[string]any{"terminal": "t3"}); err != nil {
		t.Fatal(err)
	}
	if got := fake.killedSnapshot(); len(got) != 1 || got[0] != "t3" {
		t.Fatalf("killed = %v", got)
	}
	if root := a.Spaces()[0].Tabs[0].Root; !isLeaf(root) || root.Terminal != "t1" {
		t.Fatalf("tree after close = %#v", root)
	}

	if err := a.Run(api.CmdSpaceClose, map[string]any{"id": "s1"}); err != nil {
		t.Fatal(err)
	}
	if got := fake.killedSnapshot(); len(got) != 3 || got[1] != "t1" || got[2] != "t2" {
		t.Fatalf("killed = %v", got)
	}
	if len(a.Spaces()) != 0 {
		t.Fatalf("spaces = %d", len(a.Spaces()))
	}
}

func TestPaneSplitWithExistingTerminalNeedsNoCore(t *testing.T) {
	a := testApp()
	a.model.createSpace("api", "/tmp/api", "")
	a.model.setTerminal("p1", "t1")

	if err := a.Run(api.CmdPaneSplit, map[string]any{
		"pane": "p1", "direction": "right", "terminal": "t9",
	}); err != nil {
		t.Fatal(err)
	}
	root := a.Spaces()[0].Tabs[0].Root
	if root.Vertical || root.B.Terminal != "t9" {
		t.Fatalf("split = %#v", root)
	}

	if err := a.Run("pane.resize", map[string]any{"id": root.ID, "ratio": 0.3}); err != nil {
		t.Fatal(err)
	}
	if got := a.Spaces()[0].Tabs[0].Root.Ratio; got != 0.3 {
		t.Fatalf("ratio = %v", got)
	}
	if err := a.Run("pane.resize", map[string]any{"id": root.ID, "ratio": 2}); err == nil {
		t.Fatal("ratio 2 should fail")
	}
}

func TestStartCommandsNeedACore(t *testing.T) {
	a := testApp()
	if err := a.Run(api.CmdSpaceNew, nil); err == nil || !strings.Contains(err.Error(), "core not connected") {
		t.Fatalf("space.new err = %v", err)
	}
	if err := a.Run(api.CmdTabNew, map[string]any{"space": "s1"}); err == nil || !strings.Contains(err.Error(), "core not connected") {
		t.Fatalf("tab.new err = %v", err)
	}
	if err := a.Run(api.CmdPaneSplit, map[string]any{"direction": "right"}); err == nil || !strings.Contains(err.Error(), "core not connected") {
		t.Fatalf("pane.split err = %v", err)
	}

	// A failed start rolls the layout back.
	fake := newFakeCore()
	fake.runErr = errNoCore
	a.SetCore(fake)
	if err := a.Run(api.CmdSpaceNew, map[string]any{"name": "api"}); err == nil {
		t.Fatal("space.new should report the start error")
	}
	if len(a.Spaces()) != 0 {
		t.Fatalf("failed start left a space: %#v", a.Spaces())
	}
}

func TestAgentCommands(t *testing.T) {
	a := testApp()
	a.SetAgents([]api.Agent{
		{Address: "t9", Status: "done"},
		{Address: "t8", Pinned: true},
	})

	if err := a.Run(api.CmdAgentNext, nil); err != nil {
		t.Fatal(err)
	}
	if got := a.ActiveAgent(); got != "t8" {
		t.Fatalf("next agent = %q", got)
	}
	if err := a.Run(api.CmdAgentFocus, map[string]any{"terminal": "t9"}); err != nil {
		t.Fatal(err)
	}
	if got := a.Agents(); len(got) != 2 || got[1].Address != "t9" || got[1].Status != "idle" || !got[1].Seen {
		t.Fatalf("agents = %#v", got)
	}
	if err := a.Run(api.CmdAgentPin, map[string]any{"terminal": "t8", "pinned": false}); err != nil {
		t.Fatal(err)
	}
	for _, ag := range a.Agents() {
		if ag.Address == "t8" && ag.Pinned {
			t.Fatalf("t8 still pinned")
		}
	}
	if err := a.Run(api.CmdAgentPin, map[string]any{"agent": "t8"}); err != nil {
		t.Fatal(err)
	}
	if err := a.Run(api.CmdAgentPeek, map[string]any{"agent": "t8"}); err != nil {
		t.Fatal(err)
	}
	if got := a.Overlay(); got != "peek" {
		t.Fatalf("overlay = %q", got)
	}
	if err := a.Run(api.CmdAgentFocus, map[string]any{"terminal": "nope"}); err == nil {
		t.Fatal("focusing an unknown agent should fail")
	}
}

func TestOverlaysRetryAndSettings(t *testing.T) {
	a := testApp()
	fake := newFakeCore()
	a.SetCore(fake)

	for _, tc := range []struct {
		command string
		overlay string
	}{
		{api.CmdPalette, "palette"},
		{api.CmdSettings, "settings"},
		{api.CmdAbout, "about"},
	} {
		if err := a.Run(tc.command, nil); err != nil {
			t.Fatal(err)
		}
		if got := a.Overlay(); got != tc.overlay {
			t.Fatalf("%s overlay = %q", tc.command, got)
		}
	}

	a.SetConnection(Disconnected, "lost")
	if err := a.Run(api.CmdRetryCore, nil); err != nil {
		t.Fatal(err)
	}
	if got := a.Connection(); got != "connected" {
		t.Fatalf("connection after retry = %q", got)
	}

	if err := a.Run("app.set", map[string]any{"key": "telemetry", "value": true}); err != nil {
		t.Fatal(err)
	}
	if !a.Config().Privacy.Telemetry {
		t.Fatal("telemetry not set")
	}
	if err := a.Run("app.set", map[string]any{"key": "clipboard_write", "value": true}); err != nil {
		t.Fatal(err)
	}
	if got := a.Config().Privacy.ClipboardWrite; got != "always" {
		t.Fatalf("clipboard_write = %q", got)
	}
	if err := a.Run("app.set", map[string]any{"key": "clipboard_write", "value": "ask"}); err != nil {
		t.Fatal(err)
	}
	if err := a.Run("app.set", map[string]any{"key": "telemetry", "value": "yes"}); err == nil {
		t.Fatal("a string for a bool setting should fail")
	}
	if err := a.Run("app.set", map[string]any{"key": "nope", "value": true}); err == nil {
		t.Fatal("an unknown setting should fail")
	}
}

func TestCommandRunThroughModClosure(t *testing.T) {
	a := testApp()
	a.model.createSpace("api", "/tmp/api", "")
	var selectCmd api.Command
	for _, c := range a.Commands() {
		if c.ID == api.CmdSpaceRename {
			selectCmd = c
			break
		}
	}
	if selectCmd.Run == nil {
		t.Fatal("space.rename missing")
	}
	if err := selectCmd.Run(a, map[string]any{"name": "work"}); err != nil {
		t.Fatal(err)
	}
	if got := a.Spaces()[0].Name; got != "work" {
		t.Fatalf("name = %q", got)
	}
}

func TestTerminalKeepKillLabelPeek(t *testing.T) {
	a := testApp()
	fake := newFakeCore()
	a.SetCore(fake)
	a.model.createSpace("api", "/tmp/api", "")
	a.model.setTerminal("p1", "t1")

	// An existing pane is focused, not duplicated.
	if err := a.Run("terminal.keep", map[string]any{"terminal": "t1"}); err != nil {
		t.Fatal(err)
	}
	if got := a.Spaces()[0].Tabs[0].Root.Terminal; got != "t1" {
		t.Fatalf("tree = %#v", a.Spaces()[0].Tabs[0].Root)
	}

	// A terminal without a pane gets one beside the focused pane.
	if err := a.Run("terminal.keep", map[string]any{"terminal": "t9"}); err != nil {
		t.Fatal(err)
	}
	root := a.Spaces()[0].Tabs[0].Root
	if !isLeaf(root.A) || !isLeaf(root.B) || root.B.Terminal != "t9" {
		t.Fatalf("kept tree = %#v", root)
	}

	// place=tab opens a new tab for it.
	if err := a.Run("terminal.keep", map[string]any{"terminal": "t10", "place": "tab"}); err != nil {
		t.Fatal(err)
	}
	spaces := a.Spaces()
	if len(spaces[0].Tabs) != 2 || spaces[0].Tabs[1].Root.Terminal != "t10" {
		t.Fatalf("tabs = %#v", spaces[0].Tabs)
	}

	if err := a.Run("terminal.keep", map[string]any{"terminal": "t11", "place": "sideways"}); err == nil {
		t.Fatal("an unknown place should fail")
	}

	// The core keeps the terminal; killing it is explicit.
	if err := a.Run("terminal.kill", map[string]any{"terminal": "t9"}); err != nil {
		t.Fatal(err)
	}
	if got := fake.killedSnapshot(); len(got) != 1 || got[0] != "t9" {
		t.Fatalf("killed = %v", got)
	}
	if err := a.Run("terminal.label", map[string]any{
		"terminal": "t9", "labels": map[string]string{"hiik.space": "s1"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := a.Run("terminal.label", map[string]any{
		"terminal": "t9", "labels": map[string]any{"bad": 3},
	}); err == nil || !strings.Contains(err.Error(), "map of strings") {
		t.Fatalf("bad labels err = %v", err)
	}
	if err := a.Run("terminal.peek", map[string]any{"terminal": "t1"}); err != nil {
		t.Fatal(err)
	}
	if got := a.Overlay(); got != "peek" {
		t.Fatalf("overlay = %q", got)
	}

	// Without a core, kill and label report the connection clearly.
	offline := testApp()
	offline.model.createSpace("api", "/tmp/api", "")
	if err := offline.Run("terminal.kill", map[string]any{"terminal": "t1"}); err == nil {
		t.Fatal("terminal.kill without a core should fail")
	}
}
