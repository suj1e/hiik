package store

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Appearance values.
const (
	AppearanceSystem = "system"
	AppearanceLight  = "light"
	AppearanceDark   = "dark"
)

// Layout values.
const (
	LayoutHerdr     = "herdr"
	LayoutTerminals = "terminals"
)

// OptionKey values: how macOS Option behaves in terminals.
const (
	OptionAuto   = "auto"
	OptionAlt    = "alt"
	OptionNative = "native"
	OptionLeft   = "left"
	OptionRight  = "right"
)

// CursorShape values: the shape a terminal draws its cursor with.
const (
	CursorBlock     = "block"
	CursorBar       = "bar"
	CursorUnderline = "underline"
)

// Clipboard write policies.
const (
	ClipboardAlways = "always"
	ClipboardAsk    = "ask"
	ClipboardNever  = "never"
)

// DefaultTerminalSize is the terminal font size in points.
const DefaultTerminalSize float32 = 13

// Terminal padding and opacity defaults, matching the reference's look.
const (
	DefaultPaddingX float32 = 10
	DefaultPaddingY float32 = 6
	MinPadding      float32 = 0
	MaxPadding      float32 = 80

	DefaultOpacity float32 = 1
	MinOpacity     float32 = 0
	MaxOpacity     float32 = 1
)

// App zoom limits: one means as designed; the zoom commands step in tenths.
const (
	DefaultZoom float32 = 1
	MinZoom     float32 = 0.5
	MaxZoom     float32 = 3
)

// Terminal text contrast limits, as a WCAG ratio.
const (
	DefaultMinimumContrast float32 = 1
	MinMinimumContrast     float32 = 1
	MaxMinimumContrast     float32 = 21
)

// Settings is settings.json. Unknown keys are kept in Extra and written
// back unchanged.
type Settings struct {
	Appearance   string            `json:"appearance,omitempty"`
	Theme        string            `json:"theme,omitempty"`
	Layout       string            `json:"layout,omitempty"`
	OptionKey    string            `json:"optionKey,omitempty"`
	UIFont       string            `json:"uiFont,omitempty"`
	TerminalFont string            `json:"terminalFont,omitempty"`
	TerminalSize float32           `json:"terminalSize,omitempty"`
	CorePath     string            `json:"corePath,omitempty"`
	Shell        string            `json:"shell,omitempty"`
	Keys         map[string]string `json:"keys,omitempty"`

	// LayoutThemes picks a theme for one layout, by layout name. A missing
	// entry falls back to Theme.
	LayoutThemes map[string]string `json:"layoutThemes,omitempty"`

	// Terminal detail. Ligatures turn programming ligatures on in the
	// terminal font. PaddingX and PaddingY are the room around the grid in
	// DIPs. Opacity is the terminal background's opacity from 0 to 1.
	Ligatures   bool    `json:"ligatures"`
	PaddingX    float32 `json:"paddingX"`
	PaddingY    float32 `json:"paddingY"`
	Opacity     float32 `json:"opacity"`
	CursorShape string  `json:"cursorShape,omitempty"`
	CursorBlink bool    `json:"cursorBlink"`
	// PaneScrollbars shows the scrollback thumb at a terminal's edge.
	PaneScrollbars bool `json:"paneScrollbars"`

	// Zoom scales the app from 0.5 to 3.
	Zoom float32 `json:"zoom,omitempty"`

	// MinimumContrast is the least contrast terminal text has with its
	// background, a WCAG ratio from 1 (none) to 21.
	MinimumContrast float32 `json:"minimumContrast,omitempty"`
	// ClipboardRead lets a program read the clipboard (OSC 52).
	ClipboardRead bool `json:"clipboardRead"`

	NotifyBlocked bool `json:"notifyBlocked"`
	NotifyDone    bool `json:"notifyDone"`
	Sound         bool `json:"sound"`
	// SoundsDone and SoundsBlocked are the files played for a done or a
	// blocked agent: absolute, or relative to HIIK_HOME. An empty value
	// plays the built-in tone.
	SoundsDone    string  `json:"soundDone,omitempty"`
	SoundsBlocked string  `json:"soundBlocked,omitempty"`
	Privacy       Privacy `json:"privacy"`

	// Extra holds keys this version does not know, so a newer file survives
	// a round trip through an older app.
	Extra map[string]json.RawMessage `json:"-"`
}

