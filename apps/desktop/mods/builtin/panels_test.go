package builtin

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"hiik/mods/api"
)

type btPanelApp struct {
	spaces   []api.Space
	agents   []api.Agent
	machines []api.Machine
	active   string
	ran      []string
	args     []map[string]any
	runErr   error
}

func (a *btPanelApp) Spaces() []api.Space     { return a.spaces }
func (a *btPanelApp) Agents() []api.Agent     { return a.agents }
func (a *btPanelApp) Machines() []api.Machine { return a.machines }
func (a *btPanelApp) ActiveSpace() string     { return a.active }
func (a *btPanelApp) Commands() []api.Command { return nil }
func (a *btPanelApp) Connection() string      { return "connected" }
func (a *btPanelApp) Settings() map[string]any {
	return map[string]any{
		"notify_blocked":  false,
		"notify_done":     false,
		"sound":           false,
		"clipboard_write": true,
		"telemetry":       false,
	}
}
func (a *btPanelApp) Run(name string, args map[string]any) error {
	a.ran = append(a.ran, name)
	a.args = append(a.args, args)
	return a.runErr
}

// Exec and ReadTextFile are the machine access a panel may use. This test
// app has none; remote panel tests use their own fake.
func (a *btPanelApp) Exec(on, command string, args ...string) (string, string, int, error) {
	return "", "machine commands are not available in this test app", -1,
		errors.New("machine commands are not available in this test app")
}

func (a *btPanelApp) ReadTextFile(path, on string) (string, error) {
	return "", errors.New("file reads are not available in this test app")
}

func btRenderPanel(t *testing.T, p api.Panel, app api.App, props map[string]any, wantText string) {
	t.Helper()
	tt := ui.NewTester(func(c *ui.Context) {
		p.Render(&api.RenderContext{C: c, App: app, Props: props})
	}, 900, 600)
	tt.Frame()
	if wantText != "" && !tt.HasText(wantText) {
		t.Fatalf("panel %s did not show %q; texts: %v", p.Title, wantText, tt.Texts())
	}
}

func TestBuiltinRegisteredPanelsRender(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	app := &btPanelApp{
		spaces: []api.Space{{ID: "s1", Name: "One", Folder: dir}},
		active: "s1",
		agents: []api.Agent{{Address: "t3", Title: "Agent three"}},
	}
	panels := api.Panels()
	for _, id := range []string{api.PanelFiles, api.PanelChanges, api.PanelWorktree, api.PanelPeek} {
		if p := panels[id]; p.Render == nil {
			t.Fatalf("panel %s is not registered", id)
		}
	}
	cases := []struct {
		name  string
		id    string
		props map[string]any
		want  string
	}{
		{"files", api.PanelFiles, nil, "Files"},
		{"changes", api.PanelChanges, nil, "Changes"},
		{"worktree", api.PanelWorktree, nil, "Worktrees"},
		{"peek", api.PanelPeek, map[string]any{"terminal": "t3"}, "t3"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			btRenderPanel(t, panels[tc.id], app, tc.props, tc.want)
		})
	}
}

func TestBuiltinPanelsWithRemoteSpaceUseTheMachine(t *testing.T) {
	app := &btPanelApp{
		spaces: []api.Space{{ID: "r1", Name: "Remote", Folder: "/elsewhere", Machine: "builder"}},
		active: "r1",
	}
	panels := api.Panels()
	for _, id := range []string{api.PanelFiles, api.PanelChanges, api.PanelWorktree} {
		t.Run(id, func(t *testing.T) {
			btRenderPanel(t, panels[id], app, nil, "builder:/elsewhere")
		})
	}
}

