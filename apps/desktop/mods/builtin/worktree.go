package builtin

// Worktree lists and manages git worktrees for the active space's
// repository, on this computer or on the space's machine through the core.
//
// Creating uses `git worktree add -b <branch> <path>`. The destination is
// taken from the panel's path field; when that is empty the panel uses a
// unique path outside the repository (see defaultWorktreePath locally and
// DefaultWorktreePathOn on a machine) rather than writing into the
// repository. The shell names the repository with props {"space":
// *api.Space} or {"folder": "..."}; a host may prefill the form with
// {"branch": "...", "path": "..."}. Removing a worktree always asks for a
// second click and offers a separate Force remove.

import (
	"errors"
	"path/filepath"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"

	"hiik/mods/api"
)

func init() {
	api.Register(api.Mod{
		ID:   "worktree",
		Name: "Worktrees",
		Panels: []api.Panel{{
			ID:     api.PanelWorktree,
			Title:  "Worktrees",
			Render: renderWorktreePanel,
		}},
	})
}

// worktreeState is the per-folder cache and form state.
type worktreeState struct {
	folder     string
	on         string // "" is this computer
	app        api.App
	root       string
	err        string
	items      []Worktree
	branch     string
	path       string
	message    string
	messageErr bool
	confirm    string // the path the Remove confirmation names
}

var (
	worktreeMu        sync.Mutex
	worktreesByFolder = map[string]*worktreeState{}
)

func resetWorktreeState() {
	worktreeMu.Lock()
	worktreesByFolder = map[string]*worktreeState{}
	worktreeMu.Unlock()
}

// worktreeStateOnLocked returns the state for a folder on a machine. The
// caller holds worktreeMu.
func worktreeStateOnLocked(folder, on string, app api.App) *worktreeState {
	key := folder
	if on != "" {
		key = on + "\x00" + folder
	}
	if st := worktreesByFolder[key]; st != nil {
		if app != nil {
			st.app = app
		}
		return st
	}
	st := &worktreeState{folder: folder, on: on, app: app}
	st.reloadLocked()
	worktreesByFolder[key] = st
	return st
}

// label names the machine and folder in the header.
func (st *worktreeState) label() string { return machineLabel(st.on, st.folder) }

// reloadLocked asks git for the repository root and its worktrees. The
// caller holds worktreeMu.
func (st *worktreeState) reloadLocked() {
	st.confirm = ""
	var (
		root  string
		items []Worktree
		err   error
	)
	if st.on != "" {
		root, err = GitRepoRootOn(st.app, st.on, st.folder)
	} else {
		root, err = GitRepoRoot(st.folder)
	}
	if err != nil {
		st.root = ""
		st.err = GitMessage(err)
		st.items = nil
		return
	}
	if st.on != "" {
		items, err = GitWorktreesOn(st.app, st.on, st.folder)
	} else {
		items, err = GitWorktrees(st.folder)
	}
	if err != nil {
		st.root = root
		st.err = GitMessage(err)
		st.items = nil
		return
	}
	st.root = root
	st.err = ""
	st.items = items
}

func (st *worktreeState) setMessage(err error, text string) {
	if err != nil {
		st.message = GitMessage(err)
		st.messageErr = true
		return
	}
	st.message = text
	st.messageErr = false
}

// createLocked adds a worktree from the form. The caller holds worktreeMu.
func (st *worktreeState) createLocked() {
	branch := strings.TrimSpace(st.branch)
	if branch == "" {
		st.setMessage(errors.New("enter a branch name"), "")
		return
	}
	path := strings.TrimSpace(st.path)
	var err error
	if st.on != "" {
		if path == "" {
			path = DefaultWorktreePathOn(st.app, st.on, st.root, branch)
		}
		err = GitAddWorktreeOn(st.app, st.on, st.root, branch, path)
	} else {
		if path == "" {
			path = defaultWorktreePath(st.root, branch)
		}
		if abs, absErr := filepath.Abs(path); absErr == nil {
			path = abs
		}
		err = GitAddWorktree(st.root, branch, path)
	}
	if err != nil {
		st.setMessage(err, "")
		return
	}
	st.branch = ""
	st.path = ""
	st.setMessage(nil, "Created "+path)
	st.reloadLocked()
}

