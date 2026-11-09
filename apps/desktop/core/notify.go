package core

import (
	"hiik/core/hc"
	"hiik/mods/api"
)

// SetNotifier installs the function that shows a notification. The UI passes
// a function that queues it for the next frame, so the notifier is called from
// the event goroutine and must not block or touch a window.
func (a *App) SetNotifier(fn func(api.Notification)) {
	a.mu.Lock()
	a.notifier = fn
	a.mu.Unlock()
}

// notify delivers n when a notifier is installed and settings allow its
// level. It reads the settings under the lock and then calls the notifier
// without it.
func (a *App) notify(n api.Notification) {
	a.mu.RLock()
	fn := a.notifier
	enabled, sound := true, a.settings.Sound
	switch n.Level {
	case api.LevelBlocked:
		enabled = a.settings.NotifyBlocked
	case api.LevelDone:
		enabled = a.settings.NotifyDone
	}
	a.mu.RUnlock()
	if fn == nil || !enabled {
		return
	}
	n.Sound = sound
	fn(n)
}

// announced is the last notification made for a terminal: its level and the
// turn it belonged to.
type announced struct {
	level string
	turn  uint64
}

// attention remembers what was already announced for each terminal, so one
// turn is announced once even when the core repeats a status event.
type attention struct {
	last map[string]announced
}

// newAttention starts from a snapshot. A terminal that is already blocked or
// done when the app first sees it is not news, so the snapshot seeds the map
// instead of notifying.
func newAttention(terms map[string]hc.Terminal) *attention {
	att := &attention{last: make(map[string]announced, len(terms))}
	att.replace(terms)
	return att
}

// replace seeds the map from a snapshot or a missed-events replacement.
func (a *attention) replace(terms map[string]hc.Terminal) {
	for k := range a.last {
		delete(a.last, k)
	}
	for _, t := range terms {
		a.seed(t)
	}
}

// forget drops a terminal that the core removed.
func (a *attention) forget(addr string) {
	delete(a.last, addr)
}

// seed records a terminal without announcing it. A new record whose status is
// not blocked or done clears any earlier entry for the same address, so the
// next block is news.
func (a *attention) seed(t hc.Terminal) {
	if level, ok := attentionLevel(t); ok {
		a.last[t.Address] = announced{level: level, turn: t.Turn}
		return
	}
	delete(a.last, t.Address)
}

// pending reports the level to announce for t, if its state is blocked or done
// and that level was not already announced for this turn.
func (a *attention) pending(t hc.Terminal) (string, bool) {
	level, ok := attentionLevel(t)
	if !ok {
		return "", false
	}
	now := announced{level: level, turn: t.Turn}
	if a.last[t.Address] == now {
		return "", false
	}
	a.last[t.Address] = now
	return level, true
}

// attentionLevel returns the notification level of a terminal's status.
func attentionLevel(t hc.Terminal) (string, bool) {
	if t.Status == nil {
		return "", false
	}
	switch t.Status.State {
	case api.LevelBlocked:
		return api.LevelBlocked, true
	case api.LevelDone:
		return api.LevelDone, true
	}
	return "", false
}

// observeEvent announces a terminal that turned blocked or done. It reads the
// record after applyEvent, so a replaced stream seeds state instead of
// notifying.
func (a *App) observeEvent(att *attention, terms map[string]hc.Terminal, ev hc.Event) {
	switch ev.Type {
	case "missed":
		att.replace(terms)
	case "removed":
		att.forget(ev.Terminal)
	case "created", "restarted":
		att.seed(terms[ev.Terminal])
	case "status":
		t := terms[ev.Terminal]
		level, ok := att.pending(t)
		if !ok {
			return
		}
		a.unsettleAttention(terms, ev.Terminal)
		a.notify(notificationFor(level, t))
	}
}

// unsettleAttention brings a settled tab back when an agent in it needs a
// person. A sub-agent reports to its parent, so the parent's tab comes back.
func (a *App) unsettleAttention(terms map[string]hc.Terminal, address string) {
	root := address
	seen := map[string]bool{root: true}
	for {
		t, ok := terms[root]
		if !ok {
			break
		}
		parent := terminalParent(t)
		if parent == "" || seen[parent] {
			break
		}
		seen[parent] = true
		root = parent
	}

	a.mu.Lock()
	changed := false
	if paneID, ok := a.model.paneByTerminal(root); ok {
		changed = a.model.unsettlePane(paneID)
	}
	if !changed {
		if paneID, ok := a.model.paneByTerminal(address); ok {
			changed = a.model.unsettlePane(paneID)
		}
	}
	a.mu.Unlock()
	if changed {
		a.persistState()
	}
}

// notificationFor builds the message for one attention event.
func notificationFor(level string, t hc.Terminal) api.Notification {
	name := t.Title
	if name == "" {
		name = t.Address
	}
	n := api.Notification{Level: level, Terminal: t.Address}
	switch level {
	case api.LevelBlocked:
		n.Title = name + " needs you"
		n.Body = "Blocked, waiting for input"
	case api.LevelDone:
		n.Title = name + " finished"
		n.Body = "Turn complete"
	default:
		n.Title = name
	}
	return n
}
