package core

import (
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"hiik/core/hc"
	"hiik/core/store"
	"hiik/mods/api"
)

// model is the app's layout: spaces, tabs, panes, the selected space and the
// selected agent. App holds its lock, so no method here takes one.
type model struct {
	spaces []api.Space
	agents []api.Agent

	// settled holds the tabs put away on this screen, by tab ID. A settled
	// tab keeps running and comes back when shown or when an agent needs a
	// person.
	settled map[string]bool
	// shown holds, by pane ID, the terminal that pane owned before
	// terminal.show put another one there for now.
	shown map[string]string
	// terminals is the core's last record of each terminal, keyed by
	// address. The UI reads titles, folders and exit state from it.
	terminals map[string]hc.Terminal

	activeSpace string
	activeAgent string
	ui          api.UIState

	seqSpace int
	seqTab   int
	seqPane  int
}

// modelFromState builds the model from a loaded state file and repairs
// anything that cannot be rendered.
func modelFromState(st store.State) model {
	m := model{
		spaces:      st.SpacesAPI(),
		activeSpace: st.ActiveSpace,
		ui:          st.UI,
		settled:     map[string]bool{},
		shown:       map[string]string{},
		terminals:   map[string]hc.Terminal{},
	}
	m.reserveIDs()
	m.normalize()
	return m
}

// snapshot converts the model to its on-disk shape. It copies every node.
// Borrowed terminals from terminal.show are views, so the file keeps the
// terminals the panes own.
func (m *model) snapshot() store.State {
	spaces := m.spaces
	if len(m.shown) > 0 {
		spaces = cloneSpaces(m.spaces)
		for i := range spaces {
			for j := range spaces[i].Tabs {
				walkNode(spaces[i].Tabs[j].Root, func(n *api.Node) {
					if isLeaf(n) {
						if original, ok := m.shown[n.ID]; ok {
							n.Terminal = original
						}
					}
				})
			}
		}
	}
	return store.FromAPI(spaces, m.activeSpace, m.ui)
}

// --- IDs ---

func (m *model) nextSpaceID() string {
	m.seqSpace++
	return "s" + strconv.Itoa(m.seqSpace)
}

func (m *model) nextTabID() string {
	m.seqTab++
	return "t" + strconv.Itoa(m.seqTab)
}

func (m *model) nextPaneID() string {
	m.seqPane++
	return "p" + strconv.Itoa(m.seqPane)
}

func (m *model) reserveIDs() {
	for i := range m.spaces {
		sp := &m.spaces[i]
		m.bumpID(&m.seqSpace, sp.ID, "s")
		for j := range sp.Tabs {
			tab := &sp.Tabs[j]
			m.bumpID(&m.seqTab, tab.ID, "t")
			walkNode(tab.Root, func(n *api.Node) {
				m.bumpID(&m.seqPane, n.ID, "p")
			})
		}
	}
}

func (m *model) bumpID(seq *int, id, prefix string) {
	if !strings.HasPrefix(id, prefix) {
		return
	}
	n, err := strconv.Atoi(id[len(prefix):])
	if err == nil && n > *seq {
		*seq = n
	}
}

// ensureMaps makes the model's optional maps safe to write. A model built
// without modelFromState starts with none.
func (m *model) ensureMaps() {
	if m.settled == nil {
		m.settled = map[string]bool{}
	}
	if m.shown == nil {
		m.shown = map[string]string{}
	}
	if m.terminals == nil {
		m.terminals = map[string]hc.Terminal{}
	}
}

// --- Normalize ---

func (m *model) normalize() {
	m.ensureMaps()
	usedSpace := make(map[string]bool, len(m.spaces))
	usedTab := make(map[string]bool)
	usedPane := make(map[string]bool)

	spaces := make([]api.Space, 0, len(m.spaces))
	for i := range m.spaces {
		sp := m.spaces[i]
		if sp.ID == "" || usedSpace[sp.ID] {
			sp.ID = m.nextSpaceID()
		}
		usedSpace[sp.ID] = true
		if strings.TrimSpace(sp.Name) == "" {
			sp.Name = deriveSpaceName("", sp.Folder, sp.Machine)
		}
		if len(sp.Tabs) == 0 {
			tabID := m.nextTabID()
			paneID := m.nextPaneID()
			sp.Tabs = []api.Tab{{
				ID:    tabID,
				Name:  "1",
				Root:  &api.Node{ID: paneID},
				Focus: paneID,
			}}
		}
		for j := range sp.Tabs {
			tab := &sp.Tabs[j]
			if tab.ID == "" || usedTab[tab.ID] {
				tab.ID = m.nextTabID()
			}
			usedTab[tab.ID] = true
			if strings.TrimSpace(tab.Name) == "" {
				tab.Name = strconv.Itoa(j + 1)
			}
			tab.Root = m.normalizeNode(tab.Root, usedPane)
			if tab.Focus == "" || !isLeaf(findNode(tab.Root, tab.Focus)) {
				tab.Focus = firstLeafID(tab.Root)
			}
		}
		if sp.Active < 0 {
			sp.Active = 0
		}
		if sp.Active >= len(sp.Tabs) {
			sp.Active = len(sp.Tabs) - 1
		}
		spaces = append(spaces, sp)
	}
	m.spaces = spaces

	if m.spaceIndex(m.activeSpace) < 0 {
		if len(m.spaces) > 0 {
			m.activeSpace = m.spaces[0].ID
		} else {
			m.activeSpace = ""
		}
	}
	if m.ui.Zoomed != "" && !m.paneExists(m.ui.Zoomed) {
		m.ui.Zoomed = ""
	}
	if m.ui.Focus == "" || !m.paneExists(m.ui.Focus) {
		m.ui.Focus = m.activeTabFocus()
	}
	if m.activeAgent != "" && m.agentIndex(m.activeAgent) < 0 {
		m.activeAgent = ""
	}
	m.pruneSettled()
	m.pruneShown()
	sortAgents(m.agents)
}

