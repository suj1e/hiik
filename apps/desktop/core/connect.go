package core

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"hiik/core/hc"
	"hiik/mods/api"
)

const (
	minBackoff = time.Second
	maxBackoff = 30 * time.Second
)

// connect dials the core, applies its snapshot, consumes events and
// reconnects until the context is canceled or Close runs.
func (a *App) connect(parent context.Context) error {
	ctx, cancel := context.WithCancel(parent)
	a.mu.Lock()
	a.runCtx, a.runCancel = ctx, cancel
	a.mu.Unlock()
	defer cancel()

	backoff := minBackoff
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		cfg := a.Config()
		c, err := hc.Dial(ctx, hc.Options{
			Binary:  cfg.CorePath,
			Session: os.Getenv("HC_SESSION"),
		})
		if err != nil {
			a.SetConnection(Disconnected, err.Error())
			if !sleep(ctx, backoff) {
				return ctx.Err()
			}
			backoff = nextBackoff(backoff)
			continue
		}

		sub, err := c.Subscribe(ctx)
		if err != nil {
			_ = c.Close()
			a.SetConnection(Disconnected, err.Error())
			if !sleep(ctx, backoff) {
				return ctx.Err()
			}
			backoff = nextBackoff(backoff)
			continue
		}

		a.SetCore(c)
		_ = a.RefreshMachines(ctx)
		go a.RefreshPrograms(ctx)
		a.mu.Lock()
		a.hello = c.Hello()
		a.mu.Unlock()
		a.SetConnection(Connected, "")
		backoff = minBackoff

		terms := make(map[string]hc.Terminal, len(sub.Snapshot.Terminals))
		applySnapshot(terms, sub)
		a.setTerminalRecords(terms)
		a.adoptTerminals(sub.Snapshot.Terminals)
		a.SetAgents(agentsFrom(terms, a.Spaces()))
		a.consume(ctx, c, sub, terms)

		sub.Close()
		_ = c.Close()
		a.SetCore(nil)
		a.SetConnection(Disconnected, "core connection lost")
		if !sleep(ctx, minBackoff) {
			return ctx.Err()
		}
	}
}

// consume applies events until the stream or the core ends.
func (a *App) consume(ctx context.Context, c hc.Client, sub *hc.Subscription, terms map[string]hc.Terminal) {
	att := newAttention(terms)
	for {
		select {
		case ev, ok := <-sub.Events:
			if !ok {
				return
			}
			changed := applyEvent(terms, ev)
			a.setTerminalRecords(terms)
			a.adoptTerminals(sub.Snapshot.Terminals)
			if changed {
				a.SetAgents(agentsFrom(terms, a.Spaces()))
			}
			if ev.Type == "removed" {
				a.restoreRemovedTerminal(ev.Terminal)
			}
			a.observeEvent(att, terms, ev)
		case <-c.Lost():
			return
		case <-ctx.Done():
			return
		}
	}
}

// applySnapshot replaces the terminal map.
func applySnapshot(terms map[string]hc.Terminal, sub *hc.Subscription) {
	for k := range terms {
		delete(terms, k)
	}
	for _, t := range sub.Snapshot.Terminals {
		terms[t.Address] = t
	}
}

// applyEvent updates the terminal map. It reports whether the agent view
// may have changed.
func applyEvent(terms map[string]hc.Terminal, ev hc.Event) bool {
	addr := ev.Terminal
	switch ev.Type {
	case "missed":
		var snap struct {
			Terminals []hc.Terminal `json:"terminals"`
		}
		if json.Unmarshal(ev.Data, &snap) != nil {
			return false
		}
		for k := range terms {
			delete(terms, k)
		}
		for _, t := range snap.Terminals {
			terms[t.Address] = t
		}
		return true
	case "created", "restarted":
		var t hc.Terminal
		if json.Unmarshal(ev.Data, &t) != nil {
			return false
		}
		if t.Address == "" {
			t.Address = addr
		}
		terms[t.Address] = t
		return true
	case "removed":
		delete(terms, addr)
		return true
	case "labels":
		var d struct {
			Record *hc.Terminal      `json:"record"`
			Labels map[string]string `json:"labels"`
		}
		if json.Unmarshal(ev.Data, &d) != nil {
			return false
		}
		if d.Record != nil {
			terms[addr] = *d.Record
		} else {
			t := terms[addr]
			t.Labels = d.Labels
			terms[addr] = t
		}
		return true
	case "status":
		var d struct {
			Status *hc.Status `json:"status"`
			Turn   uint64     `json:"turn"`
		}
		if json.Unmarshal(ev.Data, &d) != nil {
			return false
		}
		t := terms[addr]
		t.Status = d.Status
		t.Turn = d.Turn
		terms[addr] = t
		return true
	case "turn":
		var d struct {
			Turn uint64 `json:"turn"`
		}
		if json.Unmarshal(ev.Data, &d) != nil {
			return false
		}
		t := terms[addr]
		t.Turn = d.Turn
		terms[addr] = t
		return true
	case "title":
		var d struct {
			Title string `json:"title"`
		}
		if json.Unmarshal(ev.Data, &d) != nil {
			return false
		}
		t := terms[addr]
		t.Title = d.Title
		terms[addr] = t
		return true
	case "foreground":
		var f hc.Foreground
		if json.Unmarshal(ev.Data, &f) != nil {
			return false
		}
		t := terms[addr]
		t.Foreground = &f
		terms[addr] = t
		return true
	case "finished", "lost":
		var d struct {
			ExitCode *int `json:"exit_code"`
		}
		_ = json.Unmarshal(ev.Data, &d)
		t := terms[addr]
		if ev.Type == "finished" {
			t.State = "finished"
			t.ExitCode = d.ExitCode
		} else {
			t.State = "live"
			t.ExitCode = nil
		}
		terms[addr] = t
		return true
	case "keep", "size":
		// Facts that do not change the agent list.
		return false
	default:
		return false
	}
}

