package api

// KeyBinding is one action's key in the keymap table.
type KeyBinding struct {
	// Command is the command ID the binding runs.
	Command string
	// Title is the command's title.
	Title string
	// Chord is the current binding, as "cmd+shift+p"; "" means unbound.
	Chord string
	// Default is the binding the app ships with, before overrides.
	Default string
	// Explicit is true when the chord came from settings or keys.json,
	// including an explicit unbinding (Chord == "").
	Explicit bool
	// Section is the keys.json section the binding belongs to: "global",
	// "terminal", "layout:<name>" or "mod:<name>". The app-wide keymap is
	// global; the others are shown in Settings and applied by their owner.
	Section string
}
