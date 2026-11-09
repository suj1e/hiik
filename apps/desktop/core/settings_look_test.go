package core

import (
	"math"
	"testing"

	"hiik/core/store"
)

func TestTerminalLookSettingsRoundTrip(t *testing.T) {
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a := newApp(st)

	for _, tc := range []struct {
		key   string
		value any
	}{
		{"ligatures", true},
		{"padding_x", 14.0},
		{"padding_y", 9.0},
		{"opacity", 0.75},
		{"cursor_shape", "bar"},
		{"cursor_blink", false},
		{"pane_scrollbars", false},
		{"zoom", 1.3},
		{"option_key", "left"},
		{"layout_themes", map[string]string{" herdr ": " ink ", "win98": "moss"}},
	} {
		if err := a.Run("app.set", map[string]any{"key": tc.key, "value": tc.value}); err != nil {
			t.Fatalf("app.set %s: %v", tc.key, err)
		}
	}
	if err := st.Flush(); err != nil {
		t.Fatal(err)
	}

	got, err := st.LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if !got.Ligatures {
		t.Error("ligatures was not saved")
	}
	if got.PaddingX != 14 || got.PaddingY != 9 {
		t.Errorf("padding = %g,%g", got.PaddingX, got.PaddingY)
	}
	if math.Abs(float64(got.Opacity)-0.75) > 0.001 {
		t.Errorf("opacity = %g", got.Opacity)
	}
	if got.CursorShape != store.CursorBar {
		t.Errorf("cursorShape = %q", got.CursorShape)
	}
	if got.CursorBlink {
		t.Error("cursorBlink was not saved as false")
	}
	if got.PaneScrollbars {
		t.Error("paneScrollbars was not saved as false")
	}
	if math.Abs(float64(got.Zoom)-1.3) > 0.001 {
		t.Errorf("zoom = %g", got.Zoom)
	}
	if got.OptionKey != store.OptionLeft {
		t.Errorf("optionKey = %q", got.OptionKey)
	}
	if got.LayoutThemes["herdr"] != "ink" || got.LayoutThemes["win98"] != "moss" {
		t.Errorf("layoutThemes = %#v", got.LayoutThemes)
	}
}

func TestSettingsNormalizeTerminalDetail(t *testing.T) {
	s := store.DefaultSettings()
	s.Layout = "  win98 "
	s.OptionKey = "sometimes"
	s.CursorShape = "triangle"
	s.Opacity = 2
	s.Zoom = 9
	s.PaddingX = -3
	s.LayoutThemes = map[string]string{"": "ink", " herdr ": " moss ", "win98": " "}

	warnings := s.Normalize()
	if len(warnings) < 5 {
		t.Fatalf("warnings = %v", warnings)
	}
	if s.Layout != "win98" {
		t.Errorf("layout = %q, want a user layout name kept", s.Layout)
	}
	if s.OptionKey != store.OptionAuto {
		t.Errorf("optionKey = %q", s.OptionKey)
	}
	if s.CursorShape != store.CursorBlock {
		t.Errorf("cursorShape = %q", s.CursorShape)
	}
	if s.Opacity != store.DefaultOpacity || s.Zoom != store.DefaultZoom {
		t.Errorf("opacity = %g, zoom = %g", s.Opacity, s.Zoom)
	}
	if s.PaddingX != store.DefaultPaddingX {
		t.Errorf("paddingX = %g", s.PaddingX)
	}
	if len(s.LayoutThemes) != 1 || s.LayoutThemes["herdr"] != "moss" {
		t.Errorf("layoutThemes = %#v", s.LayoutThemes)
	}
}

func TestZeroOpacityAndPaddingPersist(t *testing.T) {
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a := newApp(st)
	for _, tc := range []struct {
		key   string
		value any
	}{
		{"opacity", 0.0},
		{"padding_x", 0.0},
		{"padding_y", 0.0},
		{"ligatures", true},
		{"cursor_blink", false},
		{"pane_scrollbars", false},
	} {
		if err := a.Run("app.set", map[string]any{"key": tc.key, "value": tc.value}); err != nil {
			t.Fatalf("app.set %s: %v", tc.key, err)
		}
	}
	if err := st.Flush(); err != nil {
		t.Fatal(err)
	}
	got, err := st.LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if got.Opacity != 0 || got.PaddingX != 0 || got.PaddingY != 0 {
		t.Fatalf("explicit zeros did not survive: opacity=%g padding=%g,%g",
			got.Opacity, got.PaddingX, got.PaddingY)
	}
	if !got.Ligatures || got.CursorBlink || got.PaneScrollbars {
		t.Fatalf("booleans did not survive: %#v", got)
	}
}

func TestSettingsNormalizeKeepsExplicitZeroLook(t *testing.T) {
	s := store.DefaultSettings()
	s.Opacity = 0
	s.PaddingX = 0
	s.PaddingY = 0
	s.CursorShape = store.CursorUnderline
	s.CursorBlink = false
	s.PaneScrollbars = false
	s.Ligatures = true
	if warnings := s.Normalize(); len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
	if s.Opacity != 0 || s.PaddingX != 0 || s.PaddingY != 0 {
		t.Errorf("explicit zeros changed: %#v", s)
	}
}
