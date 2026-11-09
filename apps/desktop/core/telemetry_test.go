package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"hiik/core/store"
)

func telemetryTestApp(t *testing.T) (*App, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HIIK_HOME", home)
	a := &App{}
	a.SetSettings(store.DefaultSettings())
	return a, filepath.Join(home, "telemetry")
}

func TestTelemetryOffDeletesPendingAudits(t *testing.T) {
	a, dir := telemetryTestApp(t)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(dir, "audit-1.json")
	if err := os.WriteFile(stale, []byte(`{"version":"old"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(keep, []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := a.SyncTelemetry(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale audit still exists: %v", err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("ClearTelemetry removed a file it does not own: %v", err)
	}
	if _, err := a.WriteTelemetryAudit(); err == nil {
		t.Fatal("writing an audit while telemetry is off should fail")
	}
}

func TestTelemetryAuditIsLocalOnly(t *testing.T) {
	a, dir := telemetryTestApp(t)
	settings := a.Config()
	settings.Privacy.Telemetry = true
	a.SetSettings(settings)

	if err := a.SyncTelemetry(); err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(dir, "audit-*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("audits = %v, want one", files)
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	var audit TelemetryAudit
	if err := json.Unmarshal(data, &audit); err != nil {
		t.Fatalf("audit is not JSON: %v", err)
	}
	if audit.Version != Version {
		t.Fatalf("version = %q, want %q", audit.Version, Version)
	}
	if audit.OS != runtime.GOOS || audit.Arch != runtime.GOARCH {
		t.Fatalf("platform = %s/%s", audit.OS, audit.Arch)
	}
	if audit.Counts.Spaces != 0 || audit.Counts.Panes != 0 {
		t.Fatalf("counts = %#v, want zero", audit.Counts)
	}
	for _, secret := range []string{"folder", "cwd", "command", "terminal"} {
		if strings.Contains(string(data), secret) {
			t.Fatalf("audit contains %q: %s", secret, data)
		}
	}

	settings = a.Config()
	settings.Privacy.Telemetry = false
	a.SetSettings(settings)
	if err := a.SyncTelemetry(); err != nil {
		t.Fatal(err)
	}
	files, err = filepath.Glob(filepath.Join(dir, "audit-*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatalf("audits after disabling = %v, want none", files)
	}
}

func TestTelemetryAuditHasOnlyWhitelistedFields(t *testing.T) {
	a, _ := telemetryTestApp(t)
	settings := a.Config()
	settings.Privacy.Telemetry = true
	settings.CorePath = "/Users/secret/bin/hc"
	settings.Shell = "/bin/zsh"
	settings.Theme = "atom"
	a.SetSettings(settings)
	a.model.createSpace("secret-project", "/Users/secret/project", "secret-machine")

	path, err := a.WriteTelemetryAudit()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var audit map[string]any
	if err := json.Unmarshal(data, &audit); err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"version": true, "os": true, "arch": true, "generatedAt": true,
		"counts": true, "layout": true, "appearance": true, "theme": true,
	}
	for key := range audit {
		if !want[key] {
			t.Errorf("unexpected audit field %q", key)
		}
	}
	for _, leak := range []string{
		"secret", "/Users/", "CorePath", "corePath", "Shell",
	} {
		if strings.Contains(string(data), leak) {
			t.Fatalf("audit leaks %q: %s", leak, data)
		}
	}
}
