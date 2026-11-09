package ui

import (
	"sync"

	mui "github.com/egoist/mygo/ui"

	"hiik/mods/api"
)

// toastBatch bounds how many notifications one frame shows; mygo stacks three
// toasts at most.
const toastBatch = 3

// pending holds notifications queued from the core's event goroutine until
// the shell drains them in a frame.
var pending struct {
	mu   sync.Mutex
	list []api.Notification
}

// Notify queues n for the next frame and plays its tone when n asks for one.
// It is safe from any goroutine and reads no window; the shell calls
// DrainNotifications from its view.
func Notify(n api.Notification) {
	if n.Title == "" && n.Body == "" {
		return
	}
	pending.mu.Lock()
	pending.list = append(pending.list, n)
	pending.mu.Unlock()
	if n.Sound {
		playNotificationSound(n.Level)
	}
}

// DrainNotifications shows up to a few queued notifications with c.Toast and
// keeps the rest for later frames. The shell calls it once per frame.
func DrainNotifications(c *mui.Context) {
	if c == nil {
		return
	}
	pending.mu.Lock()
	n := min(len(pending.list), toastBatch)
	batch := append([]api.Notification(nil), pending.list[:n]...)
	pending.list = append([]api.Notification(nil), pending.list[n:]...)
	pending.mu.Unlock()
	for _, item := range batch {
		c.Toast(notificationText(item))
	}
}

// notificationText joins a notification's title and body for a toast.
func notificationText(n api.Notification) string {
	switch {
	case n.Title == "":
		return n.Body
	case n.Body == "":
		return n.Title
	default:
		return n.Title + " — " + n.Body
	}
}
