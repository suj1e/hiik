package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"hiik/core/hc"
	"hiik/core/store"
	"hiik/mods/api"
)

// errNoCore is returned by commands that need a running core.
var errNoCore = errors.New("core not connected")

// callTimeout bounds one core RPC made outside a frame.
const callTimeout = 30 * time.Second

// cmd is one built-in command.
type cmd struct {
	id    string
	title string
	keys  []api.Binding
	args  argsSpec
	run   func(*App, map[string]any) error
}

func (c *cmd) execute(a *App, args map[string]any) error {
	return c.executeAs(a, args, true)
}

// executeAs validates arguments, runs the shared confirmation path for close
// commands, then performs the command. A person may be asked; an agent call
// never is.
func (c *cmd) executeAs(a *App, args map[string]any, person bool) error {
	if args == nil {
		args = map[string]any{}
	}
	if err := c.args.validate(c.id, args); err != nil {
		return err
	}
	handled, err := a.confirmClose(c, args, person)
	if err != nil {
		return err
	}
	if handled {
		return nil
	}
	return c.run(a, args)
}

// confirmClose asks before a close ends terminals. It reports whether the
// command was handed to the confirmer. Without a confirmer a person's close
// proceeds; an agent's close fails so it can pass confirmed: true.
func (a *App) confirmClose(c *cmd, args map[string]any, person bool) (bool, error) {
	title, message, needed := a.closeConfirmation(c.id, args)
	if !needed {
		return false, nil
	}
	if confirmed, ok := argBool(args, "confirmed"); ok && confirmed {
		return false, nil
	}
	if !person {
		return false, fmt.Errorf("%s ends its terminals: pass confirmed: true", c.id)
	}
	a.mu.RLock()
	fn := a.confirmer
	a.mu.RUnlock()
	if fn == nil {
		return false, nil
	}
	confirmed := make(map[string]any, len(args)+1)
	for key, value := range args {
		confirmed[key] = value
	}
	confirmed["confirmed"] = true
	fn(title, message, func() { _ = c.run(a, confirmed) })
	return true, nil
}

// closeConfirmation describes the question a command would ask, if any. Only
// closing a space asks: it ends every terminal in it, however the command
// reaches it (space.close, or the last tab or pane).
func (a *App) closeConfirmation(command string, args map[string]any) (title, message string, needed bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	si, ok := a.closeSpaceIndexLocked(command, args)
	if !ok {
		return "", "", false
	}
	sp := &a.model.spaces[si]
	terminals := 0
	for ti := range sp.Tabs {
		terminals += len(a.model.collectTerminals(sp.Tabs[ti].Root))
	}
	return "Close space?", fmt.Sprintf("%s — %d tabs, %d terminals", sp.Name, len(sp.Tabs), terminals), true
}

// closeSpaceIndexLocked returns the space a close would remove, if the
// command would remove one. App holds the lock.
func (a *App) closeSpaceIndexLocked(command string, args map[string]any) (int, bool) {
	switch command {
	case api.CmdSpaceClose:
		id := spaceArg(args)
		if id == "" {
			id = a.model.activeSpace
		}
		si := a.model.spaceIndex(id)
		return si, si >= 0
	case api.CmdTabClose:
		spaceID, tabID := a.tabTarget(args)
		si := a.model.spaceIndex(spaceID)
		if si < 0 || len(a.model.spaces[si].Tabs) != 1 {
			return -1, false
		}
		if tabIndex(&a.model.spaces[si], tabID) < 0 {
			return -1, false
		}
		return si, true
	case api.CmdPaneClose:
		target := paneArg(args)
		if target == "" {
			if term := argString(args, "terminal"); term != "" {
				id, ok := a.model.paneByTerminal(term)
				if !ok {
					return -1, false
				}
				target = id
			}
		}
		si, ti, ok := a.model.paneLocation(target)
		if !ok || len(a.model.spaces[si].Tabs) != 1 {
			return -1, false
		}
		if !isLeaf(a.model.spaces[si].Tabs[ti].Root) {
			return -1, false
		}
		return si, true
	default:
		return -1, false
	}
}

