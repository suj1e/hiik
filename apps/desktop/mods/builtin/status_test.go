package builtin

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"hiik/mods/api"
)

// statusApp adds the optional keymap and git answers the status bar reads.
type statusApp struct {
	*btPanelApp
	keys   []api.KeyBinding
	branch string
}

func (a *statusApp) Keymap() []api.KeyBinding { return a.keys }

func (a *statusApp) Exec(on, command string, args ...string) (string, string, int, error) {
	if command == "git" {
		for _, arg := range args {
			if arg == "status" {
				if a.branch == "" {
					return "", "", 0, nil
				}
				return "# branch.head " + a.branch + "\n", "", 0, nil
			}
		}
	}
	return "", "", 0, nil
}

func statusPanel() api.Panel { return api.Panels()[api.PanelStatus] }

func renderStatusPanel(t *testing.T, app api.App, props map[string]any) *ui.Tester {
	t.Helper()
	panel := statusPanel()
	tt := ui.NewTester(func(c *ui.Context) {
		panel.Render(&api.RenderContext{C: c, App: app, Props: props})
	}, 900, 600)
	tt.Frame()
	return tt
}

func TestStatusShowsMachineAgentsAndNeeds(t *testing.T) {
	app := &statusApp{
		btPanelApp: &btPanelApp{
			active: "s1",
			spaces: []api.Space{{ID: "s1", Name: "One", Folder: "/work", Machine: "builder/t3"}},
			agents: []api.Agent{
				{Address: "a1", Status: "working"},
				{Address: "a2", Status: "blocked"},
				{Address: "a3", Status: "idle"},
			},
		},
		branch: "main",
	}
	tt := renderStatusPanel(t, app, nil)
	for _, want := range []string{"builder/t3", "main", "3 agents", "1 needs you"} {
		if !tt.HasText(want) {
			t.Fatalf("the status bar does not show %q: %v", want, tt.Texts())
		}
	}
	if tt.HasText("spaces") {
		t.Fatalf("the status bar still counts spaces: %v", tt.Texts())
	}
}

func TestStatusPaletteHintUsesTheBinding(t *testing.T) {
	app := &statusApp{
		btPanelApp: &btPanelApp{
			active: "s1",
			spaces: []api.Space{{ID: "s1", Name: "One"}},
		},
		keys: []api.KeyBinding{{Command: api.CmdPalette, Chord: "ctrl-p", Section: "global"}},
	}
	tt := renderStatusPanel(t, app, nil)
	want := api.FormatChord("ctrl-p")
	if !tt.HasText(want) {
		t.Fatalf("the hint does not show the real binding %q: %v", want, tt.Texts())
	}
	if tt.HasText("Shift P") {
		t.Fatalf("the hint keeps a stale hardcoded key: %v", tt.Texts())
	}
}

func TestStatusPaletteHintWithoutABinding(t *testing.T) {
	app := &statusApp{
		btPanelApp: &btPanelApp{
			active: "s1",
			spaces: []api.Space{{ID: "s1", Name: "One"}},
		},
		keys: []api.KeyBinding{{Command: "app.help", Chord: "ctrl-k", Section: "global"}},
	}
	tt := renderStatusPanel(t, app, nil)
	if !tt.HasText("commands") {
		t.Fatalf("an unbound palette shows no hint: %v", tt.Texts())
	}
	if tt.HasText(api.FormatChord("ctrl-k")) {
		t.Fatalf("an unbound palette invented a key: %v", tt.Texts())
	}
}

func TestStatusUsesTheHostsPaletteHint(t *testing.T) {
	app := &statusApp{
		btPanelApp: &btPanelApp{
			active: "s1",
			spaces: []api.Space{{ID: "s1", Name: "One"}},
		},
	}
	tt := renderStatusPanel(t, app, map[string]any{"palette": "⌘P  commands"})
	if !tt.HasText("⌘P") {
		t.Fatalf("the status bar ignored the host hint: %v", tt.Texts())
	}
}

func TestStatusRetryOnlyWhenDisconnected(t *testing.T) {
	app := &statusApp{btPanelApp: &btPanelApp{active: "s1", spaces: []api.Space{{ID: "s1", Name: "One"}}}}
	connected := renderStatusPanel(t, app, map[string]any{"connection": "connected"})
	if connected.HasText("Retry") {
		t.Fatalf("a connected status shows Retry: %v", connected.Texts())
	}
	offline := renderStatusPanel(t, app, map[string]any{"connection": "disconnected"})
	if !offline.HasText("Retry") {
		t.Fatalf("a disconnected status hides Retry: %v", offline.Texts())
	}
}

func TestStatusPanelDoesNotShowTheConnectionDot(t *testing.T) {
	app := &statusApp{btPanelApp: &btPanelApp{active: "s1", spaces: []api.Space{{ID: "s1", Name: "One"}}}}
	tt := renderStatusPanel(t, app, map[string]any{"connection": "connected"})
	for _, text := range tt.Texts() {
		if strings.Contains(text, "connected") {
			t.Fatalf("the status bar still shows the connection: %v", tt.Texts())
		}
	}
}
