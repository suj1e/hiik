// Command accept-runtime checks that a built (unsigned) release has the
// Windows and Linux artifacts and prints the manual runtime steps for each.
// It never signs, installs or launches anything.
//
//	go run ./accept-runtime -dist ../apps/desktop/dist
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"hiik/scripts/internal/release"
)

func main() {
	root := flag.String("root", "..", "repository root")
	dist := flag.String("dist", filepath.Join("..", "apps", "desktop", "dist"), "built dist directory")
	version := flag.String("version", "", "release version; defaults to apps/desktop/mygo.json")
	platform := flag.String("platform", "all", "windows, linux, both, or all")
	flag.Parse()

	v, err := resolveVersion(*root, *version)
	if err != nil {
		fatal(err)
	}
	platforms, err := parsePlatforms(*platform)
	if err != nil {
		fatal(err)
	}
	problems := release.Problems(*dist, v, platforms...)
	if len(problems) > 0 {
		fatal(fmt.Errorf("runtime acceptance for %s: %s", v, strings.Join(problems, "; ")))
	}
	fmt.Printf("artifacts ok: %s (%s)\n", v, strings.Join(platforms, ", "))
	steps, err := runtimeSteps(v, platforms)
	if err != nil {
		fatal(err)
	}
	fmt.Print(steps)
}

// resolveVersion reads the release version from mygo.json unless the caller
// named one.
func resolveVersion(root, version string) (string, error) {
	if version = strings.TrimSpace(version); version != "" {
		return version, nil
	}
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
	if strings.TrimSpace(cfg.Version) == "" {
		return "", fmt.Errorf("mygo.json has no version")
	}
	return cfg.Version, nil
}

// parsePlatforms turns the flag into the platforms to accept. "all" and
// "both" mean the two platforms this script is for.
func parsePlatforms(value string) ([]string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" || value == "all" || value == "both" || value == "windows,linux" || value == "linux,windows" {
		return []string{"windows", "linux"}, nil
	}
	var out []string
	for _, part := range strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ' '
	}) {
		switch part {
		case "windows", "linux":
			out = append(out, part)
		default:
			return nil, fmt.Errorf("unknown platform %q: use windows, linux or all", part)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no platform: use windows, linux or all")
	}
	return out, nil
}

// runtimeSteps is the manual acceptance checklist. There is no signature to
// verify, so the steps run the package and the app.
func runtimeSteps(version string, platforms []string) (string, error) {
	var b strings.Builder
	for _, platform := range platforms {
		switch platform {
		case "windows":
			fmt.Fprintf(&b, `
windows:
  1. Get-AuthenticodeSignature .\windows-amd64\hiik-setup-%[1]s-amd64.exe   # Status NotSigned
  2. .\windows-amd64\hiik-setup-%[1]s-amd64.exe
  3. Launch hiik from the Start menu; SmartScreen's "more info -> run anyway" is expected.
  4. Create a space, run a shell, type, split, resize, close and reopen.
`, version)
		case "linux":
			fmt.Fprintf(&b, `
linux:
  1. dpkg-deb -I linux-amd64/hiik_%[1]s_amd64.deb
  2. sudo apt install ./linux-amd64/hiik_%[1]s_amd64.deb
  3. hiik
  4. mkdir -p /tmp/hiik-portable && tar -xzf linux-amd64/hiik-%[1]s-linux-amd64.tar.gz -C /tmp/hiik-portable
  5. /tmp/hiik-portable/hiik
  6. In both: create a space, run a shell, type, split, resize, close and reopen.
`, version)
		default:
			return "", fmt.Errorf("no runtime steps for %q", platform)
		}
	}
	b.WriteString("\nUpdates are unsigned: verify each download by running it, never by a signature.\n")
	return b.String(), nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
