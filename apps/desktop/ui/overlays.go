package ui

import (
	"strings"

	mui "github.com/egoist/mygo/ui"

	"hiik/mods/api"
)

// applyOverlay opens the overlay a command asked for and clears the request.
func (s *Shell) applyOverlay() {
	switch s.app.Overlay() {
	case "palette":
		s.paletteOpen = true
	case "settings":
		s.settingsOpen = true
	case "about":
		s.aboutOpen = true
	case "whats_new":
		s.whatsNewOpen = true
	case "machines":
		s.machinesOpen = true
	case "peek":
		s.peek = s.app.ActiveAgent()
	default:
		return
	}
	s.app.SetOverlay("")
}

// about draws the about panel over the window.
func (s *Shell) about(c *mui.Context) {
	s.renderPanel(c, api.PanelAbout, nil)
}

// whatsNew draws the bundled release notes.
func (s *Shell) whatsNew(c *mui.Context) {
	t := c.Theme()
	mui.Column(c).Width(640).Gap(10).Children(func() {
		mui.Text(c, "What's new").FontSize(t.FontSize + 2).Bold()
		mui.Scroll(c).MaxHeight(460).Children(func() {
			mui.Text(c, s.app.ReleaseNotes()).FontSize(t.FontSize - 1).TextColor(t.TextMuted)
		})
		mui.Row(c).Justify(mui.End).Children(func() {
			mui.Button(c, "Done").OnClick(func() { s.whatsNewOpen = false })
		})
	})
}

// machines draws the machine manager over the window.
func (s *Shell) machines(c *mui.Context) {
	s.renderPanel(c, api.PanelMachines, nil)
}

// overlays draws the command palette, settings, support dialogs and the
// small shell dialogs.
func (s *Shell) overlays(c *mui.Context) {
	if s.aboutOpen {
		open := s.aboutOpen
		mui.Modal(c, &s.aboutOpen, func() { s.about(c) })
		if open && !s.aboutOpen {
			s.restorePaneFocus()
		}
	}
	if s.whatsNewOpen {
		open := s.whatsNewOpen
		mui.Modal(c, &s.whatsNewOpen, func() { s.whatsNew(c) })
		if open && !s.whatsNewOpen {
			s.restorePaneFocus()
		}
	}
	if s.machinesOpen {
		open := s.machinesOpen
		mui.Modal(c, &s.machinesOpen, func() { s.machines(c) })
		if open && !s.machinesOpen {
			s.restorePaneFocus()
		}
	}
	if s.paletteOpen {
		open := s.paletteOpen
		mui.Modal(c, &s.paletteOpen, func() { s.palette(c) })
		if open && !s.paletteOpen {
			s.restorePaneFocus()
		}
	}
	if s.settingsOpen {
		open := s.settingsOpen
		mui.Modal(c, &s.settingsOpen, func() { s.settings(c) })
		if open && !s.settingsOpen {
			s.restorePaneFocus()
		}
	}
	if s.worktreeOpen {
		open := s.worktreeOpen
		mui.Modal(c, &s.worktreeOpen, func() { s.worktree(c) })
		if open && !s.worktreeOpen {
			s.restorePaneFocus()
		}
	}
	if s.peek != "" {
		open := true
		mui.Modal(c, &open, func() { s.peekView(c, &open) })
		if !open {
			s.peek = ""
			s.restorePaneFocus()
		}
	}
	if s.dialog.kind != dialogNone {
		open := true
		mui.Modal(c, &open, func() { s.dialogView(c, &open) })
		if !open {
			s.dialog = dialog{}
			s.restorePaneFocus()
		}
	}
}

// restorePaneFocus gives the keyboard back to the pane focused before an
// overlay opened. The next frame calls Terminal.Focus once.
func (s *Shell) restorePaneFocus() {
	if s.focus != "" {
		s.focusPending = true
	}
}

