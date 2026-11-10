package builtin

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"hiik/icons"
	"hiik/mods/api"
	"hiik/resources"
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

	hovered, _ := rc.Props["hovered"].(bool)

	header := ui.Row(rc.C).
		FillWidth().
		Gap(t.Space(1.5)).
		AlignItems(ui.Center)
	if title != "" {
		header.Tooltip(title)
	}
	header.Children(func() {
		if known {
			sideProgramBadge(rc.C, t, program, hasProgram, agent.Status)
			label := app
			if label == "" && hasProgram {
				label = program.Name
			}
			if label != "" && label != title {
				ui.Text(rc.C, label).
					FontSize(t.FontSize - 1).
					TextColor(t.Text).
					SingleLine()
			}
			if clean := paneTitleText(title); clean != "" && clean != label {
				ui.Text(rc.C, clean).
					FontSize(t.FontSize - 2).
					TextColor(t.TextMuted).
					SingleLine().
					Shrink(1).
					MaxWidth(t.Rem(16))
			}
		} else {
			ui.Text(rc.C, title).
				FontSize(t.FontSize - 1).
				TextColor(t.TextMuted).
				SingleLine().
				Shrink(1).
				MaxWidth(t.Rem(16))
		}
		if cwd != "" {
			ui.Text(rc.C, cwd).
				FontSize(t.FontSize - 2).
				TextColor(t.TextMuted).
				SingleLine().
				Shrink(1).
				Tooltip(cwd)
		}
		ui.Box(rc.C).Grow(1)
		if !hovered {
			return
		}
		ui.Toolbar(rc.C, func() {
			split := ui.ButtonBase(rc.C).
				Key("mods/panes/split/"+terminal).
				Height(t.Space(5)).
				Padding(0, t.Space(1.5)).
				Radius(t.Radius).
				Label("Split pane").
				Tooltip("Split pane")
			split.Children(func() {
				ui.Text(rc.C, "Split").FontSize(t.FontSize - 2).TextColor(t.TextMuted)
			})
			split.OnClick(func() {
				args := map[string]any{"direction": "right"}
				if pane != "" {
					args["pane"] = pane
				}
				_ = rc.App.Run(api.CmdPaneSplit, args)
			})
			close := ui.ButtonBase(rc.C).
				Key("mods/panes/close/"+terminal).
				Height(t.Space(5)).
				Padding(0, t.Space(1.5)).
				Radius(t.Radius).
				Label("Close pane").
				Tooltip("Close pane")
			close.Children(func() {
				ui.Text(rc.C, "Close").FontSize(t.FontSize - 2).TextColor(t.TextMuted)
			})
			close.OnClick(func() {
				var args map[string]any
				if pane != "" {
					args = map[string]any{"pane": pane}
				}
				_ = rc.App.Run(api.CmdPaneClose, args)
			})
		}).Label("Pane actions")
	})
}

// paneTitleText is the terminal title if it starts with something a person
// reads: an ASCII letter or digit, or a Han character. A program that
// prefixes its title with status glyphs keeps the full title in the header
// tooltip instead.
func paneTitleText(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return ""
	}
	r, _ := utf8.DecodeRuneInString(title)
	switch {
	case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9':
		return title
	case unicode.Is(unicode.Han, r):
		return title
	}
	return ""
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
		tile := ui.Box(c).
			Size(18, 18).
			Radius(4).
			Background(t.Surface).
			Center()
		tile.Children(func() {
			ui.Icon(c, icons.Terminal).FontSize(t.FontSize - 1).TextColor(t.TextMuted)
		})
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
		name = strings.TrimSpace(name)
		if name == "" {
			return nil
		}
		mu.Lock()
		defer mu.Unlock()
		if svg, ok := cache[name]; ok {
			return svg
		}
		if failed[name] {
			return nil
		}
		data, err := resources.Icons.ReadFile(name)
		if err != nil {
			path := sideIconPath(name)
			if path == "" {
				failed[name] = true
				return nil
			}
			data, err = os.ReadFile(path)
		}
		if err != nil {
			failed[name] = true
			return nil
		}
		svg, err := ui.ParseSVG(data)
		if err != nil {
			failed[name] = true
			return nil
		}
		cache[name] = svg
		return svg
	}
}()

// sideIconPath finds a user icon: an absolute path or one under HIIK_HOME.
// The shipped marks are embedded in resources.Icons instead.
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
	home := os.Getenv("HIIK_HOME")
	if home == "" {
		return ""
	}
	path := filepath.Join(home, name)
	if sideIsFile(path) {
		return path
	}
	return ""
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
