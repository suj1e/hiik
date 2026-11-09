package ui

import (
	"fmt"
	"math"
	"strings"

	mui "github.com/egoist/mygo/ui"

	"hiik/mods/api"
)

// The settings modal's fixed size and the width of its navigation column.
const (
	settingsWidth    = 760
	settingsHeight   = 560
	settingsNavWidth = 180
	// settingsBodyWidth is the content column's width inside the page
	// padding (28 DIPs each side).
	settingsBodyWidth = settingsWidth - settingsNavWidth - 56
)

// newWindowHook opens another window on the same app. A feature file that
// implements multi-window support sets it; settings hides the button when
// it is nil.
var newWindowHook func(*Shell)

// settingsSections are the pages of the settings modal, in navigation order.
var settingsSections = []struct {
	ID    string
	Title string
}{
	{"look", "Look"},
	{"keyboard", "Keyboard"},
	{"machines", "Machines"},
	{"alerts", "Alerts"},
	{"updates", "Updates"},
	{"privacy", "Privacy"},
}

// settingsDraft is the modal's local edit state. Choices that apply at once
// read Settings() every frame; the free-form fields keep what is typed here
// until it is submitted.
type settingsDraft struct {
	uiFont       string
	terminalFont string
	terminalSize float64
	soundDone    string
	soundBlocked string
	contrast     float64
	paddingX     float64
	paddingY     float64
	opacity      float64

	seenUIFont       string
	seenTerminalFont string
	seenTerminalSize float64
	seenSoundDone    string
	seenSoundBlocked string
	seenContrast     float64
	seenPaddingX     float64
	seenPaddingY     float64
	seenOpacity      float64

	// The keyboard table's local state: the section shown, the command
	// being recorded and the text typed, and the last invalid chord.
	keySection string
	keyEditing string
	keyText    string
	keyError   string
}

// newSettingsDraft seeds the text fields from the live settings.
func newSettingsDraft(settings map[string]any) settingsDraft {
	d := settingsDraft{}
	d.sync(settings)
	return d
}

// sync follows a setting changed elsewhere, such as another window or a
// settings.json edit, without discarding text that is still being typed.
func (d *settingsDraft) sync(settings map[string]any) {
	if d == nil {
		return
	}
	if v := settingString(settings, "ui_font"); v != d.seenUIFont {
		d.seenUIFont = v
		d.uiFont = v
	}
	if v := settingString(settings, "terminal_font"); v != d.seenTerminalFont {
		d.seenTerminalFont = v
		d.terminalFont = v
	}
	if v := float64(settingFloat(settings, "terminal_size", 13)); v != d.seenTerminalSize {
		d.seenTerminalSize = v
		d.terminalSize = v
	}
	if v := settingString(settings, "sound_done"); v != d.seenSoundDone {
		d.seenSoundDone = v
		d.soundDone = v
	}
	if v := settingString(settings, "sound_blocked"); v != d.seenSoundBlocked {
		d.seenSoundBlocked = v
		d.soundBlocked = v
	}
	if v := float64(settingFloat(settings, "minimum_contrast", 1)); v != d.seenContrast {
		d.seenContrast = v
		d.contrast = v
	}
	if v := float64(settingNumber(settings, "padding_x", 10)); v != d.seenPaddingX {
		d.seenPaddingX = v
		d.paddingX = v
	}
	if v := float64(settingNumber(settings, "padding_y", 6)); v != d.seenPaddingY {
		d.seenPaddingY = v
		d.paddingY = v
	}
	if v := float64(settingNumber(settings, "opacity", 1)); v != d.seenOpacity {
		d.seenOpacity = v
		d.opacity = v
	}
}

// settingsSectionID is the selected page, or Look when none is.
func (s *Shell) settingsSectionID() string {
	for _, sec := range settingsSections {
		if sec.ID == s.settingsSection {
			return sec.ID
		}
	}
	return "look"
}

// settingsTitle is the page title for a section ID.
func settingsTitle(id string) string {
	for _, sec := range settingsSections {
		if sec.ID == id {
			return sec.Title
		}
	}
	return "Look"
}

