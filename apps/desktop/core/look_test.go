package core

import (
	"testing"

	"hiik/core/store"
)

func TestAppSetLookPersists(t *testing.T) {
	dir := t.TempDir()
	st, err := store.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	a := newApp(st)

	for _, tc := range []struct {
		key   string
		value any
	}{
		{"appearance", "dark"},
		{"theme", "graphite"},
		{"ui_font", "Inter"},
		{"terminal_font", "JetBrains Mono"},
		{"terminal_size", 15},
		{"core_path", "/opt/hiik/cores"},
		{"keys", map[string]any{"tab.new": "Mod+Shift+T"}},
	} {
		if err := a.Run("app.set", map[string]any{"key": tc.key, "value": tc.value}); err != nil {
			t.Fatalf("app.set %s: %v", tc.key, err)
		}
	}
	if err := st.Flush(); err != nil {
		t.Fatal(err)
	}

	got, err := st.LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if got.Appearance != "dark" {
		t.Errorf("appearance = %q", got.Appearance)
	}
	if got.Theme != "graphite" {
		t.Errorf("theme = %q", got.Theme)
	}
	if got.UIFont != "Inter" {
		t.Errorf("uiFont = %q", got.UIFont)
	}
	if got.TerminalFont != "JetBrains Mono" {
		t.Errorf("terminalFont = %q", got.TerminalFont)
	}
	if got.TerminalSize != 15 {
		t.Errorf("terminalSize = %g", got.TerminalSize)
	}
	if got.CorePath != "/opt/hiik/cores" {
		t.Errorf("corePath = %q", got.CorePath)
	}
	if got.Keys["tab.new"] != "Mod+Shift+T" {
		t.Errorf("keys = %#v", got.Keys)
	}
}

func TestAppSetLookValidation(t *testing.T) {
	a := testApp()
	for _, tc := range []struct {
		key   string
		value any
	}{
		{"appearance", "sepia"},
		{"appearance", true},
		{"terminal_size", "big"},
		{"terminal_size", 3},
		{"terminal_size", 100},
		{"ui_font", 12},
		{"keys", []string{"tab.new"}},
	} {
		if err := a.Run("app.set", map[string]any{"key": tc.key, "value": tc.value}); err == nil {
			t.Errorf("app.set %s=%#v should fail", tc.key, tc.value)
		}
	}
}

func TestLookSettingsAreCopies(t *testing.T) {
	a := testApp()
	if err := a.Run("app.set", map[string]any{
		"key": "keys", "value": map[string]any{"tab.new": "Mod+T"},
	}); err != nil {
		t.Fatal(err)
	}
	settings := a.Settings()
	for _, key := range []string{
		"appearance", "theme", "ui_font", "terminal_font",
		"terminal_size", "core_path", "keys",
	} {
		if _, ok := settings[key]; !ok {
			t.Fatalf("Settings() is missing %q: %#v", key, settings)
		}
	}
	keys := settings["keys"].(map[string]string)
	keys["tab.new"] = "Mod+X"
	if got := a.Look().Keys["tab.new"]; got != "Mod+T" {
		t.Fatalf("a caller changed the stored keymap: %q", got)
	}
	look := a.Look()
	look.Keys["tab.close"] = "Mod+Y"
	if _, ok := a.Look().Keys["tab.close"]; ok {
		t.Fatal("a caller changed the stored keymap through Look")
	}
}
