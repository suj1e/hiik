package builtin

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

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

const (
	spaceHeaderHeight  = 31
	spaceMachineHeight = 26
	spaceRowHeight     = 30
	spaceTreeRowHeight = 28
	spaceAvatarSize    = 18
)

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

// spaceListEntries lays the spaces out in display order: machine headers
// over their rows when the panel groups machines, else the rows alone.
func spaceListEntries(rows []spaceRow, groups []spaceMachineGroup, grouped bool) []spaceListEntry {
	if !grouped {
		out := make([]spaceListEntry, len(rows))
		for i := range rows {
			out[i] = spaceListEntry{Group: -1, Row: i}
		}
		return out
	}
	at := make(map[string]int, len(rows))
	for i, row := range rows {
		at[row.Space.ID] = i
	}
	var out []spaceListEntry
	for gi, group := range groups {
		out = append(out, spaceListEntry{Group: gi, Row: -1})
		for _, row := range group.Spaces {
			if i, ok := at[row.Space.ID]; ok {
				out = append(out, spaceListEntry{Group: -1, Row: i})
			}
		}
	}
	return out
}

// spaceListKey identifies a list entry across frames, for its selection
// and its place.
func spaceListKey(entries []spaceListEntry, rows []spaceRow, groups []spaceMachineGroup, i int) any {
	if i < 0 || i >= len(entries) {
		return i
	}
	e := entries[i]
	if e.Group >= 0 && e.Group < len(groups) {
		return "machine\x00" + groups[e.Group].Machine
	}
	if e.Row >= 0 && e.Row < len(rows) {
		return "space\x00" + rows[e.Row].Space.ID
	}
	return i
}

// spaceEntryFor returns the list entry showing the space id, or -1.
func spaceEntryFor(entries []spaceListEntry, rows []spaceRow, id string) int {
	for i, e := range entries {
		if e.Row >= 0 && e.Row < len(rows) && rows[e.Row].Space.ID == id {
			return i
		}
	}
	return -1
}

// spaceReorderTarget turns a ListState.Reorder target into the index the
// move command wants, counting the spaces in the app's own order. to is
// the list row the dragged rows go before, past the last entry for the
// end, and headers are skipped to the row after them.
func spaceReorderTarget(entries []spaceListEntry, rows []spaceRow, from, to, total int) int {
	if from < 0 || from >= len(entries) || total == 0 {
		return -1
	}
	e := entries[from]
	if e.Row < 0 || e.Row >= len(rows) {
		return -1
	}
	moved := rows[e.Row].Index
	for j := to; j < len(entries); j++ {
		k := entries[j]
		if k.Row < 0 || k.Row >= len(rows) {
			continue
		}
		anchor := rows[k.Row].Index
		if moved < anchor {
			return anchor - 1
		}
		return anchor
	}
	return total - 1
}

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
	entries := spaceListEntries(rows, groups, grouped)

	selected := ui.Local(root, "mods/spaces/selected", func() int { return -1 })
	state := ui.Local(root, "mods/spaces/list", func() ui.ListState { return ui.ListState{} })
	*selected = spaceEntryFor(entries, rows, active)
	state.Key = func(i int) any { return spaceListKey(entries, rows, groups, i) }
	state.Header = nil
	if grouped {
		state.Header = func(i int) bool {
			return i >= 0 && i < len(entries) && entries[i].Group >= 0
		}
	}
	state.Selected = selected
	state.Reorder = nil
	if !compact {
		state.Reorder = func(dragged []int, to int) {
			if len(dragged) != 1 {
				return
			}
			from := dragged[0]
			if from < 0 || from >= len(entries) || entries[from].Row < 0 {
				return
			}
			target := spaceReorderTarget(entries, rows, from, to, len(spaces))
			if target < 0 || target == rows[entries[from].Row].Index {
				return
			}
			_ = rc.App.Run(api.CmdSpaceMove, map[string]any{
				"id": rows[entries[from].Row].Space.ID,
				"to": target,
			})
		}
	}

	root.Children(func() {
		if compact {
			head := ui.Row(c).
				Key("mods/spaces/header").
				Height(spaceHeaderHeight).
				Padding(0, 0, 14, 0).
				Center()
			head.Children(func() {
				if ui.Button(c, "+").
					Key("mods/spaces/new").
					Height(22).
					Width(22).
					Padding(0, 0).
					Radius(6).
					Label("New space").
					Tooltip("New space").
					Clicked() {
					_ = rc.App.Run(api.CmdSpaceNew, nil)
				}
			})
		} else {
			ui.Toolbar(c, func() {
				ui.Text(c, "Spaces").FontSize(11.5).TextColor(t.TextMuted).SingleLine()
				ui.Spacer(c)
				if ui.Button(c, "+").
					Key("mods/spaces/new").
					Height(22).
					Width(22).
					Padding(0, 0).
					Radius(6).
					Label("New space").
					Tooltip("New space").
					Clicked() {
					_ = rc.App.Run(api.CmdSpaceNew, nil)
				}
			}).Height(spaceHeaderHeight).
				Padding(0, 10, 0, 16)
		}

		if len(spaces) == 0 {
			ui.Text(c, "No spaces yet").
				FontSize(t.FontSize-1).
				TextColor(t.TextMuted).
				Padding(0, 4)
			return
		}

		list := ui.List(c, state, len(entries), func(i int) {
			renderSpaceListRow(rc, entries, groups, rows, i, compact, active, statuses)
		}).Fill()
		if compact {
			list.Padding(0, 0, 14, 0)
		} else {
			list.Padding(0, 8, 14, 8)
		}
		list.KeepScrollbars(true)
	})
}

