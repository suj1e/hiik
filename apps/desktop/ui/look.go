package ui

import (
	"fmt"
	"strings"
	"sync"

	mui "github.com/egoist/mygo/ui"
)

// Theme names the app knows. A user theme on disk may have any other name;
// lookTheme then follows only the appearance.
const (
	ThemeGraphite = "graphite"
	ThemeInk      = "ink"
	ThemeMoss     = "moss"
	ThemePaper    = "paper"
	ThemeAtom     = "atom one dark"
)

// applyLook makes the frame use the look the settings describe: the
// appearance, the layout's theme, the interface font and the zoom. It must
// run before any widget is built.
func applyLook(c *mui.Context, settings map[string]any) {
	applyLookFor(c, settings, settingString(settings, "layout"))
}

// applyLookFor is applyLook for one layout. The shell's settings map carries
// the layout, so applyLook can find it; this form is for a caller that
// already has the layout name (a preview, or a test).
func applyLookFor(c *mui.Context, settings map[string]any, layout string) {
	systemDark := false
	if t := c.Theme(); t != nil {
		systemDark = t.Dark
	}
	c.SetTheme(lookThemeFor(systemDark, settings, layout))
	zoom := settingFloat(settings, "zoom", 1)
	if zoom < 0.5 {
		zoom = 0.5
	}
	if zoom > 3 {
		zoom = 3
	}
	c.SetZoom(zoom)
}

// lookTheme returns the theme for the settings. It is a package-level cached
// value: callers must not mutate it. When the appearance is system, the
// theme name picks the family; when it is light or dark, it wins over the
// name.
func lookTheme(systemDark bool, settings map[string]any) *mui.Theme {
	return lookThemeFor(systemDark, settings, settingString(settings, "layout"))
}

// lookThemeFor is lookTheme for one layout, whose entry in the layout_themes
// setting wins over the global theme.
func lookThemeFor(systemDark bool, settings map[string]any, layout string) *mui.Theme {
	key := lookKey(systemDark, settings, layout)
	lookCacheMu.Lock()
	defer lookCacheMu.Unlock()
	if t := lookCache[key]; t != nil {
		return t
	}
	t := buildLookThemeFor(systemDark, settings, layout)
	lookCache[key] = t
	return t
}

// buildLookTheme is buildLookThemeFor for the layout in the settings.
func buildLookTheme(systemDark bool, settings map[string]any) *mui.Theme {
	return buildLookThemeFor(systemDark, settings, settingString(settings, "layout"))
}

// buildLookThemeFor is lookTheme without the cache.
func buildLookThemeFor(systemDark bool, settings map[string]any, layout string) *mui.Theme {
	appearance := strings.ToLower(settingString(settings, "appearance"))
	name := layoutThemeName(settings, layout)

	dark := systemDark
	switch appearance {
	case "dark":
		dark = true
	case "light":
		dark = false
	default: // system, or unset
		dark = themeIsDark(name, systemDark)
	}

	t := mui.LightTheme()
	if dark {
		t = mui.DarkTheme()
	}
	// A theme file supplies colors only; the family still comes from the
	// appearance. A shipped theme with no file keeps its coded accent, and
	// an explicit light appearance keeps the light family.
	if def, ok := readThemeFile(name); ok {
		applyThemeFile(t, def, dark)
	} else if def, ok := builtinThemes[name]; ok {
		if dark {
			applyThemeFile(t, def, dark)
		}
	} else {
		styleTheme(t, name, dark)
	}
	if family := settingString(settings, "ui_font"); family != "" {
		t.Font = family
	}
	return t
}

// layoutThemeName is the theme a layout shows: its own layout_themes entry,
// else the global theme.
func layoutThemeName(settings map[string]any, layout string) string {
	layout = strings.TrimSpace(layout)
	if layout != "" {
		if themes := settingThemeMap(settings); themes != nil {
			if name := strings.TrimSpace(themes[layout]); name != "" {
				return strings.ToLower(name)
			}
		}
	}
	return strings.ToLower(settingString(settings, "theme"))
}

