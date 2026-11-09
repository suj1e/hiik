package ui

import (
	"github.com/egoist/mygo/plugins/terminal"
	mui "github.com/egoist/mygo/ui"
)

// defaultTerminalSize is the terminal font size in DIPs when the setting is
// unset, as terminal.Options does.
const defaultTerminalSize float32 = 13

// terminalLook builds a terminal's colors and font from the UI theme and the
// look settings. It uses only the theme's colors, so a theme change reaches
// the terminal without a second palette.
func terminalLook(t *mui.Theme, settings map[string]any) (*terminal.Theme, terminal.Font) {
	if t == nil {
		t = mui.DarkTheme()
	}
	th := &terminal.Theme{
		Foreground:    t.Text,
		Background:    t.Background,
		Cursor:        t.Accent,
		CursorText:    t.Background,
		Selection:     t.Selection,
		SelectionText: t.Text,
		Palette:       terminalPaletteFor(t),
	}
	// mygo draws a theme color's alpha, so less than one lets what is
	// behind the terminal show through. The terminal has no blend of its
	// own; this is the closest the plugin offers to background-opacity.
	if opacity := terminalOpacity(settings); opacity < 1 {
		th.Background = mui.RGBA(th.Background.R, th.Background.G, th.Background.B, opacity)
	}
	font := terminal.Font{
		Family: settingString(settings, "terminal_font"),
		Size:   terminalFontSize(settings),
	}
	// Coding fonts make most of their ligatures with calt and the rest with
	// liga. Turning both off is how mygo exposes the reference's
	// font_ligatures = false.
	if !settingBool(settings, "ligatures", false) {
		font.Features = []string{"-calt", "-liga"}
	}
	return th, font
}

// terminalFontSize is the terminal font size. The frame's zoom scales the
// whole view, so the terminal does not multiply its font again.
func terminalFontSize(settings map[string]any) float32 {
	return settingFloat(settings, "terminal_size", defaultTerminalSize)
}

// terminalOpacity is the terminal background's opacity, from 0 to 1.
func terminalOpacity(settings map[string]any) float32 {
	opacity := settingNumber(settings, "opacity", 1)
	if opacity < 0 {
		return 0
	}
	if opacity > 1 {
		return 1
	}
	return opacity
}

// terminalTransparent reports whether the terminal's background should not
// be drawn at all. mygo has no opacity setting on the view, so the layout
// draws the background undrawn only at zero; between zero and one the theme
// carries the alpha.
func terminalTransparent(settings map[string]any) bool {
	return terminalOpacity(settings) <= 0
}

// terminalPadding is the room around the terminal grid. A zero setting means
// no padding, which mygo spells as a negative value because zero asks for its
// 6 by 4 default. The frame's zoom scales the whole view.
func terminalPadding(settings map[string]any) terminal.Padding {
	pad := func(value float32) float32 {
		if value <= 0 {
			return -1
		}
		return value
	}
	return terminal.Padding{
		X: pad(settingNumber(settings, "padding_x", 10)),
		Y: pad(settingNumber(settings, "padding_y", 6)),
	}
}

// terminalCursor is the cursor shape and whether it never blinks. mygo's
// cursor shapes are the three the settings offer.
func terminalCursor(settings map[string]any) (terminal.CursorStyle, bool) {
	shape := terminal.CursorBlock
	switch settingString(settings, "cursor_shape") {
	case "bar":
		shape = terminal.CursorBar
	case "underline":
		shape = terminal.CursorUnderline
	}
	return shape, !settingBool(settings, "cursor_blink", true)
}

// optionAsAlt reports whether Option should act as the terminal's Alt.
// mygo has one switch, so the left and right choices both turn it on; the
// other Option key still types characters on a Mac with either choice.
func optionAsAlt(settings map[string]any) bool {
	if b, ok := settings["option_as_alt"].(bool); ok {
		return b
	}
	switch settingString(settings, "option_key") {
	case "alt", "left", "right":
		return true
	default:
		return false
	}
}

// paneScrollbars reports whether a terminal shows its scrollback thumb.
// mygo draws it only while the pointer is over the view and has no switch to
// hide it, so this is exposed for the pane frame to honor.
func paneScrollbars(settings map[string]any) bool {
	return settingBool(settings, "pane_scrollbars", true)
}

// terminalPalette derives the 16 ANSI colors from the theme's colors: the
// background and text for black and white, and the semantic colors for the
// rest, with the bright half mixed toward the theme's far end.
func terminalPalette(t *mui.Theme) [16]mui.Color {
	black, white := t.Background, t.Text
	brightEdge := t.Text
	brightBlack, brightWhite := t.TextMuted, t.Text
	if !t.Dark {
		black, white = t.Text, t.TextMuted
		brightEdge = t.Background
		brightWhite = t.Background
	}
	base := [8]mui.Color{
		black,
		t.Danger,
		t.Success,
		t.Warning,
		t.Accent,
		t.Accent.Mix(t.Danger, 0.5),
		t.Accent.Mix(t.Success, 0.5),
		white,
	}
	var p [16]mui.Color
	for i, c := range base {
		p[i] = c
		p[8+i] = c.Mix(brightEdge, 0.35)
	}
	p[8] = brightBlack
	p[15] = brightWhite
	return p
}

// terminalPaletteFor prefers a theme file's 16 ANSI colors and falls back to
// the semantic palette.
func terminalPaletteFor(t *mui.Theme) [16]mui.Color {
	if p, ok := ThemePalette(t); ok {
		return p
	}
	return terminalPalette(t)
}

// termLookKey identifies a terminal look so the shell changes an attached
// terminal only when the look changed. The theme pointer is the cached
// theme, so equal settings share it.
type termLookKey struct {
	theme     *mui.Theme
	family    string
	size      float32
	ligatures bool
	opacity   float32
}

func terminalLookKey(t *mui.Theme, settings map[string]any) termLookKey {
	return termLookKey{
		theme:     t,
		family:    settingString(settings, "terminal_font"),
		size:      terminalFontSize(settings),
		ligatures: settingBool(settings, "ligatures", false),
		opacity:   terminalOpacity(settings),
	}
}

// shellTermLook is the terminal's latest colors and font, kept by the shell
// so an attach and a live update use the same values.
type shellTermLook struct {
	theme *terminal.Theme
	font  terminal.Font
	key   termLookKey
}

// refreshTermLook updates the terminal look from the UI theme and the
// settings. It changes attached terminals only when the look changed.
func (s *Shell) refreshTermLook(t *mui.Theme, settings map[string]any) {
	key := terminalLookKey(t, settings)
	s.attachmentsMu.Lock()
	if s.termLook != nil && s.termLook.key == key {
		s.attachmentsMu.Unlock()
		return
	}
	theme, font := terminalLook(t, settings)
	s.termLook = &shellTermLook{theme: theme, font: font, key: key}
	terms := make([]*paneTerm, 0, len(s.attachments))
	for _, ts := range s.attachments {
		if ts.term != nil {
			terms = append(terms, ts)
		}
	}
	s.attachmentsMu.Unlock()
	for _, ts := range terms {
		if ts.term != nil {
			ts.term.SetTheme(theme, nil)
			ts.term.SetFont(font)
		}
	}
}

// terminalLookFor returns the terminal colors and font to attach with.
func (s *Shell) terminalLookFor() (*terminal.Theme, terminal.Font) {
	s.attachmentsMu.Lock()
	defer s.attachmentsMu.Unlock()
	if s.termLook != nil {
		return s.termLook.theme, s.termLook.font
	}
	theme, font := terminalLook(mui.DarkTheme(), nil)
	return theme, font
}
