package builtin

// Changes lists a repository's staged, unstaged, untracked and conflicted
// paths and previews a unified diff. It works on this computer or on a
// space's machine through the core, and treats a folder outside a
// repository as an ordinary, empty state rather than an error dialog.
//
// The panel runs git only when the folder first appears and when Refresh is
// pressed; frames read the cached result under changesMu.

import (
	"context"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"hiik/mods/api"
)

const changesMaxDisplayBytes = 512 << 10

func init() {
	api.Register(api.Mod{
		ID:   "changes",
		Name: "Changes",
		Panels: []api.Panel{{
			ID:     api.PanelChanges,
			Title:  "Changes",
			Render: renderChangesPanel,
		}},
	})
}

// changeSelection names the row whose diff is shown.
type changeSelection struct {
	group string
	path  string
}

func (s *changeSelection) key() string {
	if s == nil {
		return ""
	}
	return s.group + "\x00" + s.path
}

// changesState is the per-folder cache.
type changesState struct {
	folder    string
	on        string // "" is this computer
	app       api.App
	split     float32 // percent of the first pane
	err       string
	entries   []StatusEntry
	selection *changeSelection
	diff      string
	diffErr   string
	list      ui.ListState
}

var (
	changesMu       sync.Mutex
	changesByFolder = map[string]*changesState{}
	changesTimers   = map[string]context.CancelFunc{}
)

func resetChangesState() {
	changesMu.Lock()
	for _, cancel := range changesTimers {
		cancel()
	}
	changesTimers = map[string]context.CancelFunc{}
	changesByFolder = map[string]*changesState{}
	changesMu.Unlock()
}

// changesStateOnLocked returns the state for a folder on a machine. The
// caller holds changesMu.
func changesStateOnLocked(folder, on string, app api.App) *changesState {
	key := folder
	if on != "" {
		key = on + "\x00" + folder
	}
	if st := changesByFolder[key]; st != nil {
		if app != nil {
			st.app = app
		}
		return st
	}
	st := &changesState{folder: folder, on: on, app: app, split: 38}
	st.reloadLocked()
	changesByFolder[key] = st
	changesTimerLocked(st, key)
	return st
}

// changesTimerLocked refreshes git status on a timer once the app runs. The
// caller holds changesMu.
func changesTimerLocked(st *changesState, key string) {
	if !mygo.App.IsReady() {
		return
	}
	if _, ok := changesTimers[key]; ok {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	changesTimers[key] = cancel
	every := 5 * time.Second
	if st.on != "" {
		every = 15 * time.Second
	}
	go func() {
		ticker := time.NewTicker(every)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				changesMu.Lock()
				if cur := changesByFolder[key]; cur != nil {
					cur.reloadLocked()
				}
				changesMu.Unlock()
				if inv, ok := st.app.(interface{ Invalidate() }); ok {
					inv.Invalidate()
				}
			}
		}
	}()
}

// label names the machine and folder in the header.
func (st *changesState) label() string { return machineLabel(st.on, st.folder) }

// reloadLocked asks git status and drops a selection that no longer exists.
// The caller holds changesMu.
func (st *changesState) reloadLocked() {
	sel := st.selection
	var (
		entries []StatusEntry
		err     error
	)
	if st.on != "" {
		entries, err = GitStatusOn(st.app, st.on, st.folder)
	} else {
		entries, err = GitStatus(st.folder)
	}
	if err != nil {
		st.err = GitMessage(err)
		st.entries = nil
		st.selection = nil
		st.diff = ""
		st.diffErr = ""
		return
	}
	st.err = ""
	st.entries = entries
	st.selection = nil
	st.diff = ""
	st.diffErr = ""
	if sel != nil {
		for _, entry := range entries {
			if entry.Path == sel.path && changeInGroup(entry, sel.group) {
				st.selectLocked(entry, sel.group)
				break
			}
		}
	}
}

