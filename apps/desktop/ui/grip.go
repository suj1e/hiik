package ui

import (
	"sort"

	mui "github.com/egoist/mygo/ui"

	"hiik/mods/api"
)

// paneDragState is one grip drag: which pane is dragged, where the pointer
// is in the window, and the drop target under it.
type paneDragState struct {
	active bool
	source string
	startX float32
	startY float32
	x      float32
	y      float32
	moved  bool
	target string
	edge   string // left, right, up, down, or "" to swap
}

// paneGripSize is the small handle over a pane card.
const (
	paneGripW     float32 = 28
	paneGripH     float32 = 18
	paneDragSlop  float32 = 4
	paneDropShare float32 = 0.25
)

func init() {
	registerShellHook(paneGripHook)
}

// paneGripHook draws the pane grips and the drop preview. It runs at the
// end of the shell's frame, above the panes and below notifications.
func paneGripHook(s *Shell, c *mui.Context) {
	if s == nil || c == nil {
		return
	}
	if s.overlayOpen() || s.zoom != "" {
		s.paneDrag = paneDragState{}
		return
	}
	_, tab := s.activeTab()
	if tab == nil || tab.Root == nil || s.paneArea.W <= 0 || s.paneArea.H <= 0 {
		s.paneDrag = paneDragState{}
		return
	}
	rects := tabPaneRects(tab)
	if len(rects) < 2 {
		s.paneDrag = paneDragState{}
		return
	}

	t := c.Theme()
	ids := make([]string, 0, len(rects))
	for id := range rects {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		s.paneGrip(c, t, id, s.windowPaneRect(rects[id]))
	}

	if s.paneDrag.active {
		c.OnWindowShortcut(0, mui.KeyEscape, func() {
			s.paneDrag = paneDragState{}
		})
	}
	s.paneDropPreview(c, t, rects)
}

// paneGrip draws one grip and follows the pointer while it presses it.
func (s *Shell) paneGrip(c *mui.Context, t *mui.Theme, id string, r mui.Rect) {
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
		OnClick(func() { s.setFocus(id) })
	if s.paneDrag.active && s.paneDrag.source == id {
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

	px, py, _ := grip.PointerPosition()
	pressed := grip.Pressed()
	dx, dy, held := grip.Dragged()
	_, _, ended := grip.DragEnded()

	if pressed && (!s.paneDrag.active || s.paneDrag.source != id) {
		wx, wy := x+px, y+py
		s.setFocus(id)
		s.paneDrag = paneDragState{active: true, source: id, startX: wx, startY: wy, x: wx, y: wy}
	}
	if !s.paneDrag.active || s.paneDrag.source != id {
		return
	}
	if held {
		// Dragged keeps reporting after the pointer leaves the grip.
		s.paneDrag.x += dx
		s.paneDrag.y += dy
		if !s.paneDrag.moved {
			mx, my := s.paneDrag.x-s.paneDrag.startX, s.paneDrag.y-s.paneDrag.startY
			if mx*mx+my*my >= paneDragSlop*paneDragSlop {
				s.paneDrag.moved = true
			}
		}
	}
	switch {
	case ended:
		if s.paneDrag.moved {
			s.updatePaneDrop(id)
		}
		s.finishPaneDrag()
	case !held:
		// A press that never moved is a click, not a drop.
		s.paneDrag = paneDragState{}
	default:
		s.updatePaneDrop(id)
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

// updatePaneDrop finds the pane under the pointer and which part of it the
// drop would split, or its center to swap.
func (s *Shell) updatePaneDrop(source string) {
	s.paneDrag.target, s.paneDrag.edge = "", ""
	_, tab := s.activeTab()
	if tab == nil {
		return
	}
	rects := tabPaneRects(tab)
	ids := make([]string, 0, len(rects))
	for id := range rects {
		if id != source {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		r := s.windowPaneRect(rects[id])
		if !r.Contains(s.paneDrag.x, s.paneDrag.y) {
			continue
		}
		s.paneDrag.target = id
		s.paneDrag.edge = paneEdgeAt(r, s.paneDrag.x, s.paneDrag.y)
		return
	}
}

// paneEdgeAt returns the edge a drop near one would split, or "" for the
// center.
func paneEdgeAt(r mui.Rect, x, y float32) string {
	if r.W <= 0 || r.H <= 0 {
		return ""
	}
	fx := (x - r.X) / r.W
	fy := (y - r.Y) / r.H
	type edgeDistance struct {
		distance float32
		name     string
	}
	edges := []edgeDistance{
		{fx, "left"},
		{1 - fx, "right"},
		{fy, "up"},
		{1 - fy, "down"},
	}
	best := edges[0]
	for _, e := range edges[1:] {
		if e.distance < best.distance {
			best = e
		}
	}
	if best.distance >= paneDropShare {
		return ""
	}
	return best.name
}

// paneDropPreview paints where the drop would go.
func (s *Shell) paneDropPreview(c *mui.Context, t *mui.Theme, rects map[string]paneRect) {
	if !s.paneDrag.active || !s.paneDrag.moved || s.paneDrag.target == "" {
		return
	}
	fr, ok := rects[s.paneDrag.target]
	if !ok {
		return
	}
	r := s.windowPaneRect(fr)
	switch s.paneDrag.edge {
	case "left":
		r.W /= 2
	case "right":
		r.X += r.W / 2
		r.W /= 2
	case "up":
		r.H /= 2
	case "down":
		r.Y += r.H / 2
		r.H /= 2
	}
	mui.Box(c).
		Key("shell/pane-drop").
		Absolute().
		Left(r.X).
		Top(r.Y).
		Size(r.W, r.H).
		Radius(6).
		Background(t.Accent.Alpha(0.18)).
		Border(2, t.Accent).
		IgnoreInput(true)
}

// finishPaneDrag runs the split or swap the drop asks for, then clears the
// drag. Escape clears it before the pointer lets go, so nothing runs.
func (s *Shell) finishPaneDrag() {
	drag := s.paneDrag
	s.paneDrag = paneDragState{}
	if !drag.active || !drag.moved || drag.target == "" || drag.target == drag.source {
		return
	}
	if drag.edge == "" {
		s.run(api.CmdPaneSwap, map[string]any{"a": drag.source, "b": drag.target})
		return
	}
	dir := "right"
	if drag.edge == "up" || drag.edge == "down" {
		dir = "down"
	}
	if err := s.app.Run(api.CmdPaneSplit, map[string]any{
		"pane":      drag.target,
		"id":        drag.target,
		"direction": dir,
	}); err != nil {
		s.toast = err.Error()
		return
	}
	// The split command always puts the new pane on the right or bottom.
	// Dropping on the opposite edge flips the pair, so the split lands
	// under the pointer.
	if drag.edge == "left" || drag.edge == "up" {
		s.run(api.CmdPaneSwap, map[string]any{"direction": drag.edge})
	}
}
