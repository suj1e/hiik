package core

import (
	"errors"
	"fmt"
	"strings"

	"hiik/core/store"
)

// Look is the part of the settings that changes how the app looks: the
// appearance, the theme (global and per layout), the fonts, the terminal's
// size, padding, opacity, cursor and scrollbars, the zoom, the core path
// and the keymap overrides. It is a copy; mutating it does not change the
// app.
type Look struct {
	Appearance      string
	Theme           string
	Layout          string
	LayoutThemes    map[string]string
	OptionKey       string
	UIFont          string
	TerminalFont    string
	TerminalSize    float32
	Ligatures       bool
	PaddingX        float32
	PaddingY        float32
	Opacity         float32
	CursorShape     string
	CursorBlink     bool
	PaneScrollbars  bool
	Zoom            float32
	MinimumContrast float32
	ClipboardRead   bool
	CorePath        string
	Keys            map[string]string
	SoundsDone      string
	SoundsBlocked   string
}

// errNotLookKey reports that a key is not one setLookValue changes.
var errNotLookKey = errors.New("core: not a look setting")

// lookOf returns the look values with defaults filled, without changing the
// stored settings.
func lookOf(s store.Settings) Look {
	l := Look{
		Appearance:      strings.TrimSpace(s.Appearance),
		Theme:           strings.TrimSpace(s.Theme),
		Layout:          strings.TrimSpace(s.Layout),
		LayoutThemes:    cloneStringMap(s.LayoutThemes),
		OptionKey:       strings.TrimSpace(s.OptionKey),
		UIFont:          strings.TrimSpace(s.UIFont),
		TerminalFont:    strings.TrimSpace(s.TerminalFont),
		TerminalSize:    s.TerminalSize,
		Ligatures:       s.Ligatures,
		PaddingX:        s.PaddingX,
		PaddingY:        s.PaddingY,
		Opacity:         s.Opacity,
		CursorShape:     strings.ToLower(strings.TrimSpace(s.CursorShape)),
		CursorBlink:     s.CursorBlink,
		PaneScrollbars:  s.PaneScrollbars,
		Zoom:            s.Zoom,
		MinimumContrast: s.MinimumContrast,
		ClipboardRead:   s.ClipboardRead,
		CorePath:        strings.TrimSpace(s.CorePath),
		Keys:            cloneKeys(s.Keys),
		SoundsDone:      strings.TrimSpace(s.SoundsDone),
		SoundsBlocked:   strings.TrimSpace(s.SoundsBlocked),
	}
	if l.Appearance == "" {
		l.Appearance = store.AppearanceSystem
	}
	if l.Layout == "" {
		l.Layout = store.LayoutHerdr
	}
	if l.OptionKey == "" {
		l.OptionKey = store.OptionAuto
	}
	if l.TerminalSize == 0 {
		l.TerminalSize = store.DefaultTerminalSize
	}
	if l.CursorShape == "" {
		l.CursorShape = store.CursorBlock
	}
	if l.Zoom == 0 {
		l.Zoom = store.DefaultZoom
	}
	if l.MinimumContrast == 0 {
		l.MinimumContrast = store.DefaultMinimumContrast
	}
	return l
}

// Look returns the current look settings. The returned Keys map is a copy.
func (a *App) Look() Look {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return lookOf(a.settings)
}

// settingsView builds the toggles map for Settings. a.mu is held.
func (a *App) settingsView() map[string]any {
	l := lookOf(a.settings)
	out := map[string]any{
		"notify_blocked":   a.settings.NotifyBlocked,
		"notify_done":      a.settings.NotifyDone,
		"sound":            a.settings.Sound,
		"sound_done":       l.SoundsDone,
		"sound_blocked":    l.SoundsBlocked,
		"clipboard_write":  a.settings.Privacy.ClipboardWrite == store.ClipboardAlways,
		"clipboard_read":   l.ClipboardRead,
		"telemetry":        a.settings.Privacy.Telemetry,
		"appearance":       l.Appearance,
		"theme":            l.Theme,
		"layout":           l.Layout,
		"layout_themes":    l.LayoutThemes,
		"option_key":       l.OptionKey,
		"option_as_alt":    OptionAsAlt(l.OptionKey),
		"ui_font":          l.UIFont,
		"terminal_font":    l.TerminalFont,
		"terminal_size":    l.TerminalSize,
		"ligatures":        l.Ligatures,
		"padding_x":        l.PaddingX,
		"padding_y":        l.PaddingY,
		"opacity":          l.Opacity,
		"cursor_shape":     l.CursorShape,
		"cursor_blink":     l.CursorBlink,
		"pane_scrollbars":  l.PaneScrollbars,
		"zoom":             l.Zoom,
		"minimum_contrast": l.MinimumContrast,
		"core_path":        l.CorePath,
		"keys":             l.Keys,
	}
	return out
}

