package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestParsePlatforms(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"all", []string{"windows", "linux"}},
		{"both", []string{"windows", "linux"}},
		{"", []string{"windows", "linux"}},
		{"windows", []string{"windows"}},
		{"linux", []string{"linux"}},
		{"windows,linux", []string{"windows", "linux"}},
		{"linux, windows", []string{"linux", "windows"}},
	}
	for _, tc := range cases {
		got, err := parsePlatforms(tc.in)
		if err != nil {
			t.Fatalf("parsePlatforms(%q): %v", tc.in, err)
		}
		if !slices.Equal(got, tc.want) {
			t.Fatalf("parsePlatforms(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
	for _, bad := range []string{"darwin", "windows,darwin", "freebsd"} {
		if _, err := parsePlatforms(bad); err == nil {
			t.Fatalf("parsePlatforms(%q) succeeded", bad)
		}
	}
}

func TestResolveVersionReadsOnlyWhenMissing(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "apps", "desktop")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mygo.json"), []byte(`{"version":"1.2.3"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := resolveVersion(root, ""); err != nil || got != "1.2.3" {
		t.Fatalf("resolveVersion = %q, %v", got, err)
	}
	if got, err := resolveVersion(root, "9.9.9"); err != nil || got != "9.9.9" {
		t.Fatalf("explicit version = %q, %v", got, err)
	}
}

func TestRuntimeStepsNameTheArtifacts(t *testing.T) {
	steps, err := runtimeSteps("1.2.3", []string{"windows", "linux"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"hiik-setup-1.2.3-amd64.exe",
		"hiik_1.2.3_amd64.deb",
		"hiik-1.2.3-linux-amd64.tar.gz",
		"never by a signature",
	} {
		if !strings.Contains(steps, want) {
			t.Errorf("steps do not mention %q:\n%s", want, steps)
		}
	}
	if _, err := runtimeSteps("1.2.3", []string{"darwin"}); err == nil {
		t.Fatal("darwin has no runtime steps here")
	}
}
