package builtin

// Files is a small, self-contained file tree and preview. It reads this
// computer, or a space's machine through the core (machine.exec for one
// listing at a time, file.read for a text preview).
//
// Expansion and preview state live in this package, keyed by machine and
// folder, and are guarded by filesMu. Nothing runs on a timer and no
// goroutine is started per frame: a folder is read once when it first
// appears, a subfolder when it is expanded, and on an explicit Refresh.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"hiik/mods/api"
)

const (
	// filesMaxPreviewBytes is how much text a preview reads.
	filesMaxPreviewBytes = 1 << 20
	// filesMaxDisplayBytes keeps a huge preview from making text layout
	// expensive; the notice says the rest is there.
	filesMaxDisplayBytes = 256 << 10
	// filesMaxEntries caps one directory. A tree of 10k rows would build
	// 10k widgets each frame; deeper folders stay one click away.
	filesMaxEntries = 2000
)

func init() {
	api.Register(api.Mod{
		ID:   "files",
		Name: "Files",
		Panels: []api.Panel{{
			ID:     api.PanelFiles,
			Title:  "Files",
			Render: renderFilesPanel,
		}},
	})
}

// builtinPanelTarget is the folder a workspace panel shows.
type builtinPanelTarget struct {
	Folder   string
	Space    api.Space
	HasSpace bool
	Remote   bool
}

// builtinPanelTargetFor resolves a prop folder first, then the active (or named)
// space. Props may carry "folder" or "space"; a remote space is reported so
// a panel can run its work on that machine.
func builtinPanelTargetFor(rc *api.RenderContext) builtinPanelTarget {
	var target builtinPanelTarget
	if rc == nil {
		return target
	}
	// The shell passes {"space": *api.Space, "folder": string} to the files
	// panel and {"space": *api.Space} to the others. Keep the string form
	// for a host that names a space by ID, and fall back to the active space.
	if sp, ok := builtinPropSpace(rc.Props); ok {
		target.Space, target.HasSpace = sp, true
		target.Folder = sp.Folder
		if sp.Machine != "" {
			target.Remote = true
		}
	}
	// The shell's live terminal folder wins over the space's launch folder;
	// the remote flag still makes the commands run on the right machine.
	if folder := builtinPropString(rc.Props, "folder"); folder != "" {
		target.Folder = folder
		return target
	}
	if target.HasSpace {
		target.Folder = target.Space.Folder
		return target
	}
	var spaces []api.Space
	var active string
	if rc.App != nil {
		spaces = rc.App.Spaces()
		active = rc.App.ActiveSpace()
	}
	want := builtinPropString(rc.Props, "space")
	if want == "" {
		want = active
	}
	for _, sp := range spaces {
		if sp.ID == want {
			target.Space, target.HasSpace = sp, true
			break
		}
	}
	if !target.HasSpace && want == "" && len(spaces) == 1 {
		target.Space, target.HasSpace = spaces[0], true
	}
	if target.HasSpace {
		target.Folder = target.Space.Folder
		if target.Space.Machine != "" {
			target.Remote = true
			return target
		}
	}
	return target
}

// builtinPropSpace reads the shell's *api.Space prop, also accepting a value
// for a host that cannot take a pointer.
func builtinPropSpace(props map[string]any) (api.Space, bool) {
	if props == nil {
		return api.Space{}, false
	}
	switch v := props["space"].(type) {
	case *api.Space:
		if v != nil {
			return *v, true
		}
	case api.Space:
		return v, true
	}
	return api.Space{}, false
}

