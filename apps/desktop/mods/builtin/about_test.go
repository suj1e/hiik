package builtin

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"hiik/mods/api"
)

func TestAboutPanelRenders(t *testing.T) {
	p, ok := api.Panels()[api.PanelAbout]
	if !ok {
		t.Fatal("about panel is not registered")
	}
	app := &btPanelApp{}
	tt := ui.NewTester(func(c *ui.Context) {
		p.Render(&api.RenderContext{C: c, App: app})
	}, 520, 640)
	for _, want := range []string{"hiik", "Version 0.1.0", "Check for updates", "What's new", "Feedback"} {
		if !tt.HasText(want) {
			t.Errorf("about panel does not show %q", want)
		}
	}
}
