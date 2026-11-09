package api

import (
	"fmt"
	"sync"
)

var (
	mu         sync.RWMutex
	registered []Mod
)

// Register adds a mod. Mods call it from init, and duplicate IDs panic so a
// conflict shows at startup rather than as a missing command.
func Register(m Mod) {
	mu.Lock()
	defer mu.Unlock()
	if m.ID == "" {
		panic("mods: a mod needs an ID")
	}
	for _, other := range registered {
		if other.ID == m.ID {
			panic(fmt.Sprintf("mods: duplicate mod ID %q", m.ID))
		}
	}
	for _, p := range m.Panels {
		if p.ID == "" {
			panic(fmt.Sprintf("mods: mod %q has a panel without an ID", m.ID))
		}
	}
	registered = append(registered, m)
}

// All returns the registered mods.
func All() []Mod {
	mu.RLock()
	defer mu.RUnlock()
	return append([]Mod(nil), registered...)
}

// Panels returns every registered panel, keyed by its panel ID. Panel IDs
// are global, so the shell and the layout name a panel without its mod.
func Panels() map[string]Panel {
	mu.RLock()
	defer mu.RUnlock()
	out := make(map[string]Panel)
	for _, m := range registered {
		for _, p := range m.Panels {
			out[p.ID] = p
		}
	}
	return out
}

// Commands returns every registered command.
func Commands() []Command {
	mu.RLock()
	defer mu.RUnlock()
	var out []Command
	for _, m := range registered {
		out = append(out, m.Commands...)
	}
	return out
}
