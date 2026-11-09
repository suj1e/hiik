package core

import (
	"encoding/json"
	"testing"

	"hiik/core/hc"
	"hiik/core/store"
	"hiik/mods/api"
)

// statusReport builds the core's status event for a terminal.
func statusReport(addr string, turn uint64, state string) hc.Event {
	data, err := json.Marshal(map[string]any{
		"status": map[string]string{"state": state},
		"turn":   turn,
	})
	if err != nil {
		panic(err)
	}
	return hc.Event{Type: "status", Terminal: addr, Data: data}
}

func TestAttentionNotifiesOncePerTurn(t *testing.T) {
	a := testApp()
	var got []api.Notification
	a.SetNotifier(func(n api.Notification) { got = append(got, n) })

	terms := map[string]hc.Terminal{
		"t1": {
			Address: "t1",
			State:   "live",
			Title:   "claude",
			Turn:    1,
			Status:  &hc.Status{State: "working"},
		},
	}
	att := newAttention(terms)
	observe := func(ev hc.Event) {
		applyEvent(terms, ev)
		a.observeEvent(att, terms, ev)
	}

	observe(statusReport("t1", 1, "working"))
	if len(got) != 0 {
		t.Fatalf("a repeat of the snapshot announced %d notifications", len(got))
	}

	observe(statusReport("t1", 1, "blocked"))
	if len(got) != 1 {
		t.Fatalf("blocked announced %d notifications, want 1", len(got))
	}
	if got[0].Level != api.LevelBlocked || got[0].Terminal != "t1" || !got[0].Sound {
		t.Fatalf("blocked notification = %+v", got[0])
	}

	observe(statusReport("t1", 1, "blocked"))
	if len(got) != 1 {
		t.Fatalf("a repeated blocked status announced again: %d", len(got))
	}

	// Working and blocking again in the same turn is still one turn.
	observe(statusReport("t1", 1, "working"))
	observe(statusReport("t1", 1, "blocked"))
	if len(got) != 1 {
		t.Fatalf("the same turn announced twice: %d", len(got))
	}

	// A new turn may block and then finish.
	observe(statusReport("t1", 2, "working"))
	observe(statusReport("t1", 2, "blocked"))
	if len(got) != 2 {
		t.Fatalf("turn 2 block announced %d notifications, want 2", len(got))
	}
	observe(statusReport("t1", 2, "done"))
	if len(got) != 3 || got[2].Level != api.LevelDone {
		t.Fatalf("done notification = %+v", got)
	}
	observe(statusReport("t1", 2, "done"))
	if len(got) != 3 {
		t.Fatalf("a repeated done status announced again: %d", len(got))
	}
}

func TestAttentionSkipsSnapshot(t *testing.T) {
	a := testApp()
	var got []api.Notification
	a.SetNotifier(func(n api.Notification) { got = append(got, n) })

	terms := map[string]hc.Terminal{
		"t1": {Address: "t1", Turn: 4, Status: &hc.Status{State: "blocked"}},
	}
	att := newAttention(terms)
	ev := statusReport("t1", 4, "blocked")
	applyEvent(terms, ev)
	a.observeEvent(att, terms, ev)
	if len(got) != 0 {
		t.Fatalf("the initial snapshot announced %d notifications", len(got))
	}

	ev = statusReport("t1", 5, "blocked")
	applyEvent(terms, ev)
	a.observeEvent(att, terms, ev)
	if len(got) != 1 {
		t.Fatalf("a blocked turn after the snapshot announced %d, want 1", len(got))
	}
}

func TestNotifyRespectsSettings(t *testing.T) {
	a := testApp()
	var got []api.Notification
	a.SetNotifier(func(n api.Notification) { got = append(got, n) })

	s := store.DefaultSettings()
	s.NotifyBlocked = false
	s.NotifyDone = false
	s.Sound = false
	a.SetSettings(s)

	terms := map[string]hc.Terminal{
		"t1": {Address: "t1", Turn: 1, Status: &hc.Status{State: "working"}},
	}
	att := newAttention(terms)
	observe := func(ev hc.Event) {
		applyEvent(terms, ev)
		a.observeEvent(att, terms, ev)
	}

	observe(statusReport("t1", 1, "blocked"))
	observe(statusReport("t1", 1, "done"))
	if len(got) != 0 {
		t.Fatalf("disabled levels announced %d notifications", len(got))
	}

	s = store.DefaultSettings()
	s.Sound = false
	a.SetSettings(s)
	observe(statusReport("t1", 2, "blocked"))
	if len(got) != 1 {
		t.Fatalf("enabled blocked announced %d notifications, want 1", len(got))
	}
	if got[0].Sound {
		t.Fatal("a disabled sound still asked for a tone")
	}
}
