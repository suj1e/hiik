package ui

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	mui "github.com/egoist/mygo/ui"

	"hiik/core/hc"
	"hiik/mods/api"
	_ "hiik/mods/builtin"
)

// fakeApp is a scripted app for the shell.
type fakeApp struct {
	mu       sync.Mutex
	spaces   []api.Space
	agents   []api.Agent
	active   string
	agent    string
	cmds     []api.Command
	conn     string
	state    api.UIState
	settings map[string]any
	core     hc.Client
	overlay  string
	ran      []string
	ranArgs  []map[string]any
}

func (a *fakeApp) Spaces() []api.Space {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]api.Space(nil), a.spaces...)
}

func (a *fakeApp) Agents() []api.Agent {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]api.Agent(nil), a.agents...)
}

func (a *fakeApp) Machines() []api.Machine {
	a.mu.Lock()
	defer a.mu.Unlock()
	return nil
}

func (a *fakeApp) Programs() []api.Program {
	return []api.Program{{Key: "claude", Name: "Claude Code", Run: []string{"claude"}, Installed: true}}
}

func (a *fakeApp) Keymap() []api.KeyBinding {
	a.mu.Lock()
	defer a.mu.Unlock()
	var out []api.KeyBinding
	switch m := a.settings["keys"].(type) {
	case map[string]string:
		for command, chord := range m {
			out = append(out, api.KeyBinding{Command: command, Chord: chord, Explicit: true})
		}
	case map[string]any:
		for command, raw := range m {
			if chord, ok := raw.(string); ok {
				out = append(out, api.KeyBinding{Command: command, Chord: chord, Explicit: true})
			}
		}
	}
	return out
}

func (a *fakeApp) Exec(string, string, ...string) (string, string, int, error) {
	return "", "", 0, nil
}

func (a *fakeApp) ReadTextFile(string, string) (string, error) { return "", nil }

func (a *fakeApp) SetNotifier(func(api.Notification)) {}

func (a *fakeApp) Overlay() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.overlay
}

func (a *fakeApp) SetOverlay(name string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.overlay = name
}

func (a *fakeApp) ReleaseNotes() string { return "0.1.0" }

func (a *fakeApp) Version() string { return "0.1.0" }

func (a *fakeApp) CoreVersion() string { return "0.1.0" }

func (a *fakeApp) ActiveSpace() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.active
}

func (a *fakeApp) ActiveAgent() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.agent
}

func (a *fakeApp) Commands() []api.Command {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]api.Command(nil), a.cmds...)
}

func (a *fakeApp) Connection() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.conn == "" {
		return "connecting"
	}
	return a.conn
}

func (a *fakeApp) Run(name string, args map[string]any) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.ran = append(a.ran, name)
	a.ranArgs = append(a.ranArgs, args)
	switch name {
	case api.CmdAgentPeek:
		if address, _ := args["address"].(string); address != "" {
			a.agent = address
			a.overlay = "peek"
		}
	}
	return nil
}

func (a *fakeApp) Core() hc.Client {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.core
}

func (a *fakeApp) UIState() api.UIState {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.state
}

func (a *fakeApp) SetUIState(st api.UIState) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.state = st
}

func (a *fakeApp) Settings() map[string]any {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make(map[string]any, len(a.settings))
	for k, v := range a.settings {
		out[k] = v
	}
	return out
}

func (a *fakeApp) setState(st api.UIState) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.state = st
}

func (a *fakeApp) setActive(id string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.active = id
}

func (a *fakeApp) ranNames() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.ran...)
}

// fakeCore is a scripted core client.
type fakeCore struct {
	mu        sync.Mutex
	reqs      []hc.AttachRequest
	conns     []*fakeConn
	attachErr error
	closed    int
	lost      chan struct{}
}

func (f *fakeCore) Close() error {
	f.mu.Lock()
	f.closed++
	f.mu.Unlock()
	return nil
}

