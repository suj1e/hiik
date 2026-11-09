package core

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"hiik/core/hc"
)

func TestSelectUpdateAssetMatchesPlatform(t *testing.T) {
	wrongOS := "windows"
	if runtime.GOOS == "windows" {
		wrongOS = "darwin"
	}
	good := fmt.Sprintf("hiik-%s-%s.zip", runtime.GOOS, runtime.GOARCH)
	assets := []UpdateAsset{
		{Name: good, URL: "https://example.test/good"},
		{Name: fmt.Sprintf("hiik-%s-%s.zip", wrongOS, runtime.GOARCH), URL: "https://example.test/wrong-os"},
		{Name: fmt.Sprintf("hiik-%s-%s.sha256", runtime.GOOS, runtime.GOARCH), URL: "https://example.test/checksums"},
	}
	got, err := selectUpdateAsset(assets)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != good {
		t.Fatalf("selectUpdateAsset = %q, want %q", got.Name, good)
	}
}

func TestSelectUpdateAssetRejectsChecksums(t *testing.T) {
	assets := []UpdateAsset{
		{Name: fmt.Sprintf("hiik-%s-%s.sha256", runtime.GOOS, runtime.GOARCH), URL: "https://example.test/sha"},
	}
	if _, err := selectUpdateAsset(assets); err == nil {
		t.Fatal("a checksum is not an installable asset")
	}
}

func TestUpdateCoreWithoutBinaryIsClear(t *testing.T) {
	a := &App{}
	a.settings.CorePath = "/nonexistent/hiik-test-hc"
	if _, err := a.UpdateCore(context.Background()); err == nil {
		t.Fatal("missing hc should be an error")
	} else if !strings.Contains(err.Error(), "core update unavailable") {
		t.Fatalf("error = %q", err)
	}
}

func TestAssetExtScorePrefersPackages(t *testing.T) {
	cases := []struct {
		name, goos string
		want       int
	}{
		{"hiik-1.0.0.dmg", "darwin", 40},
		{"hiik-1.0.0.zip", "darwin", 20},
		{"hiik-setup-1.0.0.exe", "windows", 40},
		{"hiik-1.0.0.zip", "windows", 20},
		{"hiik-1.0.0.AppImage", "linux", 40},
		{"hiik_1.0.0_amd64.deb", "linux", 35},
		{"hiik-1.0.0.tar.gz", "linux", 20},
	}
	for _, tc := range cases {
		if got := assetExtScore(strings.ToLower(tc.name), tc.goos); got != tc.want {
			t.Errorf("assetExtScore(%q, %s) = %d, want %d", tc.name, tc.goos, got, tc.want)
		}
	}
	for _, name := range []string{"hiik-1.0.0.sha256", "hiik-1.0.0.txt"} {
		if got := assetExtScore(name, runtime.GOOS); got != 0 {
			t.Errorf("assetExtScore(%q) = %d, want 0", name, got)
		}
	}
}

func TestCoreOwnerNamesPackageManagers(t *testing.T) {
	cases := []struct {
		path        string
		installedBy string
		want        string
	}{
		{"/opt/homebrew/Cellar/hc/0.1.19/bin/hc", "", "homebrew"},
		{"/usr/local/Homebrew/bin/hc", "", "homebrew"},
		{"/nix/store/abc-hc-0.1.19/bin/hc", "", "nix"},
		{"/Users/me/.local/share/mise/installs/hc/0.1.19/hc", "", "mise"},
		{"/Users/me/.local/share/mise/shims/hc", "", "mise"},
		{"/Users/me/.cargo/bin/hc", "", "cargo"},
		{"/usr/local/bin/hc", "", ""},
		{"/usr/local/bin/hc", "system", "system"},
		{"/opt/homebrew/bin/hc", "nix", "nix"},
	}
	for _, tc := range cases {
		if got := coreOwner(tc.path, tc.installedBy); got != tc.want {
			t.Errorf("coreOwner(%q, %q) = %q, want %q",
				tc.path, tc.installedBy, got, tc.want)
		}
	}
}

// statusCore is a fake core answering `server.status`.
type statusCore struct {
	*fakeCore
	status coreServerStatus
	err    error
}

func (c *statusCore) Call(_ context.Context, method string, _, result any) error {
	if c.err != nil {
		return c.err
	}
	if method != "server.status" {
		return nil
	}
	if out, ok := result.(*coreServerStatus); ok {
		*out = c.status
	}
	return nil
}