// buildCommands fills the registry. It runs before the app is shared.
func (a *App) buildCommands() {
	a.cmds = []*cmd{
		{id: api.CmdSpaceNew, title: "New space", args: argsSpec{
			"name": kindString, "folder": kindString, "machine": kindString, "argv": kindStrings,
		}, run: runSpaceNew},
		{id: api.CmdSpaceClose, title: "Close space", args: argsSpec{
			"id": kindString, "space": kindString, "confirmed": kindBool,
		}, run: runSpaceClose},
		{id: api.CmdSpaceRename, title: "Rename space", args: argsSpec{
			"id": kindString, "space": kindString, "name": kindString,
		}, run: runSpaceRename},
		{id: api.CmdSpaceMove, title: "Move space", args: argsSpec{
			"id": kindString, "space": kindString, "delta": kindInt, "to": kindInt,
		}, run: runSpaceMove},
		{id: api.CmdSpaceSelect, title: "Select space", args: argsSpec{
			"id": kindString, "space": kindString, "delta": kindInt,
		}, run: runSpaceSelect},
		{id: api.CmdTabNew, title: "New tab", args: argsSpec{
			"space": kindString, "name": kindString, "folder": kindString,
			"argv": kindStrings, "task": kindString,
		}, run: runTabNew},
		{id: api.CmdTabClose, title: "Close tab", args: argsSpec{
			"space": kindString, "tab": kindString, "id": kindString, "confirmed": kindBool,
		}, run: runTabClose},
		{id: "tab.settle", title: "Settle tab", args: argsSpec{
			"space": kindString, "tab": kindString, "id": kindString,
		}, run: runTabSettle},
		{id: "tab.unsettle", title: "Unsettle tab", args: argsSpec{
			"space": kindString, "tab": kindString, "id": kindString,
		}, run: runTabUnsettle},
		{id: api.CmdTabRename, title: "Rename tab", args: argsSpec{
			"space": kindString, "tab": kindString, "id": kindString, "name": kindString,
		}, run: runTabRename},
		{id: api.CmdTabMove, title: "Move tab", args: argsSpec{
			"space": kindString, "tab": kindString, "id": kindString,
			"delta": kindInt, "to": kindInt,
		}, run: runTabMove},
		{id: api.CmdTabSelect, title: "Select tab", args: argsSpec{
			"space": kindString, "tab": kindString, "id": kindString, "delta": kindInt,
		}, run: runTabSelect},
		{id: api.CmdPaneSplit, title: "Split pane", args: argsSpec{
			"pane": kindString, "node": kindString, "id": kindString,
			"direction": kindString, "dir": kindString, "terminal": kindString,
			"mod": kindString, "props": kindAny,
		}, run: runPaneSplit},
		{id: api.CmdPaneClose, title: "Close pane", args: argsSpec{
			"pane": kindString, "node": kindString, "id": kindString,
			"terminal": kindString, "confirmed": kindBool,
		}, run: runPaneClose},
		{id: api.CmdPaneZoom, title: "Zoom pane", args: argsSpec{
			"pane": kindString, "node": kindString, "id": kindString,
		}, run: runPaneZoom},
		{id: api.CmdPaneFocus, title: "Focus pane", args: argsSpec{
			"pane": kindString, "node": kindString, "id": kindString,
		}, run: runPaneFocus},
		{id: api.CmdPaneGo, title: "Go to pane", args: argsSpec{
			"pane": kindString, "node": kindString, "id": kindString,
			"direction": kindString, "dir": kindString,
		}, run: runPaneGo},
		{id: api.CmdPaneSwap, title: "Swap panes", args: argsSpec{
			"a": kindString, "b": kindString,
			"pane": kindString, "node": kindString, "id": kindString,
			"direction": kindString,
		}, run: runPaneSwap},
		{id: "pane.resize", title: "Resize pane", args: argsSpec{
			"pane": kindString, "node": kindString, "id": kindString, "ratio": kindFloat,
		}, run: runPaneResize},
		{id: "terminal.keep", title: "Keep terminal as a pane", args: argsSpec{
			"terminal": kindString, "place": kindString,
		}, run: runTerminalKeep},
		{id: "terminal.kill", title: "Kill terminal", args: argsSpec{
			"terminal": kindString,
		}, run: runTerminalKill},
		{id: "terminal.label", title: "Label terminal", args: argsSpec{
			"terminal": kindString, "labels": kindStringsMap,
		}, run: runTerminalLabel},
		{id: "terminal.peek", title: "Peek at terminal", args: argsSpec{
			"terminal": kindString,
		}, run: runTerminalPeek},
		{id: "terminal.show", title: "Show terminal for now", args: argsSpec{
			"terminal": kindString, "pane": kindString,
		}, run: runTerminalShow},
		{id: "terminal.park", title: "Park terminal", args: argsSpec{
			"terminal": kindString,
		}, run: runTerminalPark},
		{id: "terminal.scroll", title: "Scroll terminal", args: argsSpec{
			"lines": kindInt, "pages": kindFloat, "to": kindString, "terminal": kindString,
		}, run: runTerminalScroll},
		{id: "terminal.paste_image", title: "Paste image into terminal", args: argsSpec{},
			run: runTerminalPasteImage},
		{id: "terminal.right_click", title: "Right-click policy", args: argsSpec{
			"to": kindString, "terminal": kindString,
		}, run: runTerminalRightClick},
		{id: api.CmdAgentNext, title: "Next agent", args: argsSpec{}, run: runAgentNext},
		{id: api.CmdAgentPrev, title: "Previous agent", args: argsSpec{}, run: runAgentPrev},
		{id: api.CmdAgentFocus, title: "Focus agent", args: argsSpec{
			"address": kindString, "terminal": kindString, "agent": kindString,
		}, run: runAgentFocus},
		{id: api.CmdAgentPin, title: "Pin agent", args: argsSpec{
			"address": kindString, "terminal": kindString, "agent": kindString,
			"pinned": kindBool,
		}, run: runAgentPin},
		{id: api.CmdAgentPeek, title: "Peek at agent", args: argsSpec{
			"address": kindString, "terminal": kindString, "agent": kindString,
		}, run: runAgentPeek},
		{id: api.CmdPalette, title: "Command palette", args: argsSpec{}, run: overlayCommand("palette")},
		{id: api.CmdSettings, title: "Settings", args: argsSpec{}, run: overlayCommand("settings")},
		{id: api.CmdAbout, title: "About hiik", args: argsSpec{}, run: overlayCommand("about")},
		{id: "app.help", title: "Keys and help", args: argsSpec{}, run: overlayCommand("help")},
		{id: "app.welcome", title: "Show welcome", args: argsSpec{}, run: overlayCommand("welcome")},
		{id: "app.set", title: "Set setting", args: argsSpec{
			"key": kindString, "value": kindAny,
		}, run: runAppSet},
		{id: api.CmdRetryCore, title: "Retry core connection", args: argsSpec{}, run: runRetryCore},
	}
	a.cmds = append(a.cmds, extraCommands...)
	a.byID = make(map[string]*cmd, len(a.cmds))
	for _, c := range a.cmds {
		if _, exists := a.byID[c.id]; exists {
			continue
		}
		a.byID[c.id] = c
	}
}

// --- Space commands ---

func runSpaceNew(a *App, args map[string]any) error {
	c, err := a.coreClient()
	if err != nil {
		return err
	}
	name := argString(args, "name")
	folder := argString(args, "folder")
	if folder == "" {
		a.mu.RLock()
		if si := a.model.spaceIndex(a.model.activeSpace); si >= 0 {
			folder = a.model.spaces[si].Folder
		}
		a.mu.RUnlock()
	}
	if folder == "" {
		folder, _ = os.UserHomeDir()
	}
	machine := argString(args, "machine")
	argv := argStrings(args, "argv")

	a.mu.Lock()
	spaceID, tabID, paneID := a.model.createSpace(name, folder, machine)
	a.mu.Unlock()

	if err := a.startTerminal(c, spaceID, tabID, paneID, folder, machine, argv, ""); err != nil {
		a.mu.Lock()
		_, _ = a.model.closeSpace(spaceID)
		a.mu.Unlock()
		a.persistState()
		return err
	}
	a.persistState()
	return nil
}