// agentsFrom turns live terminals into the agent list. A terminal is an
// agent when its program reports status or the core knows its agent.
func agentsFrom(terms map[string]hc.Terminal, spaces []api.Space) []api.Agent {
	where := make(map[string][2]string)
	for _, sp := range spaces {
		for _, tab := range sp.Tabs {
			walkNode(tab.Root, func(n *api.Node) {
				if n.Terminal != "" {
					where[n.Terminal] = [2]string{sp.ID, tab.ID}
				}
			})
		}
	}

	out := make([]api.Agent, 0, len(terms))
	for _, t := range terms {
		if t.State == "finished" {
			continue
		}
		if !isAgentTerminal(terms, t) {
			continue
		}
		parent := terminalParent(t)
		seenTurn := agentSeenTurn(t.Labels)
		ag := api.Agent{
			Address:  t.Address,
			App:      t.Agent,
			Title:    t.Title,
			Pinned:   agentPinned(t.Labels),
			Turn:     t.Turn,
			Parent:   parent,
			SeenTurn: seenTurn,
		}
		if t.Status != nil {
			ag.Status = t.Status.State
			ag.Seen = ag.Status != "done" && ag.Status != "blocked"
		}
		if seenTurn != 0 && t.Turn <= seenTurn {
			// Looking at the last turn consumed its "done"; the label makes
			// that survive a reconnect as well as this session.
			ag.Seen = true
			if ag.Status == "done" {
				ag.Status = "idle"
			}
		}
		if at, ok := where[t.Address]; ok {
			ag.Space, ag.Tab = at[0], at[1]
		}
		if v := t.Labels["hiik.space"]; v != "" {
			ag.Space = v
		}
		if v := t.Labels["hiik.tab"]; v != "" {
			ag.Tab = v
		}
		out = append(out, ag)
	}
	return orderAgents(out)
}

// terminalParent is the address of the terminal that started t, from the
// core's parent field or a label some programs set.
func terminalParent(t hc.Terminal) string {
	if t.Parent != "" {
		return t.Parent
	}
	if p := t.Labels["herdr.parent"]; p != "" {
		return p
	}
	return t.Labels["hiik.parent"]
}

// isAgentTerminal reports whether t reports status, was started by an agent
// template, or is a descendant of a terminal that does. That last case is
// how a program an agent ran with `hc run` shows as a sub-agent.
func isAgentTerminal(terms map[string]hc.Terminal, t hc.Terminal) bool {
	if t.Status != nil && t.Status.State != "" {
		return true
	}
	if t.Agent != "" {
		return true
	}
	seen := map[string]bool{t.Address: true}
	for parent := terminalParent(t); parent != ""; {
		p, ok := terms[parent]
		if !ok || seen[parent] {
			return false
		}
		seen[parent] = true
		if p.Status != nil && p.Status.State != "" {
			return true
		}
		if p.Agent != "" {
			return true
		}
		parent = terminalParent(p)
	}
	return false
}

func priority(status string) int {
	switch status {
	case "blocked", "error":
		return 4
	case "done":
		return 3
	case "working":
		return 2
	case "idle":
		return 1
	default:
		return 0
	}
}

func nextBackoff(d time.Duration) time.Duration {
	d *= 2
	if d > maxBackoff {
		return maxBackoff
	}
	return d
}

func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
