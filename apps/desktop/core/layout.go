package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"hiik/core/store"
	"hiik/mods/api"
)

// This file is the layout engine: it reads layouts/<name>.json, validates
// them, keeps the runtime changes layout node commands make, and registers
// those commands. The shell draws the tree; this file decides which tree.

// cachedLayout is one parsed layout file and the file stamp it came from.
type cachedLayout struct {
	modTime time.Time
	size    int64
	layout  api.Layout
	err     error
}

// layoutRuntime is the app's layouts: the parsed files, the names that were
// rejected, and the changes commands made to the shown layout.
type layoutRuntime struct {
	mu       sync.Mutex
	files    map[string]cachedLayout
	warnings []string
	changes  map[string]*layoutChanges
}

// layoutChanges is one layout's node changes, so hiding a node in one
// layout does not hide the same id in another.
type layoutChanges struct {
	hidden map[string]bool
	sizes  map[string]api.LayoutSize
	stacks map[string]string
}

func newLayoutChanges() *layoutChanges {
	return &layoutChanges{
		hidden: make(map[string]bool),
		sizes:  make(map[string]api.LayoutSize),
		stacks: make(map[string]string),
	}
}

var (
	layoutMu       sync.Mutex
	layoutRuntimes = make(map[*App]*layoutRuntime)
)

func layoutRuntimeFor(a *App) *layoutRuntime {
	layoutMu.Lock()
	defer layoutMu.Unlock()
	rt := layoutRuntimes[a]
	if rt == nil {
		rt = &layoutRuntime{
			files:   make(map[string]cachedLayout),
			changes: make(map[string]*layoutChanges),
		}
		layoutRuntimes[a] = rt
	}
	return rt
}

// layoutDir returns the directory holding layouts/, settings.json and
// state.json.
func (a *App) layoutDir() string {
	if a != nil && a.store != nil {
		return a.store.Dir()
	}
	dir, err := store.Dir()
	if err != nil {
		return ""
	}
	return dir
}

// layoutNamesLocked lists the built-in layouts and the valid files in
// layouts/, recording why a file was left out. rt.mu is held.
func (rt *layoutRuntime) layoutNamesLocked(dir string) []string {
	names := api.BuiltinLayoutNames()
	if dir == "" {
		return names
	}
	layoutDir := filepath.Join(dir, "layouts")
	entries, err := os.ReadDir(layoutDir)
	if err != nil {
		return names
	}
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		seen[name] = true
	}
	var extra []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".json")
		if err := api.LayoutNameInvalid(name); err != nil {
			rt.warnLocked(fmt.Sprintf("layouts/%s: %v", e.Name(), err))
			continue
		}
		if seen[name] {
			continue
		}
		if _, err := rt.readLocked(dir, name); err != nil {
			continue
		}
		seen[name] = true
		extra = append(extra, name)
	}
	sort.Strings(extra)
	return append(names, extra...)
}

// readLocked parses one layout file, reusing the parsed tree while the file
// is unchanged. rt.mu is held.
func (rt *layoutRuntime) readLocked(dir, name string) (api.Layout, error) {
	if l, ok := api.BuiltinLayout(name); ok {
		return l, nil
	}
	if err := api.LayoutNameInvalid(name); err != nil {
		return api.Layout{}, err
	}
	path := filepath.Join(dir, "layouts", name+".json")
	info, err := os.Stat(path)
	if err != nil {
		return api.Layout{}, err
	}
	if cached, ok := rt.files[name]; ok && cached.modTime.Equal(info.ModTime()) && cached.size == info.Size() {
		return cached.layout, cached.err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		rt.files[name] = cachedLayout{modTime: info.ModTime(), size: info.Size(), err: err}
		rt.warnLocked(fmt.Sprintf("layouts/%s.json: %v", name, err))
		return api.Layout{}, err
	}
	l, err := api.ParseLayout(data)
	if err != nil {
		err = fmt.Errorf("layouts/%s.json: %w", name, err)
		rt.files[name] = cachedLayout{modTime: info.ModTime(), size: info.Size(), err: err}
		rt.warnLocked(err.Error())
		return api.Layout{}, err
	}
	rt.files[name] = cachedLayout{modTime: info.ModTime(), size: info.Size(), layout: l}
	return l, nil
}

