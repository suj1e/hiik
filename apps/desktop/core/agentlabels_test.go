package core

import (
	"errors"
	"strings"
	"testing"

	"hiik/core/hc"
	"hiik/mods/api"
)

func agentOrder(list []api.Agent) string {
	out := make([]string, len(list))
	for i, ag := range list {
		out[i] = ag.Address
	}
	return strings.Join(out, ",")
}

func TestAgentsFromGroupsSubAgentsAndRollsUpStatus(t *testing.T) {
	terms := map[string]hc.Terminal{
		"t1": {
			Address: "t1",
			State:   "live",
			Turn:    3,
			Status:  &hc.Status{State: "working"},
			Labels:  map[string]string{"herdr.seen": "3"},
		},
		"t2": {
			Address: "t2",
			State:   "live",
			Parent:  "t1",
			Status:  &hc.Status{State: "blocked"},
		},
		"t3": {
			Address: "t3",
			State:   "live",
			Parent:  "t2",
			Status:  &hc.Status{State: "working"},
		},
		"t4": {
			Address: "t4",
			State:   "live",
			Status:  &hc.Status{State: "idle"},
			Labels:  map[string]string{"herdr.pinned": "1"},
		},
		// A program an agent ran with `hc run` has a parent but may not
		// report status yet; it still belongs under the agent.
		"t5": {
			Address: "t5",
			State:   "live",
			Parent:  "t1",
			Turn:    4,
			Status:  &hc.Status{State: "done"},
			Labels:  map[string]string{"herdr.seen": "4"},
		},
	}
	agents := agentsFrom(terms, nil)
	if got := agentOrder(agents); got != "t4,t1,t2,t3,t5" {
		t.Fatalf("order = %s, want pinned parent then its children", got)
	}
	byAddr := make(map[string]api.Agent, len(agents))
	for _, ag := range agents {
		byAddr[ag.Address] = ag
	}
	if got := byAddr["t2"].Parent; got != "t1" {
		t.Errorf("t2 parent = %q", got)
	}
	if got := byAddr["t3"].Parent; got != "t2" {
		t.Errorf("t3 parent = %q", got)
	}
	if got := byAddr["t1"].Status; got != "blocked" {
		t.Errorf("parent status = %q, want the blocked child to roll up", got)
	}
	if !byAddr["t1"].Seen || byAddr["t1"].SeenTurn != 3 {
		t.Errorf("seen label was not read back: %#v", byAddr["t1"])
	}
	if !byAddr["t4"].Pinned {
		t.Errorf("pin label was not read back")
	}
	if got := byAddr["t5"].Status; got != "idle" || !byAddr["t5"].Seen {
		t.Errorf("seen done = %#v, want idle and seen", byAddr["t5"])
	}
}

func TestAgentFocusWritesSeenOnlyUpward(t *testing.T) {
	a := testApp()
	fake := newFakeCore()
	a.SetCore(fake)
	a.SetAgents([]api.Agent{{Address: "t1", Status: "done", Turn: 5, SeenTurn: 3}})

	if err := a.Run(api.CmdAgentFocus, map[string]any{"address": "t1"}); err != nil {
		t.Fatal(err)
	}
	calls := fake.labelsSnapshot()
	if len(calls) != 1 || calls[0].Address != "t1" || calls[0].Labels[labelSeen] != "5" {
		t.Fatalf("labels = %#v", calls)
	}
	ag := a.Agents()[0]
	if ag.Status != "idle" || !ag.Seen || ag.SeenTurn != 5 {
		t.Fatalf("focused agent = %#v", ag)
	}

	// The label only rises: looking at the same turn writes nothing.
	if err := a.Run(api.CmdAgentFocus, map[string]any{"address": "t1"}); err != nil {
		t.Fatal(err)
	}
	if got := len(fake.labelsSnapshot()); got != 1 {
		t.Fatalf("second focus wrote again: %d calls", got)
	}
}

