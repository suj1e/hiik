package ui

import (
	"testing"
	"time"

	mui "github.com/egoist/mygo/ui"

	"hiik/mods/api"
)

func gripApp() *fakeApp {
	return &fakeApp{
		active: "s1",
		spaces: []api.Space{{ID: "s1", Name: "alpha", Tabs: []api.Tab{{
			ID: "t1", Name: "first", Focus: "n1",
			Root: &api.Node{ID: "split", Ratio: 0.5,
				A: leaf("n1", "term-a"),
				B: leaf("n2", "term-b"),
			},
		}}}},
	}
}

func dragGrip(t *testing.T, tt *mui.Tester, s *Shell, from, target string, escape bool) {
	t.Helper()
	start, ok := tt.Find("Move pane " + from)
	if !ok {
		t.Fatalf("no grip for %s; texts: %v", from, tt.Texts())
	}
	_, tab := s.activeTab()
	rects := tabPaneRects(tab)
	want := s.windowPaneRect(rects[target])
	x0, y0 := start.X+start.W/2, start.Y+start.H/2
	x1, y1 := want.X+want.W/2, want.Y+want.H/2
	tt.Move(x0, y0)
	tt.Press(x0, y0)
	for i := 1; i <= 6; i++ {
		k := float32(i) / 6
		tt.Advance(10 * time.Millisecond)
		tt.Move(x0+(x1-x0)*k, y0+(y1-y0)*k)
	}
	targetID := s.paneDrag.target
	if targetID != target {
		t.Fatalf("drop target = %q, want %q", targetID, target)
	}
	if escape {
		tt.Key(0, mui.KeyEscape)
		tt.Frame()
		if s.paneDrag.active {
			t.Fatal("Escape left the pane drag active")
		}
	}
	tt.Advance(10 * time.Millisecond)
	tt.Release(x1, y1)
	tt.Frame()
}

func TestPaneGripCenterSwaps(t *testing.T) {
	app := gripApp()
	s, tt := newTestShell(app)
	tt.Frame()
	dragGrip(t, tt, s, "n1", "n2", false)
	names := app.ranNames()
	if len(names) == 0 || names[len(names)-1] != api.CmdPaneSwap {
		t.Fatalf("dropping in a pane center ran %v, want %s", names, api.CmdPaneSwap)
	}
	args := app.ranArgs[len(app.ranArgs)-1]
	if args["a"] != "n1" || args["b"] != "n2" {
		t.Fatalf("swap args = %v, want a=n1 b=n2", args)
	}
}

func TestPaneGripEscapeCancels(t *testing.T) {
	app := gripApp()
	s, tt := newTestShell(app)
	tt.Frame()
	dragGrip(t, tt, s, "n1", "n2", true)
	if len(app.ranNames()) != 0 {
		t.Fatalf("Escape still ran %v", app.ranNames())
	}
}

func TestPaneGripEdgeSplits(t *testing.T) {
	app := gripApp()
	s, tt := newTestShell(app)
	tt.Frame()
	_, tab := s.activeTab()
	start, ok := tt.Find("Move pane n1")
	if !ok {
		t.Fatal("no grip for n1")
	}
	want := s.windowPaneRect(tabPaneRects(tab)["n2"])
	x0, y0 := start.X+start.W/2, start.Y+start.H/2
	// The left quarter of the other pane asks for a split.
	x1, y1 := want.X+want.W*0.08, want.Y+want.H/2
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
	names := app.ranNames()
	if len(names) < 2 || names[0] != api.CmdPaneSplit || names[1] != api.CmdPaneSwap {
		t.Fatalf("dropping on the left edge ran %v, want %s then %s", names, api.CmdPaneSplit, api.CmdPaneSwap)
	}
	args := app.ranArgs[0]
	if args["pane"] != "n2" || args["direction"] != "right" {
		t.Fatalf("split args = %v, want pane=n2 direction=right", args)
	}
	flip := app.ranArgs[1]
	if flip["direction"] != "left" {
		t.Fatalf("flip args = %v, want direction=left", flip)
	}
}

func TestPaneGripTakesThePress(t *testing.T) {
	app := gripApp()
	s, tt := newTestShell(app)
	tt.Frame()
	if err := tt.Click("Move pane n2"); err != nil {
		t.Fatalf("grip is not the input target: %v", err)
	}
	if s.focus != "n2" {
		t.Fatalf("clicking the grip focused %q, want n2", s.focus)
	}
}