func (rt *layoutRuntime) warnLocked(msg string) {
	for _, old := range rt.warnings {
		if old == msg {
			return
		}
	}
	rt.warnings = append(rt.warnings, msg)
}

// invalidate forgets a parsed file so the next read sees a change.
func (rt *layoutRuntime) invalidate(name string) {
	rt.mu.Lock()
	delete(rt.files, name)
	rt.mu.Unlock()
}

// LayoutNames returns the built-in layouts followed by the user's layouts.
func (a *App) LayoutNames() []string {
	rt := layoutRuntimeFor(a)
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.layoutNamesLocked(a.layoutDir())
}

// LayoutWarnings returns why layout files were left out.
func (a *App) LayoutWarnings() []string {
	rt := layoutRuntimeFor(a)
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return append([]string(nil), rt.warnings...)
}

// Layout returns the layout the shell should draw. An unknown or missing
// name falls back to herdr.
func (a *App) Layout() string {
	a.mu.RLock()
	name := strings.TrimSpace(a.settings.Layout)
	a.mu.RUnlock()
	if name != "" && a.layoutExists(name) {
		return name
	}
	return api.LayoutHerdr
}

// LayoutTree returns a layout by name: a built-in tree, or a validated file.
func (a *App) LayoutTree(name string) (api.Layout, bool) {
	name = strings.TrimSpace(name)
	if l, ok := api.BuiltinLayout(name); ok {
		return l, true
	}
	dir := a.layoutDir()
	if dir == "" {
		return api.Layout{}, false
	}
	rt := layoutRuntimeFor(a)
	rt.mu.Lock()
	defer rt.mu.Unlock()
	l, err := rt.readLocked(dir, name)
	if err != nil {
		return api.Layout{}, false
	}
	return l, true
}

// LayoutState returns a copy of the node changes layout commands made.
func (a *App) LayoutState() api.LayoutState {
	name := a.Layout()
	rt := layoutRuntimeFor(a)
	rt.mu.Lock()
	defer rt.mu.Unlock()
	changes := rt.changes[name]
	if changes == nil {
		return api.LayoutState{}
	}
	return api.LayoutState{
		Hidden: copyHidden(changes.hidden),
		Sizes:  copySizes(changes.sizes),
		Stacks: copyStringMap(changes.stacks),
	}
}

// changesLocked returns the node changes of one layout. rt.mu is held.
func (rt *layoutRuntime) changesLocked(name string) *layoutChanges {
	changes := rt.changes[name]
	if changes == nil {
		changes = newLayoutChanges()
		rt.changes[name] = changes
	}
	return changes
}

func (a *App) layoutExists(name string) bool {
	if _, ok := api.BuiltinLayout(name); ok {
		return true
	}
	dir := a.layoutDir()
	if dir == "" {
		return false
	}
	rt := layoutRuntimeFor(a)
	rt.mu.Lock()
	defer rt.mu.Unlock()
	_, err := rt.readLocked(dir, name)
	return err == nil
}

// setLayout selects a layout and stores it in settings.
func (a *App) setLayout(name string) error {
	name = strings.TrimSpace(name)
	if err := api.LayoutNameInvalid(name); err != nil {
		return fmt.Errorf("layout: %w", err)
	}
	if !a.layoutExists(name) {
		return fmt.Errorf("layout: unknown layout %q; have %s", name, strings.Join(a.LayoutNames(), ", "))
	}
	a.mu.Lock()
	a.settings.Layout = name
	a.mu.Unlock()
	a.persistSettings()
	return nil
}

