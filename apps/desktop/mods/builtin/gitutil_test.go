package builtin

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuiltinParseGitStatus(t *testing.T) {
	data := []byte("M  staged.go\x00" +
		" M unstaged.go\x00" +
		"?? new.go\x00" +
		"RM renamed.go\x00old.go\x00" +
		"A  added.go\x00" +
		"UU conflict.go\x00")
	entries, err := ParseGitStatus(data)
	if err != nil {
		t.Fatalf("ParseGitStatus: %v", err)
	}
	if len(entries) != 6 {
		t.Fatalf("got %d entries, want 6: %+v", len(entries), entries)
	}
	byPath := map[string]StatusEntry{}
	for _, e := range entries {
		byPath[e.Path] = e
	}
	staged := byPath["staged.go"]
	if !staged.Staged() || staged.Unstaged() || staged.Conflict() {
		t.Errorf("staged.go = %+v, want only staged", staged)
	}
	unstaged := byPath["unstaged.go"]
	if unstaged.Staged() || !unstaged.Unstaged() || unstaged.Conflict() {
		t.Errorf("unstaged.go = %+v, want only unstaged", unstaged)
	}
	if !byPath["new.go"].Untracked() {
		t.Errorf("new.go is not untracked: %+v", byPath["new.go"])
	}
	renamed := byPath["renamed.go"]
	if renamed.OrigPath != "old.go" || renamed.Code() != "RM" {
		t.Errorf("renamed.go = %+v, want RM from old.go", renamed)
	}
	if !byPath["conflict.go"].Conflict() {
		t.Errorf("conflict.go is not a conflict: %+v", byPath["conflict.go"])
	}
}

func TestBuiltinParseGitStatusMalformedDoesNotPanic(t *testing.T) {
	if _, err := ParseGitStatus([]byte("M")); err == nil {
		t.Error("ParseGitStatus(\"M\") succeeded, want an error")
	}
	if _, err := ParseGitStatus([]byte("MXX")); err == nil {
		t.Error("ParseGitStatus(\"MXX\") succeeded, want an error")
	}
	entries, err := ParseGitStatus([]byte("M  no-trailing-nul"))
	if err != nil {
		t.Fatalf("ParseGitStatus without a final NUL: %v", err)
	}
	if len(entries) != 1 || entries[0].Path != "no-trailing-nul" {
		t.Errorf("entries = %+v, want one path", entries)
	}
}

func TestBuiltinParseGitWorktrees(t *testing.T) {
	data := []byte("worktree /repo\n" +
		"HEAD 1111111111111111111111111111111111111111\n" +
		"branch refs/heads/main\n" +
		"\n" +
		"worktree /tmp/wt\n" +
		"HEAD 2222222222222222222222222222222222222222\n" +
		"detached\n" +
		"locked host is away\n" +
		"\n" +
		"worktree /bare\n" +
		"bare\n")
	trees := ParseGitWorktrees(data)
	if len(trees) != 3 {
		t.Fatalf("got %d worktrees, want 3: %+v", len(trees), trees)
	}
	if trees[0].Path != "/repo" || trees[0].Branch != "main" {
		t.Errorf("first = %+v, want /repo on main", trees[0])
	}
	if !trees[1].Detached || !trees[1].Locked || trees[1].LockReason != "host is away" {
		t.Errorf("second = %+v, want detached and locked", trees[1])
	}
	if !trees[2].Bare {
		t.Errorf("third = %+v, want bare", trees[2])
	}
}

func btHaveGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
}

func btGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func btInitRepo(t *testing.T) string {
	t.Helper()
	btHaveGit(t)
	dir := t.TempDir()
	btGit(t, dir, "init", "-q")
	btGit(t, dir, "config", "user.email", "test@example.com")
	btGit(t, dir, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	btGit(t, dir, "add", "a.txt")
	btGit(t, dir, "commit", "-qm", "initial")
	return dir
}

func TestBuiltinGitStatusAndDiff(t *testing.T) {
	dir := btInitRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "staged.txt"), []byte("staged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	btGit(t, dir, "add", "staged.txt")

	entries, err := GitStatus(dir)
	if err != nil {
		t.Fatalf("GitStatus: %v", err)
	}
	byPath := map[string]StatusEntry{}
	for _, e := range entries {
		byPath[e.Path] = e
	}
	if e := byPath["a.txt"]; !e.Unstaged() {
		t.Errorf("a.txt = %+v, want unstaged", e)
	}
	if e := byPath["staged.txt"]; !e.Staged() {
		t.Errorf("staged.txt = %+v, want staged", e)
	}
	if e := byPath["new.txt"]; !e.Untracked() {
		t.Errorf("new.txt = %+v, want untracked", e)
	}

	diff, err := GitDiff(dir, "a.txt", false)
	if err != nil {
		t.Fatalf("GitDiff: %v", err)
	}
	if !strings.Contains(diff, "-one") || !strings.Contains(diff, "+two") {
		t.Errorf("diff = %q, want -one and +two", diff)
	}
	staged, err := GitDiff(dir, "staged.txt", true)
	if err != nil {
		t.Fatalf("GitDiff staged: %v", err)
	}
	if !strings.Contains(staged, "+staged") {
		t.Errorf("staged diff = %q, want +staged", staged)
	}

	nonRepo := t.TempDir()
	if _, err := GitStatus(nonRepo); err == nil {
		t.Fatal("GitStatus in a non-repository succeeded")
	} else if !strings.Contains(strings.ToLower(GitMessage(err)), "not a git repository") {
		t.Errorf("GitMessage(%v) = %q", err, GitMessage(err))
	}
}

func TestBuiltinGitWorktreesCreateAndRemove(t *testing.T) {
	dir := btInitRepo(t)
	root, err := GitRepoRoot(dir)
	if err != nil {
		t.Fatalf("GitRepoRoot: %v", err)
	}
	if root == "" {
		t.Fatal("GitRepoRoot returned an empty root")
	}
	before, err := GitWorktrees(dir)
	if err != nil {
		t.Fatalf("GitWorktrees: %v", err)
	}
	if len(before) == 0 {
		t.Fatal("GitWorktrees returned no entries")
	}
	wtPath := filepath.Join(t.TempDir(), "feature")
	branch := "hiik-test-feature"
	if err := GitAddWorktree(dir, branch, wtPath); err != nil {
		t.Fatalf("GitAddWorktree: %v", err)
	}
	after, err := GitWorktrees(dir)
	if err != nil {
		t.Fatalf("GitWorktrees after add: %v", err)
	}
	found := false
	for _, wt := range after {
		if wt.Branch == branch {
			found = true
		}
	}
	if !found {
		t.Fatalf("worktree on %s not listed: %+v", branch, after)
	}
	if err := GitRemoveWorktree(dir, wtPath, false); err != nil {
		t.Fatalf("GitRemoveWorktree: %v", err)
	}
	removed, err := GitWorktrees(dir)
	if err != nil {
		t.Fatalf("GitWorktrees after remove: %v", err)
	}
	for _, wt := range removed {
		if wt.Branch == branch {
			t.Fatalf("worktree on %s is still listed", branch)
		}
	}
}
