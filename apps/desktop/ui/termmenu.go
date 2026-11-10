package ui

import (
	mui "github.com/egoist/mygo/ui"

	"hiik/icons"
	"hiik/mods/api"
)

// paneMenu is the app's menu of terminal actions: Find, Split Right, Split
// Down and Close Pane. The terminal's own view menu already offers Copy,
// Paste, Select All and Clear on the grid, and mygo exposes no hook to add
// items to it, so this wrapper adds only the app actions on the pane header
// rather than building a second copy of the terminal menu.
func (s *Shell) paneMenu(m *mui.Menu, n *api.Node) {
	if n == nil {
		return
	}
	if m.Item("Find…").Chosen() {
		s.setFocus(n.ID)
		s.openFind()
	}
	m.Separator()
	if m.Item("Split Right").Chosen() {
		s.run(api.CmdPaneSplit, map[string]any{
			"pane": n.ID, "node": n.ID, "id": n.ID, "dir": "right",
		})
	}
	if m.Item("Split Down").Chosen() {
		s.run(api.CmdPaneSplit, map[string]any{
			"pane": n.ID, "node": n.ID, "id": n.ID, "dir": "down",
		})
	}
	m.Separator()
	if m.Item("Close Pane").Chosen() {
		s.run(api.CmdPaneClose, map[string]any{
			"id": n.ID, "node": n.ID, "terminal": n.Terminal,
		})
	}
}

// paneMenuButton draws the pane header's "more" menu button.
func (s *Shell) paneMenuButton(c *mui.Context, n *api.Node) {
	button := mui.MenuButton(c, "", func(m *mui.Menu) { s.paneMenu(m, n) }).
		Height(20).
		Label("Pane menu").
		Tooltip("Pane menu")
	button.Children(func() {
		mui.Icon(c, icons.More).FontSize(c.Theme().FontSize + 1).TextColor(c.Theme().TextMuted)
	})
}
