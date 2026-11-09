// Package api is the contract between hiik's shell and its mods.
//
// A mod is Go code compiled into the app. It may add panels, commands and
// keys; the shipped interface is built from the same kinds of mods. There is
// no scripting runtime and no ABI: every mod is compiled with the app, so the
// compiler checks it.
package api

import (
	"github.com/egoist/mygo/ui"
)

// Mod is one unit of app behavior.
type Mod struct {
	// ID is unique and stable; commands, panels and settings name it.
	ID string
	// Name is what the interface shows.
	Name string
	// Panels are views a layout or a pane can show, by their IDs.
	Panels []Panel
	// Commands are actions the palette, keys and menus can run.
	Commands []Command
}

// Panel is a view a layout names by mod and panel ID.
type Panel struct {
	// ID is unique within its mod.
	ID string
	// Title is the panel's default tab or header label.
	Title string
	// Render builds the panel for one frame.
	Render func(*RenderContext)
}

// RenderContext is what a panel gets for one frame.
type RenderContext struct {
	C     *ui.Context
	App   App
	Props map[string]any
}

// Command is an action.
type Command struct {
	// ID is unique across the app, as "space.new".
	ID string
	// Title is what the palette shows.
	Title string
	// Keys are the default bindings, which a user's keymap may override.
	Keys []Binding
	// Run performs the action. It runs off the frame's build pass.
	Run func(App, map[string]any) error
}

// Binding is one key chord.
type Binding struct {
	Mods ui.Modifiers
	Key  ui.Key
}

// App is what a mod may read and do.
type App interface {
	// Spaces returns a snapshot of the spaces.
	Spaces() []Space
	// Agents returns a snapshot of the agents.
	Agents() []Agent
	// Machines returns a snapshot of the machines the core knows.
	Machines() []Machine
	// ActiveSpace returns the selected space's ID, or "".
	ActiveSpace() string
	// Commands returns the app's command registry.
	Commands() []Command
	// Settings returns a copy of the settable toggles, keyed
	// notify_blocked, notify_done, sound, clipboard_write and telemetry.
	Settings() map[string]any
	// Connection returns "connecting", "connected" or "disconnected".
	Connection() string
	// Run runs an app command.
	Run(name string, args map[string]any) error
	// Exec runs one program on a machine and returns its stdout, stderr
	// and exit code. A nonzero exit is a result, not an error. An empty on
	// runs here; a named machine goes through the core's machine.exec.
	Exec(on, command string, args ...string) (stdout string, stderr string, code int, err error)
	// ReadTextFile reads a text file on a machine, up to 1 MiB. An empty
	// on reads this computer; a named machine uses the core's file.read.
	ReadTextFile(path, on string) (string, error)
}
