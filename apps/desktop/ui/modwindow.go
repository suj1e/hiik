package ui

import (
	"sync"

	mui "github.com/egoist/mygo/ui"

	"hiik/mods/api"
)

// Mod windows and floating regions. A window is a framed panel over the
// shell: it stays until closed, and Keep stops a click outside or Escape
// from dismissing it. A float is a smaller framed panel snapped to one
// edge; several can be open at once.
//
// Both are host-side: mods name a panel from api.Panels() and pass props.
// The requests are queued so a mod may call the helpers from a click or a
// goroutine without touching the frame being built; the shell hook then
// moves them into that window's surface.

// modWindowState is one open window.
type modWindowState struct {
	panelID string
	props   map[string]any
	keep    bool
	open    bool
	pos     mui.Point
}

// modFloatState is one open floating region.
type modFloatState struct {
	panelID string
	edge    string
	props   map[string]any
	pos     mui.Point
}

// modSurfaceState is what one window shows outside its panes.
type modSurfaceState struct {
	windows     map[string]*modWindowState
	windowOrder []string
	floats      map[string]*modFloatState
	floatOrder  []string
}

type modWindowRequest struct {
	panelID string
	props   map[string]any
}

type modFloatRequest struct {
	panelID string
	edge    string
	props   map[string]any
}

var (
	modSurfaceMu   sync.Mutex
	modSurfaces    = make(map[*Shell]*modSurfaceState)
	pendingWindows []modWindowRequest
	pendingFloats  []modFloatRequest
)

func init() {
	registerShellHook(runModSurfaceHook)
	api.RegisterModSurface(modSurfaceHost{})
}

// modSurfaceHost is the api.ModSurface the commands and mods reach.
type modSurfaceHost struct{}

func (modSurfaceHost) OpenWindow(panelID string, props map[string]any) {
	OpenModWindow(panelID, props)
}

func (modSurfaceHost) CloseWindow(panelID string) { CloseModWindow(panelID) }

func (modSurfaceHost) OpenFloat(panelID, edge string, props map[string]any) {
	OpenModFloat(panelID, edge, props)
}

func (modSurfaceHost) CloseFloat(panelID string) { CloseModFloat(panelID) }

// OpenModWindow shows panelID as a window with props, or updates the props
// of the window already showing it. It does not block the frame.
func OpenModWindow(panelID string, props map[string]any) {
	if panelID == "" {
		return
	}
	modSurfaceMu.Lock()
	pendingWindows = append(pendingWindows, modWindowRequest{
		panelID: panelID,
		props:   cloneModProps(props),
	})
	modSurfaceMu.Unlock()
}

// CloseModWindow closes the window showing panelID, if one is open or
// waiting to open.
func CloseModWindow(panelID string) {
	modSurfaceMu.Lock()
	pendingWindows = dropWindowRequest(pendingWindows, panelID)
	for _, st := range modSurfaces {
		st.removeWindow(panelID)
	}
	modSurfaceMu.Unlock()
}

// OpenModFloat shows panelID in a floating region on edge with props. An
// empty or unknown edge becomes the right edge.
func OpenModFloat(panelID, edge string, props map[string]any) {
	if panelID == "" {
		return
	}
	modSurfaceMu.Lock()
	pendingFloats = append(pendingFloats, modFloatRequest{
		panelID: panelID,
		edge:    normFloatEdge(edge),
		props:   cloneModProps(props),
	})
	modSurfaceMu.Unlock()
}

// CloseModFloat closes the floating region showing panelID, if one is open
// or waiting to open.
func CloseModFloat(panelID string) {
	modSurfaceMu.Lock()
	pendingFloats = dropFloatRequest(pendingFloats, panelID)
	for _, st := range modSurfaces {
		st.removeFloat(panelID)
	}
	modSurfaceMu.Unlock()
}

func dropWindowRequest(list []modWindowRequest, panelID string) []modWindowRequest {
	out := list[:0]
	for _, r := range list {
		if r.panelID != panelID {
			out = append(out, r)
		}
	}
	return out
}

func dropFloatRequest(list []modFloatRequest, panelID string) []modFloatRequest {
	out := list[:0]
	for _, r := range list {
		if r.panelID != panelID {
			out = append(out, r)
		}
	}
	return out
}

// cloneModProps copies a props map so a caller may reuse or change its own.
func cloneModProps(props map[string]any) map[string]any {
	if props == nil {
		return nil
	}
	out := make(map[string]any, len(props))
	for k, v := range props {
		out[k] = v
	}
	return out
}

