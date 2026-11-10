package ui

import (
	"sync"

	mui "github.com/egoist/mygo/ui"

	"hiik/mods/api"
)

// Mod panels are Go functions registered with api.Register. This file is
// the pane side of the host: a leaf node whose Mod names a panel renders
// that panel in the pane body. Windows and floats live in modwindow.go.

func init() {
	registerModPaneRenderer(drawModPane)
}

var (
	modAppMu sync.RWMutex
	modApp   api.App
)

// setModSurfaceApp records the app the panels render against. All windows
// share one app, so the last frame's app is the right one for a panel.
func setModSurfaceApp(a api.App) {
	modAppMu.Lock()
	modApp = a
	modAppMu.Unlock()
}

// modSurfaceApp returns the app panels render against, or nil before the
// shell's first frame.
func modSurfaceApp() api.App {
	modAppMu.RLock()
	defer modAppMu.RUnlock()
	return modApp
}

// drawModPane draws the panel a pane node names. An unknown panel or one
// without a renderer shows a notice instead of an empty pane.
func drawModPane(c *mui.Context, n *api.Node) bool {
	if n == nil || n.Mod == "" {
		return false
	}
	renderModPanel(c, n.Mod, n.Props)
	return true
}

// renderModPanel draws one registered panel by ID. It reports whether a
// renderer ran; a false result leaves the notice visible.
func renderModPanel(c *mui.Context, panelID string, props map[string]any) bool {
	p, ok := api.Panels()[panelID]
	if !ok {
		modPanelNotice(c, "Mod panel "+quotePanel(panelID)+" is not installed.")
		return false
	}
	if p.Render == nil {
		modPanelNotice(c, "Mod panel "+quotePanel(panelID)+" has no renderer.")
		return false
	}
	p.Render(&api.RenderContext{C: c, App: modSurfaceApp(), Props: props})
	return true
}

// modPanelTitle is a panel's title, or its ID when it has none.
func modPanelTitle(panelID string) string {
	if p, ok := api.Panels()[panelID]; ok && p.Title != "" {
		return p.Title
	}
	return panelID
}

// modPanelNotice fills the host area with a short explanation.
func modPanelNotice(c *mui.Context, text string) {
	t := c.Theme()
	mui.Column(c).Fill().Center().Padding(12).Children(func() {
		mui.Text(c, text).FontSize(t.FontSize - 1).TextColor(t.TextMuted).SingleLine()
	})
}

// quotePanel quotes a panel ID for a notice without pulling in JSON.
func quotePanel(id string) string {
	return "\"" + id + "\""
}
