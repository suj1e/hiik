package builtin

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"

	"hiik/mods/api"
)

func init() {
	api.Register(api.Mod{
		ID:   "panes",
		Name: "Panes",
		Panels: []api.Panel{{
			ID:     api.PanelPaneHead,
			Title:  "Pane",
			Render: renderPaneHead,
		}},
	})
}

// sideProgramSource is the optional part of an app that carries the program
// table. The core app implements it; a small test app need not. The table
// includes detection-only rows, which a new-tab menu leaves out.
type sideProgramSource interface {
	ProgramTable() []api.Program
}

// sideStartableSource is the older, startable-only view of the same table.
type sideStartableSource interface {
	Programs() []api.Program
}

// sidePrograms reads the widest program table the app offers.
func sidePrograms(app api.App) []api.Program {
	if source, ok := app.(sideProgramSource); ok {
		return source.ProgramTable()
	}
	if source, ok := app.(sideStartableSource); ok {
		return source.Programs()
	}
	return nil
}

// renderPaneHead draws one pane's header. The host passes the terminal
// address in props{"terminal"}; title and cwd are optional props, and the
// agent snapshot fills them in when it can.
func renderPaneHead(rc *api.RenderContext) {
	if rc == nil || rc.C == nil || rc.App == nil {
		return
	}
	t := rc.C.Theme()
	terminal := sideStr(rc.Props, "terminal")
	if terminal == "" {
		terminal = sideStr(rc.Props, "address")
	}
	if terminal == "" {
		if n, ok := rc.Props["node"].(*api.Node); ok && n != nil {
			terminal = n.Terminal
		}
	}
	agent, known := sideFindAgent(rc.App, terminal)
	program, hasProgram := sideProgramFor(rc.App, agent, known)
	pane := sideStr(rc.Props, "pane")
	if pane == "" {
		pane = sideStr(rc.Props, "id")
	}

	title := sideStr(rc.Props, "title")
	app := ""
	cwd := sideStr(rc.Props, "cwd")
	if known {
		if title == "" {
			title = agent.Title
		}
		app = agent.App
		if hasProgram && program.Name != "" {
			app = program.Name
		}
		if cwd == "" {
			cwd = sideSpaceFolder(rc.App, agent.Space)
		}
	}
	if title == "" && known {
		title = sideAgentLabel(agent)
	}
	if title == "" {
		title = terminal
	}
	if title == "" {
		title = "Terminal"
	}

	ui.Row(rc.C).
		FillWidth().
		Gap(8).
		AlignItems(ui.Center).
		Children(func() {
			if known {
				sideProgramBadge(rc.C, t, program, hasProgram, agent.Status)
			}
			ui.Text(rc.C, title).FontSize(t.FontSize - 1).Bold().SingleLine()
			if known && app != "" && app != title {
				ui.Text(rc.C, app).
					FontSize(t.FontSize - 2).
					TextColor(t.TextMuted).
					SingleLine()
			}
			if cwd != "" {
				ui.Text(rc.C, cwd).
					FontSize(t.FontSize - 2).
					TextColor(t.TextMuted).
					SingleLine().
					Shrink(1)
			}
			ui.Box(rc.C).Grow(1)
			ui.ButtonBase(rc.C).
				Key("mods/panes/split/"+terminal).
				Height(20).
				Padding(0, 6).
				Radius(t.Radius).
				Label("Split pane").
				Tooltip("Split pane").
				OnClick(func() {
					args := map[string]any{"direction": "right"}
					if pane != "" {
						args["pane"] = pane
					}
					_ = rc.App.Run(api.CmdPaneSplit, args)
				}).
				Children(func() {
					ui.Text(rc.C, "Split").FontSize(t.FontSize - 2).TextColor(t.TextMuted)
				})
			ui.ButtonBase(rc.C).
				Key("mods/panes/close/"+terminal).
				Height(20).
				Padding(0, 6).
				Radius(t.Radius).
				Label("Close pane").
				Tooltip("Close pane").
				OnClick(func() {
					var args map[string]any
					if pane != "" {
						args = map[string]any{"pane": pane}
					}
					_ = rc.App.Run(api.CmdPaneClose, args)
				}).
				Children(func() {
					ui.Text(rc.C, "Close").FontSize(t.FontSize - 2).TextColor(t.TextMuted)
				})
		})
}

