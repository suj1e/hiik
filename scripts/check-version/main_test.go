package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestSemver(t *testing.T) {
	for _, v := range []string{"0.1.0", "1.2.3", "10.20.30", "1.0.0-rc.1"} {
		if !semver.MatchString(v) {
			t.Errorf("%q is not accepted", v)
		}
	}
	for _, v := range []string{"", "1", "1.2", "v1.2.3", "1.2.3.4", "1.2.x"} {
		if semver.MatchString(v) {
			t.Errorf("%q is accepted", v)
		}
	}
}

func TestHasVersionSection(t *testing.T) {
	log := "# Release notes\n\n## 0.2.0\n\n- New.\n\n## 0.1.0\n\n- First.\n"
	for _, v := range []string{"0.1.0", "0.2.0"} {
		if !hasVersionSection(log, v) {
			t.Errorf("%s: no section found", v)
		}
	}
	for _, v := range []string{"0.3.0", "0.1"} {
		if hasVersionSection(log, v) {
			t.Errorf("%s: section found", v)
		}
	}
}

func TestExpectedArtifactsUseVersion(t *testing.T) {
	got := expectedArtifacts("1.2.3")
	want := []string{
		filepath.Join("darwin-universal", "hiik-1.2.3.dmg"),
		filepath.Join("windows-amd64", "hiik-setup-1.2.3-amd64.exe"),
		filepath.Join("linux-amd64", "hiik_1.2.3_amd64.deb"),
		filepath.Join("linux-amd64", "hiik-1.2.3-linux-amd64.tar.gz"),
	}
	if !slices.Equal(got, want) {
		t.Fatalf("expectedArtifacts = %v, want %v", got, want)
	}
}

func TestCheckDistNeedsEveryArtifact(t *testing.T) {
	dir := t.TempDir()
	if err := checkDist(dir, "1.2.3"); err == nil {
		t.Fatal("an empty dist should fail")
	}
	for _, rel := range expectedArtifacts("1.2.3") {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("artifact"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := checkDist(dir, "1.2.3"); err != nil {
		t.Fatal(err)
	}
	empty := filepath.Join(dir, expectedArtifacts("1.2.3")[0])
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := checkDist(dir, "1.2.3"); err == nil {
		t.Fatal("an empty artifact should fail")
	}
}

func TestCoreVersionReadsContract(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "contracts"), 0o755); err != nil {
		t.Fatal(err)
	}
	schema := `{"info":{"version":"0.1.19"}}`
	if err := os.WriteFile(filepath.Join(root, "contracts", "hc.openrpc.json"), []byte(schema), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := coreVersion(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != "0.1.19" {
		t.Fatalf("core version = %q", got)
	}
}
