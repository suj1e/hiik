package ui

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/plugins/terminal"
	mui "github.com/egoist/mygo/ui"

	"hiik/mods/api"
)

// layoutApp is a fake app with a layout registry and terminal state.
type layoutApp struct {
	*fakeApp
	name  string
	tree  api.Layout
	state api.LayoutState
	infos map[string]api.TerminalInfo
}

func (a *layoutApp) Layout() string { return a.name }

func (a *layoutApp) LayoutTree(name string) (api.Layout, bool) {
	if name == a.name && a.tree.ID != "" {
		return a.tree, true
	}
	return api.BuiltinLayout(name)
}

func (a *layoutApp) LayoutState() api.LayoutState { return a.state.Clone() }

func (a *layoutApp) TerminalInfo(address string) (api.TerminalInfo, bool) {
	info, ok := a.infos[address]
	return info, ok
}

func TestTerminalsLayoutKeepsFullWidthBar(t *testing.T) {
	app := &fakeApp{
		active:   "s1",
		settings: map[string]any{"layout": api.LayoutTerminals},
		spaces: []api.Space{{ID: "s1", Name: "alpha", Tabs: []api.Tab{{
			ID: "t1", Name: "first", Root: leaf("n1", "term-a"),
		}}}},
	}
	_, tt := newTestShell(app)
	for _, want := range []string{"alpha", "first"} {
		if !tt.HasText(want) {
			t.Errorf("the terminals layout does not show %q; texts: %q", want, tt.Texts())
		}
	}
	if tt.HasText("hiik") {
		t.Fatalf("the terminals layout shows the herdr title bar: %q", tt.Texts())
	}
}

func TestUserLayoutRendersTreeAndFallsBack(t *testing.T) {
	app := &layoutApp{
		fakeApp: &fakeApp{},
		name:    "custom",
		tree: api.Layout{ID: "root", Type: "column", Children: []api.Layout{
			{ID: "main", Type: "block", Content: &api.LayoutContent{Mod: "missing/panel"}},
		}},
	}
	_, tt := newTestShell(app)
	tt.Frame()
	if !textsContain(tt.Texts(), "missing/panel") {
		t.Fatalf("a user layout block was not drawn; texts: %q", tt.Texts())
	}

	app.name = "gone"
	app.tree = api.Layout{}
	tt.Frame()
	if !tt.HasText("hiik") {
		t.Fatalf("an unknown layout did not fall back to herdr; texts: %q", tt.Texts())
	}
}

func TestFloatingBlockDrawsAndCloses(t *testing.T) {
	app := &layoutApp{
		fakeApp: &fakeApp{},
		name:    "floaty-layout",
		tree: api.Layout{ID: "root", Type: "block",
			Content: &api.LayoutContent{Mod: "missing/main"},
			Floating: []api.LayoutFloat{{
				ID: "floaty", Content: &api.LayoutContent{Mod: "floaty"},
				Edge: "right", Align: "end",
			}},
		},
	}
	_, tt := newTestShell(app)
	tt.Frame()
	tt.Frame()
	if !tt.HasText("floaty") {
		t.Fatalf("the floating block was not drawn; texts: %q", tt.Texts())
	}
	if err := tt.Click("×"); err != nil {
		t.Fatalf("the floating block has no close button: %v", err)
	}
	tt.Frame()
	if tt.HasText("floaty") {
		t.Fatalf("the floating block did not close; texts: %q", tt.Texts())
	}
}

func TestModPaneFallsBackToNotice(t *testing.T) {
	app := &fakeApp{
		active: "s1",
		spaces: []api.Space{{ID: "s1", Name: "alpha", Tabs: []api.Tab{{
			ID: "t1", Name: "first", Root: &api.Node{ID: "m1", Mod: "nope/panel"},
		}}}},
	}
	_, tt := newTestShell(app)
	if !textsContain(tt.Texts(), "nope/panel") {
		t.Fatalf("a mod pane without a renderer shows no notice; texts: %q", tt.Texts())
	}
}

func TestEndedTerminalShowsRestartAndClose(t *testing.T) {
	code := 7
	app := &layoutApp{
		fakeApp: &fakeApp{
			active: "s1",
			spaces: []api.Space{{ID: "s1", Name: "alpha", Folder: "/work", Tabs: []api.Tab{{
				ID: "t1", Name: "first", Root: leaf("n1", "term-a"),
			}}}},
		},
		infos: map[string]api.TerminalInfo{
			"term-a": {Address: "term-a", Title: "build", Cwd: "/work/project", State: "finished", ExitCode: &code},
		},
	}
	s, tt := newTestShell(app)
	old := s.attachments["term-a"]
	if !tt.HasText("Exited with code 7") {
		t.Fatalf("an ended terminal shows no exit code; texts: %q", tt.Texts())
	}
	for _, want := range []string{"build", "/work/project", "Restart", "Close"} {
		if !tt.HasText(want) {
			t.Errorf("the ended terminal does not show %q; texts: %q", want, tt.Texts())
		}
	}
	if err := tt.Click("Restart"); err != nil {
		t.Fatalf("Restart is not clickable: %v", err)
	}
	found := false
	for _, args := range app.ranArgs {
		if args["folder"] == "/work/project" {
			found = true
		}
	}
	if !found {
		t.Fatalf("Restart did not start a terminal in the old folder: %#v", app.ranArgs)
	}
	if s.attachments["term-a"] == old {
		t.Fatal("Restart kept the dead attachment")
	}
}

