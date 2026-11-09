package builtin

import (
	"reflect"
	"testing"

	"github.com/egoist/mygo/ui"

	"hiik/mods/api"
)

func TestSpaceListEntriesGroupMachines(t *testing.T) {
	rows := []spaceRow{
		{Space: api.Space{ID: "l1", Name: "Local"}},
		{Space: api.Space{ID: "r1", Name: "Remote", Machine: "builder"}},
	}
	groups, grouped := spaceMachineGroups(rows, []api.Machine{{Name: "builder", State: "ready"}})
	if !grouped {
		t.Fatal("a remote machine must group rows")
	}
	entries := spaceListEntries(rows, groups, grouped)
	want := []spaceListEntry{
		{Group: 0, Row: -1},
		{Group: -1, Row: 0},
		{Group: 1, Row: -1},
		{Group: -1, Row: 1},
	}
	if !reflect.DeepEqual(entries, want) {
		t.Fatalf("entries = %#v, want %#v", entries, want)
	}
	flat := spaceListEntries(rows, nil, false)
	if len(flat) != 2 || flat[0].Row != 0 || flat[1].Row != 1 {
		t.Fatalf("flat entries = %#v", flat)
	}
}

func TestSpaceReorderTargetSkipsHeaders(t *testing.T) {
	rows := []spaceRow{
		{Space: api.Space{ID: "a"}, Index: 0},
		{Space: api.Space{ID: "b"}, Index: 1},
		{Space: api.Space{ID: "c"}, Index: 2},
	}
	entries := spaceListEntries(rows, nil, false)
	cases := []struct {
		from, to, want int
	}{
		{0, 2, 1}, // before c, after b
		{2, 0, 0}, // before a
		{0, 3, 2}, // past the last row
		{1, 1, 1}, // where it is
	}
	for _, tc := range cases {
		if got := spaceReorderTarget(entries, rows, tc.from, tc.to, len(rows)); got != tc.want {
			t.Errorf("reorder(%d -> %d) = %d, want %d", tc.from, tc.to, got, tc.want)
		}
	}
	grouped := []spaceListEntry{
		{Group: 0, Row: -1},
		{Group: -1, Row: 0},
		{Group: -1, Row: 1},
		{Group: 1, Row: -1},
		{Group: -1, Row: 2},
	}
	if got := spaceReorderTarget(grouped, rows, 1, 0, len(rows)); got != 0 {
		t.Errorf("reorder across a header = %d, want 0", got)
	}
	if got := spaceReorderTarget(grouped, rows, 1, 5, len(rows)); got != 2 {
		t.Errorf("reorder past the end = %d, want 2", got)
	}
}

func TestParseSpaceGitStatus(t *testing.T) {
	out := "# branch.oid abc\n# branch.head spike\n# branch.upstream origin/spike\n# branch.ab +12 -3\n"
	branch, ahead, behind := parseSpaceGitStatus(out)
	if branch != "spike" || ahead != 12 || behind != 3 {
		t.Fatalf("status = %q %d %d, want spike 12 3", branch, ahead, behind)
	}
	branch, ahead, behind = parseSpaceGitStatus("# branch.head (detached)\n")
	if branch != "" || ahead != 0 || behind != 0 {
		t.Fatalf("detached status = %q %d %d, want empty", branch, ahead, behind)
	}
}

func TestSpaceGitTailParts(t *testing.T) {
	cases := []struct {
		name string
		info spaceGitInfo
		tree bool
		want []string
	}{
		{
			name: "api",
			info: spaceGitInfo{OK: true, Repo: "hiik", Branch: "feature/x", Ahead: 2, Behind: 1},
			want: []string{"hiik · feature/x", "↑2", "↓1"},
		},
		{
			name: "api",
			info: spaceGitInfo{OK: true, Repo: "hiik", Branch: "api"},
			want: []string{"hiik"},
		},
		{
			name: "api",
			info: spaceGitInfo{OK: true, Repo: "hiik", Branch: "main"},
			want: []string{"hiik"},
		},
		{
			name: "feature/x",
			info: spaceGitInfo{OK: true, Repo: "hiik", Branch: "feature/x"},
			want: []string{"hiik"},
		},
		{
			name: "feature/x",
			info: spaceGitInfo{OK: true, Repo: "hiik", Branch: "feature/x", Ahead: 1},
			tree: true,
			want: []string{"↑1"},
		},
		{
			name: "repo",
			info: spaceGitInfo{OK: true, Repo: "hiik", Branch: "feature/x"},
			tree: true,
			want: []string{"feature/x"},
		},
		{
			name: "api",
			info: spaceGitInfo{},
			want: nil,
		},
	}
	for _, tc := range cases {
		got := spaceGitTailParts(tc.info, tc.name, tc.tree)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("spaceGitTailParts(%+v, %q, %v) = %v, want %v", tc.info, tc.name, tc.tree, got, tc.want)
		}
	}
}

