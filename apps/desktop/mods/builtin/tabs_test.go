package builtin

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"hiik/mods/api"
)

func renderTabsPanel(t *testing.T, app api.App, sp api.Space, active int) *ui.Tester {
	t.Helper()
	panel := api.Panels()[api.PanelTabs]
	tt := ui.NewTester(func(c *ui.Context) {
		panel.Render(&api.RenderContext{C: c, App: app, Props: map[string]any{
			"space": &sp, "tabs": sp.Tabs, "active": active,
		}})
	}, 900, 200)
	tt.Frame()
	return tt
}

func TestTabsShowStatusAndOneClose(t *testing.T) {
	sp := api.Space{ID: "s1", Name: "One", Active: 0, Tabs: []api.Tab{
		{ID: "t1", Name: "first"},
		{ID: "t2", Name: "second"},
		{ID: "t3", Name: "third"},
	}}
	app := &btPanelApp{
		spaces: []api.Space{sp},
		active: "s1",
		agents: []api.Agent{
			{Address: "a1", Space: "s1", Tab: "t1", Status: "working"},
			{Address: "a2", Space: "s1", Tab: "t2", Status: "blocked"},
		},
	}
	tt := renderTabsPanel(t, app, sp, 0)
	for _, want := range []string{"first", "second", "third", "Working", "Blocked"} {
		if !tt.HasText(want) {
			t.Fatalf("the tab strip does not show %q: %v", want, tt.Texts())
		}
	}
	closes := 0
	for _, text := range tt.Texts() {
		if text == "✕" {
			closes++
		}
	}
	if closes != 1 {
		t.Fatalf("the tab strip shows %d close buttons, want one: %v", closes, tt.Texts())
	}
}

func TestTabsClickSelects(t *testing.T) {
	sp := api.Space{ID: "s1", Name: "One", Active: 0, Tabs: []api.Tab{
		{ID: "t1", Name: "first"},
		{ID: "t2", Name: "second"},
	}}
	app := &btPanelApp{spaces: []api.Space{sp}, active: "s1"}
	tt := renderTabsPanel(t, app, sp, 0)
	if err := tt.Click("second"); err != nil {
		t.Fatal(err)
	}
	if len(app.ran) != 1 || app.ran[0] != api.CmdTabSelect {
		t.Fatalf("clicking a tab ran %v, want %s", app.ran, api.CmdTabSelect)
	}
	if app.args[0]["tab"] != "t2" {
		t.Fatalf("select ran for %v, want t2", app.args[0]["tab"])
	}
}

func TestTabsEmptyState(t *testing.T) {
	sp := api.Space{ID: "s1", Name: "One"}
	app := &btPanelApp{spaces: []api.Space{sp}, active: "s1"}
	tt := renderTabsPanel(t, app, sp, 0)
	if !tt.HasText("No tabs") {
		t.Fatalf("an empty tab strip shows no message: %v", tt.Texts())
	}
}
