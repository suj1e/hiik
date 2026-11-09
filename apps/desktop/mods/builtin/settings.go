package builtin

import (
	"runtime/debug"
	"strings"

	"github.com/egoist/mygo/ui"

	"hiik/mods/api"
)

func init() {
	api.Register(api.Mod{
		ID:   "settings",
		Name: "Settings",
		Panels: []api.Panel{{
			ID:     api.PanelSettings,
			Title:  "Settings",
			Render: renderSettings,
		}},
	})
}

func renderSettings(rc *api.RenderContext) {
	if rc == nil || rc.C == nil || rc.App == nil {
		return
	}
	t := rc.C.Theme()
	conn := rc.App.Connection()
	settings := sideSettings(rc)

	ui.Column(rc.C).Gap(t.Spacing).Padding(12).Children(func() {
		sideSection(rc, "Version")
		sideInfoRow(rc, "hiik", sideVersion(rc.Props))
		sideInfoRow(rc, "core", sideCoreVersion(rc.Props, conn))
		sideInfoRow(rc, "connection", conn, sideConnectionColor(t, conn))
		if err := sideStr(rc.Props, "connectionError"); err != "" {
			ui.Text(rc.C, err).
				FontSize(t.FontSize - 2).
				TextColor(t.Danger)
		}

		ui.Divider(rc.C)

		sideSection(rc, "Notifications")
		sideToggle(rc, settings, "notify_blocked", "Notify when an agent is blocked", false)
		sideToggle(rc, settings, "notify_done", "Notify when an agent finishes while away", false)
		sideToggle(rc, settings, "sound", "Play a sound with notifications", false)

		ui.Divider(rc.C)

		sideSection(rc, "Privacy")
		sideToggle(rc, settings, "clipboard_write", "Allow writing to the clipboard", true)
		sideToggle(rc, settings, "telemetry", "Send anonymous telemetry", false)
		ui.Text(rc.C, "Settings apply live. Nothing is sent without your action.").
			FontSize(t.FontSize - 2).
			TextColor(t.TextMuted)
	})
}

// sideSection draws a small heading.
func sideSection(rc *api.RenderContext, title string) {
	t := rc.C.Theme()
	ui.Text(rc.C, title).
		FontSize(t.FontSize-2).
		Bold().
		TextColor(t.TextMuted).
		Padding(0, 0, t.Spacing, 0)
}

// sideInfoRow draws a read-only label and value.
func sideInfoRow(rc *api.RenderContext, label, value string, colors ...ui.Color) {
	t := rc.C.Theme()
	color := t.Text
	if len(colors) > 0 {
		color = colors[0]
	}
	ui.Row(rc.C).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(rc.C, label).FontSize(t.FontSize - 1).TextColor(t.TextMuted)
		ui.Box(rc.C).Grow(1)
		ui.Text(rc.C, value).FontSize(t.FontSize - 1).TextColor(color).SingleLine()
	})
}

// sideToggle draws one switch; flipping it runs app.set with the setting
// key and its new value. The panel keeps no copy of the setting.
func sideToggle(rc *api.RenderContext, settings map[string]any, key, label string, def bool) {
	t := rc.C.Theme()
	on := sideSetting(settings, rc.Props, key, def)
	ui.Row(rc.C).Padding(0, 0, t.Spacing, 0).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Text(rc.C, label).FontSize(t.FontSize - 1).SingleLine()
		ui.Box(rc.C).Grow(1)
		sw := ui.Switch(rc.C.Key("mods/settings/"+key), &on).Label(label)
		if sw.Changed() {
			_ = rc.App.Run("app.set", map[string]any{"key": key, "value": on})
		}
	})
}

// sideVersion is the app version from props, else the build's module
// version.
func sideVersion(props map[string]any) string {
	if v := sideStr(props, "version"); v != "" {
		return v
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		v := info.Main.Version
		if v != "" && v != "(devel)" {
			return strings.TrimPrefix(v, "v")
		}
	}
	return "dev"
}

// sideCoreVersion is the connected core's version, when the host passes
// it.
func sideCoreVersion(props map[string]any, conn string) string {
	if v := sideStr(props, "coreVersion"); v != "" {
		return v
	}
	if v := sideStr(props, "core_version"); v != "" {
		return v
	}
	if conn == "connected" {
		return "unknown"
	}
	return "—"
}

func sideConnectionColor(t *ui.Theme, conn string) ui.Color {
	switch conn {
	case "connected":
		return t.Success
	case "connecting":
		return t.Warning
	default:
		return t.Danger
	}
}
