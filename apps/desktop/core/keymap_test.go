package core

import (
	"os"
	"path/filepath"
	"testing"

	mui "github.com/egoist/mygo/ui"

	"hiik/core/store"
	"hiik/mods/api"
)

func keymapApp(keys map[string]string) *App {
	settings := store.DefaultSettings()
	settings.Keys = keys
	return &App{
		cmds: []*cmd{
			{id: "tab.new", title: "New tab", keys: []api.Binding{{Mods: mui.Ctrl, Key: mui.KeyT}}},
			{id: "pane.close", title: "Close pane"},
		},
		settings: settings,
	}
}

func findBinding(list []api.KeyBinding, command string) (api.KeyBinding, bool) {
	for _, b := range list {
		if b.Command == command {
			return b, true
		}
	}
	return api.KeyBinding{}, false
}

func TestKeymapUsesDefaultsAndKeysJSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HIIK_HOME", home)
	app := keymapApp(nil)

	binding, ok := findBinding(app.Keymap(), "tab.new")
	if !ok {
		t.Fatal("tab.new is missing from the keymap")
	}
	if binding.Default != "Ctrl+T" || binding.Chord != "Ctrl+T" {
		t.Fatalf("default = %q, chord = %q", binding.Default, binding.Chord)
	}
	if binding, _ := findBinding(app.Keymap(), "pane.close"); binding.Default != "" || binding.Chord != "" {
		t.Fatalf("unkeyed command = %#v", binding)
	}

	// A flat file maps a command to a chord.
	if err := os.WriteFile(filepath.Join(home, "keys.json"),
		[]byte(`{"tab.new":"Mod+Shift+T"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	binding, _ = findBinding(app.Keymap(), "tab.new")
	if binding.Chord != "Mod+Shift+T" {
		t.Fatalf("flat override = %q", binding.Chord)
	}
	if binding.Default != "Ctrl+T" {
		t.Fatalf("default changed to %q", binding.Default)
	}

	// The reference's sectioned shape maps a chord to a command, and a
	// change applies without building a new app.
	if err := os.WriteFile(filepath.Join(home, "keys.json"),
		[]byte(`{"global":{"alt-2":"tab.new"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	binding, _ = findBinding(app.Keymap(), "tab.new")
	if binding.Chord != "Alt+2" {
		t.Fatalf("sectioned override = %q", binding.Chord)
	}
	if problems := app.KeymapProblems(); len(problems) != 0 {
		t.Fatalf("problems = %v", problems)
	}
}

func TestKeymapSettingsWinAndUnbind(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HIIK_HOME", home)
	if err := os.WriteFile(filepath.Join(home, "keys.json"),
		[]byte(`{"tab.new":"Alt+9"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	app := keymapApp(map[string]string{"tab.new": "Mod+Shift+T"})
	if binding, _ := findBinding(app.Keymap(), "tab.new"); binding.Chord != "Mod+Shift+T" {
		t.Fatalf("settings did not win: %q", binding.Chord)
	}

	app = keymapApp(map[string]string{"tab.new": "none"})
	if binding, _ := findBinding(app.Keymap(), "tab.new"); binding.Chord != "" {
		t.Fatalf("none did not unbind: %q", binding.Chord)
	}
}

func TestKeymapInvalidChordIsReportedNotFatal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HIIK_HOME", home)
	app := keymapApp(map[string]string{"tab.new": "Nope+K"})

	binding, ok := findBinding(app.Keymap(), "tab.new")
	if !ok {
		t.Fatal("an invalid chord removed the command")
	}
	if binding.Chord != "" {
		t.Fatalf("invalid chord bound %q", binding.Chord)
	}
	problems := app.KeymapProblems()
	if len(problems) == 0 {
		t.Fatal("invalid chord was not reported")
	}

	// A bad chord in keys.json is reported and the shipped one applies.
	if err := os.WriteFile(filepath.Join(home, "keys.json"),
		[]byte(`{"tab.new":"Garbage+K"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	app = keymapApp(nil)
	if binding, _ := findBinding(app.Keymap(), "tab.new"); binding.Chord != "Ctrl+T" {
		t.Fatalf("bad keys.json chord changed the default to %q", binding.Chord)
	}
	if problems := app.KeymapProblems(); len(problems) == 0 {
		t.Fatal("bad keys.json chord was not reported")
	}

	// A malformed file must not prevent startup or hide the shipped chord.
	if err := os.WriteFile(filepath.Join(home, "keys.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	app = keymapApp(nil)
	if binding, _ := findBinding(app.Keymap(), "tab.new"); binding.Chord != "Ctrl+T" {
		t.Fatalf("broken keys.json changed the default to %q", binding.Chord)
	}
	if problems := app.KeymapProblems(); len(problems) == 0 {
		t.Fatal("broken keys.json was not reported")
	}
}