func (f *fakeCore) Hello() hc.Hello { return hc.Hello{} }

func (f *fakeCore) Call(context.Context, string, any, any) error { return nil }

func (f *fakeCore) Subscribe(context.Context) (*hc.Subscription, error) {
	return &hc.Subscription{}, nil
}

func (f *fakeCore) List(context.Context, bool) ([]hc.Terminal, error) { return nil, nil }

func (f *fakeCore) Run(context.Context, hc.RunRequest) (hc.Terminal, error) {
	return hc.Terminal{}, nil
}

func (f *fakeCore) Kill(context.Context, string) error { return nil }

func (f *fakeCore) Label(context.Context, string, map[string]string) error { return nil }

func (f *fakeCore) Attach(_ context.Context, req hc.AttachRequest) (*hc.Attachment, error) {
	f.mu.Lock()
	if f.attachErr != nil {
		err := f.attachErr
		f.mu.Unlock()
		return nil, err
	}
	conn := newFakeConn()
	f.reqs = append(f.reqs, req)
	f.conns = append(f.conns, conn)
	f.mu.Unlock()
	return &hc.Attachment{
		Channel: 1,
		Conn:    conn,
		Close:   func() error { return conn.Close() },
	}, nil
}

func (f *fakeCore) Lost() <-chan struct{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.lost == nil {
		f.lost = make(chan struct{})
	}
	return f.lost
}

func (f *fakeCore) attachRequests() []hc.AttachRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]hc.AttachRequest(nil), f.reqs...)
}

// fakeConn is a bytes-backed stream that blocks until closed.
type fakeConn struct {
	once   sync.Once
	done   chan struct{}
	mu     sync.Mutex
	wrote  []byte
	closed bool
}

func newFakeConn() *fakeConn { return &fakeConn{done: make(chan struct{})} }

func (c *fakeConn) Read(p []byte) (int, error) {
	<-c.done
	return 0, io.EOF
}

func (c *fakeConn) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.wrote = append(c.wrote, p...)
	return len(p), nil
}

func (c *fakeConn) Close() error {
	c.once.Do(func() {
		c.mu.Lock()
		c.closed = true
		c.mu.Unlock()
		close(c.done)
	})
	return nil
}

func (c *fakeConn) Resize(cols, rows int) error { return nil }

func (c *fakeConn) ResizePixels(cols, rows, cellWidth, cellHeight int) error { return nil }

func newTestShell(app App) (*Shell, *mui.Tester) {
	s := New(app)
	return s, mui.NewTester(s.View, 1180, 760)
}

func leaf(id, address string) *api.Node {
	return &api.Node{ID: id, Terminal: address}
}

func TestShellShowsEmptyState(t *testing.T) {
	app := &fakeApp{}
	s, tt := newTestShell(app)
	_ = s
	for _, want := range []string{"hiik", "No spaces yet", "No agent status yet.", "No space open"} {
		if !tt.HasText(want) {
			t.Errorf("the empty shell does not show %q; texts: %q", want, tt.Texts())
		}
	}
}

func TestSidebarAndTabs(t *testing.T) {
	app := &fakeApp{
		active: "s1",
		spaces: []api.Space{
			{ID: "s1", Name: "alpha", Tabs: []api.Tab{
				{ID: "t1", Name: "first", Root: leaf("n1", "term-a")},
				{ID: "t2", Name: "second", Root: leaf("n2", "term-b")},
			}},
			{ID: "s2", Name: "beta"},
		},
		agents: []api.Agent{{Address: "a1", App: "claude", Status: "working"}},
	}
	_, tt := newTestShell(app)
	for _, want := range []string{"alpha", "beta", "first", "second", "claude"} {
		if !tt.HasText(want) {
			t.Errorf("the sidebar or tabs do not show %q; texts: %q", want, tt.Texts())
		}
	}
	if tt.HasText("No spaces yet") || tt.HasText("No agent status yet.") {
		t.Fatalf("a populated shell shows an empty state: %q", tt.Texts())
	}
	if err := tt.Click("beta"); err != nil {
		t.Fatal(err)
	}
	if !contains(app.ranNames(), api.CmdSpaceSelect) {
		t.Fatalf("clicking a space ran %q, want %q", app.ranNames(), api.CmdSpaceSelect)
	}
	if err := tt.Click("second"); err != nil {
		t.Fatal(err)
	}
	if !contains(app.ranNames(), api.CmdTabSelect) {
		t.Fatalf("clicking a tab ran %q, want %q", app.ranNames(), api.CmdTabSelect)
	}
}