// normFloatEdge accepts the four sides and falls back to the right edge.
func normFloatEdge(edge string) string {
	switch edge {
	case "left", "right", "top", "bottom":
		return edge
	}
	return api.FloatEdgeRight
}

func newModSurfaceState() *modSurfaceState {
	return &modSurfaceState{
		windows: make(map[string]*modWindowState),
		floats:  make(map[string]*modFloatState),
	}
}

// setWindow opens a window or replaces the props of the one already open.
func (st *modSurfaceState) setWindow(panelID string, props map[string]any) {
	if w := st.windows[panelID]; w != nil {
		w.props = props
		w.open = true
		return
	}
	st.windows[panelID] = &modWindowState{panelID: panelID, props: props, open: true}
	st.windowOrder = append(st.windowOrder, panelID)
}

func (st *modSurfaceState) removeWindow(panelID string) {
	delete(st.windows, panelID)
	for i, id := range st.windowOrder {
		if id == panelID {
			st.windowOrder = append(st.windowOrder[:i], st.windowOrder[i+1:]...)
			return
		}
	}
}

// setFloat opens a float or moves the one already open to a new edge.
func (st *modSurfaceState) setFloat(panelID, edge string, props map[string]any) {
	if f := st.floats[panelID]; f != nil {
		f.edge = edge
		f.props = floatProps(panelID, props)
		return
	}
	st.floats[panelID] = &modFloatState{
		panelID: panelID,
		edge:    edge,
		props:   floatProps(panelID, props),
	}
	st.floatOrder = append(st.floatOrder, panelID)
}

func (st *modSurfaceState) removeFloat(panelID string) {
	delete(st.floats, panelID)
	for i, id := range st.floatOrder {
		if id == panelID {
			st.floatOrder = append(st.floatOrder[:i], st.floatOrder[i+1:]...)
			return
		}
	}
}

// floatProps tells the panel which float it is without changing the
// caller's map.
func floatProps(panelID string, props map[string]any) map[string]any {
	out := cloneModProps(props)
	if out == nil {
		out = make(map[string]any, 1)
	}
	out["float"] = panelID
	return out
}

// runModSurfaceHook moves queued requests into this window and draws the
// windows and floats. Floats step aside for a modal, as the inspector does.
func runModSurfaceHook(s *Shell, c *mui.Context) {
	if s == nil || s.app == nil {
		return
	}
	setModSurfaceApp(s.app)
	st := s.consumeModSurface()
	if !s.overlayOpen() {
		s.drawModFloats(c, st)
	}
	s.drawModWindows(c, st)
}

// consumeModSurface returns this window's surface after taking the queued
// requests. Whichever window drains first owns them, as overlays do.
func (s *Shell) consumeModSurface() *modSurfaceState {
	modSurfaceMu.Lock()
	defer modSurfaceMu.Unlock()
	windows := pendingWindows
	pendingWindows = nil
	floats := pendingFloats
	pendingFloats = nil
	if len(windows) == 0 && len(floats) == 0 {
		return modSurfaces[s]
	}
	st := modSurfaces[s]
	if st == nil {
		st = newModSurfaceState()
		modSurfaces[s] = st
	}
	for _, r := range windows {
		st.setWindow(r.panelID, r.props)
	}
	for _, r := range floats {
		st.setFloat(r.panelID, r.edge, r.props)
	}
	return st
}

// drawModFloats draws the floating regions on their edges. Several on one
// edge cascade so their titles stay reachable.
func (s *Shell) drawModFloats(c *mui.Context, st *modSurfaceState) {
	if st == nil {
		return
	}
	modSurfaceMu.Lock()
	list := make([]*modFloatState, 0, len(st.floatOrder))
	for _, id := range st.floatOrder {
		if f := st.floats[id]; f != nil {
			list = append(list, f)
		}
	}
	modSurfaceMu.Unlock()
	for i, f := range list {
		s.drawModFloat(c, f, i)
	}
}

