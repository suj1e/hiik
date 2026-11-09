package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"hiik/mods/api"
)

// programEntry is one row of the program table while it is merged. The
// shipped table is data: apps/desktop/programs.json, embedded by the app.
type programEntry struct {
	Key   string
	Name  string
	Run   []string
	Icon  string
	Tint  bool
	Tile  string
	Color string
	Match []string
}

// programPatch is one row from a person's programs.json. Pointers and nil
// slices tell an absent field from an explicit empty one, so a patch changes
// only what it names.
type programPatch struct {
	Name  *string   `json:"name"`
	Run   *[]string `json:"run"`
	Icon  *string   `json:"icon"`
	Tint  *bool     `json:"tint"`
	Tile  *string   `json:"tile"`
	Color *string   `json:"color"`
	Match *[]string `json:"match"`
}

// namedProgramPatch is one patch with the key that named it.
type namedProgramPatch struct {
	key   string
	patch programPatch
}

// bundledProgramsJSON is the shipped table, set by the app package's embed.
var bundledProgramsState struct {
	sync.Mutex
	data   []byte
	loaded bool
	list   []programEntry
}

// SetBundledPrograms installs the shipped program table. The app calls it
// from init with its embedded programs.json; tests and embedders may leave it
// unset and the table is read from the module's copy.
func SetBundledPrograms(data []byte) {
	bundledProgramsState.Lock()
	bundledProgramsState.data = append([]byte(nil), data...)
	bundledProgramsState.loaded = false
	bundledProgramsState.list = nil
	bundledProgramsState.Unlock()

	programsState.Lock()
	programsState.loaded = false
	programsState.Unlock()
}

// builtinPrograms returns the shipped table, in its file order.
func builtinPrograms() []programEntry {
	bundledProgramsState.Lock()
	defer bundledProgramsState.Unlock()
	if !bundledProgramsState.loaded {
		data := bundledProgramsState.data
		if len(bytes.TrimSpace(data)) == 0 {
			data = shippedProgramsFile()
		}
		list, _ := parseBuiltinPrograms(data)
		bundledProgramsState.list = list
		bundledProgramsState.loaded = true
	}
	return cloneProgramEntries(bundledProgramsState.list)
}

// shippedProgramsFile reads apps/desktop/programs.json when the app embed is
// not installed: `go test` and tools use the source tree.
func shippedProgramsFile() []byte {
	var candidates []string
	if _, file, _, ok := runtime.Caller(0); ok {
		candidates = append(candidates, filepath.Join(filepath.Dir(file), "..", "programs.json"))
	}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(wd, "programs.json"),
			filepath.Join(wd, "..", "programs.json"))
	}
	for _, path := range candidates {
		if data, err := os.ReadFile(path); err == nil {
			return data
		}
	}
	return nil
}

// Programs returns the agents the new-tab menu offers, with the last
// installed check. A person's $HIIK_HOME/programs.json is laid over the
// shipped table field by field.
//
// An entry without a command is a known program, not a startable one: it
// only gives an agent a name and an icon, so the menu leaves it out.
func (a *App) Programs() []api.Program {
	all := a.programSnapshot()
	out := make([]api.Program, 0, len(all))
	for _, p := range all {
		if len(p.Run) == 0 {
			continue
		}
		out = append(out, p)
	}
	return out
}

// ProgramTable returns every known program, including the detection-only
// rows that have no command. Agent labels and icons read this table.
func (a *App) ProgramTable() []api.Program {
	return a.programSnapshot()
}

func (a *App) programSnapshot() []api.Program {
	programsState.Lock()
	defer programsState.Unlock()
	ensureProgramsLocked(a.configDir())
	return clonePrograms(programListLocked())
}

// RefreshPrograms checks which agents this machine can start. It asks the
// user's login shell, as an app opened from the Dock does not inherit the
// shell's PATH, then falls back to this process's PATH.
func (a *App) RefreshPrograms(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}

	programsState.Lock()
	ensureProgramsLocked(a.configDir())
	names := make([]string, 0, len(programsState.entries))
	for _, p := range programsState.entries {
		if len(p.Run) > 0 {
			names = append(names, p.Run[0])
		}
	}
	programsState.Unlock()

	found := shellCommands(ctx, names)

	programsState.Lock()
	defer programsState.Unlock()
	// The table may have changed while the shell ran; mark installed on
	// whatever rows are there now.
	for i := range programsState.entries {
		p := &programsState.entries[i]
		if len(p.Run) == 0 {
			continue
		}
		_, err := exec.LookPath(p.Run[0])
		p.Installed = found[p.Run[0]] || err == nil
	}
	programsState.list = nil
}

var programsState struct {
	sync.Mutex
	loaded  bool
	path    string
	modTime int64
	size    int64
	entries []programEntryWithInstalled
	list    []api.Program
}

// programEntryWithInstalled is one merged entry and the machine check.
type programEntryWithInstalled struct {
	programEntry
	Installed bool
}