// Privacy is what the app may read or send.
type Privacy struct {
	Telemetry      bool   `json:"telemetry"`
	ClipboardWrite string `json:"clipboardWrite,omitempty"`
}

// DefaultSettings returns the settings used when no file exists.
func DefaultSettings() Settings {
	return Settings{
		Appearance:      AppearanceSystem,
		Layout:          LayoutHerdr,
		OptionKey:       OptionAuto,
		TerminalSize:    DefaultTerminalSize,
		PaddingX:        DefaultPaddingX,
		PaddingY:        DefaultPaddingY,
		Opacity:         DefaultOpacity,
		CursorShape:     CursorBlock,
		CursorBlink:     true,
		PaneScrollbars:  true,
		Zoom:            DefaultZoom,
		MinimumContrast: DefaultMinimumContrast,
		NotifyBlocked:   true,
		NotifyDone:      true,
		Sound:           true,
		Privacy: Privacy{
			ClipboardWrite: ClipboardAlways,
		},
	}
}

var settingKeys = []string{
	"appearance", "theme", "layout", "optionKey", "uiFont", "terminalFont", "terminalSize",
	"corePath", "shell", "keys", "layoutThemes",
	"ligatures", "paddingX", "paddingY", "opacity", "cursorShape", "cursorBlink",
	"paneScrollbars", "zoom",
	"minimumContrast", "clipboardRead",
	"notifyBlocked", "notifyDone", "sound", "soundDone", "soundBlocked",
	"privacy",
}