func TestParseSpaceWorktrees(t *testing.T) {
	out := "worktree /repo\nHEAD abc\nbranch refs/heads/main\n\n" +
		"worktree /repo/wt\nHEAD def\nbranch refs/heads/spike\n\n"
	got := parseSpaceWorktrees(out)
	want := []string{"/repo", "/repo/wt"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("worktrees = %v, want %v", got, want)
	}
}

func TestSpaceMachineGroups(t *testing.T) {
	rows := []spaceRow{
		{Space: api.Space{ID: "l1", Name: "Local"}},
		{Space: api.Space{ID: "r1", Name: "Remote", Machine: "builder"}},
	}
	groups, grouped := spaceMachineGroups(rows, []api.Machine{{Name: "builder", State: "ready"}})
	if !grouped {
		t.Fatal("a remote machine must group rows")
	}
	if len(groups) != 2 {
		t.Fatalf("groups = %d, want 2", len(groups))
	}
	if groups[0].Machine != "" || groups[0].Label != "This computer" {
		t.Fatalf("local group = %#v", groups[0])
	}
	if len(groups[0].Spaces) != 1 || groups[0].Spaces[0].Space.ID != "l1" {
		t.Fatalf("local spaces = %#v", groups[0].Spaces)
	}
	if groups[1].Machine != "builder" || groups[1].Label != "builder" || groups[1].State != "ready" {
		t.Fatalf("remote group = %#v", groups[1])
	}
	if len(groups[1].Spaces) != 1 || groups[1].Spaces[0].Space.ID != "r1" {
		t.Fatalf("remote spaces = %#v", groups[1].Spaces)
	}
}

func TestSpaceMachineGroupsWithoutRemoteMachines(t *testing.T) {
	rows := []spaceRow{{Space: api.Space{ID: "l1", Name: "Local"}}}
	groups, grouped := spaceMachineGroups(rows, nil)
	if grouped || groups != nil {
		t.Fatalf("one local machine should not group: %v %v", groups, grouped)
	}
}

func TestSpaceRowsNestWorktrees(t *testing.T) {
	rows := []spaceRow{
		{
			Space: api.Space{ID: "wt", Name: "spike"},
			Index: 0,
			Git:   spaceGitInfo{OK: true, Repo: "repo", Linked: true, Root: "/repo/wt"},
		},
		{
			Space: api.Space{ID: "main", Name: "repo"},
			Index: 1,
			Git:   spaceGitInfo{OK: true, Repo: "repo", Root: "/repo"},
		},
		{
			Space: api.Space{ID: "other", Name: "other"},
			Index: 2,
			Git:   spaceGitInfo{OK: true, Repo: "other", Root: "/other"},
		},
	}
	got := spaceRowsNest(rows)
	if len(got) != 3 {
		t.Fatalf("rows = %d, want 3", len(got))
	}
	if got[0].Space.ID != "main" || got[1].Space.ID != "wt" || got[2].Space.ID != "other" {
		t.Fatalf("order = %v", spaceRowIDs(got))
	}
	if got[1].Tree != "└─" {
		t.Fatalf("worktree prefix = %q, want └─", got[1].Tree)
	}
	if got[2].Tree != "" {
		t.Fatalf("unrelated row prefix = %q, want none", got[2].Tree)
	}
}

