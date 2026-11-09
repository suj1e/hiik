package core

import (
	"testing"

	"hiik/core/hc"
	"hiik/mods/api"
)

func TestAdoptTerminalsFromSharedLabels(t *testing.T) {
	a := testApp()
	a.adoptTerminals([]hc.Terminal{
		{Address: "t7", State: "live", Cwd: "/tmp/api", Labels: map[string]string{"herdr.space": "s9", "herdr.tab": "t2"}},
		{Address: "t8", State: "live", Cwd: "/tmp/web", Labels: map[string]string{"herdr.space": "s10", "herdr.tab": "t3"}},
	})
	spaces := a.Spaces()
	if len(spaces) != 2 {
		t.Fatalf("spaces = %d, want 2", len(spaces))
	}
	if got := firstTerminal(spaces[0].Tabs[0].Root); got != "t7" {
		t.Fatalf("first terminal = %q, want t7", got)
	}
	// A second connect sees the same terminals and does not duplicate them.
	a.adoptTerminals([]hc.Terminal{
		{Address: "t7", State: "live", Cwd: "/tmp/api", Labels: map[string]string{"herdr.space": "s9", "herdr.tab": "t2"}},
	})
	if got := len(a.Spaces()); got != 2 {
		t.Fatalf("spaces after re-adopt = %d, want 2", got)
	}
}

func firstTerminal(n *api.Node) string {
	if n == nil {
		return ""
	}
	if n.Terminal != "" {
		return n.Terminal
	}
	if got := firstTerminal(n.A); got != "" {
		return got
	}
	return firstTerminal(n.B)
}

func TestAdoptUsesHerdrWhereNames(t *testing.T) {
	a := testApp()
	a.adoptTerminals([]hc.Terminal{{
		Address: "t7", State: "live", Cwd: "/Users/sujie",
		Labels: map[string]string{
			"herdr.space": "sp1", "herdr.tab": "tb1", "where": "hiik / Build",
		},
	}})
	spaces := a.Spaces()
	if len(spaces) != 1 || spaces[0].Name != "hiik" {
		t.Fatalf("spaces = %#v, want one named hiik", spaces)
	}
	if got := spaces[0].Tabs[0].Name; got != "Build" {
		t.Fatalf("tab name = %q, want Build", got)
	}
}

func TestAdoptRenamesAnAutoNamedSpace(t *testing.T) {
	a := testApp()
	term := hc.Terminal{
		Address: "t7", State: "live", Cwd: "/Users/sujie",
		Labels: map[string]string{"herdr.space": "sp1", "herdr.tab": "tb1", "where": "sujie / 1"},
	}
	a.adoptTerminals([]hc.Terminal{term})
	term.Labels["where"] = "hiik / Build"
	a.adoptTerminals([]hc.Terminal{term})
	spaces := a.Spaces()
	if len(spaces) != 1 || spaces[0].Name != "hiik" || spaces[0].Tabs[0].Name != "Build" {
		t.Fatalf("after rename = %#v", spaces)
	}
}
