package builtin

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"hiik/icons"
	"hiik/mods/api"
)

func init() {
	api.Register(api.Mod{
		ID:   "spaces",
		Name: "Spaces",
		Panels: []api.Panel{{
			ID:     api.PanelSpaces,
			Title:  "Spaces",
			Render: renderSpaces,
		}},
		Commands: []api.Command{
			sideWrap(api.CmdSpaceNew, "New space"),
			sideWrap(api.CmdSpaceRename, "Rename space"),
			sideWrap(api.CmdSpaceClose, "Close space"),
			sideWrap(api.CmdSpaceMove, "Move space"),
			sideWrap(api.CmdSpaceSelect, "Select space"),
		},
	})
}

// sideWrap registers a mod command that forwards its arguments to the
// app's command of the same ID.
func sideWrap(id, title string) api.Command {
	return api.Command{
		ID:    id,
		Title: title,
		Run: func(app api.App, args map[string]any) error {
			return app.Run(id, args)
		},
	}
}

// sideCompact reports whether the host drew the sidebar as a narrow rail.
func sideCompact(props map[string]any) bool {
	v, _ := props["compact"].(bool)
	return v
}

// spaceListEntry is one row of the spaces list: a machine header, or a
// space row.
type spaceListEntry struct {
	Group int // the machine group this heads, -1 for a space row
	Row   int // the space row, -1 for a machine header
}

// renderSpaces draws the source list of spaces. The normal sidebar is a
// ui.Sidebar: it owns selection, the keyboard and the source-list look. The
// compact rail is a ui.List of numbers.
func renderSpaces(rc *api.RenderContext) {
	if rc == nil || rc.C == nil || rc.App == nil {
		return
	}
	c := rc.C
	t := c.Theme()
	spaces := sideSpaces(rc)
	active := sideActive(rc)
	compact := sideCompact(rc.Props)

	root := ui.Column(c).Key("mods/spaces/panel").Fill()
	cache := ui.Local(root, "mods/spaces/git", newSpaceGitCache)
	if cache == nil {
		value := newSpaceGitCache()
		cache = &value
	}
	rows := spaceRows(spaces, func(sp api.Space) spaceGitInfo {
		return cache.info(rc.App, sp.Machine, spaceGitFolder(rc.App, sp))
	})
	groups, grouped := spaceMachineGroups(rows, rc.App.Machines())
	statuses := spaceStatuses(sideAgents(rc))

	root.Children(func() {
		if compact {
			compactSpacesHeader(rc)
			if len(rows) == 0 {
				ui.Text(c, "No spaces yet").
					FontSize(t.FontSize-1).
					TextColor(t.TextMuted).
					Padding(0, 4)
				return
			}
			renderCompactSpaces(rc, root, rows, statuses, active)
			return
		}

		spacesToolbar(rc, groups)
		if len(spaces) == 0 {
			ui.Text(c, "No spaces yet").
				FontSize(t.FontSize-1).
				TextColor(t.TextMuted).
				Padding(0, t.Space(1))
			return
		}

		selected := active
		open := ui.Local(root, "mods/spaces/sections", func() map[string]*bool {
			return map[string]*bool{}
		})
		side := ui.Sidebar(c, &selected, func() {
			if grouped {
				for _, group := range groups {
					if len(group.Spaces) == 0 {
						continue
					}
					key := group.Machine
					if key == "" {
						key = "local"
					}
					shown := (*open)[key]
					if shown == nil {
						value := true
						shown = &value
						(*open)[key] = shown
					}
					group := group
					ui.SidebarSection(c, spaceMachineSectionTitle(group), shown, func() {
						for _, row := range group.Spaces {
							spaceSidebarItem(rc, row, statuses)
						}
					})
				}
				return
			}
			for _, row := range rows {
				spaceSidebarItem(rc, row, statuses)
			}
		}).Grow(1).Label("Spaces")
		if side.Changed() && selected != "" && selected != active {
			_ = rc.App.Run(api.CmdSpaceSelect, map[string]any{"id": selected})
		}
	})
}