// findLayoutNode returns the node with id in a layout tree, including
// floating blocks.
func findLayoutNode(n *api.Layout, id string) *api.Layout {
	if n == nil {
		return nil
	}
	if n.ID == id {
		return n
	}
	for i := range n.Children {
		if got := findLayoutNode(&n.Children[i], id); got != nil {
			return got
		}
	}
	return nil
}

func findLayoutFloat(n *api.Layout, id string) *api.LayoutFloat {
	if n == nil {
		return nil
	}
	for i := range n.Floating {
		if n.Floating[i].ID == id {
			return &n.Floating[i]
		}
	}
	for i := range n.Children {
		if got := findLayoutFloat(&n.Children[i], id); got != nil {
			return got
		}
	}
	return nil
}

// currentLayoutTree returns the shown layout's tree.
func (a *App) currentLayoutTree() (api.Layout, error) {
	name := a.Layout()
	l, ok := a.LayoutTree(name)
	if !ok {
		return api.Layout{}, fmt.Errorf("layout: %q is not readable", name)
	}
	return l, nil
}

// --- Commands ---

func init() {
	registerCommand(&cmd{id: "layout.switch", title: "Switch layout", args: argsSpec{
		"name": kindString,
	}, run: runLayoutSwitch})
	registerCommand(&cmd{id: "layout.clone", title: "Duplicate layout", args: argsSpec{
		"name": kindString, "as": kindString,
	}, run: runLayoutClone})
	registerCommand(&cmd{id: "layout.reset", title: "Reset layout", args: argsSpec{
		"name": kindString,
	}, run: runLayoutReset})
	registerCommand(&cmd{id: "layout.toggle", title: "Toggle layout node", args: argsSpec{
		"id": kindString,
	}, run: runLayoutToggle})
	registerCommand(&cmd{id: "layout.show", title: "Show layout node", args: argsSpec{
		"id": kindString,
	}, run: runLayoutShow})
	registerCommand(&cmd{id: "layout.hide", title: "Hide layout node", args: argsSpec{
		"id": kindString,
	}, run: runLayoutHide})
	registerCommand(&cmd{id: "layout.resize", title: "Resize layout node", args: argsSpec{
		"id": kindString, "px": kindFloat, "weight": kindFloat,
	}, run: runLayoutResize})
	registerCommand(&cmd{id: "layout.select", title: "Select layout stack child", args: argsSpec{
		"stack": kindString, "child": kindString,
	}, run: runLayoutSelect})
	registerCommand(&cmd{id: "layout.set", title: "Replace a personal layout", args: argsSpec{
		"layout": kindAny, "name": kindString,
	}, run: runLayoutSet})
}

// runLayoutSet replaces a personal layout tree.
func runLayoutSet(a *App, args map[string]any) error {
	raw, ok := args["layout"]
	if !ok || raw == nil {
		return errors.New("layout.set: layout is required")
	}
	name := strings.TrimSpace(argString(args, "name"))
	if name == "" {
		name = a.Layout()
	}
	if _, builtin := api.BuiltinLayout(name); builtin {
		return fmt.Errorf("layout.set: %q is built in; clone it first", name)
	}
	if err := api.LayoutNameInvalid(name); err != nil {
		return fmt.Errorf("layout.set: %w", err)
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return fmt.Errorf("layout.set: %w", err)
	}
	tree, err := api.ParseLayout(data)
	if err != nil {
		return fmt.Errorf("layout.set: %w", err)
	}
	if err := api.ValidateLayout(&tree); err != nil {
		return fmt.Errorf("layout.set: %w", err)
	}
	dir := a.layoutDir()
	if dir == "" {
		return errors.New("layout.set: no config directory")
	}
	layoutDir := filepath.Join(dir, "layouts")
	if err := os.MkdirAll(layoutDir, 0o700); err != nil {
		return fmt.Errorf("layout.set: %w", err)
	}
	path := filepath.Join(layoutDir, name+".json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("layout.set: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("layout.set: %w", err)
	}
	rt := layoutRuntimeFor(a)
	rt.mu.Lock()
	delete(rt.files, name)
	rt.mu.Unlock()
	return a.setLayout(name)
}

