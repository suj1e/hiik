package core

import (
	"fmt"
	"math"
	"strings"

	"hiik/core/store"
)

// The appearance commands change the person's zoom and the shown layout's
// theme. They are registered here so a build without the settings panel
// still has them.
func init() {
	registerCommand(&cmd{id: "app.zoom_in", title: "Zoom in", run: runZoomIn})
	registerCommand(&cmd{id: "app.zoom_out", title: "Zoom out", run: runZoomOut})
	registerCommand(&cmd{id: "app.zoom_reset", title: "Reset zoom", run: runZoomReset})
	registerCommand(&cmd{id: "app.theme", title: "Use a theme", args: argsSpec{
		"name": kindString,
	}, run: runAppTheme})
}

func runZoomIn(a *App, _ map[string]any) error {
	return setZoom(a, nextZoom(a.Look().Zoom, 1))
}

func runZoomOut(a *App, _ map[string]any) error {
	return setZoom(a, nextZoom(a.Look().Zoom, -1))
}

func runZoomReset(a *App, _ map[string]any) error {
	return setZoom(a, store.DefaultZoom)
}

// setZoom stores and persists a zoom already inside the bounds.
func setZoom(a *App, zoom float32) error {
	if zoom < store.MinZoom || zoom > store.MaxZoom {
		return fmt.Errorf("app: zoom %g is out of range", zoom)
	}
	a.mu.Lock()
	a.settings.Zoom = zoom
	a.persistSettingsLocked()
	a.mu.Unlock()
	return nil
}

// nextZoom steps one tenth up or down from current, clamped to the app's
// limits. Zero steps resets to one. The tenths match the reference: zoom
// moves in 0.1 steps rather than by a multiplier, so 1.0, 1.1, 1.2 … is the
// sequence people see, and 0.5 and 3 are the ends.
func nextZoom(current float32, steps int) float32 {
	if steps == 0 {
		return store.DefaultZoom
	}
	if current < store.MinZoom || current > store.MaxZoom {
		current = store.DefaultZoom
	}
	tenths := math.Round(float64(current)*10) + float64(steps)
	low := math.Round(float64(store.MinZoom) * 10)
	high := math.Round(float64(store.MaxZoom) * 10)
	if tenths < low {
		tenths = low
	}
	if tenths > high {
		tenths = high
	}
	return float32(tenths / 10)
}

// runAppTheme sets the shown layout's theme. The theme name is not checked
// here: themes on disk are the UI's to enumerate, and an unknown name falls
// back to the appearance's family.
func runAppTheme(a *App, args map[string]any) error {
	name := strings.TrimSpace(argString(args, "name"))
	if name == "" {
		return fmt.Errorf("app.theme: name is required")
	}
	a.mu.Lock()
	layout := strings.TrimSpace(a.settings.Layout)
	if layout == "" {
		layout = store.LayoutHerdr
	}
	themes := cloneStringMap(a.settings.LayoutThemes)
	themes[layout] = name
	a.settings.LayoutThemes = store.NormalizeThemeMap(themes)
	a.settings.Normalize()
	a.persistSettingsLocked()
	a.mu.Unlock()
	return nil
}