// pruneSettled drops put-away marks for tabs that are gone.
func (m *model) pruneSettled() {
	if len(m.settled) == 0 {
		return
	}
	live := make(map[string]bool)
	for si := range m.spaces {
		for ti := range m.spaces[si].Tabs {
			live[m.spaces[si].Tabs[ti].ID] = true
		}
	}
	for id := range m.settled {
		if !live[id] {
			delete(m.settled, id)
		}
	}
}

// pruneShown drops borrowed-terminal marks for panes that are gone.
func (m *model) pruneShown() {
	for paneID := range m.shown {
		if !m.paneExists(paneID) {
			delete(m.shown, paneID)
		}
	}
}

func (m *model) normalizeNode(n *api.Node, used map[string]bool) *api.Node {
	if n == nil {
		return &api.Node{ID: m.newNodeID(used)}
	}
	if n.ID == "" || used[n.ID] {
		n.ID = m.newNodeID(used)
	}
	used[n.ID] = true

	if n.A == nil && n.B == nil {
		n.Vertical = false
		n.Ratio = 0
		return n
	}
	n.A = m.normalizeNode(n.A, used)
	if n.B == nil {
		n.B = &api.Node{ID: m.newNodeID(used)}
	}
	n.B = m.normalizeNode(n.B, used)
	if n.Ratio <= 0 || n.Ratio >= 1 {
		n.Ratio = 0.5
	}
	return n
}

func (m *model) newNodeID(used map[string]bool) string {
	for {
		id := m.nextPaneID()
		if !used[id] {
			return id
		}
	}
}

// --- Space helpers ---

func (m *model) spaceIndex(id string) int {
	if id == "" {
		return -1
	}
	for i := range m.spaces {
		if m.spaces[i].ID == id {
			return i
		}
	}
	return -1
}

func activeTab(sp *api.Space) *api.Tab {
	if sp == nil || len(sp.Tabs) == 0 {
		return nil
	}
	i := sp.Active
	if i < 0 || i >= len(sp.Tabs) {
		i = 0
	}
	return &sp.Tabs[i]
}

func tabIndex(sp *api.Space, tabID string) int {
	if sp == nil || len(sp.Tabs) == 0 {
		return -1
	}
	if tabID == "" {
		i := sp.Active
		if i < 0 || i >= len(sp.Tabs) {
			i = 0
		}
		return i
	}
	for i := range sp.Tabs {
		if sp.Tabs[i].ID == tabID {
			return i
		}
	}
	return -1
}

// deriveSpaceName returns name, or a name derived from the folder or machine.
func deriveSpaceName(name, folder, machine string) string {
	if s := strings.TrimSpace(name); s != "" {
		return s
	}
	if folder = strings.TrimSpace(folder); folder != "" {
		base := filepath.Base(filepath.Clean(folder))
		if base != "" && base != "." && base != string(filepath.Separator) {
			return base
		}
	}
	if machine = strings.TrimSpace(machine); machine != "" {
		return machine
	}
	return "space"
}

func tabName(name string, number int) string {
	if s := strings.TrimSpace(name); s != "" {
		return s
	}
	return strconv.Itoa(number)
}

// --- Space mutations ---

// createSpace adds a space with one tab and one leaf, and selects it.
func (m *model) createSpace(name, folder, machine string) (spaceID, tabID, paneID string) {
	spaceID = m.nextSpaceID()
	tabID = m.nextTabID()
	paneID = m.nextPaneID()
	m.spaces = append(m.spaces, api.Space{
		ID:      spaceID,
		Name:    deriveSpaceName(name, folder, machine),
		Folder:  folder,
		Machine: machine,
		Tabs: []api.Tab{{
			ID:    tabID,
			Name:  "1",
			Root:  &api.Node{ID: paneID},
			Focus: paneID,
		}},
	})
	m.activeSpace = spaceID
	m.ui.Zoomed = ""
	m.ui.Focus = paneID
	return spaceID, tabID, paneID
}

// closeSpace removes a space and returns the terminals in it.
func (m *model) closeSpace(spaceID string) ([]string, error) {
	si := m.spaceIndex(spaceID)
	if si < 0 {
		return nil, fmt.Errorf("space %q not found", spaceID)
	}
	wasActive := m.activeSpace == spaceID
	terms := m.removeSpaceAt(si)
	if wasActive {
		switch {
		case len(m.spaces) == 0:
			m.activeSpace = ""
		case si >= len(m.spaces):
			m.activeSpace = m.spaces[len(m.spaces)-1].ID
		default:
			m.activeSpace = m.spaces[si].ID
		}
	}
	m.fixActive()
	return terms, nil
}

