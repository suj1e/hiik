package core

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	mui "github.com/egoist/mygo/ui"

	"hiik/core/store"
	"hiik/mods/api"
)

// keymapState caches what the last Keymap call found in keys.json. The file
// is read on every call so a change applies without a restart (the app
// reloads the settings and reconnects too), and its size is small.
var keymapState struct {
	sync.Mutex
	problems     []string
	fileProblems []string
	sections     map[string]map[string]string
	dir          string
	stamp        string
	loaded       bool
}

// Keymap returns every built-in command with the chord it currently runs and
// the chord the app ships.
//
// A chord comes from, in order: the keys setting in settings.json, an
// override in $HIIK_HOME/keys.json, the command's own shipped binding. The
// settings value wins over keys.json. An unbound value ("", "none", "-")
// leaves the command without a chord. An invalid value is reported through
// KeymapProblems and the next source applies, so a typo never breaks the
// app.
func (a *App) Keymap() []api.KeyBinding {
	a.mu.RLock()
	cmds := append([]*cmd(nil), a.cmds...)
	settings := cloneKeys(a.settings.Keys)
	a.mu.RUnlock()

	jsonSections, problems := cachedKeysJSON(a.configDir())
	jsonOverrides := jsonSections["global"]
	if len(settings) > 0 {
		// Settings are already the person's most direct word; normalise
		// them and report each bad chord once.
		clean := make(map[string]string, len(settings))
		for command, raw := range settings {
			if isUnboundChord(raw) {
				clean[command] = ""
				continue
			}
			chord, ok := canonicalChord(raw)
			if !ok {
				problems = append(problems, "settings.keys."+command+": "+raw+" is not a key")
				clean[command] = ""
				continue
			}
			clean[command] = chord
		}
		settings = clean
	}

	out := make([]api.KeyBinding, 0, len(cmds))
	for _, c := range cmds {
		if c == nil {
			continue
		}
		binding := api.KeyBinding{Command: c.id, Title: c.title, Section: "global"}
		if len(c.keys) > 0 {
			binding.Default = bindingChord(c.keys[0])
		}
		binding.Chord = binding.Default

		if chord, has := jsonOverrides[c.id]; has {
			binding.Explicit = true
			if chord == "" {
				binding.Chord = ""
			} else if _, ok := canonicalChord(chord); ok {
				binding.Chord = chord
			}
		}
		if chord, has := settings[c.id]; has {
			binding.Explicit = true
			binding.Chord = chord
		}
		out = append(out, binding)
	}

	keymapState.Lock()
	keymapState.problems = problems
	keymapState.Unlock()
	return out
}

// KeymapProblems returns the problems from the last Keymap call: a keys.json
// file that is not JSON, or a chord that names no key. The app keeps running
// in every case.
func (a *App) KeymapProblems() []string {
	keymapState.Lock()
	defer keymapState.Unlock()
	return append([]string(nil), keymapState.problems...)
}

