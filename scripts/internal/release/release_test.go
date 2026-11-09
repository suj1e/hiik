package release

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestArtifactsCoverEveryReleaseAsset(t *testing.T) {
	got := Paths("1.2.3")
	want := []string{
		filepath.Join("darwin-universal", "hiik-1.2.3.dmg"),
		filepath.Join("windows-amd64", "hiik-setup-1.2.3-amd64.exe"),
		filepath.Join("linux-amd64", "hiik_1.2.3_amd64.deb"),
		filepath.Join("linux-amd64", "hiik-1.2.3-linux-amd64.tar.gz"),
	}
	if !slices.Equal(got, want) {
		t.Fatalf("Paths = %v, want %v", got, want)
	}
}

func TestForPlatformsFilters(t *testing.T) {
	got := ForPlatforms("1.2.3", "windows")
	if len(got) != 1 || got[0].Platform != "windows" {
		t.Fatalf("windows artifacts = %+v", got)
	}
	if got := ForPlatforms("1.2.3", "linux"); len(got) != 2 {
		t.Fatalf("linux artifacts = %+v", got)
	}
	if got := ForPlatforms("1.2.3"); len(got) != 4 {
		t.Fatalf("all artifacts = %+v", got)
	}
	if got := ForPlatforms("1.2.3", "freebsd"); len(got) != 0 {
		t.Fatalf("freebsd artifacts = %+v", got)
	}
}

func TestProblemsReportsMissingAndEmpty(t *testing.T) {
	dir := t.TempDir()
	if problems := Problems(dir, "1.2.3"); len(problems) != 4 {
		t.Fatalf("problems = %v, want four", problems)
	}
	for _, rel := range Paths("1.2.3") {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("artifact"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if problems := Problems(dir, "1.2.3"); len(problems) != 0 {
		t.Fatalf("problems = %v", problems)
	}
	if err := os.WriteFile(filepath.Join(dir, Paths("1.2.3")[0]), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if problems := Problems(dir, "1.2.3"); len(problems) != 1 {
		t.Fatalf("problems = %v, want one empty", problems)
	}
}
