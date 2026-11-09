package core

import (
	"context"
	"testing"

	"hiik/mods/api"
)

func TestEmptyAppSnapshots(t *testing.T) {
	a := testApp()
	if got := a.Spaces(); got != nil && len(got) != 0 {
		t.Fatalf("spaces = %#v", got)
	}
	if got := a.Agents(); len(got) != 0 {
		t.Fatalf("agents = %#v", got)
	}
	if got := a.ActiveSpace(); got != "" {
		t.Fatalf("active space = %q", got)
	}
	if got := a.Connection(); got != "connecting" {
		t.Fatalf("connection = %q", got)
	}
	if err := a.Run("", nil); err == nil {
		t.Fatal("an empty command should fail")
	}
}

func TestConnectionWords(t *testing.T) {
	a := testApp()
	for _, tc := range []struct {
		state Connection
		word  string
	}{
		{Connecting, "connecting"},
		{Connected, "connected"},
		{Disconnected, "disconnected"},
	} {
		a.SetConnection(tc.state, "")
		if got := a.Connection(); got != tc.word {
			t.Fatalf("Connection() = %q, want %q", got, tc.word)
		}
		if got := a.State(); got != tc.state {
			t.Fatalf("State() = %v, want %v", got, tc.state)
		}
	}
	a.SetConnection(Disconnected, "core lost")
	if got := a.LastError(); got != "core lost" {
		t.Fatalf("LastError = %q", got)
	}
}

func TestStartAndClose(t *testing.T) {
	a := testApp()
	if err := a.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := a.Connection(); got != "connecting" {
		t.Fatalf("connection without a core = %q", got)
	}

	fake := newFakeCore()
	a.SetCore(fake)
	if err := a.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := a.Connection(); got != "connected" {
		t.Fatalf("connection after Start = %q", got)
	}

	a.Close()
	if !fake.closed {
		t.Fatal("Close did not close the client")
	}
	if a.Core() != nil {
		t.Fatal("Close left the core attached")
	}
	if got := a.Connection(); got != "disconnected" {
		t.Fatalf("connection after Close = %q", got)
	}
}

func TestSpacesSnapshotIsACopy(t *testing.T) {
	a := testApp()
	a.model.createSpace("api", "/tmp/api", "")
	if _, err := a.model.splitPane("p1", false); err != nil {
		t.Fatal(err)
	}

	snapshot := a.Spaces()
	snapshot[0].Name = "mutated"
	snapshot[0].Tabs[0].Root.Ratio = 0.9
	snapshot[0].Tabs[0].Root.A.Terminal = "t9"

	live := a.Spaces()
	if live[0].Name != "api" {
		t.Fatalf("snapshot leaked a name change: %q", live[0].Name)
	}
	if live[0].Tabs[0].Root.Ratio != 0.5 {
		t.Fatalf("snapshot leaked a ratio change: %v", live[0].Tabs[0].Root.Ratio)
	}
	if live[0].Tabs[0].Root.A.Terminal != "" {
		t.Fatalf("snapshot leaked a terminal change: %q", live[0].Tabs[0].Root.A.Terminal)
	}
}

func TestSetUIStateRepairsValues(t *testing.T) {
	a := testApp()
	a.model.createSpace("api", "/tmp/api", "")
	a.SetUIState(api.UIState{SidebarW: 10, Zoomed: "nope", Focus: "p1"})

	ui := a.UIState()
	if ui.SidebarW != 160 {
		t.Fatalf("sidebar width = %v", ui.SidebarW)
	}
	if ui.Zoomed != "" {
		t.Fatalf("stale zoom kept: %q", ui.Zoomed)
	}
	if ui.Focus != "p1" {
		t.Fatalf("focus = %q", ui.Focus)
	}
}

func TestSettingsMapHasDefaults(t *testing.T) {
	a := testApp()
	settings := a.Settings()
	want := map[string]bool{
		"notify_blocked":  true,
		"notify_done":     true,
		"sound":           true,
		"clipboard_write": true,
		"telemetry":       false,
	}
	for key, value := range want {
		got, ok := settings[key]
		if !ok {
			t.Fatalf("Settings() is missing %q: %#v", key, settings)
		}
		if got != value {
			t.Fatalf("Settings()[%q] = %#v, want %v", key, got, value)
		}
	}

	// The map is a copy.
	settings["telemetry"] = true
	if a.Settings()["telemetry"] != false {
		t.Fatal("Settings() exposed the live map")
	}
}

func TestNewPersistsLayoutSettingsAndUI(t *testing.T) {
	t.Setenv("HIIK_HOME", t.TempDir())

	a := New()
	fake := newFakeCore()
	a.SetCore(fake)
	if err := a.Run(api.CmdSpaceNew, map[string]any{
		"name": "api", "folder": "/tmp/api", "argv": []string{"bash"},
	}); err != nil {
		t.Fatal(err)
	}
	a.SetUIState(api.UIState{SidebarW: 240, Zoomed: "p1", Focus: "p1"})
	if err := a.Run("app.set", map[string]any{"key": "telemetry", "value": true}); err != nil {
		t.Fatal(err)
	}
	a.Close()

	b := New()
	spaces := b.Spaces()
	if len(spaces) != 1 {
		t.Fatalf("loaded spaces = %d", len(spaces))
	}
	if spaces[0].Name != "api" || spaces[0].Folder != "/tmp/api" {
		t.Fatalf("loaded space = %#v", spaces[0])
	}
	if got := spaces[0].Tabs[0].Root.Terminal; got != "t1" {
		t.Fatalf("loaded terminal = %q", got)
	}
	if got := b.ActiveSpace(); got != spaces[0].ID {
		t.Fatalf("loaded active space = %q", got)
	}
	if ui := b.UIState(); ui.SidebarW != 240 || ui.Zoomed != "p1" {
		t.Fatalf("loaded ui = %#v", ui)
	}
	if got := b.Settings()["telemetry"]; got != true {
		t.Fatalf("loaded telemetry = %#v", got)
	}
	b.Close()
}

func TestSettingsPersistAcrossRestart(t *testing.T) {
	t.Setenv("HIIK_HOME", t.TempDir())

	a := New()
	for _, key := range []string{"notify_blocked", "notify_done", "sound"} {
		if err := a.Run("app.set", map[string]any{"key": key, "value": false}); err != nil {
			t.Fatal(err)
		}
	}
	a.Close()

	b := New()
	settings := b.Settings()
	for _, key := range []string{"notify_blocked", "notify_done", "sound"} {
		if settings[key] != false {
			t.Fatalf("%s = %#v, want false", key, settings[key])
		}
	}
	b.Close()
}
