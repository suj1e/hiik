package core

import (
	"strings"
	"testing"

	"hiik/mods/api"
)

type fakeModSurface struct {
	window     string
	windowProp map[string]any
	float      string
	edge       string
	floatProp  map[string]any
	closed     string
}

func (f *fakeModSurface) OpenWindow(panelID string, props map[string]any) {
	f.window = panelID
	f.windowProp = props
}

func (f *fakeModSurface) CloseWindow(panelID string) { f.closed = panelID }

func (f *fakeModSurface) OpenFloat(panelID, edge string, props map[string]any) {
	f.float = panelID
	f.edge = edge
	f.floatProp = props
}

func (f *fakeModSurface) CloseFloat(panelID string) { f.closed = panelID }

func TestModWindowUnknownPanel(t *testing.T) {
	a := testApp()
	err := a.Run(api.CmdModWindow, map[string]any{"panel": "no/such"})
	if err == nil || !strings.Contains(err.Error(), `unknown mod panel "no/such"`) {
		t.Fatalf("mod.window error = %v", err)
	}
	err = a.Run(api.CmdModFloat, map[string]any{"mod": "no/such"})
	if err == nil || !strings.Contains(err.Error(), `unknown mod panel "no/such"`) {
		t.Fatalf("mod.float error = %v", err)
	}
	err = a.Run(api.CmdModWindow, nil)
	if err == nil || !strings.Contains(err.Error(), "needs a panel") {
		t.Fatalf("missing panel error = %v", err)
	}
}

func TestModCommandsDispatchToSurface(t *testing.T) {
	api.Register(api.Mod{
		ID:   "core-test-mod",
		Name: "Core test",
		Panels: []api.Panel{{
			ID:     "core-test/panel",
			Title:  "Core test panel",
			Render: func(*api.RenderContext) {},
		}},
	})
	host := &fakeModSurface{}
	api.RegisterModSurface(host)
	a := testApp()

	props := map[string]any{"key": "value"}
	if err := a.Run(api.CmdModWindow, map[string]any{"panel": "core-test/panel", "props": props}); err != nil {
		t.Fatalf("mod.window: %v", err)
	}
	if host.window != "core-test/panel" {
		t.Fatalf("mod.window opened %q", host.window)
	}
	if host.windowProp["key"] != "value" {
		t.Fatalf("mod.window props = %#v", host.windowProp)
	}

	if err := a.Run(api.CmdModFloat, map[string]any{"mod": "core-test/panel", "edge": "left"}); err != nil {
		t.Fatalf("mod.float: %v", err)
	}
	if host.float != "core-test/panel" || host.edge != "left" {
		t.Fatalf("mod.float opened %q on %q", host.float, host.edge)
	}

	if err := a.Run(api.CmdModFloat, map[string]any{"panel": "core-test/panel", "edge": "sideways"}); err == nil ||
		!strings.Contains(err.Error(), `unknown float edge "sideways"`) {
		t.Fatalf("bad edge error = %v", err)
	}
	if err := a.Run(api.CmdModWindow, map[string]any{"panel": "core-test/panel", "props": "nope"}); err == nil ||
		!strings.Contains(err.Error(), "props must be an object") {
		t.Fatalf("bad props error = %v", err)
	}
}
