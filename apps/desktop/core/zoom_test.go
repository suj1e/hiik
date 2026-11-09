package core

import (
	"math"
	"testing"

	"hiik/core/store"
)

func TestZoomCommandsStepAndReset(t *testing.T) {
	a := newApp(nil)

	if err := a.Run("app.zoom_in", nil); err != nil {
		t.Fatal(err)
	}
	if got := a.Look().Zoom; math.Abs(float64(got)-1.1) > 0.001 {
		t.Fatalf("zoom after in = %g, want 1.1", got)
	}
	if err := a.Run("app.zoom_reset", nil); err != nil {
		t.Fatal(err)
	}
	if got := a.Look().Zoom; got != store.DefaultZoom {
		t.Fatalf("zoom after reset = %g, want %g", got, store.DefaultZoom)
	}

	for range 40 {
		if err := a.Run("app.zoom_in", nil); err != nil {
			t.Fatal(err)
		}
	}
	if got := a.Look().Zoom; got != store.MaxZoom {
		t.Fatalf("zoom at the top = %g, want %g", got, store.MaxZoom)
	}
	for range 40 {
		if err := a.Run("app.zoom_out", nil); err != nil {
			t.Fatal(err)
		}
	}
	if got := a.Look().Zoom; got != store.MinZoom {
		t.Fatalf("zoom at the bottom = %g, want %g", got, store.MinZoom)
	}
}

func TestZoomSettingBounds(t *testing.T) {
	a := newApp(nil)
	for _, value := range []float64{0.4, 0, 3.1} {
		if err := a.Run("app.set", map[string]any{"key": "zoom", "value": value}); err == nil {
			t.Errorf("zoom %g should be out of range", value)
		}
	}
	for _, value := range []float64{0.5, 1, 3} {
		if err := a.Run("app.set", map[string]any{"key": "zoom", "value": value}); err != nil {
			t.Errorf("zoom %g: %v", value, err)
		}
		if got := a.Look().Zoom; math.Abs(float64(got)-value) > 0.001 {
			t.Errorf("zoom = %g, want %g", got, value)
		}
	}
}

func TestOptionAsAlt(t *testing.T) {
	for _, tc := range []struct {
		option string
		want   bool
	}{
		{"auto", false},
		{"alt", true},
		{"native", false},
		{"left", true},
		{"right", true},
		{" Alt ", true},
		{"sometimes", false},
		{"", false},
	} {
		if got := OptionAsAlt(tc.option); got != tc.want {
			t.Errorf("OptionAsAlt(%q) = %v, want %v", tc.option, got, tc.want)
		}
	}

	a := newApp(nil)
	if err := a.Run("app.set", map[string]any{"key": "option_key", "value": "left"}); err != nil {
		t.Fatal(err)
	}
	if got := a.Settings()["option_as_alt"]; got != true {
		t.Fatalf("option_as_alt = %#v, want true", got)
	}
	if got := a.Settings()["option_key"]; got != "left" {
		t.Fatalf("option_key = %#v", got)
	}
}

func TestAppThemeWritesTheLayoutTheme(t *testing.T) {
	a := newApp(nil)
	if err := a.Run("app.set", map[string]any{"key": "layout", "value": "win98"}); err != nil {
		t.Fatal(err)
	}
	if err := a.Run("app.theme", map[string]any{"name": "moss"}); err != nil {
		t.Fatal(err)
	}
	if got := a.Look().LayoutThemes["win98"]; got != "moss" {
		t.Fatalf("layout theme = %q, want moss", got)
	}
	if err := a.Run("app.theme", map[string]any{"name": " "}); err == nil {
		t.Fatal("a blank theme name should fail")
	}
}
