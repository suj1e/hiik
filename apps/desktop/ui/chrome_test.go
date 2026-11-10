package ui

import (
	"testing"

	"hiik/mods/api"
)

func terminalsLayoutApp() *fakeApp {
	return &fakeApp{
		active:   "s1",
		settings: map[string]any{"layout": "terminals", "theme": ThemeGraphite},
		spaces: []api.Space{{ID: "s1", Name: "alpha", Tabs: []api.Tab{
			{ID: "t1", Name: "first", Root: leaf("n1", "term-a")},
			{ID: "t2", Name: "second", Root: leaf("n2", "term-b")},
		}}},
		agents: []api.Agent{{Address: "a1", Title: "claude"}},
	}
}

func TestTerminalsLayoutMovesChromeToTheTitleBar(t *testing.T) {
	app := terminalsLayoutApp()
	s, tt := newTestShell(app)
	tt.Frame()
	for _, gone := range []string{"SPACES", "AGENTS", "No agents"} {
		if tt.HasText(gone) {
			t.Fatalf("the Terminals layout still shows the sidebar (%q): %v", gone, tt.Texts())
		}
	}
	for _, want := range []string{"alpha", "first", "second"} {
		if !tt.HasText(want) {
			t.Fatalf("the Terminals title bar does not show %q: %v", want, tt.Texts())
		}
	}
	if s.paneArea.W <= 0 {
		t.Fatal("the pane area was not measured")
	}
	if s.paneArea.X > 20 {
		t.Fatalf("the Terminals pane area starts at x=%v, want the full width", s.paneArea.X)
	}
}

func TestHerdrLayoutKeepsTheSidebar(t *testing.T) {
	app := terminalsLayoutApp()
	app.settings["layout"] = "herdr"
	s, tt := newTestShell(app)
	tt.Frame()
	if !tt.HasText("Spaces") || !tt.HasText("Agents") {
		t.Fatalf("the Herdr layout lost its sidebar: %v", tt.Texts())
	}
	if s.paneArea.X < 100 {
		t.Fatalf("the Herdr pane area starts at x=%v, want it beside the sidebar", s.paneArea.X)
	}
}

func TestHerdrStatusLivesUnderTheWorkColumn(t *testing.T) {
	app := terminalsLayoutApp()
	app.settings["layout"] = "herdr"
	s, tt := newTestShell(app)
	tt.Frame()
	status, ok := tt.Find("1 agents")
	if !ok {
		t.Fatalf("the herdr layout shows no status bar: %v", tt.Texts())
	}
	if status.X <= s.sidebarW {
		t.Fatalf("the status bar starts at x=%v, inside the sidebar width %v", status.X, s.sidebarW)
	}
	if status.Y < 700 {
		t.Fatalf("the status bar sits at y=%v, not at the bottom of the work column", status.Y)
	}
	if tt.HasText("core:") || tt.HasText("0 spaces") {
		t.Fatalf("the status bar still shows the old full-width summary: %v", tt.Texts())
	}
}

func TestTerminalsLayoutHasNoStatusBar(t *testing.T) {
	app := terminalsLayoutApp()
	_, tt := newTestShell(app)
	tt.Frame()
	if tt.HasText("1 agents") || tt.HasText("This computer") || tt.HasText("core:") {
		t.Fatalf("the Terminals layout drew a status bar: %v", tt.Texts())
	}
}

func TestStatusShowsThePaletteBinding(t *testing.T) {
	app := terminalsLayoutApp()
	app.settings["layout"] = "herdr"
	app.settings["keys"] = map[string]string{"app.palette": "ctrl-k"}
	_, tt := newTestShell(app)
	tt.Frame()
	want := chordLabel("ctrl-k")
	if !tt.HasText(want) {
		t.Fatalf("the status bar does not show the real palette key %q: %v", want, tt.Texts())
	}
	if tt.HasText("⌘P") || tt.HasText("Control P") {
		t.Fatalf("the status bar shows a hardcoded palette key: %v", tt.Texts())
	}
}

func TestStatusShowsTheDefaultPaletteBinding(t *testing.T) {
	app := terminalsLayoutApp()
	app.settings["layout"] = "herdr"
	_, tt := newTestShell(app)
	tt.Frame()
	want := api.FormatChord("Mod+P") + "  commands"
	if !tt.HasText(want) {
		t.Fatalf("the status bar does not show the default palette key %q: %v", want, tt.Texts())
	}
}

func TestStatusOffersCommandsWithoutAPaletteBinding(t *testing.T) {
	app := terminalsLayoutApp()
	app.settings["layout"] = "herdr"
	app.settings["keys"] = map[string]string{"app.help": "ctrl-k"}
	_, tt := newTestShell(app)
	tt.Frame()
	if !tt.HasText("commands") {
		t.Fatalf("an unbound palette shows no commands hint: %v", tt.Texts())
	}
	if tt.HasText(chordLabel("ctrl-k")) {
		t.Fatalf("an unbound palette invented a key: %v", tt.Texts())
	}
}

func TestHerdrTabsShowStatusAndOneClose(t *testing.T) {
	app := terminalsLayoutApp()
	app.settings["layout"] = "herdr"
	app.agents = []api.Agent{
		{Address: "a1", Space: "s1", Tab: "t1", App: "claude", Status: "working"},
		{Address: "a2", Space: "s1", Tab: "t2", Status: "blocked"},
	}
	_, tt := newTestShell(app)
	tt.Frame()
	if !tt.HasText("Working") {
		t.Fatalf("the working tab shows no spinner label: %v", tt.Texts())
	}
	if !tt.HasText("Blocked") {
		t.Fatalf("the blocked tab shows no status mark: %v", tt.Texts())
	}
	closes := 0
	for _, text := range tt.Texts() {
		if text == "Close tab" {
			closes++
		}
	}
	if closes != 1 {
		t.Fatalf("the tab strip shows %d close buttons, want the active tab's one: %v", closes, tt.Texts())
	}
}