// settingsView draws the settings modal: a navigation column on the left and
// the selected page on the right.
func (s *Shell) settingsView(c *mui.Context, settings map[string]any) {
	t := c.Theme()
	root := mui.Row(c).
		Width(settingsWidth).
		Height(settingsHeight).
		Radius(t.Radius + 2).
		Background(t.Background)
	d := mui.Local(root, "settings/draft", func() settingsDraft {
		return newSettingsDraft(settings)
	})
	if d == nil {
		seed := newSettingsDraft(settings)
		d = &seed
	}
	d.sync(settings)
	root.Children(func() {
		s.settingsNav(c, t)
		s.settingsPage(c, t, settings, d)
	})
}

// settingsNav draws the section list and the feedback button.
func (s *Shell) settingsNav(c *mui.Context, t *mui.Theme) {
	current := s.settingsSectionID()
	mui.Column(c).
		Fill().
		Width(settingsNavWidth).
		Padding(10).
		Gap(2).
		BorderWidth(0, 1, 0, 0).
		BorderColor(t.Border).
		Background(t.Surface).
		Children(func() {
			mui.Text(c, "Settings").
				FontSize(16).
				Bold().
				TextColor(t.Text).
				Padding(8, 10, 12, 10)
			for _, sec := range settingsSections {
				sec := sec
				selected := sec.ID == current
				row := mui.Row(c).
					Height(30).
					Padding(0, 10).
					Radius(7).
					AlignItems(mui.Center).
					OnClick(func() { s.settingsSection = sec.ID })
				if selected {
					row.Background(t.SurfaceHover)
				}
				row.Children(func() {
					color := t.TextMuted
					if selected {
						color = t.Text
					}
					mui.Text(c, sec.Title).FontSize(13).TextColor(color)
				})
			}
			mui.Box(c).Grow(1)
			mui.Button(c, "Send feedback").
				Width(settingsNavWidth - 20).
				OnClick(func() { s.aboutOpen = true })
		})
}

// settingsPage draws the selected section under a scroll.
func (s *Shell) settingsPage(c *mui.Context, t *mui.Theme, settings map[string]any, d *settingsDraft) {
	id := s.settingsSectionID()
	mui.Column(c).Grow(1).Height(settingsHeight).Children(func() {
		mui.Scroll(c).Fill().Children(func() {
			mui.Column(c).
				Width(settingsWidth-settingsNavWidth).
				Padding(28, 20).
				Gap(4).
				Children(func() {
					mui.Text(c, strings.ToUpper(settingsTitle(id))).
						FontSize(11).
						Bold().
						TextColor(t.TextMuted).
						Padding(0, 0, 8, 0)
					switch id {
					case "keyboard":
						s.settingsKeyboard(c, settings, d)
					case "machines":
						s.settingsMachines(c)
					case "alerts":
						s.settingsAlerts(c, settings, d)
					case "updates":
						s.settingsUpdates(c)
					case "privacy":
						s.settingsPrivacy(c, settings, d)
					default:
						s.settingsLook(c, settings, d)
					}
				})
		})
	})
}

// settingsHeading draws a small uppercase heading above a group of rows.
func settingsHeading(c *mui.Context, title string) {
	mui.Text(c, strings.ToUpper(title)).
		FontSize(10).
		Bold().
		TextColor(c.Theme().TextMuted).
		Padding(10, 0, 6, 0)
}

// settingsNote draws a muted explanation.
func settingsNote(c *mui.Context, text string) {
	mui.Text(c, text).
		FontSize(11).
		TextColor(c.Theme().TextMuted).
		Padding(8, 0, 2, 0)
}

// settingsRow draws a setting's name and explanation on the left and its
// control on the right.
func settingsRow(c *mui.Context, title, about string, control func()) {
	t := c.Theme()
	mui.Row(c).
		Width(settingsBodyWidth).
		Padding(0, 0, 12, 0).
		Gap(16).
		AlignItems(mui.Center).
		BorderWidth(0, 0, 1, 0).
		BorderColor(t.Border).
		Children(func() {
			mui.Column(c).Grow(1).Gap(2).Children(func() {
				mui.Text(c, title).FontSize(13).TextColor(t.Text)
				if about != "" {
					mui.Text(c, about).FontSize(11).TextColor(t.TextMuted)
				}
			})
			mui.Row(c).Shrink(0).Gap(6).AlignItems(mui.Center).Children(control)
		})
}

