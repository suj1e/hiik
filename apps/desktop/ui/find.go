package ui

import (
	"github.com/egoist/mygo/plugins/terminal"
	mui "github.com/egoist/mygo/ui"
)

// init registers the find bar and its shortcut. The shell calls the hook
// once per frame, so the shortcut works while the bar is closed and the bar
// draws while it is open, without shell.go knowing about either.
func init() {
	registerShellHook(findHook)
}

// findHook binds the find shortcut and draws the find bar above the panes.
// The bar itself is MyGo's ui.FindBar: the app only feeds it the terminal
// search's matches and current match, and maps the bar's moves to the
// terminal's Search, SearchNext and SearchPrevious.
func findHook(s *Shell, c *mui.Context) {
	c.OnWindowShortcut(shortcutMods(), mui.KeyF, func() {
		if s.overlayOpen() {
			return
		}
		s.openFind()
	})
	if !s.findOpen || s.overlayOpen() {
		return
	}
	// Escape and Done close the bar themselves; this catches Escape from
	// the controls around the field too.
	c.OnWindowShortcut(0, mui.KeyEscape, s.closeFind)

	term := s.findTargetTerminal()
	st := terminal.SearchStatus{}
	if term != nil {
		st = term.SearchStatus()
		if st.Text != s.findQuery {
			term.Search(s.findQuery)
			st = term.SearchStatus()
		}
	}

	// The bar counts from 1 and the terminal's Selected is 1-based; both
	// choose among the newest first.
	matches := st.Total
	current := max(st.Selected-1, 0)
	if matches > 0 {
		current = min(current, matches-1)
	}
	beforeQuery, beforeCurrent := s.findQuery, current

	width, _ := c.Size()
	left := float32(12)
	if settingString(s.app.Settings(), "layout") != "terminals" {
		left = s.sidebarW + 8
	}
	// The find bar floats below the layout's top bar: the work tab bar in
	// the herdr layout, the terminals bar in the terminals layout.
	top := float32(tabBarHeight)
	if right := width - 12; left+320 > right {
		left = 12
	}
	barWidth := min(float32(520), width-left-12)

	mui.Overlay(c, func() {
		mui.FindBar(c, &s.findOpen, &s.findQuery, matches, &current).
			Absolute().
			Top(top).
			Left(left).
			Width(barWidth)
	})

	switch {
	case !s.findOpen:
		// Done, Escape in the field, or the shortcut above closed it.
		s.endFind()
	case s.findQuery != beforeQuery:
		if term != nil {
			term.Search(s.findQuery)
		}
	default:
		if forward, moved := findStep(beforeCurrent, current, matches); moved && term != nil {
			if forward {
				term.SearchNext()
			} else {
				term.SearchPrevious()
			}
		}
	}
}

// findTargetTerminal returns the terminal of the focused pane of the active
// tab, or the tab's first terminal when focus is gone.
func (s *Shell) findTargetTerminal() *terminal.Terminal {
	_, tab := s.activeTab()
	if tab == nil || tab.Root == nil {
		return nil
	}
	n := findNode(tab.Root, s.focus)
	if n == nil || n.A != nil && n.B != nil {
		_, id := firstLeaf(tab.Root)
		n = findNode(tab.Root, id)
	}
	if n == nil || n.Terminal == "" {
		return nil
	}
	return s.attachment(n.Terminal).term
}

// openFind opens the find bar for the focused pane.
func (s *Shell) openFind() {
	if s.findOpen {
		return
	}
	s.findQuery = ""
	s.findOpen = true
}

// endFind ends the terminal search and gives the keyboard back to the
// focused pane, whichever way the bar closed.
func (s *Shell) endFind() {
	if term := s.findTargetTerminal(); term != nil {
		term.EndSearch()
	}
	s.findQuery = ""
	s.focusPending = true
}

// closeFind closes the bar and ends its search.
func (s *Shell) closeFind() {
	if !s.findOpen {
		return
	}
	s.findOpen = false
	s.endFind()
}

// findStep maps a move of the bar's current match to the terminal's, which
// counts matches from the newest. It reports whether the move went forward,
// toward older output, and whether the bar moved at all. Wraps go from one
// end to the other, so they are read before the index's direction.
func findStep(before, after, matches int) (forward, moved bool) {
	if matches <= 1 || before == after {
		return false, false
	}
	if matches == 2 {
		// With two matches the index alone cannot tell a wrap from a
		// step: Next goes toward the older match, Previous toward the
		// newer one.
		return after > before, true
	}
	switch {
	case before == 0 && after == matches-1:
		return false, true // Previous wrapped from the newest to the oldest.
	case before == matches-1 && after == 0:
		return true, true // Next wrapped from the oldest to the newest.
	case after > before:
		return true, true
	default:
		return false, true
	}
}