// lookKey identifies a combination of settings, the layout's theme and the
// system appearance.
func lookKey(systemDark bool, settings map[string]any, layout string) string {
	var b strings.Builder
	if systemDark {
		b.WriteByte('1')
	} else {
		b.WriteByte('0')
	}
	name := layoutThemeName(settings, layout)
	for _, key := range []string{"appearance", "ui_font"} {
		b.WriteByte(0)
		b.WriteString(settingString(settings, key))
	}
	b.WriteByte(0)
	b.WriteString(strings.TrimSpace(layout))
	b.WriteByte(0)
	b.WriteString(name)
	b.WriteByte(0)
	b.WriteString(fmt.Sprintf("%g", settingFloat(settings, "zoom", 1)))
	b.WriteByte(0)
	b.WriteString(themeFileStamp(name))
	return b.String()
}

// themeIsDark reports whether a theme name selects the dark family.
func themeIsDark(name string, systemDark bool) bool {
	switch name {
	case ThemeGraphite, ThemeInk, ThemeMoss, ThemeAtom:
		return true
	case ThemePaper:
		return false
	default:
		return systemDark
	}
}

// styleTheme applies the palette a known theme name asks for. It leaves the
// default colors of the matching family, and ignores a name whose family the
// appearance overrode.
func styleTheme(t *mui.Theme, name string, dark bool) {
	if t == nil {
		return
	}
	switch name {
	case ThemeInk:
		if !dark {
			return
		}
		// Ink is monochrome: the accent is the text color.
		t.Accent = t.Text
		t.AccentHover = t.Text.Mix(t.Background, 0.08)
		t.AccentPressed = t.Text.Mix(t.Background, 0.18)
		t.AccentText = t.Background
		t.InactiveAccent = t.Text
	case ThemeMoss:
		if !dark {
			return
		}
		// Moss accents with the green of the success color.
		t.Accent = t.Success
		t.AccentHover = t.Success.Mix(t.Text, 0.15)
		t.AccentPressed = t.Success.Mix(t.Background, 0.2)
		t.AccentText = t.Background
		t.InactiveAccent = t.Success
	}
}

// settingString reads a trimmed string from a settings map.
func settingString(settings map[string]any, key string) string {
	if settings == nil {
		return ""
	}
	s, _ := settings[key].(string)
	return strings.TrimSpace(s)
}

// settingFloat reads a number from a settings map, or returns def.
func settingFloat(settings map[string]any, key string, def float32) float32 {
	if settings == nil {
		return def
	}
	switch n := settings[key].(type) {
	case float32:
		if n > 0 {
			return n
		}
	case float64:
		if n > 0 {
			return float32(n)
		}
	case int:
		if n > 0 {
			return float32(n)
		}
	}
	return def
}

// settingNumber reads a number from a settings map, including zero, or
// returns def when the key is absent. settingFloat keeps zero as unset,
// which is right for sizes but not for padding and opacity.
func settingNumber(settings map[string]any, key string, def float32) float32 {
	if settings == nil {
		return def
	}
	switch n := settings[key].(type) {
	case float32:
		return n
	case float64:
		return float32(n)
	case int:
		return float32(n)
	}
	return def
}

// settingKeys reads the keymap overrides, tolerating either map shape a
// caller may pass.
func settingKeys(settings map[string]any) map[string]string {
	if settings == nil {
		return nil
	}
	switch m := settings["keys"].(type) {
	case map[string]string:
		return m
	case map[string]any:
		out := make(map[string]string, len(m))
		for action, item := range m {
			if chord, ok := item.(string); ok {
				out[action] = chord
			}
		}
		return out
	}
	return nil
}

// settingThemeMap reads the per-layout themes, tolerating either map shape.
func settingThemeMap(settings map[string]any) map[string]string {
	if settings == nil {
		return nil
	}
	switch m := settings["layout_themes"].(type) {
	case map[string]string:
		return m
	case map[string]any:
		out := make(map[string]string, len(m))
		for layout, item := range m {
			if theme, ok := item.(string); ok {
				out[layout] = theme
			}
		}
		return out
	}
	return nil
}

var (
	lookCacheMu sync.Mutex
	lookCache   = map[string]*mui.Theme{}
)
