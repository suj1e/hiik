package ui

import (
	"testing"

	"github.com/egoist/mygo/plugins/terminal"
	mui "github.com/egoist/mygo/ui"
)

func TestKeymapEditWritesAndValidates(t *testing.T) {
	keys := map[string]string{"pane.close": "Mod+W"}
	next, ok := keymapEdit(keys, "tab.new", "Mod+Shift+T")
	if !ok {
		t.Fatal("Mod+Shift+T should be valid")
	}
	if next["tab.new"] != "Mod+Shift+T" {
		t.Fatalf("tab.new = %q", next["tab.new"])
	}
	if next["pane.close"] != "Mod+W" {
		t.Fatalf("the existing keys were lost: %#v", next)
	}
	if keys["tab.new"] != "" {
		t.Fatal("keymapEdit changed the caller's map")
	}

	if _, ok := keymapEdit(keys, "tab.new", "Mod+NotAKey"); ok {
		t.Fatal("an invalid chord should be refused")
	}
	for _, unbound := range []string{"", "none", "unbound", "-"} {
		next, ok := keymapEdit(keys, "tab.new", unbound)
		if !ok || next["tab.new"] != "" {
			t.Errorf("%q should unbind, got %#v ok=%v", unbound, next, ok)
		}
	}
}

func TestLayoutThemeNamePrefersTheLayout(t *testing.T) {
	settings := map[string]any{
		"theme":         "graphite",
		"layout":        "win98",
		"layout_themes": map[string]string{"win98": "paper"},
	}
	if got := layoutThemeName(settings, "win98"); got != ThemePaper {
		t.Fatalf("win98 theme = %q", got)
	}
	if got := layoutThemeName(settings, "herdr"); got != ThemeGraphite {
		t.Fatalf("herdr theme = %q", got)
	}
	// map[string]any, the shape a decoded settings file gives.
	settings["layout_themes"] = map[string]any{"win98": " Ink "}
	if got := layoutThemeName(settings, "win98"); got != ThemeInk {
		t.Fatalf("win98 theme = %q", got)
	}
}

func TestLookThemeUsesTheLayoutTheme(t *testing.T) {
	base := map[string]any{
		"appearance":    "system",
		"theme":         ThemeGraphite,
		"layout":        "herdr",
		"layout_themes": map[string]string{"win98": ThemePaper},
	}
	if t1, t2 := lookTheme(false, base), lookTheme(false, map[string]any{
		"appearance":    "system",
		"theme":         ThemeGraphite,
		"layout":        "win98",
		"layout_themes": map[string]string{"win98": ThemePaper},
	}); t1 == t2 || t1.Dark == t2.Dark {
		t.Fatalf("layout themes should build different themes: herdr dark=%v, win98 dark=%v", t1.Dark, t2.Dark)
	}
}

func TestApplyLookSetsFrameZoom(t *testing.T) {
	var zoom float32
	tt := mui.NewTester(func(c *mui.Context) {
		applyLookFor(c, map[string]any{"appearance": "dark", "zoom": 1.5}, "herdr")
		zoom = c.Zoom()
	}, 120, 80)
	tt.Frame()
	if zoom != 1.5 {
		t.Fatalf("frame zoom = %g, want 1.5", zoom)
	}
}

func TestTerminalLookLigaturesAndZoom(t *testing.T) {
	th, font := terminalLook(mui.DarkTheme(), map[string]any{
		"terminal_size": 13.0,
		"zoom":          1.5,
		"ligatures":     true,
	})
	if font.Size != 13 {
		t.Fatalf("font size = %g, want 13 (frame zoom scales the view)", font.Size)
	}
	if len(font.Features) != 0 {
		t.Fatalf("ligatures on should leave features alone: %v", font.Features)
	}
	if th.Background.A != 255 {
		t.Fatalf("opaque background alpha = %d", th.Background.A)
	}

	_, off := terminalLook(mui.DarkTheme(), map[string]any{"ligatures": false})
	want := map[string]bool{"-calt": true, "-liga": true}
	for _, feature := range off.Features {
		delete(want, feature)
	}
	if len(want) != 0 {
		t.Fatalf("ligatures off features = %v", off.Features)
	}

	th, _ = terminalLook(mui.DarkTheme(), map[string]any{"opacity": 0.5})
	if th.Background.A >= 250 {
		t.Fatalf("opacity 0.5 background alpha = %d", th.Background.A)
	}
}

func TestTerminalOptionsFollowSettings(t *testing.T) {
	settings := map[string]any{
		"padding_x":       10.0,
		"padding_y":       6.0,
		"zoom":            2.0,
		"cursor_shape":    "underline",
		"cursor_blink":    false,
		"option_key":      "left",
		"pane_scrollbars": false,
		"opacity":         0.0,
	}
	pad := terminalPadding(settings)
	if pad.X != 10 || pad.Y != 6 {
		t.Fatalf("padding = %+v, want 10x6 (frame zoom scales it)", pad)
	}
	if got := terminalPadding(map[string]any{"padding_x": 0.0, "padding_y": 0.0}); got.X != -1 || got.Y != -1 {
		t.Fatalf("zero padding should be none: %+v", got)
	}
	shape, noBlink := terminalCursor(settings)
	if shape != terminal.CursorUnderline || !noBlink {
		t.Fatalf("cursor = %v noBlink=%v", shape, noBlink)
	}
	if !optionAsAlt(settings) {
		t.Fatal("left option should map to Alt")
	}
	if optionAsAlt(map[string]any{"option_key": "native"}) {
		t.Fatal("native option should not map to Alt")
	}
	if paneScrollbars(settings) {
		t.Fatal("pane_scrollbars false should hide the thumb")
	}
	if !terminalTransparent(settings) {
		t.Fatal("opacity 0 should leave the background undrawn")
	}
}

func TestLayoutTitle(t *testing.T) {
	if layoutTitle("herdr") != "Default" || layoutTitle("terminals") != "Terminals" || layoutTitle("win98") != "win98" {
		t.Fatalf("titles: %q %q %q", layoutTitle("herdr"), layoutTitle("terminals"), layoutTitle("win98"))
	}
}