// OptionAsAlt reports whether an option_key value makes the Option key act
// as the terminal's Alt. mygo's terminal has one OptionAsAlt switch, so the
// left and right values both turn it on; the other Option key cannot stay a
// character key. Only macOS is affected.
func OptionAsAlt(optionKey string) bool {
	switch strings.ToLower(strings.TrimSpace(optionKey)) {
	case store.OptionAlt, store.OptionLeft, store.OptionRight:
		return true
	default:
		return false
	}
}

// cloneKeys copies a keymap so callers cannot mutate the stored one.
func cloneKeys(keys map[string]string) map[string]string {
	return cloneStringMap(keys)
}

// cloneStringMap copies a string map, returning an empty map for nil so a
// caller can write to it.
func cloneStringMap(m map[string]string) map[string]string {
	if len(m) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(m))
	for key, value := range m {
		out[key] = value
	}
	return out
}

// normalizeLook trims free-form look values and repairs the ones with a
// known set of values. It leaves a user theme name alone: themes on disk are
// not enumerated here.
func normalizeLook(s *store.Settings) {
	s.Appearance = strings.TrimSpace(s.Appearance)
	s.Theme = strings.TrimSpace(s.Theme)
	s.Layout = strings.TrimSpace(s.Layout)
	s.OptionKey = strings.TrimSpace(s.OptionKey)
	s.UIFont = strings.TrimSpace(s.UIFont)
	s.TerminalFont = strings.TrimSpace(s.TerminalFont)
	s.CorePath = strings.TrimSpace(s.CorePath)
	s.CursorShape = strings.ToLower(strings.TrimSpace(s.CursorShape))
	s.SoundsDone = strings.TrimSpace(s.SoundsDone)
	s.SoundsBlocked = strings.TrimSpace(s.SoundsBlocked)
	s.LayoutThemes = store.NormalizeThemeMap(s.LayoutThemes)
	if len(s.Keys) == 0 {
		s.Keys = nil
	}
}

