package core

import (
	"context"

	"hiik/core/hc"
	"hiik/mods/api"
)

// Start adopts a core installed with SetCore and records its handshake.
// It never dials: Connect does that, so tests and embedders choose their
// own core. Without a client, the link stays "connecting".
func (a *App) Start(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	a.mu.RLock()
	c := a.core
	a.mu.RUnlock()
	if c == nil {
		a.SetConnection(Connecting, "")
		return nil
	}
	a.mu.Lock()
	a.hello = c.Hello()
	a.conn = Connected
	a.lastErr = ""
	a.mu.Unlock()
	return nil
}

// Connect dials herdr-core, applies its snapshot and events, and
// reconnects when the link drops. It blocks until ctx is canceled or Close
// is called. The desktop app calls it once on a goroutine.
func (a *App) Connect(ctx context.Context) error {
	return a.connect(ctx)
}

// Close detaches from the core, closes the client and flushes the store.
func (a *App) Close() {
	a.mu.Lock()
	cancel := a.runCancel
	a.runCancel = nil
	a.runCtx = nil
	c := a.core
	st := a.store
	a.core = nil
	a.store = nil
	a.conn = Disconnected
	a.lastErr = ""
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if c != nil {
		_ = c.Close()
	}
	if st != nil {
		_ = st.Close()
	}
}

// SetCore installs the core client. A nil client marks the link
// disconnected. The caller owns the client's lifecycle until Close.
func (a *App) SetCore(c hc.Client) {
	a.mu.Lock()
	a.core = c
	if c == nil {
		a.conn = Disconnected
	} else {
		a.conn = Connected
		a.lastErr = ""
	}
	a.mu.Unlock()
}

// Commands returns the command registry: built-ins first, then commands a
// mod registered that do not reuse a built-in ID.
func (a *App) Commands() []api.Command {
	a.mu.RLock()
	builtins := append([]*cmd(nil), a.cmds...)
	a.mu.RUnlock()

	out := make([]api.Command, 0, len(builtins)+len(api.Commands()))
	seen := make(map[string]bool, len(builtins))
	for _, c := range builtins {
		c := c
		seen[c.id] = true
		out = append(out, api.Command{
			ID:    c.id,
			Title: c.title,
			Keys:  c.keys,
			Run: func(_ api.App, args map[string]any) error {
				return c.execute(a, args)
			},
		})
	}
	for _, other := range api.Commands() {
		if seen[other.ID] {
			continue
		}
		seen[other.ID] = true
		out = append(out, other)
	}
	return out
}

// UIState returns the persisted view state.
func (a *App) UIState() api.UIState {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.model.ui
}

// SetUIState records the view state and saves it.
func (a *App) SetUIState(s api.UIState) {
	if s.SidebarW != 0 && s.SidebarW < 160 {
		s.SidebarW = 160
	}
	a.mu.Lock()
	if s.Zoomed != "" && !a.model.paneExists(s.Zoomed) {
		s.Zoomed = ""
	}
	a.model.ui = s
	a.mu.Unlock()
	a.persistState()
}

// Core returns the connected core client, or nil before Start.
func (a *App) Core() hc.Client {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.core
}
