package core

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"hiik/mods/api"
)

func TestMachineCommandArgumentValidation(t *testing.T) {
	a := testApp()
	cases := []struct {
		name string
		args map[string]any
		want string
	}{
		{api.CmdMachineAdd, map[string]any{}, "target is required"},
		{api.CmdMachineAdd, map[string]any{"target": "vps", "via": "fly ssh console"}, "via command needs a name"},
		{api.CmdMachineAdd, map[string]any{"host": "vps"}, "core not connected"},
		{api.CmdMachineAdd, map[string]any{"target": "vps", "nope": 1}, "unknown argument"},
		{api.CmdMachineAdd, map[string]any{"target": 5}, "must be a string"},
		{api.CmdMachineConnect, map[string]any{}, "machine is required"},
		{api.CmdMachineRename, map[string]any{"machine": "a"}, "new name is required"},
		{api.CmdMachineRename, map[string]any{"new_name": "b"}, "machine is required"},
		{api.CmdMachineRm, map[string]any{}, "machine is required"},
		{api.CmdMachineList, map[string]any{"machine": "a"}, "unknown argument"},
	}
	for _, tc := range cases {
		err := a.Run(tc.name, tc.args)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s %v: err = %v, want %q", tc.name, tc.args, err, tc.want)
		}
	}
}

func TestMachineCommandNeedsACore(t *testing.T) {
	a := testApp()
	cases := []struct {
		name string
		args map[string]any
	}{
		{api.CmdMachineList, nil},
		{api.CmdMachineConnect, map[string]any{"machine": "builder"}},
		{api.CmdMachineRename, map[string]any{"machine": "builder", "new_name": "work"}},
		{api.CmdMachineRm, map[string]any{"machine": "builder"}},
	}
	for _, tc := range cases {
		err := a.Run(tc.name, tc.args)
		if err == nil || !strings.Contains(err.Error(), "core not connected") {
			t.Errorf("%s: err = %v, want core not connected", tc.name, err)
		}
	}
}

func TestMachinesSnapshotIsACopy(t *testing.T) {
	a := testApp()
	if got := a.Machines(); len(got) != 0 {
		t.Fatalf("machines = %v, want none", got)
	}
	a.SetMachines([]api.Machine{{Name: "builder", Host: "builder.local", State: "ready"}})
	got := a.Machines()
	if len(got) != 1 || got[0].Name != "builder" {
		t.Fatalf("machines = %v", got)
	}
	got[0].Name = "changed"
	if name := a.Machines()[0].Name; name != "builder" {
		t.Fatalf("cache was mutated: %q", name)
	}
}

func TestRefreshMachinesWithoutCoreIsANoop(t *testing.T) {
	a := testApp()
	if err := a.RefreshMachines(context.Background()); err != nil {
		t.Fatalf("RefreshMachines: %v", err)
	}
}

func TestMachineFromView(t *testing.T) {
	cases := []struct {
		view machineView
		want api.Machine
	}{
		{
			machineView{Name: "builder", Host: "builder.local", Target: "me@builder", State: "connected"},
			api.Machine{Name: "builder", Host: "builder.local", State: "ready"},
		},
		{
			machineView{Name: "builder", Target: "me@builder", State: "connecting"},
			api.Machine{Name: "builder", Host: "me@builder", State: "loading"},
		},
		{
			machineView{Name: "builder", Host: "builder.local", State: "needs_login"},
			api.Machine{Name: "builder", Host: "builder.local", State: "unavailable", Error: "needs login"},
		},
		{
			machineView{Name: "box", Host: "box.local", State: "unreachable", Why: "ssh: refused", Temporary: true},
			api.Machine{Name: "box", Host: "box.local", State: "unavailable", Error: "ssh: refused", Temporary: true},
		},
		{
			machineView{Name: "odd", Host: "odd.local", State: "sleeping"},
			api.Machine{Name: "odd", Host: "odd.local", State: "unavailable"},
		},
	}
	for _, tc := range cases {
		if got := machineFromView(tc.view); got != tc.want {
			t.Errorf("machineFromView(%+v) = %+v, want %+v", tc.view, got, tc.want)
		}
	}
}

func TestMachineOSFromUname(t *testing.T) {
	cases := []struct {
		out  string
		want machineOS
	}{
		{"Darwin arm64\n", machineOS{Name: "macOS", Arch: "arm64"}},
		{"Darwin x86_64\n", machineOS{Name: "macOS", Arch: "amd64"}},
		{"Linux x86_64\n", machineOS{Name: "Linux", Arch: "amd64"}},
		{"Linux aarch64\n", machineOS{Name: "Linux", Arch: "arm64"}},
		{"MINGW64_NT-10.0-22631 x86_64\n", machineOS{Name: "Windows", Arch: "amd64"}},
		{"CYGWIN_NT-10.0 aarch64\n", machineOS{Name: "Windows", Arch: "arm64"}},
		{"FreeBSD amd64\n", machineOS{Name: "FreeBSD", Arch: "amd64"}},
		{"SunOS sun4u\n", machineOS{Name: "SunOS", Arch: "sun4u"}},
		{"Linux\n", machineOS{Name: "Linux"}},
		{"Welcome to builder\nLinux riscv64\n", machineOS{Name: "Linux", Arch: "riscv64"}},
		{"Plan9 386\n", machineOS{Name: "Plan9", Arch: "386"}},
		{"", machineOS{}},
	}
	for _, tc := range cases {
		if got := machineOSFromUname(tc.out); got != tc.want {
			t.Errorf("machineOSFromUname(%q) = %+v, want %+v", tc.out, got, tc.want)
		}
	}
}

