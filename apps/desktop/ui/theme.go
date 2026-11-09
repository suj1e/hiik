package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	mui "github.com/egoist/mygo/ui"

	"hiik/core/store"
)

// themeFile is one theme on disk: $HIIK_HOME/themes/<name>.json. The fields
// are data, not code: a theme only supplies colors, and a missing color
// keeps the one of the light or dark family the appearance chose.
type themeFile struct {
	Background string            `json:"background"`
	Foreground string            `json:"foreground"`
	Selection  string            `json:"selection"`
	Cursor     string            `json:"cursor"`
	ANSI       []string          `json:"ansi"`
	Colors     map[string]string `json:"colors"`
}

// builtinThemes are the palettes shipped as code. A user theme of the same
// name wins over one of these.
var builtinThemes = map[string]themeFile{
	ThemeAtom: {
		Background: "#21252b",
		Foreground: "#abb2bf",
		Selection:  "#323844",
		Cursor:     "#abb2bf",
		ANSI: []string{
			"#454c59", "#e06c75", "#98c379", "#e5c07b",
			"#61afef", "#c678dd", "#56b6c2", "#abb2bf",
			"#767676", "#e06c75", "#98c379", "#e5c07b",
			"#61afef", "#c678dd", "#56b6c2", "#abb2bf",
		},
		Colors: map[string]string{
			"surface":     "#282c34",
			"raised":      "#2c313a",
			"hover":       "#2c313a",
			"border":      "#3e4451",
			"accent":      "#61afef",
			"muted":       "#7f848e",
			"dim":         "#5c6370",
			"success":     "#98c379",
			"warning":     "#e5c07b",
			"danger":      "#e06c75",
			"selection":   "#323844",
			"cursor":      "#abb2bf",
			"scrollbar":   "#4b5263",
			"inverse":     "#abb2bf",
			"inverseText": "#21252b",
		},
	},
}

// ThemeNames lists the shipped themes and the user's themes on disk, in the
// order the settings picker shows them. A user theme with a shipped name is
// listed once, as the user's.
func ThemeNames() []string {
	names := []string{ThemeGraphite, ThemeInk, ThemeMoss, ThemeAtom, ThemePaper}
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		seen[name] = true
	}
	for _, name := range userThemeNames() {
		if !seen[name] {
			names = append(names, name)
			seen[name] = true
		}
	}
	return names
}

