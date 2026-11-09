package core

import (
	_ "embed"
	"strings"
)

// Version is the app version. scripts/check-version keeps it and
// apps/desktop/mygo.json in step.
const Version = "0.1.0"

//go:embed release_notes.md
var releaseNotes string

// Version returns the app version.
func (a *App) Version() string { return Version }

// CoreVersion returns the connected core's version, or "".
func (a *App) CoreVersion() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.hello.Version
}

// ReleaseNotes returns the bundled notes for this version.
func (a *App) ReleaseNotes() string {
	return strings.TrimSpace(releaseNotes)
}
