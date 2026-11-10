package builtin

// Layout helpers for the spaces panel: display names, machine groups, git
// tails, and worktree nesting.

import (
	"fmt"
	"path"
	"runtime"
	"strings"

	"github.com/egoist/mygo/ui"

	"hiik/icons"
	"hiik/mods/api"
)

// spaceDisplayName is the one-line name of a space.
func spaceDisplayName(sp api.Space) string {
	if sp.Name != "" {
		return sp.Name
	}
	if name := spaceFolderName(sp.Folder); name != "" {
		return name
	}
	return sp.ID
}

// spaceFolderName is the last component of a folder path, on either kind
// of separator, or "" for a root or empty path.
func spaceFolderName(folder string) string {
	folder = strings.TrimRight(folder, "/\\")
	if folder == "" {
		return ""
	}
	if i := strings.LastIndexAny(folder, "/\\"); i >= 0 {
		return folder[i+1:]
	}
	return folder
}

// spaceRow is one line of the spaces list: a space, its place in the app's
// list, its git tail, and its tree prefix when it nests under a checkout.
type spaceRow struct {
	Space api.Space
	Index int
	Tree  string
	Git   spaceGitInfo
}

// spaceRows pairs spaces with their git state, then nests worktrees under
// their main checkout.
func spaceRows(spaces []api.Space, info func(api.Space) spaceGitInfo) []spaceRow {
	rows := make([]spaceRow, 0, len(spaces))
	for i, sp := range spaces {
		row := spaceRow{Space: sp, Index: i}
		if info != nil {
			row.Git = info(sp)
		}
		rows = append(rows, row)
	}
	return spaceRowsNest(rows)
}

// spaceRowIDs returns the IDs of rows in list order.
func spaceRowIDs(rows []spaceRow) []string {
	out := make([]string, len(rows))
	for i, row := range rows {
		out[i] = row.Space.ID
	}
	return out
}

// spaceRowsNest moves linked worktrees after their main checkout's space
// and marks them with a tree prefix. A worktree whose checkout has no
// space, or is on another machine, stays where it is.
func spaceRowsNest(rows []spaceRow) []spaceRow {
	mains := make(map[string]int)
	for i, row := range rows {
		g := row.Git
		if !g.OK || g.Repo == "" || g.Linked {
			continue
		}
		if _, ok := mains[g.Repo]; !ok {
			mains[g.Repo] = i
		}
	}
	out := make([]spaceRow, 0, len(rows))
	placed := make(map[string]bool)
	for i, row := range rows {
		g := row.Git
		if g.OK && g.Linked && g.Repo != "" {
			if m, ok := mains[g.Repo]; ok && m != i && sameSpaceMachine(rows[m].Space, row.Space) {
				continue
			}
		}
		out = append(out, row)
		if !g.OK || g.Linked || g.Repo == "" || placed[g.Repo] {
			continue
		}
		placed[g.Repo] = true
		var kids []spaceRow
		for j, kid := range rows {
			if j == i {
				continue
			}
			kg := kid.Git
			if kg.OK && kg.Linked && kg.Repo == g.Repo && sameSpaceMachine(kid.Space, row.Space) {
				kids = append(kids, kid)
			}
		}
		for n := range kids {
			if n == len(kids)-1 {
				kids[n].Tree = "└─"
			} else {
				kids[n].Tree = "├─"
			}
			out = append(out, kids[n])
		}
	}
	return out
}

// sameSpaceMachine reports whether two spaces belong to the same machine.
func sameSpaceMachine(a, b api.Space) bool {
	return a.Machine == b.Machine
}

// spaceMachineGroup is the spaces under one machine, ready to list under
// its header.
type spaceMachineGroup struct {
	Machine string
	Label   string
	State   string
	// OS and Arch describe the machine; the local group knows its own.
	OS     string
	Arch   string
	Spaces []spaceRow
}