// userThemeNames returns the names of the theme files in the app's themes
// directory, sorted.
func userThemeNames() []string {
	dir, ok := themesDir()
	if !ok {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		name = strings.TrimSuffix(name, ".json")
		if validThemeName(name) {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// themesDir returns the theme directory under HIIK_HOME, or the app's config
// directory when that is unset.
func themesDir() (string, bool) {
	dir, err := store.Dir()
	if err != nil || dir == "" {
		return "", false
	}
	return filepath.Join(dir, "themes"), true
}

// validThemeName reports whether name can be one file under the theme
// directory.
func validThemeName(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	return !strings.ContainsAny(name, `/\`)
}

// themeFileCache remembers one theme file and when it was read, so a frame
// stats it but reloads only a file that changed.
type themeFileCache struct {
	stamp string
	def   themeFile
	ok    bool
}

var (
	themeFilesMu sync.Mutex
	themeFiles   = map[string]themeFileCache{}
)

// readThemeFile reads themes/<name>.json, or returns false when it is
// absent or invalid.
func readThemeFile(name string) (themeFile, bool) {
	if !validThemeName(name) {
		return themeFile{}, false
	}
	dir, ok := themesDir()
	if !ok {
		return themeFile{}, false
	}
	path := filepath.Join(dir, name+".json")
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		themeFilesMu.Lock()
		themeFiles[name] = themeFileCache{stamp: "missing"}
		themeFilesMu.Unlock()
		return themeFile{}, false
	}
	stamp := path + "|" + info.ModTime().UTC().Format(time.RFC3339Nano) + ":" +
		time.Duration(info.Size()).String()
	themeFilesMu.Lock()
	if cached, ok := themeFiles[name]; ok && cached.stamp == stamp {
		themeFilesMu.Unlock()
		return cached.def, cached.ok
	}
	themeFilesMu.Unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		return themeFile{}, false
	}
	var def themeFile
	if err := json.Unmarshal(data, &def); err != nil {
		themeFilesMu.Lock()
		themeFiles[name] = themeFileCache{stamp: stamp}
		themeFilesMu.Unlock()
		return themeFile{}, false
	}
	parsed := normalizeThemeFile(def)
	themeFilesMu.Lock()
	themeFiles[name] = themeFileCache{stamp: stamp, def: parsed, ok: true}
	themeFilesMu.Unlock()
	return parsed, true
}

// normalizeThemeFile drops colors a theme file cannot use, so a malformed
// value never reaches the theme.
func normalizeThemeFile(def themeFile) themeFile {
	out := themeFile{}
	if _, ok := parseHexColor(def.Background); ok {
		out.Background = def.Background
	}
	if _, ok := parseHexColor(def.Foreground); ok {
		out.Foreground = def.Foreground
	}
	if _, ok := parseHexColor(def.Selection); ok {
		out.Selection = def.Selection
	}
	if _, ok := parseHexColor(def.Cursor); ok {
		out.Cursor = def.Cursor
	}
	if len(def.ANSI) == 16 {
		ansi := make([]string, 16)
		valid := true
		for i, value := range def.ANSI {
			if _, ok := parseHexColor(value); !ok {
				valid = false
				break
			}
			ansi[i] = value
		}
		if valid {
			out.ANSI = ansi
		}
	}
	if len(def.Colors) > 0 {
		colors := make(map[string]string, len(def.Colors))
		for key, value := range def.Colors {
			if _, ok := parseHexColor(value); ok {
				colors[key] = value
			}
		}
		if len(colors) > 0 {
			out.Colors = colors
		}
	}
	return out
}

// themeFileStamp changes when the selected theme file changes, so the
// lookTheme cache reloads a theme the user edited.
func themeFileStamp(name string) string {
	if !validThemeName(name) {
		return ""
	}
	dir, ok := themesDir()
	if !ok {
		return ""
	}
	path := filepath.Join(dir, name+".json")
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return ""
	}
	return path + "|" + info.ModTime().UTC().Format(time.RFC3339Nano) + ":" +
		time.Duration(info.Size()).String()
}

// applyThemeFile applies a theme's colors to t. Missing roles are derived
// from the background and foreground so a file with just those two still
// gives a usable theme.
func applyThemeFile(t *mui.Theme, def themeFile, dark bool) {
	if t == nil {
		return
	}
	t.Dark = dark
	setColor(&t.Background, def.Background)
	setColor(&t.Text, def.Foreground)
	setColor(&t.Selection, def.Selection)

	colors := def.Colors
	get := func(keys ...string) (mui.Color, bool) {
		for _, key := range keys {
			if value, ok := colors[key]; ok {
				if c, ok := parseHexColor(value); ok {
					return c, true
				}
			}
		}
		for _, key := range keys {
			for stored, value := range colors {
				if themeRoleKey(stored) != themeRoleKey(key) {
					continue
				}
				if c, ok := parseHexColor(value); ok {
					return c, true
				}
			}
		}
		return mui.Color{}, false
	}

	background, foreground := t.Background, t.Text
	if def.Background == "" {
		if c, ok := get("background", "bg"); ok {
			t.Background = c
			background = c
		}
	}
	if def.Foreground == "" {
		if c, ok := get("foreground", "text"); ok {
			t.Text = c
			foreground = c
		}
	}
	if def.Selection == "" {
		if c, ok := get("selection"); ok {
			t.Selection = c
		}
	}
	if c, ok := get("surface"); ok {
		t.Surface = c
	} else {
		t.Surface = background.Mix(foreground, 0.05)
	}
	if c, ok := get("raised", "surfaceHover", "hover"); ok {
		t.SurfaceHover = c
	} else {
		t.SurfaceHover = background.Mix(foreground, 0.09)
	}
	if c, ok := get("surfacePressed"); ok {
		t.SurfacePressed = c
	} else {
		t.SurfacePressed = background.Mix(foreground, 0.14)
	}
	if c, ok := get("border"); ok {
		t.Border = c
	} else {
		t.Border = background.Mix(foreground, 0.14)
	}
	if c, ok := get("muted", "dim", "textMuted"); ok {
		t.TextMuted = c
	} else {
		t.TextMuted = foreground.Mix(background, 0.45)
	}
	if c, ok := get("accent"); ok {
		t.Accent = c
	} else if ansi, ok := themeANSI(def); ok {
		t.Accent = ansi[4]
	} else {
		t.Accent = foreground
	}
	if c, ok := get("accentHover"); ok {
		t.AccentHover = c
	} else {
		t.AccentHover = t.Accent.Mix(foreground, 0.15)
	}
	if c, ok := get("accentPressed"); ok {
		t.AccentPressed = c
	} else {
		t.AccentPressed = t.Accent.Mix(background, 0.2)
	}
	if c, ok := get("accentText"); ok {
		t.AccentText = c
	} else {
		t.AccentText = background
	}
	t.InactiveAccent = t.Accent
	if c, ok := get("danger", "error"); ok {
		t.Danger = c
	} else if ansi, ok := themeANSI(def); ok {
		t.Danger = ansi[1]
	}
	if c, ok := get("warning"); ok {
		t.Warning = c
	} else if ansi, ok := themeANSI(def); ok {
		t.Warning = ansi[3]
	}
	if c, ok := get("success"); ok {
		t.Success = c
	} else if ansi, ok := themeANSI(def); ok {
		t.Success = ansi[2]
	}
	t.Focus = t.Accent
	if c, ok := get("scrollbar"); ok {
		t.Scrollbar = c
	} else {
		t.Scrollbar = t.Border
	}
	if c, ok := get("inverse"); ok {
		t.Inverse = c
	} else {
		t.Inverse = foreground
	}
	if c, ok := get("inverseText"); ok {
		t.InverseText = c
	} else {
		t.InverseText = background
	}
	storeThemeANSI(t, def)
}

// themeRoleKey folds a color role's spelling so "surfaceHover",
// "surface_hover" and "surfacehover" name the same role.
func themeRoleKey(key string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	return strings.NewReplacer("-", "", "_", "", " ", "").Replace(key)
}

// themeANSI returns the theme file's 16-color palette, if it has one.
func themeANSI(def themeFile) ([16]mui.Color, bool) {
	if len(def.ANSI) != 16 {
		return [16]mui.Color{}, false
	}
	var out [16]mui.Color
	for i, value := range def.ANSI {
		c, ok := parseHexColor(value)
		if !ok {
			return [16]mui.Color{}, false
		}
		out[i] = c
	}
	return out, true
}

// themePalettes remembers the ANSI palette of each theme it builds, so the
// terminal look can use the file's own colors.
var (
	themePalettesMu sync.Mutex
	themePalettes   = map[*mui.Theme][16]mui.Color{}
)

// storeThemeANSI records the file's palette for a built theme.
func storeThemeANSI(t *mui.Theme, def themeFile) {
	ansi, ok := themeANSI(def)
	if !ok || t == nil {
		return
	}
	themePalettesMu.Lock()
	themePalettes[t] = ansi
	themePalettesMu.Unlock()
}

// ThemePalette returns the ANSI palette a theme file supplied, if any. It
// is the terminal's palette when the theme has one.
func ThemePalette(t *mui.Theme) ([16]mui.Color, bool) {
	if t == nil {
		return [16]mui.Color{}, false
	}
	themePalettesMu.Lock()
	defer themePalettesMu.Unlock()
	p, ok := themePalettes[t]
	return p, ok
}

// parseHexColor parses a CSS hex color, reporting false instead of
// panicking on an invalid value from a theme file.
func parseHexColor(s string) (mui.Color, bool) {
	h := strings.TrimPrefix(strings.TrimSpace(s), "#")
	var v [8]uint8
	for i := range len(h) {
		d, ok := hexValue(h[i])
		if !ok {
			return mui.Color{}, false
		}
		if i < len(v) {
			v[i] = d
		}
	}
	switch len(h) {
	case 3, 4:
		a := uint8(15)
		if len(h) == 4 {
			a = v[3]
		}
		return mui.RGBA(v[0]*17, v[1]*17, v[2]*17, float32(a)/15), true
	case 6, 8:
		a := uint8(255)
		if len(h) == 8 {
			a = v[6]<<4 | v[7]
		}
		return mui.RGBA(v[0]<<4|v[1], v[2]<<4|v[3], v[4]<<4|v[5], float32(a)/255), true
	}
	return mui.Color{}, false
}

// hexValue returns the value of a hexadecimal digit.
func hexValue(c byte) (uint8, bool) {
	switch {
	case '0' <= c && c <= '9':
		return c - '0', true
	case 'a' <= c && c <= 'f':
		return c - 'a' + 10, true
	case 'A' <= c && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}

// setColor replaces dst when value is a usable color.
func setColor(dst *mui.Color, value string) {
	if c, ok := parseHexColor(value); ok {
		*dst = c
	}
}