// KeymapSection returns the bindings keys.json lists in one section:
// "global", "terminal", "layout:<name>" or "mod:<name>". The global section
// is Keymap's table, including settings and shipped defaults; the others are
// the file's entries alone, in a stable order, because their owner applies
// them and this build does not run them yet.
func (a *App) KeymapSection(section string) []api.KeyBinding {
	section = strings.TrimSpace(section)
	if section == "" || section == "global" {
		return a.Keymap()
	}
	sections, _ := cachedKeysJSON(a.configDir())
	entries := sections[section]
	if len(entries) == 0 {
		return nil
	}

	a.mu.RLock()
	titles := make(map[string]string, len(entries))
	for action := range entries {
		if c := a.byID[commandPart(action)]; c != nil {
			titles[action] = c.title
		}
	}
	a.mu.RUnlock()

	out := make([]api.KeyBinding, 0, len(entries))
	for action, chord := range entries {
		title := titles[action]
		if title == "" {
			title = action
		}
		out = append(out, api.KeyBinding{
			Command:  action,
			Title:    title,
			Chord:    chord,
			Explicit: true,
			Section:  section,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Command != out[j].Command {
			return out[i].Command < out[j].Command
		}
		return out[i].Chord < out[j].Chord
	})
	return out
}

// KeymapSections returns the sections keys.json has, with global first. The
// settings table uses it to offer a tab per section.
func (a *App) KeymapSections() []string {
	sections, _ := cachedKeysJSON(a.configDir())
	out := []string{"global"}
	for name := range sections {
		if name != "global" {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// commandPart is the command an action names, without its first argument:
// "pane.split:right" names "pane.split".
func commandPart(action string) string {
	if i := strings.IndexByte(action, ':'); i > 0 {
		return action[:i]
	}
	return action
}

// configDir is where the person's own files (keys.json, programs.json and
// the store) live. The app's store wins; a bare app uses the environment.
func (a *App) configDir() string {
	a.mu.RLock()
	st := a.store
	a.mu.RUnlock()
	if st != nil {
		return st.Dir()
	}
	dir, err := store.Dir()
	if err != nil {
		return ""
	}
	return dir
}

// cachedKeysJSON reads keys.json when its directory, modification time or
// size changed, so a view that draws every frame does not reparse it.
func cachedKeysJSON(dir string) (map[string]map[string]string, []string) {
	stamp := keysStamp(dir)
	keymapState.Lock()
	if keymapState.loaded && keymapState.dir == dir && keymapState.stamp == stamp {
		sections := cloneSections(keymapState.sections)
		problems := append([]string(nil), keymapState.fileProblems...)
		keymapState.Unlock()
		return sections, problems
	}
	keymapState.Unlock()

	sections, problems := loadKeysJSON(dir)

	keymapState.Lock()
	keymapState.sections = cloneSections(sections)
	keymapState.fileProblems = append([]string(nil), problems...)
	keymapState.dir = dir
	keymapState.stamp = stamp
	keymapState.loaded = true
	keymapState.Unlock()
	return sections, problems
}

// cloneSections copies a section table so callers cannot mutate the cache.
func cloneSections(in map[string]map[string]string) map[string]map[string]string {
	out := make(map[string]map[string]string, len(in))
	for name, entries := range in {
		out[name] = cloneKeys(entries)
	}
	return out
}

func keysStamp(dir string) string {
	if dir == "" {
		return ""
	}
	fi, err := os.Stat(filepath.Join(dir, "keys.json"))
	if err != nil {
		return ""
	}
	return fi.ModTime().String() + ":" + strconv.FormatInt(fi.Size(), 10)
}

// loadKeysJSON reads $HIIK_HOME/keys.json. A missing file is not a problem;
// a broken one is reported and ignored.
func loadKeysJSON(dir string) (map[string]map[string]string, []string) {
	out := map[string]map[string]string{}
	if dir == "" {
		return out, nil
	}
	path := filepath.Join(dir, "keys.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return out, []string{"keys.json: " + err.Error()}
	}
	return parseKeysSections(data)
}

// parseKeysJSON accepts both shapes a keys file may use:
//
//	{"tab.new": "Mod+Shift+T"}                    // command to chord
//	{"global": {"ctrl-shift-t": "tab.new"}}       // chord to command, by section
//
// It returns the global section, the one the app-wide keymap uses.
func parseKeysJSON(data []byte) (map[string]string, []string) {
	sections, problems := parseKeysSections(data)
	out := sections["global"]
	if out == nil {
		out = map[string]string{}
	}
	return out, problems
}

// parseKeysSections reads every section of keys.json. The global section
// (and a section named after one command) applies to the app-wide keymap;
// terminal, layout:<name> and mod:<name> sections are kept for their owners
// and the Settings table.
func parseKeysSections(data []byte) (map[string]map[string]string, []string) {
	out := map[string]map[string]string{}
	section := func(name string) map[string]string {
		if out[name] == nil {
			out[name] = map[string]string{}
		}
		return out[name]
	}
	var problems []string

	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return out, []string{"keys.json: " + err.Error()}
	}
	for key, raw := range top {
		var text string
		if err := json.Unmarshal(raw, &text); err == nil {
			if text == "terminal" || strings.HasPrefix(text, "send:") {
				continue
			}
			// Top-level string: usually command to chord, but accept the
			// reference's chord to command spelling as well.
			command, chord := key, text
			if !looksLikeCommand(key) && looksLikeCommand(text) {
				command, chord = text, key
			}
			storeSectionChord(section("global"), &problems, "global", command, chord)
			continue
		}

		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			problems = append(problems, "keys.json."+key+": expected a chord or a section")
			continue
		}
		switch {
		case key == "global":
			parseKeysSection(section("global"), &problems, "global", fields)
		case looksLikeCommand(key):
			// A section named after one command, as {"tab.new":{"chord":"Mod+T"}}.
			for field, rawValue := range fields {
				if field != "chord" && field != "key" && field != "keys" {
					problems = append(problems, "keys.json."+key+": unknown field "+field)
					continue
				}
				var chord string
				if err := json.Unmarshal(rawValue, &chord); err != nil {
					problems = append(problems, "keys.json."+key+"."+field+": expected a chord")
					continue
				}
				storeSectionChord(section("global"), &problems, "global", key, chord)
			}
		default:
			// terminal, layout:<name> and mod:<name> keep their own keys.
			// Any other object is a newer file's metadata, not a section.
			if key == "terminal" || strings.HasPrefix(key, "layout:") ||
				strings.HasPrefix(key, "mod:") {
				parseKeysSection(section(key), &problems, key, fields)
			}
		}
	}
	for name, entries := range out {
		if len(entries) == 0 && name != "global" {
			delete(out, name)
		}
	}
	return out, problems
}

// parseKeysSection reads one section, which may map chord to action (the
// shipped shape) or action to chord. An action is a command, possibly with
// its first argument after a colon, "terminal" or "send:<text>".
func parseKeysSection(out map[string]string, problems *[]string, section string, entries map[string]json.RawMessage) {
	for key, raw := range entries {
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			// A command with named arguments is an object; this keymap
			// carries chords only, so leave it to the shell that runs it.
			trimmed := bytes.TrimSpace(raw)
			if len(trimmed) > 0 && trimmed[0] == '{' {
				continue
			}
			*problems = append(*problems, "keys.json."+section+"."+key+": expected a chord")
			continue
		}
		action, chord := key, text
		switch {
		case looksLikeCommand(text) || text == "terminal" || strings.HasPrefix(text, "send:"):
			// The shipped shape: chord to action.
			action, chord = text, key
		case looksLikeChord(text) && !looksLikeChord(key):
			// The other shape: action to chord.
			action, chord = key, text
		case looksLikeChord(key):
			action, chord = text, key
		case isUnboundChord(text) && looksLikeCommand(key):
			action, chord = key, text
		default:
			*problems = append(*problems,
				"keys.json."+section+"."+key+": expected an action and a chord")
			continue
		}
		storeSectionChord(out, problems, section, action, chord)
	}
}

// storeSectionChord normalises one binding and keeps it when the chord is
// valid.
func storeSectionChord(out map[string]string, problems *[]string, section, action, raw string) {
	if action == "" {
		return
	}
	if isUnboundChord(raw) {
		out[action] = ""
		return
	}
	chord, ok := canonicalChord(raw)
	if !ok {
		*problems = append(*problems, "keys.json."+section+"."+action+": "+raw+" is not a key")
		return
	}
	out[action] = chord
}

// isUnboundChord reports whether a value leaves a command without a key.
func isUnboundChord(chord string) bool {
	switch strings.ToLower(strings.TrimSpace(chord)) {
	case "", "none", "unbound", "-":
		return true
	}
	return false
}

// looksLikeCommand reports whether s names a command rather than a chord.
func looksLikeCommand(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || strings.HasPrefix(s, "send:") {
		return false
	}
	dot := strings.IndexByte(s, '.')
	if dot <= 0 || dot == len(s)-1 {
		return false
	}
	return !strings.ContainsAny(s, "+- ")
}

// looksLikeChord reports whether s parses as a chord.
func looksLikeChord(s string) bool {
	_, ok := canonicalChord(s)
	return ok
}

// bindingChord formats a shipped binding as a canonical chord.
func bindingChord(b api.Binding) string {
	var parts []string
	if b.Mods&mui.Super != 0 {
		parts = append(parts, "Cmd")
	}
	if b.Mods&mui.Ctrl != 0 {
		parts = append(parts, "Ctrl")
	}
	if b.Mods&mui.Alt != 0 {
		parts = append(parts, "Alt")
	}
	if b.Mods&mui.Shift != 0 {
		parts = append(parts, "Shift")
	}
	name, ok := keyChordNames[b.Key]
	if !ok {
		return ""
	}
	return strings.Join(append(parts, name), "+")
}

// canonicalChord parses a chord in either spelling, "Mod+Shift+K" or the
// reference's "cmd-shift-k", and returns one canonical form. It reports
// whether every part names a modifier or key.
func canonicalChord(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	// Try "+" first, then the hyphen spelling; "ctrl-+" is the plus key in
	// the hyphen spelling, so a failed first try must not be final.
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

// splitChord splits raw on sep and reads the modifiers and last key.
func splitChord(raw, sep string) ([]string, mui.Key, bool) {
	parts := strings.Split(strings.ToLower(raw), sep)
	// A trailing empty part is the separator's own key: "Mod++" and
	// "ctrl--" bind the plus and minus keys.
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

// chordModifierNames maps a modifier spelling to its display name.
var chordModifierNames = map[string]string{
	"mod": "Mod", "cmd": "Cmd", "command": "Cmd", "super": "Cmd", "meta": "Cmd",
	"ctrl": "Ctrl", "control": "Ctrl",
	"alt": "Alt", "opt": "Alt", "option": "Alt",
	"shift": "Shift",
}

// chordKeyNames maps key spellings to their keys.
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
		"+": mui.KeyEqual, // the plus key shares its code with '='; the chord keeps its name below.
	}
	for r := 'a'; r <= 'z'; r++ {
		m[string(r)] = letterChordKeys[r]
	}
	for i, key := range digitChordKeys {
		m[string(rune('0'+i))] = key
	}
	for i, key := range functionChordKeys {
		m["f"+strconv.Itoa(i+1)] = key
	}
	return m
}()

var letterChordKeys = map[rune]mui.Key{
	'a': mui.KeyA, 'b': mui.KeyB, 'c': mui.KeyC, 'd': mui.KeyD,
	'e': mui.KeyE, 'f': mui.KeyF, 'g': mui.KeyG, 'h': mui.KeyH,
	'i': mui.KeyI, 'j': mui.KeyJ, 'k': mui.KeyK, 'l': mui.KeyL,
	'm': mui.KeyM, 'n': mui.KeyN, 'o': mui.KeyO, 'p': mui.KeyP,
	'q': mui.KeyQ, 'r': mui.KeyR, 's': mui.KeyS, 't': mui.KeyT,
	'u': mui.KeyU, 'v': mui.KeyV, 'w': mui.KeyW, 'x': mui.KeyX,
	'y': mui.KeyY, 'z': mui.KeyZ,
}

var digitChordKeys = [...]mui.Key{
	mui.Key0, mui.Key1, mui.Key2, mui.Key3, mui.Key4,
	mui.Key5, mui.Key6, mui.Key7, mui.Key8, mui.Key9,
}

var functionChordKeys = [...]mui.Key{
	mui.KeyF1, mui.KeyF2, mui.KeyF3, mui.KeyF4, mui.KeyF5, mui.KeyF6,
	mui.KeyF7, mui.KeyF8, mui.KeyF9, mui.KeyF10, mui.KeyF11, mui.KeyF12,
}

// keyChordNames is the display name of each key for a chord.
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
		m[letterChordKeys[r]] = strings.ToUpper(string(r))
	}
	for i, key := range digitChordKeys {
		m[key] = string(rune('0' + i))
	}
	for i, key := range functionChordKeys {
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