// spacesToolbar is the source list's title and its one "new" menu: a space
// here, or on any known machine.
func spacesToolbar(rc *api.RenderContext, groups []spaceMachineGroup) {
	c := rc.C
	t := c.Theme()
	ui.Toolbar(c, func() {
		ui.Text(c, "Spaces").FontSize(t.FontSize - 1).TextColor(t.TextMuted)
		ui.Spacer(c)
		newSpace := ui.MenuButton(c, "+", func(m *ui.Menu) {
			if m.Item("New space").Chosen() {
				_ = rc.App.Run(api.CmdSpaceNew, nil)
			}
			separated := false
			for _, group := range groups {
				if group.Machine == "" {
					continue
				}
				if !separated {
					m.Separator()
					separated = true
				}
				if m.Item("New space on " + group.Label).Chosen() {
					machineNewSpace(rc, group.Machine, group.Label)
				}
			}
		})
		newSpace.Key("mods/spaces/new").Label("New space").Tooltip("New space")
	}).Height(t.Space(7)).
		Padding(0, t.Space(2.5), 0, t.Space(3.5))
}

// compactSpacesHeader is the "+" above the rail.
func compactSpacesHeader(rc *api.RenderContext) {
	c := rc.C
	t := c.Theme()
	head := ui.Row(c).
		Key("mods/spaces/header").
		Height(t.Space(7)).
		Padding(0, 0, t.Space(3.5), 0).
		Center()
	head.Children(func() {
		if ui.Button(c, "+").
			Key("mods/spaces/new").
			Height(t.Space(5.5)).
			Width(t.Space(5.5)).
			Padding(0, 0).
			Radius(t.Space(1.5)).
			Label("New space").
			Tooltip("New space").
			Clicked() {
			_ = rc.App.Run(api.CmdSpaceNew, nil)
		}
	})
}

// renderCompactSpaces draws the rail: one numbered row per space, in the
// native list so the arrows and the selected row follow it.
func renderCompactSpaces(rc *api.RenderContext, root ui.Element, rows []spaceRow, statuses map[string]string, active string) {
	c := rc.C
	t := c.Theme()
	state := ui.Local(root, "mods/spaces/compact/list", func() ui.ListState { return ui.ListState{} })
	selected := -1
	for i := range rows {
		if rows[i].Space.ID == active {
			selected = i
			break
		}
	}
	state.Key = func(i int) any { return rows[i].Space.ID }
	state.Selected = &selected
	listFocused := state.Handle.FocusWithin(c)
	list := ui.List(c, state, len(rows), func(i int) {
		renderCompactSpaceRow(rc, rows[i], i == selected, statuses[rows[i].Space.ID], listFocused)
	}).Fill()
	list.Padding(0, 0, t.Space(3.5), 0)
	list.KeepScrollbars(true)
	if list.Changed() && selected >= 0 && selected < len(rows) {
		_ = rc.App.Run(api.CmdSpaceSelect, map[string]any{"id": rows[selected].Space.ID})
	}
}

// renderCompactSpaceRow is one numbered cell of the rail.
func renderCompactSpaceRow(rc *api.RenderContext, row spaceRow, selected bool, status string, listFocused bool) {
	c := rc.C
	t := c.Theme()
	sp := row.Space
	cell := ui.Row(c.Key("mods/spaces/" + sp.ID)).
		FillWidth().
		Height(t.Space(6)).
		Radius(t.Radius).
		Center().
		OnClick(func() {
			_ = rc.App.Run(api.CmdSpaceSelect, map[string]any{"id": sp.ID})
		}).
		ContextMenu(spaceMenu(rc, sp))
	if selected {
		if listFocused {
			cell.Background(t.Accent)
		} else {
			cell.Background(t.SurfacePressed)
		}
	} else if cell.Hovered() {
		cell.Background(t.SurfaceHover)
	}
	cell.Children(func() {
		color := t.TextMuted
		statusColor := sideStatusColor(t, status)
		switch {
		case selected && listFocused:
			color, statusColor = t.AccentText, t.AccentText
		case selected:
			color = t.Text
		case cell.Hovered():
			color = t.Text
		}
		ui.Text(c, fmt.Sprintf("%d", row.Index+1)).
			FontSize(t.FontSize - 1).
			TextColor(color)
		ui.Box(c).
			Size(t.Space(1.25), t.Space(1.25)).
			Radius(t.Space(0.625)).
			Background(statusColor).
			Absolute().
			Right(t.Space(1))
	})
}

