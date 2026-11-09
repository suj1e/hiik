package core

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	mui "github.com/egoist/mygo/ui"
)

func TestProgramsOfferOmpAndClaude(t *testing.T) {
	t.Setenv("HIIK_HOME", t.TempDir())
	app := &App{}
	app.RefreshPrograms(context.Background())
	byKey := make(map[string]bool)
	for _, p := range app.Programs() {
		byKey[p.Key] = true
		if p.Key == "omp" && (len(p.Run) == 0 || p.Run[0] != "omp") {
			t.Fatalf("omp run = %v", p.Run)
		}
		if p.Icon == "" {
			t.Errorf("program %q has no icon", p.Key)
		}
	}
	for _, key := range []string{"omp", "claude", "codex"} {
		if !byKey[key] {
			t.Errorf("programs do not offer %q", key)
		}
	}
}

func TestProgramsShipInMenuOrder(t *testing.T) {
	t.Setenv("HIIK_HOME", t.TempDir())
	app := &App{}
	want := []string{
		"omp", "claude", "codex", "pi", "pig", "opencode", "cursor", "copilot",
		"amp", "grok", "dsh-tui", "antigravity", "kimi", "kiro", "hermes",
		"qoder", "crush", "gemini",
	}
	if table := app.ProgramTable(); len(table) < 120 {
		t.Fatalf("program table = %d rows, want the reference's ~120", len(table))
	}
	got := app.Programs()
	if len(got) < len(want) {
		t.Fatalf("startable programs = %d, want at least %d", len(got), len(want))
	}
	for i, key := range want {
		if got[i].Key != key {
			t.Fatalf("program %d = %q, want %q", i, got[i].Key, key)
		}
	}
}

// TestProgramsCoverReferenceTable checks the rows the reference ships and
// the common terminals, editors and tools this table adds.
func TestProgramsCoverReferenceTable(t *testing.T) {
	t.Setenv("HIIK_HOME", t.TempDir())
	app := &App{}
	table := app.ProgramTable()
	byKey := make(map[string]bool, len(table))
	for _, p := range table {
		byKey[p.Key] = true
	}
	for _, p := range app.Programs() {
		if len(p.Run) == 0 {
			t.Errorf("startable program %q has no command", p.Key)
		}
	}
	for _, key := range []string{
		"claude", "codex", "pi", "pig", "opencode", "cursor", "copilot",
		"amp", "grok", "dsh-tui", "antigravity", "kimi", "kiro", "hermes",
		"qoder", "crush", "gemini", "vim", "nvim", "emacs", "git", "docker",
		"kitty", "wezterm", "ghostty", "code", "zed", "jq", "make",
	} {
		if !byKey[key] {
			t.Errorf("programs do not offer %q", key)
		}
	}
}

func TestProgramsJSONOverridesFieldByField(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HIIK_HOME", home)
	if err := os.WriteFile(filepath.Join(home, "programs.json"), []byte(`{
		"programs": {
			"claude": {"name": "Claude Plus", "tint": true},
			"custom": {"name": "Custom", "run": ["custom"], "icon": "icons/custom.svg",
				"tint": true, "tile": "#123456", "color": "#abcdef", "match": ["custom-agent"]}
		}
	}`), 0o644); err != nil {
		t.Fatal(err)
	}

	app := &App{}
	var claude, custom bool
	for _, p := range app.Programs() {
		switch p.Key {
		case "claude":
			claude = true
			if p.Name != "Claude Plus" {
				t.Errorf("claude name = %q", p.Name)
			}
			if len(p.Run) != 1 || p.Run[0] != "claude" {
				t.Errorf("claude run was lost: %v", p.Run)
			}
			if p.Icon != "icons/claude.svg" {
				t.Errorf("claude icon was lost: %q", p.Icon)
			}
			if !p.Tint {
				t.Error("claude tint was not overridden")
			}
		case "custom":
			custom = true
			if p.Name != "Custom" || p.Tile != "#123456" || p.Color != "#abcdef" ||
				len(p.Match) != 1 || p.Match[0] != "custom-agent" {
				t.Errorf("custom = %#v", p)
			}
		}
	}
	if !claude || !custom {
		t.Fatalf("merged programs missing entries: claude=%v custom=%v", claude, custom)
	}
}

func TestShippedProgramIconsParse(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Skip("no source path")
	}
	root := filepath.Join(filepath.Dir(file), "..")
	entries, err := parseBuiltinPrograms(shippedProgramsFile())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("the shipped program table is empty")
	}
	for _, entry := range entries {
		if entry.Icon == "" {
			t.Errorf("%s has no icon", entry.Key)
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, "resources", entry.Icon))
		if err != nil {
			t.Errorf("%s icon: %v", entry.Key, err)
			continue
		}
		if _, err := mui.ParseSVG(data); err != nil {
			t.Errorf("%s icon does not parse: %v", entry.Key, err)
		}
	}
}
