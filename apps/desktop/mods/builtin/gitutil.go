package builtin

// Git helpers for the changes and worktree panels. They run the user's git
// with a timeout, cap what they read and parse porcelain output without a
// third-party library.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"hiik/mods/api"
)

const (
	// gitTimeout bounds one git process. A repository on a slow network
	// mount fails with a message instead of blocking the panel forever.
	gitTimeout = 10 * time.Second

	// maxGitStatusBytes is large enough for a big repository and small
	// enough that malformed output cannot exhaust memory.
	maxGitStatusBytes  = 8 << 20
	maxGitDiffBytes    = 4 << 20
	maxGitWorktreeSize = 1 << 20
)

// ErrGitMissing reports that no git executable is on PATH.
var ErrGitMissing = errors.New("git is not installed")

// capWriter keeps at most max bytes and notes the rest as truncated.
type capWriter struct {
	buf       bytes.Buffer
	max       int
	truncated bool
}

func (w *capWriter) String() string { return w.buf.String() }

func (w *capWriter) Write(p []byte) (int, error) {
	n := len(p)
	if room := w.max - w.buf.Len(); room > 0 {
		if len(p) > room {
			p = p[:room]
		}
		_, _ = w.buf.Write(p)
	}
	if len(p) < n {
		w.truncated = true
	}
	return n, nil
}

// gitOutput runs git in dir and returns stdout. A missing git yields
// ErrGitMissing, a timeout a message naming the deadline, and a non-zero
// exit the trimmed stderr.
func gitOutput(dir string, max int, args ...string) (out []byte, truncated bool, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	// Never prompt for credentials from a panel, and do not take the index
	// lock for a read-only query.
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0")
	stdout := &capWriter{max: max}
	stderr := &capWriter{max: 64 << 10}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, false, ErrGitMissing
		}
		if ctx.Err() == context.DeadlineExceeded {
			return nil, stdout.truncated, fmt.Errorf("git %s timed out after %s", strings.Join(args, " "), gitTimeout)
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return []byte(stdout.String()), stdout.truncated, fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return []byte(stdout.String()), stdout.truncated, nil
}

// GitMessage turns a git helper error into a short line a panel can show.
func GitMessage(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrGitMissing):
		return "git is not installed"
	case errors.Is(err, context.DeadlineExceeded):
		return "git timed out"
	}
	s := err.Error()
	if strings.Contains(strings.ToLower(s), "not a git repository") {
		return "Not a git repository"
	}
	if strings.Contains(strings.ToLower(s), "timed out") {
		return "git timed out"
	}
	return s
}

// StatusEntry is one path in `git status --porcelain=v1`.
type StatusEntry struct {
	Path     string // the current path
	OrigPath string // the source path of a rename or copy, if any
	X        byte   // the index status
	Y        byte   // the work tree status
}

// Untracked reports an untracked path.
func (s StatusEntry) Untracked() bool { return s.X == '?' && s.Y == '?' }

// Ignored reports an ignored path. Git omits those unless asked.
func (s StatusEntry) Ignored() bool { return s.X == '!' && s.Y == '!' }

// Conflict reports an unmerged path.
func (s StatusEntry) Conflict() bool {
	switch string([]byte{s.X, s.Y}) {
	case "DD", "AU", "UD", "UA", "DU", "AA", "UU":
		return true
	}
	return false
}

// Staged reports a change in the index.
func (s StatusEntry) Staged() bool {
	return !s.Untracked() && !s.Ignored() && !s.Conflict() && s.X != ' '
}

// Unstaged reports a change in the work tree.
func (s StatusEntry) Unstaged() bool {
	return !s.Untracked() && !s.Ignored() && !s.Conflict() && s.Y != ' '
}

// Code returns the two status letters, with a space for no change, as git
// prints them.
func (s StatusEntry) Code() string {
	if s.Untracked() {
		return "??"
	}
	return string([]byte{s.X, s.Y})
}