// runLayoutSwitch shows a named layout, or the next one when name is empty.
func runLayoutSwitch(a *App, args map[string]any) error {
	name := strings.TrimSpace(argString(args, "name"))
	if name == "" {
		names := a.LayoutNames()
		current := a.Layout()
		for i, other := range names {
			if other == current {
				name = names[(i+1)%len(names)]
				break
			}
		}
		if name == "" {
			name = api.LayoutHerdr
		}
	}
	return a.setLayout(name)
}

// runLayoutClone copies a layout to a new file and shows it.
func runLayoutClone(a *App, args map[string]any) error {
	name := strings.TrimSpace(argString(args, "name"))
	if name == "" {
		return errors.New("layout.clone: name is required")
	}
	as := strings.TrimSpace(argString(args, "as"))
	if as == "" {
		as = name + "-copy"
	}
	if err := api.LayoutNameInvalid(as); err != nil {
		return fmt.Errorf("layout.clone: %w", err)
	}
	if a.layoutExists(as) {
		return fmt.Errorf("layout.clone: %q already exists", as)
	}
	tree, ok := a.LayoutTree(name)
	if !ok {
		return fmt.Errorf("layout.clone: unknown layout %q", name)
	}
	dir := a.layoutDir()
	if dir == "" {
		return errors.New("layout.clone: no config directory")
	}
	layoutDir := filepath.Join(dir, "layouts")
	if err := os.MkdirAll(layoutDir, 0o700); err != nil {
		return fmt.Errorf("layout.clone: %w", err)
	}
	data, err := json.MarshalIndent(tree, "", "  ")
	if err != nil {
		return fmt.Errorf("layout.clone: %w", err)
	}
	path := filepath.Join(layoutDir, as+".json")
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("layout.clone: %w", err)
	}
	rt := layoutRuntimeFor(a)
	rt.mu.Lock()
	delete(rt.files, as)
	if source := rt.changes[name]; source != nil {
		clone := newLayoutChanges()
		clone.hidden = copyHidden(source.hidden)
		clone.sizes = copySizes(source.sizes)
		clone.stacks = copyStringMap(source.stacks)
		rt.changes[as] = clone
	}
	rt.mu.Unlock()
	return a.setLayout(as)
}

// runLayoutReset clears the node changes of a layout and reloads its file.
func runLayoutReset(a *App, args map[string]any) error {
	name := strings.TrimSpace(argString(args, "name"))
	if name == "" {
		name = a.Layout()
	}
	if !a.layoutExists(name) {
		return fmt.Errorf("layout.reset: unknown layout %q", name)
	}
	rt := layoutRuntimeFor(a)
	rt.mu.Lock()
	delete(rt.changes, name)
	delete(rt.files, name)
	rt.mu.Unlock()
	return nil
}

// runLayoutToggle hides a visible node, or shows a hidden one.
func runLayoutToggle(a *App, args map[string]any) error {
	id := strings.TrimSpace(argString(args, "id"))
	if id == "" {
		return errors.New("layout.toggle: id is required")
	}
	tree, err := a.currentLayoutTree()
	if err != nil {
		return err
	}
	node := findLayoutNode(&tree, id)
	floating := findLayoutFloat(&tree, id)
	if node == nil && floating == nil {
		return fmt.Errorf("layout.toggle: no node %q in layout %q", id, a.Layout())
	}
	name := a.Layout()
	rt := layoutRuntimeFor(a)
	rt.mu.Lock()
	defer rt.mu.Unlock()
	changes := rt.changesLocked(name)
	hidden, has := changes.hidden[id]
	if !has {
		if node != nil {
			hidden = node.Hidden
		}
	}
	changes.hidden[id] = !hidden
	return nil
}