// setLookValue changes one look key in s. It returns errNotLookKey when the
// key is not a look setting, so app.set can report an unknown key.
func setLookValue(s *store.Settings, key string, value any) error {
	switch key {
	case "appearance":
		v, ok := value.(string)
		if !ok {
			return fmt.Errorf("app.set: appearance needs a string")
		}
		switch v = strings.TrimSpace(v); v {
		case store.AppearanceSystem, store.AppearanceLight, store.AppearanceDark:
			s.Appearance = v
		default:
			return fmt.Errorf("app.set: appearance %q is unknown", v)
		}
	case "theme":
		v, ok := value.(string)
		if !ok {
			return fmt.Errorf("app.set: theme needs a string")
		}
		s.Theme = strings.TrimSpace(v)
	case "layout":
		v, ok := value.(string)
		if !ok {
			return fmt.Errorf("app.set: layout needs a string")
		}
		if v = strings.TrimSpace(v); v == "" {
			return fmt.Errorf("app.set: layout must not be empty")
		}
		// A user layout from layouts/*.json has its own name; the layout
		// engine validates it at load time.
		s.Layout = v
	case "option_key":
		v, ok := value.(string)
		if !ok {
			return fmt.Errorf("app.set: option_key needs a string")
		}
		switch v = strings.TrimSpace(v); v {
		case store.OptionAuto, store.OptionAlt, store.OptionNative, store.OptionLeft, store.OptionRight:
			s.OptionKey = v
		default:
			return fmt.Errorf("app.set: option_key %q is unknown", v)
		}
	case "ui_font":
		v, ok := value.(string)
		if !ok {
			return fmt.Errorf("app.set: ui_font needs a string")
		}
		s.UIFont = strings.TrimSpace(v)
	case "terminal_font":
		v, ok := value.(string)
		if !ok {
			return fmt.Errorf("app.set: terminal_font needs a string")
		}
		s.TerminalFont = strings.TrimSpace(v)
	case "terminal_size":
		f, ok := floatNumber(value)
		if !ok {
			return fmt.Errorf("app.set: terminal_size needs a number")
		}
		size := float32(f)
		if size < 6 || size > 72 {
			return fmt.Errorf("app.set: terminal_size %g is out of range", f)
		}
		s.TerminalSize = size
	case "ligatures":
		b, ok := value.(bool)
		if !ok {
			return fmt.Errorf("app.set: ligatures needs a bool")
		}
		s.Ligatures = b
	case "padding_x", "padding_y":
		f, ok := floatNumber(value)
		if !ok {
			return fmt.Errorf("app.set: %s needs a number", key)
		}
		pad := float32(f)
		if pad < store.MinPadding || pad > store.MaxPadding {
			return fmt.Errorf("app.set: %s %g is out of range", key, f)
		}
		if key == "padding_x" {
			s.PaddingX = pad
		} else {
			s.PaddingY = pad
		}
	case "opacity":
		f, ok := floatNumber(value)
		if !ok {
			return fmt.Errorf("app.set: opacity needs a number")
		}
		opacity := float32(f)
		if opacity < store.MinOpacity || opacity > store.MaxOpacity {
			return fmt.Errorf("app.set: opacity %g is out of range", f)
		}
		s.Opacity = opacity
	case "cursor_shape":
		v, ok := value.(string)
		if !ok {
			return fmt.Errorf("app.set: cursor_shape needs a string")
		}
		switch v = strings.ToLower(strings.TrimSpace(v)); v {
		case store.CursorBlock, store.CursorBar, store.CursorUnderline:
			s.CursorShape = v
		default:
			return fmt.Errorf("app.set: cursor_shape %q is unknown", v)
		}
	case "cursor_blink":
		b, ok := value.(bool)
		if !ok {
			return fmt.Errorf("app.set: cursor_blink needs a bool")
		}
		s.CursorBlink = b
	case "pane_scrollbars":
		b, ok := value.(bool)
		if !ok {
			return fmt.Errorf("app.set: pane_scrollbars needs a bool")
		}
		s.PaneScrollbars = b
	case "zoom":
		f, ok := floatNumber(value)
		if !ok {
			return fmt.Errorf("app.set: zoom needs a number")
		}
		zoom := float32(f)
		if zoom < store.MinZoom || zoom > store.MaxZoom {
			return fmt.Errorf("app.set: zoom %g is out of range", f)
		}
		s.Zoom = zoom
	case "layout_themes":
		v, ok := stringMap(value)
		if !ok {
			return fmt.Errorf("app.set: layout_themes needs a map of strings")
		}
		s.LayoutThemes = store.NormalizeThemeMap(cloneStringMap(v))
	case "core_path":
		v, ok := value.(string)
		if !ok {
			return fmt.Errorf("app.set: core_path needs a string")
		}
		s.CorePath = strings.TrimSpace(v)
	case "minimum_contrast":
		f, ok := floatNumber(value)
		if !ok {
			return fmt.Errorf("app.set: minimum_contrast needs a number")
		}
		ratio := float32(f)
		if ratio < store.MinMinimumContrast || ratio > store.MaxMinimumContrast {
			return fmt.Errorf("app.set: minimum_contrast %g is out of range", f)
		}
		s.MinimumContrast = ratio
	case "clipboard_read":
		b, ok := value.(bool)
		if !ok {
			return fmt.Errorf("app.set: clipboard_read needs a bool")
		}
		s.ClipboardRead = b
	case "sound_done":
		v, ok := value.(string)
		if !ok {
			return fmt.Errorf("app.set: sound_done needs a string")
		}
		s.SoundsDone = strings.TrimSpace(v)
	case "sound_blocked":
		v, ok := value.(string)
		if !ok {
			return fmt.Errorf("app.set: sound_blocked needs a string")
		}
		s.SoundsBlocked = strings.TrimSpace(v)
	case "keys":
		v, ok := stringMap(value)
		if !ok {
			return fmt.Errorf("app.set: keys needs a map of strings")
		}
		s.Keys = cloneKeys(v)
	default:
		return errNotLookKey
	}
	return nil
}