func builtinPropString(props map[string]any, key string) string {
	if props == nil {
		return ""
	}
	v, ok := props[key]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// renderBuiltinNotice draws the one-line reason a panel has nothing to show.
func renderBuiltinNotice(c *ui.Context, title, detail string) {
	t := c.Theme()
	ui.Column(c).Fill().Center().Gap(5).Padding(16).Children(func() {
		ui.Text(c, title).FontSize(t.FontSize).TextColor(t.TextMuted).TextAlign(ui.Center)
		if detail != "" {
			ui.Text(c, detail).FontSize(t.FontSize - 2).TextColor(t.TextMuted).TextAlign(ui.Center)
		}
	})
}

// fileTreeNode is one directory or file. open and loaded are the panel's
// expansion state; children is filled the first time the node opens.
type fileTreeNode struct {
	name     string
	path     string
	dir      bool
	symlink  bool
	open     bool
	loaded   bool
	err      string
	note     string
	children []*fileTreeNode
}

// filePreview is the text or image placeholder for the selected file.
type filePreview struct {
	Path      string
	Text      string
	Size      int64
	SizeKnown bool
	Truncated bool
	Image     bool
	Binary    bool
}

// filesTree is the per-folder state.
type filesTree struct {
	folder     string
	on         string  // "" is this computer
	app        api.App // the app used for remote commands
	root       *fileTreeNode
	split      float32 // percent of the first pane
	selected   string
	preview    *filePreview
	previewErr string
	err        string
}

var (
	filesMu       sync.Mutex
	filesByFolder = map[string]*filesTree{}
	filesWatches  = map[string]context.CancelFunc{}
)

// resetFilesState clears the cached trees. Tests use it; a refresh in the
// UI keeps the map so expansion elsewhere is unaffected.
func resetFilesState() {
	filesMu.Lock()
	for _, cancel := range filesWatches {
		cancel()
	}
	filesWatches = map[string]context.CancelFunc{}
	filesByFolder = map[string]*filesTree{}
	filesMu.Unlock()
}

// filesTreeLocked returns the local state for a folder. The caller holds
// filesMu.
func filesTreeLocked(folder string) *filesTree {
	return filesTreeOnLocked(folder, "", nil)
}

// filesTreeOnLocked returns the state for a folder on a machine. The
// caller holds filesMu.
func filesTreeOnLocked(folder, on string, app api.App) *filesTree {
	key := folder
	if on != "" {
		key = on + "\x00" + folder
	}
	if st := filesByFolder[key]; st != nil {
		if app != nil {
			st.app = app
		}
		return st
	}
	st := &filesTree{folder: folder, on: on, app: app, split: 42}
	st.reloadLocked()
	filesByFolder[key] = st
	filesWatchLocked(st, key)
	return st
}

// filesWatchLocked starts mygo's file watch for a local folder once. Remote
// folders are not watched: the core does not expose a remote watch, and a
// timer would run a command per frame in the panel. The caller holds
// filesMu.
func filesWatchLocked(st *filesTree, key string) {
	if st.on != "" || !mygo.App.IsReady() {
		return
	}
	if _, ok := filesWatches[key]; ok {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	filesWatches[key] = cancel
	go func() {
		w, err := mygo.WatchFiles(ctx, st.folder, mygo.FileWatchOptions{
			Recursive: true,
			HintsOnly: true,
			Ignore: func(p string, isDir bool) bool {
				if !isDir {
					return false
				}
				switch path.Base(p) {
				case ".git", "node_modules", "vendor":
					return true
				}
				return false
			},
		})
		if err != nil {
			return
		}
		defer w.Close()
		for {
			if _, err := w.Next(ctx); err != nil {
				return
			}
			filesMu.Lock()
			if cur := filesByFolder[key]; cur != nil {
				cur.refreshLocked()
			}
			filesMu.Unlock()
			if inv, ok := st.app.(interface{ Invalidate() }); ok {
				inv.Invalidate()
			}
		}
	}()
}

// refreshLocked reloads the open folders and keeps expansion and selection
// where their paths still exist. The caller holds filesMu.
func (st *filesTree) refreshLocked() {
	if st.root == nil {
		return
	}
	var walk func(n *fileTreeNode)
	walk = func(n *fileTreeNode) {
		if n == nil || !n.dir {
			return
		}
		n.loaded = false
		n.err = ""
		if n != st.root && !n.open {
			return
		}
		st.ensureLoadedLocked(n)
		for _, child := range n.children {
			walk(child)
		}
	}
	walk(st.root)
	if st.selected != "" && !filesHasPath(st.root, st.selected) {
		st.selected = ""
		st.preview = nil
		st.previewErr = ""
	}
}

// filesHasPath reports whether the tree still shows path.
func filesHasPath(n *fileTreeNode, target string) bool {
	if n == nil {
		return false
	}
	if n.path == target {
		return true
	}
	for _, child := range n.children {
		if filesHasPath(child, target) {
			return true
		}
	}
	return false
}

// label names the machine and folder in a header.
func (st *filesTree) label() string { return machineLabel(st.on, st.folder) }

// machineLabel shows a remote folder with its machine.
func machineLabel(on, folder string) string {
	if on == "" {
		return folder
	}
	return on + ":" + folder
}

// reloadLocked reads the root and forgets expansion and selection. The
// caller holds filesMu.
func (st *filesTree) reloadLocked() {
	name := ""
	if st.on == "" {
		name = filepath.Base(filepath.Clean(st.folder))
		if name == "." || name == string(filepath.Separator) {
			name = ""
		}
	} else {
		name = remoteBase(st.folder)
	}
	if name == "" {
		name = st.folder
	}
	st.root = &fileTreeNode{
		name: name,
		path: st.folder,
		dir:  true,
		open: true,
	}
	st.selected = ""
	st.preview = nil
	st.previewErr = ""
	st.err = ""
	st.ensureLoadedLocked(st.root)
	if st.root.err != "" {
		st.err = st.root.err
	}
}

// ensureLoadedLocked reads one directory. The caller holds filesMu.
func (st *filesTree) ensureLoadedLocked(n *fileTreeNode) {
	if !n.dir || n.loaded {
		return
	}
	n.loaded = true
	if st.on != "" {
		nodes, note, err := readRemoteDir(st.app, st.on, n.path)
		if err != nil {
			n.err = err.Error()
			n.children = nil
			return
		}
		if len(nodes) > filesMaxEntries {
			note = fmt.Sprintf("showing %d of %d entries", filesMaxEntries, len(nodes))
			nodes = nodes[:filesMaxEntries]
		}
		n.note = note
		n.children = nodes
		sortFileNodes(n.children)
		return
	}
	ents, err := os.ReadDir(n.path)
	if err != nil {
		n.err = err.Error()
		n.children = nil
		return
	}
	if len(ents) > filesMaxEntries {
		n.note = fmt.Sprintf("showing %d of %d entries", filesMaxEntries, len(ents))
		ents = ents[:filesMaxEntries]
	}
	n.children = make([]*fileTreeNode, 0, len(ents))
	for _, ent := range ents {
		n.children = append(n.children, &fileTreeNode{
			name:    ent.Name(),
			path:    filepath.Join(n.path, ent.Name()),
			dir:     ent.IsDir(),
			symlink: ent.Type()&os.ModeSymlink != 0,
		})
	}
	sortFileNodes(n.children)
}

// sortFileNodes puts folders first, then names in a stable case-insensitive
// order.
func sortFileNodes(nodes []*fileTreeNode) {
	sort.SliceStable(nodes, func(i, j int) bool {
		a, b := nodes[i], nodes[j]
		if a.dir != b.dir {
			return a.dir
		}
		return strings.ToLower(a.name) < strings.ToLower(b.name)
	})
}

// selectLocked previews one file. The caller holds filesMu.
func (st *filesTree) selectLocked(path string) {
	st.selected = path
	st.preview = nil
	st.previewErr = ""
	var (
		p   *filePreview
		err error
	)
	if st.on != "" {
		p, err = readRemotePreview(st.app, st.on, path)
	} else {
		p, err = readFilePreview(path)
	}
	if err != nil {
		st.previewErr = err.Error()
		return
	}
	st.preview = p
}

// readFilePreview reads a text file up to the preview cap or describes an
// image without decoding it in this panel.
func readFilePreview(path string) (*filePreview, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%s is a folder", filepath.Base(path))
	}
	if isImagePath(path) {
		return &filePreview{Path: path, Size: info.Size(), SizeKnown: true, Image: true}, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, filesMaxPreviewBytes+1))
	if err != nil {
		return nil, err
	}
	p := &filePreview{Path: path, Size: info.Size(), SizeKnown: true}
	if len(data) > filesMaxPreviewBytes {
		data = data[:filesMaxPreviewBytes]
		p.Truncated = true
	}
	if bytes.IndexByte(data, 0) >= 0 {
		p.Binary = true
		return p, nil
	}
	p.Text = strings.ToValidUTF8(string(data), "\uFFFD")
	return p, nil
}