func TestAgentFocusKeepsFocusButShowsLabelError(t *testing.T) {
	a := testApp()
	fake := newFakeCore()
	fake.labelErr = errors.New("machine/t1: connection lost")
	a.SetCore(fake)
	a.SetAgents([]api.Agent{{Address: "machine/t1", Status: "working", Turn: 2}})

	err := a.Run(api.CmdAgentFocus, map[string]any{"address": "machine/t1"})
	if err == nil || !strings.Contains(err.Error(), "connection lost") {
		t.Fatalf("err = %v, want the core's message", err)
	}
	// The user still looked at the agent; only the durable record failed.
	if got := a.ActiveAgent(); got != "machine/t1" {
		t.Fatalf("active agent = %q", got)
	}
	if ag := a.Agents()[0]; !ag.Seen || ag.SeenTurn != 0 {
		t.Fatalf("agent = %#v", ag)
	}
}

func TestAgentPinWritesAndRemovesLabel(t *testing.T) {
	a := testApp()
	fake := newFakeCore()
	a.SetCore(fake)
	a.SetAgents([]api.Agent{{Address: "t1", Status: "idle"}})

	if err := a.Run(api.CmdAgentPin, map[string]any{"address": "t1", "pinned": true}); err != nil {
		t.Fatal(err)
	}
	if !a.Agents()[0].Pinned {
		t.Fatal("pin did not stick")
	}
	calls := fake.labelsSnapshot()
	if len(calls) != 1 || calls[0].Labels[labelPinned] != "1" {
		t.Fatalf("pin labels = %#v", calls)
	}

	if err := a.Run(api.CmdAgentPin, map[string]any{"address": "t1"}); err != nil {
		t.Fatal(err)
	}
	if a.Agents()[0].Pinned {
		t.Fatal("unpin did not stick")
	}
	calls = fake.labelsSnapshot()
	if len(calls) != 2 {
		t.Fatalf("label calls = %#v", calls)
	}
	if v, ok := calls[1].Labels[labelPinned]; !ok || v != "" {
		t.Fatalf("unpin label = %#v, want an empty removal", calls[1].Labels)
	}
}

func TestAgentPinRevertsWhenTheCoreRefusesTheLabel(t *testing.T) {
	a := testApp()
	fake := newFakeCore()
	fake.labelErr = errors.New("machine/t1: connection lost")
	a.SetCore(fake)
	a.SetAgents([]api.Agent{{Address: "machine/t1", Status: "idle"}})

	err := a.Run(api.CmdAgentPin, map[string]any{"address": "machine/t1"})
	if err == nil || !strings.Contains(err.Error(), "connection lost") {
		t.Fatalf("err = %v, want the core's message", err)
	}
	if a.Agents()[0].Pinned {
		t.Fatal("a refused label must not leave a local pin")
	}
}

func TestAgentNextPrevFollowGroupedOrder(t *testing.T) {
	a := testApp()
	a.SetAgents([]api.Agent{
		{Address: "t1", Status: "idle"},
		{Address: "t2", Status: "idle", Parent: "t1"},
		{Address: "t3", Status: "idle"},
	})
	if err := a.Run(api.CmdAgentNext, nil); err != nil {
		t.Fatal(err)
	}
	if got := a.ActiveAgent(); got != "t1" {
		t.Fatalf("first next = %q", got)
	}
	if err := a.Run(api.CmdAgentNext, nil); err != nil {
		t.Fatal(err)
	}
	if got := a.ActiveAgent(); got != "t2" {
		t.Fatalf("next over a sub-agent = %q", got)
	}
	if err := a.Run(api.CmdAgentNext, nil); err != nil {
		t.Fatal(err)
	}
	if got := a.ActiveAgent(); got != "t3" {
		t.Fatalf("third next = %q", got)
	}
	if err := a.Run(api.CmdAgentPrev, nil); err != nil {
		t.Fatal(err)
	}
	if got := a.ActiveAgent(); got != "t2" {
		t.Fatalf("prev = %q", got)
	}
}
