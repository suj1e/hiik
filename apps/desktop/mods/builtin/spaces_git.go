package builtin

// Best-effort git state for the spaces list: status, branch and
// ahead/behind, plus the checkout's worktrees so linked ones can nest under
// their main space. Answers are cached per machine and folder, so a panel
// asks once for each place it shows.

import (
	"strconv"
	"strings"

	"hiik/mods/api"
)

// spaceGitInfo is one folder's git state, or OK false when it has none.
type spaceGitInfo struct {
	OK     bool
	Root   string
	Repo   string
	Branch string
	Ahead  int
	Behind int
	Linked bool
}

// spaceGitCache remembers git answers by machine and folder.
type spaceGitCache struct {
	infos map[string]spaceGitInfo
}

// newSpaceGitCache returns an empty cache.
func newSpaceGitCache() spaceGitCache {
	return spaceGitCache{infos: make(map[string]spaceGitInfo)}
}

// spaceGitKey names one place in the cache.
func spaceGitKey(on, folder string) string {
	return on + "\x00" + folder
}

// info returns a folder's git state, reading it the first time only.
func (c *spaceGitCache) info(app api.App, on, folder string) spaceGitInfo {
	if c == nil || app == nil || folder == "" {
		return spaceGitInfo{}
	}
	key := spaceGitKey(on, folder)
	if info, ok := c.infos[key]; ok {
		return info
	}
	if c.infos == nil {
		c.infos = make(map[string]spaceGitInfo)
	}
	info := readSpaceGit(app, on, folder)
	c.infos[key] = info
	return info
}

// readSpaceGit asks git once about a folder. Any failure leaves the panel
// with nothing to draw for it, never an error row.
func readSpaceGit(app api.App, on, folder string) spaceGitInfo {
	if app == nil || folder == "" {
		return spaceGitInfo{}
	}
	stdout, _, code, err := app.Exec(on, "git", "-C", folder,
		"--no-optional-locks", "status", "--porcelain=v2", "--branch")
	if err != nil || code != 0 {
		return spaceGitInfo{}
	}
	info := spaceGitInfo{OK: true}
	info.Branch, info.Ahead, info.Behind = parseSpaceGitStatus(stdout)

	if out, _, code, err := app.Exec(on, "git", "-C", folder, "rev-parse", "--show-toplevel"); err == nil && code == 0 {
		info.Root = normalizeSpacePath(out)
	}
	if out, _, code, err := app.Exec(on, "git", "-C", folder, "worktree", "list", "--porcelain"); err == nil && code == 0 {
		paths := parseSpaceWorktrees(out)
		if len(paths) > 0 {
			info.Repo = spaceRepoName(paths[0])
			// The first worktree is the main checkout; a linked one is
			// anywhere else.
			info.Linked = info.Root != "" && info.Root != paths[0]
		}
	}
	if info.Repo == "" && info.Root != "" {
		info.Repo = spaceRepoName(info.Root)
	}
	return info
}

// parseSpaceGitStatus reads `git status --porcelain=v2 --branch` output
// for the branch and its divergence from upstream.
func parseSpaceGitStatus(out string) (branch string, ahead, behind int) {
	for _, raw := range strings.Split(out, "\n") {
		line := strings.TrimSuffix(raw, "\r")
		switch {
		case strings.HasPrefix(line, "# branch.head "):
			branch = strings.TrimSpace(strings.TrimPrefix(line, "# branch.head "))
			if branch == "(detached)" {
				branch = ""
			}
		case strings.HasPrefix(line, "# branch.ab "):
			for _, field := range strings.Fields(strings.TrimPrefix(line, "# branch.ab ")) {
				n, err := strconv.Atoi(strings.TrimPrefix(field, "+"))
				if err != nil {
					continue
				}
				if strings.HasPrefix(field, "-") {
					behind = absSpaceInt(n)
				} else {
					ahead = absSpaceInt(n)
				}
			}
		}
	}
	return branch, ahead, behind
}

// parseSpaceWorktrees reads the worktree paths of porcelain output, main
// checkout first.
func parseSpaceWorktrees(out string) []string {
	worktrees := ParseGitWorktrees([]byte(out))
	paths := make([]string, 0, len(worktrees))
	for _, wt := range worktrees {
		if p := normalizeSpacePath(wt.Path); p != "" {
			paths = append(paths, p)
		}
	}
	return paths
}

// absSpaceInt returns n without its sign.
func absSpaceInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
