package core

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"hiik/core/hc"
	"hiik/mods/api"
)

// recordingCore is a fake core that keeps the generic Call requests the app
// makes, such as terminal.keep.
type recordingCore struct {
	*fakeCore

	mu    sync.Mutex
	calls []recordedCall
	// callErr answers every Call when set.
	callErr error
}

type recordedCall struct {
	method string
	params map[string]any
}

func newRecordingCore() *recordingCore {
	return &recordingCore{fakeCore: newFakeCore()}
}

func (f *recordingCore) Call(_ context.Context, method string, params, _ any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	copied := make(map[string]any)
	if given, ok := params.(map[string]any); ok {
		for key, value := range given {
			copied[key] = value
		}
	}
	f.calls = append(f.calls, recordedCall{method: method, params: copied})
	return f.callErr
}

func (f *recordingCore) callsSnapshot() []recordedCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]recordedCall(nil), f.calls...)
}

// seedTerminal puts a terminal in the model's core records.
func seedTerminal(a *App, t hc.Terminal) {
	a.setTerminalRecords(map[string]hc.Terminal{t.Address: t})
}

func tabIDAt(sp api.Space, index int) string {
	if index < 0 || index >= len(sp.Tabs) {
		return ""
	}
	return sp.Tabs[index].ID
}

func TestSettlePutsTabsAwayAndShowingBringsThemBack(t *testing.T) {
	a := testApp()
	m := &a.model
	m.createSpace("api", "/tmp/api", "")
	m.createTab("s1", "logs")
	if err := a.Run(api.CmdTabSelect, map[string]any{"id": "t1"}); err != nil {
		t.Fatal(err)
	}

	if err := a.Run("tab.settle", map[string]any{"id": "t1"}); err != nil {
		t.Fatal(err)
	}
	if !a.TabSettled("s1", "t1") {
		t.Fatal("t1 should be settled")
	}
	sp := a.Spaces()[0]
	if got := tabIDAt(sp, sp.Active); got != "t2" {
		t.Fatalf("active tab = %q, want t2 (the next unsettled tab)", got)
	}

	// tab.select delta skips settled tabs: only t2 is left.
	if err := a.Run(api.CmdTabSelect, map[string]any{"delta": 1}); err != nil {
		t.Fatal(err)
	}
	sp = a.Spaces()[0]
	if got := tabIDAt(sp, sp.Active); got != "t2" {
		t.Fatalf("active tab = %q, want t2 (settled t1 skipped)", got)
	}

	if err := a.Run("tab.unsettle", map[string]any{"id": "t1"}); err != nil {
		t.Fatal(err)
	}
	if a.TabSettled("s1", "t1") {
		t.Fatal("t1 should be back in the lists")
	}
	if err := a.Run(api.CmdTabSelect, map[string]any{"delta": -1}); err != nil {
		t.Fatal(err)
	}
	sp = a.Spaces()[0]
	if got := tabIDAt(sp, sp.Active); got != "t1" {
		t.Fatalf("active tab = %q, want t1", got)
	}

	// Showing a settled tab directly brings it back too.
	if err := a.Run("tab.settle", map[string]any{"id": "t1"}); err != nil {
		t.Fatal(err)
	}
	if err := a.Run(api.CmdTabSelect, map[string]any{"id": "t1"}); err != nil {
		t.Fatal(err)
	}
	if a.TabSettled("s1", "t1") {
		t.Fatal("selecting a settled tab should unsettle it")
	}
}

func TestAgentAttentionUnsettlesItsTab(t *testing.T) {
	a := testApp()
	m := &a.model
	m.createSpace("api", "/tmp/api", "")
	m.setTerminal("p1", "t1")
	if err := a.Run("tab.settle", map[string]any{"id": "t1"}); err != nil {
		t.Fatal(err)
	}
	if !a.TabSettled("s1", "t1") {
		t.Fatal("t1 should start settled")
	}

	terms := map[string]hc.Terminal{
		"t1": {Address: "t1", State: "live", Status: &hc.Status{State: "blocked"}},
	}
	att := newAttention(map[string]hc.Terminal{})
	ev := hc.Event{
		Type:     "status",
		Terminal: "t1",
		Data:     json.RawMessage(`{"status":{"state":"blocked"},"turn":1}`),
	}
	a.observeEvent(att, terms, ev)

	if a.TabSettled("s1", "t1") {
		t.Fatal("an agent needing a person should bring its tab back")
	}
}

