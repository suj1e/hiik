package ui

import (
	"fmt"
	"strings"

	mui "github.com/egoist/mygo/ui"

	"hiik/mods/api"
)

// shortcut is one default key binding: the action ID the keymap overrides,
// the default chord, and what it does.
type shortcut struct {
	action string
	chord  string
	run    func(*Shell)
}

// defaultShortcuts returns the shipped key table. An override in the keys
// setting replaces the chord of one action; the action's other chord, if any,
// is not bound.
func defaultShortcuts(s *Shell) []shortcut {
	out := []shortcut{
		{api.CmdPalette, "Mod+P", func(sh *Shell) { sh.openPalette() }},
		{api.CmdTabNew, "Mod+T", func(sh *Shell) {
			sh.run(api.CmdTabNew, nil)
		}},
		{api.CmdTabClose, "Mod+W", func(sh *Shell) { sh.confirmCloseTab() }},
		{api.CmdPaneSplit, "Mod+D", func(sh *Shell) {
			sh.run(api.CmdPaneSplit, map[string]any{"dir": "right"})
		}},
		{api.CmdPaneSplit + ".down", "Mod+Shift+D", func(sh *Shell) {
			sh.run(api.CmdPaneSplit, map[string]any{"dir": "down"})
		}},
		{api.CmdTabRename, "Mod+Shift+R", func(sh *Shell) { sh.renameTab() }},
		{api.CmdPaneZoom, "Mod+Shift+Z", func(sh *Shell) { sh.toggleZoom() }},
		{api.CmdSettings, "Mod+Alt+Enter", func(sh *Shell) { sh.openSettings() }},
		{api.CmdSettings, "Mod+Comma", func(sh *Shell) { sh.openSettings() }},
		{api.CmdPaneFocus + ".left", "Mod+Alt+Left", func(sh *Shell) { sh.moveFocus("left") }},
		{api.CmdPaneFocus + ".right", "Mod+Alt+Right", func(sh *Shell) { sh.moveFocus("right") }},
		{api.CmdPaneFocus + ".up", "Mod+Alt+Up", func(sh *Shell) { sh.moveFocus("up") }},
		{api.CmdPaneFocus + ".down", "Mod+Alt+Down", func(sh *Shell) { sh.moveFocus("down") }},
	}
	for i := 1; i <= 9; i++ {
		i := i
		out = append(out, shortcut{
			action: fmt.Sprintf("space.select.%d", i),
			chord:  fmt.Sprintf("Mod+%d", i),
			run:    func(sh *Shell) { sh.selectSpaceAt(i - 1) },
		})
	}
	return out
}

// keymapSectioner is the part of *core.App the Settings table reads to show
// every keys.json section. A fake in tests need not implement it.
type keymapSectioner interface {
	KeymapSection(section string) []api.KeyBinding
	KeymapSections() []string
}

// keymapSection returns one keys.json section, or nil when the app does not
// support sections.
func (s *Shell) keymapSection(section string) []api.KeyBinding {
	if app, ok := s.app.(keymapSectioner); ok {
		return app.KeymapSection(section)
	}
	return nil
}

// keymapSectionNames lists the table's sections: global and terminal, the
// shown layout's, then every other layout and mod section keys.json has.
func (s *Shell) keymapSectionNames(settings map[string]any) []string {
	seen := map[string]bool{}
	var out []string
	add := func(name string) {
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		out = append(out, name)
	}
	layout := settingString(settings, "layout")
	if layout == "" {
		layout = "herdr"
	}
	add("global")
	add("terminal")
	add("layout:" + layout)
	if app, ok := s.app.(keymapSectioner); ok {
		for _, name := range app.KeymapSections() {
			if strings.HasPrefix(name, "layout:") || strings.HasPrefix(name, "mod:") {
				add(name)
			}
		}
	}
	return out
}

// keymapSectionTitle is a section's label in the table's tabs.
func keymapSectionTitle(section string) string {
	switch {
	case section == "global":
		return "Global"
	case section == "terminal":
		return "Terminal"
	case strings.HasPrefix(section, "layout:"):
		return "Layout: " + strings.TrimPrefix(section, "layout:")
	case strings.HasPrefix(section, "mod:"):
		return "Mod: " + strings.TrimPrefix(section, "mod:")
	}
	return section
}

// keymapEdit applies one chord to a copy of the keys setting. An empty,
// "none", "unbound" or "-" value unbinds the command. It reports whether the
// chord is valid; an invalid chord leaves the map untouched.
func keymapEdit(keys map[string]string, command, raw string) (map[string]string, bool) {
	out := make(map[string]string, len(keys)+1)
	for name, chord := range keys {
		out[name] = chord
	}
	raw = strings.TrimSpace(raw)
	if isUnbound(raw) {
		out[command] = ""
		return out, true
	}
	if _, _, ok := parseChord(raw); !ok {
		return nil, false
	}
	out[command] = raw
	return out, true
}

// isUnbound reports whether a keymap value removes an action's binding.
func isUnbound(chord string) bool {
	switch strings.ToLower(strings.TrimSpace(chord)) {
	case "", "none", "unbound", "-":
		return true
	}
	return false
}