// GitStatus lists the repository's changes at dir.
func GitStatus(dir string) ([]StatusEntry, error) {
	out, truncated, err := gitOutput(dir, maxGitStatusBytes, "status", "--porcelain=v1", "-z", "--untracked-files=normal")
	if err != nil {
		return nil, err
	}
	if truncated {
		return nil, errors.New("git status output is too large")
	}
	return ParseGitStatus(out)
}

// ParseGitStatus parses `git status --porcelain=v1 -z` output. In that
// format a rename is "XY newpath\0oldpath\0"; a missing final NUL is
// accepted so fixtures and truncated buffers do not panic.
func ParseGitStatus(data []byte) ([]StatusEntry, error) {
	var entries []StatusEntry
	for i := 0; i < len(data); {
		if data[i] == 0 {
			i++
			continue
		}
		if len(data)-i < 3 {
			return nil, fmt.Errorf("truncated git status entry at byte %d", i)
		}
		x, y, sep := data[i], data[i+1], data[i+2]
		if sep != ' ' {
			return nil, fmt.Errorf("malformed git status entry at byte %d", i)
		}
		i += 3
		end := bytes.IndexByte(data[i:], 0)
		var path string
		if end < 0 {
			path = string(data[i:])
			i = len(data)
		} else {
			path = string(data[i : i+end])
			i += end + 1
		}
		e := StatusEntry{Path: path, X: x, Y: y}
		if x == 'R' || x == 'C' || y == 'R' || y == 'C' {
			if i < len(data) {
				end = bytes.IndexByte(data[i:], 0)
				if end < 0 {
					e.OrigPath = string(data[i:])
					i = len(data)
				} else {
					e.OrigPath = string(data[i : i+end])
					i += end + 1
				}
			}
		}
		if e.Path == "" {
			return nil, fmt.Errorf("git status entry at byte %d has no path", i)
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// GitDiff returns a unified, color-free diff for one path. staged reads the
// index against HEAD; otherwise the work tree against the index.
func GitDiff(dir, path string, staged bool) (string, error) {
	args := []string{"diff", "--no-color", "--no-ext-diff"}
	if staged {
		args = append(args, "--cached")
	}
	args = append(args, "--", path)
	out, truncated, err := gitOutput(dir, maxGitDiffBytes, args...)
	if err != nil {
		return "", err
	}
	s := string(out)
	if truncated {
		s += "\n… diff truncated\n"
	}
	return s, nil
}

// Worktree is one entry of `git worktree list --porcelain`.
type Worktree struct {
	Path        string
	Head        string
	Branch      string // the short branch name, or "" when detached or bare
	Bare        bool
	Detached    bool
	Locked      bool
	LockReason  string
	Prunable    bool
	PruneReason string
}

// GitWorktrees lists the worktrees of the repository at dir.
func GitWorktrees(dir string) ([]Worktree, error) {
	out, truncated, err := gitOutput(dir, maxGitWorktreeSize, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	if truncated {
		return nil, errors.New("git worktree output is too large")
	}
	return ParseGitWorktrees(out), nil
}

// ParseGitWorktrees parses the block-oriented porcelain worktree list.
func ParseGitWorktrees(data []byte) []Worktree {
	var out []Worktree
	var cur *Worktree
	flush := func() {
		if cur != nil && cur.Path != "" {
			out = append(out, *cur)
		}
		cur = nil
	}
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSuffix(raw, "\r")
		if line == "" {
			flush()
			continue
		}
		key, value, _ := strings.Cut(line, " ")
		switch key {
		case "worktree":
			flush()
			cur = &Worktree{Path: value}
		case "HEAD":
			if cur != nil {
				cur.Head = value
			}
		case "branch":
			if cur != nil {
				cur.Branch = strings.TrimPrefix(value, "refs/heads/")
			}
		case "bare":
			if cur != nil {
				cur.Bare = true
			}
		case "detached":
			if cur != nil {
				cur.Detached = true
			}
		case "locked":
			if cur != nil {
				cur.Locked = true
				cur.LockReason = value
			}
		case "prunable":
			if cur != nil {
				cur.Prunable = true
				cur.PruneReason = value
			}
		}
	}
	flush()
	return out
}

// GitRepoRoot returns the working tree root that contains dir.
func GitRepoRoot(dir string) (string, error) {
	out, _, err := gitOutput(dir, 1<<20, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	root := strings.TrimRight(string(out), "\r\n")
	if root == "" {
		return "", errors.New("git did not report a repository root")
	}
	return root, nil
}

// GitAddWorktree creates a worktree at path on a new branch. A relative
// path is made absolute first so the result does not depend on git's
// working directory.
func GitAddWorktree(dir, branch, path string) error {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return errors.New("a branch name is required")
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("a worktree path is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	_, _, err = gitOutput(dir, 1<<20, "worktree", "add", "-b", branch, abs)
	return err
}

// GitRemoveWorktree removes the worktree at path. force is passed only when
// the user confirmed a second time, because git then discards changes.
func GitRemoveWorktree(dir, path string, force bool) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("a worktree path is required")
	}
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, path)
	_, _, err := gitOutput(dir, 1<<20, args...)
	return err
}

// defaultWorktreePath builds an unused path outside the repository. The
// caller may pass an explicit path instead; the panels document that.
func defaultWorktreePath(root, branch string) string {
	base := filepath.Base(filepath.Clean(root))
	if base == "." || base == string(filepath.Separator) || base == "" {
		base = "repo"
	}
	safe := sanitizeWorktreeName(branch)
	if safe == "" {
		safe = "worktree"
	}
	dir := filepath.Join(os.TempDir(), "hiik-worktrees")
	for i := 0; ; i++ {
		name := base + "-" + safe
		if i > 0 {
			name += "-" + strconv.Itoa(i+1)
		}
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err != nil {
			return p
		}
	}
}

// sanitizeWorktreeName makes a branch usable as one path component.
func sanitizeWorktreeName(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-.")
}

// The functions below run the same git queries on a machine the core
// reaches. Panels call them with the core, a machine name and a folder; the
// arguments are passed as one argv, so a path is never split by a shell.

// gitOn runs git on a machine and returns its stdout. A missing app, a
// failed exec and a nonzero exit are all errors carrying the core's message.
func gitOn(app api.App, on, dir string, args ...string) (string, error) {
	if app == nil {
		return "", errors.New("no app is available for machine commands")
	}
	argv := make([]string, 0, len(args)+2)
	argv = append(argv, "-C", dir)
	argv = append(argv, args...)
	stdout, stderr, code, err := app.Exec(on, "git", argv...)
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	if code != 0 {
		msg := strings.TrimSpace(stderr)
		if msg == "" {
			msg = fmt.Sprintf("exit %d", code)
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return stdout, nil
}

// GitStatusOn lists a repository's changes on a machine.
func GitStatusOn(app api.App, on, dir string) ([]StatusEntry, error) {
	out, err := gitOn(app, on, dir, "status", "--porcelain=v1", "-z")
	if err != nil {
		return nil, err
	}
	if len(out) > maxGitStatusBytes {
		return nil, errors.New("git status output is too large")
	}
	return ParseGitStatus([]byte(out))
}

// GitDiffOn returns a unified, color-free diff for one path on a machine.
func GitDiffOn(app api.App, on, dir, file string, staged bool) (string, error) {
	args := []string{"diff", "--no-color", "--no-ext-diff"}
	if staged {
		args = append(args, "--cached")
	}
	args = append(args, "--", file)
	out, err := gitOn(app, on, dir, args...)
	if err != nil {
		return "", err
	}
	if len(out) > maxGitDiffBytes {
		out = strings.ToValidUTF8(out[:maxGitDiffBytes], "") + "\n… diff truncated\n"
	}
	return out, nil
}

// GitWorktreesOn lists the worktrees of a repository on a machine.
func GitWorktreesOn(app api.App, on, dir string) ([]Worktree, error) {
	out, err := gitOn(app, on, dir, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	if len(out) > maxGitWorktreeSize {
		return nil, errors.New("git worktree output is too large")
	}
	return ParseGitWorktrees([]byte(out)), nil
}

// GitRepoRootOn returns the working tree root that contains dir on a
// machine.
func GitRepoRootOn(app api.App, on, dir string) (string, error) {
	out, err := gitOn(app, on, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	root := strings.TrimRight(out, "\r\n")
	if root == "" {
		return "", errors.New("git did not report a repository root")
	}
	return root, nil
}

// GitAddWorktreeOn creates a worktree on a machine. The destination is on
// that machine; its parent folders are made there with mkdir -p.
func GitAddWorktreeOn(app api.App, on, dir, branch, dest string) error {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return errors.New("a branch name is required")
	}
	dest = strings.TrimSpace(dest)
	if dest == "" {
		return errors.New("a worktree path is required")
	}
	if app == nil {
		return errors.New("no app is available for machine commands")
	}
	dest = expandRemoteHome(app, on, dest)
	if parent := path.Dir(dest); parent != "" && parent != "." && parent != "/" {
		_, stderr, code, err := app.Exec(on, "mkdir", "-p", parent)
		if err != nil {
			return fmt.Errorf("mkdir -p %s: %w", parent, err)
		}
		if code != 0 {
			msg := strings.TrimSpace(stderr)
			if msg == "" {
				msg = fmt.Sprintf("exit %d", code)
			}
			return fmt.Errorf("mkdir -p %s: %s", parent, msg)
		}
	}
	_, err := gitOn(app, on, dir, "worktree", "add", "-b", branch, dest)
	return err
}

// GitRemoveWorktreeOn removes a worktree on a machine. force is passed only
// after the panel's destructive confirmation.
func GitRemoveWorktreeOn(app api.App, on, dir, dest string, force bool) error {
	dest = strings.TrimSpace(dest)
	if dest == "" {
		return errors.New("a worktree path is required")
	}
	dest = expandRemoteHome(app, on, dest)
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, dest)
	_, err := gitOn(app, on, dir, args...)
	return err
}

// DefaultWorktreePathOn builds an unused destination on a machine, under
// the machine's own home rather than this computer's temp directory.
func DefaultWorktreePathOn(app api.App, on, root, branch string) string {
	name := worktreeDirName(root, branch)
	home := remoteHome(app, on)
	var dir string
	if home != "" {
		dir = path.Join(home, "hiik-worktrees")
	} else {
		// Without a home, stay next to the repository on that machine.
		dir = path.Join(path.Dir(strings.TrimRight(root, "/")), "hiik-worktrees")
	}
	for i := 0; ; i++ {
		leaf := name
		if i > 0 {
			leaf += "-" + strconv.Itoa(i+1)
		}
		p := path.Join(dir, leaf)
		if !remotePathExists(app, on, p) {
			return p
		}
	}
}

// worktreeDirName is the default folder name for a branch in a Unix path.
func worktreeDirName(root, branch string) string {
	base := path.Base(strings.TrimRight(root, "/"))
	if base == "." || base == "/" || base == "" {
		base = "repo"
	}
	safe := sanitizeWorktreeName(branch)
	if safe == "" {
		safe = "worktree"
	}
	return base + "-" + safe
}

// remoteHome asks the machine for its home directory. It is empty when the
// machine or the command is unavailable.
func remoteHome(app api.App, on string) string {
	if app == nil {
		return ""
	}
	out, _, code, err := app.Exec(on, "printenv", "HOME")
	if err != nil || code != 0 {
		return ""
	}
	return strings.TrimSpace(out)
}

// remotePathExists reports whether a path exists on a machine.
func remotePathExists(app api.App, on, p string) bool {
	if app == nil {
		return false
	}
	_, _, code, err := app.Exec(on, "test", "-e", p)
	return err == nil && code == 0
}

// expandRemoteHome turns a leading ~/ into the machine's home, because the
// program is run without a shell that would expand it.
func expandRemoteHome(app api.App, on, p string) string {
	if p == "~" {
		if home := remoteHome(app, on); home != "" {
			return home
		}
		return p
	}
	if strings.HasPrefix(p, "~/") {
		if home := remoteHome(app, on); home != "" {
			return path.Join(home, p[2:])
		}
	}
	return p
}