func TestTerminalShowUsesAPaneForNowAndRestoresIt(t *testing.T) {
	a := testApp()
	m := &a.model
	m.createSpace("api", "/tmp/api", "")
	m.setTerminal("p1", "t1")
	m.createTab("s1", "logs")
	m.setTerminal("p2", "t2")
	seedTerminal(a, hc.Terminal{Address: "t3", State: "live"})

	if err := a.Run("terminal.show", map[string]any{"pane": "p1", "terminal": "t3"}); err != nil {
		t.Fatal(err)
	}
	if got := terminalAt(a, "p1"); got != "t3" {
		t.Fatalf("p1 shows %q, want t3", got)
	}
	// The borrowed terminal is not p1's own: closing the tab ends t1 only.
	if err := a.Run("terminal.show", map[string]any{"pane": "p1"}); err != nil {
		t.Fatal(err)
	}
	if got := terminalAt(a, "p1"); got != "t1" {
		t.Fatalf("p1 shows %q after restore, want t1", got)
	}

	// A terminal with a pane of its own is shown there, not borrowed.
	if err := a.Run("terminal.show", map[string]any{"pane": "p1", "terminal": "t2"}); err != nil {
		t.Fatal(err)
	}
	if got := terminalAt(a, "p1"); got != "t1" {
		t.Fatalf("p1 shows %q, want its own t1", got)
	}
	sp := a.Spaces()[0]
	if got := tabIDAt(sp, sp.Active); got != "t2" {
		t.Fatalf("active tab = %q, want t2 (t2's own pane)", got)
	}
}

func TestTerminalParkRemovesThePaneAndDisablesKeep(t *testing.T) {
	a := testApp()
	c := newRecordingCore()
	a.SetCore(c)

	m := &a.model
	m.createSpace("api", "/tmp/api", "")
	m.setTerminal("p1", "t1")
	seedTerminal(a, hc.Terminal{Address: "t1", State: "live"})

	if err := a.Run("terminal.park", map[string]any{"terminal": "t1"}); err != nil {
		t.Fatal(err)
	}
	if got := len(a.Spaces()); got != 0 {
		t.Fatalf("spaces = %d, want 0 after parking the only pane", got)
	}
	if killed := c.killedSnapshot(); len(killed) != 0 {
		t.Fatalf("park must not kill, killed %v", killed)
	}
	labels := c.labelsSnapshot()
	if len(labels) != 1 || labels[0].Address != "t1" {
		t.Fatalf("labels = %+v", labels)
	}
	if got := labels[0].Labels[labelSpace]; got != "" {
		t.Fatalf("herdr space label = %q, want cleared", got)
	}
	if got := labels[0].Labels[labelTab]; got != "" {
		t.Fatalf("herdr tab label = %q, want cleared", got)
	}
	calls := c.callsSnapshot()
	if len(calls) != 1 || calls[0].method != "terminal.keep" {
		t.Fatalf("calls = %+v, want one terminal.keep", calls)
	}
	if got, ok := calls[0].params["keep"].(bool); !ok || got {
		t.Fatalf("terminal.keep keep = %v, want false", calls[0].params["keep"])
	}
}

func TestTerminalScrollValidatesOneTarget(t *testing.T) {
	a := testApp()
	m := &a.model
	m.createSpace("api", "/tmp/api", "")
	m.setTerminal("p1", "t1")

	type gotScroll struct {
		address     string
		lines, page float32
		to          string
	}
	var got gotScroll
	a.SetTerminalScroller(func(address string, lines, pages float32, to string) bool {
		got = gotScroll{address: address, lines: lines, page: pages, to: to}
		return true
	})

	cases := []struct {
		name string
		args map[string]any
		want string
	}{
		{"terminal.scroll", map[string]any{}, "needs one of lines, pages, to"},
		{"terminal.scroll", map[string]any{"lines": 1, "pages": 1}, "not lines and pages"},
		{"terminal.scroll", map[string]any{"lines": 1.5}, "must be an integer"},
		{"terminal.scroll", map[string]any{"to": "middle"}, `"top" or "bottom"`},
	}
	for _, tc := range cases {
		err := a.Run(tc.name, tc.args)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%v: err = %v, want %q", tc.args, err, tc.want)
		}
	}

	if err := a.Run("terminal.scroll", map[string]any{"lines": -3}); err != nil {
		t.Fatal(err)
	}
	if got.address != "t1" || got.lines != -3 || got.page != 0 || got.to != "" {
		t.Fatalf("scroll = %+v", got)
	}
	if err := a.Run("terminal.scroll", map[string]any{"pages": 0.5}); err != nil {
		t.Fatal(err)
	}
	if got.page != 0.5 || got.lines != 0 {
		t.Fatalf("scroll = %+v", got)
	}
	if err := a.Run("terminal.scroll", map[string]any{"to": "top"}); err != nil {
		t.Fatal(err)
	}
	if got.to != "top" {
		t.Fatalf("scroll = %+v", got)
	}

	a.SetTerminalScroller(func(string, float32, float32, string) bool { return false })
	if err := a.Run("terminal.scroll", map[string]any{"lines": 1}); err == nil ||
		!strings.Contains(err.Error(), "not attached") {
		t.Fatalf("err = %v, want not attached", err)
	}
}