func runSpaceClose(a *App, args map[string]any) error {
	a.mu.Lock()
	id := spaceArg(args)
	if id == "" {
		id = a.model.activeSpace
	}
	terms, err := a.model.closeSpace(id)
	a.mu.Unlock()
	if err != nil {
		return err
	}
	a.persistState()
	return a.kill(terms)
}

func runSpaceRename(a *App, args map[string]any) error {
	a.mu.Lock()
	id := spaceArg(args)
	if id == "" {
		id = a.model.activeSpace
	}
	err := a.model.renameSpace(id, argString(args, "name"))
	a.mu.Unlock()
	if err != nil {
		return err
	}
	a.persistState()
	return nil
}

func runSpaceMove(a *App, args map[string]any) error {
	id, delta, to, err := parseMove(args, "space")
	if err != nil {
		return err
	}
	a.mu.Lock()
	if id == "" {
		id = spaceArg(args)
	}
	if id == "" {
		id = a.model.activeSpace
	}
	err = a.model.moveSpace(id, delta, to)
	a.mu.Unlock()
	if err != nil {
		return err
	}
	a.persistState()
	return nil
}

func runSpaceSelect(a *App, args map[string]any) error {
	id := spaceArg(args)
	delta, hasDelta := argInt(args, "delta")
	if id != "" && hasDelta {
		return fmt.Errorf("%s: give id or delta, not both", api.CmdSpaceSelect)
	}
	a.mu.Lock()
	var err error
	if id != "" {
		err = a.model.selectSpace(id)
	} else {
		if !hasDelta {
			delta = 1
		}
		err = a.model.selectSpaceDelta(delta)
	}
	a.mu.Unlock()
	if err != nil {
		return err
	}
	a.persistState()
	return nil
}

// --- Tab commands ---

func runTabNew(a *App, args map[string]any) error {
	c, err := a.coreClient()
	if err != nil {
		return err
	}

	a.mu.Lock()
	spaceID := argString(args, "space")
	if spaceID == "" {
		spaceID = a.model.activeSpace
	}
	si := a.model.spaceIndex(spaceID)
	if si < 0 {
		a.mu.Unlock()
		return fmt.Errorf("space %q not found", spaceID)
	}
	folder := argString(args, "folder")
	if folder == "" {
		folder = a.model.spaces[si].Folder
	}
	machine := a.model.spaces[si].Machine
	tabID, paneID, err := a.model.createTab(spaceID, argString(args, "name"))
	a.mu.Unlock()
	if err != nil {
		return err
	}

	if err := a.startTerminal(c, spaceID, tabID, paneID, folder, machine, argStrings(args, "argv"), argString(args, "task")); err != nil {
		a.mu.Lock()
		_, _ = a.model.closeTab(spaceID, tabID)
		a.mu.Unlock()
		a.persistState()
		return err
	}
	a.persistState()
	return nil
}

func runTabClose(a *App, args map[string]any) error {
	a.mu.Lock()
	spaceID, tabID := a.tabTarget(args)
	terms, err := a.model.closeTab(spaceID, tabID)
	a.mu.Unlock()
	if err != nil {
		return err
	}
	a.persistState()
	return a.kill(terms)
}

func runTabRename(a *App, args map[string]any) error {
	a.mu.Lock()
	spaceID, tabID := a.tabTarget(args)
	err := a.model.renameTab(spaceID, tabID, argString(args, "name"))
	a.mu.Unlock()
	if err != nil {
		return err
	}
	a.persistState()
	return nil
}

func runTabMove(a *App, args map[string]any) error {
	_, delta, to, err := parseMove(args, "tab")
	if err != nil {
		return err
	}
	a.mu.Lock()
	spaceID, tabID := a.tabTarget(args)
	err = a.model.moveTab(spaceID, tabID, delta, to)
	a.mu.Unlock()
	if err != nil {
		return err
	}
	a.persistState()
	return nil
}

func runTabSelect(a *App, args map[string]any) error {
	spaceID := argString(args, "space")
	tabID := argString(args, "tab")
	if tabID == "" {
		tabID = argString(args, "id")
	}
	delta, hasDelta := argInt(args, "delta")
	if tabID != "" && hasDelta {
		return fmt.Errorf("%s: give tab or delta, not both", api.CmdTabSelect)
	}
	a.mu.Lock()
	if spaceID == "" {
		spaceID = a.model.activeSpace
	}
	var err error
	if tabID != "" {
		err = a.model.selectTab(spaceID, tabID)
	} else {
		if !hasDelta {
			delta = 1
		}
		err = a.model.selectTabDelta(spaceID, delta)
	}
	a.mu.Unlock()
	if err != nil {
		return err
	}
	a.persistState()
	return nil
}

// tabTarget returns the space and tab IDs a tab command addresses.
func (a *App) tabTarget(args map[string]any) (spaceID, tabID string) {
	spaceID = argString(args, "space")
	if spaceID == "" {
		spaceID = a.model.activeSpace
	}
	tabID = argString(args, "tab")
	if tabID == "" {
		tabID = argString(args, "id")
	}
	return spaceID, tabID
}

// tabLocation resolves the indexes a tab command addresses.
func (a *App) tabLocation(args map[string]any) (si, ti int, err error) {
	spaceID, tabID := a.tabTarget(args)
	si = a.model.spaceIndex(spaceID)
	if si < 0 {
		return -1, -1, fmt.Errorf("space %q not found", spaceID)
	}
	ti = tabIndex(&a.model.spaces[si], tabID)
	if ti < 0 {
		return -1, -1, fmt.Errorf("tab %q not found", tabID)
	}
	return si, ti, nil
}

func runTabSettle(a *App, args map[string]any) error {
	a.mu.Lock()
	si, ti, err := a.tabLocation(args)
	if err == nil {
		a.model.settleTab(si, ti)
	}
	a.mu.Unlock()
	if err != nil {
		return err
	}
	a.persistState()
	return nil
}

