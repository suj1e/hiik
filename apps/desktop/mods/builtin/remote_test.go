package builtin

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"hiik/mods/api"
)

// remoteExecCall is one command the fake app was asked to run.
type remoteExecCall struct {
	on      string
	command string
	args    []string
}

// remoteReply is the canned result for one command line.
type remoteReply struct {
	stdout string
	stderr string
	code   int
	err    error
}

// remotePanelApp is an api.App whose machine access is canned output. It
// records every Exec and ReadTextFile call so a test can assert the exact
// command and argument boundaries.
type remotePanelApp struct {
	btPanelApp
	calls     []remoteExecCall
	output    map[string]remoteReply
	reads     map[string]string
	readCalls []remoteExecCall
	readErr   error
}

var _ api.App = (*remotePanelApp)(nil)

func newRemoteApp() *remotePanelApp {
	return &remotePanelApp{
		output: map[string]remoteReply{},
		reads:  map[string]string{},
	}
}

// execKey names a command and its arguments without ambiguity.
func execKey(command string, args ...string) string {
	return command + "\x00" + strings.Join(args, "\x00")
}

func (a *remotePanelApp) Exec(on, command string, args ...string) (string, string, int, error) {
	a.calls = append(a.calls, remoteExecCall{on: on, command: command, args: append([]string(nil), args...)})
	if r, ok := a.output[execKey(command, args...)]; ok {
		return r.stdout, r.stderr, r.code, r.err
	}
	return "", "unexpected command: " + execKey(command, args...), 127, nil
}

func (a *remotePanelApp) ReadTextFile(path, on string) (string, error) {
	a.readCalls = append(a.readCalls, remoteExecCall{on: on, args: []string{path}})
	if a.readErr != nil {
		return "", a.readErr
	}
	if text, ok := a.reads[path]; ok {
		return text, nil
	}
	return "", fmt.Errorf("file.read on %s: not_text", on)
}

// expectCall finds one recorded command and checks its machine.
func (a *remotePanelApp) expectCall(t *testing.T, on, command string, args ...string) {
	t.Helper()
	want := execKey(command, args...)
	for _, c := range a.calls {
		if c.on == on && execKey(c.command, c.args...) == want {
			return
		}
	}
	var got []string
	for _, c := range a.calls {
		got = append(got, c.on+": "+execKey(c.command, c.args...))
	}
	t.Fatalf("did not run %s on %s; ran %v", want, on, got)
}

func (a *remotePanelApp) countCalls(command string, args ...string) int {
	want := execKey(command, args...)
	n := 0
	for _, c := range a.calls {
		if execKey(c.command, c.args...) == want {
			n++
		}
	}
	return n
}

func TestRemoteFilesListPreviewAndExpand(t *testing.T) {
	app := newRemoteApp()
	app.output[execKey("ls", "-1Ap", "--", "/repo")] = remoteReply{stdout: "main.go\nsub/\nshot.png\n"}
	app.output[execKey("ls", "-1Ap", "--", "/repo/sub")] = remoteReply{stdout: "deep.txt\n"}
	app.reads["/repo/main.go"] = "package main\n"

	resetFilesState()
	filesMu.Lock()
	st := filesTreeOnLocked("/repo", "builder", app)
	filesMu.Unlock()
	if st.err != "" {
		t.Fatalf("remote tree: %s", st.err)
	}
	if st.root == nil || len(st.root.children) != 3 {
		t.Fatalf("root children = %+v, want 3", st.root)
	}
	first := st.root.children[0]
	if !first.dir || first.name != "sub" || first.path != "/repo/sub" {
		t.Errorf("first child = %+v, want /repo/sub as a folder", first)
	}
	app.expectCall(t, "builder", "ls", "-1Ap", "--", "/repo")

	filesMu.Lock()
	st.ensureLoadedLocked(first)
	filesMu.Unlock()
	if first.err != "" {
		t.Fatalf("load sub: %s", first.err)
	}
	if len(first.children) != 1 || first.children[0].path != "/repo/sub/deep.txt" {
		t.Fatalf("sub children = %+v", first.children)
	}
	app.expectCall(t, "builder", "ls", "-1Ap", "--", "/repo/sub")

	filesMu.Lock()
	st.selectLocked("/repo/main.go")
	filesMu.Unlock()
	if st.previewErr != "" {
		t.Fatalf("preview: %s", st.previewErr)
	}
	if st.preview == nil || st.preview.Text != "package main\n" {
		t.Errorf("preview = %+v, want the text", st.preview)
	}
	if len(app.readCalls) != 1 || app.readCalls[0].on != "builder" || app.readCalls[0].args[0] != "/repo/main.go" {
		t.Errorf("read calls = %+v", app.readCalls)
	}

	filesMu.Lock()
	st.selectLocked("/repo/shot.png")
	filesMu.Unlock()
	if st.preview == nil || !st.preview.Image {
		t.Errorf("image preview = %+v, want an image placeholder", st.preview)
	}
	if len(app.readCalls) != 1 {
		t.Errorf("an image was read as text: %+v", app.readCalls)
	}

	// The panel shows the same tree without the old local-only notice.
	space := &api.Space{ID: "r1", Name: "Remote", Folder: "/repo", Machine: "builder"}
	btRenderPanel(t, api.Panels()[api.PanelFiles], app, map[string]any{"space": space}, "main.go")
}

