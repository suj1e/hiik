package core

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
)

// UpdateAsset is one file a release offers. The app downloads the asset it
// matches to this computer's OS and architecture and opens it; it never
// replaces the running binary.
type UpdateAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

// UpdateStatus is the result of the last update check.
type UpdateStatus struct {
	CheckedAt time.Time
	Current   string
	Latest    string
	URL       string
	Notes     string
	Assets    []UpdateAsset
	Err       string
}

var (
	updateMu     sync.Mutex
	lastUpdate   UpdateStatus
	pendingPlan  InstallPlan
	updateClient = &http.Client{Timeout: 15 * time.Second}
	// downloadClient has its own, longer timeout: a release asset can be
	// hundreds of megabytes.
	downloadClient = &http.Client{Timeout: 10 * time.Minute}
)

// maxUpdateBytes bounds one downloaded release asset.
const maxUpdateBytes = 512 << 20

// clientCoreGeneration is the core protocol generation this app speaks. A
// core of another generation is updated by updating hiik, not `hc update`.
const clientCoreGeneration = 1

// InstallPlan is a downloaded release asset waiting for the shell to open
// it. Unsigned builds never replace the running binary: the user opens the
// package and quits hiik, and the package replaces the installed app.
type InstallPlan struct {
	Version string `json:"version"`
	Asset   string `json:"asset"`
	Path    string `json:"path"`
}

// CoreUpdateInfo is what the app learned about the connected core before
// running `hc update`: its protocol generation, version and package-manager
// owner. An owner means the package manager must perform the update.
type CoreUpdateInfo struct {
	Binary      string
	Version     string
	Generation  int
	InstalledBy string
}

// coreServerStatus is the part of `server.status` this app reads.
type coreServerStatus struct {
	Version     string  `json:"version"`
	InstalledBy *string `json:"installed_by"`
}

// UpdateStatus returns the last check, if any.
func (a *App) UpdateStatus() UpdateStatus {
	updateMu.Lock()
	defer updateMu.Unlock()
	return lastUpdate
}

// CheckUpdates asks the configured GitHub releases endpoint for the latest
// version. It never installs anything: releases are unsigned, so the app
// opens the release page instead.
func (a *App) CheckUpdates(ctx context.Context) (UpdateStatus, error) {
	repo := strings.TrimSpace(os.Getenv("HIIK_UPDATE_REPO"))
	if repo == "" {
		err := fmt.Errorf("updates are not configured: set HIIK_UPDATE_REPO to owner/repo")
		storeUpdate(UpdateStatus{CheckedAt: time.Now(), Current: Version, Err: err.Error()})
		return lastStatus(), err
	}
	url := "https://api.github.com/repos/" + repo + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return UpdateStatus{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "hiik/"+Version)
	resp, err := updateClient.Do(req)
	if err != nil {
		storeUpdate(UpdateStatus{CheckedAt: time.Now(), Current: Version, Err: err.Error()})
		return lastStatus(), err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("updates: GitHub returned %s", resp.Status)
		storeUpdate(UpdateStatus{CheckedAt: time.Now(), Current: Version, Err: err.Error()})
		return lastStatus(), err
	}
	var release struct {
		TagName string        `json:"tag_name"`
		Name    string        `json:"name"`
		Body    string        `json:"body"`
		HTMLURL string        `json:"html_url"`
		Assets  []UpdateAsset `json:"assets"`
	}
	if err := json.Unmarshal(body, &release); err != nil {
		storeUpdate(UpdateStatus{CheckedAt: time.Now(), Current: Version, Err: err.Error()})
		return lastStatus(), err
	}
	st := UpdateStatus{
		CheckedAt: time.Now(),
		Current:   Version,
		Latest:    strings.TrimPrefix(release.TagName, "v"),
		URL:       release.HTMLURL,
		Notes:     release.Body,
		Assets:    append([]UpdateAsset(nil), release.Assets...),
	}
	if st.Latest == "" {
		st.Latest = strings.TrimPrefix(release.Name, "v")
	}
	storeUpdate(st)
	return st, nil
}

func storeUpdate(st UpdateStatus) {
	updateMu.Lock()
	lastUpdate = st
	updateMu.Unlock()
}

func lastStatus() UpdateStatus {
	updateMu.Lock()
	defer updateMu.Unlock()
	return lastUpdate
}