// renderSpaceListRow draws one entry: a machine header, or a space.
func renderSpaceListRow(rc *api.RenderContext, entries []spaceListEntry, groups []spaceMachineGroup, rows []spaceRow, i int, compact bool, active string, statuses map[string]string) {
	if i < 0 || i >= len(entries) {
		return
	}
	e := entries[i]
	if e.Group >= 0 {
		if e.Group < len(groups) {
			renderSpaceMachineHeader(rc, groups[e.Group], compact)
		}
		return
	}
	if e.Row < 0 || e.Row >= len(rows) {
		return
	}
	row := rows[e.Row]
	renderSpaceRow(rc, row, compact, row.Space.ID == active, statuses[row.Space.ID])
}

// spaceMenu is a space's right-click menu: rename or close.
func spaceMenu(rc *api.RenderContext, sp api.Space) func(*ui.Menu) {
	return func(m *ui.Menu) {
		if m.Item("Rename…").Chosen() {
			_ = rc.App.Run(api.CmdSpaceRename, map[string]any{"id": sp.ID})
		}
		m.Separator()
		if m.Item("Close space").Chosen() {
			_ = rc.App.Run(api.CmdSpaceClose, map[string]any{"id": sp.ID})
		}
	}
}

// renderSpaceMachineHeader draws the row that names one machine and offers
// a new space on it. In the rail it is one centered cell.
func renderSpaceMachineHeader(rc *api.RenderContext, group spaceMachineGroup, compact bool) {
	c := rc.C
	t := c.Theme()
	key := "mods/spaces/machine/" + group.Machine
	if group.Machine == "" {
		key = "mods/spaces/machine/local"
	}
	if compact {
		cell := ui.Row(c.Key(key)).
			FillWidth().
			Height(spaceMachineHeight).
			Center().
			Background(t.Background).
			Tooltip(group.Label + " · " + machineStateLabel(group.State))
		cell.Children(func() {
			ui.Text(c, spaceMachineGlyph(group.Machine)).FontSize(12).TextColor(t.TextMuted)
			if group.Machine != "" {
				ui.Box(c).
					Size(5, 5).
					Radius(3).
					Background(machineStateColor(t, group.State)).
					Absolute().
					Right(4).
					Top(6)
			}
		})
		return
	}
	head := ui.Row(c.Key(key)).
		FillWidth().
		Height(spaceMachineHeight).
		Padding(0, 8).
		Gap(7).
		Radius(6).
		AlignItems(ui.Center).
		Background(t.Background).
		Tooltip(group.Label)
	if head.Hovered() {
		head.Background(t.Surface)
	}
	head.Children(func() {
		ui.Text(c, spaceMachineGlyph(group.Machine)).FontSize(12).TextColor(t.TextMuted)
		ui.Text(c, group.Label).
			FontSize(11).
			TextColor(t.TextMuted).
			SingleLine().
			Grow(1)
		if n := len(group.Spaces); n > 0 {
			ui.Badge(c, fmt.Sprintf("%d", n)).
				Tooltip(fmt.Sprintf("%d spaces on %s", n, group.Label))
		}
		ui.Box(c).
			Size(6, 6).
			Radius(3).
			Background(machineStateColor(t, group.State)).
			Tooltip(machineStateLabel(group.State))
		if ui.Button(c, "+").
			Key(key+"/new").
			Height(18).
			Width(18).
			Padding(0, 0).
			Radius(4).
			Label("New space on " + group.Label).
			Tooltip("New space on " + group.Label).
			Clicked() {
			machineNewSpace(rc, group.Machine, group.Label)
		}
	})
}