// runLayoutShow shows a layout node.
func runLayoutShow(a *App, args map[string]any) error {
	return setLayoutHidden(a, argString(args, "id"), false, "layout.show")
}

// runLayoutHide hides a layout node.
func runLayoutHide(a *App, args map[string]any) error {
	return setLayoutHidden(a, argString(args, "id"), true, "layout.hide")
}

func setLayoutHidden(a *App, id string, hidden bool, command string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("%s: id is required", command)
	}
	if err := checkLayoutNode(a, id, command); err != nil {
		return err
	}
	name := a.Layout()
	rt := layoutRuntimeFor(a)
	rt.mu.Lock()
	rt.changesLocked(name).hidden[id] = hidden
	rt.mu.Unlock()
	return nil
}

// runLayoutResize gives a layout node a fixed px or a weight.
func runLayoutResize(a *App, args map[string]any) error {
	id := strings.TrimSpace(argString(args, "id"))
	if id == "" {
		return errors.New("layout.resize: id is required")
	}
	if err := checkLayoutNode(a, id, "layout.resize"); err != nil {
		return err
	}
	px, hasPx := argFloat(args, "px")
	weight, hasWeight := argFloat(args, "weight")
	switch {
	case hasPx && hasWeight:
		return errors.New("layout.resize: give px or weight, not both")
	case !hasPx && !hasWeight:
		return errors.New("layout.resize: give px or weight")
	case hasPx && px < 0:
		return errors.New("layout.resize: px must not be negative")
	case hasWeight && weight <= 0:
		return errors.New("layout.resize: weight must be greater than zero")
	}
	size := api.LayoutSize{}
	if hasPx {
		v := float32(px)
		size.Px = new(v)
	} else {
		v := float32(weight)
		size.Weight = new(v)
	}
	name := a.Layout()
	rt := layoutRuntimeFor(a)
	rt.mu.Lock()
	rt.changesLocked(name).sizes[id] = size
	rt.mu.Unlock()
	return nil
}

// runLayoutSelect makes a stack show one of its children.
func runLayoutSelect(a *App, args map[string]any) error {
	stack := strings.TrimSpace(argString(args, "stack"))
	child := strings.TrimSpace(argString(args, "child"))
	if stack == "" || child == "" {
		return errors.New("layout.select: stack and child are required")
	}
	tree, err := a.currentLayoutTree()
	if err != nil {
		return err
	}
	node := findLayoutNode(&tree, stack)
	if node == nil {
		return fmt.Errorf("layout.select: no node %q in layout %q", stack, a.Layout())
	}
	if node.Type != "stack" {
		return fmt.Errorf("layout.select: %q is a %s, not a stack", stack, node.Type)
	}
	for i := range node.Children {
		if node.Children[i].ID == child {
			name := a.Layout()
			rt := layoutRuntimeFor(a)
			rt.mu.Lock()
			rt.changesLocked(name).stacks[stack] = child
			rt.mu.Unlock()
			return nil
		}
	}
	return fmt.Errorf("layout.select: %q is not a child of stack %q", child, stack)
}

func checkLayoutNode(a *App, id, command string) error {
	tree, err := a.currentLayoutTree()
	if err != nil {
		return err
	}
	if findLayoutNode(&tree, id) == nil && findLayoutFloat(&tree, id) == nil {
		return fmt.Errorf("%s: no node %q in layout %q", command, id, a.Layout())
	}
	return nil
}

func copyHidden(in map[string]bool) map[string]bool {
	out := make(map[string]bool, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func copySizes(in map[string]api.LayoutSize) map[string]api.LayoutSize {
	out := make(map[string]api.LayoutSize, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func copyStringMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