func TestSpaceRowsDoNotNestAcrossMachines(t *testing.T) {
	rows := []spaceRow{
		{
			Space: api.Space{ID: "wt", Name: "spike", Machine: "builder"},
			Index: 0,
			Git:   spaceGitInfo{OK: true, Repo: "repo", Linked: true, Root: "/repo/wt"},
		},
		{
			Space: api.Space{ID: "main", Name: "repo"},
			Index: 1,
			Git:   spaceGitInfo{OK: true, Repo: "repo", Root: "/repo"},
		},
	}
	got := spaceRowsNest(rows)
	if len(got) != 2 || got[0].Space.ID != "wt" || got[1].Space.ID != "main" {
		t.Fatalf("order = %v, want both flat", spaceRowIDs(got))
	}
	for _, row := range got {
		if row.Tree != "" {
			t.Fatalf("row %s got tree prefix %q", row.Space.ID, row.Tree)
		}
	}
}

type spacesTestApp struct {
	btPanelApp
	exec func(on, command string, args ...string) (string, string, int, error)
}

func (a *spacesTestApp) Exec(on, command string, args ...string) (string, string, int, error) {
	if a.exec != nil {
		return a.exec(on, command, args...)
	}
	return "", "", 0, nil
}

func TestSpaceGitCacheAsksOncePerFolder(t *testing.T) {
	calls := 0
	app := &spacesTestApp{exec: func(on, command string, args ...string) (string, string, int, error) {
		calls++
		return "", "not a repository", 1, nil
	}}
	cache := newSpaceGitCache()
	cache.info(app, "", "/tmp/one")
	cache.info(app, "", "/tmp/one")
	if calls != 1 {
		t.Fatalf("git ran %d times for one folder, want 1", calls)
	}
	cache.info(app, "builder", "/tmp/one")
	if calls != 2 {
		t.Fatalf("git ran %d times for a second machine, want 2", calls)
	}
}

func TestSpacesPanelGroupsMachinesAndShowsGitTail(t *testing.T) {
	mainDir := t.TempDir()
	app := &spacesTestApp{
		btPanelApp: btPanelApp{
			spaces: []api.Space{
				{ID: "s1", Name: "api", Folder: mainDir},
				{ID: "s2", Name: "worker", Folder: "/remote/work", Machine: "builder"},
			},
			machines: []api.Machine{{Name: "builder", State: "ready"}},
			active:   "s1",
		},
		exec: func(on, command string, args ...string) (string, string, int, error) {
			if on != "" {
				return "", "unavailable", 1, nil
			}
			for _, arg := range args {
				switch arg {
				case "status":
					return "# branch.head spike\n# branch.ab +2 -1\n", "", 0, nil
				case "rev-parse":
					return mainDir + "\n", "", 0, nil
				case "worktree":
					return "worktree " + mainDir + "\nHEAD abc\nbranch refs/heads/spike\n\n", "", 0, nil
				}
			}
			return "", "", 1, nil
		},
	}
	p := api.Panels()[api.PanelSpaces]
	if p.Render == nil {
		t.Fatalf("panel %s is not registered", api.PanelSpaces)
	}
	tt := ui.NewTester(func(c *ui.Context) {
		p.Render(&api.RenderContext{C: c, App: app})
	}, 900, 600)
	tt.Frame()
	for _, want := range []string{"Spaces", "api", "This computer", "builder", "worker", "spike", "↑2", "↓1"} {
		if !tt.HasText(want) {
			t.Errorf("spaces panel did not show %q; texts: %v", want, tt.Texts())
		}
	}
	if tt.HasText("local") {
		t.Error("spaces panel still shows a local machine badge")
	}
}

func TestSpacesPanelCompactRail(t *testing.T) {
	app := &spacesTestApp{
		btPanelApp: btPanelApp{
			spaces: []api.Space{
				{ID: "s1", Name: "api", Folder: "/tmp/api"},
				{ID: "s2", Name: "web", Folder: "/tmp/web"},
			},
			active: "s1",
		},
		exec: func(on, command string, args ...string) (string, string, int, error) {
			return "", "", 1, nil
		},
	}
	p := api.Panels()[api.PanelSpaces]
	if p.Render == nil {
		t.Fatalf("panel %s is not registered", api.PanelSpaces)
	}
	tt := ui.NewTester(func(c *ui.Context) {
		p.Render(&api.RenderContext{C: c, App: app, Props: map[string]any{"compact": true}})
	}, 120, 600)
	tt.Frame()
	for _, want := range []string{"1", "2"} {
		if !tt.HasText(want) {
			t.Errorf("compact rail did not show %q; texts: %v", want, tt.Texts())
		}
	}
	if tt.HasText("Spaces") {
		t.Error("compact rail still shows the header title")
	}
}