func TestRemoteFilesShowsCommandError(t *testing.T) {
	app := newRemoteApp()
	app.output[execKey("ls", "-1Ap", "--", "/gone")] = remoteReply{
		stderr: "ls: /gone: No such file or directory", code: 2,
	}
	app.output[execKey("find", "/gone", "-mindepth", "1", "-maxdepth", "1", "-print")] = remoteReply{
		stderr: "find: /gone: No such file or directory", code: 1,
	}

	resetFilesState()
	filesMu.Lock()
	st := filesTreeOnLocked("/gone", "builder", app)
	filesMu.Unlock()
	if !strings.Contains(st.err, "No such file or directory") {
		t.Fatalf("error = %q, want the core's message", st.err)
	}
}

func TestRemoteFilesFallsBackToFind(t *testing.T) {
	app := newRemoteApp()
	app.output[execKey("ls", "-1Ap", "--", "/repo")] = remoteReply{stderr: "ls: illegal option --", code: 1}
	app.output[execKey("find", "/repo", "-mindepth", "1", "-maxdepth", "1", "-print")] = remoteReply{
		stdout: "/repo/a.txt\n/repo/sub\n",
	}
	app.output[execKey("find", "/repo", "-mindepth", "1", "-maxdepth", "1", "-type", "d", "-print")] = remoteReply{
		stdout: "/repo/sub\n",
	}

	resetFilesState()
	filesMu.Lock()
	st := filesTreeOnLocked("/repo", "builder", app)
	filesMu.Unlock()
	if st.err != "" {
		t.Fatalf("fallback tree: %s", st.err)
	}
	if len(st.root.children) != 2 {
		t.Fatalf("children = %+v, want 2", st.root.children)
	}
	if !st.root.children[0].dir || st.root.children[0].name != "sub" {
		t.Errorf("first child = %+v, want the folder first", st.root.children[0])
	}
}

func TestRemoteChangesStatusDiffAndUntracked(t *testing.T) {
	app := newRemoteApp()
	app.output[execKey("git", "-C", "/repo", "status", "--porcelain=v1", "-z")] = remoteReply{
		stdout: " M a.go\x00?? new.txt\x00",
	}
	app.output[execKey("git", "-C", "/repo", "diff", "--no-color", "--no-ext-diff", "--", "a.go")] = remoteReply{
		stdout: "@@ -1 +1 @@\n-old\n+new\n",
	}
	app.reads["/repo/new.txt"] = "hello\n"

	resetChangesState()
	changesMu.Lock()
	st := changesStateOnLocked("/repo", "builder", app)
	changesMu.Unlock()
	if st.err != "" {
		t.Fatalf("status: %s", st.err)
	}
	if len(st.entries) != 2 {
		t.Fatalf("entries = %+v, want 2", st.entries)
	}
	app.expectCall(t, "builder", "git", "-C", "/repo", "status", "--porcelain=v1", "-z")

	changesMu.Lock()
	st.selectLocked(StatusEntry{Path: "a.go", X: ' ', Y: 'M'}, "unstaged")
	changesMu.Unlock()
	if st.diffErr != "" || !strings.Contains(st.diff, "+new") {
		t.Fatalf("diff = %q, err = %q", st.diff, st.diffErr)
	}
	app.expectCall(t, "builder", "git", "-C", "/repo", "diff", "--no-color", "--no-ext-diff", "--", "a.go")

	changesMu.Lock()
	st.selectLocked(StatusEntry{Path: "new.txt", X: '?', Y: '?'}, "untracked")
	changesMu.Unlock()
	if st.diffErr != "" || st.diff != "hello\n" {
		t.Fatalf("untracked diff = %q, err = %q", st.diff, st.diffErr)
	}
	if len(app.readCalls) != 1 || app.readCalls[0].args[0] != "/repo/new.txt" {
		t.Errorf("read calls = %+v", app.readCalls)
	}
}

func TestRemoteChangesShowsGitMessage(t *testing.T) {
	app := newRemoteApp()
	app.output[execKey("git", "-C", "/plain", "status", "--porcelain=v1", "-z")] = remoteReply{
		stderr: "fatal: not a git repository (or any of the parent directories): .git", code: 128,
	}

	resetChangesState()
	changesMu.Lock()
	st := changesStateOnLocked("/plain", "builder", app)
	changesMu.Unlock()
	if st.err != "Not a git repository" {
		t.Fatalf("error = %q, want Not a git repository", st.err)
	}
}