func TestTerminalRightClickStoresPolicyAndCallsHook(t *testing.T) {
	a := testApp()
	c := newRecordingCore()
	a.SetCore(c)
	m := &a.model
	m.createSpace("api", "/tmp/api", "")
	m.setTerminal("p1", "t1")
	seedTerminal(a, hc.Terminal{Address: "t1", State: "live"})

	var hookedAddress, hookedTo string
	a.SetRightClickPolicy(func(address, to string) {
		hookedAddress, hookedTo = address, to
	})

	if err := a.Run("terminal.right_click", map[string]any{"to": "program"}); err != nil {
		t.Fatal(err)
	}
	labels := c.labelsSnapshot()
	if len(labels) != 1 || labels[0].Labels[labelRightClick] != "program" {
		t.Fatalf("labels = %+v", labels)
	}
	if hookedAddress != "t1" || hookedTo != "program" {
		t.Fatalf("hook = %q %q", hookedAddress, hookedTo)
	}
	got, ok := a.model.terminal("t1")
	if !ok || got.Labels[labelRightClick] != "program" {
		t.Fatalf("model label = %q", got.Labels[labelRightClick])
	}

	if err := a.Run("terminal.right_click", map[string]any{"to": "menu", "terminal": "t1"}); err != nil {
		t.Fatal(err)
	}
	if hookedTo != "menu" {
		t.Fatalf("hook = %q, want menu", hookedTo)
	}
	if err := a.Run("terminal.right_click", map[string]any{"to": "sideways"}); err == nil {
		t.Fatal("an unknown policy should fail")
	}
	if err := a.Run("terminal.right_click", map[string]any{"to": "program", "terminal": "nope"}); err == nil {
		t.Fatal("an unknown terminal should fail")
	}
}

func TestTerminalInfoFollowsTitleAndFinishedEvents(t *testing.T) {
	a := testApp()
	terms := map[string]hc.Terminal{
		"t1": {
			Address: "t1", State: "live", Title: "bash", Cwd: "/tmp",
			Foreground: &hc.Foreground{Cwd: "/repo"},
			Labels:     map[string]string{labelSeen: "4", labelPinned: "1"},
		},
	}
	a.setTerminalRecords(terms)

	info, ok := a.TerminalInfo("t1")
	if !ok {
		t.Fatal("t1 not found")
	}
	if info.Title != "bash" || info.Cwd != "/repo" || info.State != "live" {
		t.Fatalf("info = %+v", info)
	}
	if info.SeenTurn != 4 || !info.Pinned || info.ExitCode != nil {
		t.Fatalf("info = %+v", info)
	}

	title := hc.Event{Type: "title", Terminal: "t1", Data: json.RawMessage(`{"title":"vim"}`)}
	applyEvent(terms, title)
	a.setTerminalRecords(terms)
	if info, _ := a.TerminalInfo("t1"); info.Title != "vim" {
		t.Fatalf("title = %q, want vim", info.Title)
	}

	finished := hc.Event{Type: "finished", Terminal: "t1", Data: json.RawMessage(`{"exit_code":3}`)}
	applyEvent(terms, finished)
	a.setTerminalRecords(terms)
	info, _ = a.TerminalInfo("t1")
	if info.State != "finished" {
		t.Fatalf("state = %q, want finished", info.State)
	}
	if info.ExitCode == nil || *info.ExitCode != 3 {
		t.Fatalf("exit code = %v, want 3", info.ExitCode)
	}
	if all := a.TerminalInfos(); len(all) != 1 || all["t1"].State != "finished" {
		t.Fatalf("infos = %+v", all)
	}
	if _, ok := a.TerminalInfo("gone"); ok {
		t.Fatal("an unknown terminal should not be found")
	}
}

