package ui

import (
	"testing"

	"github.com/egoist/mygo/plugins/terminal"
	mui "github.com/egoist/mygo/ui"

	"hiik/mods/api"
)

func TestFindBarSearchesFocusedTerminal(t *testing.T) {
	app := &fakeApp{
		active: "s1",
		spaces: []api.Space{{ID: "s1", Name: "alpha", Tabs: []api.Tab{{
			ID: "t1", Name: "first", Root: leaf("n1", "term-a"),
		}}}},
	}
	s, tt := newTestShell(app)
	term, err := terminal.New(terminal.Options{Conn: newFakeConn()})
	if err != nil {
		t.Fatalf("creating a terminal: %v", err)
	}
	defer term.Close()
	term.Feed([]byte("needle one\nneedle two\n"))
	s.attachmentsMu.Lock()
	s.attachments["term-a"] = &paneTerm{term: term}
	s.attachmentsMu.Unlock()
	tt.Frame()

	tt.Key(shortcutMods(), mui.KeyF)
	tt.Frame()
	if !s.findOpen {
		t.Fatalf("the find shortcut did not open the bar; texts: %q", tt.Texts())
	}
	// MyGo's ui.FindBar has these controls, its own accessibility included.
	for _, label := range []string{"Find", "Previous", "Next", "Done"} {
		if _, ok := tt.Find(label); !ok {
			t.Fatalf("the find bar is missing %q; texts: %q", label, tt.Texts())
		}
	}

	tt.Type("needle")
	tt.Frame()
	if got := term.SearchStatus().Text; got != "needle" {
		t.Fatalf("the terminal searches for %q, want %q", got, "needle")
	}
	if got := term.SearchStatus(); got.Total != 2 || got.Selected != 1 {
		t.Fatalf("the bar found %d of %d, want 1 of 2", got.Selected, got.Total)
	}
	if !tt.HasText("1 of 2") {
		t.Fatalf("the bar does not show its match count: %q", tt.Texts())
	}

	// Enter goes to the next match, toward older output; Shift+Enter comes
	// back.
	tt.Key(0, mui.KeyEnter)
	if got := term.SearchStatus().Selected; got != 2 {
		t.Fatalf("Enter chose match %d, want 2", got)
	}
	if !tt.HasText("2 of 2") {
		t.Fatalf("the bar does not follow Enter: %q", tt.Texts())
	}
	tt.Key(mui.Shift, mui.KeyEnter)
	if got := term.SearchStatus().Selected; got != 1 {
		t.Fatalf("Shift+Enter chose match %d, want 1", got)
	}

	tt.Key(0, mui.KeyEscape)
	if s.findOpen {
		t.Fatalf("Escape left the find bar open: %q", tt.Texts())
	}
	if got := term.SearchStatus().Text; got != "" {
		t.Fatalf("closing the bar left the search for %q", got)
	}
	tt.Frame()
	if !tt.Focused("Terminal") {
		t.Fatal("the terminal did not get the keyboard back after the bar closed")
	}
}

func TestFindStep(t *testing.T) {
	cases := []struct {
		before, after, matches int
		forward, moved         bool
	}{
		{0, 1, 3, true, true},
		{2, 0, 3, true, true},   // Next wrapped.
		{1, 0, 3, false, true},  // Previous.
		{0, 2, 3, false, true},  // Previous wrapped.
		{0, 1, 2, true, true},   // Two matches, Next.
		{1, 0, 2, false, true},  // Two matches, Previous.
		{0, 0, 1, false, false}, // One match holds still.
		{0, 0, 0, false, false}, // No matches.
	}
	for _, tc := range cases {
		forward, moved := findStep(tc.before, tc.after, tc.matches)
		if forward != tc.forward || moved != tc.moved {
			t.Errorf("findStep(%d, %d, %d) = %v, %v, want %v, %v",
				tc.before, tc.after, tc.matches, forward, moved, tc.forward, tc.moved)
		}
	}
}
