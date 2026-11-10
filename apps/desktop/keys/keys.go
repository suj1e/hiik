// Package keys parses, canonicalizes and formats the app's key chords. One
// table keeps the keymap editor, the command registry and the displayed
// shortcut in step.
package keys

import (
	"runtime"
	"strconv"
	"strings"

	mui "github.com/egoist/mygo/ui"
)

// Unbound reports a value that leaves a command without a key.
func Unbound(chord string) bool {
	switch strings.ToLower(strings.TrimSpace(chord)) {
	case "", "none", "unbound", "-":
		return true
	}
	return false
}

// ShortcutMods is the app's Mod: Command on macOS, Control+Shift elsewhere.
func ShortcutMods() mui.Modifiers {
	if runtime.GOOS == "darwin" {
		return mui.Cmd
	}
	return mui.Ctrl | mui.Shift
}

// Canonical parses a chord in either spelling, "Mod+Shift+K" or the
// reference's "cmd-shift-k", and returns one canonical form. It reports
// whether every part names a modifier or key.
func Canonical(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	for _, sep := range []string{"+", "-"} {
		if sep == "-" && !strings.Contains(raw, "-") {
			continue
		}
		mods, key, ok := splitChord(raw, sep)
		if !ok {
			continue
		}
		name, ok := keyChordNames[key]
		if !ok {
			continue
		}
		var names []string
		for _, mod := range mods {
			if n, ok := chordModifierNames[mod]; ok {
				names = append(names, n)
			}
		}
		return strings.Join(append(names, name), "+"), true
	}
	return "", false
}

// Parse reads a chord as "Mod+Shift+K": Mod is the platform's shortcut
// modifier, and the named keys include letters, digits, function keys and the
// navigation keys.
func Parse(raw string) (mui.Modifiers, mui.Key, bool) {
	chord, ok := Canonical(raw)
	if !ok {
		return 0, 0, false
	}
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
	key, ok := displayKeys[strings.ToUpper(keyName)]
	if !ok {
		return 0, 0, false
	}
	return mods, key, true
}

// Format renders a canonical chord as the platform shows it. Mod is Command
// on macOS, Ctrl+Shift elsewhere. An empty or unbound chord returns "".
func Format(chord string) string { return formatFor(chord, runtime.GOOS) }

func formatFor(chord, goos string) string {
	raw := strings.TrimSpace(chord)
	if raw == "" || Unbound(raw) {
		return ""
	}
	sep := "+"
	if !strings.Contains(raw, "+") && strings.Contains(raw, "-") {
		sep = "-"
	}
	parts := strings.Split(raw, sep)
	key := strings.TrimSpace(parts[len(parts)-1])
	if key == "" && len(parts) > 1 {
		key = strings.TrimSpace(parts[len(parts)-2])
		parts = parts[:len(parts)-2]
	}
	var ctrl, shift, alt, cmd bool
	for _, part := range parts[:max(0, len(parts)-1)] {
		switch strings.ToLower(strings.TrimSpace(part)) {
		case "mod":
			if goos == "darwin" {
				cmd = true
			} else {
				ctrl, shift = true, true
			}
		case "cmd", "command", "super", "meta":
			cmd = true
		case "ctrl", "control":
			ctrl = true
		case "alt", "opt", "option":
			alt = true
		case "shift":
			shift = true
		}
	}
	key = chordKey(key)
	if key == "" {
		return ""
	}
	if goos == "darwin" {
		var b strings.Builder
		if ctrl {
			b.WriteString("⌃")
		}
		if alt {
			b.WriteString("⌥")
		}
		if shift {
			b.WriteString("⇧")
		}
		if cmd {
			b.WriteString("⌘")
		}
		b.WriteString(key)
		return b.String()
	}
	mods := make([]string, 0, 4)
	if ctrl {
		mods = append(mods, "Ctrl")
	}
	if alt {
		mods = append(mods, "Alt")
	}
	if shift {
		mods = append(mods, "Shift")
	}
	if cmd {
		mods = append(mods, "Super")
	}
	mods = append(mods, key)
	return strings.Join(mods, "+")
}