// ensureProgramsLocked merges the shipped table and the person's file when
// the file changed since the last look. The caller holds programsState.
func ensureProgramsLocked(dir string) {
	path := ""
	if dir != "" {
		path = filepath.Join(dir, "programs.json")
	}
	info, statErr := os.Stat(path)
	var modTime, size int64
	if statErr == nil {
		modTime = info.ModTime().UnixNano()
		size = info.Size()
	}
	if programsState.loaded && programsState.path == path &&
		programsState.modTime == modTime && programsState.size == size {
		return
	}

	entries := builtinPrograms()
	if statErr == nil {
		if data, err := os.ReadFile(path); err == nil {
			if patches, err := parseProgramPatches(data); err == nil {
				entries = mergePrograms(entries, patches)
			}
		}
	}

	programsState.entries = make([]programEntryWithInstalled, len(entries))
	for i, entry := range entries {
		programsState.entries[i] = programEntryWithInstalled{programEntry: entry}
	}
	programsState.list = nil
	programsState.loaded = true
	programsState.path = path
	programsState.modTime = modTime
	programsState.size = size
}

// programListLocked turns merged entries into the api snapshot.
func programListLocked() []api.Program {
	if programsState.list != nil {
		return programsState.list
	}
	out := make([]api.Program, 0, len(programsState.entries))
	for _, entry := range programsState.entries {
		out = append(out, api.Program{
			Key:       entry.Key,
			Name:      entry.Name,
			Run:       append([]string(nil), entry.Run...),
			Installed: entry.Installed,
			Icon:      entry.Icon,
			Tint:      entry.Tint,
			Tile:      entry.Tile,
			Color:     entry.Color,
			Match:     append([]string(nil), entry.Match...),
		})
	}
	programsState.list = out
	return out
}

func clonePrograms(list []api.Program) []api.Program {
	if len(list) == 0 {
		return nil
	}
	out := make([]api.Program, len(list))
	for i, p := range list {
		p.Run = append([]string(nil), p.Run...)
		p.Match = append([]string(nil), p.Match...)
		out[i] = p
	}
	return out
}

func cloneProgramEntries(list []programEntry) []programEntry {
	if len(list) == 0 {
		return nil
	}
	out := make([]programEntry, len(list))
	for i, p := range list {
		p.Run = append([]string(nil), p.Run...)
		p.Match = append([]string(nil), p.Match...)
		out[i] = p
	}
	return out
}

// parseBuiltinPrograms reads the shipped table, keeping the file's order.
func parseBuiltinPrograms(data []byte) ([]programEntry, error) {
	patches, err := parseProgramPatches(data)
	if err != nil {
		return nil, err
	}
	out := make([]programEntry, 0, len(patches))
	for _, named := range patches {
		entry := programEntry{Key: named.key}
		applyProgramPatch(&entry, named.patch)
		out = append(out, entry)
	}
	return out, nil
}

// parseProgramPatches reads {"programs": {key: {...}}}, or a flat map for a
// hand-written file, in the order the file lists the keys.
func parseProgramPatches(data []byte) ([]namedProgramPatch, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, nil
	}
	raw := data
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if section, ok := doc["programs"]; ok {
		raw = section
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return nil, errors.New("programs.json: expected an object")
	}
	var out []namedProgramPatch
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil, errors.New("programs.json: expected a program key")
		}
		var patch programPatch
		if err := dec.Decode(&patch); err != nil {
			return nil, err
		}
		out = append(out, namedProgramPatch{key: key, patch: patch})
	}
	return out, nil
}

// mergePrograms lays patches over the shipped rows by key; a new key is
// appended, so programs.json can add an agent as well as change one.
func mergePrograms(base []programEntry, patches []namedProgramPatch) []programEntry {
	out := cloneProgramEntries(base)
	index := make(map[string]int, len(out))
	for i, entry := range out {
		index[entry.Key] = i
	}
	for _, named := range patches {
		if i, ok := index[named.key]; ok {
			applyProgramPatch(&out[i], named.patch)
			continue
		}
		entry := programEntry{Key: named.key}
		applyProgramPatch(&entry, named.patch)
		index[named.key] = len(out)
		out = append(out, entry)
	}
	return out
}

// applyProgramPatch changes only the fields the patch names.
func applyProgramPatch(entry *programEntry, patch programPatch) {
	if patch.Name != nil {
		entry.Name = *patch.Name
	}
	if patch.Run != nil {
		entry.Run = append([]string(nil), (*patch.Run)...)
	}
	if patch.Icon != nil {
		entry.Icon = *patch.Icon
	}
	if patch.Tint != nil {
		entry.Tint = *patch.Tint
	}
	if patch.Tile != nil {
		entry.Tile = *patch.Tile
	}
	if patch.Color != nil {
		entry.Color = *patch.Color
	}
	if patch.Match != nil {
		entry.Match = append([]string(nil), (*patch.Match)...)
	}
}

// shellCommands asks the login shell which of names it can run. It uses
// marked lines so a startup file's output cannot be mistaken for an answer.
func shellCommands(ctx context.Context, names []string) map[string]bool {
	out := make(map[string]bool, len(names))
	if len(names) == 0 {
		return out
	}
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	quoted := make([]string, len(names))
	for i, name := range names {
		quoted[i] = "'" + strings.ReplaceAll(name, "'", `'\''`) + "'"
	}
	script := "for c in " + strings.Join(quoted, " ") + `; do command -v "$c" >/dev/null 2>&1 && printf 'hiik-has:%s\n' "$c"; done`
	cmd := exec.CommandContext(ctx, shell, "-ic", script)
	body, err := cmd.Output()
	if err != nil && len(body) == 0 {
		return out
	}
	for _, line := range strings.Split(string(body), "\n") {
		if name, ok := strings.CutPrefix(strings.TrimSpace(line), "hiik-has:"); ok && name != "" {
			out[name] = true
		}
	}
	return out
}