// settingsLook draws the layout, theme and size and font controls.
func (s *Shell) settingsLook(c *mui.Context, settings map[string]any, d *settingsDraft) {
	t := c.Theme()

	settingsHeading(c, "Layout")
	names, layout := s.layoutChoices(settings)
	mui.Row(c).Width(settingsBodyWidth).Wrap().Gap(8).Children(func() {
		for _, name := range names {
			name := name
			s.settingsLayoutCard(c, t, name, layoutTitle(name), name == layout)
		}
	})

	settingsHeading(c, "Theme")
	settingsNote(c, "The theme a layout starts from; a layout can pick its own below.")
	settingsRow(c, "Appearance", "Light or dark, or as your system is now.", func() {
		index := 0
		switch settingString(settings, "appearance") {
		case "light":
			index = 1
		case "dark":
			index = 2
		}
		seg := mui.Segmented(c, &index, "System", "Light", "Dark").Label("Appearance")
		if seg.Changed() {
			values := []string{"system", "light", "dark"}
			s.run("app.set", map[string]any{"key": "appearance", "value": values[index]})
		}
	})
	theme := settingString(settings, "theme")
	if theme == "" {
		theme = ThemeGraphite
	}
	mui.Row(c).Width(settingsBodyWidth).Wrap().Gap(8).Children(func() {
		for _, name := range ThemeNames() {
			name := name
			s.settingsThemeCard(c, t, name, themeTitle(name), name == theme, func() {
				s.run("app.set", map[string]any{"key": "theme", "value": name})
			})
		}
	})

	// The reference keeps the theme picker on the layout it was used on, so
	// each layout can be light or dark on its own. An empty entry follows
	// the global theme above.
	settingsHeading(c, "Theme for "+layout)
	layoutTheme := layoutThemeName(settings, layout)
	themes := settingThemeMap(settings)
	if len(themes) > 0 && strings.TrimSpace(themes[layout]) != "" {
		settingsRow(c, "This layout's theme", "It wins over the theme above. Clear it to follow the global theme.", func() {
			if mui.Button(c, "Use the global theme").Clicked() {
				next := make(map[string]string, len(themes))
				for name, value := range themes {
					if name != layout {
						next[name] = value
					}
				}
				s.run("app.set", map[string]any{"key": "layout_themes", "value": next})
			}
		})
	}
	mui.Row(c).Width(settingsBodyWidth).Wrap().Gap(8).Children(func() {
		for _, name := range ThemeNames() {
			name := name
			s.settingsThemeCard(c, t, name, themeTitle(name), name == layoutTheme, func() {
				next := make(map[string]string, len(themes)+1)
				for key, value := range themes {
					next[key] = value
				}
				next[layout] = name
				s.run("app.set", map[string]any{"key": "layout_themes", "value": next})
			})
		}
	})

	settingsHeading(c, "Size and fonts")
	settingsRow(c, "Interface font", "The family of labels, menus and panels. Empty uses the system font.", func() {
		input := mui.TextInput(c, &d.uiFont).
			Width(220).
			Placeholder("System")
		if input.Submitted() {
			s.run("app.set", map[string]any{"key": "ui_font", "value": d.uiFont})
			d.seenUIFont = d.uiFont
		}
	})
	settingsRow(c, "Terminal font", "The family the terminal draws with. Empty uses the built-in font.", func() {
		input := mui.TextInput(c, &d.terminalFont).
			Width(220).
			Placeholder("Built-in")
		if input.Submitted() {
			s.run("app.set", map[string]any{"key": "terminal_font", "value": d.terminalFont})
			d.seenTerminalFont = d.terminalFont
		}
	})
	settingsRow(c, "Terminal size", "The terminal's font size in points, from 6 to 72.", func() {
		number := mui.NumberInput(c, &d.terminalSize, 6, 72, 1).Label("Terminal size").Width(120)
		if number.Changed() {
			size := math.Round(d.terminalSize)
			s.run("app.set", map[string]any{"key": "terminal_size", "value": size})
			d.seenTerminalSize = size
		}
	})

	settingsHeading(c, "Terminal")
	settingsRow(c, "Ligatures", "Programming ligatures in the terminal font, for fonts that have them.", func() {
		on := settingBool(settings, "ligatures", false)
		sw := mui.Switch(c, &on).Label("Ligatures")
		if sw.Changed() {
			s.run("app.set", map[string]any{"key": "ligatures", "value": on})
		}
	})
	settingsRow(c, "Padding across", "Room left and right of the grid, in points. Zero means none.", func() {
		number := mui.NumberInput(c, &d.paddingX, 0, 80, 1).Label("Padding across").Width(110)
		if number.Changed() {
			value := math.Round(d.paddingX)
			s.run("app.set", map[string]any{"key": "padding_x", "value": value})
			d.seenPaddingX = value
		}
	})
	settingsRow(c, "Padding down", "Room above and below the grid, in points. Zero means none.", func() {
		number := mui.NumberInput(c, &d.paddingY, 0, 80, 1).Label("Padding down").Width(110)
		if number.Changed() {
			value := math.Round(d.paddingY)
			s.run("app.set", map[string]any{"key": "padding_y", "value": value})
			d.seenPaddingY = value
		}
	})
	settingsRow(c, "Opacity", "How solid the terminal's background is, from 0 to 1. Below 1, what is behind it shows through.", func() {
		number := mui.NumberInput(c, &d.opacity, 0, 1, 0.05).Label("Opacity").Width(110)
		if number.Changed() {
			value := math.Round(d.opacity*100) / 100
			s.run("app.set", map[string]any{"key": "opacity", "value": value})
			d.seenOpacity = value
		}
	})
	settingsRow(c, "Cursor", "The shape the terminal draws the cursor with.", func() {
		index := 0
		switch settingString(settings, "cursor_shape") {
		case "bar":
			index = 1
		case "underline":
			index = 2
		}
		seg := mui.Segmented(c, &index, "Block", "Bar", "Underline").Label("Cursor")
		if seg.Changed() {
			values := []string{"block", "bar", "underline"}
			s.run("app.set", map[string]any{"key": "cursor_shape", "value": values[index]})
		}
	})
	settingsRow(c, "Blink the cursor", "Programs may still change it.", func() {
		on := settingBool(settings, "cursor_blink", true)
		sw := mui.Switch(c, &on).Label("Blink the cursor")
		if sw.Changed() {
			s.run("app.set", map[string]any{"key": "cursor_blink", "value": on})
		}
	})
	settingsRow(c, "Scrollbars", "Show the scroll position at a terminal's edge when it has scrollback.", func() {
		on := settingBool(settings, "pane_scrollbars", true)
		sw := mui.Switch(c, &on).Label("Scrollbars")
		if sw.Changed() {
			s.run("app.set", map[string]any{"key": "pane_scrollbars", "value": on})
		}
	})

	settingsHeading(c, "Zoom")
	settingsNote(c, "Zoom scales the whole window content, as a browser zooms a page.")
	settingsRow(c, "Zoom", "From 0.5 to 3. The zoom keys step by a tenth.", func() {
		zoom := float64(settingFloat(settings, "zoom", 1))
		number := mui.NumberInput(c, &zoom, 0.5, 3, 0.1).Label("Zoom").Width(100)
		if number.Changed() {
			value := math.Round(zoom*10) / 10
			s.run("app.set", map[string]any{"key": "zoom", "value": value})
		}
		if mui.Button(c, "Zoom out").Clicked() {
			s.run("app.zoom_out", nil)
		}
		if mui.Button(c, "Zoom in").Clicked() {
			s.run("app.zoom_in", nil)
		}
		if mui.Button(c, "Reset").Clicked() {
			s.run("app.zoom_reset", nil)
		}
	})
}

