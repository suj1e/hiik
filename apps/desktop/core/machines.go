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

// machineOS is what a probe learned about a machine. Name is the display
// name ("macOS", "Linux", "Windows") and Arch the Go-style architecture.
type machineOS struct {
	Name string
	Arch string
}

// machineKey identifies a machine's OS cache. A rename or a new host is a
// different key, so a stale answer never labels a different machine.
func machineKey(m api.Machine) string {
	return m.Name + "\x00" + m.Host
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

// SetMachines replaces the machine cache with a snapshot from the core and
// attaches an OS already detected for a machine of the same name.
func (a *App) SetMachines(list []api.Machine) {
	next := append([]api.Machine(nil), list...)
	a.mu.Lock()
	for i := range next {
		if next[i].OS != "" {
			continue
		}
		if info, ok := a.machineOS[machineKey(next[i])]; ok {
			next[i].OS = info.Name
			next[i].Arch = info.Arch
		}
	}
	a.machines = next
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
	a.probeMachines()
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

// probeMachines starts one probe per ready machine whose OS is still
// unknown. A probe is a bounded machine.exec call, not a loop; it updates
// the cache and wakes the window once.
func (a *App) probeMachines() {
	for _, m := range a.Machines() {
		if m.State != "ready" || m.OS != "" || strings.TrimSpace(m.Name) == "" {
			continue
		}
		key := machineKey(m)
		a.mu.Lock()
		if a.machineProbing == nil {
			a.machineProbing = map[string]bool{}
		}
		if a.machineProbing[key] {
			a.mu.Unlock()
			continue
		}
		a.machineProbing[key] = true
		a.mu.Unlock()
		go a.probeMachine(key, m.Name)
	}
}

// probeMachine asks one machine for its OS and keeps the answer. A failed
// probe is silent: the row simply has no OS until a later refresh.
func (a *App) probeMachine(key, name string) {
	info, ok := detectMachineOS(a.Exec, name)
	a.mu.Lock()
	delete(a.machineProbing, key)
	if ok {
		if a.machineOS == nil {
			a.machineOS = map[string]machineOS{}
		}
		a.machineOS[key] = info
		for i := range a.machines {
			if machineKey(a.machines[i]) == key {
				a.machines[i].OS = info.Name
				a.machines[i].Arch = info.Arch
			}
		}
	}
	a.mu.Unlock()
	if ok {
		a.invalidateUI()
	}
}

// machineExec is the one machine call a probe needs. *App.Exec matches it.
type machineExec func(on, command string, args ...string) (stdout, stderr string, code int, err error)

// detectMachineOS asks the machine's shell for its kernel and architecture.
// A POSIX shell answers `uname -sm`; Windows without uname answers `cmd /c
// ver`. Both calls are bounded by Exec's timeout.
func detectMachineOS(exec machineExec, name string) (machineOS, bool) {
	if out, _, code, err := exec(name, "uname", "-sm"); err == nil && code == 0 {
		if info := machineOSFromUname(out); info.Name != "" {
			return info, true
		}
	}
	if out, _, code, err := exec(name, "cmd", "/c", "ver"); err == nil && code == 0 {
		if info := machineOSFromVer(out); info.Name != "" {
			info.Arch = windowsArch(exec, name)
			return info, true
		}
	}
	return machineOS{}, false
}

// windowsArch asks cmd for the processor architecture. A shell that does
// not answer leaves Arch empty; the OS is still known.
func windowsArch(exec machineExec, name string) string {
	out, _, code, err := exec(name, "cmd", "/c", "echo", "%PROCESSOR_ARCHITECTURE%")
	if err != nil || code != 0 {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	fields := strings.Fields(strings.TrimSpace(lines[len(lines)-1]))
	if len(fields) == 0 {
		return ""
	}
	switch strings.ToLower(fields[0]) {
	case "amd64", "x86_64":
		return "amd64"
	case "arm64", "aarch64":
		return "arm64"
	case "x86", "i386", "i686":
		return "386"
	default:
		return machineArch(fields[0])
	}
}

// machineOSFromUname reads `uname -sm`: "Darwin arm64", "Linux x86_64" or
// Windows' "MINGW64_NT-10.0 x86_64". The last non-empty line is used so a
// login banner does not become the kernel name.
func machineOSFromUname(out string) machineOS {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	fields := strings.Fields(strings.TrimSpace(lines[len(lines)-1]))
	if len(fields) == 0 {
		return machineOS{}
	}
	arch := ""
	if len(fields) > 1 {
		arch = machineArch(fields[len(fields)-1])
	}
	kernel := strings.ToLower(fields[0])
	switch {
	case strings.HasPrefix(kernel, "darwin"):
		return machineOS{Name: "macOS", Arch: arch}
	case strings.HasPrefix(kernel, "linux"):
		return machineOS{Name: "Linux", Arch: arch}
	case strings.HasPrefix(kernel, "mingw"),
		strings.HasPrefix(kernel, "msys"),
		strings.HasPrefix(kernel, "cygwin"):
		return machineOS{Name: "Windows", Arch: arch}
	case strings.HasPrefix(kernel, "freebsd"):
		return machineOS{Name: "FreeBSD", Arch: arch}
	case strings.HasPrefix(kernel, "openbsd"):
		return machineOS{Name: "OpenBSD", Arch: arch}
	case strings.HasPrefix(kernel, "netbsd"):
		return machineOS{Name: "NetBSD", Arch: arch}
	case strings.HasPrefix(kernel, "dragonfly"):
		return machineOS{Name: "DragonFly", Arch: arch}
	case strings.HasPrefix(kernel, "sunos"):
		return machineOS{Name: "SunOS", Arch: arch}
	case strings.HasPrefix(kernel, "aix"):
		return machineOS{Name: "AIX", Arch: arch}
	default:
		return machineOS{Name: fields[0], Arch: arch}
	}
}

// machineOSFromVer reads the one line `cmd /c ver` prints on Windows.
func machineOSFromVer(out string) machineOS {
	if strings.Contains(strings.ToLower(out), "windows") {
		return machineOS{Name: "Windows"}
	}
	return machineOS{}
}

// machineArch maps kernel architecture names to the names Go uses.
func machineArch(arch string) string {
	switch strings.ToLower(arch) {
	case "x86_64", "amd64", "x64":
		return "amd64"
	case "aarch64", "arm64":
		return "arm64"
	case "i386", "i486", "i586", "i686", "x86":
		return "386"
	case "armv7l", "armv7":
		return "arm"
	default:
		return arch
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
