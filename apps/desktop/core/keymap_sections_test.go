package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sectionedKeys = `{
  "global": {"alt-2": "tab.new"},
  "terminal": {"ctrl-left": "send:\u001bb", "ctrl-shift-c": "terminal.copy", "ctrl-1": "terminal"},
  "layout:win98": {"ctrl-shift-pagedown": "space.next"},
  "mod:editor": {"ctrl-s": "editor.save", "ctrl-shift-bad": "editor.bad"}
}`

func writeKeys(t *testing.T, home, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(home, "keys.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestKeymapSectionsAreReadAndListed(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HIIK_HOME", home)
	writeKeys(t, home, sectionedKeys)

	app := keymapApp(nil)

	// The app-wide table is global, and keeps the file's override.
	tab, ok := findBinding(app.Keymap(), "tab.new")
	if !ok {
		t.Fatal("tab.new is missing")
	}
	if tab.Chord != "Alt+2" {
		t.Fatalf("global tab.new = %q", tab.Chord)
	}
	if tab.Section != "global" {
		t.Fatalf("global section = %q", tab.Section)
	}

	terminal := app.KeymapSection("terminal")
	for _, tc := range []struct{ command, chord string }{
		{"terminal", "Ctrl+1"},
		{"terminal.copy", "Ctrl+Shift+C"},
		{"send:\x1bb", "Ctrl+Left"},
	} {
		binding, ok := findBinding(terminal, tc.command)
		if !ok {
			t.Fatalf("terminal section is missing %q: %#v", tc.command, terminal)
		}
		if binding.Chord != tc.chord {
			t.Errorf("%s chord = %q, want %q", tc.command, binding.Chord, tc.chord)
		}
		if binding.Section != "terminal" {
			t.Errorf("%s section = %q", tc.command, binding.Section)
		}
	}

	layout := app.KeymapSection("layout:win98")
	if binding, ok := findBinding(layout, "space.next"); !ok || binding.Chord != "Ctrl+Shift+Pagedown" {
		t.Fatalf("layout section = %#v", layout)
	}
	mod := app.KeymapSection("mod:editor")
	if binding, ok := findBinding(mod, "editor.save"); !ok || binding.Chord != "Ctrl+S" {
		t.Fatalf("mod section = %#v", mod)
	}
	if _, ok := findBinding(mod, "editor.bad"); ok {
		t.Fatal("an invalid chord should not be listed")
	}
	if problems := app.KeymapProblems(); len(problems) == 0 ||
		!strings.Contains(strings.Join(problems, "\n"), "ctrl-shift-bad") {
		t.Fatalf("problems = %v", problems)
	}

	sections := app.KeymapSections()
	want := []string{"global", "layout:win98", "mod:editor", "terminal"}
	if len(sections) != len(want) {
		t.Fatalf("sections = %v", sections)
	}
	for i := range want {
		if sections[i] != want[i] {
			t.Fatalf("sections = %v, want %v", sections, want)
		}
	}

	// The global table is Keymap itself.
	if got := app.KeymapSection("global"); len(got) != len(app.Keymap()) {
		t.Fatalf("global section has %d bindings, Keymap has %d", len(got), len(app.Keymap()))
	}
	if app.KeymapSection("mod:missing") != nil {
		t.Fatal("a missing section should be empty")
	}
}

func TestKeymapSettingsWinOverSections(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HIIK_HOME", home)
	writeKeys(t, home, sectionedKeys)

	app := keymapApp(map[string]string{"tab.new": "Mod+Shift+T"})
	tab, _ := findBinding(app.Keymap(), "tab.new")
	if tab.Chord != "Mod+Shift+T" {
		t.Fatalf("settings should win: %q", tab.Chord)
	}
	if !tab.Explicit {
		t.Fatal("a settings chord should be explicit")
	}
}

func TestParseKeysSectionsAcceptsActionToChord(t *testing.T) {
	sections, problems := parseKeysSections([]byte(
		`{"terminal":{"terminal.copy":"Ctrl+Shift+C","tab.new":"Mod+T"}}`))
	if len(problems) != 0 {
		t.Fatalf("problems = %v", problems)
	}
	if got := sections["terminal"]["terminal.copy"]; got != "Ctrl+Shift+C" {
		t.Fatalf("terminal.copy = %q", got)
	}
	if got := sections["terminal"]["tab.new"]; got != "Mod+T" {
		t.Fatalf("tab.new = %q", got)
	}
}

// A section binding keeps its action's command ID when a built-in knows it.
func TestKeymapSectionTitleUsesCommand(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HIIK_HOME", home)
	writeKeys(t, home, `{"terminal":{"ctrl-shift-t":"tab.new"}}`)

	app := keymapApp(nil)
	app.byID = map[string]*cmd{"tab.new": {id: "tab.new", title: "New tab"}}

	list := app.KeymapSection("terminal")
	binding, ok := findBinding(list, "tab.new")
	if !ok {
		t.Fatalf("terminal = %#v", list)
	}
	if binding.Title != "New tab" {
		t.Fatalf("title = %q", binding.Title)
	}
}