// Normalize fills missing values with defaults and reports values that were
// rejected.
func (s *Settings) Normalize() []string {
	var warnings []string
	switch s.Appearance {
	case "":
		s.Appearance = AppearanceSystem
	case AppearanceSystem, AppearanceLight, AppearanceDark:
	default:
		warnings = append(warnings, fmt.Sprintf(
			"settings: appearance %q is unknown; using %q", s.Appearance, AppearanceSystem))
		s.Appearance = AppearanceSystem
	}

	// A layout name may come from the person's own layouts/*.json files, so
	// any non-empty name is kept here; the layout engine validates it when
	// it loads the layout and falls back when it does not exist.
	s.Layout = strings.TrimSpace(s.Layout)
	if s.Layout == "" {
		s.Layout = LayoutHerdr
	}

	s.OptionKey = strings.TrimSpace(s.OptionKey)
	switch s.OptionKey {
	case "":
		s.OptionKey = OptionAuto
	case OptionAuto, OptionAlt, OptionNative, OptionLeft, OptionRight:
	default:
		warnings = append(warnings, fmt.Sprintf(
			"settings: optionKey %q is unknown; using %q", s.OptionKey, OptionAuto))
		s.OptionKey = OptionAuto
	}

	switch {
	case s.TerminalSize == 0:
		s.TerminalSize = DefaultTerminalSize
	case s.TerminalSize < 6 || s.TerminalSize > 72:
		warnings = append(warnings, fmt.Sprintf(
			"settings: terminalSize %g is out of range; using %g", s.TerminalSize, DefaultTerminalSize))
		s.TerminalSize = DefaultTerminalSize
	}

	if s.PaddingX < MinPadding || s.PaddingX > MaxPadding {
		warnings = append(warnings, fmt.Sprintf(
			"settings: paddingX %g is out of range; using %g", s.PaddingX, DefaultPaddingX))
		s.PaddingX = DefaultPaddingX
	}
	if s.PaddingY < MinPadding || s.PaddingY > MaxPadding {
		warnings = append(warnings, fmt.Sprintf(
			"settings: paddingY %g is out of range; using %g", s.PaddingY, DefaultPaddingY))
		s.PaddingY = DefaultPaddingY
	}
	if s.Opacity < MinOpacity || s.Opacity > MaxOpacity {
		warnings = append(warnings, fmt.Sprintf(
			"settings: opacity %g is out of range; using %g", s.Opacity, DefaultOpacity))
		s.Opacity = DefaultOpacity
	}
	s.CursorShape = strings.ToLower(strings.TrimSpace(s.CursorShape))
	switch s.CursorShape {
	case "":
		s.CursorShape = CursorBlock
	case CursorBlock, CursorBar, CursorUnderline:
	default:
		warnings = append(warnings, fmt.Sprintf(
			"settings: cursorShape %q is unknown; using %q", s.CursorShape, CursorBlock))
		s.CursorShape = CursorBlock
	}
	switch {
	case s.Zoom == 0:
		s.Zoom = DefaultZoom
	case s.Zoom < MinZoom || s.Zoom > MaxZoom:
		warnings = append(warnings, fmt.Sprintf(
			"settings: zoom %g is out of range; using %g", s.Zoom, DefaultZoom))
		s.Zoom = DefaultZoom
	}
	s.LayoutThemes = NormalizeThemeMap(s.LayoutThemes)

	switch {
	case s.MinimumContrast == 0:
		s.MinimumContrast = DefaultMinimumContrast
	case s.MinimumContrast < MinMinimumContrast || s.MinimumContrast > MaxMinimumContrast:
		warnings = append(warnings, fmt.Sprintf(
			"settings: minimumContrast %g is out of range; using %g", s.MinimumContrast, DefaultMinimumContrast))
		s.MinimumContrast = DefaultMinimumContrast
	}

	switch s.Privacy.ClipboardWrite {
	case "":
		s.Privacy.ClipboardWrite = ClipboardAlways
	case ClipboardAlways, ClipboardAsk, ClipboardNever:
	default:
		warnings = append(warnings, fmt.Sprintf(
			"settings: clipboardWrite %q is unknown; using %q", s.Privacy.ClipboardWrite, ClipboardAlways))
		s.Privacy.ClipboardWrite = ClipboardAlways
	}
	return warnings
}

// NormalizeThemeMap trims layout names and theme names and drops empty
// entries. A map with nothing left becomes nil, so it is not written.
func NormalizeThemeMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for layout, theme := range in {
		layout = strings.TrimSpace(layout)
		theme = strings.TrimSpace(theme)
		if layout == "" || theme == "" {
			continue
		}
		out[layout] = theme
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

type settingsJSON Settings

// UnmarshalJSON starts from the defaults, so an absent field keeps its
// default and an explicit false overrides it. Known fields are decoded and
// the rest are kept in Extra.
func (s *Settings) UnmarshalJSON(data []byte) error {
	plain := settingsJSON(DefaultSettings())
	if err := json.Unmarshal(data, &plain); err != nil {
		return err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	for _, key := range settingKeys {
		delete(raw, key)
	}
	*s = Settings(plain)
	if len(raw) > 0 {
		s.Extra = raw
	}
	return nil
}

// MarshalJSON writes the known fields plus Extra. A known field always wins
// over a stale Extra entry.
func (s Settings) MarshalJSON() ([]byte, error) {
	known, err := json.Marshal(settingsJSON(s))
	if err != nil {
		return nil, err
	}
	var out map[string]json.RawMessage
	if err := json.Unmarshal(known, &out); err != nil {
		return nil, err
	}
	for key, value := range s.Extra {
		if _, ok := out[key]; !ok {
			out[key] = value
		}
	}
	return json.Marshal(out)
}
