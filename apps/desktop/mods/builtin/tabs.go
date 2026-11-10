package builtin

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"hiik/icons"
	"hiik/mods/api"
)

// tabHeight is the tab chip height, matching the shell's tab bar.
const tabHeight = 29

func init() {
	api.Register(api.Mod{
		ID:   "tabs",
		Name: "Tabs",
		Panels: []api.Panel{{
			ID:     api.PanelTabs,
			Title:  "Tabs",
			Render: renderTabs,
		}},
		Commands: []api.Command{
			sideWrap(api.CmdTabNew, "New tab"),
			sideWrap(api.CmdTabClose, "Close tab"),
			sideWrap(api.CmdTabRename, "Rename tab"),
			sideWrap(api.CmdTabSelect, "Select tab"),
		},
	})
}

// sideTabArgs names a tab, and its space when known, for core commands.
func sideTabArgs(sp api.Space, tabID string) map[string]any {
	args := map[string]any{"tab": tabID}
	if sp.ID != "" {
		args["space"] = sp.ID
	}
	return args
}

// tabEditState is the tab being renamed in place, and the text typed so
// far. A mod cannot open the shell's rename prompt, so the tab strip edits
// itself.
type tabEditState struct {
	id    string
	value string
}

func renderTabs(rc *api.RenderContext) {
	if rc == nil || rc.C == nil || rc.App == nil {
		return
	}
	t := rc.C.Theme()
	var sp api.Space
	found := false
	if v, ok := rc.Props["space"].(*api.Space); ok && v != nil {
		sp = *v
		found = true
	}
	if tabs, ok := rc.Props["tabs"].([]api.Tab); ok && tabs != nil {
		sp.Tabs = tabs
		found = true
	}
	if !found {
		activeID := sideActive(rc)
		for _, s := range sideSpaces(rc) {
			if s.ID == activeID {
				sp, found = s, true
				break
			}
		}
	}
	if !found || len(sp.Tabs) == 0 {
		ui.Text(rc.C, "No tabs").FontSize(t.FontSize - 1).TextColor(t.TextMuted)
		return
	}
	activeIdx := sp.Active
	if v, ok := rc.Props["active"].(int); ok {
		activeIdx = v
	}
	if activeIdx < 0 || activeIdx >= len(sp.Tabs) {
		activeIdx = 0
	}

	selected := activeIdx
	tabs := ui.TabsBase(rc.C.Key("mods/tabs/strip"), &selected, len(sp.Tabs))
	list := tabs.List.
		Height(tabHeight).
		Gap(3).
		AlignItems(ui.Center)
	edit := ui.Local(list, "mods/tabs/edit", func() tabEditState { return tabEditState{} })
	if edit != nil && edit.id != "" {
		live := false
		for _, tb := range sp.Tabs {
			if tb.ID == edit.id {
				live = true
				break
			}
		}
		if !live {
			edit.id = ""
		}
	}
	agents := sideAgents(rc)
	list.Children(func() {
		for i := range sp.Tabs {
			i := i
			tb := sp.Tabs[i]
			name := tb.Name
			if name == "" {
				name = fmt.Sprintf("%d", i+1)
			}
			on := i == selected
			wrap := ui.Row(rc.C.Key("mods/tabs/" + sp.ID + "/" + tb.ID)).
				Height(tabHeight).
				Shrink(0)
			wrap.Drag(tb.ID)
			if dragged, ok := ui.Drop[string](wrap); ok && dragged != tb.ID {
				args := sideTabArgs(sp, dragged)
				args["to"] = i
				_ = rc.App.Run(api.CmdTabMove, args)
			}
			wrap.ContextMenu(tabMenu(rc, sp, tb, edit))
			wrap.Children(func() {
				chip := tabs.Tab(i).
					Height(tabHeight).
					Padding(0, 10).
					Gap(7).
					Radius(t.Radius+3).
					Border(1, t.Surface).
					AlignItems(ui.Center).
					Background(t.Surface).
					TextColor(t.TextMuted)
				if on {
					chip.Background(t.SurfaceHover).
						Border(1, t.Border).
						TextColor(t.Text)
				} else if chip.Hovered() {
					chip.Background(t.SurfaceHover).TextColor(t.Text)
				}
				if chip.DoubleClicked() && edit != nil {
					edit.id = tb.ID
					edit.value = name
				}
				chip.Children(func() {
					status := tabStatusFor(agents, sp, tb)
					if ag, ok := tabAgent(agents, sp, tb); ok {
						if program, ok := sideProgramFor(rc.App, ag, true); ok && sideIconSVG(program.Icon) != nil {
							sideProgramBadge(rc.C, t, program, true, ag.Status)
						}
					}
					if edit != nil && edit.id == tb.ID {
						field := ui.TextInput(rc.C.Key("mods/tabs/rename/"+tb.ID), &edit.value).
							Height(tabHeight - 7).
							Width(150).
							FontSize(t.FontSize - 1)
						field.Focus()
						field.HandleInput(func(ev ui.InputEvent) bool {
							if ev.Kind == ui.InputKeyDown && ev.Key == ui.KeyEscape {
								edit.id = ""
								return true
							}
							return false
						})
						if field.Submitted() {
							if value := strings.TrimSpace(edit.value); value != "" {
								args := sideTabArgs(sp, tb.ID)
								args["name"] = value
								_ = rc.App.Run(api.CmdTabRename, args)
							}
							edit.id = ""
						}
						return
					}
					ui.Text(rc.C, name).
						FontSize(t.FontSize - 0.5).
						TextColor(func() ui.Color {
							if on {
								return t.Text
							}
							return t.TextMuted
						}()).
						SingleLine().
						MaxWidth(170)
					switch status {
					case "working":
						ui.Spinner(rc.C).Size(12, 12).TextColor(t.Accent).Label("Working")
					case "blocked", "done":
						label := "Done"
						if status == "blocked" {
							label = "Blocked"
						}
						ui.Box(rc.C).
							Size(6, 6).
							Radius(3).
							Background(func() ui.Color {
								if status == "blocked" {
									return t.Danger
								}
								return t.Success
							}()).
							Label(label)
					}
					if on {
						close := ui.Icon(rc.C, icons.X).
							FontSize(t.FontSize-1).
							TextColor(t.TextMuted).
							Padding(0, 2).
							Cursor(ui.CursorPointer).
							Label("Close tab").
							Tooltip("Close tab").
							OnClick(func() {
								_ = rc.App.Run(api.CmdTabClose, sideTabArgs(sp, tb.ID))
							})
						close.Description("Close tab")
					}
				})
			})
		}
	})
	if list.Changed() && selected != activeIdx {
		_ = rc.App.Run(api.CmdTabSelect, sideTabArgs(sp, sp.Tabs[selected].ID))
	}
}