// settingsLayoutCard draws one layout choice with a small preview.
func (s *Shell) settingsLayoutCard(c *mui.Context, t *mui.Theme, value, title string, selected bool) {
	card := mui.Column(c).
		Width(124).
		Padding(6).
		Gap(6).
		Radius(10).
		BorderWidth(2).
		OnClick(func() {
			s.run("app.set", map[string]any{"key": "layout", "value": value})
		})
	if selected {
		card.BorderColor(t.Accent).Background(t.SurfaceHover)
	} else {
		card.BorderColor(mui.RGBA(0, 0, 0, 0))
	}
	card.Children(func() {
		mui.Box(c).
			Width(112).
			Height(62).
			Radius(6).
			Background(t.Surface).
			Children(func() { settingsLayoutPreview(c, t, value) })
		mui.Text(c, title).FontSize(11).TextColor(t.Text)
	})
}

// themeTitle is a theme's label in the picker.
func themeTitle(name string) string {
	if name == ThemeAtom {
		return "atom one dark"
	}
	return name
}

// layoutNamesSource lets the layout engine offer the user's layouts. It is
// optional so a test app without a layout registry still shows the built-ins.
type layoutNamesSource interface {
	LayoutNames() []string
}

// layoutChoices is the layouts Settings offers and the one that is shown.
// The layout engine's Layout() is the source of truth for the shown layout:
// an unknown or missing name falls back to herdr there.
func (s *Shell) layoutChoices(settings map[string]any) ([]string, string) {
	names := api.BuiltinLayoutNames()
	if src, ok := s.app.(layoutNamesSource); ok {
		if list := src.LayoutNames(); len(list) > 0 {
			names = list
		}
	}
	current := settingString(settings, "layout")
	if current == "" {
		current = api.LayoutHerdr
	}
	if src, ok := s.app.(layoutSource); ok {
		if name := strings.TrimSpace(src.Layout()); name != "" {
			current = name
		}
	}
	if !containsString(names, current) {
		names = append(names, current)
	}
	return names, current
}

