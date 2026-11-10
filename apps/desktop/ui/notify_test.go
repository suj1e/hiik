package ui

import (
	"testing"

	mui "github.com/egoist/mygo/ui"

	"hiik/mods/api"
)

func TestNotifyQueuesAndDrains(t *testing.T) {
	pending.mu.Lock()
	pending.list = nil
	pending.mu.Unlock()

	DrainNotifications(nil, false) // no frame yet: nothing to do, no panic

	Notify(api.Notification{})
	pending.mu.Lock()
	queued := len(pending.list)
	pending.mu.Unlock()
	if queued != 0 {
		t.Fatalf("an empty notification queued %d items", queued)
	}

	total := toastBatch + 2
	for range total {
		Notify(api.Notification{
			Title: "claude",
			Body:  "blocked",
			Level: api.LevelBlocked,
		})
	}

	drained := false
	remaining := -1
	tt := mui.NewTester(func(c *mui.Context) {
		if drained {
			return
		}
		drained = true
		DrainNotifications(c, false)
		pending.mu.Lock()
		remaining = len(pending.list)
		pending.mu.Unlock()
	}, 480, 320)
	if !drained {
		t.Fatal("the first frame never drained the queue")
	}
	if remaining != total-toastBatch {
		t.Fatalf("after one frame the queue has %d, want %d", remaining, total-toastBatch)
	}
	if !tt.HasText("claude") {
		t.Fatalf("the toast is missing; texts = %q", tt.Texts())
	}

	drained = false
	tt.Frame()
	if !drained {
		t.Fatal("the next frame never drained the queue")
	}
	if remaining != 0 {
		t.Fatalf("after two frames the queue has %d, want 0", remaining)
	}
}