func TestCoreUpdateInfoReadsStatus(t *testing.T) {
	a := testApp()
	homebrew := "homebrew"
	c := &statusCore{
		fakeCore: newFakeCore(),
		status: coreServerStatus{
			Version:     "hc 0.1.20",
			InstalledBy: &homebrew,
		},
	}
	a.SetCore(c)
	a.hello = hc.Hello{Generation: 1, Version: "0.1.19"}

	info := a.CoreUpdateInfo(context.Background(), "/usr/local/bin/hc")
	if info.Generation != 1 {
		t.Fatalf("generation = %d", info.Generation)
	}
	if info.Version != "0.1.20" {
		t.Fatalf("version = %q", info.Version)
	}
	if info.InstalledBy != "homebrew" {
		t.Fatalf("installed by = %q", info.InstalledBy)
	}
}

func TestCoreUpdateRefusesManagedAndWrongGeneration(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fixture is a shell script")
	}
	dir := t.TempDir()
	hc := filepath.Join(dir, "hc")
	marker := filepath.Join(dir, "ran")
	script := "#!/bin/sh\necho ran > " + marker + "\necho updated\n"
	if err := os.WriteFile(hc, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	managed := testApp()
	managed.settings.CorePath = hc
	managed.hello = hcHello(1, "0.1.19")
	nix := "nix"
	managed.SetCore(&statusCore{
		fakeCore: newFakeCore(),
		status:   coreServerStatus{InstalledBy: &nix},
	})
	if _, err := managed.UpdateCore(context.Background()); err == nil ||
		!strings.Contains(err.Error(), "owned by nix") {
		t.Fatalf("managed core error = %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("hc update ran for a package-managed core")
	}

	wrong := testApp()
	wrong.settings.CorePath = hc
	wrong.hello = hcHello(2, "0.2.0")
	wrong.SetCore(newFakeCore())
	if _, err := wrong.UpdateCore(context.Background()); err == nil ||
		!strings.Contains(err.Error(), "update hiik instead") {
		t.Fatalf("wrong generation error = %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("hc update ran for a core of another generation")
	}

	plain := testApp()
	plain.settings.CorePath = hc
	plain.hello = hcHello(1, "0.1.19")
	plain.SetCore(newFakeCore())
	msg, err := plain.UpdateCore(context.Background())
	if err != nil {
		t.Fatalf("plain core update: %v", err)
	}
	if msg != "updated" {
		t.Fatalf("message = %q", msg)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("hc update did not run: %v", err)
	}
}

func TestInstallUpdateRecordsIntent(t *testing.T) {
	resetPendingInstall()
	t.Cleanup(resetPendingInstall)

	body := []byte("package bytes")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	name := fmt.Sprintf("hiik-%s-%s.zip", runtime.GOOS, runtime.GOARCH)
	storeUpdate(UpdateStatus{
		CheckedAt: time.Now(),
		Current:   Version,
		Latest:    "9.9.9",
		Assets:    []UpdateAsset{{Name: name, URL: srv.URL, Size: int64(len(body))}},
	})

	a := testApp()
	err := a.Run("app.install_update", nil)
	if err == nil || !strings.Contains(err.Error(), "quit hiik") {
		t.Fatalf("install message = %v", err)
	}
	if got := a.Overlay(); got != "install_update" {
		t.Fatalf("overlay = %q, want install_update", got)
	}
	plan, ok := a.PendingInstall()
	if !ok {
		t.Fatal("no pending install")
	}
	if plan.Version != "9.9.9" || plan.Asset != name {
		t.Fatalf("plan = %+v", plan)
	}
	want := filepath.Join(home, "Downloads", name)
	if plan.Path != want {
		t.Fatalf("path = %q, want %q", plan.Path, want)
	}
	got, err := os.ReadFile(want)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(body) {
		t.Fatalf("downloaded %q", got)
	}
	// A second run reuses the download instead of checking again.
	storeUpdate(UpdateStatus{})
	if err := a.Run("app.install_update", nil); err == nil {
		t.Fatal("install should still report its message")
	}
	if _, ok := a.PendingInstall(); !ok {
		t.Fatal("pending install was lost")
	}
}

func TestOpenInstallPackageWithoutDownloadIsClear(t *testing.T) {
	resetPendingInstall()
	if err := (&App{}).OpenInstallPackage(); err == nil ||
		!strings.Contains(err.Error(), "app.install_update") {
		t.Fatalf("error = %v", err)
	}
}

func resetPendingInstall() {
	updateMu.Lock()
	pendingPlan = InstallPlan{}
	updateMu.Unlock()
}

func hcHello(generation int, version string) hc.Hello {
	return hc.Hello{Generation: generation, Version: version}
}