// sideProgramFor finds the program row for a known agent, by its core app
// name or one of the row's other match names.
func sideProgramFor(app api.App, agent api.Agent, known bool) (api.Program, bool) {
	if !known || app == nil {
		return api.Program{}, false
	}
	table := sidePrograms(app)
	if len(table) == 0 {
		return api.Program{}, false
	}
	name := strings.ToLower(strings.TrimSpace(agent.App))
	if name == "" {
		return api.Program{}, false
	}
	for _, program := range table {
		if strings.EqualFold(program.Key, name) ||
			strings.EqualFold(program.Name, name) {
			return program, true
		}
		for _, match := range program.Match {
			if strings.EqualFold(match, name) {
				return program, true
			}
		}
		if len(program.Run) > 0 && strings.EqualFold(program.Run[0], name) {
			return program, true
		}
	}
	return api.Program{}, false
}

// sideProgramBadge draws the program's tile and icon, or the agent's status
// dot when the table has no icon for it.
func sideProgramBadge(c *ui.Context, t *ui.Theme, program api.Program, hasProgram bool, status string) {
	svg := (*ui.SVG)(nil)
	if hasProgram {
		svg = sideIconSVG(program.Icon)
	}
	if svg == nil {
		ui.Box(c).
			Size(6, 6).
			Radius(3).
			Background(sideStatusColor(t, status))
		return
	}

	box := ui.Box(c).Size(18, 18).Radius(4).Padding(3)
	if tile, ok := sideHex(program.Tile); ok {
		box.Background(tile)
	} else {
		box.Background(t.Surface)
	}
	if program.Tint {
		color := t.Text
		if c, ok := sideHex(program.Color); ok {
			color = c
		}
		box.TextColor(color)
	}
	box.Children(func() {
		if program.Tint {
			ui.Icon(c, svg).Size(12, 12)
			return
		}
		ui.Image(c, svg).Size(12, 12)
	})
}

// sideHex parses a "#rrggbb" color from the program table, rejecting a bad
// value instead of letting it panic a frame.
func sideHex(value string) (color ui.Color, ok bool) {
	if strings.TrimSpace(value) == "" {
		return ui.Transparent, false
	}
	defer func() {
		if recover() != nil {
			color, ok = ui.Transparent, false
		}
	}()
	return ui.Hex(value), true
}

// sideIconSVG loads a program icon once and keeps the parsed SVG. A missing
// or malformed file draws the status dot instead.
var sideIconSVG = func() func(string) *ui.SVG {
	var mu sync.Mutex
	cache := map[string]*ui.SVG{}
	failed := map[string]bool{}
	return func(name string) *ui.SVG {
		path := sideIconPath(name)
		if path == "" {
			return nil
		}
		mu.Lock()
		defer mu.Unlock()
		if svg, ok := cache[path]; ok {
			return svg
		}
		if failed[path] {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			failed[path] = true
			return nil
		}
		svg, err := ui.ParseSVG(data)
		if err != nil {
			failed[path] = true
			return nil
		}
		cache[path] = svg
		return svg
	}
}()

// sideIconPath finds the icon file for a table entry: an absolute path, the
// person's HIIK_HOME, the app's resources, or the source tree while testing.
func sideIconPath(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if filepath.IsAbs(name) {
		if sideIsFile(name) {
			return name
		}
		return ""
	}
	for _, root := range sideResourceRoots() {
		if root == "" {
			continue
		}
		path := filepath.Join(root, name)
		if sideIsFile(path) {
			return path
		}
	}
	return ""
}

// sideResourceRoots lists where a resource-relative icon may live, best
// first. The shipped icons are in apps/desktop/resources/icons.
func sideResourceRoots() []string {
	var roots []string
	if home := os.Getenv("HIIK_HOME"); home != "" {
		roots = append(roots, home)
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		roots = append(roots, dir, filepath.Join(dir, "..", "Resources"))
	}
	if wd, err := os.Getwd(); err == nil {
		roots = append(roots, filepath.Join(wd, "resources"), wd)
	}
	if _, file, _, ok := runtime.Caller(0); ok {
		roots = append(roots, filepath.Join(filepath.Dir(file), "..", "..", "resources"))
	}
	return roots
}

// sideIsFile reports whether path is a regular file.
func sideIsFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// sideSpaceFolder returns the folder of a space, or "".
func sideSpaceFolder(app api.App, id string) string {
	if id == "" {
		return ""
	}
	for _, sp := range app.Spaces() {
		if sp.ID == id {
			return sp.Folder
		}
	}
	return ""
}
