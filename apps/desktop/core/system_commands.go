package core

import (
	"context"
	"fmt"

	"hiik/mods/api"
)

// SetWindowAction installs the shell's window controller: close, minimize,
// maximize.
func (a *App) SetWindowAction(fn func(action string)) {
	a.mu.Lock()
	a.windowDo = fn
	a.mu.Unlock()
}

// SetAppAction installs the shell's app controller: quit, hide, hide_others,
// show_all.
func (a *App) SetAppAction(fn func(action string)) {
	a.mu.Lock()
	a.appDo = fn
	a.mu.Unlock()
}

// SetTerminalCopier installs the shell's copy action for a terminal.
func (a *App) SetTerminalCopier(fn func(address string) error) {
	a.mu.Lock()
	a.copier = fn
	a.mu.Unlock()
}

// SetTerminalPaster installs the shell's paste action for a terminal.
func (a *App) SetTerminalPaster(fn func(address string) error) {
	a.mu.Lock()
	a.textPaster = fn
	a.mu.Unlock()
}

func init() {
	registerCommand(&cmd{id: "space.next", title: "Next space", run: func(a *App, _ map[string]any) error {
		a.mu.Lock()
		err := a.model.selectSpaceDelta(1)
		a.mu.Unlock()
		a.persistState()
		return err
	}})
	registerCommand(&cmd{id: "space.prev", title: "Previous space", run: func(a *App, _ map[string]any) error {
		a.mu.Lock()
		err := a.model.selectSpaceDelta(-1)
		a.mu.Unlock()
		a.persistState()
		return err
	}})
	registerCommand(&cmd{id: "tab.next", title: "Next tab", run: func(a *App, _ map[string]any) error {
		return a.tabDelta(1)
	}})
	registerCommand(&cmd{id: "tab.prev", title: "Previous tab", run: func(a *App, _ map[string]any) error {
		return a.tabDelta(-1)
	}})
	registerCommand(&cmd{id: "terminal.open", title: "Open terminal", args: argsSpec{
		"argv": kindStrings, "folder": kindString, "place": kindString, "name": kindString,
	}, run: runTerminalOpen})
	registerCommand(&cmd{id: "terminal.pin", title: "Pin terminal", args: argsSpec{
		"terminal": kindString, "address": kindString, "pinned": kindBool,
	}, run: func(a *App, args map[string]any) error {
		addr := argString(args, "terminal")
		if addr == "" {
			addr = argString(args, "address")
		}
		return runAgentPin(a, map[string]any{"address": addr, "terminal": addr, "pinned": args["pinned"]})
	}})
	registerCommand(&cmd{id: "terminal.search", title: "Search terminal", run: func(a *App, _ map[string]any) error {
		a.SetOverlay("find")
		return nil
	}})
	registerCommand(&cmd{id: "terminal.copy", title: "Copy terminal selection", run: func(a *App, _ map[string]any) error {
		a.mu.RLock()
		fn := a.copier
		a.mu.RUnlock()
		if fn == nil {
			return fmt.Errorf("terminal.copy: the shell has no selection copier")
		}
		return fn(a.focusedTerminal())
	}})
	registerCommand(&cmd{id: "terminal.paste", title: "Paste into terminal", run: func(a *App, _ map[string]any) error {
		a.mu.RLock()
		fn := a.textPaster
		a.mu.RUnlock()
		if fn == nil {
			return fmt.Errorf("terminal.paste: the shell has no text paster")
		}
		return fn(a.focusedTerminal())
	}})
	for _, action := range []string{"close", "minimize", "maximize"} {
		action := action
		registerCommand(&cmd{id: "window." + action, title: "Window " + action, run: func(a *App, _ map[string]any) error {
			a.mu.RLock()
			fn := a.windowDo
			a.mu.RUnlock()
			if fn == nil {
				return fmt.Errorf("window.%s: no window is attached", action)
			}
			fn(action)
			return nil
		}})
	}
	for _, action := range []string{"quit", "hide", "hide_others", "show_all"} {
		action := action
		registerCommand(&cmd{id: "app." + action, title: "App " + action, run: func(a *App, _ map[string]any) error {
			a.mu.RLock()
			fn := a.appDo
			a.mu.RUnlock()
			if fn == nil {
				return fmt.Errorf("app.%s: no app is attached", action)
			}
			fn(action)
			return nil
		}})
	}
	registerCommand(&cmd{id: "app.update", title: "Restart into the downloaded update", run: func(a *App, _ map[string]any) error {
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		defer cancel()
		plan, err := a.InstallUpdate(ctx)
		if err != nil {
			return err
		}
		a.SetOverlay("install_update")
		return fmt.Errorf("hiik %s is ready at %s", plan.Version, plan.Path)
	}})
	registerCommand(&cmd{id: "overlay.close", title: "Close overlay", run: func(a *App, _ map[string]any) error {
		a.SetOverlay("")
		return nil
	}})
	registerCommand(&cmd{id: "app.reset", title: "Reset layout and keys", run: func(a *App, _ map[string]any) error {
		a.mu.Lock()
		a.settings.Layout = "herdr"
		a.settings.Keys = nil
		a.settings.Zoom = 1
		a.settings.Theme = ""
		a.model.ui = api.UIState{SidebarW: 240}
		a.mu.Unlock()
		a.persistSettings()
		a.persistState()
		a.SetOverlay("welcome")
		return nil
	}})
}

func (a *App) tabDelta(delta int) error {
	a.mu.Lock()
	err := a.model.selectTabDelta(a.model.activeSpace, delta)
	a.mu.Unlock()
	a.persistState()
	return err
}

func (a *App) focusedTerminal() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	pane := a.model.ui.Focus
	if pane == "" {
		pane = a.model.activeTabFocus()
	}
	si := a.model.spaceIndex(a.model.activeSpace)
	if si < 0 {
		return ""
	}
	sp := &a.model.spaces[si]
	if sp.Active < 0 || sp.Active >= len(sp.Tabs) {
		return ""
	}
	if n := findNode(sp.Tabs[sp.Active].Root, pane); n != nil {
		return n.Terminal
	}
	return ""
}

func runTerminalOpen(a *App, args map[string]any) error {
	place := argString(args, "place")
	if place == "" || place == "tab" {
		return runTabNew(a, args)
	}
	dir := place
	if dir != "right" && dir != "down" {
		return fmt.Errorf("terminal.open: place must be tab, right or down")
	}
	return runPaneSplit(a, map[string]any{
		"direction": dir,
		"argv":      args["argv"],
		"folder":    args["folder"],
	})
}
