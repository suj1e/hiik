package ui

import (
	"fmt"
	"runtime"
	"sync"

	"github.com/egoist/mygo"
	mui "github.com/egoist/mygo/ui"
)

// The window feature: another window on the same app model, a title-bar
// button that opens it, and the shell-side confirmation for updating core.
//
// Windows share core.App (spaces, tabs, panes, agents). Each has its own
// Shell, so view state (sidebar width, focus, zoom, overlays through the
// shell), terminal attachments and window geometry are independent. Closing
// one window detaches only its terminals; the core keeps running them for
// the other windows.

var (
	windowMu   sync.Mutex
	windowSeq  int
	windowStep = 26
)

func init() {
	newWindowHook = func(s *Shell) { s.openAnotherWindow() }
	registerShellHook(runWindowHook)
}

// runWindowHook runs once per frame: it consumes window requests a command
// queued, binds the new-window shortcut and reconciles the local telemetry
// audit with the setting.
func runWindowHook(s *Shell, c *mui.Context) {
	if s == nil || s.app == nil {
		return
	}
	s.consumeWindowRequests()
	s.registerWindowShortcuts(c)
	syncTelemetry(s)
}

// consumeWindowRequests handles the overlays that are really requests to
// this window. The core owns commands but not windows, so a command records
// its intent in the overlay slot and the shell acts on it here.
func (s *Shell) consumeWindowRequests() {
	switch s.app.Overlay() {
	case "new_window":
		s.app.SetOverlay("")
		s.openAnotherWindow()
	case "update_core":
		s.app.SetOverlay("")
		s.askConfirm("Update core",
			"Run `hc update`? Terminals keep running and are not restarted.",
			func() {
				s.run("app.update_core", map[string]any{"confirm": true})
			})
	}
}

// registerWindowShortcuts binds new window and floating inspector. It uses
// the same keymap overrides as the rest of the app.
func (s *Shell) registerWindowShortcuts(c *mui.Context) {
	overrides := settingKeys(s.app.Settings())
	actions := []shortcut{
		{action: "app.new_window", chord: "Mod+Shift+N", run: func(sh *Shell) {
			sh.openAnotherWindow()
		}},
		{action: "app.floating_panel", chord: "Mod+Shift+I", run: func(sh *Shell) {
			toggleFloat(sh)
		}},
	}
	for _, sc := range actions {
		mods, key, ok, overridden := resolveShortcut(sc, overrides)
		if !ok {
			if overridden {
				s.toast = fmt.Sprintf("keymap: %q is not a key for %s", overrides[sc.action], sc.action)
			}
			continue
		}
		c.OnWindowShortcut(mods, key, func() { sc.run(s) })
	}
}

// openAnotherWindow opens a new window on the same app model without
// blocking the frame that asked for it. Each window gets its own Shell and
// its own window state key, so geometry and attachments do not collide.
func (s *Shell) openAnotherWindow() {
	if s == nil || s.app == nil {
		return
	}
	go openWindow(s.app)
}

// openWindow creates the shell and native window. It may run off the main
// thread; mygo creates the window on the main thread.
func openWindow(app App) {
	windowMu.Lock()
	windowSeq++
	n := windowSeq
	windowMu.Unlock()

	ns := New(app)
	offset := (n % 8) * windowStep
	opts := mygo.WindowOptions{
		Title:     "hiik",
		Width:     1180,
		Height:    760,
		X:         40 + offset,
		Y:         40 + offset,
		MinWidth:  820,
		MinHeight: 520,
		StateKey:  fmt.Sprintf("window-%d", n),
		Content:   mui.View(ns.View),
	}
	if runtime.GOOS == "darwin" {
		opts.TitleBarStyle = mygo.TitleBarHidden
	}
	win := mygo.NewWindow(opts)
	ns.win = win
	win.OnClosed(ns.Close)
}

// telemetrySyncer is the part of the app that reconciles local audits.
type telemetrySyncer interface {
	SyncTelemetry() error
}

// syncTelemetry writes or removes the local audit when the setting changed.
// It is opt-in and never sends anything.
func syncTelemetry(s *Shell) {
	if ts, ok := s.app.(telemetrySyncer); ok {
		_ = ts.SyncTelemetry()
	}
}
