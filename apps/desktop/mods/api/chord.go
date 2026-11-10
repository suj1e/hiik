package api

import "hiik/keys"

// FormatChord renders a canonical chord as the platform shows it. Mod is
// Command on macOS, Ctrl+Shift elsewhere. An empty or unbound chord returns
// "". It is the same table the keymap parses.
func FormatChord(chord string) string { return keys.Format(chord) }
