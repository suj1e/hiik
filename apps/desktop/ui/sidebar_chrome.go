package ui

import (
	"runtime"

	mui "github.com/egoist/mygo/ui"

	"hiik/icons"
)

// The sidebar's geometry: a compact default, a rail below the compact
// threshold, and a brand and footer that stay put around the two lists.
const (
	sidebarDefaultW = 228
	sidebarRailW    = 44
	sidebarMacRailW = 76
	sidebarCompactW = 160
)

// sidebarRailWidth is the collapsed width: wide enough for the macOS
// traffic lights on that platform, otherwise a compact icon rail.
func sidebarRailWidth() float32 {
	if runtime.GOOS == "darwin" {
		return sidebarMacRailW
	}
	return sidebarRailW
}

// sidebarCompact reports whether the sidebar shows its text labels.
func (s *Shell) sidebarCompact() bool {
	return s.sidebarW < sidebarCompactW
}

// activeAgentAddress returns the app's selected agent address, or "".
func (s *Shell) activeAgentAddress() string {
	if s.app == nil {
		return ""
	}
	return s.app.ActiveAgent()
}

// toggleSidebar collapses the sidebar to its rail, or restores the width it
// had before. A sidebar that was already narrow at startup restores the
// default width.
func (s *Shell) toggleSidebar() {
	if s.sidebarCompact() {
		width := s.sidebarPrevW
		if width < sidebarCompactW {
			width = sidebarDefaultW
		}
		s.sidebarW = width
		return
	}
	s.sidebarPrevW = s.sidebarW
	s.sidebarW = sidebarRailWidth()
}

// sidebarBrand draws the 46px row at the top of the sidebar: the logo, the
// app name, the palette button and the collapse toggle. On the rail it keeps
// a 32px tile that expands the sidebar again.
func (s *Shell) sidebarBrand(c *mui.Context) {
	t := c.Theme()
	row := mui.Row(c).
		FillWidth().
		Height(40).
		AlignItems(mui.Center).
		DragWindow()
	if s.sidebarCompact() {
		// macOS puts the traffic lights in this rail's brand row; a tile
		// there would sit under them. The footer still restores the width.
		if runtime.GOOS == "darwin" {
			return
		}
		row.Padding(0, 6).Justify(mui.Center).Children(func() {
			tile := mui.Row(c).
				Size(32, 32).
				Radius(8).
				Background(t.Accent).
				Justify(mui.Center).
				AlignItems(mui.Center).
				Tooltip("Expand sidebar").
				Cursor(mui.CursorPointer).
				OnClick(s.toggleSidebar)
			if tile.Hovered() {
				tile.Background(t.AccentHover)
			}
			tile.Children(func() {
				mui.Icon(c, icons.Mark).FontSize(t.FontSize + 3).TextColor(t.AccentText)
			})
		})
		return
	}
	left := max(c.TitleBar().Left, 14)
	if runtime.GOOS == "darwin" {
		left = max(left, 76)
	}
	row.Padding(0, t.Space(2.5), 0, left).Gap(t.Space(1.5)).Children(func() {
		mui.Icon(c, icons.Mark).FontSize(t.FontSize + 5).TextColor(t.Accent)
		mui.Text(c, "hiik").FontSize(t.FontSize + 0.5).Bold().TextColor(t.Text).SingleLine()
		mui.Box(c).Grow(1)
		chord := s.paletteChord()
		tip := "Command palette"
		if chord != "" {
			tip += " (" + chord + ")"
		}
		s.sidebarIconButton(c, icons.Search, tip, s.openPalette)
		s.sidebarIconButton(c, icons.PanelLeft, "Collapse sidebar", s.toggleSidebar)
	})
}

// sidebarIconButton is one square icon button of the sidebar brand row.
func (s *Shell) sidebarIconButton(c *mui.Context, icon *mui.SVG, tip string, fn func()) {
	t := c.Theme()
	button := mui.Row(c).
		Size(t.Space(6), t.Space(6)).
		Radius(t.Radius).
		Justify(mui.Center).
		AlignItems(mui.Center).
		Tooltip(tip).
		Label(tip).
		Cursor(mui.CursorPointer).
		OnClick(fn)
	color := t.TextMuted
	if button.Hovered() {
		button.Background(t.SurfaceHover)
		color = t.Text
	}
	button.Children(func() {
		mui.Icon(c, icon).FontSize(t.FontSize + 1).TextColor(color)
	})
}

// sidebarFooter draws the 69px footer: Settings, then Add machine. On the
// rail the two rows shrink to 32px icons.
func (s *Shell) sidebarFooter(c *mui.Context) {
	t := c.Theme()
	footer := mui.Column(c).
		FillWidth().
		Height(69).
		Background(t.Surface).
		BorderWidth(1, 0, 0, 0).
		BorderColor(t.Border)
	if s.sidebarCompact() {
		footer.Padding(4, 0, 8, 0).AlignItems(mui.Center)
	} else {
		footer.Padding(4, 8, 8, 8)
	}
	footer.Children(func() {
		s.sidebarFooterRow(c, "Settings", "⚙", func() { s.openSettings() })
		s.sidebarFooterRow(c, "Add machine", "+", func() { s.machinesOpen = true })
	})
}

// sidebarFooterRow is one row of the sidebar footer.
func (s *Shell) sidebarFooterRow(c *mui.Context, label, glyph string, fn func()) {
	t := c.Theme()
	compact := s.sidebarCompact()
	row := mui.Row(c).
		Height(28).
		Radius(8).
		AlignItems(mui.Center).
		Tooltip(label).
		Cursor(mui.CursorPointer).
		OnClick(fn)
	color := t.TextMuted
	if row.Hovered() {
		row.Background(t.SurfaceHover)
		color = t.Text
	}
	if compact {
		row.Width(32).Justify(mui.Center)
	} else {
		row.FillWidth().Padding(0, 8).Gap(9)
	}
	row.Children(func() {
		mui.Text(c, glyph).FontSize(t.FontSize + 1).TextColor(color)
		if !compact {
			mui.Text(c, label).FontSize(t.FontSize).TextColor(color).SingleLine()
		}
	})
}
