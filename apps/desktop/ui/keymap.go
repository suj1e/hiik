package ui

import (
	"fmt"
	"strings"

	mui "github.com/egoist/mygo/ui"

	"hiik/keys"
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
func keymapEdit(bindings map[string]string, command, raw string) (map[string]string, bool) {
	out := make(map[string]string, len(bindings)+1)
	for name, chord := range bindings {
		out[name] = chord
	}
	raw = strings.TrimSpace(raw)
	if keys.Unbound(raw) {
		out[command] = ""
		return out, true
	}
	if _, _, ok := keys.Parse(raw); !ok {
		return nil, false
	}
	out[command] = raw
	return out, true
}

// resolveShortcut picks the chord for one action: its override when the
// keys setting has one, else its default. ok is false for an unbound or
// invalid chord; overridden reports whether the keys setting supplied it.
func resolveShortcut(sc shortcut, overrides map[string]string) (mods mui.Modifiers, key mui.Key, ok, overridden bool) {
	chord := sc.chord
	if raw, has := overrides[sc.action]; has {
		overridden = true
		if keys.Unbound(raw) {
			return 0, 0, false, true
		}
		chord = raw
	}
	mods, key, ok = keys.Parse(chord)
	if !ok {
		return 0, 0, false, overridden
	}
	return mods, key, true, overridden
}