func runTabUnsettle(a *App, args map[string]any) error {
	a.mu.Lock()
	si, ti, err := a.tabLocation(args)
	if err == nil {
		a.model.unsettleTab(si, ti)
	}
	a.mu.Unlock()
	if err != nil {
		return err
	}
	a.persistState()
	return nil
}

// --- Pane commands ---

func runPaneSplit(a *App, args map[string]any) error {
	direction := argString(args, "direction")
	if direction == "" {
		direction = argString(args, "dir")
	}
	vertical, err := splitDirection(direction)
	if err != nil {
		return err
	}
	terminal := argString(args, "terminal")
	mod := argString(args, "mod")
	var props map[string]any
	if raw, ok := args["props"]; ok && raw != nil {
		var good bool
		props, good = raw.(map[string]any)
		if !good {
			return fmt.Errorf("pane.split: props must be an object")
		}
	}
	if terminal != "" && mod != "" {
		return fmt.Errorf("pane.split: give terminal or mod, not both")
	}
	if mod != "" {
		if _, ok := api.Panels()[mod]; !ok {
			return fmt.Errorf("pane.split: unknown mod panel %q", mod)
		}
	}

	var c hc.Client
	if terminal == "" && mod == "" {
		c, err = a.coreClient()
		if err != nil {
			return err
		}
	}

	a.mu.Lock()
	paneID, err := a.model.resolvePane(paneArg(args))
	if err != nil {
		a.mu.Unlock()
		return err
	}
	si, ti, ok := a.model.paneLocation(paneID)
	if !ok {
		a.mu.Unlock()
		return fmt.Errorf("pane %q not found", paneID)
	}
	sp := &a.model.spaces[si]
	spaceID := sp.ID
	tabID := sp.Tabs[ti].ID
	cwd := sp.Folder
	machine := sp.Machine
	newPaneID, err := a.model.splitPane(paneID, vertical)
	if err == nil && terminal != "" {
		a.model.setTerminal(newPaneID, terminal)
	}
	if err == nil && mod != "" {
		err = a.model.setPaneMod(newPaneID, mod, props)
	}
	a.mu.Unlock()
	if err != nil {
		return err
	}

	if terminal == "" && mod == "" {
		if err := a.startTerminal(c, spaceID, tabID, newPaneID, cwd, machine, nil, ""); err != nil {
			a.mu.Lock()
			_, _ = a.model.closePane(newPaneID)
			a.mu.Unlock()
			a.persistState()
			return err
		}
	}
	a.persistState()
	return nil
}

func runPaneClose(a *App, args map[string]any) error {
	a.mu.Lock()
	target := paneArg(args)
	if target == "" {
		if term := argString(args, "terminal"); term != "" {
			id, ok := a.model.paneByTerminal(term)
			if !ok {
				a.mu.Unlock()
				return fmt.Errorf("pane with terminal %q not found", term)
			}
			target = id
		}
	}
	paneID, err := a.model.resolvePane(target)
	if err != nil {
		a.mu.Unlock()
		return err
	}
	terms, err := a.model.closePane(paneID)
	a.mu.Unlock()
	if err != nil {
		return err
	}
	a.persistState()
	return a.kill(terms)
}

func runPaneResize(a *App, args map[string]any) error {
	ratio, ok := argFloat(args, "ratio")
	if !ok {
		return fmt.Errorf("pane.resize: ratio is required")
	}
	a.mu.Lock()
	target := paneArg(args)
	if target == "" {
		target = a.model.activeTabFocus()
		if target == "" {
			a.mu.Unlock()
			return fmt.Errorf("pane.resize: no focused pane")
		}
	}
	err := a.model.setRatio(target, float32(ratio))
	a.mu.Unlock()
	if err != nil {
		return err
	}
	a.persistState()
	return nil
}

func runPaneZoom(a *App, args map[string]any) error {
	a.mu.Lock()
	paneID, err := a.model.resolvePane(paneArg(args))
	if err != nil {
		a.mu.Unlock()
		return err
	}
	_, err = a.model.zoomPane(paneID)
	a.mu.Unlock()
	if err != nil {
		return err
	}
	a.persistState()
	return nil
}

func runPaneFocus(a *App, args map[string]any) error {
	a.mu.Lock()
	paneID, err := a.model.resolvePane(paneArg(args))
	if err == nil {
		err = a.model.focusPane(paneID)
	}
	a.mu.Unlock()
	if err != nil {
		return err
	}
	a.persistState()
	return nil
}

func runPaneGo(a *App, args map[string]any) error {
	direction := argString(args, "direction")
	if direction == "" {
		direction = argString(args, "dir")
	}
	if err := checkDirection(api.CmdPaneGo, direction); err != nil {
		return err
	}
	a.mu.Lock()
	paneID, err := a.model.resolvePane(paneArg(args))
	if err == nil {
		_, err = a.model.goPane(paneID, direction)
	}
	a.mu.Unlock()
	if err != nil {
		return err
	}
	a.persistState()
	return nil
}

func runPaneSwap(a *App, args map[string]any) error {
	direction := argString(args, "direction")
	if direction != "" {
		if err := checkDirection(api.CmdPaneSwap, direction); err != nil {
			return err
		}
	}

	a.mu.Lock()
	source := paneArg(args)
	if source == "" {
		source = argString(args, "a")
	}
	aID, err := a.model.resolvePane(source)
	if err != nil {
		a.mu.Unlock()
		return err
	}
	bID := argString(args, "b")
	if bID == "" {
		if direction == "" {
			a.mu.Unlock()
			return fmt.Errorf("%s: give b or direction", api.CmdPaneSwap)
		}
		si, ti, ok := a.model.paneLocation(aID)
		if !ok {
			a.mu.Unlock()
			return fmt.Errorf("pane %q not found", aID)
		}
		var found bool
		bID, found = neighbor(&a.model.spaces[si].Tabs[ti], aID, direction)
		if !found {
			a.mu.Unlock()
			return fmt.Errorf("%s: no pane to the %s", api.CmdPaneSwap, direction)
		}
	}
	err = a.model.swapPanes(aID, bID)
	a.mu.Unlock()
	if err != nil {
		return err
	}
	a.persistState()
	return nil
}