// readRemoteDir lists one directory on a machine. ls -1Ap is tried first;
// find is a fallback for a system whose ls rejects --. Every path travels
// as one argv element, so spaces and shell signs need no quoting.
func readRemoteDir(app api.App, on, folder string) ([]*fileTreeNode, string, error) {
	if app == nil {
		return nil, "", fmt.Errorf("no app is available for %s", on)
	}
	stdout, stderr, code, err := app.Exec(on, "ls", "-1Ap", "--", folder)
	if err != nil {
		return nil, "", err
	}
	if code == 0 {
		return parseRemoteLS(folder, stdout), "", nil
	}
	lsErr := fmt.Sprintf("ls -1Ap -- %s: %s", folder, execMessage(stderr, code))

	out, _, findCode, findErr := app.Exec(on, "find", folder, "-mindepth", "1", "-maxdepth", "1", "-print")
	if findErr != nil || findCode != 0 {
		return nil, "", errors.New(lsErr)
	}
	dirsOut, _, dirCode, dirErr := app.Exec(on, "find", folder, "-mindepth", "1", "-maxdepth", "1", "-type", "d", "-print")
	dirs := map[string]bool{}
	if dirErr == nil && dirCode == 0 {
		for _, line := range strings.Split(dirsOut, "\n") {
			line = strings.TrimSuffix(line, "\r")
			if line != "" {
				dirs[line] = true
			}
		}
	}
	return parseRemoteFind(folder, out, dirs), "", nil
}

