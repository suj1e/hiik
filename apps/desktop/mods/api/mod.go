package api

import (
	"errors"
	"fmt"
	"sync"
)

// Command IDs for showing a mod panel as a window or a floating region.
// The shell owns the visuals; the commands only name the panel and props.
const (
	CmdModWindow = "mod.window"
	CmdModFloat  = "mod.float"
)

// PanelByID returns the registered panel with id. Panel IDs are global,
// so a caller never needs the mod that owns one.
func PanelByID(id string) (Panel, bool) {
	p, ok := Panels()[id]
	return p, ok
}

// FloatEdgeRight is where a float sits when a caller does not choose an
// edge.
const FloatEdgeRight = "right"

// ValidFloatEdge reports whether edge names a side a float may sit on. An
// empty edge means the default, FloatEdgeRight.
func ValidFloatEdge(edge string) bool {
	switch edge {
	case "", "left", "right", "top", "bottom":
		return true
	}
	return false
}

// ErrNoModSurface reports that no shell is open to show a mod panel. It is
// not an error in a mod: it only means the host is not there yet.
var ErrNoModSurface = errors.New("no window is open to show a mod panel")

// ModSurface is the part of a shell that shows mod panels outside a pane:
// a window over the shell, or a floating region on one edge.
//
// The shell installs it with RegisterModSurface from its init. A mod opens
// a window with OpenModWindow or the mod.window command.
type ModSurface interface {
	OpenWindow(panelID string, props map[string]any)
	CloseWindow(panelID string)
	OpenFloat(panelID, edge string, props map[string]any)
	CloseFloat(panelID string)
}

var (
	surfaceMu sync.RWMutex
	surface   ModSurface
)

// RegisterModSurface installs the shell side of mod windows and floats. A
// second call replaces the first; a test host can stand in for the shell.
func RegisterModSurface(s ModSurface) {
	surfaceMu.Lock()
	surface = s
	surfaceMu.Unlock()
}

// CurrentModSurface returns what RegisterModSurface last installed, or nil.
func CurrentModSurface() ModSurface {
	surfaceMu.RLock()
	defer surfaceMu.RUnlock()
	return surface
}

// OpenModWindow asks the shell to show panelID as a window with props. It
// returns an error for an unregistered panel or when no shell is open, so
// a command surfaces both instead of failing silently.
func OpenModWindow(panelID string, props map[string]any) error {
	if _, ok := PanelByID(panelID); !ok {
		return fmt.Errorf("unknown mod panel %q", panelID)
	}
	s := CurrentModSurface()
	if s == nil {
		return ErrNoModSurface
	}
	s.OpenWindow(panelID, props)
	return nil
}

// CloseModWindow closes the window showing panelID, if it is open.
func CloseModWindow(panelID string) error {
	if _, ok := PanelByID(panelID); !ok {
		return fmt.Errorf("unknown mod panel %q", panelID)
	}
	if s := CurrentModSurface(); s != nil {
		s.CloseWindow(panelID)
	}
	return nil
}

// OpenModFloat asks the shell to show panelID in a floating region on edge
// with props. An empty edge means FloatEdgeRight.
func OpenModFloat(panelID, edge string, props map[string]any) error {
	if _, ok := PanelByID(panelID); !ok {
		return fmt.Errorf("unknown mod panel %q", panelID)
	}
	if !ValidFloatEdge(edge) {
		return fmt.Errorf("unknown float edge %q", edge)
	}
	s := CurrentModSurface()
	if s == nil {
		return ErrNoModSurface
	}
	s.OpenFloat(panelID, edge, props)
	return nil
}

// CloseModFloat closes the floating region showing panelID, if it is open.
func CloseModFloat(panelID string) error {
	if _, ok := PanelByID(panelID); !ok {
		return fmt.Errorf("unknown mod panel %q", panelID)
	}
	if s := CurrentModSurface(); s != nil {
		s.CloseFloat(panelID)
	}
	return nil
}
