package core_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"hiik/core"
	"hiik/core/hc"
	"hiik/mods/api"
)

// TestE2ESpaceLifecycle drives the real client and a real core in an
// isolated session: create a space, attach to its terminal, close it.
func TestE2ESpaceLifecycle(t *testing.T) {
	if os.Getenv("HIIK_E2E") != "1" {
		t.Skip("set HIIK_E2E=1 to run against a real herdr-core")
	}
	t.Setenv("HC_SESSION", "hiik-e2e-core")
	t.Setenv("HIIK_HOME", t.TempDir())

	app := core.New()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- app.Connect(ctx) }()
	defer app.Close()

	deadline := time.Now().Add(20 * time.Second)
	for app.Connection() != "connected" {
		if time.Now().After(deadline) {
			t.Fatalf("core did not connect: %s", app.LastError())
		}
		time.Sleep(50 * time.Millisecond)
	}

	if err := app.Run(api.CmdSpaceNew, map[string]any{"name": "e2e"}); err != nil {
		t.Fatalf("space.new: %v", err)
	}
	spaces := app.Spaces()
	if len(spaces) != 1 {
		t.Fatalf("spaces = %d, want 1", len(spaces))
	}
	if len(spaces[0].Tabs) != 1 {
		t.Fatalf("tabs = %d, want 1", len(spaces[0].Tabs))
	}
	term := firstTerminal(spaces[0].Tabs[0].Root)
	if term == "" {
		t.Fatal("the new space has no terminal")
	}

	at, err := app.Core().Attach(ctx, hc.AttachRequest{Terminal: term, Cols: 80, Rows: 24})
	if err != nil {
		t.Fatalf("attach %s: %v", term, err)
	}
	_ = at.Close()

	if err := app.Run(api.CmdSpaceClose, map[string]any{"id": spaces[0].ID}); err != nil {
		t.Fatalf("space.close: %v", err)
	}
	if got := len(app.Spaces()); got != 0 {
		t.Fatalf("spaces after close = %d, want 0", got)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Connect did not return after cancel")
	}
}

// TestE2ERemoteSpace creates a space on a real machine, runs its shell,
// attaches and closes it. It needs an opted-in machine already added to the
// core this app reaches: HIIK_E2E=1 HIIK_E2E_MACHINE=<name>. Without them,
// or when the core does not know the machine, it skips.
func TestE2ERemoteSpace(t *testing.T) {
	if os.Getenv("HIIK_E2E") != "1" {
		t.Skip("set HIIK_E2E=1 to run against a real herdr-core")
	}
	machine := strings.TrimSpace(os.Getenv("HIIK_E2E_MACHINE"))
	if machine == "" {
		t.Skip("set HIIK_E2E_MACHINE=<machine name> to run against a real machine")
	}
	t.Setenv("HIIK_HOME", t.TempDir())

	app := core.New()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- app.Connect(ctx) }()
	defer app.Close()

	deadline := time.Now().Add(30 * time.Second)
	for app.Connection() != "connected" {
		if time.Now().After(deadline) {
			t.Fatalf("core did not connect: %s", app.LastError())
		}
		time.Sleep(50 * time.Millisecond)
	}

	_ = app.RefreshMachines(ctx)
	known := false
	for _, m := range app.Machines() {
		if m.Name == machine {
			known = true
			break
		}
	}
	if !known {
		t.Skipf("the core does not know machine %q", machine)
	}

	if err := app.Run(api.CmdSpaceNew, map[string]any{
		"name": "remote-e2e", "machine": machine,
	}); err != nil {
		t.Fatalf("space.new on %s: %v", machine, err)
	}
	spaces := app.Spaces()
	if len(spaces) != 1 {
		t.Fatalf("spaces = %d, want 1", len(spaces))
	}
	sp := spaces[0]
	if sp.Machine != machine {
		t.Fatalf("space machine = %q, want %q", sp.Machine, machine)
	}
	term := firstTerminal(sp.Tabs[0].Root)
	if term == "" {
		t.Fatal("the remote space has no terminal")
	}
	if !strings.HasPrefix(term, machine+"/") {
		t.Fatalf("terminal = %q, want an address on %s", term, machine)
	}

	at, err := app.Core().Attach(ctx, hc.AttachRequest{Terminal: term, Cols: 80, Rows: 24})
	if err != nil {
		t.Fatalf("attach %s: %v", term, err)
	}
	if err := at.Close(); err != nil {
		t.Fatalf("detach %s: %v", term, err)
	}

	if err := app.Run(api.CmdSpaceClose, map[string]any{"id": sp.ID}); err != nil {
		t.Fatalf("space.close: %v", err)
	}
	if got := len(app.Spaces()); got != 0 {
		t.Fatalf("spaces after close = %d, want 0", got)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Connect did not return after cancel")
	}
}

func firstTerminal(n *api.Node) string {
	if n == nil {
		return ""
	}
	if n.Terminal != "" {
		return n.Terminal
	}
	if t := firstTerminal(n.A); t != "" {
		return t
	}
	return firstTerminal(n.B)
}
