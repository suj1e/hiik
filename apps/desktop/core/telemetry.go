package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"hiik/core/store"
	"hiik/mods/api"
)

// errTelemetryOff reports a write while the user has telemetry disabled.
var errTelemetryOff = errors.New("telemetry is off")

// TelemetryCounts is how many of each thing the app holds.
type TelemetryCounts struct {
	Spaces   int `json:"spaces"`
	Tabs     int `json:"tabs"`
	Panes    int `json:"panes"`
	Agents   int `json:"agents"`
	Machines int `json:"machines"`
}

// TelemetryAudit is exactly what an opt-in report would contain: the app
// version, the platform, counts and look settings. It never carries
// terminal contents, folder names, machine targets or any other user text.
// The app writes this JSON locally for inspection and sends nothing.
type TelemetryAudit struct {
	Version     string          `json:"version"`
	OS          string          `json:"os"`
	Arch        string          `json:"arch"`
	GeneratedAt time.Time       `json:"generatedAt"`
	Counts      TelemetryCounts `json:"counts"`
	Layout      string          `json:"layout"`
	Appearance  string          `json:"appearance"`
	Theme       string          `json:"theme"`
}

// telemetrySeen remembers the last state synced for one app, so a frame
// hook can call SyncTelemetry cheaply. The key is the app pointer; an app
// lives for the process, so this map is small.
var (
	telemetryMu   sync.Mutex
	telemetrySeen = make(map[*App]bool)
)

// TelemetryAudit returns the report the app would write while telemetry is
// enabled. It reads only snapshots.
func (a *App) TelemetryAudit() TelemetryAudit {
	spaces := a.Spaces()
	agents := a.Agents()
	machines := a.Machines()
	a.mu.RLock()
	settings := a.settings
	a.mu.RUnlock()

	var counts TelemetryCounts
	counts.Spaces = len(spaces)
	for _, sp := range spaces {
		counts.Tabs += len(sp.Tabs)
		for i := range sp.Tabs {
			counts.Panes += countLeaves(sp.Tabs[i].Root)
		}
	}
	counts.Agents = len(agents)
	counts.Machines = len(machines)

	return TelemetryAudit{
		Version:     Version,
		OS:          runtime.GOOS,
		Arch:        runtime.GOARCH,
		GeneratedAt: time.Now().UTC(),
		Counts:      counts,
		Layout:      settings.Layout,
		Appearance:  settings.Appearance,
		Theme:       settings.Theme,
	}
}

// countLeaves counts terminal leaves in a pane tree.
func countLeaves(n *api.Node) int {
	if n == nil {
		return 0
	}
	if n.A == nil || n.B == nil {
		return 1
	}
	return countLeaves(n.A) + countLeaves(n.B)
}

// telemetryDir returns the folder holding local audits. It is under the
// app's config directory, $HIIK_HOME/telemetry.
func (a *App) telemetryDir() (string, error) {
	a.mu.RLock()
	st := a.store
	a.mu.RUnlock()
	dir := ""
	if st != nil {
		dir = st.Dir()
	} else {
		d, err := store.Dir()
		if err != nil {
			return "", err
		}
		dir = d
	}
	return filepath.Join(dir, "telemetry"), nil
}

// WriteTelemetryAudit writes one local audit to
// $HIIK_HOME/telemetry/audit-<time>.json. It refuses when telemetry is
// off and removes pending audits in that case. Nothing leaves the machine.
func (a *App) WriteTelemetryAudit() (string, error) {
	audit := a.TelemetryAudit()
	a.mu.RLock()
	enabled := a.settings.Privacy.Telemetry
	a.mu.RUnlock()
	if !enabled {
		_ = a.ClearTelemetry()
		return "", errTelemetryOff
	}
	dir, err := a.telemetryDir()
	if err != nil {
		return "", fmt.Errorf("telemetry: %w", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("telemetry: create %s: %w", dir, err)
	}
	data, err := json.MarshalIndent(audit, "", "  ")
	if err != nil {
		return "", fmt.Errorf("telemetry: encode: %w", err)
	}
	data = append(data, '\n')
	name := fmt.Sprintf("audit-%d.json", audit.GeneratedAt.UnixNano())
	path := filepath.Join(dir, name)
	if err := writeTelemetryFile(path, data); err != nil {
		return "", err
	}
	return path, nil
}

// writeTelemetryFile writes data atomically next to path.
func writeTelemetryFile(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("telemetry: write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("telemetry: save %s: %w", path, err)
	}
	return nil
}

// ClearTelemetry removes local audits the app wrote. It leaves files it
// does not own alone.
func (a *App) ClearTelemetry() error {
	dir, err := a.telemetryDir()
	if err != nil {
		return fmt.Errorf("telemetry: %w", err)
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("telemetry: read %s: %w", dir, err)
	}
	var first error
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "audit-") || !strings.HasSuffix(name, ".json") {
			continue
		}
		if err := os.Remove(filepath.Join(dir, name)); err != nil && first == nil {
			first = fmt.Errorf("telemetry: remove %s: %w", name, err)
		}
	}
	return first
}

// SyncTelemetry reconciles the local audit folder with the current
// setting: it writes one audit when telemetry is enabled and removes
// pending audits when it is disabled. A frame hook calls it; repeated
// calls with the same setting do nothing.
func (a *App) SyncTelemetry() error {
	a.mu.RLock()
	enabled := a.settings.Privacy.Telemetry
	a.mu.RUnlock()

	telemetryMu.Lock()
	last, seen := telemetrySeen[a]
	if seen && last == enabled {
		telemetryMu.Unlock()
		return nil
	}
	telemetrySeen[a] = enabled
	telemetryMu.Unlock()

	var err error
	if enabled {
		_, err = a.WriteTelemetryAudit()
	} else {
		err = a.ClearTelemetry()
	}
	// A failure does not retry every frame: the setting is remembered and
	// the next change tries again, so a read-only config never costs disk
	// work on each redraw.
	return err
}
