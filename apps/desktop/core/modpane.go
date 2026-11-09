package core

import (
	"fmt"

	"hiik/mods/api"
)

// Mod windows and floats are host-side: the shell owns their visuals, and
// the core only validates the panel and forwards the request. The commands
// exist so keys, the palette and mods can open a panel without importing
// the ui package.

func init() {
	registerCommand(&cmd{
		id:    api.CmdModWindow,
		title: "Open mod window",
		args: argsSpec{
			"panel": kindString, "mod": kindString, "props": kindAny,
		},
		run: runModWindow,
	})
	registerCommand(&cmd{
		id:    api.CmdModFloat,
		title: "Open mod float",
		args: argsSpec{
			"panel": kindString, "mod": kindString, "edge": kindString, "props": kindAny,
		},
		run: runModFloat,
	})
	registerCommand(&cmd{
		id:    "mod.open",
		title: "Open mod",
		args: argsSpec{
			"panel": kindString, "mod": kindString, "props": kindAny,
		},
		run: runModWindow,
	})
}

// runModWindow shows a registered mod panel as a window over the shell.
func runModWindow(_ *App, args map[string]any) error {
	panel, err := modPanelArg(api.CmdModWindow, args)
	if err != nil {
		return err
	}
	props, err := modPropsArg(api.CmdModWindow, args)
	if err != nil {
		return err
	}
	if err := api.OpenModWindow(panel, props); err != nil {
		return fmt.Errorf("%s: %w", api.CmdModWindow, err)
	}
	return nil
}

// runModFloat shows a registered mod panel in a floating region on an edge.
func runModFloat(_ *App, args map[string]any) error {
	panel, err := modPanelArg(api.CmdModFloat, args)
	if err != nil {
		return err
	}
	edge := argString(args, "edge")
	if !api.ValidFloatEdge(edge) {
		return fmt.Errorf("command %s: unknown float edge %q; use left, right, top or bottom",
			api.CmdModFloat, edge)
	}
	if edge == "" {
		edge = api.FloatEdgeRight
	}
	props, err := modPropsArg(api.CmdModFloat, args)
	if err != nil {
		return err
	}
	if err := api.OpenModFloat(panel, edge, props); err != nil {
		return fmt.Errorf("%s: %w", api.CmdModFloat, err)
	}
	return nil
}

// modPanelArg reads "panel" (or its alias "mod") and requires a registered
// panel, so a typo is an error rather than an empty window.
func modPanelArg(command string, args map[string]any) (string, error) {
	id := argString(args, "panel")
	if id == "" {
		id = argString(args, "mod")
	}
	if id == "" {
		return "", fmt.Errorf("command %s: needs a panel", command)
	}
	if _, ok := api.PanelByID(id); !ok {
		return "", fmt.Errorf("command %s: unknown mod panel %q", command, id)
	}
	return id, nil
}

// modPropsArg validates the optional props object. A nil map is fine; a
// non-object is a caller error.
func modPropsArg(command string, args map[string]any) (map[string]any, error) {
	raw, ok := args["props"]
	if !ok || raw == nil {
		return nil, nil
	}
	props, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("command %s: props must be an object", command)
	}
	return props, nil
}