// palette draws the command palette.
func (s *Shell) palette(c *mui.Context) {
	t := c.Theme()
	mui.Column(c).Width(560).Gap(8).Children(func() {
		mui.Text(c, "Commands").FontSize(t.FontSize - 2).TextColor(t.TextMuted)
		input := mui.SearchField(c, &s.paletteQuery).
			AutoFocus().
			Label("Search commands").
			Placeholder("Search commands")
		input.OnShortcut(0, mui.KeyDown, func() { s.paletteMove(1) })
		input.OnShortcut(0, mui.KeyUp, func() { s.paletteMove(-1) })
		if input.Changed() {
			s.paletteSel = 0
		}
		cmds := filteredCommands(s.app.Commands(), s.paletteQuery)
		if s.paletteSel >= len(cmds) {
			s.paletteSel = len(cmds) - 1
		}
		if s.paletteSel < 0 {
			s.paletteSel = 0
		}
		run := func(i int) {
			if i < 0 || i >= len(cmds) {
				return
			}
			id := cmds[i].ID
			s.paletteOpen = false
			s.run(id, nil)
		}
		s.paletteList.Selected = &s.paletteSel
		s.paletteList.Label = func(i int) string {
			if i < 0 || i >= len(cmds) {
				return ""
			}
			return cmds[i].Title
		}
		list := mui.List(c, &s.paletteList, len(cmds)).Width(560).MaxHeight(360)
		list.Rows(func(row mui.ListRow) {
			i := row.Index
			if i < 0 || i >= len(cmds) {
				return
			}
			cmd := cmds[i]
			r := mui.Row(row.Context).
				Height(28).
				Padding(0, 10).
				Radius(t.Radius).
				AlignItems(mui.Center).
				OnClick(func() { run(i) })
			switch {
			case row.Selected() || i == s.paletteSel:
				r.Background(t.SurfaceHover)
			}
			r.Children(func() {
				mui.Text(c, cmd.Title).FontSize(t.FontSize - 1).SingleLine()
				mui.Box(c).Grow(1)
				mui.Text(c, cmd.ID).FontSize(t.FontSize - 3).TextColor(t.TextMuted)
			})
		})
		if len(cmds) == 0 {
			mui.Text(c, "No matching commands").FontSize(t.FontSize - 2).TextColor(t.TextMuted)
		}
		if input.Submitted() || list.Submitted() {
			run(s.paletteSel)
		}
		mui.Text(c, "↑↓ choose · Enter run · Esc close").FontSize(t.FontSize - 3).TextColor(t.TextMuted)
	})
}

// paletteMove moves the palette's choice by delta within the filtered list.
func (s *Shell) paletteMove(delta int) {
	cmds := filteredCommands(s.app.Commands(), s.paletteQuery)
	if len(cmds) == 0 {
		s.paletteSel = 0
		return
	}
	s.paletteSel += delta
	if s.paletteSel < 0 {
		s.paletteSel = 0
	}
	if s.paletteSel >= len(cmds) {
		s.paletteSel = len(cmds) - 1
	}
}

// worktree draws the worktree modal for the active space.
func (s *Shell) worktree(c *mui.Context) {
	t := c.Theme()
	sp, _ := s.activeTab()
	mui.Column(c).Width(600).Gap(8).Children(func() {
		mui.Row(c).AlignItems(mui.Center).Children(func() {
			mui.Text(c, "Worktrees").FontSize(t.FontSize).Bold()
			mui.Box(c).Grow(1)
			mui.Button(c, "Close").OnClick(func() { s.worktreeOpen = false })
		})
		if sp == nil {
			mui.Text(c, "No space open.").FontSize(t.FontSize - 1).TextColor(t.TextMuted)
			return
		}
		s.renderPanel(c, api.PanelWorktree, map[string]any{
			"space": sp,
		})
	})
}

// peekView draws the peek modal over the tab.
func (s *Shell) peekView(c *mui.Context, open *bool) {
	t := c.Theme()
	address := s.peek
	mui.Column(c).Width(760).MaxHeight(600).Gap(8).Children(func() {
		mui.Row(c).AlignItems(mui.Center).Gap(8).Children(func() {
			mui.Text(c, "Peek").FontSize(t.FontSize).Bold()
			mui.Text(c, address).FontSize(t.FontSize - 2).TextColor(t.TextMuted).SingleLine()
			mui.Box(c).Grow(1)
			mui.Button(c, "Close").OnClick(func() { *open = false })
		})
		s.renderPanel(c, api.PanelPeek, map[string]any{
			"terminal": address,
		})
	})
}

// settings draws the settings modal.
func (s *Shell) settings(c *mui.Context) {
	s.settingsView(c, s.app.Settings())
}

// dialogView draws the shell's rename or confirm prompt.
func (s *Shell) dialogView(c *mui.Context, open *bool) {
	t := c.Theme()
	d := s.dialog
	mui.Column(c).Width(360).Gap(8).Children(func() {
		mui.Text(c, d.title).FontSize(t.FontSize).Bold()
		switch d.kind {
		case dialogRename:
			input := mui.TextInput(c, &s.dialog.value).AutoFocus()
			ok := func() {
				name := strings.TrimSpace(s.dialog.value)
				fn := s.dialog.rename
				*open = false
				if fn != nil {
					fn(name)
				}
			}
			row := mui.Row(c).Gap(8).Justify(mui.End)
			row.Children(func() {
				mui.Button(c, "Cancel").OnClick(func() { *open = false })
				mui.Button(c, "Rename").OnClick(ok)
			})
			if input.Submitted() {
				ok()
			}
		case dialogConfirm:
			mui.Text(c, d.message).FontSize(t.FontSize - 1).TextColor(t.TextMuted)
			mui.Row(c).Gap(8).Justify(mui.End).Children(func() {
				mui.Button(c, "Cancel").OnClick(func() { *open = false })
				mui.Button(c, "Confirm").OnClick(func() {
					fn := s.dialog.confirm
					*open = false
					if fn != nil {
						fn()
					}
				})
			})
		}
	})
}
