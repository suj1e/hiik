package ui

import (
	"fmt"
	"sync"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	mui "github.com/egoist/mygo/ui"
)

var systemHooks sync.Map // *Shell -> struct{}

func init() {
	registerShellHook(installSystemHooks)
}

// installSystemHooks gives the app the shell's window, app, copy and paste
// actions once per window.
func installSystemHooks(s *Shell, _ *mui.Context) {
	if s == nil || s.app == nil || s.win == nil {
		return
	}
	if _, loaded := systemHooks.LoadOrStore(s, struct{}{}); loaded {
		return
	}

	if app, ok := s.app.(interface{ SetWindowAction(func(string)) }); ok {
		app.SetWindowAction(func(action string) {
			switch action {
			case "close":
				s.win.Close()
			case "minimize":
				s.win.Minimize()
			case "maximize":
				s.win.Maximize()
			}
		})
	}
	if app, ok := s.app.(interface{ SetAppAction(func(string)) }); ok {
		app.SetAppAction(func(action string) {
			switch action {
			case "quit":
				mygo.App.Quit()
			case "hide":
				mygo.App.Hide()
			}
		})
	}
	if app, ok := s.app.(interface{ SetTerminalPaster(func(string) error) }); ok {
		app.SetTerminalPaster(func(address string) error {
			ts := s.attachment(address)
			if ts.term == nil {
				return fmt.Errorf("terminal %s is not attached", address)
			}
			data, err := mygo.Clipboard.Read(transfer.Text)
			if err != nil {
				return err
			}
			text, err := data.Read(transfer.Text)
			if err != nil {
				return err
			}
			ts.term.Paste(string(text))
			return nil
		})
	}
	if app, ok := s.app.(interface{ SetTerminalCopier(func(string) error) }); ok {
		app.SetTerminalCopier(func(string) error {
			return fmt.Errorf("copy is handled by the terminal view and its key binding")
		})
	}
}
