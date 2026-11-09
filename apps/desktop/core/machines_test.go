package core

import (
	"context"
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
