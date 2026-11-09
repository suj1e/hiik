package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewer(t *testing.T) {
	cases := []struct {
		current, latest string
		want            bool
	}{
		{"0.1.0", "0.1.1", true},
		{"0.1.0", "0.2.0", true},
		{"0.1.0", "1.0.0", true},
		{"1.2.3", "1.2.3", false},
		{"1.2.3", "1.2.2", false},
		{"1.2.3", "v1.2.4", true},
		{"", "0.1.0", true},
	}
	for _, tc := range cases {
		if got := Newer(tc.current, tc.latest); got != tc.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", tc.current, tc.latest, got, tc.want)
		}
	}
}

func TestSaveFeedback(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HIIK_HOME", home)
	a := &App{}
	path, err := a.SaveFeedback("the palette lost focus")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(path, filepath.Join(home, "feedback")) {
		t.Fatalf("path = %q", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "the palette lost focus") {
		t.Fatalf("feedback = %q", data)
	}
	if _, err := a.SaveFeedback("   "); err == nil {
		t.Fatal("empty feedback should fail")
	}
}

func TestReleaseNotesBundled(t *testing.T) {
	a := &App{}
	if a.Version() != Version || Version == "" {
		t.Fatalf("version = %q", a.Version())
	}
	if !strings.Contains(a.ReleaseNotes(), "0.1.0") {
		t.Fatalf("release notes = %q", a.ReleaseNotes())
	}
}
