// Package core is the app's model and the only place that talks to herdr-core.
// It has no mygo UI imports beyond the types in mods/api.
package core

import (
	"context"
	"fmt"
	"sync"

	"hiik/core/hc"
	"hiik/core/store"
	"hiik/mods/api"
)

// Connection is the state of the link to herdr-core.
type Connection int

const (
	// Connecting means the app has not reached a core yet.
	Connecting Connection = iota
	// Connected means calls and subscriptions are live.
	Connected
	// Disconnected means the last link was lost and a retry is pending.
	Disconnected
)

// App is hiik's state. Its methods are safe from any goroutine.
type App struct {
	mu sync.RWMutex

	core     hc.Client
	model    model
	machines []api.Machine
	settings store.Settings
	store    *store.Store

	// OS and architecture detected through machine.exec, keyed by machine
	// name. probing keeps one probe per machine in flight.
	machineOS      map[string]machineOS
	machineProbing map[string]bool

	cmds []*cmd
	byID map[string]*cmd

	conn       Connection
	lastErr    string
	hello      hc.Hello
	overlay    string
	notifier   func(api.Notification)
	invalidate func()

	// Hooks the shell installs. They are nil in tests and headless runs.
	confirmer  func(title, message string, run func())
	scroller   func(address string, lines, pages float32, to string) bool
	paster     func(address string) error
	rightClick func(address, to string)
	windowDo   func(action string)
	appDo      func(action string)
	textPaster func(address string) error
	copier     func(address string) error

	runCtx    context.Context
	runCancel context.CancelFunc
}

// New returns an app with no core connection yet, loading state and
// settings from the config directory. A broken store never prevents
// startup: the app uses defaults and keeps running.
func New() *App {
	st, err := store.Open()
	if err != nil {
		st = nil
	}
	return newApp(st)
}

func newApp(st *store.Store) *App {
	a := &App{
		store:          st,
		conn:           Connecting,
		settings:       store.DefaultSettings(),
		machineOS:      map[string]machineOS{},
		machineProbing: map[string]bool{},
	}
	if st != nil {
		state, err := st.LoadState()
		if err == nil {
			a.model = modelFromState(state)
		}
		settings, err := st.LoadSettings()
		if err == nil {
			a.settings = settings
		}
	}
	a.buildCommands()
	return a
}

// Spaces returns a snapshot of the spaces. The caller may mutate the
// returned nodes without touching the app.
func (a *App) Spaces() []api.Space {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return cloneSpaces(a.model.spaces)
}

// Agents returns a snapshot of the agents.
func (a *App) Agents() []api.Agent {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return append([]api.Agent(nil), a.model.agents...)
}

// ActiveSpace returns the selected space's ID, or "".
func (a *App) ActiveSpace() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.model.activeSpace
}

// TabSettled reports whether a tab is put away. Empty IDs mean the selected
// space and its shown tab.
func (a *App) TabSettled(spaceID, tabID string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	si := a.model.spaceIndex(spaceID)
	if si < 0 {
		si = a.model.spaceIndex(a.model.activeSpace)
	}
	if si < 0 {
		return false
	}
	ti := tabIndex(&a.model.spaces[si], tabID)
	if ti < 0 {
		return false
	}
	return a.model.settled[a.model.spaces[si].Tabs[ti].ID]
}

// ActiveAgent returns the selected agent's address, or "".
func (a *App) ActiveAgent() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.model.activeAgentAddress()
}

// State returns the state of the link to herdr-core.
func (a *App) State() Connection {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.conn
}

// LastError returns why the last connection failed, or "".
func (a *App) LastError() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.lastErr
}

// Connection returns the connection state as a word for mods.
func (a *App) Connection() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	switch a.conn {
	case Connected:
		return "connected"
	case Disconnected:
		return "disconnected"
	default:
		return "connecting"
	}
}