// resolveShortcut picks the chord for one action: its override when the
// keys setting has one, else its default. ok is false for an unbound or
// invalid chord; overridden reports whether the keys setting supplied it.
func resolveShortcut(sc shortcut, overrides map[string]string) (mods mui.Modifiers, key mui.Key, ok, overridden bool) {
	chord := sc.chord
	if raw, has := overrides[sc.action]; has {
		overridden = true
		if isUnbound(raw) {
			return 0, 0, false, true
		}
		chord = raw
	}
	mods, key, ok = parseChord(chord)
	if !ok {
		return 0, 0, false, overridden
	}
	return mods, key, true, overridden
}

// parseChord reads a chord as "Mod+Shift+K": Mod is the platform's shortcut
// modifier, and the named keys include letters, digits, function keys and the
// navigation keys.
func parseChord(chord string) (mui.Modifiers, mui.Key, bool) {
	parts := strings.Split(chord, "+")
	keyName := strings.TrimSpace(parts[len(parts)-1])
	var mods mui.Modifiers
	for _, part := range parts[:len(parts)-1] {
		mod, ok := parseModifier(part)
		if !ok {
			return 0, 0, false
		}
		mods |= mod
	}
	key, ok := parseKey(keyName)
	if !ok {
		return 0, 0, false
	}
	return mods, key, true
}

// parseModifier reads one modifier name.
func parseModifier(name string) (mui.Modifiers, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "mod":
		return shortcutMods(), true
	case "cmd", "command", "super", "meta":
		return mui.Super, true
	case "ctrl", "control":
		return mui.Ctrl, true
	case "alt", "opt", "option":
		return mui.Alt, true
	case "shift":
		return mui.Shift, true
	}
	return 0, false
}

// parseKey reads one key name, by letter, digit, function key or name.
func parseKey(name string) (mui.Key, bool) {
	upper := strings.ToUpper(strings.TrimSpace(name))
	if k, ok := namedKeys[upper]; ok {
		return k, true
	}
	return 0, false
}

var namedKeys = func() map[string]mui.Key {
	m := map[string]mui.Key{
		"ENTER":     mui.KeyEnter,
		"RETURN":    mui.KeyEnter,
		"ESC":       mui.KeyEscape,
		"ESCAPE":    mui.KeyEscape,
		"TAB":       mui.KeyTab,
		"SPACE":     mui.KeySpace,
		"BACKSPACE": mui.KeyBackspace,
		"DELETE":    mui.KeyDelete,
		"INSERT":    mui.KeyInsert,
		"HOME":      mui.KeyHome,
		"END":       mui.KeyEnd,
		"PAGEUP":    mui.KeyPageUp,
		"PAGEDOWN":  mui.KeyPageDown,
		"LEFT":      mui.KeyLeft,
		"RIGHT":     mui.KeyRight,
		"UP":        mui.KeyUp,
		"DOWN":      mui.KeyDown,
		"COMMA":     mui.KeyComma,
		"PERIOD":    mui.KeyPeriod,
		"SLASH":     mui.KeySlash,
		"BACKSLASH": mui.KeyBackslash,
		"SEMICOLON": mui.KeySemicolon,
		"QUOTE":     mui.KeyQuote,
		"MINUS":     mui.KeyMinus,
		"EQUAL":     mui.KeyEqual,
		"BACKQUOTE": mui.KeyBackquote,
		"[":         mui.KeyBracketLeft,
		"]":         mui.KeyBracketRight,
		",":         mui.KeyComma,
		".":         mui.KeyPeriod,
		"/":         mui.KeySlash,
		"\\":        mui.KeyBackslash,
		";":         mui.KeySemicolon,
		"'":         mui.KeyQuote,
		"-":         mui.KeyMinus,
		"=":         mui.KeyEqual,
		"`":         mui.KeyBackquote,
	}
	for r := 'A'; r <= 'Z'; r++ {
		if k, ok := letterKeys[r]; ok {
			m[string(r)] = k
		}
	}
	for i, k := range digitKeys {
		m[fmt.Sprintf("%d", i)] = k
	}
	for i, k := range functionKeys {
		m[fmt.Sprintf("F%d", i+1)] = k
	}
	return m
}()

var letterKeys = map[rune]mui.Key{
	'A': mui.KeyA, 'B': mui.KeyB, 'C': mui.KeyC, 'D': mui.KeyD,
	'E': mui.KeyE, 'F': mui.KeyF, 'G': mui.KeyG, 'H': mui.KeyH,
	'I': mui.KeyI, 'J': mui.KeyJ, 'K': mui.KeyK, 'L': mui.KeyL,
	'M': mui.KeyM, 'N': mui.KeyN, 'O': mui.KeyO, 'P': mui.KeyP,
	'Q': mui.KeyQ, 'R': mui.KeyR, 'S': mui.KeyS, 'T': mui.KeyT,
	'U': mui.KeyU, 'V': mui.KeyV, 'W': mui.KeyW, 'X': mui.KeyX,
	'Y': mui.KeyY, 'Z': mui.KeyZ,
}

var digitKeys = [...]mui.Key{
	mui.Key0, mui.Key1, mui.Key2, mui.Key3, mui.Key4,
	mui.Key5, mui.Key6, mui.Key7, mui.Key8, mui.Key9,
}

var functionKeys = [...]mui.Key{
	mui.KeyF1, mui.KeyF2, mui.KeyF3, mui.KeyF4, mui.KeyF5, mui.KeyF6,
	mui.KeyF7, mui.KeyF8, mui.KeyF9, mui.KeyF10, mui.KeyF11, mui.KeyF12,
}
