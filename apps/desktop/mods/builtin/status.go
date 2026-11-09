package builtin

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/egoist/mygo/ui"

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
	label := func(glyph, text string) {
		ui.Row(rc.C).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(rc.C, glyph).FontSize(12).TextColor(muted)
			ui.Text(rc.C, text).FontSize(11).TextColor(muted).SingleLine()
		})
	}

	root := ui.Row(rc.C).
		Key("mods/status/bar").
		FillWidth().
		Height(statusHeight).
		Padding(0, 14).
		Gap(16).
		AlignItems(ui.Center).
		Background(t.Surface).
		BorderWidth(1, 0, 0, 0).
		BorderColor(t.Border)
	root.Children(func() {
		machine := "This computer"
		if sp != nil && sp.Machine != "" {
			machine = sp.Machine
		}
		label(spaceMachineGlyph(spMachine(sp)), machine)
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
			label("⑂", branch)
		}
		ui.Row(rc.C).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Badge(rc.C, fmt.Sprintf("%d agents", count))
			if needs > 0 {
				ui.Badge(rc.C, fmt.Sprintf("· %d needs you", needs)).TextColor(t.Danger)
			}
		})
		ui.Box(rc.C).Grow(1)
		if conn == "disconnected" {
			retry := ui.ButtonBase(rc.C).
				Key("mods/status/retry").
				Height(20).
				Padding(0, 6).
				Radius(3).
				Label("Retry core connection").
				Tooltip("Retry core connection")
			retry.Children(func() {
				ui.Text(rc.C, "Retry").FontSize(10).TextColor(t.Accent)
			})
			retry.OnClick(func() { _ = rc.App.Run(api.CmdRetryCore, nil) })
		}
		palette := ui.ButtonBase(rc.C).
			Key("mods/status/palette").
			Height(20).
			Padding(0, 6).
			Radius(3).
			Label("Command palette").
			Tooltip("Command palette")
		palette.Children(func() {
			ui.Text(rc.C, statusPaletteHint(rc)).FontSize(10.5).TextColor(muted)
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
		return statusChordLabel(chord) + "  commands"
	}
	return "commands"
}

// statusChordLabel formats a chord as the platform shows it.
func statusChordLabel(chord string) string {
	raw := strings.TrimSpace(chord)
	if raw == "" {
		return ""
	}
	sep := "+"
	if !strings.Contains(raw, "+") && strings.Contains(raw, "-") {
		sep = "-"
	}
	parts := strings.Split(raw, sep)
	key := parts[len(parts)-1]
	if key == "" && len(parts) > 1 {
		key = parts[len(parts)-2]
		parts = parts[:len(parts)-2]
	}
	var mods []string
	for _, part := range parts[:len(parts)-1] {
		switch strings.ToLower(strings.TrimSpace(part)) {
		case "cmd", "command", "super", "meta", "mod":
			if runtime.GOOS == "darwin" {
				mods = append(mods, "⌘")
			} else {
				mods = append(mods, "Cmd+")
			}
		case "ctrl", "control":
			if runtime.GOOS == "darwin" {
				mods = append(mods, "⌃")
			} else {
				mods = append(mods, "Ctrl+")
			}
		case "alt", "opt", "option":
			if runtime.GOOS == "darwin" {
				mods = append(mods, "⌥")
			} else {
				mods = append(mods, "Alt+")
			}
		case "shift":
			if runtime.GOOS == "darwin" {
				mods = append(mods, "⇧")
			} else {
				mods = append(mods, "Shift+")
			}
		}
	}
	key = strings.ToUpper(key)
	if key == "" {
		return ""
	}
	return strings.Join(mods, "") + key
}