// renderWorktreePanel draws the list and the create form.
func renderWorktreePanel(rc *api.RenderContext) {
	if rc == nil || rc.C == nil {
		return
	}
	target := builtinPanelTargetFor(rc)
	if target.Folder == "" {
		renderBuiltinNotice(rc.C, "No folder for this space", "Open a space with a folder to manage worktrees.")
		return
	}
	if target.Remote && rc.App == nil {
		renderBuiltinNotice(rc.C, "Cannot reach "+target.Space.Machine, "No app is available to run commands there.")
		return
	}

	worktreeMu.Lock()
	defer worktreeMu.Unlock()
	st := worktreeStateOnLocked(target.Folder, target.Space.Machine, rc.App)
	if st.branch == "" {
		st.branch = builtinPropString(rc.Props, "branch")
	}
	if st.path == "" {
		st.path = builtinPropString(rc.Props, "path")
	}
	c := rc.C
	t := c.Theme()
	ui.Column(c).Fill().Children(func() {
		worktreeHeader(c, st, t)
		if st.err != "" {
			renderBuiltinNotice(c, st.err, st.folder)
			return
		}
		ui.Column(c).Fill().Children(func() {
			ui.Scroll(c).Grow(1).Children(func() {
				ui.Column(c).Gap(2).Padding(4).Children(func() {
					for _, wt := range st.items {
						worktreeRow(rc, c, st, t, wt)
					}
				})
			})
			worktreeForm(c, st, t)
		})
	})
}

func worktreeHeader(c *ui.Context, st *worktreeState, t *ui.Theme) {
	ui.Row(c).
		Height(34).
		Padding(0, 10).
		Gap(8).
		AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).
		BorderColor(t.Border).
		Children(func() {
			ui.Text(c, "Worktrees").FontSize(12).Bold()
			label := st.label()
			if st.root != "" {
				label = machineLabel(st.on, st.root)
			}
			ui.Text(c, label).FontSize(11).TextColor(t.TextMuted).SingleLine().Grow(1).Tooltip(label)
			if ui.Button(c.Key("worktree/refresh"), "Refresh").Clicked() {
				st.reloadLocked()
				st.setMessage(nil, "")
			}
		})
}

func worktreeRow(rc *api.RenderContext, c *ui.Context, st *worktreeState, t *ui.Theme, wt Worktree) {
	key := "worktree:" + wt.Path
	ui.Column(c.Key(key)).
		Padding(6, 10).
		Gap(4).
		BorderWidth(0, 0, 1, 0).
		BorderColor(t.Border).
		Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				name := filepath.Base(filepath.Clean(wt.Path))
				if st.on != "" {
					name = remoteBase(wt.Path)
				}
				if name == "." || name == string(filepath.Separator) || name == "" {
					name = wt.Path
				}
				ui.Text(c, name).FontSize(12).Bold().SingleLine()
				if wt.Branch != "" {
					ui.Text(c, wt.Branch).FontSize(10).TextColor(t.TextMuted).SingleLine()
				}
				if wt.Bare {
					ui.Text(c, "bare").FontSize(10).TextColor(t.TextMuted)
				}
				if wt.Detached {
					ui.Text(c, "detached").FontSize(10).TextColor(t.TextMuted)
				}
				if wt.Locked {
					ui.Text(c, "locked").FontSize(10).TextColor(t.Warning).Tooltip(wt.LockReason)
				}
				if wt.Prunable {
					ui.Text(c, "prunable").FontSize(10).TextColor(t.Warning).Tooltip(wt.PruneReason)
				}
				ui.Box(c).Grow(1)
				if ui.Button(c.Key(key+"/open"), "Open").Clicked() {
					worktreeOpen(rc, st, wt)
				}
				if ui.Button(c.Key(key+"/remove"), "Remove").Clicked() {
					st.confirm = wt.Path
					st.setMessage(nil, "")
				}
			})
			ui.Text(c, wt.Path).FontSize(10).TextColor(t.TextMuted).SingleLine().Tooltip(wt.Path)
			if st.confirm == wt.Path {
				ui.Text(c, "Remove this worktree? Its folder and any uncommitted changes are discarded.").
					FontSize(11).TextColor(t.Danger).Padding(2, 0)
				ui.Row(c).Gap(6).Children(func() {
					if ui.Button(c.Key(key+"/confirm"), "Remove").Clicked() {
						worktreeRemove(st, wt.Path, false)
					}
					if ui.Button(c.Key(key+"/force"), "Force remove").Clicked() {
						worktreeRemove(st, wt.Path, true)
					}
					if ui.Button(c.Key(key+"/cancel"), "Cancel").Clicked() {
						st.confirm = ""
					}
				})
			}
		})
}