func TestPaneSplitRendersBothLeaves(t *testing.T) {
	app := &fakeApp{
		active: "s1",
		spaces: []api.Space{{ID: "s1", Name: "alpha", Tabs: []api.Tab{{
			ID: "t1", Name: "first",
			Root: &api.Node{ID: "split", Ratio: 0.5,
				A: leaf("n1", "term-a"),
				B: leaf("n2", "term-b"),
			},
		}}}},
	}
	_, tt := newTestShell(app)
	for _, want := range []string{"term-a", "term-b"} {
		if !tt.HasText(want) {
			t.Errorf("the split does not show %q; texts: %q", want, tt.Texts())
		}
	}
}

func TestPaletteFiltersCommands(t *testing.T) {
	app := &fakeApp{
		active: "s1",
		spaces: []api.Space{{ID: "s1", Name: "alpha", Tabs: []api.Tab{{
			ID: "t1", Name: "first", Root: leaf("n1", "term-a"),
		}}}},
		cmds: []api.Command{
			{ID: "space.new", Title: "New space"},
			{ID: "app.settings", Title: "Settings"},
		},
	}
	s, tt := newTestShell(app)
	tt.Key(shortcutMods(), mui.KeyP)
	tt.Frame()
	if !s.paletteOpen {
		t.Fatalf("Cmd+P did not open the palette; texts: %q", tt.Texts())
	}
	if !tt.HasText("New space") || !tt.HasText("Settings") {
		t.Fatalf("the palette does not list every command: %q", tt.Texts())
	}
	tt.Type("set")
	tt.Frame()
	if !tt.HasText("Settings") {
		t.Fatalf("the filtered palette lost Settings: %q", tt.Texts())
	}
	if tt.HasText("space.new") {
		t.Fatalf("the filtered palette still shows space.new: %q", tt.Texts())
	}
	tt.Key(0, mui.KeyEnter)
	tt.Frame()
	if !contains(app.ranNames(), "app.settings") {
		t.Fatalf("Enter did not run Settings; ran %q", app.ranNames())
	}
	if s.paletteOpen {
		t.Fatal("running a command left the palette open")
	}
}

func TestPaletteArrowsMoveSelection(t *testing.T) {
	app := &fakeApp{
		active: "s1",
		spaces: []api.Space{{ID: "s1", Name: "alpha", Tabs: []api.Tab{{
			ID: "t1", Name: "first", Root: leaf("n1", "term-a"),
		}}}},
		cmds: []api.Command{
			{ID: "a.one", Title: "One"},
			{ID: "a.two", Title: "Two"},
		},
	}
	s, tt := newTestShell(app)
	tt.Key(shortcutMods(), mui.KeyP)
	tt.Frame()
	s.paletteSel = 0
	tt.Key(0, mui.KeyDown)
	tt.Frame()
	if s.paletteSel != 1 {
		t.Fatalf("Down left the choice at %d, want 1", s.paletteSel)
	}
	tt.Key(0, mui.KeyEnter)
	tt.Frame()
	if !contains(app.ranNames(), "a.two") {
		t.Fatalf("Enter did not run the chosen command; ran %q", app.ranNames())
	}
}