// layoutTitle is a layout's label in the picker.
func layoutTitle(name string) string {
	switch name {
	case api.LayoutHerdr:
		return "Default"
	case api.LayoutTerminals:
		return "Terminals"
	}
	return name
}

// settingsLayoutPreview draws a small picture of a layout: its sidebar, tab
// strip and pane cards, in the theme's own colors.
func settingsLayoutPreview(c *mui.Context, t *mui.Theme, name string) {
	line := func(w float32) {
		mui.Box(c).Width(w).Height(3).Radius(2).Background(t.TextMuted)
	}
	lines := func(n int) {
		mui.Column(c).Gap(4).Padding(5).Children(func() {
			for i := range n {
				if i%2 == 0 {
					line(32)
				} else {
					line(22)
				}
			}
		})
	}
	term := func() {
		mui.Box(c).
			Grow(1).
			Fill().
			Radius(2).
			Background(t.Background).
			Border(1, t.Border)
	}
	if name == "terminals" {
		mui.Column(c).
			Fill().
			Padding(4).
			Gap(4).
			Radius(6).
			Background(t.Background).
			Border(1, t.Border).
			Children(func() {
				// The tabs ride in the title bar, and the panes use the
				// full width below them.
				mui.Row(c).
					Height(10).
					Padding(1).
					Gap(3).
					Radius(4).
					Background(t.Surface).
					Children(func() {
						mui.Box(c).
							Width(30).
							Height(6).
							Radius(3).
							Background(t.SurfaceHover).
							Border(1, t.Border)
						mui.Box(c).Width(30).Height(6).Radius(3).Background(t.Background)
					})
				mui.Row(c).Grow(1).Gap(4).Children(func() {
					for range 2 {
						mui.Column(c).
							Grow(1).
							Fill().
							Radius(4).
							Background(t.Surface).
							Border(1, t.Border).
							Children(func() {
								mui.Row(c).
									Height(7).
									Padding(0, 3).
									AlignItems(mui.Center).
									BorderWidth(0, 0, 1, 0).
									BorderColor(t.Border).
									Children(func() { line(14) })
							})
					}
				})
			})
		return
	}
	// The default layout keeps the spaces and agents in a sidebar on the left.
	mui.Row(c).
		Fill().
		Radius(6).
		Background(t.Surface).
		Border(1, t.Border).
		Clip().
		Children(func() {
			mui.Column(c).
				Width(42).
				Fill().
				BorderWidth(0, 1, 0, 0).
				BorderColor(t.Border).
				Children(func() { lines(4) })
			mui.Column(c).Grow(1).Fill().Children(func() {
				mui.Row(c).
					Height(11).
					Padding(2).
					Gap(2).
					AlignItems(mui.Center).
					BorderWidth(0, 0, 1, 0).
					BorderColor(t.Border).
					Children(func() {
						mui.Box(c).Width(20).Height(3).Radius(2).Background(t.Accent)
						mui.Box(c).Width(16).Height(3).Radius(2).Background(t.SurfaceHover)
						mui.Box(c).Width(16).Height(3).Radius(2).Background(t.SurfaceHover)
					})
				mui.Row(c).Grow(1).Padding(4).Gap(3).Children(func() {
					term()
					term()
				})
			})
		})
}