func (m *model) removeSpaceAt(si int) []string {
	sp := m.spaces[si]
	var terms []string
	for i := range sp.Tabs {
		terms = append(terms, m.collectTerminals(sp.Tabs[i].Root)...)
		delete(m.settled, sp.Tabs[i].ID)
	}
	m.spaces = append(m.spaces[:si], m.spaces[si+1:]...)
	m.pruneShown()
	return terms
}

func (m *model) renameSpace(spaceID, name string) error {
	si := m.spaceIndex(spaceID)
	if si < 0 {
		return fmt.Errorf("space %q not found", spaceID)
	}
	sp := &m.spaces[si]
	sp.Name = deriveSpaceName(name, sp.Folder, sp.Machine)
	return nil
}

func (m *model) moveSpace(spaceID string, delta int, to *int) error {
	si := m.spaceIndex(spaceID)
	if si < 0 {
		return fmt.Errorf("space %q not found", spaceID)
	}
	target, err := moveTarget("space", si, len(m.spaces), delta, to)
	if err != nil {
		return err
	}
	if target == si {
		return nil
	}
	sp := m.spaces[si]
	m.spaces = append(m.spaces[:si], m.spaces[si+1:]...)
	m.spaces = append(m.spaces, api.Space{})
	copy(m.spaces[target+1:], m.spaces[target:])
	m.spaces[target] = sp
	return nil
}

func (m *model) selectSpace(spaceID string) error {
	si := m.spaceIndex(spaceID)
	if si < 0 {
		return fmt.Errorf("space %q not found", spaceID)
	}
	m.activeSpace = spaceID
	m.ui.Zoomed = ""
	if tab := activeTab(&m.spaces[si]); tab != nil {
		m.ui.Focus = tab.Focus
	}
	return nil
}

func (m *model) selectSpaceDelta(delta int) error {
	if delta == 0 {
		return fmt.Errorf("space.select: delta must not be zero")
	}
	if len(m.spaces) == 0 {
		return fmt.Errorf("space.select: no spaces")
	}
	si := m.spaceIndex(m.activeSpace)
	if si < 0 {
		si = 0
	}
	si = (si + delta) % len(m.spaces)
	if si < 0 {
		si += len(m.spaces)
	}
	return m.selectSpace(m.spaces[si].ID)
}

// --- Tab mutations ---

// createTab adds a tab with one leaf and selects it.
func (m *model) createTab(spaceID, name string) (tabID, paneID string, err error) {
	si := m.spaceIndex(spaceID)
	if si < 0 {
		return "", "", fmt.Errorf("space %q not found", spaceID)
	}
	sp := &m.spaces[si]
	tabID = m.nextTabID()
	paneID = m.nextPaneID()
	sp.Tabs = append(sp.Tabs, api.Tab{
		ID:    tabID,
		Name:  tabName(name, len(sp.Tabs)+1),
		Root:  &api.Node{ID: paneID},
		Focus: paneID,
	})
	sp.Active = len(sp.Tabs) - 1
	m.activeSpace = sp.ID
	m.ui.Zoomed = ""
	m.ui.Focus = paneID
	return tabID, paneID, nil
}

// closeTab removes a tab, closing the space when it was the last tab, and
// returns the terminals in it.
func (m *model) closeTab(spaceID, tabID string) ([]string, error) {
	si := m.spaceIndex(spaceID)
	if si < 0 {
		return nil, fmt.Errorf("space %q not found", spaceID)
	}
	ti := tabIndex(&m.spaces[si], tabID)
	if ti < 0 {
		return nil, fmt.Errorf("tab %q not found", tabID)
	}
	return m.closeTabAt(si, ti)
}

func (m *model) closeTabAt(si, ti int) ([]string, error) {
	m.ensureMaps()
	sp := &m.spaces[si]
	terms := m.collectTerminals(sp.Tabs[ti].Root)
	delete(m.settled, sp.Tabs[ti].ID)
	wasActive := sp.Active == ti
	sp.Tabs = append(sp.Tabs[:ti], sp.Tabs[ti+1:]...)
	if len(sp.Tabs) == 0 {
		terms = append(terms, m.removeSpaceAt(si)...)
		m.fixActive()
		return terms, nil
	}
	switch {
	case wasActive && ti < len(sp.Tabs):
		sp.Active = ti
	case wasActive:
		sp.Active = len(sp.Tabs) - 1
	case sp.Active > ti:
		sp.Active--
	}
	m.fixActive()
	m.pruneShown()
	return terms, nil
}

func (m *model) renameTab(spaceID, tabID, name string) error {
	si := m.spaceIndex(spaceID)
	if si < 0 {
		return fmt.Errorf("space %q not found", spaceID)
	}
	sp := &m.spaces[si]
	ti := tabIndex(sp, tabID)
	if ti < 0 {
		return fmt.Errorf("tab %q not found", tabID)
	}
	sp.Tabs[ti].Name = tabName(name, ti+1)
	return nil
}

