package ui

import (
	"sort"

	mui "github.com/egoist/mygo/ui"

	"hiik/mods/api"
)

// paneGripSize is the small handle over a pane card.
const (
	paneGripW float32 = 28
	paneGripH float32 = 18
)

func init() {
	registerShellHook(paneGripHook)
}

// paneGripHook draws the pane grips and, while one is dragging, the native
// drop zones over the other panes. It runs above the panes and below
// notifications.
func paneGripHook(s *Shell, c *mui.Context) {
	if s == nil || c == nil {
		return
	}
	if s.overlayOpen() || s.zoom != "" {
		return
	}
	_, tab := s.activeTab()
	if tab == nil || tab.Root == nil || s.paneArea.W <= 0 || s.paneArea.H <= 0 {
		return
	}
	rects := tabPaneRects(tab)
	if len(rects) < 2 {
		return
	}

	t := c.Theme()
	ids := make([]string, 0, len(rects))
	for id := range rects {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	dragging := false
	for _, id := range ids {
		if s.paneGrip(c, t, id, s.windowPaneRect(rects[id])) {
			dragging = true
		}
	}
	for _, id := range ids {
		s.paneDropZones(c, t, id, s.windowPaneRect(rects[id]), dragging)
	}
}

// paneGrip draws one grip. A press that moves starts mygo's native drag;
// a press that does not move focuses the pane.
func (s *Shell) paneGrip(c *mui.Context, t *mui.Theme, id string, r mui.Rect) bool {
	x := r.X + (r.W-paneGripW)/2
	y := r.Y + 5
	grip := mui.Box(c).
		Key("shell/pane-grip/"+id).
		Absolute().
		Left(x).
		Top(y).
		Size(paneGripW, paneGripH).
		Radius(4).
		Background(t.Background.Alpha(0.85)).
		Border(1, t.Border).
		Cursor(mui.CursorGrab).
		Label("Move pane " + id).
		Drag(id).
		OnClick(func() { s.setFocus(id) })
	if grip.Dragging() {
		grip.Cursor(mui.CursorGrabbing)
	}
	grip.Children(func() {
		for range 2 {
			mui.Row(c).Gap(3).AlignItems(mui.Center).Children(func() {
				for range 3 {
					mui.Box(c).Size(2, 2).Radius(1).Background(t.TextMuted)
				}
			})
		}
	})
	return grip.Dragging()
}

// paneDropZones makes a pane take a dragged pane: the center swaps, and
// the quarters split toward the edge. drop is native Drag/Drop; no pointer
// arithmetic belongs to the app.
func (s *Shell) paneDropZones(c *mui.Context, t *mui.Theme, id string, r mui.Rect, active bool) {
	zones := []struct {
		edge       string
		x, y, w, h float32
	}{
		{"left", r.X, r.Y, r.W * 0.25, r.H},
		{"right", r.X + r.W*0.75, r.Y, r.W * 0.25, r.H},
		{"up", r.X, r.Y, r.W, r.H * 0.25},
		{"down", r.X, r.Y + r.H*0.75, r.W, r.H * 0.25},
		{"", r.X + r.W*0.25, r.Y + r.H*0.25, r.W * 0.5, r.H * 0.5},
	}
	for _, z := range zones {
		key := "shell/pane-drop/" + id + "/" + z.edge
		zone := mui.Box(c).
			Key(key).
			Absolute().
			Left(z.x).
			Top(z.y).
			Size(z.w, z.h).
			Radius(6).
			IgnoreInput(!active)
		if source, ok := mui.DragOver[string](zone); ok && source != id {
			zone.Background(t.Accent.Alpha(0.15)).Border(2, t.Accent)
		}
		if source, ok := mui.Drop[string](zone); ok && source != id {
			s.dropPane(source, id, z.edge)
		}
	}
}

// dropPane runs the split or swap one drop asks for.
func (s *Shell) dropPane(source, target, edge string) {
	if edge == "" {
		s.run(api.CmdPaneSwap, map[string]any{"a": source, "b": target})
		return
	}
	dir := "right"
	if edge == "up" || edge == "down" {
		dir = "down"
	}
	if err := s.app.Run(api.CmdPaneSplit, map[string]any{
		"pane":      target,
		"id":        target,
		"direction": dir,
	}); err != nil {
		s.toast = err.Error()
		return
	}
	if edge == "left" || edge == "up" {
		s.run(api.CmdPaneSwap, map[string]any{"direction": edge})
	}
}

// windowPaneRect turns a pane's share of the tab into window DIPs, inside
// the 6 DIP padding of the pane area.
func (s *Shell) windowPaneRect(fr paneRect) mui.Rect {
	const pad = 6
	x, y := s.paneArea.X+pad, s.paneArea.Y+pad
	w, h := s.paneArea.W-2*pad, s.paneArea.H-2*pad
	if w < 0 {
		w = 0
	}
	if h < 0 {
		h = 0
	}
	return mui.Rect{X: x + fr.X*w, Y: y + fr.Y*h, W: fr.W * w, H: fr.H * h}
}

// tabPaneRects returns each pane leaf's share of the tab.
func tabPaneRects(tab *api.Tab) map[string]paneRect {
	out := make(map[string]paneRect)
	if tab == nil || tab.Root == nil {
		return out
	}
	collectRects(tab.Root, paneRect{0, 0, 1, 1}, out)
	return out
}