// FromBinding formats a shipped binding as a canonical chord.
func FromBinding(mods mui.Modifiers, key mui.Key) string {
	var parts []string
	if mods&mui.Super != 0 {
		parts = append(parts, "Cmd")
	}
	if mods&mui.Ctrl != 0 {
		parts = append(parts, "Ctrl")
	}
	if mods&mui.Alt != 0 {
		parts = append(parts, "Alt")
	}
	if mods&mui.Shift != 0 {
		parts = append(parts, "Shift")
	}
	name, ok := keyChordNames[key]
	if !ok {
		return ""
	}
	return strings.Join(append(parts, name), "+")
}

func splitChord(raw, sep string) ([]string, mui.Key, bool) {
	parts := strings.Split(strings.ToLower(raw), sep)
	if sep == "+" && len(parts) >= 2 && parts[len(parts)-1] == "" {
		parts[len(parts)-1] = "+"
		if len(parts) >= 3 && parts[len(parts)-2] == "" {
			parts = append(parts[:len(parts)-2], parts[len(parts)-1])
		}
	}
	if sep == "-" && len(parts) >= 2 && parts[len(parts)-1] == "" {
		parts[len(parts)-1] = "-"
		if len(parts) >= 3 && parts[len(parts)-2] == "" {
			parts = append(parts[:len(parts)-2], parts[len(parts)-1])
		}
	}
	if len(parts) == 0 || strings.TrimSpace(parts[len(parts)-1]) == "" {
		return nil, 0, false
	}
	keyName := strings.TrimSpace(parts[len(parts)-1])
	key, ok := chordKeyNames[keyName]
	if !ok {
		return nil, 0, false
	}
	mods := make([]string, 0, len(parts)-1)
	for _, part := range parts[:len(parts)-1] {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, 0, false
		}
		if _, ok := chordModifierNames[part]; !ok {
			return nil, 0, false
		}
		mods = append(mods, part)
	}
	return mods, key, true
}

func parseModifier(name string) (mui.Modifiers, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "mod":
		return ShortcutMods(), true
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

func chordKey(key string) string {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "":
		return ""
	case "enter", "return":
		return "Enter"
	case "esc", "escape":
		return "Esc"
	case "tab":
		return "Tab"
	case "space":
		return "Space"
	case "backspace":
		return "Backspace"
	case "delete", "del":
		return "Delete"
	case "home":
		return "Home"
	case "end":
		return "End"
	case "pageup", "page-up":
		return "Page Up"
	case "pagedown", "page-down":
		return "Page Down"
	case "up":
		return "Up"
	case "down":
		return "Down"
	case "left":
		return "Left"
	case "right":
		return "Right"
	}
	return strings.ToUpper(strings.TrimSpace(key))
}

var chordModifierNames = map[string]string{
	"mod": "Mod", "cmd": "Cmd", "command": "Cmd", "super": "Cmd", "meta": "Cmd",
	"ctrl": "Ctrl", "control": "Ctrl",
	"alt": "Alt", "opt": "Alt", "option": "Alt",
	"shift": "Shift",
}