func TestRightPanelToggles(t *testing.T) {
	app := &fakeApp{
		active: "s1",
		spaces: []api.Space{{ID: "s1", Name: "alpha", Folder: "/tmp/work", Tabs: []api.Tab{{
			ID: "t1", Name: "first", Root: leaf("n1", "term-a"),
		}}}},
	}
	s, tt := newTestShell(app)
	if err := tt.Click("Files"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !s.rightOpen || s.rightTab != rightFiles {
		t.Fatalf("Files did not open the right panel: open=%v tab=%q", s.rightOpen, s.rightTab)
	}
	if !tt.HasText("Refresh") {
		t.Fatalf("the right panel did not render the official files panel: %q", tt.Texts())
	}
	if !tt.HasText("term-a") {
		t.Fatalf("the right panel took the panes away: %q", tt.Texts())
	}
	if err := tt.Click("Files"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if s.rightOpen {
		t.Fatal("closing the right panel left it open")
	}
	if tt.HasText("Refresh") {
		t.Fatalf("the closed right panel still renders: %q", tt.Texts())
	}
}

func TestPeekOpensForAnAgentOutsideTheTab(t *testing.T) {
	app := &fakeApp{
		active: "s1",
		spaces: []api.Space{{ID: "s1", Name: "alpha", Tabs: []api.Tab{{
			ID: "t1", Name: "first", Root: leaf("n1", "term-a"),
		}}}},
		agents: []api.Agent{{Address: "agent-1", Title: "reviewer", Space: "s1", Tab: "t1"}},
	}
	s, tt := newTestShell(app)
	if err := tt.Click("reviewer"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if s.peek != "agent-1" {
		t.Fatalf("the agent outside the tab did not peek: %q", s.peek)
	}
	if !tt.HasText("Keep as tab") {
		t.Fatalf("the peek panel did not render: %q", tt.Texts())
	}
	tt.Key(0, mui.KeyEscape)
	tt.Frame()
	if s.peek != "" {
		t.Fatalf("closing the peek left %q", s.peek)
	}
	if tt.HasText("Keep as tab") {
		t.Fatalf("the closed peek still renders: %q", tt.Texts())
	}
}

func TestFocusReturnsAfterPalette(t *testing.T) {
	app := &fakeApp{
		active: "s1",
		spaces: []api.Space{{ID: "s1", Name: "alpha", Tabs: []api.Tab{{
			ID: "t1", Name: "first", Focus: "n1", Root: leaf("n1", "term-a"),
		}}}},
		state: api.UIState{SidebarW: 240, Focus: "n1"},
	}
	s, tt := newTestShell(app)
	if s.focus != "n1" {
		t.Fatalf("the shell focus is %q, want n1", s.focus)
	}
	tt.Key(shortcutMods(), mui.KeyP)
	tt.Frame()
	tt.Key(0, mui.KeyEscape)
	tt.Frame()
	if s.paletteOpen {
		t.Fatal("Escape did not close the palette")
	}
	if s.focus != "n1" {
		t.Fatalf("the palette changed focus to %q", s.focus)
	}
	if !s.focusPending {
		t.Fatal("closing the palette did not ask for the pane's focus back")
	}
}

func TestTerminalAttachAndDetach(t *testing.T) {
	core := &fakeCore{}
	app := &fakeApp{
		active: "s1",
		core:   core,
		spaces: []api.Space{
			{ID: "s1", Name: "alpha", Tabs: []api.Tab{{
				ID: "t1", Name: "first", Focus: "n1", Root: leaf("n1", "term-a"),
			}}},
			{ID: "s2", Name: "beta", Tabs: []api.Tab{{
				ID: "t2", Name: "second", Focus: "n2", Root: leaf("n2", "term-b"),
			}}},
		},
	}
	s, tt := newTestShell(app)
	tt.Frame()
	snap := waitAttached(t, s, "term-a")
	if snap.term == nil {
		t.Fatalf("the terminal did not attach: %s", snap.err)
	}
	reqs := core.attachRequests()
	if len(reqs) != 1 {
		t.Fatalf("attach requests: %d, want 1", len(reqs))
	}
	if reqs[0].Cols != 80 || reqs[0].Rows != 24 {
		t.Fatalf("first attach is %dx%d, want 80x24", reqs[0].Cols, reqs[0].Rows)
	}
	tt.Frame()
	app.setActive("s2")
	tt.Frame()
	waitDetached(t, s, "term-a")
	if snap := s.attachment("term-a"); snap.term != nil {
		t.Fatal("switching spaces kept term-a attached")
	}
	waitAttached(t, s, "term-b")
	if len(core.attachRequests()) != 2 {
		t.Fatalf("attach requests: %d, want 2", len(core.attachRequests()))
	}
	s.Close()
}

func waitAttached(t *testing.T, s *Shell, addr string) attachSnapshot {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		snap := s.attachment(addr)
		if snap.term != nil || snap.err != "" {
			return snap
		}
		time.Sleep(5 * time.Millisecond)
	}
	return s.attachment(addr)
}

func waitDetached(t *testing.T, s *Shell, addr string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := s.attachments[addr]; !ok {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("%s is still attached", addr)
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func TestPaletteHidesCommandsThatNeedArguments(t *testing.T) {
	cmds := []api.Command{
		{ID: "app.help", Title: "Help"},
		{ID: "terminal.copy", Title: "Copy"},
		{ID: "pane.split", Title: "Split"},
	}
	got := filteredCommands(cmds, "")
	if len(got) != 1 || got[0].ID != "app.help" {
		t.Fatalf("visible commands = %+v, want only app.help", got)
	}
}

func TestSyncModelFocusFocusesPane(t *testing.T) {
	app := &fakeApp{state: api.UIState{Focus: "p1"}}
	s := &Shell{app: app, focus: "p1", lastModelFocus: "p1"}
	tab := &api.Tab{Root: &api.Node{ID: "p1", A: &api.Node{ID: "p1", Terminal: "t0"}, B: &api.Node{ID: "p2", Terminal: "t1"}}}
	app.state = api.UIState{Focus: "p2"}
	s.syncModelFocus(tab)
	if s.focus != "p2" || !s.focusPending {
		t.Fatalf("focus = %q pending=%v, want p2", s.focus, s.focusPending)
	}
	// A second frame must not pull focus back after the user moved it.
	s.focus = "p1"
	s.focusPending = false
	s.syncModelFocus(tab)
	if s.focus != "p1" {
		t.Fatalf("focus moved without a model change: %q", s.focus)
	}
}

func TestSidebarWidthDefaultsAndToggle(t *testing.T) {
	s := New(&fakeApp{})
	if s.sidebarW != sidebarDefaultW {
		t.Fatalf("default sidebar width = %v, want %v", s.sidebarW, sidebarDefaultW)
	}
	if s.sidebarCompact() {
		t.Fatal("the default sidebar is compact")
	}

	s.sidebarW = 300
	s.toggleSidebar()
	if s.sidebarW != sidebarRailWidth() || !s.sidebarCompact() {
		t.Fatalf("collapse left width %v, want rail %v", s.sidebarW, sidebarRailWidth())
	}
	if s.sidebarPrevW != 300 {
		t.Fatalf("collapse kept previous width %v, want 300", s.sidebarPrevW)
	}
	s.toggleSidebar()
	if s.sidebarW != 300 {
		t.Fatalf("expand restored %v, want 300", s.sidebarW)
	}

	persisted := New(&fakeApp{state: api.UIState{SidebarW: 320}})
	if persisted.sidebarW != 320 {
		t.Fatalf("persisted sidebar width = %v, want 320", persisted.sidebarW)
	}

	narrow := New(&fakeApp{})
	narrow.sidebarW = 100
	narrow.toggleSidebar()
	if narrow.sidebarW != sidebarDefaultW {
		t.Fatalf("expanding a narrow sidebar gave %v, want %v", narrow.sidebarW, sidebarDefaultW)
	}
}

func TestSidebarPanelsGetChromeProps(t *testing.T) {
	app := &fakeApp{agent: "machine/a1"}
	s, tt := newTestShell(app)
	got := make(map[string]map[string]any)
	for _, id := range []string{api.PanelSpaces, api.PanelAgents} {
		id := id
		s.panels[id] = api.Panel{ID: id, Render: func(rc *api.RenderContext) {
			got[id] = rc.Props
		}}
	}

	s.sidebarW = 120
	tt.Frame()
	for _, id := range []string{api.PanelSpaces, api.PanelAgents} {
		props, ok := got[id]
		if !ok {
			t.Fatalf("panel %q did not render", id)
		}
		if props["compact"] != true {
			t.Fatalf("panel %q compact = %v, want true", id, props["compact"])
		}
		if props["activeAgent"] != "machine/a1" {
			t.Fatalf("panel %q activeAgent = %v, want machine/a1", id, props["activeAgent"])
		}
	}

	s.sidebarW = sidebarDefaultW
	tt.Frame()
	if got[api.PanelSpaces]["compact"] != false || got[api.PanelAgents]["compact"] != false {
		t.Fatalf("a wide sidebar still passes compact: %v", got)
	}
}

func TestSidebarChromeLabelsAndFooter(t *testing.T) {
	app := &fakeApp{}
	s, tt := newTestShell(app)
	for _, want := range []string{"hiik", "Settings", "Add machine"} {
		if !tt.HasText(want) {
			t.Fatalf("the sidebar does not show %q; texts: %q", want, tt.Texts())
		}
	}
	if err := tt.Click("Settings"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !s.settingsOpen {
		t.Fatal("the Settings row did not open settings")
	}

	machines := &fakeApp{}
	sm, tm := newTestShell(machines)
	if err := tm.Click("Add machine"); err != nil {
		t.Fatal(err)
	}
	tm.Frame()
	if !sm.machinesOpen {
		t.Fatal("the Add machine row did not open the machines dialog")
	}

	rail := &fakeApp{}
	sr, tr := newTestShell(rail)
	sr.sidebarW = 120
	tr.Frame()
	if tr.HasText("Add machine") || tr.HasText("Settings") {
		t.Fatalf("the rail still shows footer labels: %q", tr.Texts())
	}
}

type branchApp struct {
	*fakeApp
	infos map[string]api.TerminalInfo
}

func (a *branchApp) TerminalInfo(address string) (api.TerminalInfo, bool) {
	info, ok := a.infos[address]
	return info, ok
}

func (a *branchApp) Exec(on, command string, args ...string) (string, string, int, error) {
	if command == "git" {
		return "main\n", "", 0, nil
	}
	return "", "", 0, nil
}

func TestCurrentFolderAndBranchFollowFocusedTerminal(t *testing.T) {
	app := &branchApp{
		fakeApp: &fakeApp{
			active: "s1",
			spaces: []api.Space{{ID: "s1", Name: "hiik", Folder: "/home", Tabs: []api.Tab{{
				ID: "t1", Name: "1", Focus: "p1", Root: leaf("p1", "term-a"),
			}}}},
		},
		infos: map[string]api.TerminalInfo{"term-a": {Address: "term-a", Cwd: "/repo"}},
	}
	s, _ := newTestShell(app)
	sp := app.spaces[0]
	if got := s.currentFolder(&sp); got != "/repo" {
		t.Fatalf("currentFolder = %q, want /repo", got)
	}
	if got := s.statusBranch(&sp); got != "main" {
		t.Fatalf("statusBranch = %q, want main", got)
	}
}