// spaceSidebarItem is one space in the native source list.
func spaceSidebarItem(rc *api.RenderContext, row spaceRow, statuses map[string]string) {
	c := rc.C
	t := c.Theme()
	sp := row.Space
	name := spaceDisplayName(sp)
	label := name
	icon := icons.Folder
	if row.Tree != "" {
		label = row.Tree + " " + name
		icon = icons.Branch
	}
	item := ui.SidebarItem(c, sp.ID, icon, label)
	item.ContextMenu(spaceMenu(rc, sp))
	item.Tooltip(spaceSidebarTooltip(row))
	item.Drag(sp.ID)
	if over, ok := ui.DragOver[string](item); ok && over != sp.ID {
		item.Border(1, t.Accent)
	}
	if dropped, ok := ui.Drop[string](item); ok && dropped != sp.ID {
		_ = rc.App.Run(api.CmdSpaceMove, map[string]any{"id": dropped, "to": row.Index})
	}
	item.Children(func() {
		for _, part := range spaceGitTailParts(row.Git, name, row.Tree != "") {
			ui.Text(c, part).
				FontSize(t.FontSize - 2).
				SingleLine().
				MaxWidth(t.Rem(7))
		}
		if status := statuses[sp.ID]; status != "" {
			ui.Box(c).
				Size(t.Space(1.5), t.Space(1.5)).
				Radius(t.Space(0.75)).
				Background(sideStatusColor(t, status)).
				Tooltip(status)
		}
	})
}

// spaceSidebarTooltip names the space and its folder, or the machine when
// the folder is empty.
func spaceSidebarTooltip(row spaceRow) string {
	sp := row.Space
	parts := []string{spaceDisplayName(sp)}
	if sp.Folder != "" {
		parts = append(parts, sp.Folder)
	}
	if sp.Machine != "" {
		parts = append(parts, sp.Machine)
	}
	return strings.Join(parts, " · ")
}

// spaceMachineSectionTitle is a machine section's one line: its name, its
// OS and a state other than ready.
func spaceMachineSectionTitle(group spaceMachineGroup) string {
	parts := []string{group.Label}
	if group.OS != "" {
		parts = append(parts, group.OS)
	}
	if group.State != "" && group.State != "ready" {
		parts = append(parts, machineStateLabel(group.State))
	}
	return strings.Join(parts, " · ")
}

// spaceMenu is a space's right-click menu: rename, move or close. Moving by
// command keeps reorder available in the native source list, which does not
// drag its items.
func spaceMenu(rc *api.RenderContext, sp api.Space) func(*ui.Menu) {
	spaces := rc.App.Spaces()
	index := -1
	for i := range spaces {
		if spaces[i].ID == sp.ID {
			index = i
			break
		}
	}
	return func(m *ui.Menu) {
		if m.Item("Rename…").Chosen() {
			_ = rc.App.Run(api.CmdSpaceRename, map[string]any{"id": sp.ID})
		}
		if index > 0 {
			if m.Item("Move up").Chosen() {
				_ = rc.App.Run(api.CmdSpaceMove, map[string]any{"id": sp.ID, "delta": -1})
			}
		}
		if index >= 0 && index+1 < len(spaces) {
			if m.Item("Move down").Chosen() {
				_ = rc.App.Run(api.CmdSpaceMove, map[string]any{"id": sp.ID, "delta": 1})
			}
		}
		m.Separator()
		if m.Item("Close space").Chosen() {
			_ = rc.App.Run(api.CmdSpaceClose, map[string]any{"id": sp.ID})
		}
	}
}

// sideSpaces returns the spaces the host passed, else the app's snapshot.
func sideSpaces(rc *api.RenderContext) []api.Space {
	if v, ok := rc.Props["spaces"].([]api.Space); ok && v != nil {
		return v
	}
	return rc.App.Spaces()
}

// sideAgents returns the agents the host passed, else the app's snapshot.
func sideAgents(rc *api.RenderContext) []api.Agent {
	if v, ok := rc.Props["agents"].([]api.Agent); ok && v != nil {
		return v
	}
	return rc.App.Agents()
}

// sideActive returns the space the host marked active, else the app's.
func sideActive(rc *api.RenderContext) string {
	if v, ok := rc.Props["active"].(string); ok && v != "" {
		return v
	}
	return rc.App.ActiveSpace()
}