// --- Terminal commands ---

// keepTerminalLocked gives a running terminal a pane. It never starts a
// program: the core already runs it.
func (a *App) keepTerminalLocked(address, place string) error {
	if _, ok := a.model.paneByTerminal(address); ok {
		a.model.focusTerminal(address)
		return nil
	}
	if place == "tab" && len(a.model.spaces) > 0 {
		_, paneID, err := a.model.createTab(a.model.activeSpace, "")
		if err != nil {
			return err
		}
		a.model.setTerminal(paneID, address)
		return nil
	}
	if len(a.model.spaces) == 0 {
		folder, _ := os.UserHomeDir()
		_, _, paneID := a.model.createSpace("", folder, "")
		a.model.setTerminal(paneID, address)
		return nil
	}
	paneID, err := a.model.resolvePane("")
	if err != nil {
		_, paneID, cerr := a.model.createTab(a.model.activeSpace, "")
		if cerr != nil {
			return cerr
		}
		a.model.setTerminal(paneID, address)
		return nil
	}
	newPaneID, err := a.model.splitPane(paneID, place == "down")
	if err != nil {
		return err
	}
	a.model.setTerminal(newPaneID, address)
	return nil
}

func runTerminalKeep(a *App, args map[string]any) error {
	address := argString(args, "terminal")
	if address == "" {
		return fmt.Errorf("terminal.keep: terminal is required")
	}
	place := argString(args, "place")
	switch place {
	case "", "right", "split", "down", "tab", "here", "focus":
	default:
		return fmt.Errorf("terminal.keep: unknown place %q", place)
	}
	a.mu.Lock()
	err := a.keepTerminalLocked(address, place)
	a.mu.Unlock()
	if err != nil {
		return err
	}
	a.persistState()
	return nil
}

func runTerminalKill(a *App, args map[string]any) error {
	address := argString(args, "terminal")
	if address == "" {
		return fmt.Errorf("terminal.kill: terminal is required")
	}
	c, err := a.coreClient()
	if err != nil {
		return err
	}
	ctx, cancel := a.callContext()
	err = c.Kill(ctx, address)
	cancel()
	if err != nil {
		return fmt.Errorf("kill %s: %w", address, err)
	}
	return nil
}

func runTerminalLabel(a *App, args map[string]any) error {
	address := argString(args, "terminal")
	if address == "" {
		return fmt.Errorf("terminal.label: terminal is required")
	}
	labels, ok := argLabels(args, "labels")
	if !ok {
		return fmt.Errorf("terminal.label: labels must be a map of strings")
	}
	c, err := a.coreClient()
	if err != nil {
		return err
	}
	ctx, cancel := a.callContext()
	err = c.Label(ctx, address, labels)
	cancel()
	if err != nil {
		return fmt.Errorf("label %s: %w", address, err)
	}
	return nil
}

func runTerminalPeek(a *App, args map[string]any) error {
	address := argString(args, "terminal")
	a.mu.Lock()
	a.overlay = "peek"
	if address != "" && a.model.agentIndex(address) >= 0 {
		a.model.activeAgent = address
	}
	a.mu.Unlock()
	return nil
}

// runTerminalShow shows a running terminal in a pane in place of that pane's
// own terminal. With no terminal it gives the pane its own terminal back.
func runTerminalShow(a *App, args map[string]any) error {
	address := argString(args, "terminal")
	paneArg := argString(args, "pane")

	a.mu.Lock()
	target, err := a.model.resolvePane(paneArg)
	if err != nil {
		a.mu.Unlock()
		return err
	}
	if address == "" {
		a.model.restoreShown(target)
		a.mu.Unlock()
		a.persistState()
		return nil
	}
	if !a.model.terminalKnown(address) {
		a.mu.Unlock()
		return fmt.Errorf("terminal.show: terminal %q is not running", address)
	}
	if own, ok := a.model.owningPane(address); ok {
		// A terminal with a pane of its own is shown there.
		a.model.restoreShown(target)
		err = a.model.focusPane(own)
	} else {
		err = a.model.showInPane(target, address)
	}
	a.mu.Unlock()
	if err != nil {
		return err
	}
	a.persistState()
	return nil
}

// runTerminalPark takes a terminal's pane away and turns core keep off; the
// terminal keeps running.
func runTerminalPark(a *App, args map[string]any) error {
	address := argString(args, "terminal")
	if address == "" {
		return fmt.Errorf("terminal.park: terminal is required")
	}

	a.mu.RLock()
	c := a.core
	a.mu.RUnlock()
	if c != nil {
		ctx, cancel := a.callContext()
		err := c.Label(ctx, address, map[string]string{labelSpace: "", labelTab: ""})
		if err == nil {
			err = c.Call(ctx, "terminal.keep", map[string]any{
				"terminal": address,
				"keep":     false,
			}, nil)
		}
		cancel()
		if err != nil {
			return fmt.Errorf("terminal.park: %w", err)
		}
	}

	a.mu.Lock()
	if paneID, ok := a.model.owningPane(address); ok {
		_, _ = a.model.closePane(paneID)
	} else if paneID, ok := a.model.paneByTerminal(address); ok {
		// It was only shown in another pane; give that pane its own back.
		a.model.restoreShown(paneID)
	}
	a.model.setTerminalLabel(address, labelSpace, "")
	a.model.setTerminalLabel(address, labelTab, "")
	a.mu.Unlock()
	a.persistState()
	return nil
}

// runTerminalScroll scrolls a terminal's view of its history. Exactly one of
// lines, pages or to is required.
func runTerminalScroll(a *App, args map[string]any) error {
	lines, pages, to, err := scrollTarget(args)
	if err != nil {
		return err
	}
	address := argString(args, "terminal")
	a.mu.RLock()
	if address == "" {
		address = a.model.focusedTerminal()
	}
	fn := a.scroller
	a.mu.RUnlock()
	if address == "" {
		return fmt.Errorf("terminal.scroll: no focused terminal to scroll")
	}
	if fn == nil {
		return nil
	}
	if !fn(address, lines, pages, to) {
		return fmt.Errorf("terminal.scroll: terminal %q is not attached", address)
	}
	return nil
}