func TestCloseConfirmationIsSharedAndAgentsCannotPrompt(t *testing.T) {
	a := testApp()
	c := newRecordingCore()
	a.SetCore(c)
	m := &a.model
	m.createSpace("api", "/tmp/api", "")
	m.setTerminal("p1", "t1")
	seedTerminal(a, hc.Terminal{Address: "t1", State: "live"})

	var title, message string
	var run func()
	a.SetConfirmer(func(gotTitle, gotMessage string, gotRun func()) {
		title, message, run = gotTitle, gotMessage, gotRun
	})

	if err := a.Run(api.CmdSpaceClose, map[string]any{"id": "s1"}); err != nil {
		t.Fatal(err)
	}
	if run == nil || title != "Close space?" || !strings.Contains(message, "1 tabs, 1 terminals") {
		t.Fatalf("confirmer got %q %q %v", title, message, run != nil)
	}
	if len(a.Spaces()) != 1 {
		t.Fatal("space closed before the person agreed")
	}
	run()
	if len(a.Spaces()) != 0 {
		t.Fatal("space should close once confirmed")
	}
	if killed := c.killedSnapshot(); len(killed) != 1 || killed[0] != "t1" {
		t.Fatalf("killed = %v, want t1", killed)
	}

	// An agent's close never asks; it fails with the confirmed hint.
	agent := testApp()
	agent.model.createSpace("api", "/tmp/api", "")
	agent.model.setTerminal("p1", "t1")
	agent.SetConfirmer(func(string, string, func()) {
		t.Fatal("an agent call must not open the confirmer")
	})
	err := agent.RunAgent(api.CmdSpaceClose, map[string]any{"id": "s1"})
	if err == nil || !strings.Contains(err.Error(), "pass confirmed: true") {
		t.Fatalf("err = %v, want the confirmed hint", err)
	}
	if len(agent.Spaces()) != 1 {
		t.Fatal("agent close should not have run")
	}
	if err := agent.RunAgent(api.CmdSpaceClose, map[string]any{"id": "s1", "confirmed": true}); err != nil {
		t.Fatal(err)
	}
	if len(agent.Spaces()) != 0 {
		t.Fatal("a confirmed agent close should run")
	}
}

func TestLastTabAndPaneClosesShareTheSpaceQuestion(t *testing.T) {
	a := testApp()
	m := &a.model
	m.createSpace("api", "/tmp/api", "")
	m.createTab("s1", "logs")
	m.closeTab("s1", "t2") // leave one tab again
	pane := m.activeTabFocus()

	asked := 0
	a.SetConfirmer(func(string, string, func()) { asked++ })

	if err := a.Run(api.CmdTabClose, map[string]any{"id": "t1"}); err != nil {
		t.Fatal(err)
	}
	if asked != 1 {
		t.Fatalf("tab.close asked %d times, want 1", asked)
	}
	if err := a.Run(api.CmdPaneClose, map[string]any{"pane": pane}); err != nil {
		t.Fatal(err)
	}
	if asked != 2 {
		t.Fatalf("pane.close asked %d times total, want 2", asked)
	}
}

func TestTerminalPasteImageUsesTheFocusedTerminal(t *testing.T) {
	a := testApp()
	m := &a.model
	m.createSpace("api", "/tmp/api", "")
	m.setTerminal("p1", "t1")

	var pasted string
	a.SetImagePaster(func(address string) error {
		pasted = address
		return nil
	})
	if err := a.Run("terminal.paste_image", nil); err != nil {
		t.Fatal(err)
	}
	if pasted != "t1" {
		t.Fatalf("pasted into %q, want t1", pasted)
	}

	a.SetImagePaster(func(string) error { return errNoCore })
	err := a.Run("terminal.paste_image", nil)
	if err == nil || !strings.Contains(err.Error(), errNoCore.Error()) {
		t.Fatalf("err = %v, want the hook's error", err)
	}

	empty := testApp()
	if err := empty.Run("terminal.paste_image", nil); err == nil ||
		!strings.Contains(err.Error(), "no focused terminal") {
		t.Fatalf("err = %v, want no focused terminal", err)
	}
}

func TestHelpAndWelcomeSetTheirOverlays(t *testing.T) {
	a := testApp()
	if err := a.Run("app.help", nil); err != nil {
		t.Fatal(err)
	}
	if got := a.Overlay(); got != "help" {
		t.Fatalf("overlay = %q, want help", got)
	}
	if err := a.Run("app.welcome", nil); err != nil {
		t.Fatal(err)
	}
	if got := a.Overlay(); got != "welcome" {
		t.Fatalf("overlay = %q, want welcome", got)
	}
}

// terminalAt returns the terminal a pane shows.
func terminalAt(a *App, paneID string) string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	si, ti, ok := a.model.paneLocation(paneID)
	if !ok {
		return ""
	}
	n := findNode(a.model.spaces[si].Tabs[ti].Root, paneID)
	if !isLeaf(n) {
		return ""
	}
	return n.Terminal
}