// selectLocked loads the diff for one row. The caller holds changesMu.
func (st *changesState) selectLocked(e StatusEntry, group string) {
	sel := &changeSelection{group: group, path: e.Path}
	st.selection = sel
	st.diff = ""
	st.diffErr = ""
	switch group {
	case "untracked":
		var (
			p   *filePreview
			err error
		)
		if st.on != "" {
			p, err = readRemotePreview(st.app, st.on, remoteJoin(st.folder, e.Path))
		} else {
			p, err = readFilePreview(filepath.Join(st.folder, e.Path))
		}
		if err != nil {
			st.diffErr = err.Error()
			return
		}
		switch {
		case p.Image:
			st.diff = "Image file; no textual diff."
		case p.Binary:
			st.diff = "Binary file; no textual diff."
		case p.Text == "":
			st.diff = "Empty untracked file."
		default:
			st.diff = p.Text
			if p.Truncated {
				st.diff += "\n\n… preview truncated"
			}
		}
	default:
		var (
			diff string
			err  error
		)
		if st.on != "" {
			diff, err = GitDiffOn(st.app, st.on, st.folder, e.Path, group == "staged")
		} else {
			diff, err = GitDiff(st.folder, e.Path, group == "staged")
		}
		if err != nil {
			st.diffErr = GitMessage(err)
			return
		}
		st.diff = diff
	}
}

// renderChangesPanel draws the list and the selected diff.
func renderChangesPanel(rc *api.RenderContext) {
	if rc == nil || rc.C == nil {
		return
	}
	target := builtinPanelTargetFor(rc)
	if target.Folder == "" {
		renderBuiltinNotice(rc.C, "No folder for this space", "Open a space with a folder to see its changes.")
		return
	}
	if target.Remote && rc.App == nil {
		renderBuiltinNotice(rc.C, "Cannot reach "+target.Space.Machine, "No app is available to run commands there.")
		return
	}

	changesMu.Lock()
	defer changesMu.Unlock()
	st := changesStateOnLocked(target.Folder, target.Space.Machine, rc.App)
	c := rc.C
	t := c.Theme()
	ui.Column(c).Fill().Children(func() {
		changesHeader(c, st, t)
		if st.err != "" {
			renderBuiltinNotice(c, st.err, st.folder)
			return
		}
		ui.SplitPercent(c, &st.split, func() {
			changesListView(c, st, t)
		}, func() {
			changesDiffView(c, st, t)
		}).Fill()
	})
}

func changesHeader(c *ui.Context, st *changesState, t *ui.Theme) {
	ui.Row(c).
		Height(34).
		Padding(0, 10).
		Gap(8).
		AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).
		BorderColor(t.Border).
		Children(func() {
			ui.Text(c, "Changes").FontSize(t.FontSize - 1).Bold()
			ui.Text(c, st.label()).FontSize(t.FontSize - 2).TextColor(t.TextMuted).SingleLine().Grow(1).Tooltip(st.label())
			if ui.Button(c.Key("changes/refresh"), "Refresh").Clicked() {
				st.reloadLocked()
			}
		})
}

// changeInGroup reports whether an entry belongs to one of the panel's
// sections; a path may be both staged and unstaged.
func changeInGroup(e StatusEntry, group string) bool {
	switch group {
	case "staged":
		return e.Staged()
	case "unstaged":
		return e.Unstaged()
	case "untracked":
		return e.Untracked()
	case "conflicted":
		return e.Conflict()
	}
	return false
}

// changeGroups splits the status into the sections the panel shows.
func changeGroups(entries []StatusEntry) (staged, unstaged, untracked, conflicted []StatusEntry) {
	for _, e := range entries {
		switch {
		case e.Untracked():
			untracked = append(untracked, e)
		case e.Ignored():
			// git does not list ignored paths by default; skip anyway.
		case e.Conflict():
			conflicted = append(conflicted, e)
		default:
			if e.Staged() {
				staged = append(staged, e)
			}
			if e.Unstaged() {
				unstaged = append(unstaged, e)
			}
		}
	}
	sortEntries := func(s []StatusEntry) {
		sort.SliceStable(s, func(i, j int) bool { return s[i].Path < s[j].Path })
	}
	sortEntries(staged)
	sortEntries(unstaged)
	sortEntries(untracked)
	sortEntries(conflicted)
	return
}

// changeListRow is one row of the changes list: a group header or an
// entry.
type changeListRow struct {
	group string
	title string
	entry StatusEntry
}