// parseRemoteLS reads one name per line; ls -p marks a folder with a slash.
func parseRemoteLS(folder, out string) []*fileTreeNode {
	var nodes []*fileTreeNode
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if line == "" || line == "." || line == ".." {
			continue
		}
		dir := strings.HasSuffix(line, "/")
		name := strings.TrimSuffix(line, "/")
		if name == "" {
			continue
		}
		nodes = append(nodes, &fileTreeNode{
			name: name,
			path: remoteJoin(folder, name),
			dir:  dir,
		})
	}
	return nodes
}

// parseRemoteFind turns full paths from find into nodes, using the set of
// folders from the second find.
func parseRemoteFind(folder, out string, dirs map[string]bool) []*fileTreeNode {
	var nodes []*fileTreeNode
	for _, line := range strings.Split(out, "\n") {
		p := strings.TrimSuffix(line, "\r")
		if p == "" || p == folder {
			continue
		}
		name := remoteBase(p)
		if name == "" || name == "." || name == "/" {
			continue
		}
		nodes = append(nodes, &fileTreeNode{name: name, path: p, dir: dirs[p]})
	}
	return nodes
}

// execMessage is the line a failed remote command reports.
func execMessage(stderr string, code int) string {
	if s := strings.TrimSpace(stderr); s != "" {
		return s
	}
	return fmt.Sprintf("exit %d", code)
}