func TestRemoteWorktreesListCreateAndRemove(t *testing.T) {
	app := newRemoteApp()
	app.output[execKey("git", "-C", "/repo", "rev-parse", "--show-toplevel")] = remoteReply{stdout: "/repo\n"}
	app.output[execKey("git", "-C", "/repo", "worktree", "list", "--porcelain")] = remoteReply{
		stdout: "worktree /repo\nHEAD 1111111111111111111111111111111111111111\nbranch refs/heads/main\n\n",
	}
	app.output[execKey("mkdir", "-p", "/home/u")] = remoteReply{}
	app.output[execKey("git", "-C", "/repo", "worktree", "add", "-b", "feature", "/home/u/feature")] = remoteReply{}
	app.output[execKey("git", "-C", "/repo", "worktree", "remove", "/home/u/feature")] = remoteReply{}

	resetWorktreeState()
	worktreeMu.Lock()
	st := worktreeStateOnLocked("/repo", "builder", app)
	worktreeMu.Unlock()
	if st.err != "" {
		t.Fatalf("worktrees: %s", st.err)
	}
	if st.root != "/repo" || len(st.items) != 1 || st.items[0].Branch != "main" {
		t.Fatalf("state = root %q items %+v", st.root, st.items)
	}
	app.expectCall(t, "builder", "git", "-C", "/repo", "rev-parse", "--show-toplevel")
	app.expectCall(t, "builder", "git", "-C", "/repo", "worktree", "list", "--porcelain")

	st.branch = "feature"
	st.path = "/home/u/feature"
	worktreeMu.Lock()
	st.createLocked()
	worktreeMu.Unlock()
	if st.messageErr {
		t.Fatalf("create: %s", st.message)
	}
	if st.message != "Created /home/u/feature" {
		t.Errorf("message = %q", st.message)
	}
	app.expectCall(t, "builder", "mkdir", "-p", "/home/u")
	app.expectCall(t, "builder", "git", "-C", "/repo", "worktree", "add", "-b", "feature", "/home/u/feature")

	worktreeMu.Lock()
	worktreeRemove(st, "/home/u/feature", false)
	worktreeMu.Unlock()
	if st.messageErr {
		t.Fatalf("remove: %s", st.message)
	}
	app.expectCall(t, "builder", "git", "-C", "/repo", "worktree", "remove", "/home/u/feature")
}

func TestRemoteWorktreeDefaultPathIsOnTheMachine(t *testing.T) {
	app := newRemoteApp()
	app.output[execKey("printenv", "HOME")] = remoteReply{stdout: "/home/u\n"}
	app.output[execKey("test", "-e", "/home/u/hiik-worktrees/repo-feature")] = remoteReply{code: 1}
	app.output[execKey("git", "-C", "/repo", "rev-parse", "--show-toplevel")] = remoteReply{stdout: "/repo\n"}
	app.output[execKey("git", "-C", "/repo", "worktree", "list", "--porcelain")] = remoteReply{
		stdout: "worktree /repo\nHEAD 1111111111111111111111111111111111111111\nbranch refs/heads/main\n\n",
	}
	app.output[execKey("mkdir", "-p", "/home/u/hiik-worktrees")] = remoteReply{}
	app.output[execKey("git", "-C", "/repo", "worktree", "add", "-b", "feature", "/home/u/hiik-worktrees/repo-feature")] = remoteReply{}

	if got := DefaultWorktreePathOn(app, "builder", "/repo", "feature"); got != "/home/u/hiik-worktrees/repo-feature" {
		t.Fatalf("default path = %q, want the machine's home", got)
	}

	resetWorktreeState()
	worktreeMu.Lock()
	st := worktreeStateOnLocked("/repo", "builder", app)
	st.branch = "feature"
	st.createLocked()
	worktreeMu.Unlock()
	if st.messageErr {
		t.Fatalf("create: %s", st.message)
	}
	if st.message != "Created /home/u/hiik-worktrees/repo-feature" {
		t.Errorf("message = %q", st.message)
	}
	app.expectCall(t, "builder", "git", "-C", "/repo", "worktree", "add", "-b", "feature", "/home/u/hiik-worktrees/repo-feature")
	if n := app.countCalls("mkdir", "-p", "/home/u/hiik-worktrees"); n != 1 {
		t.Errorf("mkdir ran %d times, want 1", n)
	}
}

func TestRemoteWorktreeOpenKeepsTheMachine(t *testing.T) {
	app := newRemoteApp()
	rc := &api.RenderContext{App: app}
	st := &worktreeState{on: "builder"}
	worktreeOpen(rc, st, Worktree{Path: "/home/u/feature"})
	if st.messageErr {
		t.Fatalf("open: %s", st.message)
	}
	if len(app.ran) != 1 || app.ran[0] != api.CmdSpaceNew {
		t.Fatalf("ran = %v, want %s", app.ran, api.CmdSpaceNew)
	}
	if got := app.args[0]["machine"]; got != "builder" {
		t.Errorf("machine = %v, want builder", got)
	}
	if got := app.args[0]["folder"]; got != "/home/u/feature" {
		t.Errorf("folder = %v", got)
	}
}

func TestRemoteFileReadNotTextIsBinary(t *testing.T) {
	if !remoteNotText(errors.New("hc: file.read: not text (not_text)")) {
		t.Fatal("the core's not_text error was not recognized")
	}
	if remoteNotText(errors.New("machine.exec: unavailable")) {
		t.Fatal("an unrelated error was treated as a binary file")
	}
}