// scrollTarget parses the one scroll amount a terminal.scroll call gives.
func scrollTarget(args map[string]any) (lines, pages float32, to string, err error) {
	given := make([]string, 0, 3)
	for _, key := range [...]string{"lines", "pages", "to"} {
		if value, ok := args[key]; ok && value != nil {
			given = append(given, key)
		}
	}
	switch len(given) {
	case 0:
		return 0, 0, "", fmt.Errorf("terminal.scroll needs one of lines, pages, to")
	case 1:
	default:
		return 0, 0, "", fmt.Errorf("terminal.scroll takes one of lines, pages, to, not %s", strings.Join(given, " and "))
	}
	switch given[0] {
	case "lines":
		n, ok := argInt(args, "lines")
		if !ok {
			return 0, 0, "", fmt.Errorf("terminal.scroll lines is not a whole number")
		}
		return float32(n), 0, "", nil
	case "pages":
		p, ok := argFloat(args, "pages")
		if !ok {
			return 0, 0, "", fmt.Errorf("terminal.scroll pages is not a finite number")
		}
		return 0, float32(p), "", nil
	default:
		to = argString(args, "to")
		if to != "top" && to != "bottom" {
			return 0, 0, "", fmt.Errorf("terminal.scroll to is \"top\" or \"bottom\", not %q", to)
		}
		return 0, 0, to, nil
	}
}

// runTerminalPasteImage pastes the clipboard's image into the focused
// terminal through the shell's hook.
func runTerminalPasteImage(a *App, args map[string]any) error {
	a.mu.RLock()
	address := a.model.focusedTerminal()
	fn := a.paster
	a.mu.RUnlock()
	if address == "" {
		return fmt.Errorf("terminal.paste_image: no focused terminal")
	}
	if fn == nil {
		return nil
	}
	if err := fn(address); err != nil {
		return fmt.Errorf("terminal.paste_image: %w", err)
	}
	return nil
}

// runTerminalRightClick records where right-clicks go in a terminal: to its
// program or to the app's menu. The core keeps the label; the shell's live
// terminal view is told as well.
func runTerminalRightClick(a *App, args map[string]any) error {
	to := argString(args, "to")
	switch to {
	case "program", "menu":
	default:
		return fmt.Errorf("terminal.right_click to is \"program\" or \"menu\", not %q", to)
	}
	address := argString(args, "terminal")
	a.mu.RLock()
	if address == "" {
		address = a.model.focusedTerminal()
	}
	known := address != "" && a.model.terminalKnown(address)
	a.mu.RUnlock()
	if !known {
		return fmt.Errorf("terminal.right_click: no such running terminal")
	}

	a.mu.RLock()
	c := a.core
	a.mu.RUnlock()
	if c != nil {
		ctx, cancel := a.callContext()
		err := c.Label(ctx, address, map[string]string{labelRightClick: to})
		cancel()
		if err != nil {
			return fmt.Errorf("terminal.right_click: %w", err)
		}
	}

	a.mu.Lock()
	a.model.setTerminalLabel(address, labelRightClick, to)
	fn := a.rightClick
	a.mu.Unlock()
	if fn != nil {
		fn(address, to)
	}
	return nil
}

// --- Agent commands ---

func runAgentNext(a *App, args map[string]any) error { return a.moveAgent(1) }

func runAgentPrev(a *App, args map[string]any) error { return a.moveAgent(-1) }

func (a *App) moveAgent(delta int) error {
	a.mu.Lock()
	ag, ok := a.model.moveAgent(delta)
	if ok && ag.Address != "" {
		a.model.focusTerminal(ag.Address)
	}
	a.mu.Unlock()
	return nil
}

func runAgentFocus(a *App, args map[string]any) error {
	a.mu.Lock()
	address := agentArg(args)
	if address == "" {
		address = a.model.activeAgent
	}
	if address == "" {
		a.mu.Unlock()
		return fmt.Errorf("%s: no agent selected", api.CmdAgentFocus)
	}
	err := a.model.focusAgent(address)
	var turn, seenTurn uint64
	if err == nil {
		if ag := a.model.agent(address); ag != nil {
			turn, seenTurn = ag.Turn, ag.SeenTurn
		}
	}
	a.mu.Unlock()
	if err != nil {
		return err
	}
	// Record the turn the user has seen. The label only goes up, so a later
	// focus of an older turn writes nothing.
	if turn == 0 || turn <= seenTurn {
		return nil
	}
	if err := a.setAgentLabel(address, map[string]string{
		labelSeen: strconv.FormatUint(turn, 10),
	}); err != nil {
		return fmt.Errorf("%s: %w", api.CmdAgentFocus, err)
	}
	a.mu.Lock()
	if ag := a.model.agent(address); ag != nil && ag.Turn <= turn {
		ag.SeenTurn = turn
		ag.Seen = true
	}
	a.mu.Unlock()
	return nil
}

func runAgentPin(a *App, args map[string]any) error {
	a.mu.Lock()
	address := agentArg(args)
	if address == "" {
		address = a.model.activeAgent
	}
	if address == "" {
		a.mu.Unlock()
		return fmt.Errorf("%s: no agent selected", api.CmdAgentPin)
	}
	var (
		err    error
		pinned bool
	)
	if want, ok := argBool(args, "pinned"); ok {
		err = a.model.setAgentPinned(address, want)
		pinned = want
	} else {
		pinned, err = a.model.pinAgent(address)
	}
	if ag := a.model.agent(address); err == nil && ag != nil {
		pinned = ag.Pinned
	}
	a.mu.Unlock()
	if err != nil {
		return err
	}
	labels := map[string]string{labelPinned: "1"}
	if !pinned {
		// An empty value removes the label.
		labels[labelPinned] = ""
	}
	if err := a.setAgentLabel(address, labels); err != nil {
		// The core refused the durable record; keep the list as it was.
		a.mu.Lock()
		_ = a.model.setAgentPinned(address, !pinned)
		a.mu.Unlock()
		return fmt.Errorf("%s: %w", api.CmdAgentPin, err)
	}
	return nil
}