// Newer reports whether latest is a later semantic version than current.
func Newer(current, latest string) bool {
	parse := func(s string) [3]int {
		var out [3]int
		for i, part := range strings.SplitN(strings.TrimPrefix(s, "v"), ".", 3) {
			if i > 2 {
				break
			}
			n, _ := strconv.Atoi(strings.TrimFunc(part, func(r rune) bool { return r < '0' || r > '9' }))
			out[i] = n
		}
		return out
	}
	a, b := parse(current), parse(latest)
	for i := range 3 {
		if b[i] != a[i] {
			return b[i] > a[i]
		}
	}
	return false
}

// DownloadUpdate downloads the release asset for this computer's OS and
// architecture to ~/Downloads and opens it. It never replaces the running
// binary; the release is unsigned, so opening the file is the install step.
// The download is remembered as the pending install, so `app.install_update`
// can finish without fetching it again.
func (a *App) DownloadUpdate(ctx context.Context) (string, error) {
	plan, err := a.FetchUpdate(ctx)
	if err != nil {
		return "", err
	}
	if err := openDownloaded(plan.Path); err != nil {
		return plan.Path, err
	}
	return plan.Path, nil
}

// InstallUpdate downloads the release asset for this computer's OS and
// architecture to ~/Downloads and records it as the install intent. It does
// not open the package: the shell shows the install overlay, opens the
// package and quits, and the package replaces the installed app. The running
// binary is never replaced by hiik itself.
func (a *App) InstallUpdate(ctx context.Context) (InstallPlan, error) {
	return a.FetchUpdate(ctx)
}

// FetchUpdate downloads the release asset for this computer's OS and
// architecture to ~/Downloads and records it as the pending install. It
// opens nothing and changes no settings.
func (a *App) FetchUpdate(ctx context.Context) (InstallPlan, error) {
	st := lastStatus()
	if st.Latest == "" || len(st.Assets) == 0 {
		fresh, err := a.CheckUpdates(ctx)
		if err != nil {
			return InstallPlan{}, err
		}
		st = fresh
	}
	asset, err := selectUpdateAsset(st.Assets)
	if err != nil {
		return InstallPlan{}, err
	}
	dir, err := downloadDir()
	if err != nil {
		return InstallPlan{}, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return InstallPlan{}, fmt.Errorf("updates: create %s: %w", dir, err)
	}
	name := filepath.Base(strings.ReplaceAll(asset.Name, "/", "_"))
	if name == "" || name == "." {
		return InstallPlan{}, fmt.Errorf("updates: release asset has no name")
	}
	path := filepath.Join(dir, name)
	if err := downloadAsset(ctx, asset.URL, path); err != nil {
		return InstallPlan{}, err
	}
	plan := InstallPlan{Version: st.Latest, Asset: asset.Name, Path: path}
	updateMu.Lock()
	pendingPlan = plan
	updateMu.Unlock()
	return plan, nil
}

// PendingInstall returns the downloaded asset waiting to be opened, if it
// is still on disk.
func (a *App) PendingInstall() (InstallPlan, bool) {
	updateMu.Lock()
	plan := pendingPlan
	updateMu.Unlock()
	if plan.Path == "" {
		return InstallPlan{}, false
	}
	if _, err := os.Stat(plan.Path); err != nil {
		return InstallPlan{}, false
	}
	return plan, true
}

// OpenInstallPackage opens the downloaded package with the desktop's
// handler. The shell calls it after the person confirms the install overlay,
// then quits; hiik never swaps its own binary.
func (a *App) OpenInstallPackage() error {
	plan, ok := a.PendingInstall()
	if !ok {
		return fmt.Errorf("updates: no downloaded package; run app.install_update first")
	}
	return openDownloaded(plan.Path)
}

// ClearPendingInstall forgets the pending download without deleting it.
func (a *App) ClearPendingInstall() {
	updateMu.Lock()
	pendingPlan = InstallPlan{}
	updateMu.Unlock()
}