func (m *model) moveTab(spaceID, tabID string, delta int, to *int) error {
	si := m.spaceIndex(spaceID)
	if si < 0 {
		return fmt.Errorf("space %q not found", spaceID)
	}
	sp := &m.spaces[si]
	ti := tabIndex(sp, tabID)
	if ti < 0 {
		return fmt.Errorf("tab %q not found", tabID)
	}
	target, err := moveTarget("tab", ti, len(sp.Tabs), delta, to)
	if err != nil {
		return err
	}
	if target == ti {
		return nil
	}
	activeID := ""
	if sp.Active >= 0 && sp.Active < len(sp.Tabs) {
		activeID = sp.Tabs[sp.Active].ID
	}
	tab := sp.Tabs[ti]
	sp.Tabs = append(sp.Tabs[:ti], sp.Tabs[ti+1:]...)
	sp.Tabs = append(sp.Tabs, api.Tab{})
	copy(sp.Tabs[target+1:], sp.Tabs[target:])
	sp.Tabs[target] = tab
	if activeID != "" {
		for i := range sp.Tabs {
			if sp.Tabs[i].ID == activeID {
				sp.Active = i
				break
			}
		}
	}
	if sp.Active >= len(sp.Tabs) {
		sp.Active = len(sp.Tabs) - 1
	}
	return nil
}

func (m *model) selectTab(spaceID, tabID string) error {
	si := m.spaceIndex(spaceID)
	if si < 0 {
		return fmt.Errorf("space %q not found", spaceID)
	}
	sp := &m.spaces[si]
	ti := tabIndex(sp, tabID)
	if ti < 0 {
		return fmt.Errorf("tab %q not found", tabID)
	}
	sp.Active = ti
	m.activeSpace = sp.ID
	m.ui.Zoomed = ""
	m.ui.Focus = sp.Tabs[ti].Focus
	// Showing a settled tab brings it back to the lists.
	delete(m.settled, sp.Tabs[ti].ID)
	return nil
}

// selectTabDelta shows the next or previous unsettled tab of a space.
// Settled tabs are put away, so the walk skips them.
func (m *model) selectTabDelta(spaceID string, delta int) error {
	if delta == 0 {
		return fmt.Errorf("tab.select: delta must not be zero")
	}
	si := m.spaceIndex(spaceID)
	if si < 0 {
		return fmt.Errorf("space %q not found", spaceID)
	}
	sp := &m.spaces[si]
	if len(sp.Tabs) == 0 {
		return fmt.Errorf("tab.select: no tabs")
	}
	m.ensureMaps()
	cur := sp.Active
	if cur < 0 || cur >= len(sp.Tabs) {
		cur = 0
	}
	step := 1
	if delta < 0 {
		step = -1
	}
	n := len(sp.Tabs)
	for k := 1; k <= n; k++ {
		next := ((cur+step*k)%n + n) % n
		if !m.settled[sp.Tabs[next].ID] {
			return m.selectTab(spaceID, sp.Tabs[next].ID)
		}
	}
	return nil
}

// settleTab puts a tab away. The shown tab hands over to the next unsettled
// tab of its space, if there is one; settled tabs keep running.
func (m *model) settleTab(si, ti int) {
	m.ensureMaps()
	sp := &m.spaces[si]
	id := sp.Tabs[ti].ID
	m.settled[id] = true
	if sp.Active != ti {
		return
	}
	for k := 1; k < len(sp.Tabs); k++ {
		next := (ti + k) % len(sp.Tabs)
		if !m.settled[sp.Tabs[next].ID] {
			sp.Active = next
			m.ui.Zoomed = ""
			m.ui.Focus = sp.Tabs[next].Focus
			return
		}
	}
}

// unsettleTab brings a settled tab back to the lists. It reports whether the
// tab was settled.
func (m *model) unsettleTab(si, ti int) bool {
	id := m.spaces[si].Tabs[ti].ID
	if !m.settled[id] {
		return false
	}
	delete(m.settled, id)
	return true
}

// unsettlePane brings back the tab that holds a pane, if it was settled.
func (m *model) unsettlePane(paneID string) bool {
	si, ti, ok := m.paneLocation(paneID)
	if !ok {
		return false
	}
	return m.unsettleTab(si, ti)
}

// --- Pane mutations ---

// splitPane replaces a leaf with a split and a new leaf. vertical stacks the
// children; otherwise they sit side by side. It returns the new leaf's ID.
func (m *model) splitPane(paneID string, vertical bool) (string, error) {
	si, ti, ok := m.paneLocation(paneID)
	if !ok {
		return "", fmt.Errorf("pane %q not found", paneID)
	}
	tab := &m.spaces[si].Tabs[ti]
	target := findNode(tab.Root, paneID)
	if !isLeaf(target) {
		return "", fmt.Errorf("pane %q is not a terminal", paneID)
	}
	newPaneID := m.nextPaneID()
	splitID := m.nextPaneID()
	leaf := *target
	*target = api.Node{
		ID:       splitID,
		Vertical: vertical,
		Ratio:    0.5,
		A:        &leaf,
		B:        &api.Node{ID: newPaneID},
	}
	tab.Focus = newPaneID
	m.ui.Focus = newPaneID
	m.ui.Zoomed = ""
	return newPaneID, nil
}

// setPaneMod makes a leaf a mod pane.
func (m *model) setPaneMod(paneID, mod string, props map[string]any) error {
	si, ti, ok := m.paneLocation(paneID)
	if !ok {
		return fmt.Errorf("pane %q not found", paneID)
	}
	n := findNode(m.spaces[si].Tabs[ti].Root, paneID)
	if n == nil || !isLeaf(n) {
		return fmt.Errorf("pane %q is not a leaf", paneID)
	}
	n.Mod = mod
	n.Props = props
	n.Terminal = ""
	return nil
}