// settingsThemeCard draws one theme with a swatch of its colors.
func (s *Shell) settingsThemeCard(c *mui.Context, t *mui.Theme, value, title string, selected bool, pick func()) {
	swatch := lookTheme(themeIsDark(value, true), map[string]any{"theme": value})
	card := mui.Column(c).
		Width(124).
		Padding(6).
		Gap(6).
		Radius(10).
		BorderWidth(2).
		OnClick(pick)
	if selected {
		card.BorderColor(t.Accent).Background(t.SurfaceHover)
	} else {
		card.BorderColor(mui.RGBA(0, 0, 0, 0))
	}
	card.Children(func() {
		mui.Column(c).
			Width(112).
			Height(46).
			Padding(7).
			Gap(6).
			Radius(6).
			Background(swatch.Background).
			Border(1, t.Border).
			Children(func() {
				mui.Box(c).Width(44).Height(4).Radius(2).Background(swatch.Text)
				mui.Row(c).Gap(4).Children(func() {
					for _, color := range []mui.Color{swatch.Accent, swatch.Success, swatch.Danger, swatch.Warning} {
						mui.Box(c).Size(8, 8).Radius(4).Background(color)
					}
				})
			})
		mui.Text(c, title).FontSize(11).TextColor(t.Text).SingleLine()
	})
}

// settingsKeyboard draws the Option key choice and the editable keymap.
func (s *Shell) settingsKeyboard(c *mui.Context, settings map[string]any, d *settingsDraft) {
	settingsHeading(c, "Option key")
	settingsNote(c, "Use Option for terminal shortcuts or to type accents and special characters.")
	option := settingString(settings, "option_key")
	if option == "" {
		option = "auto"
	}
	changed := false
	mui.RadioGroup(c, func() {
		if mui.Radio(c, &option, "auto", "Automatic").Changed() {
			changed = true
		}
		if mui.Radio(c, &option, "alt", "Alt").Changed() {
			changed = true
		}
		if mui.Radio(c, &option, "native", "Types characters").Changed() {
			changed = true
		}
		if mui.Radio(c, &option, "left", "Left Option is Alt").Changed() {
			changed = true
		}
		if mui.Radio(c, &option, "right", "Right Option is Alt").Changed() {
			changed = true
		}
	}).Label("Option key")
	if changed {
		s.run("app.set", map[string]any{"key": "option_key", "value": option})
	}
	settingsNote(c, "Left and Right both use the terminal's one Alt switch: the other Option key cannot keep typing characters yet. macOS only.")

	settingsHeading(c, "Keyboard shortcuts")
	settingsNote(c, "Click Edit, then type a chord such as Mod+Shift+K, or none to unbind it. Changes apply live.")
	sections := s.keymapSectionNames(settings)
	if d.keySection == "" || !containsString(sections, d.keySection) {
		d.keySection = "global"
	}
	sectionIndex := 0
	labels := make([]string, len(sections))
	for i, section := range sections {
		labels[i] = keymapSectionTitle(section)
		if section == d.keySection {
			sectionIndex = i
		}
	}
	tab := mui.Segmented(c, &sectionIndex, labels...).Label("Keymap section")
	if tab.Changed() {
		d.keySection = sections[sectionIndex]
		d.keyEditing = ""
		d.keyError = ""
	}

	var bindings []api.KeyBinding
	if d.keySection == "global" {
		bindings = s.app.Keymap()
		index := make(map[string]int, len(bindings))
		for i, b := range bindings {
			index[b.Command] = i
		}
		for _, sc := range defaultShortcuts(s) {
			i, ok := index[sc.action]
			if !ok {
				continue
			}
			if bindings[i].Default == "" {
				bindings[i].Default = sc.chord
			}
			if !bindings[i].Explicit && bindings[i].Chord == "" {
				bindings[i].Chord = sc.chord
			}
		}
	} else {
		bindings = s.keymapSection(d.keySection)
	}
	if len(bindings) == 0 {
		settingsNote(c, "No keys are set in this section of keys.json.")
		return
	}

	for _, binding := range bindings {
		binding := binding
		title := binding.Title
		if title == "" {
			title = binding.Command
		}
		chord := binding.Chord
		if chord == "" {
			chord = binding.Default
		}
		if chord == "" {
			chord = "Unbound"
		}
		editable := binding.Section == "global" || d.keySection == "global"
		settingsRow(c, title, binding.Command, func() {
			if editable && d.keyEditing == binding.Command {
				input := mui.TextInput(c, &d.keyText).
					Width(160).
					Placeholder("Mod+Shift+K").
					AutoFocus()
				if input.Submitted() {
					next, ok := keymapEdit(settingKeys(settings), binding.Command, d.keyText)
					if !ok {
						d.keyError = fmt.Sprintf(
							"%q is not a key; use a chord such as Mod+Shift+K, or none to unbind.",
							strings.TrimSpace(d.keyText))
					} else {
						s.run("app.set", map[string]any{"key": "keys", "value": next})
						d.keyError = ""
						d.keyEditing = ""
					}
				}
				if mui.Button(c, "Cancel").Clicked() {
					d.keyError = ""
					d.keyEditing = ""
				}
				return
			}
			pill := mui.Text(c, chord).
				FontSize(11).
				TextColor(c.Theme().TextMuted).
				Padding(2, 7).
				Radius(c.Theme().Radius).
				Background(c.Theme().Surface)
			if editable {
				edit := func() {
					d.keyEditing = binding.Command
					d.keyText = binding.Chord
					d.keyError = ""
				}
				pill.OnClick(edit)
				if mui.Button(c, "Edit").Clicked() {
					edit()
				}
			}
		})
	}
	if d.keyError != "" {
		mui.Text(c, d.keyError).
			FontSize(11).
			TextColor(c.Theme().Danger).
			Padding(8, 0, 2, 0)
	}
	if d.keySection != "global" {
		settingsNote(c, "This section lives in keys.json. The shell applies global keys for now.")
	}
}

