package keys

import "testing"

func TestFormat(t *testing.T) {
	cases := []struct{ chord, goos, want string }{
		{"Mod+P", "windows", "Ctrl+Shift+P"},
		{"Mod+Shift+P", "windows", "Ctrl+Shift+P"},
		{"ctrl-k", "windows", "Ctrl+K"},
		{"none", "windows", ""},
		{"Mod+P", "darwin", "⌘P"},
		{"Mod+Shift+P", "darwin", "⇧⌘P"},
	}
	for _, tc := range cases {
		got := formatFor(tc.chord, tc.goos)
		if got != tc.want {
			t.Errorf("format(%q, %q) = %q, want %q", tc.chord, tc.goos, got, tc.want)
		}
	}
}

func TestCanonicalAndParse(t *testing.T) {
	cases := []struct{ raw, want string }{
		{"Mod+Shift+K", "Mod+Shift+K"},
		{"cmd-shift-k", "Cmd+Shift+K"},
		{"ctrl-+", "Ctrl+="},
	}
	for _, tc := range cases {
		got, ok := Canonical(tc.raw)
		if !ok || got != tc.want {
			t.Errorf("Canonical(%q) = %q, %v; want %q", tc.raw, got, ok, tc.want)
		}
	}
	if _, _, ok := Parse("Mod+NotAKey"); ok {
		t.Error("Parse accepted an unknown key")
	}
}
