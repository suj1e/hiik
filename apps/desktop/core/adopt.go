package core

import (
	"sort"
	"strconv"
	"strings"

	"hiik/core/hc"
	"hiik/mods/api"
)

// adoptTerminals shows live core terminals that another client started in
// their spaces. It reads the shared labels, groups terminals by space and
// tab, and never moves or kills anything. Terminals without a space label
// stay out of the sidebar and appear as agents only.
func (a *App) adoptTerminals(terms []hc.Terminal) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	changed := false

	sort.SliceStable(terms, func(i, j int) bool {
		si, sti := adoptWhere(terms[i])
		sj, stj := adoptWhere(terms[j])
		if si != sj {
			return si < sj
		}
		if sti != stj {
			return sti < stj
		}
		return terms[i].Address < terms[j].Address
	})

	seen := make(map[string]bool)
	for _, sp := range a.model.spaces {
		for _, tab := range sp.Tabs {
			walkNode(tab.Root, func(n *api.Node) {
				if n.Terminal != "" {
					seen[n.Terminal] = true
				}
			})
		}
	}

	active := a.model.activeSpace
	spacesByLabel := make(map[string]string)
	tabsByKey := make(map[string]string)
	first := ""

	for _, t := range terms {
		if t.Address == "" {
			continue
		}
		spaceLabel := t.Labels["herdr.space"]
		if spaceLabel == "" {
			spaceLabel = t.Labels["hiik.space"]
		}
		if spaceLabel == "" {
			continue
		}
		tabLabel := t.Labels["herdr.tab"]
		if tabLabel == "" {
			tabLabel = t.Labels["hiik.tab"]
		}
		whereSpace, whereTab := adoptWhere(t)

		if seen[t.Address] {
			if a.adoptNames(t.Address, whereSpace, whereTab) {
				changed = true
			}
			continue
		}
		if t.State != "live" {
			continue
		}

		key := spaceLabel + "\x00" + tabLabel
		if tabLabel == "" {
			key = spaceLabel + "\x00" + t.Address
		}

		spaceID, ok := spacesByLabel[spaceLabel]
		if !ok {
			folder := t.Cwd
			machine := ""
			if i := strings.IndexByte(t.Address, '/'); i > 0 {
				machine = t.Address[:i]
			}
			spaceID, tabID, paneID := a.model.createSpace(whereSpace, folder, machine)
			spacesByLabel[spaceLabel] = spaceID
			tabsByKey[key] = tabID
			if whereTab != "" {
				a.renameTab(spaceID, tabID, whereTab)
			}
			a.model.setTerminal(paneID, t.Address)
			seen[t.Address] = true
			changed = true
			if first == "" {
				first = spaceID
			}
			continue
		}
		if tabID := tabsByKey[key]; tabID != "" {
			si := a.model.spaceIndex(spaceID)
			if si < 0 {
				continue
			}
			for ti := range a.model.spaces[si].Tabs {
				tab := &a.model.spaces[si].Tabs[ti]
				if tab.ID != tabID {
					continue
				}
				paneID := firstLeafID(tab.Root)
				if paneID == "" {
					break
				}
				newPane, err := a.model.splitPane(paneID, false)
				if err == nil {
					a.model.setTerminal(newPane, t.Address)
					seen[t.Address] = true
					changed = true
				}
				break
			}
			continue
		}

		tabID, paneID, err := a.model.createTab(spaceID, whereTab)
		if err != nil {
			continue
		}
		tabsByKey[key] = tabID
		a.model.setTerminal(paneID, t.Address)
		seen[t.Address] = true
		changed = true
	}

	if active == "" && first != "" {
		a.model.activeSpace = first
		changed = true
	}
	if changed {
		a.persistStateLocked()
	}
	return changed
}

// adoptWhere splits herdr's "space / tab" label. Space and tab names may
// repeat or contain spaces, so only the first " / " separates them.
func adoptWhere(t hc.Terminal) (space, tab string) {
	where := strings.TrimSpace(t.Labels["where"])
	if where == "" {
		return "", ""
	}
	if i := strings.Index(where, " / "); i >= 0 {
		return strings.TrimSpace(where[:i]), strings.TrimSpace(where[i+3:])
	}
	return where, ""
}

// adoptNames keeps an already adopted terminal's space and tab labels in
// step with herdr after a rename. Names that no longer look automatic are
// left alone.
func (a *App) adoptNames(address, spaceName, tabName string) bool {
	si, ti := a.model.terminalLocation(address)
	if si < 0 || ti < 0 {
		return false
	}
	sp := &a.model.spaces[si]
	changed := false
	if spaceName != "" && sp.Name == deriveSpaceName("", sp.Folder, sp.Machine) && sp.Name != spaceName {
		sp.Name = spaceName
		changed = true
	}
	if tabName == "" || ti >= len(sp.Tabs) {
		return changed
	}
	tab := &sp.Tabs[ti]
	if tab.Name == strconv.Itoa(ti+1) && tab.Name != tabName {
		tab.Name = tabName
		changed = true
	}
	return changed
}

func (a *App) renameTab(spaceID, tabID, name string) {
	si := a.model.spaceIndex(spaceID)
	if si < 0 {
		return
	}
	for ti := range a.model.spaces[si].Tabs {
		if a.model.spaces[si].Tabs[ti].ID == tabID {
			a.model.spaces[si].Tabs[ti].Name = name
			return
		}
	}
}

// persistStateLocked saves while the caller already holds the write lock.
func (a *App) persistStateLocked() {
	if a.store != nil {
		a.store.SaveState(a.model.snapshot())
	}
}
