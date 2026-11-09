package ui

import (
	"hiik/core/store"
	"hiik/mods/api"
)

// LayoutNames returns the layouts the settings and layout switcher offer:
// the app's own layouts followed by the user's layouts/*.json files.
// Invalid files are skipped; the layout engine reports why.
func LayoutNames() []string {
	dir, err := store.Dir()
	if err != nil {
		return api.BuiltinLayoutNames()
	}
	names, _ := api.LayoutNamesFromDir(dir)
	return names
}

// LayoutWarnings returns why a user layout file was left out of
// LayoutNames.
func LayoutWarnings() []string {
	dir, err := store.Dir()
	if err != nil {
		return nil
	}
	_, warnings := api.LayoutNamesFromDir(dir)
	return warnings
}
