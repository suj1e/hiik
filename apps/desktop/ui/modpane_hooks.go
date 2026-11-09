package ui

import (
	mui "github.com/egoist/mygo/ui"

	"hiik/mods/api"
)

// modPaneRenderer draws a pane whose node names a mod panel. It returns
// false when the node is not a mod pane, so the caller can fall back.
type modPaneRenderer func(*mui.Context, *api.Node) bool

var modPaneRender modPaneRenderer

// registerModPaneRenderer installs the mod-pane renderer. A second call
// replaces the first.
func registerModPaneRenderer(fn modPaneRenderer) {
	modPaneRender = fn
}

// renderModPane draws a mod pane, or reports that n is not one.
func renderModPane(c *mui.Context, n *api.Node) bool {
	if n == nil || n.Mod == "" || modPaneRender == nil {
		return false
	}
	return modPaneRender(c, n)
}