var chordKeyNames = func() map[string]mui.Key {
	m := map[string]mui.Key{
		"enter": mui.KeyEnter, "return": mui.KeyEnter,
		"esc": mui.KeyEscape, "escape": mui.KeyEscape,
		"tab": mui.KeyTab, "space": mui.KeySpace,
		"backspace": mui.KeyBackspace, "delete": mui.KeyDelete, "insert": mui.KeyInsert,
		"home": mui.KeyHome, "end": mui.KeyEnd,
		"pageup": mui.KeyPageUp, "pagedown": mui.KeyPageDown,
		"left": mui.KeyLeft, "right": mui.KeyRight, "up": mui.KeyUp, "down": mui.KeyDown,
		"comma": mui.KeyComma, ",": mui.KeyComma,
		"period": mui.KeyPeriod, ".": mui.KeyPeriod,
		"slash": mui.KeySlash, "/": mui.KeySlash,
		"backslash": mui.KeyBackslash, "\\": mui.KeyBackslash,
		"semicolon": mui.KeySemicolon, ";": mui.KeySemicolon,
		"quote": mui.KeyQuote, "'": mui.KeyQuote,
		"minus": mui.KeyMinus, "-": mui.KeyMinus,
		"equal": mui.KeyEqual, "=": mui.KeyEqual,
		"backquote": mui.KeyBackquote, "`": mui.KeyBackquote,
		"bracketleft": mui.KeyBracketLeft, "[": mui.KeyBracketLeft,
		"bracketright": mui.KeyBracketRight, "]": mui.KeyBracketRight,
		"+": mui.KeyEqual,
	}
	for r := 'a'; r <= 'z'; r++ {
		m[string(r)] = letterKeys[r]
	}
	for i, key := range digitKeys {
		m[string(rune('0'+i))] = key
	}
	for i, key := range functionKeys {
		m["f"+strconv.Itoa(i+1)] = key
	}
	return m
}()

var keyChordNames = func() map[mui.Key]string {
	m := make(map[mui.Key]string, len(chordKeyNames))
	for name, key := range chordKeyNames {
		if len(name) == 1 {
			continue
		}
		if _, exists := m[key]; !exists {
			m[key] = strings.ToUpper(name[:1]) + name[1:]
		}
	}
	for r := 'a'; r <= 'z'; r++ {
		m[letterKeys[r]] = strings.ToUpper(string(r))
	}
	for i, key := range digitKeys {
		m[key] = string(rune('0' + i))
	}
	for i, key := range functionKeys {
		m[key] = "F" + strconv.Itoa(i+1)
	}
	m[mui.KeyEqual] = "="
	m[mui.KeyMinus] = "-"
	m[mui.KeyComma] = ","
	m[mui.KeyPeriod] = "."
	m[mui.KeySlash] = "/"
	m[mui.KeyBackslash] = "\\"
	m[mui.KeySemicolon] = ";"
	m[mui.KeyQuote] = "'"
	m[mui.KeyBackquote] = "`"
	m[mui.KeyBracketLeft] = "["
	m[mui.KeyBracketRight] = "]"
	return m
}()

var displayKeys = func() map[string]mui.Key {
	m := make(map[string]mui.Key, len(keyChordNames))
	for key, name := range keyChordNames {
		m[strings.ToUpper(name)] = key
	}
	return m
}()

var letterKeys = map[rune]mui.Key{
	'a': mui.KeyA, 'b': mui.KeyB, 'c': mui.KeyC, 'd': mui.KeyD,
	'e': mui.KeyE, 'f': mui.KeyF, 'g': mui.KeyG, 'h': mui.KeyH,
	'i': mui.KeyI, 'j': mui.KeyJ, 'k': mui.KeyK, 'l': mui.KeyL,
	'm': mui.KeyM, 'n': mui.KeyN, 'o': mui.KeyO, 'p': mui.KeyP,
	'q': mui.KeyQ, 'r': mui.KeyR, 's': mui.KeyS, 't': mui.KeyT,
	'u': mui.KeyU, 'v': mui.KeyV, 'w': mui.KeyW, 'x': mui.KeyX,
	'y': mui.KeyY, 'z': mui.KeyZ,
}

var digitKeys = [...]mui.Key{
	mui.Key0, mui.Key1, mui.Key2, mui.Key3, mui.Key4,
	mui.Key5, mui.Key6, mui.Key7, mui.Key8, mui.Key9,
}

var functionKeys = [...]mui.Key{
	mui.KeyF1, mui.KeyF2, mui.KeyF3, mui.KeyF4, mui.KeyF5, mui.KeyF6,
	mui.KeyF7, mui.KeyF8, mui.KeyF9, mui.KeyF10, mui.KeyF11, mui.KeyF12,
}