// containsString reports whether list has value.
func containsString(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

// settingsMachines draws this computer and the machines panel.
func (s *Shell) settingsMachines(c *mui.Context) {
	t := c.Theme()
	version := s.app.CoreVersion()
	if version == "" {
		version = "unknown"
	}
	mui.Row(c).
		Width(settingsBodyWidth).
		Padding(0, 0, 12, 0).
		Gap(10).
		AlignItems(mui.Center).
		BorderWidth(0, 0, 1, 0).
		BorderColor(t.Border).
		Children(func() {
			mui.Box(c).Size(8, 8).Radius(4).Background(t.Success)
			mui.Column(c).Grow(1).Gap(2).Children(func() {
				mui.Text(c, "This computer").FontSize(13).TextColor(t.Text)
				mui.Text(c, "Connected · core "+version).FontSize(11).TextColor(t.TextMuted)
			})
		})
	mui.Box(c).Height(6)
	s.renderPanel(c, api.PanelMachines, nil)
}

// settingsAlerts draws the sound toggle and the notification previews.
func (s *Shell) settingsAlerts(c *mui.Context, settings map[string]any, d *settingsDraft) {
	settingsHeading(c, "Sounds")
	on := settingBool(settings, "sound", true)
	settingsRow(c, "Play sounds", "When an agent needs you, and when one finishes while you're not looking.", func() {
		sw := mui.Switch(c, &on).Label("Play sounds")
		if sw.Changed() {
			s.run("app.set", map[string]any{"key": "sound", "value": on})
		}
	})
	settingsRow(c, "An agent needs you", "A lower tone for an agent that is blocked.", func() {
		if mui.Button(c, "Play").Clicked() {
			PlayNotification(api.LevelBlocked)
		}
	})
	settingsRow(c, "An agent finished", "A brighter tone for a finished turn.", func() {
		if mui.Button(c, "Play").Clicked() {
			PlayNotification(api.LevelDone)
		}
	})

	settingsHeading(c, "Custom sounds")
	settingsNote(c, "A path to your own sound file; empty plays the built-in tone.")
	settingsRow(c, "An agent needs you", "Played when an agent is blocked.", func() {
		input := mui.TextInput(c, &d.soundBlocked).
			Width(220).
			Placeholder("Built-in tone")
		if input.Submitted() {
			s.run("app.set", map[string]any{"key": "sound_blocked", "value": d.soundBlocked})
			d.seenSoundBlocked = d.soundBlocked
		}
	})
	settingsRow(c, "An agent finished", "Played when a turn finishes while you are away.", func() {
		input := mui.TextInput(c, &d.soundDone).
			Width(220).
			Placeholder("Built-in tone")
		if input.Submitted() {
			s.run("app.set", map[string]any{"key": "sound_done", "value": d.soundDone})
			d.seenSoundDone = d.soundDone
		}
	})
}

// settingsUpdates draws the app and core versions and their checks.
func (s *Shell) settingsUpdates(c *mui.Context) {
	appVersion := s.app.Version()
	if appVersion == "" {
		appVersion = "dev"
	}
	coreVersion := s.app.CoreVersion()
	if coreVersion == "" {
		coreVersion = "unknown"
	}

	settingsHeading(c, "App")
	settingsRow(c, "hiik "+appVersion, "The app you're using.", func() {
		if mui.Button(c, "What's new").Clicked() {
			s.run("app.whats_new", nil)
		}
		if mui.Button(c, "Check now").Clicked() {
			s.run("app.check_updates", nil)
		}
		if mui.Button(c, "Download update").Clicked() {
			s.run("app.download_update", nil)
		}
		if newWindowHook != nil {
			if mui.Button(c, "New window").Clicked() {
				newWindowHook(s)
			}
		}
	})

	settingsHeading(c, "Core")
	settingsRow(c, "core "+coreVersion, "The core that runs your terminals.", func() {
		if mui.Button(c, "Check now").Clicked() {
			s.run("app.check_updates", nil)
		}
		if mui.Button(c, "Update core").Clicked() {
			s.run("app.update_core", nil)
		}
	})
	settingsNote(c, "Updates are checked only when you ask. Builds are unsigned, so you install an update yourself.")
}

// settingsPrivacy draws the privacy switches and the terminal's clipboard
// and contrast policy.
func (s *Shell) settingsPrivacy(c *mui.Context, settings map[string]any, d *settingsDraft) {
	settingsHeading(c, "Privacy")
	telemetry := settingBool(settings, "telemetry", false)
	settingsRow(c, "Send anonymous usage data", "Off by default. When on, hiik writes a local audit under its config folder; it never sends it.", func() {
		sw := mui.Switch(c, &telemetry).Label("Send anonymous usage data")
		if sw.Changed() {
			s.run("app.set", map[string]any{"key": "telemetry", "value": telemetry})
		}
	})
	clipboard := settingBool(settings, "clipboard_write", true)
	settingsRow(c, "Allow writing to the clipboard", "Terminals can copy selections and agent output to the system clipboard.", func() {
		sw := mui.Switch(c, &clipboard).Label("Allow writing to the clipboard")
		if sw.Changed() {
			s.run("app.set", map[string]any{"key": "clipboard_write", "value": clipboard})
		}
	})
	clipboardRead := settingBool(settings, "clipboard_read", false)
	settingsRow(c, "Allow reading the clipboard", "A program may ask for the clipboard's contents. Off unless you allow it.", func() {
		sw := mui.Switch(c, &clipboardRead).Label("Allow reading the clipboard")
		if sw.Changed() {
			s.run("app.set", map[string]any{"key": "clipboard_read", "value": clipboardRead})
		}
	})

	settingsHeading(c, "Terminal")
	settingsRow(c, "Minimum contrast", "The contrast ratio a terminal color keeps against its background, from 1 to 21.", func() {
		number := mui.NumberInput(c, &d.contrast, 1, 21, 0.5).
			Label("Minimum contrast").
			Width(110)
		if number.Changed() {
			value := math.Round(d.contrast*10) / 10
			s.run("app.set", map[string]any{"key": "minimum_contrast", "value": value})
			d.seenContrast = value
		}
	})
	settingsNote(c, "Settings apply live.")
}

// settingBool reads a boolean setting, or def when it is absent.
func settingBool(settings map[string]any, key string, def bool) bool {
	if settings == nil {
		return def
	}
	b, ok := settings[key].(bool)
	if !ok {
		return def
	}
	return b
}