// readRemotePreview reads a text file on a machine. Images get a
// placeholder without reading; a binary file is reported by the core or by
// a NUL byte.
func readRemotePreview(app api.App, on, file string) (*filePreview, error) {
	if app == nil {
		return nil, fmt.Errorf("no app is available for %s", on)
	}
	if isImagePath(file) {
		return &filePreview{Path: file, Image: true}, nil
	}
	text, err := app.ReadTextFile(file, on)
	if err != nil {
		if remoteNotText(err) {
			return &filePreview{Path: file, Binary: true}, nil
		}
		return nil, err
	}
	data := []byte(text)
	p := &filePreview{Path: file}
	if len(data) > filesMaxPreviewBytes {
		data = data[:filesMaxPreviewBytes]
		p.Truncated = true
	} else if len(text) >= filesMaxPreviewBytes {
		// ReadTextFile caps at this size, so the file may be longer.
		p.Truncated = true
	}
	if bytes.IndexByte(data, 0) >= 0 {
		p.Binary = true
		return p, nil
	}
	p.Text = strings.ToValidUTF8(string(data), "\uFFFD")
	return p, nil
}

// remoteNotText recognizes the core's not_text error or a binary notice.
func remoteNotText(err error) bool {
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "not_text") ||
		strings.Contains(s, "not text") ||
		strings.Contains(s, "binary")
}

// remoteJoin joins a machine path without this computer's separator rules.
func remoteJoin(dir, name string) string {
	if dir == "" {
		return name
	}
	return path.Join(dir, name)
}

// remoteBase returns the last element of a machine path.
func remoteBase(p string) string {
	p = strings.TrimRight(p, "/")
	if p == "" {
		return "/"
	}
	b := path.Base(p)
	if b == "." || b == "/" {
		return p
	}
	return b
}

func isImagePath(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".ico", ".tif", ".tiff", ".svg", ".avif":
		return true
	}
	return false
}

// renderFilesPanel draws the tree and the selected file.
func renderFilesPanel(rc *api.RenderContext) {
	if rc == nil || rc.C == nil {
		return
	}
	target := builtinPanelTargetFor(rc)
	if target.Folder == "" {
		renderBuiltinNotice(rc.C, "No folder for this space", "Open a space with a folder to browse files.")
		return
	}
	if target.Remote && rc.App == nil {
		renderBuiltinNotice(rc.C, "Cannot reach "+target.Space.Machine, "No app is available to run commands there.")
		return
	}

	filesMu.Lock()
	defer filesMu.Unlock()
	st := filesTreeOnLocked(target.Folder, target.Space.Machine, rc.App)
	c := rc.C
	t := c.Theme()
	ui.Column(c).Fill().Children(func() {
		filesHeader(c, st, t)
		ui.SplitPercent(c, &st.split, func() {
			filesTreeView(c, st, t)
		}, func() {
			filesPreviewView(c, st, t)
		}).Fill()
	})
}

func filesHeader(c *ui.Context, st *filesTree, t *ui.Theme) {
	ui.Row(c).
		Height(34).
		Padding(0, 10).
		Gap(8).
		AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).
		BorderColor(t.Border).
		Children(func() {
			ui.Text(c, "Files").FontSize(t.FontSize - 1).Bold()
			ui.Text(c, st.label()).FontSize(t.FontSize - 2).TextColor(t.TextMuted).SingleLine().Grow(1).Tooltip(st.label())
			if ui.Button(c.Key("files/refresh"), "Refresh").Clicked() {
				st.reloadLocked()
			}
		})
}

func filesTreeView(c *ui.Context, st *filesTree, t *ui.Theme) {
	ui.Column(c).Fill().Children(func() {
		if st.err != "" {
			ui.Text(c, st.err).FontSize(t.FontSize - 2).TextColor(t.Danger).Padding(10)
			return
		}
		if st.root != nil && st.root.note != "" {
			ui.Text(c, st.root.note).FontSize(t.FontSize-3).TextColor(t.TextMuted).Padding(8, 10)
		}
		if st.root == nil || len(st.root.children) == 0 {
			ui.Text(c, "Empty folder").FontSize(t.FontSize - 2).TextColor(t.TextMuted).Padding(10)
			return
		}
		ui.Scroll(c).Fill().Padding(4).Children(func() {
			ui.Tree(c, func() {
				for _, n := range st.root.children {
					filesTreeRow(c, st, t, n)
				}
			})
		})
	})
}

