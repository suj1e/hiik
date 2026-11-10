package builtin

import (
	"fmt"

	"github.com/egoist/mygo/ui"

	"hiik/mods/api"
)

const aboutVersion = "0.1.0"

func init() {
	api.Register(api.Mod{
		ID:   "about",
		Name: "About",
		Panels: []api.Panel{{
			ID:    "about/panel",
			Title: "About hiik",
			Render: func(rc *api.RenderContext) {
				drawAbout(rc)
			},
		}},
		Commands: []api.Command{
			{ID: "app.check_updates", Title: "Check for updates", Run: runApp("app.check_updates")},
			{ID: "app.whats_new", Title: "What's new", Run: runApp("app.whats_new")},
		},
	})
}

func runApp(id string) func(api.App, map[string]any) error {
	return func(app api.App, args map[string]any) error { return app.Run(id, args) }
}

func drawAbout(rc *api.RenderContext) {
	c := rc.C
	t := c.Theme()
	root := ui.Column(c).Gap(12)
	draft := ui.Local(root, "feedback", func() string { return "" })

	root.Children(func() {
		ui.Text(c, "hiik").FontSize(t.FontSize + 7).Bold()
		ui.Text(c, "Native desktop client for herdr-core").FontSize(t.FontSize - 1).TextColor(t.TextMuted)
		ui.Text(c, "Version "+aboutVersion).FontSize(t.FontSize - 2).TextColor(t.TextMuted)

		ui.Row(c).Gap(8).Children(func() {
			if ui.Button(c, "Check for updates").Clicked() {
				_ = rc.App.Run("app.check_updates", nil)
			}
			if ui.Button(c, "What's new").Clicked() {
				_ = rc.App.Run("app.whats_new", nil)
			}
		})

		ui.Box(c).Height(1).Background(t.Border)
		ui.Text(c, "Feedback").FontSize(t.FontSize).Bold()
		ui.TextArea(c, draft).Placeholder("What happened? Nothing is sent until you save and share it.").Lines(3, 6)
		if ui.Button(c, "Save draft").Clicked() {
			if err := rc.App.Run("app.feedback", map[string]any{"text": *draft}); err != nil {
				ui.Text(c, fmt.Sprint(err)).FontSize(t.FontSize - 2).TextColor(t.Danger)
			}
		}
	})
}