// renderSpaceRow draws one space: its avatar or worktree prefix, name, git
// tail, and in the rail its number and status glyph.
func renderSpaceRow(rc *api.RenderContext, row spaceRow, compact bool, selected bool, status string) {
	c := rc.C
	t := c.Theme()
	sp := row.Space
	name := spaceDisplayName(sp)
	selectSpace := func() {
		_ = rc.App.Run(api.CmdSpaceSelect, map[string]any{"id": sp.ID})
	}
	menu := spaceMenu(rc, sp)

	if compact {
		cell := ui.Row(c.Key("mods/spaces/" + sp.ID)).
			FillWidth().
			Height(spaceMachineHeight).
			Radius(6).
			Center().
			OnClick(selectSpace).
			ContextMenu(menu)
		cell.Children(func() {
			color := t.TextMuted
			statusColor := sideStatusColor(t, status)
			switch {
			case selected:
				color, statusColor = t.AccentText, t.AccentText
			case cell.Hovered():
				color = t.Text
			}
			ui.Text(c, fmt.Sprintf("%d", row.Index+1)).FontSize(t.FontSize - 1).TextColor(color)
			ui.Text(c, spaceStatusGlyph(status)).
				FontSize(t.FontSize).
				TextColor(statusColor).
				Absolute().
				Right(5)
		})
		return
	}

	height := float32(spaceRowHeight)
	if row.Tree != "" {
		height = spaceTreeRowHeight
	}
	textColor := t.Text
	mutedColor := t.TextMuted
	if selected {
		textColor, mutedColor = t.AccentText, t.AccentText
	}
	el := ui.Row(c.Key("mods/spaces/"+sp.ID)).
		FillWidth().
		Height(height).
		Padding(0, 8).
		Gap(9).
		Radius(8).
		AlignItems(ui.Center).
		OnClick(selectSpace).
		ContextMenu(menu)
	el.Children(func() {
		if row.Tree != "" {
			ui.Text(c, row.Tree+" ⎇").FontSize(11).TextColor(mutedColor).SingleLine()
		} else {
			ui.Avatar(c, name, nil).Size(spaceAvatarSize, spaceAvatarSize)
		}
		ui.Text(c, name).
			FontSize(t.FontSize).
			TextColor(textColor).
			SingleLine().
			MaxWidth(150)
		parts := spaceGitTailParts(row.Git, name, row.Tree != "")
		ui.Row(c).Grow(1).Gap(5).AlignItems(ui.Center).Justify(ui.End).Children(func() {
			for _, part := range parts {
				color := mutedColor
				if !selected {
					switch {
					case strings.HasPrefix(part, "↑"):
						color = t.Success
					case strings.HasPrefix(part, "↓"):
						color = t.Danger
					}
				}
				ui.Text(c, part).FontSize(11.5).TextColor(color).SingleLine()
			}
		})
	})
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

// sideAgentLabel is the one-line name of an agent.
func sideAgentLabel(ag api.Agent) string {
	switch {
	case ag.Title != "":
		return ag.Title
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