// closePane removes a pane, its whole subtree, and returns its terminals.
func (m *model) closePane(paneID string) ([]string, error) {
	si, ti, ok := m.paneLocation(paneID)
	if !ok {
		return nil, fmt.Errorf("pane %q not found", paneID)
	}
	tab := &m.spaces[si].Tabs[ti]
	terms := m.collectTerminals(findNode(tab.Root, paneID))
	root, removed := removeNode(tab.Root, paneID)
	if !removed {
		return nil, fmt.Errorf("pane %q not found", paneID)
	}
	tab.Root = root
	if tab.Root == nil {
		more, err := m.closeTabAt(si, ti)
		return append(terms, more...), err
	}
	if !isLeaf(findNode(tab.Root, tab.Focus)) {
		tab.Focus = firstLeafID(tab.Root)
	}
	m.fixActive()
	m.pruneShown()
	return terms, nil
}

func (m *model) setTerminal(paneID, address string) bool {
	for si := range m.spaces {
		for ti := range m.spaces[si].Tabs {
			n := findNode(m.spaces[si].Tabs[ti].Root, paneID)
			if isLeaf(n) {
				n.Terminal = address
				delete(m.shown, paneID)
				return true
			}
		}
	}
	return false
}

// originalTerminal is the terminal a leaf owns: the one terminal.show put
// aside while another is shown in its pane.
func (m *model) originalTerminal(n *api.Node) string {
	if n == nil {
		return ""
	}
	if original, ok := m.shown[n.ID]; ok {
		return original
	}
	return n.Terminal
}

// owningPane returns the pane a terminal belongs to. A terminal shown in
// another pane for now does not own that pane.
func (m *model) owningPane(address string) (string, bool) {
	if address == "" {
		return "", false
	}
	for si := range m.spaces {
		for ti := range m.spaces[si].Tabs {
			var found string
			walkNode(m.spaces[si].Tabs[ti].Root, func(n *api.Node) {
				if found == "" && isLeaf(n) && m.originalTerminal(n) == address {
					found = n.ID
				}
			})
			if found != "" {
				return found, true
			}
		}
	}
	return "", false
}

// restoreShown puts a pane's own terminal back after terminal.show borrowed
// it. It reports whether the pane was borrowing one.
func (m *model) restoreShown(paneID string) bool {
	original, ok := m.shown[paneID]
	if !ok {
		return false
	}
	if si, ti, found := m.paneLocation(paneID); found {
		n := findNode(m.spaces[si].Tabs[ti].Root, paneID)
		if isLeaf(n) {
			n.Terminal = original
		}
	}
	delete(m.shown, paneID)
	return true
}

// showInPane shows a terminal in a pane for now, keeping the pane's own
// terminal to restore later.
func (m *model) showInPane(paneID, address string) error {
	m.ensureMaps()
	si, ti, ok := m.paneLocation(paneID)
	if !ok || !isLeaf(findNode(m.spaces[si].Tabs[ti].Root, paneID)) {
		return fmt.Errorf("pane %q not found", paneID)
	}
	n := findNode(m.spaces[si].Tabs[ti].Root, paneID)
	if _, borrowed := m.shown[paneID]; !borrowed {
		m.shown[paneID] = n.Terminal
	}
	n.Terminal = address
	return m.focusPane(paneID)
}

// restoreBorrowed gives every pane borrowing address its own terminal back.
// It reports whether any pane changed.
func (m *model) restoreBorrowed(address string) bool {
	if address == "" {
		return false
	}
	changed := false
	for si := range m.spaces {
		for ti := range m.spaces[si].Tabs {
			walkNode(m.spaces[si].Tabs[ti].Root, func(n *api.Node) {
				if isLeaf(n) && n.Terminal == address {
					if m.restoreShown(n.ID) {
						changed = true
					}
				}
			})
		}
	}
	return changed
}

// focusedTerminal returns the terminal in the focused pane, or "".
func (m *model) focusedTerminal() string {
	si := m.spaceIndex(m.activeSpace)
	if si < 0 {
		return ""
	}
	tab := activeTab(&m.spaces[si])
	if tab == nil {
		return ""
	}
	n := findNode(tab.Root, tab.Focus)
	if !isLeaf(n) {
		n = findNode(tab.Root, firstLeafID(tab.Root))
	}
	return n.Terminal
}

// setTerminalRecords replaces the core's terminal records, keyed by address.
func (m *model) setTerminalRecords(terms map[string]hc.Terminal) {
	m.terminals = make(map[string]hc.Terminal, len(terms))
	for address, t := range terms {
		if t.Address == "" {
			t.Address = address
		}
		m.terminals[address] = t
	}
}

// terminal returns the core's last record of a terminal.
func (m *model) terminal(address string) (hc.Terminal, bool) {
	t, ok := m.terminals[address]
	return t, ok
}

// setTerminalLabel records one label on a terminal in the model. The core
// keeps the durable copy.
func (m *model) setTerminalLabel(address, key, value string) {
	m.ensureMaps()
	t, ok := m.terminals[address]
	if !ok {
		return
	}
	labels := make(map[string]string, len(t.Labels)+1)
	for k, v := range t.Labels {
		labels[k] = v
	}
	if value == "" {
		delete(labels, key)
	} else {
		labels[key] = value
	}
	t.Labels = labels
	m.terminals[address] = t
}