// spaceMachineGroups groups rows by machine. It reports whether headers
// are needed, which is when the app knows any remote machine or a space
// names one. The local group comes first; remote groups follow the app's
// machine order, then machines only spaces know.
func spaceMachineGroups(rows []spaceRow, machines []api.Machine) ([]spaceMachineGroup, bool) {
	byName := make(map[string]api.Machine, len(machines))
	order := []string{""}
	for _, m := range machines {
		if m.Name == "" {
			continue
		}
		if _, ok := byName[m.Name]; ok {
			continue
		}
		byName[m.Name] = m
		order = append(order, m.Name)
	}
	known := make(map[string]bool, len(order))
	for _, name := range order {
		known[name] = true
	}
	for _, row := range rows {
		name := row.Space.Machine
		if name == "" || known[name] {
			continue
		}
		known[name] = true
		order = append(order, name)
	}

	grouped := len(machines) > 0
	groups := make([]spaceMachineGroup, 0, len(order))
	index := make(map[string]int, len(order))
	for _, name := range order {
		g := spaceMachineGroup{
			Machine: name,
			Label:   "This computer",
			State:   "ready",
			OS:      localOSName(),
			Arch:    runtime.GOARCH,
		}
		if name != "" {
			grouped = true
			m := byName[name]
			g.Label = m.Name
			if g.Label == "" {
				g.Label = m.Host
			}
			if g.Label == "" {
				g.Label = name
			}
			g.State = m.State
			g.OS = m.OS
			g.Arch = m.Arch
		}
		index[name] = len(groups)
		groups = append(groups, g)
	}
	for _, row := range rows {
		if i, ok := index[row.Space.Machine]; ok {
			groups[i].Spaces = append(groups[i].Spaces, row)
		}
	}
	if !grouped {
		return nil, false
	}
	return groups, true
}

// spaceMachineIcon is a monitor for this computer, a server for another.
func spaceMachineIcon(machine string) *ui.SVG {
	if machine == "" {
		return icons.Monitor
	}
	return icons.Server
}

// localOSName is this app's OS in the names the machine rows use.
func localOSName() string {
	switch runtime.GOOS {
	case "darwin":
		return "macOS"
	case "windows":
		return "Windows"
	case "linux":
		return "Linux"
	default:
		return runtime.GOOS
	}
}

// spaceGitTailParts is what the row shows after a space's name: its repo
// and branch, then ahead and behind. Master and main go, as does a value
// that already is the space's name. A worktree row is named after its
// branch, so it drops the repo too.
func spaceGitTailParts(info spaceGitInfo, name string, tree bool) []string {
	if !info.OK {
		return nil
	}
	branch := info.Branch
	if branch == "" {
		branch = "detached"
	}
	if spaceSameText(branch, name) || branch == "master" || branch == "main" {
		branch = ""
	}
	repo := info.Repo
	if tree || spaceSameText(repo, name) {
		repo = ""
	}
	var parts []string
	switch {
	case repo != "" && branch != "":
		parts = append(parts, repo+" · "+branch)
	case repo != "":
		parts = append(parts, repo)
	case branch != "":
		parts = append(parts, branch)
	}
	if info.Ahead > 0 {
		parts = append(parts, fmt.Sprintf("↑%d", info.Ahead))
	}
	if info.Behind > 0 {
		parts = append(parts, fmt.Sprintf("↓%d", info.Behind))
	}
	return parts
}

// spaceSameText reports whether two labels say the same thing, ignoring
// case as a checkout name does.
func spaceSameText(a, b string) bool {
	return a != "" && b != "" && strings.EqualFold(a, b)
}

// spaceRepoName is the repository's name from a checkout's root path.
func spaceRepoName(root string) string {
	root = normalizeSpacePath(root)
	if root == "" || root == "/" {
		return ""
	}
	return strings.TrimSuffix(path.Base(root), ".git")
}

// normalizeSpacePath drops trailing slashes so two spellings of one path
// compare equal.
func normalizeSpacePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	p = strings.TrimRight(p, "/")
	if p == "" {
		return "/"
	}
	return p
}