// selectUpdateAsset picks the best release asset for this OS and
// architecture: a native package first, then an archive.
func selectUpdateAsset(assets []UpdateAsset) (UpdateAsset, error) {
	osAliases := osAssetAliases(runtime.GOOS)
	archAliases := archAssetAliases(runtime.GOARCH)
	type scored struct {
		asset UpdateAsset
		score int
	}
	var candidates []scored
	for _, asset := range assets {
		name := strings.ToLower(asset.Name)
		if name == "" || asset.URL == "" {
			continue
		}
		if !assetHasNamePart(name, osAliases) || !assetHasNamePart(name, archAliases) {
			continue
		}
		ext := assetExtScore(name, runtime.GOOS)
		if ext == 0 {
			continue
		}
		candidates = append(candidates, scored{asset: asset, score: ext})
	}
	if len(candidates) == 0 {
		names := make([]string, 0, len(assets))
		for _, asset := range assets {
			names = append(names, asset.Name)
		}
		return UpdateAsset{}, fmt.Errorf(
			"updates: no asset for %s/%s; release offers %s",
			runtime.GOOS, runtime.GOARCH, strings.Join(names, ", "))
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return candidates[i].asset.Name < candidates[j].asset.Name
	})
	return candidates[0].asset, nil
}

// assetHasNamePart reports whether one of name's words is an alias. Words
// split on everything but letters and digits, so "darwin" never looks like
// "win"; aliases with a separator, such as "x86_64", match as a substring.
func assetHasNamePart(name string, aliases []string) bool {
	parts := strings.FieldsFunc(name, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	})
	for _, alias := range aliases {
		if strings.Contains(alias, "_") || strings.Contains(alias, "-") {
			if strings.Contains(name, alias) {
				return true
			}
			continue
		}
		for _, part := range parts {
			if part == alias {
				return true
			}
		}
	}
	return false
}

func osAssetAliases(goos string) []string {
	switch goos {
	case "darwin":
		return []string{"darwin", "macos", "mac", "osx"}
	case "windows":
		return []string{"windows", "win", "win64", "win32"}
	case "linux":
		return []string{"linux"}
	default:
		return []string{goos}
	}
}

func archAssetAliases(goarch string) []string {
	switch goarch {
	case "amd64":
		return []string{"amd64", "x64", "x86_64", "x86-64", "universal"}
	case "arm64":
		return []string{"arm64", "aarch64", "universal"}
	case "386":
		return []string{"386", "i386", "x86"}
	case "arm":
		return []string{"arm"}
	default:
		return []string{goarch, "universal"}
	}
}

// assetExtScore prefers a native package over an archive. Zero means the
// file is not something the app should open.
func assetExtScore(name, goos string) int {
	switch goos {
	case "darwin":
		switch {
		case strings.HasSuffix(name, ".dmg"):
			return 40
		case strings.HasSuffix(name, ".pkg"):
			return 35
		case strings.HasSuffix(name, ".zip"):
			return 20
		}
	case "windows":
		switch {
		case strings.HasSuffix(name, ".exe"):
			return 40
		case strings.HasSuffix(name, ".msi"):
			return 35
		case strings.HasSuffix(name, ".zip"):
			return 20
		}
	default:
		switch {
		case strings.HasSuffix(name, ".appimage"):
			return 40
		case strings.HasSuffix(name, ".deb"):
			return 35
		case strings.HasSuffix(name, ".rpm"):
			return 34
		case strings.HasSuffix(name, ".tar.gz"), strings.HasSuffix(name, ".tgz"):
			return 20
		case strings.HasSuffix(name, ".zip"):
			return 15
		}
	}
	return 0
}

// downloadDir is the user's Downloads folder, where the app parks a
// release asset for the user to open.
func downloadDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("updates: find home directory: %w", err)
	}
	return filepath.Join(home, "Downloads"), nil
}