// changeListRows flattens the status groups into the list.
func changeListRows(entries []StatusEntry) []changeListRow {
	staged, unstaged, untracked, conflicted := changeGroups(entries)
	groups := []struct {
		title string
		group string
		items []StatusEntry
	}{
		{"Staged", "staged", staged},
		{"Unstaged", "unstaged", unstaged},
		{"Untracked", "untracked", untracked},
		{"Conflicted", "conflicted", conflicted},
	}
	var out []changeListRow
	for _, group := range groups {
		if len(group.items) == 0 {
			continue
		}
		out = append(out, changeListRow{group: group.group, title: group.title})
		for _, entry := range group.items {
			out = append(out, changeListRow{group: group.group, entry: entry})
		}
	}
	return out
}

func changesListView(c *ui.Context, st *changesState, t *ui.Theme) {
	rows := changeListRows(st.entries)
	if len(rows) == 0 {
		renderBuiltinNotice(c, "No changes", "The working tree is clean.")
		return
	}
	selected := -1
	for i, row := range rows {
		if row.title != "" || st.selection == nil {
			continue
		}
		if row.group == st.selection.group && row.entry.Path == st.selection.path {
			selected = i
			break
		}
	}
	state := &st.list
	state.Key = func(i int) any {
		row := rows[i]
		if row.title != "" {
			return "header:" + row.group
		}
		return row.group + "\x00" + row.entry.Path
	}
	state.Header = func(i int) bool { return rows[i].title != "" }
	state.Label = func(i int) string {
		if rows[i].title != "" {
			return rows[i].title
		}
		return rows[i].entry.Path
	}
	state.Selected = &selected
	list := ui.List(c, state, len(rows), func(i int) {
		row := rows[i]
		if row.title != "" {
			ui.Textf(c, "%s (%d)", row.title, changeGroupCount(rows, row.group)).
				FontSize(t.FontSize-3).
				TextColor(t.TextMuted).
				Padding(4, 8).
				Background(t.Background)
			return
		}
		changesEntryRow(c, row.entry, row.group, t)
	}).Fill()
	list.Padding(4)
	if list.Changed() && selected >= 0 && selected < len(rows) && rows[selected].title == "" {
		row := rows[selected]
		st.selectLocked(row.entry, row.group)
	}
}

// changeGroupCount counts the entries of one group in the flattened rows.
func changeGroupCount(rows []changeListRow, group string) int {
	n := 0
	for _, row := range rows {
		if row.group == group && row.title == "" {
			n++
		}
	}
	return n
}

// changesEntryRow draws one path of the changes list.
func changesEntryRow(c *ui.Context, entry StatusEntry, group string, t *ui.Theme) {
	name := entry.Path
	if entry.OrigPath != "" {
		name = filepath.Base(entry.OrigPath) + " → " + name
	}
	ui.Row(c).
		Gap(6).
		Padding(2, 8).
		AlignItems(ui.Center).
		Children(func() {
			ui.Text(c, entry.Code()).FontSize(t.FontSize - 3).TextColor(t.TextMuted).Width(22).SingleLine()
			ui.Text(c, name).FontSize(t.FontSize - 1).SingleLine().Grow(1).Tooltip(entry.Path)
			if group == "untracked" {
				ui.Text(c, "new").FontSize(t.FontSize - 4).TextColor(t.Success)
			}
		})
}

func changesDiffView(c *ui.Context, st *changesState, t *ui.Theme) {
	if st.selection == nil {
		renderBuiltinNotice(c, "Select a changed file", "Its unified diff appears here.")
		return
	}
	sel := st.selection
	ui.Column(c).Fill().Children(func() {
		ui.Row(c).
			Height(30).
			Padding(0, 10).
			Gap(8).
			AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).
			BorderColor(t.Border).
			Children(func() {
				ui.Text(c, filepath.Base(sel.path)).FontSize(t.FontSize - 1).Bold().SingleLine().Grow(1).Tooltip(sel.path)
				ui.Text(c, sel.group).FontSize(t.FontSize - 3).TextColor(t.TextMuted)
			})
		if st.diffErr != "" {
			renderBuiltinNotice(c, "Cannot show the diff", st.diffErr)
			return
		}
		if st.diff == "" {
			renderBuiltinNotice(c, "No textual diff", "The file may be binary, empty, or not changed in this view.")
			return
		}
		text := st.diff
		if len(text) > changesMaxDisplayBytes {
			text = strings.ToValidUTF8(text[:changesMaxDisplayBytes], "") + "\n… diff truncated"
		}
		ui.ScrollBoth(c).Fill().Padding(8).Children(func() {
			ui.Text(c, text).FontSize(t.FontSize - 2).Selectable()
		})
	})
}