// tabMenu is a tab's right-click menu: rename, new, close.
func tabMenu(rc *api.RenderContext, sp api.Space, tb api.Tab, edit *tabEditState) func(*ui.Menu) {
	return func(m *ui.Menu) {
		if m.Item("Rename…").Chosen() && edit != nil {
			edit.id = tb.ID
			edit.value = tb.Name
			if edit.value == "" {
				edit.value = fmt.Sprintf("%d", tabIndex(sp, tb.ID)+1)
			}
		}
		if m.Item("New tab").Chosen() {
			_ = rc.App.Run(api.CmdTabNew, map[string]any{"space": sp.ID})
		}
		m.Separator()
		if m.Item("Close tab").Chosen() {
			_ = rc.App.Run(api.CmdTabClose, sideTabArgs(sp, tb.ID))
		}
	}
}

// tabIndex is a tab's place in its space.
func tabIndex(sp api.Space, id string) int {
	for i := range sp.Tabs {
		if sp.Tabs[i].ID == id {
			return i
		}
	}
	return -1
}

// tabAgent finds the agent a tab shows, by its tab ID or by a terminal in
// the tab's panes.
func tabAgent(agents []api.Agent, sp api.Space, tb api.Tab) (api.Agent, bool) {
	for _, ag := range agents {
		if ag.Space != "" && ag.Space != sp.ID {
			continue
		}
		if ag.Tab != "" && ag.Tab == tb.ID {
			return ag, true
		}
	}
	terms := map[string]bool{}
	tabTerminals(tb.Root, terms)
	for _, ag := range agents {
		if ag.Space != "" && ag.Space != sp.ID {
			continue
		}
		if terms[ag.Address] {
			return ag, true
		}
	}
	return api.Agent{}, false
}

// tabTerminals collects the terminal addresses of a tab's panes.
func tabTerminals(n *api.Node, out map[string]bool) {
	if n == nil {
		return
	}
	if n.Terminal != "" {
		out[n.Terminal] = true
	}
	tabTerminals(n.A, out)
	tabTerminals(n.B, out)
}

// tabStatusFor is the loudest status among the agents in a tab.
func tabStatusFor(agents []api.Agent, sp api.Space, tb api.Tab) string {
	seen := map[string]bool{}
	terms := map[string]bool{}
	tabTerminals(tb.Root, terms)
	for _, ag := range agents {
		if ag.Space != "" && ag.Space != sp.ID {
			continue
		}
		if ag.Tab != tb.ID && !terms[ag.Address] {
			continue
		}
		seen[ag.Status] = true
	}
	switch {
	case seen["working"]:
		return "working"
	case seen["blocked"]:
		return "blocked"
	case seen["done"]:
		return "done"
	}
	return ""
}

// dragDropIndex is the index a dragged row lands at: the row whose center
// is nearest the dragged center in the drag's axis. The dragged row counts
// too, so a small drag stays put. It returns -1 when home is not known. The
// spaces panel still reorders its rows with it; tabs use ui.Drag/ui.Drop.
func dragDropIndex(ids []string, id string, homes map[string]ui.Rect, offset *ui.Point, horizontal bool) int {
	home, ok := homes[id]
	if !ok || home.W <= 0 {
		return -1
	}
	pos := home.X + home.W/2
	if horizontal {
		pos += offset.X
	} else {
		pos = home.Y + home.H/2 + offset.Y
	}
	target := -1
	var best float32
	for i, rowID := range ids {
		r, ok := homes[rowID]
		if !ok || r.W <= 0 {
			continue
		}
		center := r.X + r.W/2
		if !horizontal {
			center = r.Y + r.H/2
		}
		d := center - pos
		if d < 0 {
			d = -d
		}
		if target < 0 || d < best {
			target, best = i, d
		}
	}
	return target
}
