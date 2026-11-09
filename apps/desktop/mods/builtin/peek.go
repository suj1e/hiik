package builtin

// Peek is the compact overlay the shell hosts above a tab. It shows a
// terminal address and two actions; it deliberately does not attach or draw
// a terminal, because the shell owns that lifecycle.
//
// Props: {"terminal": "t3"}. Keep calls the "terminal.keep" command; Close
// calls a close callback from props when the shell supplies one, and
// otherwise falls back to api.CmdPaneClose. The address is passed as both
// "address" and "terminal" because the commands may name it either way.

import (
	"sync"

	"github.com/egoist/mygo/ui"

	"hiik/mods/api"
)

func init() {
	api.Register(api.Mod{
		ID:   "peek",
		Name: "Peek",
		Panels: []api.Panel{{
			ID:     api.PanelPeek,
			Title:  "Peek",
			Render: renderPeekPanel,
		}},
	})
}

// peekErrors keeps a command failure beside the address it belongs to, so a
// shell that rebuilds the panel still shows why the last action failed.
var (
	peekMu     sync.Mutex
	peekErrors = map[string]string{}
)

func renderPeekPanel(rc *api.RenderContext) {
	if rc == nil || rc.C == nil {
		return
	}
	c := rc.C
	t := c.Theme()
	address := builtinPropString(rc.Props, "terminal")
	if address == "" {
		address = builtinPropString(rc.Props, "address")
	}
	if address == "" {
		renderBuiltinNotice(c, "No terminal to peek at", "The peek panel needs a terminal address.")
		return
	}

	var title, status, space string
	if rc.App != nil {
		for _, a := range rc.App.Agents() {
			if a.Address != address {
				continue
			}
			title, status, space = a.Title, a.Status, a.Space
			if title == "" {
				title = a.App
			}
			break
		}
	}

	peekMu.Lock()
	errText := peekErrors[address]
	peekMu.Unlock()

	ui.Column(c).
		Fill().
		Padding(10).
		Gap(6).
		Background(t.Surface).
		Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Peek").FontSize(12).Bold()
				ui.Box(c).Grow(1)
				if status != "" {
					ui.Text(c, status).FontSize(10).TextColor(t.TextMuted)
				}
			})
			ui.Text(c, address).FontSize(12).Font("monospace").Selectable().SingleLine()
			if title != "" {
				ui.Text(c, title).FontSize(11).TextColor(t.TextMuted).SingleLine().Tooltip(title)
			}
			if space != "" {
				ui.Text(c, "space "+space).FontSize(10).TextColor(t.TextMuted).SingleLine()
			}
			ui.Box(c).Grow(1)
			if errText != "" {
				ui.Text(c, errText).FontSize(10).TextColor(t.Danger).SingleLine().Tooltip(errText)
			}
			ui.Row(c).Gap(8).Children(func() {
				if ui.PrimaryButton(c.Key("peek/keep"), "Keep as tab").Clicked() {
					runPeekCommand(rc, address, peekKeepCommand, true)
				}
				if ui.Button(c.Key("peek/close"), "Close").Clicked() {
					peekClose(rc, address)
				}
			})
		})
}

// peekKeepCommand turns the peek overlay into a tab. The core owns the
// command; the panel only forwards the address.
const peekKeepCommand = "terminal.keep"

// peekClose uses a shell-provided callback when there is one, because the
// shell may have to dismiss the overlay without touching a pane. A host that
// passes only a terminal address gets the pane close command instead.
func peekClose(rc *api.RenderContext, address string) {
	if fn, ok := peekCloseFunc(rc.Props, address); ok {
		peekMu.Lock()
		delete(peekErrors, address)
		peekMu.Unlock()
		fn()
		return
	}
	runPeekCommand(rc, address, api.CmdPaneClose, false)
}

// peekCloseFunc finds a close or dismiss callback in the panel props. The
// shell's exact key is not frozen, so the common names are accepted.
func peekCloseFunc(props map[string]any, address string) (func(), bool) {
	if props == nil {
		return nil, false
	}
	for _, key := range []string{"close", "onClose", "dismiss"} {
		switch fn := props[key].(type) {
		case func():
			return fn, true
		case func() error:
			return func() { _ = fn() }, true
		case func(string):
			return func() { fn(address) }, true
		case func(string) error:
			return func() { _ = fn(address) }, true
		}
	}
	return nil, false
}

func runPeekCommand(rc *api.RenderContext, address, command string, keep bool) {
	if rc == nil || rc.App == nil {
		peekMu.Lock()
		peekErrors[address] = "No app is available."
		peekMu.Unlock()
		return
	}
	args := map[string]any{
		"address":  address,
		"terminal": address,
	}
	if keep {
		args["keep"] = true
	}
	err := rc.App.Run(command, args)
	peekMu.Lock()
	if err != nil {
		peekErrors[address] = err.Error()
	} else {
		delete(peekErrors, address)
	}
	peekMu.Unlock()
}