func (m *model) resolvePane(paneID string) (string, error) {
	if paneID == "" {
		paneID = m.activeTabFocus()
		if paneID == "" {
			return "", fmt.Errorf("no focused pane")
		}
		return paneID, nil
	}
	si, ti, ok := m.paneLocation(paneID)
	if !ok || !isLeaf(findNode(m.spaces[si].Tabs[ti].Root, paneID)) {
		return "", fmt.Errorf("pane %q not found", paneID)
	}
	return paneID, nil
}

func (m *model) paneExists(paneID string) bool {
	si, ti, ok := m.paneLocation(paneID)
	return ok && isLeaf(findNode(m.spaces[si].Tabs[ti].Root, paneID))
}

// paneLocation returns the indexes of the space and tab holding a node.
func (m *model) paneLocation(nodeID string) (si, ti int, ok bool) {
	if nodeID == "" {
		return -1, -1, false
	}
	for si := range m.spaces {
		for ti := range m.spaces[si].Tabs {
			if findNode(m.spaces[si].Tabs[ti].Root, nodeID) != nil {
				return si, ti, true
			}
		}
	}
	return -1, -1, false
}

// terminalLocation finds the space and tab that show a core terminal.
func (m *model) terminalLocation(address string) (si, ti int) {
	if address == "" {
		return -1, -1
	}
	for si := range m.spaces {
		for ti := range m.spaces[si].Tabs {
			if findTerminal(m.spaces[si].Tabs[ti].Root, address) != nil {
				return si, ti
			}
		}
	}
	return -1, -1
}

func (m *model) focusPane(paneID string) error {
	si, ti, ok := m.paneLocation(paneID)
	if !ok || !isLeaf(findNode(m.spaces[si].Tabs[ti].Root, paneID)) {
		return fmt.Errorf("pane %q not found", paneID)
	}
	sp := &m.spaces[si]
	sp.Active = ti
	sp.Tabs[ti].Focus = paneID
	if m.activeSpace != sp.ID {
		m.activeSpace = sp.ID
		m.ui.Zoomed = ""
	}
	if m.ui.Zoomed != "" && m.ui.Zoomed != paneID {
		m.ui.Zoomed = ""
	}
	m.ui.Focus = paneID
	return nil
}

// zoomPane toggles zoom for a pane and focuses it. It reports whether the
// pane is now zoomed.
func (m *model) zoomPane(paneID string) (bool, error) {
	si, ti, ok := m.paneLocation(paneID)
	if !ok || !isLeaf(findNode(m.spaces[si].Tabs[ti].Root, paneID)) {
		return false, fmt.Errorf("pane %q not found", paneID)
	}
	m.activeSpace = m.spaces[si].ID
	m.spaces[si].Active = ti
	m.spaces[si].Tabs[ti].Focus = paneID
	m.ui.Focus = paneID
	if m.ui.Zoomed == paneID {
		m.ui.Zoomed = ""
		return false, nil
	}
	m.ui.Zoomed = paneID
	return true, nil
}

// goPane focuses the neighboring pane in direction.
func (m *model) goPane(paneID, direction string) (string, error) {
	si, ti, ok := m.paneLocation(paneID)
	if !ok {
		return "", fmt.Errorf("pane %q not found", paneID)
	}
	target, ok := neighbor(&m.spaces[si].Tabs[ti], paneID, direction)
	if !ok {
		return "", fmt.Errorf("pane.go: no pane to the %s", direction)
	}
	if err := m.focusPane(target); err != nil {
		return "", err
	}
	return target, nil
}

// swapPanes exchanges the terminals of two leaves. Focus does not move.
func (m *model) swapPanes(a, b string) error {
	if a == b {
		return fmt.Errorf("pane.swap: cannot swap a pane with itself")
	}
	asi, ati, ok := m.paneLocation(a)
	if !ok {
		return fmt.Errorf("pane %q not found", a)
	}
	bsi, bti, ok := m.paneLocation(b)
	if !ok {
		return fmt.Errorf("pane %q not found", b)
	}
	an := findNode(m.spaces[asi].Tabs[ati].Root, a)
	bn := findNode(m.spaces[bsi].Tabs[bti].Root, b)
	if !isLeaf(an) || !isLeaf(bn) {
		return fmt.Errorf("pane.swap: panes must be terminals")
	}
	// A borrowed terminal would travel with the swap; put both panes' own
	// terminals back first.
	m.restoreShown(a)
	m.restoreShown(b)
	an.Terminal, bn.Terminal = bn.Terminal, an.Terminal
	return nil
}

func (m *model) setRatio(paneID string, ratio float32) error {
	if !(ratio > 0 && ratio < 1) {
		return fmt.Errorf("ratio %g must be between 0 and 1", ratio)
	}
	si, ti, ok := m.paneLocation(paneID)
	if !ok {
		return fmt.Errorf("pane %q not found", paneID)
	}
	root := m.spaces[si].Tabs[ti].Root
	n := findNode(root, paneID)
	if n == nil {
		return fmt.Errorf("pane %q not found", paneID)
	}
	if isLeaf(n) {
		n = parentSplit(root, paneID)
		if n == nil {
			return fmt.Errorf("pane %q has no split to resize", paneID)
		}
	}
	n.Ratio = ratio
	return nil
}

