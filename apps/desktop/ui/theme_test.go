package ui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	mui "github.com/egoist/mygo/ui"
)

func TestAtomOneDarkHasItsOwnPalette(t *testing.T) {
	theme := lookTheme(true, map[string]any{
		"appearance": "dark",
		"theme":      ThemeAtom,
	})
	if theme.Background != mui.Hex("#21252b") {
		t.Fatalf("atom background = %v", theme.Background)
	}
	if theme.Text != mui.Hex("#abb2bf") {
		t.Fatalf("atom foreground = %v", theme.Text)
	}
	if theme.Accent != mui.Hex("#61afef") {
		t.Fatalf("atom accent = %v", theme.Accent)
	}
}

func TestUserThemeFileLoadsAndReloads(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HIIK_HOME", home)
	dir := filepath.Join(home, "themes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "aurora.json")
	ansi := `["#000000","#ff0000","#00ff00","#ffff00","#112233","#ff00ff","#00ffff","#ffffff",` +
		`"#111111","#ff3333","#33ff33","#ffff33","#223344","#ff33ff","#33ffff","#eeeeee"]`
	write := func(body string) {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(`{"background":"#101820","foreground":"#f0f0f0","selection":"#203040","ansi":` + ansi + `}`)

	names := ThemeNames()
	found := false
	for _, name := range names {
		if name == "aurora" {
			found = true
		}
	}
	if !found {
		t.Fatalf("ThemeNames() does not list aurora: %v", names)
	}

	theme := lookTheme(true, map[string]any{"appearance": "dark", "theme": "aurora"})
	if theme.Background != mui.Hex("#101820") {
		t.Fatalf("background = %v, want #101820", theme.Background)
	}
	if theme.Text != mui.Hex("#f0f0f0") {
		t.Fatalf("foreground = %v, want #f0f0f0", theme.Text)
	}
	if theme.Accent != mui.Hex("#112233") {
		t.Fatalf("accent from ansi = %v, want #112233", theme.Accent)
	}
	if palette, ok := ThemePalette(theme); !ok || palette[4] != mui.Hex("#112233") {
		t.Fatalf("ThemePalette() = %v, %v", palette, ok)
	}

	time.Sleep(time.Millisecond)
	write(`{"background":"#102838","foreground":"#f0f0f0","selection":"#203040","ansi":` + ansi + `}`)
	theme = lookTheme(true, map[string]any{"appearance": "dark", "theme": "aurora"})
	if theme.Background != mui.Hex("#102838") {
		t.Fatalf("edited theme background = %v, want #102838", theme.Background)
	}
}

func TestBadThemeFileFallsBack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HIIK_HOME", home)
	dir := filepath.Join(home, "themes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "broken.json"), []byte(`{"background":"nope"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	theme := lookTheme(true, map[string]any{"appearance": "dark", "theme": "broken"})
	if theme.Background == (mui.Color{}) || !theme.Dark {
		t.Fatalf("a bad theme file broke the fallback: %+v", theme)
	}
}
