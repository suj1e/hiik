package ui

import (
	"testing"

	"hiik/mods/api"
)

func TestTerminalViewFillsPane(t *testing.T) {
	core := &fakeCore{}
	app := &fakeApp{
		active: "s1",
		core:   core,
		spaces: []api.Space{{ID: "s1", Name: "alpha", Tabs: []api.Tab{{
			ID: "t1", Name: "first", Focus: "n1", Root: leaf("n1", "term-a"),
		}}}},
	}
	s, tt := newTestShell(app)
	tt.Frame()
	snap := waitAttached(t, s, "term-a")
	if snap.term == nil {
		t.Fatalf("attach: %s", snap.err)
	}
	tt.Frame()
	tt.Frame()
	cols, rows := snap.term.Size()
	r, ok := tt.Find("Terminal")
	t.Logf("terminal size=%dx%d bounds=%+v ok=%v window=%dx%d", cols, rows, r, ok, 1180, 760)
	if cols <= 80 || rows <= 24 {
		t.Fatalf("terminal grid stayed %dx%d", cols, rows)
	}
	if !ok || r.W < 600 || r.H < 400 {
		t.Fatalf("terminal bounds %+v (found=%v)", r, ok)
	}
	s.Close()
}