// downloadAsset writes url to path atomically, leaving nothing behind on
// failure.
func downloadAsset(ctx context.Context, url, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("updates: download %s: %w", url, err)
	}
	req.Header.Set("User-Agent", "hiik/"+Version)
	resp, err := downloadClient.Do(req)
	if err != nil {
		return fmt.Errorf("updates: download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("updates: download %s: %s", url, resp.Status)
	}
	tmp := path + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("updates: create %s: %w", tmp, err)
	}
	ok := false
	defer func() {
		_ = f.Close()
		if !ok {
			_ = os.Remove(tmp)
		}
	}()
	n, err := io.Copy(f, io.LimitReader(resp.Body, maxUpdateBytes+1))
	if err != nil {
		return fmt.Errorf("updates: download %s: %w", url, err)
	}
	if n > maxUpdateBytes {
		return fmt.Errorf("updates: %s is larger than %d bytes", url, maxUpdateBytes)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("updates: write %s: %w", tmp, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("updates: write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("updates: save %s: %w", path, err)
	}
	ok = true
	return nil
}

// openDownloaded opens a downloaded file with the desktop's handler.
func openDownloaded(path string) error {
	if err := mygo.Shell.OpenPath(path); err != nil {
		return fmt.Errorf("updates: open %s: %w", path, err)
	}
	return nil
}

// UpdateCore runs `hc update` for the configured core binary and returns
// the core's messages. Before touching it, the app checks the connected
// core's protocol generation and whether a package manager owns the binary;
// either answers refuse with an explanation in the command result. It never
// touches terminals and never stops the core, so a running core keeps
// serving them through the update.
func (a *App) UpdateCore(ctx context.Context) (string, error) {
	a.mu.RLock()
	binary := strings.TrimSpace(a.settings.CorePath)
	a.mu.RUnlock()
	if binary == "" {
		binary = "hc"
	}
	path, err := exec.LookPath(binary)
	if err != nil {
		return "", fmt.Errorf("core update unavailable: cannot find %q: %w", binary, err)
	}
	info := a.CoreUpdateInfo(ctx, path)
	if info.InstalledBy != "" {
		return "", fmt.Errorf(
			"core update unavailable: %s is owned by %s; update it there, not `hc update`",
			path, info.InstalledBy)
	}
	if info.Generation != 0 && info.Generation != clientCoreGeneration {
		version := info.Version
		if version == "" {
			version = path
		}
		return "", fmt.Errorf(
			"core update unavailable: %s speaks protocol generation %d, this app speaks generation %d; update hiik instead",
			version, info.Generation, clientCoreGeneration)
	}
	cmd := exec.CommandContext(ctx, path, "update")
	out, err := cmd.CombinedOutput()
	msg := strings.TrimSpace(string(out))
	if err != nil {
		if msg != "" {
			return msg, fmt.Errorf("hc update failed: %s", firstLine(msg))
		}
		return "", fmt.Errorf("hc update failed: %w", err)
	}
	return msg, nil
}

// CoreUpdateInfo reports the core's protocol generation, version and the
// package manager that owns the binary at path, if any. It reads the app's
// hello and, when connected, the core's `server.status`; it never runs a
// shell and never changes anything.
func (a *App) CoreUpdateInfo(ctx context.Context, path string) CoreUpdateInfo {
	info := CoreUpdateInfo{Binary: path}
	a.mu.RLock()
	hello := a.hello
	c := a.core
	a.mu.RUnlock()
	info.Generation = hello.Generation
	info.Version = hello.Version

	if c != nil {
		var status coreServerStatus
		callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := c.Call(callCtx, "server.status", nil, &status)
		cancel()
		if err == nil {
			if v := strings.TrimSpace(status.Version); v != "" {
				info.Version = strings.TrimPrefix(v, "hc ")
			}
			if status.InstalledBy != nil {
				info.InstalledBy = strings.TrimSpace(*status.InstalledBy)
			}
		}
	}
	info.InstalledBy = coreOwner(path, info.InstalledBy)
	return info
}

// coreOwner returns the package manager that owns a core binary. The core's
// own `server.status.installed_by` wins; otherwise the install path names
// the package manager. An empty answer means a plain install `hc update` may
// replace.
func coreOwner(path, installedBy string) string {
	if installedBy = strings.TrimSpace(installedBy); installedBy != "" {
		return installedBy
	}
	p := strings.ToLower(filepath.ToSlash(filepath.Clean(path)))
	switch {
	case strings.Contains(p, "/Cellar/"),
		strings.Contains(p, "/homebrew/"):
		return "homebrew"
	case strings.Contains(p, "/nix/store/"):
		return "nix"
	case strings.Contains(p, "/mise/installs/"),
		strings.Contains(p, "/mise/shims/"):
		return "mise"
	case strings.Contains(p, "/.cargo/bin/"),
		strings.Contains(p, "/cargo/bin/"):
		return "cargo"
	}
	return ""
}

// firstLine returns the first non-empty line of s.
func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return ""
}
