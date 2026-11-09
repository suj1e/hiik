package builtin

// Changes lists a repository's staged, unstaged, untracked and conflicted
// paths and previews a unified diff. It works on this computer or on a
// space's machine through the core, and treats a folder outside a
// repository as an ordinary, empty state rather than an error dialog.
//
// The panel runs git only when the folder first appears and when Refresh is
// pressed; frames read the cached result under changesMu.

import (
	"path/filepath"
	"sort"
	"strings"
	"sync"

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
}

var (
	changesMu       sync.Mutex
	changesByFolder = map[string]*changesState{}
)

func resetChangesState() {
	changesMu.Lock()
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
	return st
}

// label names the machine and folder in the header.
func (st *changesState) label() string { return machineLabel(st.on, st.folder) }

// reloadLocked asks git status and drops a selection that no longer exists.
// The caller holds changesMu.
func (st *changesState) reloadLocked() {
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
			ui.Text(c, "Changes").FontSize(12).Bold()
			ui.Text(c, st.label()).FontSize(11).TextColor(t.TextMuted).SingleLine().Grow(1).Tooltip(st.label())
			if ui.Button(c.Key("changes/refresh"), "Refresh").Clicked() {
				st.reloadLocked()
			}
		})
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

func changesListView(c *ui.Context, st *changesState, t *ui.Theme) {
	staged, unstaged, untracked, conflicted := changeGroups(st.entries)
	if len(staged)+len(unstaged)+len(untracked)+len(conflicted) == 0 {
		renderBuiltinNotice(c, "No changes", "The working tree is clean.")
		return
	}
	ui.Scroll(c).Fill().Padding(4).Children(func() {
		ui.Column(c).Gap(6).Children(func() {
			changesGroup(c, st, t, "Staged", "staged", staged)
			changesGroup(c, st, t, "Unstaged", "unstaged", unstaged)
			changesGroup(c, st, t, "Untracked", "untracked", untracked)
			changesGroup(c, st, t, "Conflicted", "conflicted", conflicted)
		})
	})
}

func changesGroup(c *ui.Context, st *changesState, t *ui.Theme, title, group string, entries []StatusEntry) {
	if len(entries) == 0 {
		return
	}
	ui.Textf(c, "%s (%d)", title, len(entries)).FontSize(10).TextColor(t.TextMuted).Padding(4, 8)
	for _, e := range entries {
		entry := e
		key := "changes:" + group + ":" + entry.Path
		row := ui.Row(c.Key(key)).
			Height(24).
			Padding(0, 8).
			Gap(8).
			AlignItems(ui.Center).
			Radius(4)
		row.Children(func() {
			ui.Text(c, entry.Code()).FontSize(10).TextColor(t.TextMuted).Width(22).SingleLine()
			name := entry.Path
			if entry.OrigPath != "" {
				name = filepath.Base(entry.OrigPath) + " → " + name
			}
			ui.Text(c, name).FontSize(12).SingleLine().Grow(1).Tooltip(entry.Path)
		})
		if row.Clicked() {
			st.selectLocked(entry, group)
		}
		if st.selection != nil && st.selection.key() == key {
			row.Background(t.SurfaceHover)
		}
	}
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
				ui.Text(c, filepath.Base(sel.path)).FontSize(12).Bold().SingleLine().Grow(1).Tooltip(sel.path)
				ui.Text(c, sel.group).FontSize(10).TextColor(t.TextMuted)
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
			ui.Text(c, text).FontSize(11).Selectable()
		})
	})
}
