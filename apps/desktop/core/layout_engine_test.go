package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hiik/mods/api"
)

func writeLayoutFile(t *testing.T, name, body string) string {
	t.Helper()
	home := os.Getenv("HIIK_HOME")
	dir := filepath.Join(home, "layouts")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name+".json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLayoutLoadValidateAndNames(t *testing.T) {
	t.Setenv("HIIK_HOME", t.TempDir())
	a := testApp()
	writeLayoutFile(t, "mine", `{
	  "id": "root", "type": "column", "children": [
	    {"id": "top", "type": "block", "size": {"px": 40}, "content": {"mod": "tabs"}},
	    {"id": "main", "type": "block", "content": {"tab": "active"}}
	  ]
	}`)
	writeLayoutFile(t, "broken", `{"id":"root","type":"row","children":[{"id":"x","type":"block"}]}`)

	names := a.LayoutNames()
	if !hasLayoutName(names, api.LayoutHerdr) || !hasLayoutName(names, api.LayoutTerminals) || !hasLayoutName(names, "mine") {
		t.Fatalf("LayoutNames() = %v, want herdr, terminals, mine", names)
	}
	if hasLayoutName(names, "broken") {
		t.Fatalf("LayoutNames() included an invalid layout: %v", names)
	}
	if len(a.LayoutWarnings()) == 0 {
		t.Fatal("an invalid layout file was not reported")
	}
	tree, ok := a.LayoutTree("mine")
	if !ok || tree.ID != "root" || len(tree.Children) != 2 {
		t.Fatalf("LayoutTree(mine) = %#v, %v", tree, ok)
	}
	if _, ok := a.LayoutTree("broken"); ok {
		t.Fatal("LayoutTree(broken) returned an invalid tree")
	}

	if err := a.Run("layout.switch", map[string]any{"name": "mine"}); err != nil {
		t.Fatal(err)
	}
	if got := a.Layout(); got != "mine" {
		t.Fatalf("Layout() = %q, want mine", got)
	}
	if got := a.Settings()["layout"]; got != "mine" {
		t.Fatalf("settings layout = %#v, want mine", got)
	}
}

func TestLayoutSwitchFallsBackAndCycles(t *testing.T) {
	t.Setenv("HIIK_HOME", t.TempDir())
	a := testApp()
	a.mu.Lock()
	a.settings.Layout = "missing"
	a.mu.Unlock()
	if got := a.Layout(); got != api.LayoutHerdr {
		t.Fatalf("a missing layout fell back to %q, want herdr", got)
	}
	if err := a.Run("layout.switch", map[string]any{"name": "missing"}); err == nil {
		t.Fatal("layout.switch accepted an unknown layout")
	}
	if err := a.Run("layout.switch", nil); err != nil {
		t.Fatal(err)
	}
	if got := a.Layout(); got != api.LayoutTerminals {
		t.Fatalf("layout.switch with no name showed %q, want the next layout", got)
	}
}

func TestLayoutCloneAndReset(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HIIK_HOME", home)
	a := testApp()
	if err := a.Run("layout.clone", map[string]any{"name": api.LayoutHerdr}); err != nil {
		t.Fatal(err)
	}
	if got := a.Layout(); got != "herdr-copy" {
		t.Fatalf("layout.clone showed %q, want herdr-copy", got)
	}
	path := filepath.Join(home, "layouts", "herdr-copy.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("layout.clone did not write %s: %v", path, err)
	}
	tree, err := api.ParseLayout(data)
	if err != nil {
		t.Fatalf("the cloned layout does not parse: %v\n%s", err, data)
	}
	if tree.ID != "root" || len(tree.Children) == 0 {
		t.Fatalf("the clone lost its structure: %#v", tree)
	}
	if err := a.Run("layout.clone", map[string]any{"name": api.LayoutHerdr}); err == nil {
		t.Fatal("layout.clone overwrote an existing name")
	}

	if err := a.Run("layout.hide", map[string]any{"id": "sidebar"}); err != nil {
		t.Fatal(err)
	}
	if got := a.LayoutState().Hidden["sidebar"]; !got {
		t.Fatal("layout.hide did not hide the sidebar")
	}
	if err := a.Run("layout.reset", map[string]any{"name": "herdr-copy"}); err != nil {
		t.Fatal(err)
	}
	if got := a.LayoutState().Hidden["sidebar"]; got {
		t.Fatal("layout.reset kept the hidden node")
	}
}

func TestLayoutNodeCommands(t *testing.T) {
	t.Setenv("HIIK_HOME", t.TempDir())
	a := testApp()
	if err := a.Run("layout.switch", map[string]any{"name": api.LayoutHerdr}); err != nil {
		t.Fatal(err)
	}
	if err := a.Run("layout.toggle", map[string]any{"id": "sidebar"}); err != nil {
		t.Fatal(err)
	}
	if !a.LayoutState().Hidden["sidebar"] {
		t.Fatal("layout.toggle did not hide the sidebar")
	}
	if err := a.Run("layout.toggle", map[string]any{"id": "sidebar"}); err != nil {
		t.Fatal(err)
	}
	if a.LayoutState().Hidden["sidebar"] {
		t.Fatal("layout.toggle did not show the sidebar again")
	}
	if err := a.Run("layout.resize", map[string]any{"id": "sidebar", "px": 320}); err != nil {
		t.Fatal(err)
	}
	size := a.LayoutState().Sizes["sidebar"]
	if size.Px == nil || *size.Px != 320 {
		t.Fatalf("sidebar size = %#v, want 320px", size)
	}
	if err := a.Run("layout.resize", map[string]any{"id": "sidebar", "px": 300, "weight": 2}); err == nil {
		t.Fatal("layout.resize accepted px and weight together")
	}
	if err := a.Run("layout.show", map[string]any{"id": "nope"}); err == nil {
		t.Fatal("layout.show accepted an unknown node")
	}

	writeLayoutFile(t, "stacked", `{
	  "id": "root", "type": "stack", "active": "one",
	  "children": [
	    {"id": "one", "type": "block", "content": {"tab": "active"}},
	    {"id": "two", "type": "block", "content": {"mod": "tabs"}}
	  ]
	}`)
	if err := a.Run("layout.switch", map[string]any{"name": "stacked"}); err != nil {
		t.Fatal(err)
	}
	if err := a.Run("layout.select", map[string]any{"stack": "root", "child": "two"}); err != nil {
		t.Fatal(err)
	}
	if got := a.LayoutState().Stacks["root"]; got != "two" {
		t.Fatalf("stack shows %q, want two", got)
	}
	if err := a.Run("layout.select", map[string]any{"stack": "root", "child": "nope"}); err == nil {
		t.Fatal("layout.select accepted a child that is not in the stack")
	}
	if err := a.Run("layout.select", map[string]any{"stack": "one", "child": "two"}); err == nil {
		t.Fatal("layout.select accepted a node that is not a stack")
	}
}

func TestLayoutCloneWritesReadablePadding(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HIIK_HOME", home)
	a := testApp()
	if err := a.Run("layout.clone", map[string]any{"name": api.LayoutTerminals, "as": "term-copy"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(home, "layouts", "term-copy.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"X"`) || strings.Contains(string(data), `"Px"`) {
		t.Fatalf("the clone writes internal field names: %s", data)
	}
	if _, err := api.ParseLayout(data); err != nil {
		t.Fatalf("the cloned terminals layout does not parse: %v", err)
	}
}

func hasLayoutName(names []string, want string) bool {
	for _, name := range names {
		if name == want {
			return true
		}
	}
	return false
}
