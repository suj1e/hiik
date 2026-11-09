package core

import "testing"

func TestLayoutAndOptionKeySettings(t *testing.T) {
	a := testApp()
	for _, tc := range []struct {
		key, value, want string
	}{
		{"layout", "herdr", "herdr"},
		{"layout", "terminals", "terminals"},
		{"option_key", "auto", "auto"},
		{"option_key", "alt", "alt"},
		{"option_key", "native", "native"},
		{"option_key", "left", "left"},
		{"option_key", "right", "right"},
		{"layout", "win98", "win98"},
	} {
		if err := a.Run("app.set", map[string]any{"key": tc.key, "value": tc.value}); err != nil {
			t.Fatalf("app.set %s=%s: %v", tc.key, tc.value, err)
		}
		if got := a.Settings()[tc.key]; got != tc.want {
			t.Fatalf("%s = %#v, want %q", tc.key, got, tc.want)
		}
	}
	// A layout from layouts/*.json keeps its name; the layout engine
	// validates it when it loads. An empty name is still refused.
	if err := a.Run("app.set", map[string]any{"key": "layout", "value": ""}); err == nil {
		t.Fatal("an empty layout should fail")
	}
	if err := a.Run("app.set", map[string]any{"key": "option_key", "value": "sometimes"}); err == nil {
		t.Fatal("an unknown option key should fail")
	}
}
