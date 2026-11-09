// Command check-version keeps the app version, the core version and the
// release log in step, and optionally checks a built dist for the artifact
// names the release publishes.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"hiik/scripts/internal/release"
)

var semver = regexp.MustCompile(`^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$`)
var heading = regexp.MustCompile(`^##\s+(\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?)`)

func main() {
	root := flag.String("root", "..", "repository root")
	dist := flag.String("dist", "", "built dist directory to check for release artifact names")
	flag.Parse()

	app, err := appVersion(*root)
	if err != nil {
		fatal(err)
	}
	core, err := coreVersion(*root)
	if err != nil {
		fatal(err)
	}

	log, err := os.ReadFile(filepath.Join(*root, "docs", "release.md"))
	if err != nil {
		fatal(err)
	}
	if !hasVersionSection(string(log), app) {
		fatal(fmt.Errorf("docs/release.md has no section for %s", app))
	}
	if *dist != "" {
		if err := checkDist(*dist, app); err != nil {
			fatal(err)
		}
		fmt.Printf("dist ok: %d artifacts for %s\n", len(expectedArtifacts(app)), app)
	}
	fmt.Printf("version ok: app %s, core %s\n", app, core)
}

// appVersion reads and validates apps/desktop/mygo.json's version.
func appVersion(root string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, "apps", "desktop", "mygo.json"))
	if err != nil {
		return "", err
	}
	var cfg struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return "", err
	}
	if !semver.MatchString(cfg.Version) {
		return "", fmt.Errorf("mygo.json: version %q is not semver", cfg.Version)
	}
	return cfg.Version, nil
}

// coreVersion reads and validates the vendored core schema's version.
func coreVersion(root string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, "contracts", "hc.openrpc.json"))
	if err != nil {
		return "", err
	}
	var schema struct {
		Info struct {
			Version string `json:"version"`
		} `json:"info"`
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		return "", fmt.Errorf("hc.openrpc.json: %w", err)
	}
	if !semver.MatchString(schema.Info.Version) {
		return "", fmt.Errorf("hc.openrpc.json: core version %q is not semver", schema.Info.Version)
	}
	return schema.Info.Version, nil
}

// expectedArtifacts lists the release assets for version, relative to the
// dist directory. The names are the contract the updater selects from and
// the runbook documents; mygo's build layout supplies the folders.
func expectedArtifacts(version string) []string {
	return release.Paths(version)
}

// checkDist reports every expected release asset that is missing or empty.
func checkDist(dist, version string) error {
	if problems := release.Problems(dist, version); len(problems) > 0 {
		return errors.New("dist: " + strings.Join(problems, "; "))
	}
	return nil
}

// hasVersionSection reports whether the release log has a version heading,
// "## X.Y.Z", for version.
func hasVersionSection(log, version string) bool {
	for _, line := range strings.Split(log, "\n") {
		if m := heading.FindStringSubmatch(line); m != nil && m[1] == version {
			return true
		}
	}
	return false
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
