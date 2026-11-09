package core

import (
	"context"
	"fmt"
	"strings"

	"hiik/mods/api"
)

// machineListResult and machineView mirror the core's machine.list reply.
// The core owns the machines; this file only caches what it reports.
type machineListResult struct {
	Machines []machineView `json:"machines"`
}

type machineView struct {
	Name      string `json:"name"`
	Target    string `json:"target"`
	ID        string `json:"id"`
	Host      string `json:"host"`
	Version   string `json:"version"`
	State     string `json:"state"`
	Why       string `json:"why"`
	Temporary bool   `json:"temporary"`
	Via       bool   `json:"via"`
}

func init() {
	registerCommand(&cmd{
		id:    api.CmdMachineList,
		title: "List machines",
		args:  argsSpec{},
		run:   runMachineList,
	})
	registerCommand(&cmd{
		id:    api.CmdMachineAdd,
		title: "Add machine",
		args: argsSpec{
			"name": kindString, "target": kindString, "host": kindString, "via": kindString,
		},
		run: runMachineAdd,
	})
	registerCommand(&cmd{
		id:    api.CmdMachineConnect,
		title: "Connect machine",
		args:  argsSpec{"machine": kindString, "name": kindString},
		run:   runMachineConnect,
	})
	registerCommand(&cmd{
		id:    api.CmdMachineRename,
		title: "Rename machine",
		args:  argsSpec{"machine": kindString, "new_name": kindString},
		run:   runMachineRename,
	})
	registerCommand(&cmd{
		id:    api.CmdMachineRm,
		title: "Remove machine",
		args:  argsSpec{"machine": kindString, "name": kindString},
		run:   runMachineRm,
	})
}

// Machines returns a snapshot of the machines the connected core reported.
func (a *App) Machines() []api.Machine {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return append([]api.Machine(nil), a.machines...)
}

// SetMachines replaces the machine cache with a snapshot from the core.
func (a *App) SetMachines(list []api.Machine) {
	a.mu.Lock()
	a.machines = append([]api.Machine(nil), list...)
	a.mu.Unlock()
	a.invalidateUI()
}

// RefreshMachines reloads the machine list from the connected core. It is a
// no-op when the app has no core, so callers need not check first.
func (a *App) RefreshMachines(ctx context.Context) error {
	a.mu.RLock()
	c := a.core
	a.mu.RUnlock()
	if c == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var res machineListResult
	if err := c.Call(ctx, api.CmdMachineList, nil, &res); err != nil {
		return fmt.Errorf("%s: %w", api.CmdMachineList, err)
	}
	list := make([]api.Machine, 0, len(res.Machines))
	for _, v := range res.Machines {
		list = append(list, machineFromView(v))
	}
	a.SetMachines(list)
	return nil
}

// machineFromView turns the core's view into the compact row the UI shows.
// A state the UI does not know is treated as unavailable rather than ready.
func machineFromView(v machineView) api.Machine {
	host := v.Host
	if host == "" {
		host = v.Target
	}
	errText := v.Why
	if errText == "" {
		switch v.State {
		case "needs_login":
			errText = "needs login"
		case "unreachable":
			errText = "unreachable"
		}
	}
	return api.Machine{
		Name:      v.Name,
		Host:      host,
		State:     machineState(v.State),
		Error:     errText,
		Temporary: v.Temporary,
	}
}

func machineState(state string) string {
	switch state {
	case "connected":
		return "ready"
	case "connecting":
		return "loading"
	default:
		return "unavailable"
	}
}

// machineCall runs one machine method and refreshes the cache afterwards, so
// a panel sees the core's new list on its next frame.
func (a *App) machineCall(method string, params map[string]any) error {
	c, err := a.coreClient()
	if err != nil {
		return fmt.Errorf("%s: %w", method, err)
	}
	ctx, cancel := a.callContext()
	err = c.Call(ctx, method, params, nil)
	cancel()
	if err != nil {
		return fmt.Errorf("%s: %w", method, err)
	}
	return a.RefreshMachines(nil)
}

func runMachineList(a *App, args map[string]any) error {
	if _, err := a.coreClient(); err != nil {
		return fmt.Errorf("%s: %w", api.CmdMachineList, err)
	}
	return a.RefreshMachines(nil)
}

func runMachineAdd(a *App, args map[string]any) error {
	target := strings.TrimSpace(argString(args, "target"))
	if target == "" {
		target = strings.TrimSpace(argString(args, "host"))
	}
	if target == "" {
		return fmt.Errorf("%s: target is required (user@host or an ssh Host)", api.CmdMachineAdd)
	}
	name := strings.TrimSpace(argString(args, "name"))
	via := strings.TrimSpace(argString(args, "via"))
	if via != "" && name == "" {
		return fmt.Errorf("%s: a via command needs a name", api.CmdMachineAdd)
	}
	params := map[string]any{"target": target}
	if name != "" {
		params["name"] = name
	}
	if via != "" {
		params["via"] = via
	}
	return a.machineCall(api.CmdMachineAdd, params)
}

func runMachineConnect(a *App, args map[string]any) error {
	name := machineNameArg(args)
	if name == "" {
		return fmt.Errorf("%s: machine is required", api.CmdMachineConnect)
	}
	return a.machineCall(api.CmdMachineConnect, map[string]any{"machine": name})
}

func runMachineRename(a *App, args map[string]any) error {
	name := machineNameArg(args)
	if name == "" {
		return fmt.Errorf("%s: machine is required", api.CmdMachineRename)
	}
	newName := strings.TrimSpace(argString(args, "new_name"))
	if newName == "" {
		return fmt.Errorf("%s: new name is required", api.CmdMachineRename)
	}
	return a.machineCall(api.CmdMachineRename, map[string]any{
		"machine":  name,
		"new_name": newName,
	})
}

func runMachineRm(a *App, args map[string]any) error {
	name := machineNameArg(args)
	if name == "" {
		return fmt.Errorf("%s: machine is required", api.CmdMachineRm)
	}
	return a.machineCall(api.CmdMachineRm, map[string]any{"machine": name})
}

// machineNameArg returns the machine the command names. "name" is an alias
// the palette and mods may use for the same value.
func machineNameArg(args map[string]any) string {
	if name := strings.TrimSpace(argString(args, "machine")); name != "" {
		return name
	}
	return strings.TrimSpace(argString(args, "name"))
}
