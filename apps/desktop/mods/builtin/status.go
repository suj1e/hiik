package builtin

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"hiik/icons"
	"hiik/mods/api"
)

// statusHeight is the work column's status bar height, matching the shell.
const statusHeight = 28

func init() {
	api.Register(api.Mod{
		ID:   "status",
		Name: "Status",
		Panels: []api.Panel{{
			ID:     api.PanelStatus,
			Title:  "Status",
			Render: renderStatus,
		}},
	})
}

func renderStatus(rc *api.RenderContext) {
	if rc == nil || rc.C == nil || rc.App == nil {
		return
	}
	t := rc.C.Theme()
	agents := sideAgents(rc)
	count := len(agents)
	if v, ok := rc.Props["agents"].(int); ok {
		count = v
	}
	needs := 0
	for _, ag := range agents {
		if ag.Status == "blocked" {
			needs++
		}
	}
	if v, ok := rc.Props["needs"].(int); ok {
		needs = v
	}
	conn := sideStr(rc.Props, "connection")
	if conn == "" {
		conn = rc.App.Connection()
	}
	var sp *api.Space
	if v, ok := rc.Props["space"].(*api.Space); ok {
		sp = v
	}
	if sp == nil {
		active := sideActive(rc)
		for _, s := range sideSpaces(rc) {
			if s.ID == active {
				s := s
				sp = &s
				break
			}
		}
	}

	muted := t.TextMuted
	label := func(icon *ui.SVG, text string) {
		ui.Row(rc.C).Gap(t.Space(1.5)).AlignItems(ui.Center).Children(func() {
			ui.Icon(rc.C, icon).FontSize(t.FontSize).TextColor(muted)
			ui.Text(rc.C, text).FontSize(t.FontSize - 2).TextColor(muted).SingleLine()
		})
	}

	root := ui.Row(rc.C).
		Key("mods/status/bar").
		FillWidth().
		Height(statusHeight).
		Padding(0, t.Space(3.5)).
		Gap(t.Space(4)).
		AlignItems(ui.Center).
		Background(t.Surface).
		BorderWidth(1, 0, 0, 0).
		BorderColor(t.Border)
	root.Children(func() {
		machine := "This computer"
		if sp != nil && sp.Machine != "" {
			machine = sp.Machine
		}
		label(spaceMachineIcon(spMachine(sp)), machine)
		branch := sideStr(rc.Props, "branch")
		if branch == "" && sp != nil && sp.Folder != "" {
			cache := ui.Local(root, "mods/status/git", newSpaceGitCache)
			if cache != nil {
				if info := cache.info(rc.App, sp.Machine, spaceGitFolder(rc.App, *sp)); info.Branch != "" {
					branch = info.Branch
				}
			}
		}
		if branch != "" {
			label(icons.Branch, branch)
		}
		ui.Row(rc.C).Gap(t.Space(1.5)).AlignItems(ui.Center).Children(func() {
			dot := muted
			if needs > 0 {
				dot = t.Danger
			}
			ui.Box(rc.C).Size(6, 6).Radius(3).Background(dot)
			ui.Text(rc.C, fmt.Sprintf("%d agents", count)).FontSize(t.FontSize - 2).TextColor(muted)
			if needs > 0 {
				ui.Text(rc.C, fmt.Sprintf("%d needs you", needs)).FontSize(t.FontSize - 2).TextColor(t.Danger)
			}
		})
		ui.Box(rc.C).Grow(1)
		if conn == "disconnected" {
			retry := ui.ButtonBase(rc.C).
				Key("mods/status/retry").
				Height(t.Space(5)).
				Padding(0, t.Space(1.5)).
				Radius(t.Space(1)).
				Label("Retry core connection").
				Tooltip("Retry core connection")
			retry.Children(func() {
				ui.Text(rc.C, "Retry").FontSize(t.FontSize - 3).TextColor(t.Accent)
			})
			retry.OnClick(func() { _ = rc.App.Run(api.CmdRetryCore, nil) })
		}
		palette := ui.ButtonBase(rc.C).
			Key("mods/status/palette").
			Height(t.Space(5)).
			Padding(0, t.Space(1.5)).
			Radius(t.Space(1)).
			Label("Command palette").
			Tooltip("Command palette")
		palette.Children(func() {
			ui.Text(rc.C, statusPaletteHint(rc)).FontSize(t.FontSize - 2.5).TextColor(muted)
		})
		palette.OnClick(func() { _ = rc.App.Run(api.CmdPalette, nil) })
	})
}

// spMachine is the machine a space runs on, or "" for this computer.
func spMachine(sp *api.Space) string {
	if sp == nil {
		return ""
	}
	return sp.Machine
}

// keymapSource is the optional part of an app that carries the live
// keybindings. The core app implements it; a test app need not.
type keymapSource interface {
	Keymap() []api.KeyBinding
}

// statusPaletteHint shows the key the palette is actually bound to, as the
// host passed it, or as the app's keymap says. Without a binding the hint
// still offers the command.
func statusPaletteHint(rc *api.RenderContext) string {
	if hint := sideStr(rc.Props, "palette"); hint != "" {
		return hint
	}
	src, ok := rc.App.(keymapSource)
	if !ok {
		return "commands"
	}
	for _, b := range src.Keymap() {
		if b.Command != api.CmdPalette || (b.Section != "" && b.Section != "global") {
			continue
		}
		chord := strings.TrimSpace(b.Chord)
		if chord == "" {
			return "commands"
		}
		return api.FormatChord(chord) + "  commands"
	}
	return "commands"
}
