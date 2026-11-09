package ui

import (
	"runtime"

	mui "github.com/egoist/mygo/ui"
)

// The sidebar's geometry, matching the reference app: a wide default, a rail
// below the compact threshold, and a brand and footer that stay put around
// the two lists.
const (
	sidebarDefaultW = 268
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
		Height(46).
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
				mui.Text(c, "h").FontSize(15).Bold().TextColor(t.AccentText)
			})
		})
		return
	}
	left := max(c.TitleBar().Left, 14)
	if runtime.GOOS == "darwin" {
		left = max(left, 76)
	}
	row.Padding(0, 10, 0, left).Gap(4).Children(func() {
		mui.Row(c).
			Size(20, 20).
			Radius(5).
			Background(t.Accent).
			Justify(mui.Center).
			AlignItems(mui.Center).
			Children(func() {
				mui.Text(c, "h").FontSize(12).Bold().TextColor(t.AccentText)
			})
		mui.Text(c, "hiik").FontSize(13.5).Bold().TextColor(t.Text).SingleLine()
		mui.Box(c).Grow(1)
		s.sidebarIconButton(c, "P", "Command palette ("+shortcutLabel("P")+")", s.openPalette)
		s.sidebarIconButton(c, "«", "Collapse sidebar", s.toggleSidebar)
	})
}

// sidebarIconButton is one 26px square button of the sidebar brand row.
func (s *Shell) sidebarIconButton(c *mui.Context, glyph, tip string, fn func()) {
	t := c.Theme()
	button := mui.Row(c).
		Size(26, 26).
		Radius(6).
		Justify(mui.Center).
		AlignItems(mui.Center).
		Tooltip(tip).
		Cursor(mui.CursorPointer).
		OnClick(fn)
	color := t.TextMuted
	if button.Hovered() {
		button.Background(t.SurfaceHover)
		color = t.Text
	}
	button.Children(func() {
		mui.Text(c, glyph).FontSize(13).TextColor(color)
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
		mui.Text(c, glyph).FontSize(14).TextColor(color)
		if !compact {
			mui.Text(c, label).FontSize(13).TextColor(color).SingleLine()
		}
	})
}
