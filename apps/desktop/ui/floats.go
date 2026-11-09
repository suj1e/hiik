package ui

import (
	"fmt"
	"sync"

	mui "github.com/egoist/mygo/ui"

	"hiik/mods/api"
)

// The floating inspector: a rounded panel pinned to the right edge that
// shows the focused space, tab and pane, the agent state and the core
// connection. It is closed by default and drawn only while open, so it
// never covers a terminal it does not need to.

// floatState is one window's inspector: whether it is open and where the
// user dragged it, relative to its pinned position.
type floatState struct {
	open bool
	pos  mui.Point
}

var (
	floatMu sync.Mutex
	floats  = make(map[*Shell]*floatState)
)

func init() {
	registerShellHook(runFloatHook)
}

// floatFor returns the floating state of one window, creating it closed.
func floatFor(s *Shell) *floatState {
	floatMu.Lock()
	defer floatMu.Unlock()
	st := floats[s]
	if st == nil {
		st = &floatState{}
		floats[s] = st
	}
	return st
}

// toggleFloat opens or closes the inspector.
func toggleFloat(s *Shell) {
	st := floatFor(s)
	st.open = !st.open
}

// runFloatHook draws the inspector while it is open. Overlays keep the
// keyboard, so the panel steps aside for a modal.
func runFloatHook(s *Shell, c *mui.Context) {
	if s == nil || s.app == nil || s.overlayOpen() {
		return
	}
	s.floatingInspector(c)
}

// floatingInspector draws the panel over the shell from the right edge.
func (s *Shell) floatingInspector(c *mui.Context) {
	st := floatFor(s)
	if !st.open {
		return
	}
	t := c.Theme()
	sp, tab := s.activeTab()
	values := s.inspectorValues(sp, tab)

	mui.Overlay(c, func() {
		panel := mui.Column(c).
			Absolute().
			Top(56).
			Right(12).
			Width(304).
			Height(336).
			Background(t.Surface).
			Border(1, t.Border).
			Radius(10).
			Shadow(0, 8, 24, 0, mui.RGBA(0, 0, 0, 0.28)).
			Draggable(&st.pos, mui.DraggableOptions{
				// The panel starts at the right edge; keep it near a
				// plausible window so the close button stays reachable.
				Bounds: &mui.Rect{X: -1600, Y: -52, W: 1600, H: 900},
				Rigid:  true,
			})
		panel.Children(func() {
			mui.Row(c).
				Height(34).
				Padding(0, 12).
				Gap(8).
				AlignItems(mui.Center).
				Children(func() {
					mui.Text(c, "Inspector").FontSize(12).Bold()
					mui.Box(c).Grow(1)
					mui.Text(c, "×").
						FontSize(14).
						TextColor(t.TextMuted).
						Padding(0, 4).
						Cursor(mui.CursorPointer).
						Tooltip("Close inspector").
						OnClick(func() { st.open = false })
				})
			mui.Box(c).Height(1).Background(t.Border)
			mui.Column(c).Fill().Padding(10, 12).Gap(2).Children(func() {
				for _, row := range values {
					inspectorRow(c, row[0], row[1])
				}
			})
			mui.Text(c, "drag to move · close to dismiss").
				FontSize(10).
				TextColor(t.TextMuted).
				Padding(0, 12, 8, 12)
		})
	})
}

// inspectorValues builds the rows shown in the panel, all from snapshots.
func (s *Shell) inspectorValues(sp *api.Space, tab *api.Tab) [][2]string {
	space := "none"
	if sp != nil {
		space = sp.Name
		if space == "" {
			space = sp.ID
		}
	}
	tabName := "none"
	if tab != nil {
		tabName = tab.Name
		if tabName == "" && sp != nil {
			tabName = fmt.Sprintf("Tab %d", sp.Active+1)
		}
	}
	pane := "none"
	var node *api.Node
	if tab != nil && s.focus != "" {
		node = findNode(tab.Root, s.focus)
		if node != nil {
			pane = node.ID
		}
	}
	terminal := "none"
	if node != nil && node.Terminal != "" {
		terminal = node.Terminal
	}

	agents := s.app.Agents()
	agent := "—"
	if node != nil && node.Terminal != "" {
		agent = agentStatus(agents, node.Terminal)
	}
	core := s.app.Connection()
	if v := s.app.CoreVersion(); v != "" {
		core += " · " + v
	}
	return [][2]string{
		{"space", space},
		{"tab", tabName},
		{"pane", pane},
		{"terminal", terminal},
		{"agent", agent},
		{"core", core},
	}
}

// agentStatus returns the status of the agent on terminal, or "—".
func agentStatus(agents []api.Agent, terminal string) string {
	for _, ag := range agents {
		if ag.Address == terminal {
			if ag.Status == "" {
				return "idle"
			}
			return ag.Status
		}
	}
	return "—"
}

// inspectorRow draws one label and value line.
func inspectorRow(c *mui.Context, label, value string) {
	t := c.Theme()
	mui.Row(c).
		Height(24).
		Gap(8).
		AlignItems(mui.Center).
		Children(func() {
			mui.Text(c, label).FontSize(10).TextColor(t.TextMuted).Width(62)
			mui.Text(c, value).FontSize(11).SingleLine().TextColor(t.Text)
		})
}
