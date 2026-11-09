package store

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"hiik/mods/api"
)

func TestStateRoundTrip(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	want := State{
		Version:     Version,
		ActiveSpace: "s1",
		Spaces: []Space{{
			ID:        "s1",
			Name:      "api",
			Folder:    "/tmp/api",
			Machine:   "box",
			ActiveTab: 1,
			Tabs: []Tab{
				{ID: "t1", Name: "main", Focus: "p1", Root: &Node{ID: "p1", Terminal: "t7"}},
				{ID: "t2", Name: "two", Focus: "p3", Root: &Node{
					ID:       "p3",
					Vertical: true,
					Ratio:    0.4,
					A:        &Node{ID: "p2", Terminal: "box/t8"},
					B:        &Node{ID: "p4"},
				}},
			},
		}},
		UI: api.UIState{SidebarW: 240, Zoomed: "p1", Focus: "p2"},
	}
	s.SaveState(want)
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}

	got, err := s.LoadState()
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != Version {
		t.Fatalf("version = %d, want %d", got.Version, Version)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip mismatch:\n got %#v\nwant %#v", got, want)
	}

	raw, err := os.ReadFile(s.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"activeTab": 1`) {
		t.Fatalf("state.json does not use activeTab:\n%s", raw)
	}
}

func TestLoadStateMissingUsesDefaults(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	st, err := s.LoadState()
	if err != nil {
		t.Fatal(err)
	}
	if st.Version != Version || len(st.Spaces) != 0 || st.ActiveSpace != "" {
		t.Fatalf("defaults = %#v", st)
	}
}

func TestLoadStateCorruptKeepsFileAside(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.StatePath(), []byte(`{"version":1,`), 0o600); err != nil {
		t.Fatal(err)
	}
	st, err := s.LoadState()
	if err != nil {
		t.Fatal(err)
	}
	if st.Version != Version || len(st.Spaces) != 0 {
		t.Fatalf("defaults = %#v", st)
	}
	if _, err := os.Stat(s.StatePath()); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("corrupt file was not moved: %v", err)
	}
	kept, err := filepath.Glob(s.StatePath() + ".corrupt-*")
	if err != nil {
		t.Fatal(err)
	}
	if len(kept) != 1 {
		t.Fatalf("kept files = %v", kept)
	}
	if len(s.Warnings()) == 0 {
		t.Fatal("no warning for a corrupt file")
	}
}

func TestLoadStateNewerVersionKeptAside(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	newer := `{"version":2,"spaces":[{"id":"s9","name":"future","folder":"/x","tabs":[]}]}`
	if err := os.WriteFile(s.StatePath(), []byte(newer), 0o600); err != nil {
		t.Fatal(err)
	}
	st, err := s.LoadState()
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Spaces) != 0 {
		t.Fatalf("newer state was loaded: %#v", st)
	}
	kept, err := filepath.Glob(s.StatePath() + ".newer-2-*")
	if err != nil {
		t.Fatal(err)
	}
	if len(kept) != 1 {
		t.Fatalf("kept files = %v", kept)
	}
	data, err := os.ReadFile(kept[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != newer {
		t.Fatalf("kept file changed:\n%s", data)
	}
	if _, err := os.Stat(s.StatePath()); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("newer file was overwritten")
	}
}

func TestSettingsRoundTripKeepsUnknownFields(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	raw := `{"appearance":"dark","terminalSize":15,"future":{"a":1}}`
	if err := os.WriteFile(s.SettingsPath(), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	se, err := s.LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if se.Appearance != AppearanceDark || se.TerminalSize != 15 {
		t.Fatalf("settings = %#v", se)
	}
	if _, ok := se.Extra["future"]; !ok {
		t.Fatalf("unknown field not kept: %#v", se.Extra)
	}

	se.Appearance = AppearanceLight
	s.SaveSettings(se)
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}

	var out map[string]json.RawMessage
	data, err := os.ReadFile(s.SettingsPath())
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if string(out["appearance"]) != `"light"` {
		t.Fatalf("appearance = %s", out["appearance"])
	}
	var future struct {
		A int `json:"a"`
	}
	if err := json.Unmarshal(out["future"], &future); err != nil {
		t.Fatal(err)
	}
	if future.A != 1 {
		t.Fatalf("future = %s", out["future"])
	}
}

func TestSettingsDefaultsAndRepair(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	se, err := s.LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if se.Appearance != AppearanceSystem || se.TerminalSize != DefaultTerminalSize {
		t.Fatalf("defaults = %#v", se)
	}
	if se.Privacy.ClipboardWrite != ClipboardAlways || se.Privacy.Telemetry {
		t.Fatalf("privacy defaults = %#v", se.Privacy)
	}

	bad := `{"appearance":"neon","terminalSize":900,"privacy":{"clipboardWrite":"maybe"}}`
	if err := os.WriteFile(s.SettingsPath(), []byte(bad), 0o600); err != nil {
		t.Fatal(err)
	}
	se, err = s.LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if se.Appearance != AppearanceSystem || se.TerminalSize != DefaultTerminalSize ||
		se.Privacy.ClipboardWrite != ClipboardAlways {
		t.Fatalf("repaired = %#v", se)
	}
	if len(s.Warnings()) < 3 {
		t.Fatalf("warnings = %v", s.Warnings())
	}
}

func TestLoadSettingsCorruptKeepsFileAside(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.SettingsPath(), []byte(`not json`), 0o600); err != nil {
		t.Fatal(err)
	}
	se, err := s.LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if se.Appearance != AppearanceSystem {
		t.Fatalf("settings = %#v", se)
	}
	kept, err := filepath.Glob(s.SettingsPath() + ".corrupt-*")
	if err != nil {
		t.Fatal(err)
	}
	if len(kept) != 1 {
		t.Fatalf("kept files = %v", kept)
	}
}

func TestDebounceWaitsForQuiet(t *testing.T) {
	dir := t.TempDir()
	s, err := newStore(dir, 60*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	s.SaveState(State{Version: Version, ActiveSpace: "s1"})
	time.Sleep(20 * time.Millisecond)
	if _, err := os.Stat(s.StatePath()); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("state.json written before the debounce: %v", err)
	}
	time.Sleep(150 * time.Millisecond)
	if _, err := os.Stat(s.StatePath()); err != nil {
		t.Fatalf("state.json not written after the debounce: %v", err)
	}
}

func TestDebounceCoalescesWrites(t *testing.T) {
	s, err := newStore(t.TempDir(), 40*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	s.SaveState(State{Version: Version, ActiveSpace: "first"})
	s.SaveState(State{Version: Version, ActiveSpace: "second"})
	s.SaveState(State{Version: Version, ActiveSpace: "third"})
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	st, err := s.LoadState()
	if err != nil {
		t.Fatal(err)
	}
	if st.ActiveSpace != "third" {
		t.Fatalf("activeSpace = %q, want third", st.ActiveSpace)
	}
}

func TestCloseFlushesAndStopsSaving(t *testing.T) {
	dir := t.TempDir()
	s, err := newStore(dir, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	s.SaveState(State{Version: Version, ActiveSpace: "s1"})
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.StatePath()); err != nil {
		t.Fatalf("Close did not flush: %v", err)
	}
	s.SaveState(State{Version: Version, ActiveSpace: "s2"})
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	st, err := s.LoadState()
	if err != nil {
		t.Fatal(err)
	}
	if st.ActiveSpace != "s1" {
		t.Fatalf("save after Close changed the file: %q", st.ActiveSpace)
	}
}

func TestDirHonorsHIIKHome(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HIIK_HOME", dir)
	got, err := Dir()
	if err != nil {
		t.Fatal(err)
	}
	if got != dir {
		t.Fatalf("Dir = %q, want %q", got, dir)
	}
	s, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	if s.Dir() != dir {
		t.Fatalf("store dir = %q", s.Dir())
	}
}
