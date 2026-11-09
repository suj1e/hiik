package core

import (
	"hiik/core/hc"
	"hiik/mods/api"
)

// Terminal labels core reads and writes. The core owns the durable copy.
const (
	labelSpace      = "hiik.space"        // the space a kept terminal belongs to
	labelTab        = "hiik.tab"          // the tab a kept terminal belongs to
	labelRightClick = "herdr.right-click" // "program" or "menu"
)

// The shell installs these hooks at startup. Each is optional so core can be
// tested and embedded without a window.

// SetConfirmer installs the function that asks a person before a close ends
// terminals. run performs the already-confirmed command; core calls it only
// after the person agrees.
func (a *App) SetConfirmer(fn func(title, message string, run func())) {
	a.mu.Lock()
	a.confirmer = fn
	a.mu.Unlock()
}

// SetTerminalScroller installs the function that scrolls one terminal's view
// of its history. It reports false when that terminal is not attached to a
// pane on this screen.
func (a *App) SetTerminalScroller(fn func(address string, lines, pages float32, to string) bool) {
	a.mu.Lock()
	a.scroller = fn
	a.mu.Unlock()
}

// SetImagePaster installs the function that pastes the clipboard's image into
// a terminal.
func (a *App) SetImagePaster(fn func(address string) error) {
	a.mu.Lock()
	a.paster = fn
	a.mu.Unlock()
}

// SetRightClickPolicy installs the function that tells a live terminal view
// where right-clicks go ("program" or "menu").
func (a *App) SetRightClickPolicy(fn func(address, to string)) {
	a.mu.Lock()
	a.rightClick = fn
	a.mu.Unlock()
}

// setTerminalRecords replaces the app's terminal snapshot. The connect
// goroutine calls it after every snapshot and event, so the UI reads the same
// titles, folders and exit codes the core reported.
func (a *App) setTerminalRecords(terms map[string]hc.Terminal) {
	a.mu.Lock()
	a.model.setTerminalRecords(terms)
	a.mu.Unlock()
	a.invalidateUI()
}

// restoreRemovedTerminal gives panes that borrowed a removed terminal their
// own terminal back.
func (a *App) restoreRemovedTerminal(address string) {
	a.mu.Lock()
	changed := a.model.restoreBorrowed(address)
	a.mu.Unlock()
	if changed {
		a.persistState()
	}
}

// TerminalInfo returns the live state of a core terminal. A terminal the core
// no longer reports is not found.
func (a *App) TerminalInfo(address string) (api.TerminalInfo, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	t, ok := a.model.terminal(address)
	if !ok {
		return api.TerminalInfo{}, false
	}
	return terminalInfo(t), true
}

// TerminalInfos returns a copy of every terminal record, keyed by address.
func (a *App) TerminalInfos() map[string]api.TerminalInfo {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make(map[string]api.TerminalInfo, len(a.model.terminals))
	for address, t := range a.model.terminals {
		out[address] = terminalInfo(t)
	}
	return out
}

// terminalInfo converts one core record to the UI's view of it.
func terminalInfo(t hc.Terminal) api.TerminalInfo {
	info := api.TerminalInfo{
		Address:  t.Address,
		Title:    t.Title,
		Cwd:      t.Cwd,
		State:    t.State,
		SeenTurn: agentSeenTurn(t.Labels),
		Pinned:   agentPinned(t.Labels),
	}
	// The foreground program's folder is where the person is now: a shell
	// launched in home may have cd'd into a checkout. The record's own
	// folder is only a fallback.
	if t.Foreground != nil && t.Foreground.Cwd != "" {
		info.Cwd = t.Foreground.Cwd
	}
	if info.State == "" {
		info.State = "live"
	}
	if t.ExitCode != nil {
		code := *t.ExitCode
		info.ExitCode = &code
	}
	return info
}

// terminalKnown reports whether a terminal is running or was shown in a pane.
func (m *model) terminalKnown(address string) bool {
	if _, ok := m.terminal(address); ok {
		return true
	}
	_, ok := m.paneByTerminal(address)
	return ok
}