// SetConnection records the state of the link, for the core client.
func (a *App) SetConnection(c Connection, err string) {
	a.mu.Lock()
	a.conn = c
	a.lastErr = err
	a.mu.Unlock()
	a.invalidateUI()
}

// SetInvalidator installs the shell's redraw hook. Core updates from its
// connection goroutine wake the window through it, so status changes show
// without waiting for terminal output.
func (a *App) SetInvalidator(fn func()) {
	a.mu.Lock()
	a.invalidate = fn
	a.mu.Unlock()
}

// Invalidate asks the shell to draw another frame. Watchers use it from
// their goroutines; it is safe when no window is open.
func (a *App) Invalidate() { a.invalidateUI() }

func (a *App) invalidateUI() {
	a.mu.RLock()
	fn := a.invalidate
	a.mu.RUnlock()
	if fn != nil {
		fn()
	}
}

// SetAgents replaces the agent list with a snapshot from the core.
func (a *App) SetAgents(list []api.Agent) {
	a.mu.Lock()
	a.model.setAgents(list)
	a.mu.Unlock()
	a.invalidateUI()
}

// Settings returns a copy of the settable toggles for mods and panels,
// including the look settings and the keymap overrides.
func (a *App) Settings() map[string]any {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.settingsView()
}

// Config returns a copy of the stored settings.
func (a *App) Config() store.Settings {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.settings
}

// SetSettings replaces the settings, repairs bad values and saves them.
func (a *App) SetSettings(s store.Settings) {
	s.Normalize()
	a.mu.Lock()
	a.settings = s
	a.mu.Unlock()
	a.persistSettings()
}

// Overlay returns the overlay a command asked to open, or "".
func (a *App) Overlay() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.overlay
}

// SetOverlay records or clears the requested overlay.
func (a *App) SetOverlay(name string) {
	a.mu.Lock()
	a.overlay = name
	a.mu.Unlock()
}

// SetRatio records a split's ratio, for dragging a divider.
func (a *App) SetRatio(paneID string, ratio float32) error {
	a.mu.Lock()
	err := a.model.setRatio(paneID, ratio)
	a.mu.Unlock()
	if err != nil {
		return err
	}
	a.persistState()
	return nil
}

// Run runs a command by ID for a person: a close that ends terminals asks
// through the installed confirmer first. Commands that start terminals need a
// core; layout-only commands work while disconnected.
func (a *App) Run(name string, args map[string]any) error {
	return a.runCommand(name, args, true)
}

// RunAgent runs a command for an agent or another non-interactive caller.
// A close that ends terminals fails instead of asking; pass confirmed: true
// to let it run.
func (a *App) RunAgent(name string, args map[string]any) error {
	return a.runCommand(name, args, false)
}

func (a *App) runCommand(name string, args map[string]any, person bool) error {
	if name == "" {
		return fmt.Errorf("core: empty command")
	}
	a.mu.RLock()
	c := a.byID[name]
	a.mu.RUnlock()
	if c != nil {
		return c.executeAs(a, args, person)
	}
	for _, other := range api.Commands() {
		if other.ID == name {
			return other.Run(a, args)
		}
	}
	return fmt.Errorf("core: unknown command %q", name)
}

// coreClient returns the connected client or a clear error.
func (a *App) coreClient() (hc.Client, error) {
	a.mu.RLock()
	c := a.core
	a.mu.RUnlock()
	if c == nil {
		return nil, errNoCore
	}
	return c, nil
}

// persistState schedules a debounced write of the layout.
func (a *App) persistState() {
	a.mu.RLock()
	st := a.store
	state := a.model.snapshot()
	a.mu.RUnlock()
	if st != nil {
		st.SaveState(state)
	}
}

// persistSettings schedules a debounced write of the settings.
func (a *App) persistSettings() {
	a.mu.RLock()
	st := a.store
	settings := a.settings
	a.mu.RUnlock()
	if st != nil {
		st.SaveSettings(settings)
	}
}

// compile-time check that App is the app mods see.
var _ api.App = (*App)(nil)
