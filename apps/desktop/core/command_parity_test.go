package core

import "testing"

// TestReferenceCommandSurface pins the command list the app exposes. The one
// omitted command, mod.clone, is the runtime-mod clone that a compile-time
// Go mod system does not have.
func TestReferenceCommandSurface(t *testing.T) {
	want := []string{
		"agent.focus", "agent.next", "agent.prev",
		"app.check_updates", "app.feedback", "app.help", "app.hide",
		"app.hide_others", "app.palette", "app.quit", "app.reset",
		"app.settings", "app.show_all", "app.theme", "app.update",
		"app.update_core", "app.welcome", "app.whats_new", "app.zoom_in",
		"app.zoom_out", "app.zoom_reset",
		"layout.clone", "layout.hide", "layout.reset", "layout.resize",
		"layout.select", "layout.set", "layout.show", "layout.switch",
		"layout.toggle", "mod.open", "overlay.close",
		"pane.close", "pane.focus", "pane.go", "pane.split", "pane.swap",
		"pane.zoom",
		"space.close", "space.move", "space.new", "space.next", "space.prev",
		"space.rename", "space.select",
		"tab.close", "tab.move", "tab.new", "tab.next", "tab.prev",
		"tab.rename", "tab.select", "tab.settle", "tab.unsettle",
		"terminal.copy", "terminal.keep", "terminal.kill", "terminal.open",
		"terminal.park", "terminal.paste", "terminal.paste_image",
		"terminal.peek", "terminal.pin", "terminal.right_click",
		"terminal.scroll", "terminal.search", "terminal.show",
		"window.close", "window.maximize", "window.minimize",
	}
	a := testApp()
	have := make(map[string]bool, len(want))
	for _, c := range a.Commands() {
		have[c.ID] = true
	}
	for _, id := range want {
		if !have[id] {
			t.Errorf("missing command %q", id)
		}
	}
}
