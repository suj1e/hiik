package builtin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"hiik/mods/api"
)

// sideAgentCanonicalStatus folds the core's status words into the four the
// panel draws. An error reads as blocked; an unknown word reads as idle.
func sideAgentCanonicalStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "blocked", "error":
		return "blocked"
	case "done":
		return "done"
	case "working":
		return "working"
	default:
		return "idle"
	}
}

// sideAgentStatusRank orders the four statuses from most to least urgent.
func sideAgentStatusRank(status string) int {
	switch sideAgentCanonicalStatus(status) {
	case "blocked":
		return 4
	case "done":
		return 3
	case "working":
		return 2
	default:
		return 1
	}
}

// sideAgentDuration is how long a status has held, as the panel writes it.
func sideAgentDuration(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	minutes := ms / int64(time.Minute/time.Millisecond)
	if minutes < 1 {
		return "<1m"
	}
	if minutes < 60 {
		return fmt.Sprintf("%dm", minutes)
	}
	hours := minutes / 60
	if hours < 24 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dd", hours/24)
}

// sideAgentStatusKey identifies one agent's current status. A status change
// starts a new key, so the duration resets only when the status does.
func sideAgentStatusKey(ag api.Agent) string {
	return ag.Address + "\x00" + sideAgentCanonicalStatus(ag.Status)
}

// sideAgentStatusClock remembers when each agent's status last changed. It
// keeps that across frames, and best effort across restarts in a small cache
// file. A zero path keeps it in memory only, as tests want.
type sideAgentStatusClock struct {
	mu     sync.Mutex
	path   string
	seen   map[string]int64 // address\x00status -> unix milliseconds
	loaded bool
}

func newSideAgentStatusClock() *sideAgentStatusClock {
	return &sideAgentStatusClock{}
}

func sideAgentStatusPath() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "hiik", "agent-status.json")
}

var sideAgentStatuses = &sideAgentStatusClock{path: sideAgentStatusPath()}

// sideAgentDurations stamps the agents the panel is about to draw and
// returns how long each agent's status has held, in milliseconds.
func sideAgentDurations(agents []api.Agent, now time.Time) map[string]int64 {
	return sideAgentStatuses.durations(agents, now)
}

func (c *sideAgentStatusClock) durations(agents []api.Agent, now time.Time) map[string]int64 {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.loadLocked()
	if c.seen == nil {
		c.seen = make(map[string]int64, len(agents))
	}
	nowMS := now.UnixMilli()
	if nowMS <= 0 {
		nowMS = time.Now().UnixMilli()
	}
	dirty := false
	active := make(map[string]bool, len(agents))
	for _, ag := range agents {
		if ag.Address == "" {
			continue
		}
		active[ag.Address] = true
		key := sideAgentStatusKey(ag)
		if _, ok := c.seen[key]; ok {
			continue
		}
		// A different status for this address starts over: drop the old key
		// so a flapping status does not pile up entries.
		prefix := ag.Address + "\x00"
		for old := range c.seen {
			if strings.HasPrefix(old, prefix) {
				delete(c.seen, old)
			}
		}
		c.seen[key] = nowMS
		dirty = true
	}
	// Keep a gone agent's stamp for a while: a terminal that disconnects and
	// returns should not read as if its status just changed.
	ttl := int64((30 * 24 * time.Hour) / time.Millisecond)
	for key, at := range c.seen {
		address, _, _ := strings.Cut(key, "\x00")
		if !active[address] && nowMS-at > ttl {
			delete(c.seen, key)
			dirty = true
		}
	}
	if dirty {
		c.saveLocked()
	}
	out := make(map[string]int64, len(agents))
	for _, ag := range agents {
		if ag.Address == "" {
			continue
		}
		since := nowMS - c.seen[sideAgentStatusKey(ag)]
		if since < 0 {
			since = 0
		}
		out[ag.Address] = since
	}
	return out
}

func (c *sideAgentStatusClock) loadLocked() {
	if c.loaded {
		return
	}
	c.loaded = true
	if c.path == "" {
		return
	}
	data, err := os.ReadFile(c.path)
	if err != nil {
		return
	}
	var seen map[string]int64
	if err := json.Unmarshal(data, &seen); err != nil || seen == nil {
		return
	}
	c.seen = seen
}

func (c *sideAgentStatusClock) saveLocked() {
	if c.path == "" {
		return
	}
	data, err := json.Marshal(c.seen)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return
	}
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, c.path)
}