func filesTreeRow(c *ui.Context, st *filesTree, t *ui.Theme, n *fileTreeNode) {
	key := "files:" + n.path
	if n.dir {
		item := ui.TreeItem(c.Key(key), n.name, &n.open, func() {
			if !n.open {
				return
			}
			st.ensureLoadedLocked(n)
			if n.err != "" {
				ui.Text(c, n.err).FontSize(t.FontSize - 3).TextColor(t.Danger).PaddingStart(24)
				return
			}
			for _, child := range n.children {
				filesTreeRow(c, st, t, child)
			}
			if n.note != "" {
				ui.Text(c, n.note).FontSize(t.FontSize - 3).TextColor(t.TextMuted).PaddingStart(24)
			}
		})
		if n.err != "" {
			item.Tooltip(n.err)
		}
		if item.Clicked() {
			n.open = !n.open
		}
		return
	}

	item := ui.TreeItem(c.Key(key), n.name, nil, nil)
	if st.selected == n.path {
		item.Background(t.SurfaceHover)
	}
	if n.symlink {
		item.Tooltip("Symbolic link")
	}
	if item.Clicked() {
		st.selectLocked(n.path)
	}
}

func filesPreviewView(c *ui.Context, st *filesTree, t *ui.Theme) {
	if st.previewErr != "" {
		renderBuiltinNotice(c, "Cannot preview this file", st.previewErr)
		return
	}
	if st.preview == nil {
		renderBuiltinNotice(c, "Select a file to preview", "")
		return
	}
	p := st.preview
	ui.Column(c).Fill().Children(func() {
		ui.Row(c).
			Height(30).
			Padding(0, 10).
			Gap(8).
			AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).
			BorderColor(t.Border).
			Children(func() {
				name := filepath.Base(p.Path)
				if st.on != "" {
					name = remoteBase(p.Path)
				}
				ui.Text(c, name).FontSize(t.FontSize - 1).Bold().SingleLine().Grow(1).Tooltip(p.Path)
				if p.SizeKnown {
					ui.Text(c, formatFileSize(p.Size)).FontSize(t.FontSize - 3).TextColor(t.TextMuted)
				}
			})
		switch {
		case p.Image:
			filesImagePlaceholder(c, t, p)
		case p.Binary:
			renderBuiltinNotice(c, "Binary file", formatFileSize(p.Size)+"; no text preview")
		default:
			filesTextPreview(c, t, p)
		}
	})
}

func filesImagePlaceholder(c *ui.Context, t *ui.Theme, p *filePreview) {
	ui.Column(c).Fill().Center().Gap(6).Padding(16).Children(func() {
		ui.Text(c, "Image").FontSize(t.FontSize + 2).TextColor(t.TextMuted)
		ui.Text(c, filepath.Base(p.Path)).FontSize(t.FontSize - 2).TextColor(t.TextMuted).TextAlign(ui.Center)
		if p.SizeKnown {
			ui.Text(c, formatFileSize(p.Size)).FontSize(t.FontSize - 3).TextColor(t.TextMuted)
		}
	})
}

func filesTextPreview(c *ui.Context, t *ui.Theme, p *filePreview) {
	text := p.Text
	if text == "" {
		text = "(empty file)"
	}
	cut := false
	if len(text) > filesMaxDisplayBytes {
		text = strings.ToValidUTF8(text[:filesMaxDisplayBytes], "")
		cut = true
	}
	if cut || p.Truncated {
		text += "\n\n… preview truncated"
	}
	ui.Scroll(c).Fill().Padding(10).Children(func() {
		ui.Text(c, text).FontSize(t.FontSize - 1).Selectable()
	})
}

func formatFileSize(n int64) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	case n < 1024*1024*1024:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	default:
		return fmt.Sprintf("%.1f GB", float64(n)/(1024*1024*1024))
	}
}