func TestBuiltinPanelsWithMissingFolderDoNotPanic(t *testing.T) {
	app := &btPanelApp{
		spaces: []api.Space{{ID: "s1", Name: "Gone", Folder: filepath.Join(t.TempDir(), "gone")}},
		active: "s1",
	}
	panels := api.Panels()
	for _, id := range []string{api.PanelFiles, api.PanelChanges, api.PanelWorktree} {
		t.Run(id, func(t *testing.T) {
			btRenderPanel(t, panels[id], app, nil, "")
		})
	}
}

func TestBuiltinPeekCommandsUseAddress(t *testing.T) {
	app := &btPanelApp{}
	rc := &api.RenderContext{App: app, Props: map[string]any{"terminal": "t3"}}
	runPeekCommand(rc, "t3", peekKeepCommand, true)
	if len(app.ran) != 1 || app.ran[0] != peekKeepCommand {
		t.Fatalf("ran = %v, want %s", app.ran, peekKeepCommand)
	}
	if got := app.args[0]["terminal"]; got != "t3" {
		t.Errorf("terminal = %v, want t3", got)
	}
	if got := app.args[0]["place"]; got != "tab" {
		t.Errorf("place = %v, want tab", got)
	}

	runPeekCommand(rc, "t3", api.CmdPaneClose, false)
	if len(app.ran) != 2 || app.ran[1] != api.CmdPaneClose {
		t.Fatalf("ran = %v, want %s", app.ran, api.CmdPaneClose)
	}
	if got := app.args[1]["terminal"]; got != "t3" {
		t.Errorf("terminal = %v, want t3", got)
	}

	app.runErr = errors.New("nope")
	runPeekCommand(rc, "t3", api.CmdPaneClose, false)
	peekMu.Lock()
	got := peekErrors["t3"]
	peekMu.Unlock()
	if got != "nope" {
		t.Errorf("peek error = %q, want nope", got)
	}
}

func TestBuiltinPeekCloseUsesPropCallback(t *testing.T) {
	closed := 0
	app := &btPanelApp{}
	rc := &api.RenderContext{App: app, Props: map[string]any{
		"terminal": "t3",
		"close":    func() { closed++ },
	}}
	peekClose(rc, "t3")
	if closed != 1 {
		t.Fatalf("close callback ran %d times, want 1", closed)
	}
	if len(app.ran) != 0 {
		t.Fatalf("ran = %v, want no command when a close callback exists", app.ran)
	}

	peekClose(&api.RenderContext{App: app, Props: map[string]any{"terminal": "t3"}}, "t3")
	if len(app.ran) != 1 || app.ran[0] != api.CmdPaneClose {
		t.Fatalf("ran = %v, want %s", app.ran, api.CmdPaneClose)
	}
}

func TestBuiltinPanelsUseSpaceProp(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sp := &api.Space{ID: "p1", Name: "Prop", Folder: dir}
	app := &btPanelApp{}
	panels := api.Panels()
	btRenderPanel(t, panels[api.PanelFiles], app, map[string]any{"space": sp}, "a.txt")
	btRenderPanel(t, panels[api.PanelChanges], app, map[string]any{"space": sp}, "Changes")
	btRenderPanel(t, panels[api.PanelWorktree], app, map[string]any{"space": sp}, "Worktrees")

	remote := &api.Space{ID: "p2", Name: "Far", Folder: dir, Machine: "builder"}
	btRenderPanel(t, panels[api.PanelFiles], app, map[string]any{"space": remote}, "builder:")
}

func TestBuiltinWorktreeOpenRunsSpaceNew(t *testing.T) {
	app := &btPanelApp{}
	rc := &api.RenderContext{App: app}
	st := &worktreeState{}
	worktreeOpen(rc, st, Worktree{Path: filepath.Join(t.TempDir(), "feature")})
	if len(app.ran) != 1 || app.ran[0] != api.CmdSpaceNew {
		t.Fatalf("ran = %v, want %s", app.ran, api.CmdSpaceNew)
	}
	if got := app.args[0]["folder"]; got == "" {
		t.Errorf("folder = %v, want the worktree path", got)
	}
}
