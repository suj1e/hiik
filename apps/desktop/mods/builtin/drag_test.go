package builtin

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"hiik/mods/api"
)

// dragPanel renders one panel and drags from the center of from to to.
func dragPanel(t *testing.T, p api.Panel, app api.App, props map[string]any, from, to string, dx, dy float32) {
	t.Helper()
	tt := ui.NewTester(func(c *ui.Context) {
		p.Render(&api.RenderContext{C: c, App: app, Props: props})
	}, 900, 600)
	tt.Frame()
	start, ok := tt.Find(from)
	if !ok {
		t.Fatalf("no %q to drag; texts: %v", from, tt.Texts())
	}
	target, ok := tt.Find(to)
	if !ok {
		t.Fatalf("no %q to drop on; texts: %v", to, tt.Texts())
	}
	x0, y0 := start.X+start.W/2, start.Y+start.H/2
	x1, y1 := target.X+target.W/2+dx, target.Y+target.H/2+dy
	tt.Move(x0, y0)
	tt.Press(x0, y0)
	for i := 1; i <= 6; i++ {
		k := float32(i) / 6
		tt.Advance(10 * time.Millisecond)
		tt.Move(x0+(x1-x0)*k, y0+(y1-y0)*k)
	}
	tt.Advance(10 * time.Millisecond)
	tt.Release(x1, y1)
	tt.Frame()
}

func TestTabsDragReorders(t *testing.T) {
	app := &btPanelApp{
		spaces: []api.Space{{ID: "s1", Name: "One", Tabs: []api.Tab{
			{ID: "t1", Name: "first"},
			{ID: "t2", Name: "second"},
			{ID: "t3", Name: "third"},
		}}},
		active: "s1",
	}
	sp := app.spaces[0]
	panel := api.Panels()[api.PanelTabs]
	dragPanel(t, panel, app, map[string]any{"space": &sp, "tabs": sp.Tabs, "active": 0},
		"first", "third", 0, 0)
	if len(app.ran) == 0 || app.ran[len(app.ran)-1] != api.CmdTabMove {
		t.Fatalf("dragging a tab ran %v, want %s", app.ran, api.CmdTabMove)
	}
	if got := app.args[len(app.args)-1]["to"]; got != 2 {
		t.Fatalf("tab.move to = %v, want 2", got)
	}
	if got := app.args[len(app.args)-1]["tab"]; got != "t1" {
		t.Fatalf("tab.move tab = %v, want t1", got)
	}
}

func TestSpacesDragReorders(t *testing.T) {
	app := &btPanelApp{
		spaces: []api.Space{
			{ID: "s1", Name: "Alpha"},
			{ID: "s2", Name: "Beta"},
			{ID: "s3", Name: "Gamma"},
		},
		active: "s1",
	}
	panel := api.Panels()[api.PanelSpaces]
	dragPanel(t, panel, app, map[string]any{"spaces": app.spaces, "active": "s1"},
		"Alpha", "Gamma", 0, 10)
	if len(app.ran) == 0 || app.ran[len(app.ran)-1] != api.CmdSpaceMove {
		t.Fatalf("dragging a space ran %v, want %s", app.ran, api.CmdSpaceMove)
	}
	if got := app.args[len(app.args)-1]["to"]; got != 2 {
		t.Fatalf("space.move to = %v, want 2", got)
	}
}

func TestTabsClickStillSelects(t *testing.T) {
	app := &btPanelApp{
		spaces: []api.Space{{ID: "s1", Name: "One", Tabs: []api.Tab{
			{ID: "t1", Name: "first"},
			{ID: "t2", Name: "second"},
		}}},
		active: "s1",
	}
	sp := app.spaces[0]
	panel := api.Panels()[api.PanelTabs]
	tt := ui.NewTester(func(c *ui.Context) {
		panel.Render(&api.RenderContext{C: c, App: app, Props: map[string]any{
			"space": &sp, "tabs": sp.Tabs, "active": 0,
		}})
	}, 900, 600)
	tt.Frame()
	if err := tt.Click("second"); err != nil {
		t.Fatal(err)
	}
	if len(app.ran) != 1 || app.ran[0] != api.CmdTabSelect {
		t.Fatalf("clicking a tab ran %v, want %s", app.ran, api.CmdTabSelect)
	}
}

func TestTabsDropOnAnotherRowUsesItsIndex(t *testing.T) {
	app := &btPanelApp{
		spaces: []api.Space{{ID: "s1", Name: "One", Tabs: []api.Tab{
			{ID: "t1", Name: "first"},
			{ID: "t2", Name: "second"},
			{ID: "t3", Name: "third"},
		}}},
		active: "s1",
	}
	sp := app.spaces[0]
	panel := api.Panels()[api.PanelTabs]
	dragPanel(t, panel, app, map[string]any{"space": &sp, "tabs": sp.Tabs, "active": 0},
		"first", "second", 0, 0)
	if len(app.ran) == 0 || app.ran[len(app.ran)-1] != api.CmdTabMove {
		t.Fatalf("dragging a tab ran %v, want %s", app.ran, api.CmdTabMove)
	}
	if got := app.args[len(app.args)-1]["to"]; got != 1 {
		t.Fatalf("tab.move to = %v, want 1", got)
	}
}