func TestReattachAfterCoreLoss(t *testing.T) {
	app := &fakeApp{core: &fakeCore{}}
	s := New(app)
	s.attachmentsMu.Lock()
	s.attachments["term-a"] = &paneTerm{loading: false}
	s.attachmentsMu.Unlock()

	app.mu.Lock()
	app.conn = "connected"
	app.mu.Unlock()
	s.syncConnection()
	if len(s.attachments) != 0 {
		t.Fatal("a connected core did not drop the attachments made while disconnected")
	}

	s.attachmentsMu.Lock()
	s.attachments["term-a"] = &paneTerm{loading: false}
	s.attachmentsMu.Unlock()
	app.mu.Lock()
	app.conn = "disconnected"
	app.mu.Unlock()
	s.syncConnection()
	if len(s.attachments) != 0 {
		t.Fatal("a lost core did not detach the visible terminals")
	}
}

func TestTerminalOptionsReadDisplaySettings(t *testing.T) {
	settings := map[string]any{
		"ligatures":       false,
		"padding_x":       float32(12),
		"padding_y":       float32(8),
		"opacity":         float32(0),
		"cursor_shape":    "bar",
		"cursor_blink":    false,
		"option_key":      "alt",
		"clipboard_write": false,
	}
	theme, font := terminalLook(nil, settings)
	opts := terminalOptions(theme, font, settings)
	if opts.Padding.X != 12 || opts.Padding.Y != 8 {
		t.Fatalf("padding = %#v, want 12x8", opts.Padding)
	}
	if opts.Cursor != terminal.CursorBar {
		t.Fatalf("cursor = %v, want bar", opts.Cursor)
	}
	if !opts.NoBlink {
		t.Fatal("cursor_blink=false did not turn blinking off")
	}
	if !opts.Transparent {
		t.Fatal("opacity below 1 did not make the terminal transparent")
	}
	if !opts.OptionAsAlt {
		t.Fatal("option_key=alt did not reach the terminal")
	}
	if !opts.NoClipboardWrite {
		t.Fatal("clipboard_write=false did not deny terminal clipboard writes")
	}
	if opts.ResizeByConn {
		t.Fatal("the view must size the attached terminal")
	}
	features := strings.Join(opts.Font.Features, ",")
	if !strings.Contains(features, "-calt") || !strings.Contains(features, "-liga") {
		t.Fatalf("ligatures=false did not disable font features: %q", features)
	}
}

func TestHelpAndWelcomeOverlays(t *testing.T) {
	app := &fakeApp{
		overlay: "help",
		cmds:    []api.Command{{ID: "space.new", Title: "New space"}},
	}
	s, tt := newTestShell(app)
	tt.Frame()
	if !s.helpOpen {
		t.Fatal("the help overlay did not open")
	}
	if app.Overlay() != "" {
		t.Fatalf("the help overlay request was not cleared: %q", app.Overlay())
	}
	for _, want := range []string{"Help", "New space", "space.new"} {
		if !tt.HasText(want) {
			t.Errorf("help does not show %q; texts: %q", want, tt.Texts())
		}
	}

	app.SetOverlay("welcome")
	tt.Frame()
	if !s.welcomeOpen {
		t.Fatal("the welcome overlay did not open")
	}
	if !tt.HasText("Welcome to hiik") {
		t.Fatalf("welcome shows no introduction; texts: %q", tt.Texts())
	}
}

func TestLayoutChromeUsesThemeColors(t *testing.T) {
	theme := mui.DarkTheme()
	color, ok := layoutColor("accent", theme)
	if !ok || color != theme.Accent {
		t.Fatalf("layoutColor(accent) = %v, %v", color, ok)
	}
	if _, ok := layoutColor("#ff0000", theme); !ok {
		t.Fatal("layoutColor did not read a hex color")
	}
}

// textsContain reports whether any drawn text contains want.
func textsContain(texts []string, want string) bool {
	for _, text := range texts {
		if strings.Contains(text, want) {
			return true
		}
	}
	return false
}