// spaceStatuses returns the highest-priority agent status in each space,
// so a space's dot shows where it needs attention.
func spaceStatuses(agents []api.Agent) map[string]string {
	out := make(map[string]string, len(agents))
	for _, ag := range agents {
		if ag.Space == "" {
			continue
		}
		old, ok := out[ag.Space]
		if !ok || sideAgentRank(ag.Status) < sideAgentRank(old) {
			out[ag.Space] = ag.Status
		}
	}
	return out
}

func sideAgentRank(status string) int {
	switch status {
	case "blocked":
		return 0
	case "done":
		return 1
	case "working":
		return 2
	default:
		return 3
	}
}

func sideStatusColor(t *ui.Theme, status string) ui.Color {
	switch status {
	case "blocked":
		return t.Warning
	case "done":
		return t.Success
	case "working":
		return t.Accent
	default:
		return t.TextMuted
	}
}

// terminalInfoSource is the optional live terminal snapshot a panel can
// read to follow the folder a terminal cd'd into.
type terminalInfoSource interface {
	TerminalInfos() map[string]api.TerminalInfo
}

// spaceGitFolder is the folder git should inspect for a space: the first
// live terminal's current folder, else the folder the space was made in.
func spaceGitFolder(app api.App, sp api.Space) string {
	if src, ok := app.(terminalInfoSource); ok {
		infos := src.TerminalInfos()
		for _, tab := range sp.Tabs {
			var found string
			var walk func(*api.Node)
			walk = func(n *api.Node) {
				if n == nil || found != "" {
					return
				}
				if n.Terminal != "" {
					if info, ok := infos[n.Terminal]; ok && info.Cwd != "" {
						found = info.Cwd
						return
					}
				}
				walk(n.A)
				walk(n.B)
			}
			walk(tab.Root)
			if found != "" {
				return found
			}
		}
	}
	return sp.Folder
}

// sideStr reads a string from panel props.
func sideStr(props map[string]any, key string) string {
	v, _ := props[key].(string)
	return v
}

// sideSetting reads a setting from the panel's props map, then from the
// app's snapshot, then from a prop of the same name, then def.
func sideSetting(settings map[string]any, props map[string]any, key string, def bool) bool {
	if v, ok := settings[key].(bool); ok {
		return v
	}
	if v, ok := props[key].(bool); ok {
		return v
	}
	return def
}

// sideSettings is the app's settings snapshot with the host's props map
// laid over it, so a prop wins only where it names a key.
func sideSettings(rc *api.RenderContext) map[string]any {
	app := rc.App.Settings()
	props, ok := rc.Props["settings"].(map[string]any)
	if !ok || props == nil {
		return app
	}
	merged := make(map[string]any, len(app)+len(props))
	for key, value := range app {
		merged[key] = value
	}
	for key, value := range props {
		merged[key] = value
	}
	return merged
}

// sideFindAgent returns the agent at an address.
func sideFindAgent(app api.App, address string) (api.Agent, bool) {
	if address == "" {
		return api.Agent{}, false
	}
	for _, ag := range app.Agents() {
		if ag.Address == address {
			return ag, true
		}
	}
	return api.Agent{}, false
}

// sideAgentLabel is the one-line name of an agent. A title a program
// prefixes with status glyphs is not shown; the app names the row instead,
// and the row's tooltip keeps the full title.
func sideAgentLabel(ag api.Agent) string {
	if title := paneTitleText(ag.Title); title != "" {
		return title
	}
	switch {
	case ag.App != "":
		return ag.App
	default:
		return ag.Address
	}
}

// sideNames maps space and tab IDs to their display names from one
// snapshot, so a panel does not copy the model per row.
func sideNames(spaces []api.Space) (spaceNames, tabNames map[string]string) {
	spaceNames = make(map[string]string, len(spaces))
	tabNames = make(map[string]string)
	for _, sp := range spaces {
		name := sp.Name
		if name == "" {
			name = sp.Folder
		}
		if name == "" {
			name = sp.ID
		}
		spaceNames[sp.ID] = name
		for i, tb := range sp.Tabs {
			tabName := tb.Name
			if tabName == "" {
				tabName = fmt.Sprintf("%d", i+1)
			}
			tabNames[tb.ID] = tabName
		}
	}
	return spaceNames, tabNames
}