// drawModFloat draws one floating region: title bar, close, and the panel.
func (s *Shell) drawModFloat(c *mui.Context, f *modFloatState, index int) {
	t := c.Theme()
	offset := float32(index) * 24
	mui.Overlay(c, func() {
		el := mui.Column(c).
			Absolute().
			Width(300).
			Height(260).
			Background(t.Surface).
			Border(1, t.Border).
			Radius(10).
			Shadow(0, 8, 24, 0, mui.RGBA(0, 0, 0, 0.24)).
			Clip()
		switch f.edge {
		case "left":
			el = el.Top(56 + offset).Left(12)
		case "top":
			el = el.Top(56 + offset).Left(320 + offset)
		case "bottom":
			el = el.Bottom(32 + offset).Left(320 + offset)
		default:
			el = el.Top(56 + offset).Right(12)
		}
		el = el.Draggable(&f.pos, mui.DraggableOptions{
			Bounds: &mui.Rect{X: -1600, Y: -56, W: 1600, H: 860},
			Rigid:  true,
		})
		el.Children(func() {
			mui.Row(c).
				Height(30).
				Padding(0, 10).
				Gap(6).
				AlignItems(mui.Center).
				Children(func() {
					mui.Text(c, modPanelTitle(f.panelID)).FontSize(11).Bold().SingleLine()
					mui.Box(c).Grow(1)
					mui.Text(c, "×").
						FontSize(14).
						TextColor(t.TextMuted).
						Padding(0, 4).
						Cursor(mui.CursorPointer).
						Tooltip("Close float").
						OnClick(func() { CloseModFloat(f.panelID) })
				})
			mui.Box(c).Height(1).Background(t.Border)
			mui.Box(c).Fill().Padding(8, 10).Children(func() {
				renderModPanel(c, f.panelID, f.props)
			})
		})
	})
}

// drawModWindows draws each open mod window. A kept window is an overlay
// that ignores clicks outside it; the rest are modals that dismiss on
// Escape or an outside click.
func (s *Shell) drawModWindows(c *mui.Context, st *modSurfaceState) {
	if st == nil {
		return
	}
	modSurfaceMu.Lock()
	list := make([]*modWindowState, 0, len(st.windowOrder))
	for _, id := range st.windowOrder {
		if w := st.windows[id]; w != nil {
			list = append(list, w)
		}
	}
	modSurfaceMu.Unlock()
	for i, w := range list {
		if !w.keep && s.overlayOpen() {
			continue
		}
		if w.keep {
			s.drawKeptWindow(c, w, i)
			continue
		}
		open := w.open
		mui.Modal(c, &w.open, func() { s.modWindowFrame(c, w) })
		if open && !w.open {
			s.removeModWindow(w.panelID)
		}
	}
}

// drawKeptWindow draws a window that stays until its close button.
func (s *Shell) drawKeptWindow(c *mui.Context, w *modWindowState, index int) {
	offset := float32(index) * 24
	mui.Overlay(c, func() {
		s.modWindowFrame(c, w).
			Absolute().
			Top(72+offset).
			Left(80+offset).
			Draggable(&w.pos, mui.DraggableOptions{
				Bounds: &mui.Rect{X: -1400, Y: -72, W: 1400, H: 900},
				Rigid:  true,
			})
	})
}

// modWindowFrame builds the window: title, keep and close, then the panel.
func (s *Shell) modWindowFrame(c *mui.Context, w *modWindowState) mui.Element {
	t := c.Theme()
	keep := "Keep"
	tip := "Keep this window open"
	if w.keep {
		keep = "Kept"
		tip = "Let Escape or a click outside close this window"
	}
	el := mui.Column(c).
		Width(560).
		Height(400).
		Background(t.Surface).
		Border(1, t.Border).
		Radius(10).
		Shadow(0, 10, 30, 0, mui.RGBA(0, 0, 0, 0.3)).
		Clip()
	el.Children(func() {
		mui.Row(c).
			Height(34).
			Padding(0, 12).
			Gap(8).
			AlignItems(mui.Center).
			Children(func() {
				mui.Text(c, modPanelTitle(w.panelID)).FontSize(12).Bold().SingleLine()
				mui.Box(c).Grow(1)
				mui.Text(c, keep).
					FontSize(10).
					TextColor(t.TextMuted).
					Padding(0, 6).
					Cursor(mui.CursorPointer).
					Tooltip(tip).
					OnClick(func() { w.keep = !w.keep })
				mui.Text(c, "×").
					FontSize(14).
					TextColor(t.TextMuted).
					Padding(0, 4).
					Cursor(mui.CursorPointer).
					Tooltip("Close window").
					OnClick(func() { s.removeModWindow(w.panelID) })
			})
		mui.Box(c).Height(1).Background(t.Border)
		mui.Box(c).Fill().Padding(10, 12).Children(func() {
			renderModPanel(c, w.panelID, w.props)
		})
	})
	return el
}

// removeModWindow drops one window from this shell.
func (s *Shell) removeModWindow(panelID string) {
	modSurfaceMu.Lock()
	defer modSurfaceMu.Unlock()
	if st := modSurfaces[s]; st != nil {
		st.removeWindow(panelID)
	}
}