func runAgentPeek(a *App, args map[string]any) error {
	a.mu.Lock()
	address := agentArg(args)
	if address == "" {
		address = a.model.activeAgent
	}
	if address == "" {
		a.mu.Unlock()
		return fmt.Errorf("%s: no agent selected", api.CmdAgentPeek)
	}
	if a.model.agentIndex(address) < 0 {
		a.mu.Unlock()
		return fmt.Errorf("agent %q not found", address)
	}
	a.model.activeAgent = address
	a.overlay = "peek"
	a.mu.Unlock()
	return nil
}

// --- App commands ---

func overlayCommand(name string) func(*App, map[string]any) error {
	return func(a *App, _ map[string]any) error {
		a.mu.Lock()
		a.overlay = name
		a.mu.Unlock()
		return nil
	}
}

func runRetryCore(a *App, _ map[string]any) error {
	a.mu.Lock()
	c := a.core
	running := a.runCancel != nil
	a.mu.Unlock()
	if c != nil {
		return a.Start(context.Background())
	}
	if running {
		return nil
	}
	a.SetConnection(Connecting, "")
	go func() { _ = a.Connect(context.Background()) }()
	return nil
}

// runAppSet changes one settings toggle from the settings panel.
func runAppSet(a *App, args map[string]any) error {
	key := argString(args, "key")
	if key == "" {
		return fmt.Errorf("app.set: key must not be empty")
	}
	value, ok := args["value"]
	if !ok || value == nil {
		return fmt.Errorf("app.set: value must not be empty")
	}

	asBool := func() (bool, error) {
		b, ok := value.(bool)
		if !ok {
			return false, fmt.Errorf("app.set: %s needs a bool", key)
		}
		return b, nil
	}

	a.mu.Lock()
	defer func() { a.mu.Unlock() }()
	switch key {
	case "notify_blocked":
		b, err := asBool()
		if err != nil {
			return err
		}
		a.settings.NotifyBlocked = b
	case "notify_done":
		b, err := asBool()
		if err != nil {
			return err
		}
		a.settings.NotifyDone = b
	case "sound":
		b, err := asBool()
		if err != nil {
			return err
		}
		a.settings.Sound = b
	case "telemetry":
		b, err := asBool()
		if err != nil {
			return err
		}
		a.settings.Privacy.Telemetry = b
	case "clipboard_write":
		switch v := value.(type) {
		case bool:
			if v {
				a.settings.Privacy.ClipboardWrite = store.ClipboardAlways
			} else {
				a.settings.Privacy.ClipboardWrite = store.ClipboardNever
			}
		case string:
			switch v {
			case store.ClipboardAlways, store.ClipboardAsk, store.ClipboardNever:
				a.settings.Privacy.ClipboardWrite = v
			default:
				return fmt.Errorf("app.set: clipboard_write %q is unknown", v)
			}
		default:
			return fmt.Errorf("app.set: clipboard_write needs a bool or a policy string")
		}
	default:
		if err := setLookValue(&a.settings, key, value); err != nil {
			if errors.Is(err, errNotLookKey) {
				return fmt.Errorf("app.set: unknown key %q", key)
			}
			return err
		}
	}
	normalizeLook(&a.settings)
	a.settings.Normalize()
	a.persistSettingsLocked()
	return nil
}

// persistSettingsLocked is persistSettings for callers holding the lock.
func (a *App) persistSettingsLocked() {
	st := a.store
	settings := a.settings
	if st != nil {
		st.SaveSettings(settings)
	}
}

// --- Core helpers ---

// startTerminal runs a program in the core and records its address on a
// pane. The labels let a later start adopt the terminal.
func (a *App) startTerminal(c hc.Client, spaceID, tabID, paneID, cwd, machine string, argv []string, task string) error {
	req := hc.RunRequest{
		Argv: argv,
		Cwd:  cwd,
		On:   machine,
		Task: task,
		Labels: map[string]string{
			labelSpace: spaceID,
			labelTab:   tabID,
		},
	}
	ctx, cancel := a.callContext()
	term, err := c.Run(ctx, req)
	cancel()
	if err != nil {
		return fmt.Errorf("start terminal: %w", err)
	}

	a.mu.Lock()
	kept := a.model.setTerminal(paneID, term.Address)
	a.mu.Unlock()
	if !kept && term.Address != "" {
		// The pane disappeared while the program started.
		ctx, cancel := a.callContext()
		_ = c.Kill(ctx, term.Address)
		cancel()
	}
	return nil
}

// kill ends terminals. Without a core the model still closes.
func (a *App) kill(addresses []string) error {
	if len(addresses) == 0 {
		return nil
	}
	a.mu.RLock()
	c := a.core
	a.mu.RUnlock()
	if c == nil {
		return nil
	}
	var first error
	for _, address := range addresses {
		if address == "" {
			continue
		}
		ctx, cancel := a.callContext()
		err := c.Kill(ctx, address)
		cancel()
		if err != nil && first == nil {
			first = fmt.Errorf("kill %s: %w", address, err)
		}
	}
	return first
}

func (a *App) callContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), callTimeout)
}

// --- Arguments ---

type argsKind int

const (
	kindString argsKind = iota
	kindInt
	kindBool
	kindStrings
	kindFloat
	kindStringsMap
	kindAny
)

type argsSpec map[string]argsKind