func TestMachineOSFromVer(t *testing.T) {
	cases := []struct {
		out  string
		want machineOS
	}{
		{"Microsoft Windows [Version 10.0.22631.4317]\n", machineOS{Name: "Windows"}},
		{"Linux builder 6.8\n", machineOS{}},
		{"", machineOS{}},
	}
	for _, tc := range cases {
		if got := machineOSFromVer(tc.out); got != tc.want {
			t.Errorf("machineOSFromVer(%q) = %+v, want %+v", tc.out, got, tc.want)
		}
	}
}

func TestDetectMachineOSPrefersUname(t *testing.T) {
	calls := 0
	exec := func(on, command string, args ...string) (string, string, int, error) {
		calls++
		if on != "builder" || command != "uname" || len(args) != 1 || args[0] != "-sm" {
			t.Errorf("call = %q %q %v", on, command, args)
		}
		return "Linux arm64\n", "", 0, nil
	}
	got, ok := detectMachineOS(exec, "builder")
	if !ok || got != (machineOS{Name: "Linux", Arch: "arm64"}) {
		t.Fatalf("detect = %+v %v", got, ok)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want one", calls)
	}
}

func TestDetectMachineOSFallsBackToWindowsVer(t *testing.T) {
	var calls []string
	exec := func(on, command string, args ...string) (string, string, int, error) {
		calls = append(calls, command+" "+strings.Join(args, " "))
		switch {
		case command == "uname":
			return "", "uname: not found", 1, nil
		case command == "cmd" && len(args) == 2 && args[1] == "ver":
			return "Microsoft Windows [Version 10.0.22631.4317]\n", "", 0, nil
		case command == "cmd" && len(args) == 3 && args[2] == "%PROCESSOR_ARCHITECTURE%":
			return "AMD64\r\n", "", 0, nil
		default:
			return "", "", -1, nil
		}
	}
	got, ok := detectMachineOS(exec, "builder")
	if !ok || got.Name != "Windows" || got.Arch != "amd64" {
		t.Fatalf("detect = %+v %v, want Windows amd64", got, ok)
	}
	if len(calls) != 3 || calls[0] != "uname -sm" || calls[1] != "cmd /c ver" {
		t.Fatalf("calls = %v, want uname, ver then arch", calls)
	}
}

func TestDetectMachineOSGivesUp(t *testing.T) {
	exec := func(on, command string, args ...string) (string, string, int, error) {
		return "", "not found", 1, nil
	}
	if got, ok := detectMachineOS(exec, "builder"); ok || got != (machineOS{}) {
		t.Fatalf("detect = %+v %v, want none", got, ok)
	}
}

func TestSetMachinesKeepsDetectedOS(t *testing.T) {
	a := testApp()
	a.machineOS = map[string]machineOS{machineKey(api.Machine{Name: "builder"}): {Name: "Linux", Arch: "arm64"}}
	a.SetMachines([]api.Machine{{Name: "builder", State: "ready"}})
	got := a.Machines()
	if len(got) != 1 || got[0].OS != "Linux" || got[0].Arch != "arm64" {
		t.Fatalf("machines = %+v", got)
	}
}

func TestProbeMachinesSkipsUnreadyAndKnown(t *testing.T) {
	a := testApp()
	a.machineOS = map[string]machineOS{machineKey(api.Machine{Name: "known"}): {Name: "Linux", Arch: "arm64"}}
	a.SetMachines([]api.Machine{
		{Name: "known", State: "ready"},
		{Name: "down", State: "unavailable"},
		{Name: "loading", State: "loading"},
	})
	a.probeMachines()
	a.mu.Lock()
	probing := len(a.machineProbing)
	a.mu.Unlock()
	if probing != 0 {
		t.Fatalf("probes = %d, want none", probing)
	}
}

// machineExecCore answers machine.exec with one stdout script and counts
// the calls, so a probe can run without a real core.
type machineExecCore struct {
	*fakeCore
	stdout string
	calls  int
}

func (c *machineExecCore) Call(_ context.Context, method string, _, result any) error {
	if method != "machine.exec" {
		return nil
	}
	c.calls++
	r, ok := result.(*execResult)
	if !ok {
		return errors.New("machine.exec result is not execResult")
	}
	code := 0
	r.Stdout = base64.StdEncoding.EncodeToString([]byte(c.stdout))
	r.Code = &code
	return nil
}

func TestProbeMachineStoresOS(t *testing.T) {
	c := &machineExecCore{fakeCore: newFakeCore(), stdout: "Linux arm64\n"}
	a := testApp()
	a.SetCore(c)
	a.SetMachines([]api.Machine{{Name: "builder", Host: "builder.local", State: "ready"}})

	m := a.Machines()[0]
	a.probeMachine(machineKey(m), m.Name)

	got := a.Machines()[0]
	if got.OS != "Linux" || got.Arch != "arm64" {
		t.Fatalf("machine = %+v, want Linux arm64", got)
	}
	if c.calls != 1 {
		t.Fatalf("machine.exec calls = %d, want 1", c.calls)
	}
	if cached := a.machineOS[machineKey(m)]; cached.Name != "Linux" || cached.Arch != "arm64" {
		t.Fatalf("cache = %+v", cached)
	}
}
