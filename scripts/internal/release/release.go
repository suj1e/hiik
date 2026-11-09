// Package release names the artifacts an unsigned hiik release publishes.
// The updater selects from these names, the runbook documents them, and the
// release check refuses a dist that has anything else.
package release

import (
	"os"
	"path/filepath"
	"strings"
)

// Artifact is one release asset for a platform. Path is relative to the
// dist directory mygo writes.
type Artifact struct {
	Platform string
	Path     string
}

// Artifacts returns every release asset for version.
func Artifacts(version string) []Artifact {
	return []Artifact{
		{Platform: "darwin", Path: filepath.Join("darwin-universal", "hiik-"+version+".dmg")},
		{Platform: "windows", Path: filepath.Join("windows-amd64", "hiik-setup-"+version+"-amd64.exe")},
		{Platform: "linux", Path: filepath.Join("linux-amd64", "hiik_"+version+"_amd64.deb")},
		{Platform: "linux", Path: filepath.Join("linux-amd64", "hiik-"+version+"-linux-amd64.tar.gz")},
	}
}

// Paths is Artifacts' paths alone, in release order.
func Paths(version string) []string {
	artifacts := Artifacts(version)
	out := make([]string, 0, len(artifacts))
	for _, a := range artifacts {
		out = append(out, a.Path)
	}
	return out
}

// ForPlatforms keeps the artifacts built for one of platforms. An empty
// list keeps everything, so a local check can inspect the whole dist.
func ForPlatforms(version string, platforms ...string) []Artifact {
	all := Artifacts(version)
	if len(platforms) == 0 {
		return all
	}
	want := make(map[string]bool, len(platforms))
	for _, p := range platforms {
		want[strings.ToLower(strings.TrimSpace(p))] = true
	}
	out := make([]Artifact, 0, len(all))
	for _, a := range all {
		if want[a.Platform] {
			out = append(out, a)
		}
	}
	return out
}

// Problems reports every artifact that is missing, a directory or empty.
func Problems(dist, version string, platforms ...string) []string {
	var problems []string
	for _, a := range ForPlatforms(version, platforms...) {
		path := filepath.Join(dist, a.Path)
		info, err := os.Stat(path)
		if err != nil {
			problems = append(problems, "missing artifact "+a.Path)
			continue
		}
		if info.IsDir() || info.Size() == 0 {
			problems = append(problems, "empty artifact "+a.Path)
		}
	}
	return problems
}
