package ui

import mui "github.com/egoist/mygo/ui"

// shellHook draws a part of the shell from a feature file, so features can
// add overlays or controls without editing shell.go.
type shellHook func(*Shell, *mui.Context)

var shellHooks []shellHook

// registerShellHook adds a hook that runs once per frame, after the shell's
// own content and before notifications drain.
func registerShellHook(h shellHook) {
	if h != nil {
		shellHooks = append(shellHooks, h)
	}
}

// runShellHooks runs every registered hook in order.
func runShellHooks(s *Shell, c *mui.Context) {
	for _, h := range shellHooks {
		h(s, c)
	}
}
