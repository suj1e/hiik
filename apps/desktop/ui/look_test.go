package ui

import (
	"strings"
	"testing"

	mui "github.com/egoist/mygo/ui"

	"hiik/mods/api"
)

func TestLookThemeAppearance(t *testing.T) {
	if got := buildLookTheme(false, map[string]any{"appearance": "dark"}); !got.Dark {
		t.Error("appearance dark chose the light theme")
	}
	if got := buildLookTheme(true, map[string]any{"appearance": "light"}); got.Dark {
		t.Error("appearance light chose the dark theme")
	}
}

func TestLookThemeNamePicksFamily(t *testing.T) {
	for _, name := range []string{ThemeGraphite, ThemeInk, ThemeMoss} {
		if got := buildLookTheme(false, map[string]any{"appearance": "system", "theme": name}); !got.Dark {
			t.Errorf("theme %q chose the light theme", name)
		}
	}
	if got := buildLookTheme(true, map[string]any{"appearance": "system", "theme": ThemePaper}); got.Dark {
		t.Error("theme paper chose the dark theme")
	}
}

func TestLookThemeAppearanceWinsOverName(t *testing.T) {
	got := buildLookTheme(false, map[string]any{"appearance": "light", "theme": ThemeGraphite})
	if got.Dark {
		t.Error("an explicit light appearance should beat the graphite theme")
	}
}

func TestLookThemeAppliesUIFont(t *testing.T) {
	got := buildLookTheme(false, map[string]any{"ui_font": "Inter"})
	if got.Font != "Inter" {
		t.Errorf("Font = %q", got.Font)
	}
}

func TestLookThemeIsCached(t *testing.T) {
	settings := map[string]any{"appearance": "dark", "ui_font": "Inter"}
	a := lookTheme(false, settings)
	b := lookTheme(false, settings)
	if a != b {
		t.Fatal("equal settings returned different themes")
	}
	settings["ui_font"] = "Mono"
	if c := lookTheme(false, settings); c == a || c.Font != "Mono" {
		t.Fatal("a changed setting did not build another theme")
	}
}

func TestTerminalLookFollowsTheme(t *testing.T) {
	tm := mui.DarkTheme()
	theme, font := terminalLook(tm, map[string]any{
		"terminal_font": "Menlo",
		"terminal_size": 15,
	})
	if theme.Foreground != tm.Text || theme.Background != tm.Background {
		t.Error("the terminal did not take the theme's text and background")
	}
	if theme.Cursor != tm.Accent {
		t.Error("the terminal cursor is not the theme's accent")
	}
	if theme.Palette[4] != tm.Accent {
		t.Error("palette blue is not the theme's accent")
	}
	if theme.Palette[0] != tm.Background {
		t.Error("palette black is not the theme's background in a dark theme")
	}
	if !strings.HasPrefix(font.Family, "Menlo") || font.Size != 15 {
		t.Errorf("font = %#v", font)
	}
	if !strings.Contains(font.Family, "Noto Sans Mono CJK SC") {
		t.Errorf("font has no CJK fallback: %q", font.Family)
	}
}

func TestTerminalLookDefaults(t *testing.T) {
	theme, font := terminalLook(mui.LightTheme(), nil)
	if theme == nil {
		t.Fatal("no terminal theme")
	}
	if font.Size != defaultTerminalSize {
		t.Errorf("size = %g", font.Size)
	}
	if !strings.HasPrefix(font.Family, "monospace") || !strings.Contains(font.Family, "Sarasa Mono SC") {
		t.Errorf("family = %q", font.Family)
	}
}

func TestKeymapOverrideChangesBinding(t *testing.T) {
	sc := shortcut{action: api.CmdPalette, chord: "Mod+P", run: func(*Shell) {}}

	mods, key, ok, overridden := resolveShortcut(sc, map[string]string{api.CmdPalette: "Mod+J"})
	if !ok || !overridden {
		t.Fatal("the override did not resolve")
	}
	if key != mui.KeyJ {
		t.Errorf("key = %v, want J", key)
	}
	if want := shortcutMods(); mods != want {
		t.Errorf("mods = %v, want %v", mods, want)
	}

	_, key, ok, _ = resolveShortcut(sc, nil)
	if !ok || key != mui.KeyP {
		t.Errorf("the default binding did not resolve: key = %v, ok = %v", key, ok)
	}

	if _, _, ok, _ := resolveShortcut(sc, map[string]string{api.CmdPalette: "none"}); ok {
		t.Error("a none override should leave the action unbound")
	}
	if _, _, ok, _ := resolveShortcut(sc, map[string]string{api.CmdPalette: "Mod+NotAKey"}); ok {
		t.Error("an invalid override should not bind")
	}
}

func TestKeymapOverrideRegistersNewKey(t *testing.T) {
	app := &fakeApp{settings: map[string]any{
		"keys": map[string]any{api.CmdPalette: "Mod+J"},
	}}
	s, tt := newTestShell(app)
	tt.Key(shortcutMods(), mui.KeyK)
	if s.paletteOpen {
		t.Fatal("the overridden default chord still opened the palette")
	}
	tt.Key(shortcutMods(), mui.KeyJ)
	if !s.paletteOpen {
		t.Fatal("the overridden chord did not open the palette")
	}
}