// parentSplit returns the split node that directly holds id, if any.
func parentSplit(n *api.Node, id string) *api.Node {
	if n == nil || isLeaf(n) {
		return nil
	}
	if (n.A != nil && n.A.ID == id) || (n.B != nil && n.B.ID == id) {
		return n
	}
	if found := parentSplit(n.A, id); found != nil {
		return found
	}
	return parentSplit(n.B, id)
}

// paneByTerminal returns the pane showing a terminal address.
func (m *model) paneByTerminal(address string) (string, bool) {
	if address == "" {
		return "", false
	}
	for si := range m.spaces {
		for ti := range m.spaces[si].Tabs {
			if n := findTerminal(m.spaces[si].Tabs[ti].Root, address); n != nil {
				return n.ID, true
			}
		}
	}
	return "", false
}

// neighbor returns the nearest leaf in direction from paneID.
func neighbor(tab *api.Tab, paneID, direction string) (string, bool) {
	if tab == nil {
		return "", false
	}
	rects := make(map[string]paneRect)
	layoutRects(tab.Root, paneRect{w: 1000, h: 1000}, rects)
	from, ok := rects[paneID]
	if !ok {
		return "", false
	}
	ids := make([]string, 0, len(rects))
	for id := range rects {
		if id != paneID {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)

	const eps = 1e-6
	best := ""
	bestGap := math.Inf(1)
	bestCross := math.Inf(1)
	for _, id := range ids {
		r := rects[id]
		var gap, cross float64
		switch direction {
		case "left":
			gap = from.x - (r.x + r.w)
			cross = math.Abs(r.y + r.h/2 - (from.y + from.h/2))
		case "right":
			gap = r.x - (from.x + from.w)
			cross = math.Abs(r.y + r.h/2 - (from.y + from.h/2))
		case "up":
			gap = from.y - (r.y + r.h)
			cross = math.Abs(r.x + r.w/2 - (from.x + from.w/2))
		case "down":
			gap = r.y - (from.y + from.h)
			cross = math.Abs(r.x + r.w/2 - (from.x + from.w/2))
		default:
			return "", false
		}
		if gap < -eps {
			continue
		}
		if gap < bestGap-eps || (math.Abs(gap-bestGap) <= eps && cross < bestCross) {
			best, bestGap, bestCross = id, gap, cross
		}
	}
	return best, best != ""
}

type paneRect struct {
	x, y, w, h float64
}

func layoutRects(n *api.Node, r paneRect, out map[string]paneRect) {
	if n == nil {
		return
	}
	if isLeaf(n) {
		out[n.ID] = r
		return
	}
	ratio := float64(n.Ratio)
	if ratio <= 0 || ratio >= 1 {
		ratio = 0.5
	}
	if n.Vertical {
		a := paneRect{x: r.x, y: r.y, w: r.w, h: r.h * ratio}
		b := paneRect{x: r.x, y: r.y + a.h, w: r.w, h: r.h - a.h}
		layoutRects(n.A, a, out)
		layoutRects(n.B, b, out)
		return
	}
	a := paneRect{x: r.x, y: r.y, w: r.w * ratio, h: r.h}
	b := paneRect{x: r.x + a.w, y: r.y, w: r.w - a.w, h: r.h}
	layoutRects(n.A, a, out)
	layoutRects(n.B, b, out)
}

// --- Agents ---

func (m *model) setAgents(list []api.Agent) {
	agents := append([]api.Agent(nil), list...)
	sortAgents(agents)
	m.agents = agents
	if m.activeAgent != "" && m.agentIndex(m.activeAgent) < 0 {
		m.activeAgent = ""
	}
}

// sortAgents applies the one list order: sub-agents under their parent,
// pinned groups first, then blocked > done > working > idle. agent.next and
// agent.prev walk this order.
func sortAgents(list []api.Agent) {
	copy(list, orderAgents(list))
}

func (m *model) agentIndex(address string) int {
	for i := range m.agents {
		if m.agents[i].Address == address {
			return i
		}
	}
	return -1
}

func (m *model) agent(address string) *api.Agent {
	if i := m.agentIndex(address); i >= 0 {
		return &m.agents[i]
	}
	return nil
}

// moveAgent selects the next or previous agent in list order. When nothing
// is selected, it selects the first agent going forward and the last going
// back.
func (m *model) moveAgent(delta int) (api.Agent, bool) {
	if len(m.agents) == 0 {
		return api.Agent{}, false
	}
	i := m.agentIndex(m.activeAgent)
	if i < 0 {
		if delta < 0 {
			i = len(m.agents) - 1
		} else {
			i = 0
		}
	} else {
		i = (i + delta) % len(m.agents)
		if i < 0 {
			i += len(m.agents)
		}
	}
	m.activeAgent = m.agents[i].Address
	return m.agents[i], true
}

// focusAgent selects an agent, marks it seen and focuses its pane when one
// shows it.
func (m *model) focusAgent(address string) error {
	i := m.agentIndex(address)
	if i < 0 {
		return fmt.Errorf("agent %q not found", address)
	}
	ag := &m.agents[i]
	ag.Seen = true
	if ag.Status == "done" {
		ag.Status = "idle"
	}
	m.activeAgent = address
	if ag.Address != "" {
		m.focusTerminal(ag.Address)
	}
	return nil
}

func (m *model) pinAgent(address string) (bool, error) {
	i := m.agentIndex(address)
	if i < 0 {
		return false, fmt.Errorf("agent %q not found", address)
	}
	m.agents[i].Pinned = !m.agents[i].Pinned
	pinned := m.agents[i].Pinned
	sortAgents(m.agents)
	return pinned, nil
}

func (m *model) setAgentPinned(address string, pinned bool) error {
	if m.agentIndex(address) < 0 {
		return fmt.Errorf("agent %q not found", address)
	}
	m.agents[m.agentIndex(address)].Pinned = pinned
	sortAgents(m.agents)
	return nil
}

func (m *model) focusTerminal(address string) bool {
	for si := range m.spaces {
		for ti := range m.spaces[si].Tabs {
			n := findTerminal(m.spaces[si].Tabs[ti].Root, address)
			if n != nil {
				return m.focusPane(n.ID) == nil
			}
		}
	}
	return false
}

// activeAgentAddress returns the selected agent's address, or "".
func (m *model) activeAgentAddress() string {
	return m.activeAgent
}

// --- Tree walking ---

func isLeaf(n *api.Node) bool {
	return n != nil && n.A == nil && n.B == nil
}

func walkNode(n *api.Node, fn func(*api.Node)) {
	if n == nil {
		return
	}
	fn(n)
	walkNode(n.A, fn)
	walkNode(n.B, fn)
}

func findNode(n *api.Node, id string) *api.Node {
	if n == nil || id == "" {
		return nil
	}
	if n.ID == id {
		return n
	}
	if found := findNode(n.A, id); found != nil {
		return found
	}
	return findNode(n.B, id)
}

func findTerminal(n *api.Node, address string) *api.Node {
	if address == "" {
		return nil
	}
	var found *api.Node
	walkNode(n, func(node *api.Node) {
		if found == nil && isLeaf(node) && node.Terminal == address {
			found = node
		}
	})
	return found
}

func firstLeafID(n *api.Node) string {
	if n == nil {
		return ""
	}
	if isLeaf(n) {
		return n.ID
	}
	if id := firstLeafID(n.A); id != "" {
		return id
	}
	return firstLeafID(n.B)
}

// collectTerminals returns the terminals a subtree owns. A terminal shown in
// a pane for now is not its own, so closing does not end it.
func (m *model) collectTerminals(n *api.Node) []string {
	if n == nil {
		return nil
	}
	var out []string
	walkNode(n, func(node *api.Node) {
		if isLeaf(node) {
			if address := m.originalTerminal(node); address != "" {
				out = append(out, address)
			}
		}
	})
	return out
}

// removeNode returns the subtree that replaces n and whether id was found.
// A removed split is replaced by its sibling.
func removeNode(n *api.Node, id string) (*api.Node, bool) {
	if n == nil {
		return nil, false
	}
	if n.ID == id {
		return nil, true
	}
	if n.A != nil {
		if repl, ok := removeNode(n.A, id); ok {
			n.A = repl
			if n.A == nil {
				return n.B, true
			}
			return n, true
		}
	}
	if n.B != nil {
		if repl, ok := removeNode(n.B, id); ok {
			n.B = repl
			if n.B == nil {
				return n.A, true
			}
			return n, true
		}
	}
	return n, false
}

func moveTarget(what string, from, n, delta int, to *int) (int, error) {
	if to != nil {
		if *to < 0 || *to >= n {
			return 0, fmt.Errorf("%s.move: index %d out of range", what, *to)
		}
		return *to, nil
	}
	if delta == 0 {
		return 0, fmt.Errorf("%s.move: delta must not be zero", what)
	}
	target := from + delta
	if target < 0 || target >= n {
		return 0, fmt.Errorf("%s.move: index %d out of range", what, target)
	}
	return target, nil
}

// fixActive repairs the selection after a removal.
func (m *model) fixActive() {
	if m.spaceIndex(m.activeSpace) < 0 {
		if len(m.spaces) > 0 {
			m.activeSpace = m.spaces[0].ID
		} else {
			m.activeSpace = ""
		}
	}
	if m.ui.Zoomed != "" && !m.paneExists(m.ui.Zoomed) {
		m.ui.Zoomed = ""
	}
	if !m.paneExists(m.ui.Focus) {
		m.ui.Focus = m.activeTabFocus()
	}
}

func (m *model) activeTabFocus() string {
	si := m.spaceIndex(m.activeSpace)
	if si < 0 {
		return ""
	}
	tab := activeTab(&m.spaces[si])
	if tab == nil {
		return ""
	}
	if id := firstLeafID(tab.Root); id != "" {
		if !isLeaf(findNode(tab.Root, tab.Focus)) {
			tab.Focus = id
		}
		return tab.Focus
	}
	return ""
}

// cloneSpaces copies the layout so callers cannot touch the model's nodes.
func cloneSpaces(spaces []api.Space) []api.Space {
	out := make([]api.Space, len(spaces))
	for i, sp := range spaces {
		out[i] = sp
		if len(sp.Tabs) > 0 {
			out[i].Tabs = make([]api.Tab, len(sp.Tabs))
			for j, tab := range sp.Tabs {
				out[i].Tabs[j] = tab
				out[i].Tabs[j].Root = cloneNode(tab.Root)
			}
		}
	}
	return out
}

func cloneNode(n *api.Node) *api.Node {
	if n == nil {
		return nil
	}
	out := *n
	out.A = cloneNode(n.A)
	out.B = cloneNode(n.B)
	return &out
}
