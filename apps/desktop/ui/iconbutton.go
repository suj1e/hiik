package ui

import (
	mui "github.com/egoist/mygo/ui"

	"hiik/icons"
)

// smallIconButton is one native icon button for a panel's title row.
func smallIconButton(c *mui.Context, key string, icon *mui.SVG, label, tip string, fn func()) {
	t := c.Theme()
	button := mui.ButtonBase(c).
		Key(key).
		Size(t.Space(5), t.Space(5)).
		Radius(t.Radius).
		Label(label).
		Tooltip(tip)
	button.Children(func() {
		mui.Icon(c, icon).FontSize(t.FontSize - 1).TextColor(t.TextMuted)
	})
	button.OnClick(fn)
}

// closeIconButton is the native close control used by floating panels.
func closeIconButton(c *mui.Context, key, tip string, fn func()) {
	smallIconButton(c, key, icons.X, tip, tip, fn)
}