func (s argsSpec) validate(command string, args map[string]any) error {
	for key, value := range args {
		kind, ok := s[key]
		if !ok {
			return fmt.Errorf("command %s: unknown argument %q", command, key)
		}
		if value == nil {
			return fmt.Errorf("command %s: argument %q must not be nil", command, key)
		}
		switch kind {
		case kindString:
			if _, ok := value.(string); !ok {
				return fmt.Errorf("command %s: argument %q must be a string", command, key)
			}
		case kindInt:
			if _, ok := intValue(value); !ok {
				return fmt.Errorf("command %s: argument %q must be an integer", command, key)
			}
		case kindBool:
			if _, ok := value.(bool); !ok {
				return fmt.Errorf("command %s: argument %q must be a bool", command, key)
			}
		case kindStrings:
			if _, ok := stringSlice(value); !ok {
				return fmt.Errorf("command %s: argument %q must be a list of strings", command, key)
			}
		case kindFloat:
			if _, ok := floatNumber(value); !ok {
				return fmt.Errorf("command %s: argument %q must be a number", command, key)
			}
		case kindStringsMap:
			if _, ok := stringMap(value); !ok {
				return fmt.Errorf("command %s: argument %q must be a map of strings", command, key)
			}
		case kindAny:
			// Any non-nil value; the command checks its shape.
		}
	}
	return nil
}

func intValue(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int8:
		return int(n), true
	case int16:
		return int(n), true
	case int32:
		return int(n), true
	case int64:
		if int64(int(n)) != n {
			return 0, false
		}
		return int(n), true
	case uint8:
		return int(n), true
	case uint16:
		return int(n), true
	case uint32:
		if uint64(n) > math.MaxInt {
			return 0, false
		}
		return int(n), true
	case uint64:
		if n > math.MaxInt {
			return 0, false
		}
		return int(n), true
	case float32:
		return floatValue(float64(n))
	case float64:
		return floatValue(n)
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return 0, false
		}
		return intValue(i)
	default:
		return 0, false
	}
}

func floatValue(v float64) (int, bool) {
	if math.IsNaN(v) || math.IsInf(v, 0) || math.Trunc(v) != v {
		return 0, false
	}
	i := int(v)
	if float64(i) != v {
		return 0, false
	}
	return i, true
}

func stringSlice(v any) ([]string, bool) {
	switch s := v.(type) {
	case []string:
		return s, true
	case []any:
		out := make([]string, len(s))
		for i, item := range s {
			str, ok := item.(string)
			if !ok {
				return nil, false
			}
			out[i] = str
		}
		return out, true
	default:
		return nil, false
	}
}

func stringMap(v any) (map[string]string, bool) {
	switch m := v.(type) {
	case map[string]string:
		return m, true
	case map[string]any:
		out := make(map[string]string, len(m))
		for key, item := range m {
			str, ok := item.(string)
			if !ok {
				return nil, false
			}
			out[key] = str
		}
		return out, true
	default:
		return nil, false
	}
}

func argString(args map[string]any, key string) string {
	s, _ := args[key].(string)
	return s
}

func argInt(args map[string]any, key string) (int, bool) {
	v, ok := args[key]
	if !ok || v == nil {
		return 0, false
	}
	return intValue(v)
}

func argStrings(args map[string]any, key string) []string {
	out, _ := stringSlice(args[key])
	return out
}

func floatNumber(v any) (float64, bool) {
	var f float64
	switch n := v.(type) {
	case float64:
		f = n
	case float32:
		f = float64(n)
	case int:
		f = float64(n)
	case int8:
		f = float64(n)
	case int16:
		f = float64(n)
	case int32:
		f = float64(n)
	case int64:
		f = float64(n)
	case uint8:
		f = float64(n)
	case uint16:
		f = float64(n)
	case uint32:
		f = float64(n)
	case uint64:
		f = float64(n)
	case json.Number:
		parsed, err := n.Float64()
		if err != nil {
			return 0, false
		}
		f = parsed
	default:
		return 0, false
	}
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	return f, true
}

func argFloat(args map[string]any, key string) (float64, bool) {
	v, ok := args[key]
	if !ok || v == nil {
		return 0, false
	}
	return floatNumber(v)
}

func argBool(args map[string]any, key string) (bool, bool) {
	b, ok := args[key].(bool)
	return b, ok
}

func argLabels(args map[string]any, key string) (map[string]string, bool) {
	return stringMap(args[key])
}

// spaceArg returns a space ID from its canonical and alias keys.
func spaceArg(args map[string]any) string {
	if id := argString(args, "id"); id != "" {
		return id
	}
	return argString(args, "space")
}

// paneArg returns a pane or node ID from its canonical and alias keys.
func paneArg(args map[string]any) string {
	for _, key := range [...]string{"pane", "node", "id"} {
		if id := argString(args, key); id != "" {
			return id
		}
	}
	return ""
}

// agentArg returns an agent's terminal address from its alias keys.
func agentArg(args map[string]any) string {
	for _, key := range [...]string{"address", "terminal", "agent"} {
		if address := argString(args, key); address != "" {
			return address
		}
	}
	return ""
}

// parseMove parses the shared delta/to shape. Exactly one of delta and to
// must be present; otherwise the move is ambiguous. The returned id is the
// space.move "id" argument; tab commands ignore it.
func parseMove(args map[string]any, what string) (string, int, *int, error) {
	id := argString(args, "id")
	delta, hasDelta := argInt(args, "delta")
	to, hasTo := argInt(args, "to")
	switch {
	case hasDelta && hasTo:
		return id, 0, nil, fmt.Errorf("%s.move: give delta or to, not both", what)
	case !hasDelta && !hasTo:
		return id, 0, nil, fmt.Errorf("%s.move: needs delta or to", what)
	case hasTo:
		return id, 0, &to, nil
	default:
		return id, delta, nil, nil
	}
}

func splitDirection(name string) (vertical bool, err error) {
	switch name {
	case "", "right", "horizontal":
		return false, nil
	case "down", "vertical":
		return true, nil
	default:
		return false, fmt.Errorf("%s: direction must be right or down, not %q", api.CmdPaneSplit, name)
	}
}

func checkDirection(command, direction string) error {
	switch direction {
	case "left", "right", "up", "down":
		return nil
	case "":
		return fmt.Errorf("%s: direction is required", command)
	default:
		return fmt.Errorf("%s: direction must be left, right, up or down, not %q", command, direction)
	}
}
