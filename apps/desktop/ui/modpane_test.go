package ui

import (
	"sync"
	"testing"

	mui "github.com/egoist/mygo/ui"

	"hiik/mods/api"
)

var modTestOnce sync.Once

// registerModTestPanels installs the panels these tests render. The mod
// registry is process-wide, so it runs once.
func registerModTestPanels() {
	modTestOnce.Do(func() {
		api.Register(api.Mod{
			ID:   "ui-test-mod",
			Name: "UI test",
			Panels: []api.Panel{
				{
					ID:    "ui-test/panel",
					Title: "Test Panel",
					Render: func(rc *api.RenderContext) {
						mui.Text(rc.C, "Test Panel Body").FontSize(12)
						if text, ok := rc.Props["text"].(string); ok && text != "" {
							mui.Text(rc.C, text).FontSize(12)
						}
						if rc.App == nil {
							mui.Text(rc.C, "no app").FontSize(12)
						}
					},
				},
				{
					ID:    "ui-test/second",
					Title: "Second Panel",
					Render: func(rc *api.RenderContext) {
						mui.Text(rc.C, "Second Panel Body").FontSize(12)
					},
				},
			},
		})
	})
}

func TestModPaneRendererDrawsRegisteredPanel(t *testing.T) {
	registerModTestPanels()
	node := &api.Node{ID: "n1", Mod: "ui-test/panel", Props: map[string]any{"text": "from props"}}
	claimed := false
	tt := mui.NewTester(func(c *mui.Context) {
		claimed = renderModPane(c, node)
	}, 500, 300)
	if !claimed {
		t.Fatal("renderModPane did not claim a mod leaf")
	}
	for _, want := range []string{"Test Panel Body", "from props"} {
		if !tt.HasText(want) {
			t.Errorf("the mod pane does not show %q; texts: %q", want, tt.Texts())
		}
	}
}

func TestModPaneRendererFallsBackForUnknownPanel(t *testing.T) {
	registerModTestPanels()
	node := &api.Node{ID: "n1", Mod: "ui-test/missing"}
	tt := mui.NewTester(func(c *mui.Context) {
		if !renderModPane(c, node) {
			t.Error("renderModPane left an unknown mod panel to the caller")
		}
	}, 500, 300)
	if !tt.HasText(`"ui-test/missing" is not installed`) {
		t.Fatalf("the fallback notice is missing; texts: %q", tt.Texts())
	}
}

func TestModPaneRendererIgnoresTerminalLeaf(t *testing.T) {
	tt := mui.NewTester(func(c *mui.Context) {
		if renderModPane(c, &api.Node{ID: "n1", Terminal: "term-a"}) {
			t.Error("renderModPane claimed a terminal leaf")
		}
	}, 300, 200)
	if tt.HasText("is not installed") {
		t.Fatal("a terminal leaf drew a mod fallback")
	}
}

func TestModWindowOpensAndCloses(t *testing.T) {
	registerModTestPanels()
	_, tt := newTestShell(&fakeApp{})
	OpenModWindow("ui-test/panel", map[string]any{"text": "window prop"})
	tt.Frame()
	for _, want := range []string{"Test Panel", "window prop"} {
		if !tt.HasText(want) {
			t.Fatalf("the mod window does not show %q; texts: %q", want, tt.Texts())
		}
	}
	if err := tt.Click("Close window"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tt.HasText("window prop") {
		t.Fatalf("closing the window left it on screen: %q", tt.Texts())
	}
}

func TestModWindowEscapeClosesUnlessKept(t *testing.T) {
	registerModTestPanels()
	_, tt := newTestShell(&fakeApp{})

	OpenModWindow("ui-test/panel", map[string]any{"text": "escape me"})
	tt.Frame()
	tt.Key(0, mui.KeyEscape)
	tt.Frame()
	if tt.HasText("escape me") {
		t.Fatalf("Escape did not close the mod window: %q", tt.Texts())
	}

	OpenModWindow("ui-test/panel", map[string]any{"text": "keep me"})
	tt.Frame()
	if err := tt.Click("Keep"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("Kept") {
		t.Fatalf("the keep button did not toggle; texts: %q", tt.Texts())
	}
	tt.Key(0, mui.KeyEscape)
	tt.Frame()
	if !tt.HasText("keep me") {
		t.Fatalf("a kept mod window closed on Escape: %q", tt.Texts())
	}
	if err := tt.Click("Close window"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tt.HasText("keep me") {
		t.Fatalf("the kept window's close button did nothing: %q", tt.Texts())
	}
}

func TestModFloatListsAndCloses(t *testing.T) {
	registerModTestPanels()
	_, tt := newTestShell(&fakeApp{})
	OpenModFloat("ui-test/panel", "left", map[string]any{"text": "float one"})
	OpenModFloat("ui-test/second", "right", nil)
	tt.Frame()
	for _, want := range []string{"Test Panel", "float one", "Second Panel", "Second Panel Body"} {
		if !tt.HasText(want) {
			t.Fatalf("the floating regions do not show %q; texts: %q", want, tt.Texts())
		}
	}
	CloseModFloat("ui-test/panel")
	tt.Frame()
	if tt.HasText("float one") {
		t.Fatalf("the closed float still renders: %q", tt.Texts())
	}
	if !tt.HasText("Second Panel Body") {
		t.Fatalf("closing one float removed another: %q", tt.Texts())
	}
}