func worktreeOpen(rc *api.RenderContext, st *worktreeState, wt Worktree) {
	if rc.App == nil {
		st.setMessage(errors.New("no app is available to open a space"), "")
		return
	}
	args := map[string]any{
		"folder": wt.Path,
		"name":   worktreeName(st.on, wt.Path),
	}
	if st.on != "" {
		args["machine"] = st.on
	}
	if err := rc.App.Run(api.CmdSpaceNew, args); err != nil {
		st.setMessage(err, "")
		return
	}
	st.setMessage(nil, "Opened "+wt.Path)
}

// worktreeName is the last path element under the machine's rules.
func worktreeName(on, p string) string {
	if on == "" {
		return filepath.Base(filepath.Clean(p))
	}
	return remoteBase(p)
}

func worktreeRemove(st *worktreeState, path string, force bool) {
	var err error
	if st.on != "" {
		err = GitRemoveWorktreeOn(st.app, st.on, st.root, path, force)
	} else {
		err = GitRemoveWorktree(st.root, path, force)
	}
	if err != nil {
		st.setMessage(err, "")
		return
	}
	st.confirm = ""
	st.setMessage(nil, "Removed "+path)
	st.reloadLocked()
}

// worktreePathPlaceholder describes where an empty path will go.
func worktreePathPlaceholder(st *worktreeState) string {
	if st.on != "" {
		return "path (default: ~/hiik-worktrees)"
	}
	return "path (default: temp)"
}

func worktreeForm(c *ui.Context, st *worktreeState, t *ui.Theme) {
	ui.Column(c).
		Padding(10).
		Gap(6).
		BorderWidth(1, 0, 0, 0).
		BorderColor(t.Border).
		Children(func() {
			ui.Text(c, "New worktree").FontSize(11).Bold()
			ui.Row(c).Gap(6).Children(func() {
				ui.TextInput(c.Key("worktree/branch"), &st.branch).
					Placeholder("branch").
					Label("Branch").
					Width(150).
					Shrink(0)
				ui.TextInput(c.Key("worktree/path"), &st.path).
					Placeholder(worktreePathPlaceholder(st)).
					Label("Worktree path").
					Grow(1)
				if ui.Button(c.Key("worktree/create"), "Create").Clicked() {
					st.createLocked()
				}
			})
			hint := "Creates a worktree on a new branch."
			if st.root != "" && strings.TrimSpace(st.path) == "" {
				branch := strings.TrimSpace(st.branch)
				if branch == "" {
					branch = "worktree"
				}
				if st.on != "" {
					hint = "Default path: ~/hiik-worktrees/" + worktreeDirName(st.root, branch) + " on " + st.on
				} else {
					hint = "Default path: " + defaultWorktreePath(st.root, branch)
				}
			}
			ui.Text(c, hint).FontSize(10).TextColor(t.TextMuted).SingleLine().Tooltip(hint)
			if st.message != "" {
				color := t.TextMuted
				if st.messageErr {
					color = t.Danger
				}
				ui.Text(c, st.message).FontSize(10).TextColor(color).SingleLine().Tooltip(st.message)
			}
		})
}
