// Package ui draws hiik: the window, its chrome and the panes. It reads the
// model from an App and calls its commands; it owns no model state.
package ui

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	mui "github.com/egoist/mygo/ui"

	"hiik/core/hc"
	"hiik/icons"
	"hiik/keys"
	"hiik/mods/api"
)

// App is the part of the app the shell reads and calls. *core.App satisfies
// it; a test can use a fake instead.
type App interface {
	// Spaces returns a snapshot of the spaces.
	Spaces() []api.Space
	// Agents returns a snapshot of the agents.
	Agents() []api.Agent
	// Machines returns a snapshot of the machines the core knows.
	Machines() []api.Machine
	// Programs returns the agents the new-tab menu offers.
	Programs() []api.Program
	// Keymap returns the commands and their current bindings.
	Keymap() []api.KeyBinding
	// Exec runs a command here or on a machine.
	Exec(on, command string, args ...string) (stdout, stderr string, code int, err error)
	// ReadTextFile reads a text file here or on a machine.
	ReadTextFile(path, on string) (string, error)
	// ActiveSpace returns the selected space's ID, or "".
	ActiveSpace() string
	// ActiveAgent returns the selected agent's address, or "".
	ActiveAgent() string
	// Commands returns the app's command registry.
	Commands() []api.Command
	// Connection returns "connecting", "connected" or "disconnected".
	Connection() string
	// Run queues an app command; it never blocks the frame's build pass.
	Run(name string, args map[string]any) error
	// Core returns the connected core client, or nil before the app has
	// reached a core.
	Core() hc.Client
	// UIState returns the persisted view state.
	UIState() api.UIState
	// SetUIState records the view state.
	SetUIState(api.UIState)
	// Settings returns the live settings values the panels show.
	Settings() map[string]any
	// SetNotifier installs the function that shows a notification.
	SetNotifier(func(api.Notification))
	// Overlay returns the overlay a command asked to open, or "".
	Overlay() string
	// SetOverlay records or clears a requested overlay.
	SetOverlay(string)
	// ReleaseNotes returns the notes bundled with this version.
	ReleaseNotes() string
	// Version returns the app's version.
	Version() string
	// CoreVersion returns the connected core's version, or "".
	CoreVersion() string
}

// dialogKind identifies the small shell dialog shown, if any.
type dialogKind uint8

const (
	dialogNone dialogKind = iota
	dialogRename
	dialogConfirm
)

// rightFiles and rightChanges are the tabs of the shell's right panel.
const (
	rightFiles   = "files"
	rightChanges = "changes"
)

// dialog is the shell's rename or confirmation prompt.
type dialog struct {
	kind    dialogKind
	title   string
	message string
	value   string
	rename  func(string)
	confirm func()
}

// Shell is the app's one window.
//
// It holds only view state: the sidebar width, the focused and zoomed pane,
// the open overlay and the terminals it has attached. Spaces, tabs, panes and
// agents live in the app.
type Shell struct {
	app App
	win *mygo.Window

	sidebarW          float32
	sidebarPrevW      float32
	focus             string
	zoom              string
	lastModelFocus    string
	focusPending      bool
	dialog            dialog
	paletteOpen       bool
	settingsOpen      bool
	aboutOpen         bool
	whatsNewOpen      bool
	machinesOpen      bool
	findOpen          bool
	findQuery         string
	helpOpen          bool
	welcomeOpen       bool
	installUpdateOpen bool
	paneArea          mui.Rect
	paletteQuery      string
	paletteSel        int
	paletteList       mui.ListState
	settingsSection   string
	rightOpen         bool
	rightTab          string
	rightSplit        float32
	worktreeOpen      bool
	peek              string
	toast             string
	panels            map[string]api.Panel
	splitPct          map[string]*float32
	lastState         api.UIState
	save              chan api.UIState
	closeOnce         sync.Once
	attachmentsMu     sync.Mutex
	attachments       map[string]*paneTerm
	termSizes         map[string][2]int
	termLook          *shellTermLook
	services          mui.Services
	lastConn          string
	displayKey        string
	rightClick        map[string]string
	sidebarResize     float32
	tabInset          float32
	branchCache       map[string]string
}

// New returns a shell for an app.
func New(app App) *Shell {
	app.SetNotifier(Notify)
	s := &Shell{
		app:             app,
		sidebarW:        sidebarDefaultW,
		settingsSection: "look",
		rightTab:        rightFiles,
		rightSplit:      72,
		attachments:     make(map[string]*paneTerm),
		termSizes:       make(map[string][2]int),
		panels:          api.Panels(),
		splitPct:        make(map[string]*float32),
		save:            make(chan api.UIState, 1),
	}
	if st := app.UIState(); st.SidebarW > 0 || st.Focus != "" || st.Zoomed != "" {
		if st.SidebarW > 0 {
			s.sidebarW = st.SidebarW
		}
		s.focus = st.Focus
		s.zoom = st.Zoomed
		s.lastModelFocus = st.Focus
		s.focusPending = st.Focus != ""
		st.SidebarW = s.sidebarW
		s.lastState = st
	} else {
		s.lastState = api.UIState{SidebarW: s.sidebarW}
	}
	go s.saveLoop()
	s.installHooks()
	return s
}

// Open shows the window. macOS draws its chrome under the traffic lights;
// Windows and Linux keep the system title bar and its window buttons.
func (s *Shell) Open() {
	opts := mygo.WindowOptions{
		Title:     "hiik",
		Width:     1180,
		Height:    760,
		MinWidth:  820,
		MinHeight: 520,
		StateKey:  "main",
		Content:   mui.View(s.View),
	}
	if runtime.GOOS == "darwin" {
		opts.TitleBarStyle = mygo.TitleBarHidden
	}
	s.win = mygo.NewWindow(opts)
	if setter, ok := s.app.(interface{ SetInvalidator(func()) }); ok {
		setter.SetInvalidator(s.wake)
	}
	s.win.OnClosed(s.Close)
	s.win.Focus()
	mygo.App.Focus()
}

// Close detaches every terminal this shell attached. The core keeps the
// programs and their screens.
func (s *Shell) Close() {
	s.closeOnce.Do(func() {
		s.attachmentsMu.Lock()
		terms := s.attachments
		s.attachments = make(map[string]*paneTerm)
		s.attachmentsMu.Unlock()
		for _, ts := range terms {
			detachPaneTerm(ts)
		}
		floatMu.Lock()
		delete(floats, s)
		floatMu.Unlock()
		floatLayoutMu.Lock()
		delete(floatLayouts, s)
		floatLayoutMu.Unlock()
		modSurfaceMu.Lock()
		delete(modSurfaces, s)
		modSurfaceMu.Unlock()
	})
}

// saveLoop persists shell view state after a frame, never on the build pass.
func (s *Shell) saveLoop() {
	for st := range s.save {
		s.app.SetUIState(st)
	}
}

// queueSave offers the latest state to saveLoop, replacing one not yet
// taken. There is exactly one producer: the window's view function.
func (s *Shell) queueSave(st api.UIState) {
	for {
		select {
		case s.save <- st:
			return
		default:
			select {
			case <-s.save:
			default:
			}
		}
	}
}

// persist records the view state when it changed. A collapsed sidebar
// saves the width it will restore to, since the rail is not a size the user
// chose to keep.
func (s *Shell) persist() {
	width := s.sidebarW
	if s.sidebarCompact() && s.sidebarPrevW >= sidebarCompactW {
		width = s.sidebarPrevW
	}
	st := api.UIState{SidebarW: width, Focus: s.focus, Zoomed: s.zoom}
	if st == s.lastState {
		return
	}
	s.lastState = st
	s.queueSave(st)
}

// run queues an app command; a failure is shown as a toast next frame.
func (s *Shell) run(name string, args map[string]any) {
	if err := s.app.Run(name, args); err != nil {
		s.toast = err.Error()
	}
}

// overlayOpen reports whether an overlay has the keyboard.
func (s *Shell) overlayOpen() bool {
	return s.paletteOpen || s.settingsOpen || s.aboutOpen || s.whatsNewOpen ||
		s.machinesOpen || s.worktreeOpen || s.peek != "" || s.dialog.kind != dialogNone ||
		s.helpOpen || s.welcomeOpen || s.installUpdateOpen
}

// View builds the window for one frame.
func (s *Shell) View(c *mui.Context) {
	settings := s.app.Settings()
	applyLook(c, settings)
	s.applyOverlay()
	s.consumeLayoutOverlays()
	t := c.Theme()
	s.attachmentsMu.Lock()
	s.services = c.Services()
	s.attachmentsMu.Unlock()
	s.refreshTermLook(t, settings)
	s.registerShortcuts(c, settings)
	s.syncConnection()
	s.applyTerminalDisplay(c, settings)

	sp, tab := s.activeTab()
	visible := s.visibleTerminals(tab, s.zoom, settings)
	s.syncAttachments(visible)
	s.ensureFocus(tab)
	s.syncModelFocus(tab)

	name := s.layoutName(settings)
	tree, state, ok := s.layoutTree(name)

	mui.Column(c).Fill().Background(t.Background).Children(func() {
		switch {
		case name == api.LayoutTerminals:
			s.terminalsChrome(c, sp, tab, state)
		case name == api.LayoutHerdr || !ok:
			if _, herdrState, has := s.layoutTree(api.LayoutHerdr); has {
				state = herdrState
			}
			s.herdrChrome(c, sp, tab, state)
		default:
			s.renderLayout(c, tree, sp, tab, state)
		}
		s.overlays(c)
		s.layoutOverlays(c)
		runShellHooks(s, c)
	})

	s.persist()
	if s.toast != "" {
		msg := s.toast
		s.toast = ""
		c.Toast(msg)
	}
	away := s.win != nil && (!s.win.IsFocused() || s.win.IsMinimized())
	DrainNotifications(c, away)
}

// activeTab returns the selected space and its active tab, nil when there is
// none.
func (s *Shell) activeTab() (*api.Space, *api.Tab) {
	id := s.app.ActiveSpace()
	spaces := s.app.Spaces()
	for i := range spaces {
		if spaces[i].ID != id {
			continue
		}
		sp := &spaces[i]
		if len(sp.Tabs) == 0 {
			return sp, nil
		}
		n := sp.Active
		if n < 0 || n >= len(sp.Tabs) {
			n = 0
		}
		return sp, &sp.Tabs[n]
	}
	return nil, nil
}

func (s *Shell) titleBar(c *mui.Context) {
	t := c.Theme()
	bar := c.TitleBar()
	mui.Row(c).
		Height(max(bar.Height, 38)).
		Padding(0, max(bar.Right, 12), 0, max(bar.Left, 12)).
		AlignItems(mui.Center).
		Gap(8).
		DragWindow().
		Background(t.Surface).
		Children(func() {
			mui.Text(c, "hiik").FontSize(t.FontSize).Bold()
			mui.Text(c, s.connectionLabel()).FontSize(t.FontSize - 2).TextColor(t.TextMuted)
			mui.Box(c).Grow(1)
			s.paletteChip(c)
		})
}

// terminalsBar is the Terminals layout's title bar: the space picker and
// the tab strip take the place of the app name, and the panes below use the
// window's full width.
func (s *Shell) terminalsBar(c *mui.Context, sp *api.Space, tab *api.Tab) {
	t := c.Theme()
	title := c.TitleBar()
	row := mui.Row(c).
		Height(max(title.Height, terminalsBarHeight)).
		Padding(0, max(title.Right, 10), 0, max(title.Left, 10)).
		AlignItems(mui.Center).
		Gap(8).
		DragWindow().
		Background(t.Surface).
		BorderWidth(0, 0, 1, 0).
		BorderColor(t.Border)
	row.HandleInput(s.tabWheelHandler)
	row.Children(func() {
		s.spacePicker(c, sp)
		mui.Box(c).Grow(1).MinWidth(0).Children(func() {
			s.tabRows(c, sp, tab)
		})
		mui.Box(c).
			Size(6, 6).
			Radius(3).
			Background(func() mui.Color {
				switch s.app.Connection() {
				case "connected":
					return t.Success
				case "disconnected":
					return t.Danger
				default:
					return t.Warning
				}
			}())
		s.paletteChip(c)
	})
}

// spacePicker is the compact space menu of the Terminals layout.
func (s *Shell) spacePicker(c *mui.Context, sp *api.Space) {
	t := c.Theme()
	name := "No space"
	if sp != nil {
		name = sp.Name
		if name == "" {
			name = sp.Folder
		}
		if name == "" {
			name = sp.ID
		}
	}
	menu := mui.MenuButton(c, name, func(m *mui.Menu) {
		for _, space := range s.app.Spaces() {
			space := space
			label := space.Name
			if label == "" {
				label = space.Folder
			}
			if label == "" {
				label = space.ID
			}
			if m.Item(label).Chosen() {
				s.selectSpace(space.ID)
			}
		}
		m.Separator()
		if m.Item("New space").Chosen() {
			s.run(api.CmdSpaceNew, nil)
		}
	})
	menu.Height(30).
		Padding(0, 10).
		Radius(t.Radius + 8).
		Background(t.SurfaceHover)
}

// paletteChip is the small command palette button of the title bars.
func (s *Shell) paletteChip(c *mui.Context) {
	t := c.Theme()
	label := s.paletteChord()
	if label == "" {
		label = "commands"
	}
	mui.Row(c).
		Padding(2, 8).
		Radius(t.Radius).
		AlignItems(mui.Center).
		Tooltip("Command palette").
		Label("Command palette").
		OnClick(s.openPalette).
		Children(func() {
			mui.Text(c, label).FontSize(t.FontSize - 2).TextColor(t.TextMuted)
		})
}

func (s *Shell) sidebar(c *mui.Context) {
	t := c.Theme()
	compact := s.sidebarCompact()
	agent := s.activeAgentAddress()
	mui.Column(c).Fill().Background(t.Surface).Children(func() {
		s.sidebarBrand(c)
		s.sidebarList(c, func() {
			s.renderPanel(c, api.PanelSpaces, map[string]any{
				"spaces":      s.app.Spaces(),
				"active":      s.app.ActiveSpace(),
				"compact":     compact,
				"activeAgent": agent,
			})
		})
		s.sidebarList(c, func() {
			s.renderPanel(c, api.PanelAgents, map[string]any{
				"agents":      s.app.Agents(),
				"compact":     compact,
				"activeAgent": agent,
			})
		})
		s.sidebarFooter(c)
	})
}

// sidebarList draws one of the sidebar's two equal-weight lists. Each panel
// owns the scrolling inside its list, so the sidebar itself never scrolls.
func (s *Shell) sidebarList(c *mui.Context, render func()) {
	mui.Column(c).
		FillWidth().
		Grow(1).
		MinHeight(90).
		Children(render)
}

func (s *Shell) main(c *mui.Context, sp *api.Space, tab *api.Tab, showTabs bool) {
	t := c.Theme()
	mui.Column(c).Fill().Background(t.Background).Children(func() {
		if showTabs {
			s.tabStrip(c, sp, tab)
		}
		if sp == nil {
			s.paneArea = mui.Rect{}
			s.emptyMain(c)
			return
		}
		left := func() {
			if tab == nil || tab.Root == nil {
				s.paneArea = mui.Rect{}
				s.emptyMain(c)
				return
			}
			area := mui.Box(c).Key("shell/pane-area").Fill().Padding(6)
			area.Children(func() { s.panes(c, tab) })
			s.paneArea = area.Bounds()
		}
		if s.rightOpen {
			mui.SplitPercent(c, &s.rightSplit, left, func() { s.rightPanel(c, sp) }).Fill()
			return
		}
		left()
	})
}

func (s *Shell) emptyMain(c *mui.Context) {
	t := c.Theme()
	mui.Column(c).Fill().Center().Gap(6).Children(func() {
		mui.Text(c, "No space open").FontSize(t.FontSize + 2).TextColor(t.TextMuted)
		mui.Text(c, "Create a space to start a terminal.").FontSize(t.FontSize - 1).TextColor(t.TextMuted)
	})
}

func (s *Shell) tabStrip(c *mui.Context, sp *api.Space, tab *api.Tab) {
	t := c.Theme()
	titleBar := c.TitleBar()
	bar := mui.Row(c).
		FillWidth().
		Height(tabBarHeight).
		Padding(0, max(titleBar.Right, 8), 0, max(s.tabInset, 8)).
		Gap(4).
		AlignItems(mui.Center).
		Background(t.Surface).
		BorderWidth(0, 0, 1, 0).
		BorderColor(t.Border).
		DragWindow()
	bar.HandleInput(s.tabWheelHandler)
	bar.Children(func() {
		mui.Box(c).Grow(1).MinWidth(0).Children(func() {
			s.tabRows(c, sp, tab)
		})
		mui.Row(c).Gap(4).AlignItems(mui.Center).Children(func() {
			s.newTabButton(c, sp)
			s.tabToolButton(c, "Files", icons.File, s.rightOpen && s.rightTab == rightFiles, func() {
				s.toggleRightPanel(rightFiles)
			})
			s.tabToolButton(c, "Changes", icons.Branch, s.rightOpen && s.rightTab == rightChanges, func() {
				s.toggleRightPanel(rightChanges)
			})
		})
	})
}

// tabWheelHandler moves one tab forward or back per wheel notch. Shift turns
// the wheel sideways, which the scroller takes to show tabs that do not fit.
func (s *Shell) tabWheelHandler(ev mui.InputEvent) bool {
	if ev.Kind != mui.InputScroll || ev.Mods&mui.Shift != 0 || ev.DX != 0 {
		return false
	}
	switch {
	case ev.DY >= 1:
		s.run("tab.next", nil)
	case ev.DY <= -1:
		s.run("tab.prev", nil)
	default:
		return false
	}
	return true
}

// tabRows draws the tab strip inside a bar: a horizontal scroller whose
// content comes from the tabs panel.
func (s *Shell) tabRows(c *mui.Context, sp *api.Space, tab *api.Tab) {
	if sp == nil || len(sp.Tabs) == 0 {
		t := c.Theme()
		mui.Text(c, "No tabs").FontSize(t.FontSize - 1).TextColor(t.TextMuted)
		return
	}
	mui.ScrollHorizontal(c.Key("shell/tabs")).
		FillWidth().
		MinWidth(0).
		Height(tabChipHeight).
		Children(func() {
			s.renderPanel(c, api.PanelTabs, map[string]any{
				"space":  sp,
				"tabs":   tabsOf(sp),
				"active": activeTabIndex(sp),
			})
		})
}

// tabToolButton is one Files or Changes button of the bar's right cluster.
// A chosen button keeps the raised face, as a pressed toggle does.
func (s *Shell) tabToolButton(c *mui.Context, label string, icon *mui.SVG, on bool, fn func()) {
	t := c.Theme()
	button := mui.ButtonBase(c).
		Key("shell/tab-tool/"+label).
		Height(26).
		Padding(0, 8).
		Gap(8).
		Radius(t.Radius + 2).
		Label(label)
	if on {
		button.Background(t.SurfacePressed).TextColor(t.Text)
	} else {
		button.TextColor(t.TextMuted)
	}
	button.Children(func() {
		if icon != nil {
			mui.Box(c).Size(14, 14).Shrink(0).MarginEnd(3).Children(func() {
				mui.Icon(c, icon).Size(14, 14)
			})
		}
		mui.Text(c, label).FontSize(t.FontSize - 1)
	})
	button.OnClick(fn)
}

// newTabButton is the tab bar's plus: a menu of new tabs, programs, spaces
// and the worktree dialog.
func (s *Shell) newTabButton(c *mui.Context, sp *api.Space) {
	t := c.Theme()
	button := mui.ButtonBase(c).
		Key("shell/tab-new").
		Size(26, 26).
		Radius(t.Radius + 2).
		Center().
		Tooltip("New tab").
		Label("New tab")
	button.Children(func() {
		mui.Text(c, "+").FontSize(t.FontSize + 3).TextColor(t.TextMuted)
	})
	button.PopoverMenuButton(s.newTabMenu(sp))
}

// newTabMenu fills the plus menu. The Worktree entry lives here, as in the
// reference app, not in the bar.
func (s *Shell) newTabMenu(sp *api.Space) func(*mui.Menu) {
	return func(m *mui.Menu) {
		spaceID := ""
		if sp != nil {
			spaceID = sp.ID
		}
		if m.Item("Terminal").Disabled(sp == nil).Chosen() {
			s.run(api.CmdTabNew, map[string]any{"space": spaceID})
		}
		m.Separator()
		// As the reference menu: every installed agent, then at most two
		// not installed, so the menu stays short.
		installed := make([]api.Program, 0)
		missing := make([]api.Program, 0)
		for _, p := range s.app.Programs() {
			if p.Installed {
				installed = append(installed, p)
			} else {
				missing = append(missing, p)
			}
		}
		if len(missing) > 2 {
			missing = missing[:2]
		}
		for _, p := range append(installed, missing...) {
			p := p
			label := p.Name
			if !p.Installed {
				label += " (not installed)"
			}
			if m.Item(label).Disabled(sp == nil || !p.Installed).Chosen() {
				s.run(api.CmdTabNew, map[string]any{
					"space": spaceID,
					"name":  p.Name,
					"argv":  p.Run,
				})
			}
		}
		m.Separator()
		if m.Item("New space").Chosen() {
			s.run(api.CmdSpaceNew, nil)
		}
		if m.Item("New worktree…").Disabled(sp == nil).Chosen() {
			s.worktreeOpen = true
		}
		if m.Item("Open worktree…").Disabled(sp == nil).Chosen() {
			s.worktreeOpen = true
		}
	}
}

// tabMenu is a tab's right-click menu.
func (s *Shell) tabMenu(sp *api.Space, tb api.Tab) func(*mui.Menu) {
	return func(m *mui.Menu) {
		if m.Item("Rename…").Chosen() {
			s.renameTabByID(sp.ID, tb)
		}
		if m.Item("New tab").Chosen() {
			s.run(api.CmdTabNew, map[string]any{"space": sp.ID})
		}
		m.Separator()
		if m.Item("Close tab").Chosen() {
			s.run(api.CmdTabClose, map[string]any{"space": sp.ID, "tab": tb.ID})
		}
	}
}

// renameTabByID opens the shell's rename prompt for one tab.
func (s *Shell) renameTabByID(spaceID string, tb api.Tab) {
	name := tb.Name
	s.askRename("Rename tab", name, func(v string) {
		s.run(api.CmdTabRename, map[string]any{
			"space": spaceID,
			"tab":   tb.ID,
			"name":  v,
		})
	})
}

// tabProgram finds the program row of the agent tab shows, for its icon.
func (s *Shell) tabProgram(sp *api.Space, tb api.Tab) (api.Program, bool) {
	if sp == nil {
		return api.Program{}, false
	}
	for _, ag := range s.app.Agents() {
		if ag.Space != "" && ag.Space != sp.ID {
			continue
		}
		if ag.Tab != tb.ID && !tabHasTerminal(&tb, ag.Address) {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(ag.App))
		if name == "" {
			continue
		}
		for _, p := range s.app.Programs() {
			if strings.EqualFold(p.Key, name) || strings.EqualFold(p.Name, name) {
				return p, true
			}
			for _, match := range p.Match {
				if strings.EqualFold(match, name) {
					return p, true
				}
			}
			if len(p.Run) > 0 && strings.EqualFold(p.Run[0], name) {
				return p, true
			}
		}
	}
	return api.Program{}, false
}

// tabStatus is the loudest status among the agents in a tab: working,
// then blocked, then done.
func (s *Shell) tabStatus(sp *api.Space, tb api.Tab) string {
	if sp == nil {
		return ""
	}
	seen := map[string]bool{}
	for _, ag := range s.app.Agents() {
		if ag.Space != "" && ag.Space != sp.ID {
			continue
		}
		if ag.Tab != tb.ID && !tabHasTerminal(&tb, ag.Address) {
			continue
		}
		seen[ag.Status] = true
	}
	switch {
	case seen["working"]:
		return "working"
	case seen["blocked"]:
		return "blocked"
	case seen["done"]:
		return "done"
	}
	return ""
}

// toggleRightPanel opens the right panel on a tab, or closes it when that
// tab is already showing.
func (s *Shell) toggleRightPanel(tab string) {
	if s.rightOpen && s.rightTab == tab {
		s.rightOpen = false
		return
	}
	s.rightOpen = true
	s.rightTab = tab
}

// rightPanel draws the files or changes panel beside the panes.
func (s *Shell) rightPanel(c *mui.Context, sp *api.Space) {
	t := c.Theme()
	folder := s.currentFolder(sp)
	mui.Column(c).Fill().Background(t.Surface).Children(func() {
		if s.rightTab == rightChanges {
			s.renderPanel(c, api.PanelChanges, map[string]any{
				"space":  sp,
				"folder": folder,
			})
			return
		}
		s.renderPanel(c, api.PanelFiles, map[string]any{
			"space":  sp,
			"folder": folder,
		})
	})
}

// currentFolder is the folder the right panels act on: the focused
// terminal's live cwd, else the space's launch folder.
func (s *Shell) currentFolder(sp *api.Space) string {
	if sp == nil {
		return ""
	}
	if _, tab := s.activeTab(); tab != nil {
		if node := findNode(tab.Root, s.focus); node != nil && node.Terminal != "" {
			if info, ok := s.terminalInfo(node.Terminal); ok && info.Cwd != "" {
				return info.Cwd
			}
		}
	}
	return sp.Folder
}

func (s *Shell) statusBar(c *mui.Context, sp *api.Space) {
	agents := s.app.Agents()
	needs := 0
	for _, ag := range agents {
		if ag.Status == "blocked" {
			needs++
		}
	}
	s.renderPanel(c, api.PanelStatus, map[string]any{
		"space":      sp,
		"agents":     len(agents),
		"needs":      needs,
		"connection": s.app.Connection(),
		"branch":     s.statusBranch(sp),
		"palette":    s.paletteHint(),
	})
}

// statusBranch is the active space's git branch, read once per folder and
// kept for later frames. A detached HEAD or a folder that is not a
// repository leaves the bar without a branch.
func (s *Shell) statusBranch(sp *api.Space) string {
	if sp == nil {
		return ""
	}
	folder := s.currentFolder(sp)
	if folder == "" {
		return ""
	}
	key := sp.Machine + "\x00" + folder
	if branch, ok := s.branchCache[key]; ok {
		return branch
	}
	out, _, code, err := s.app.Exec(sp.Machine, "git", "-C", folder,
		"rev-parse", "--abbrev-ref", "HEAD")
	branch := ""
	if err == nil && code == 0 {
		branch = strings.TrimSpace(out)
		if branch == "HEAD" {
			branch = ""
		}
	}
	if s.branchCache == nil {
		s.branchCache = make(map[string]string)
	}
	s.branchCache[key] = branch
	return branch
}

// renderPanel draws a registered panel by ID, or a short built-in text when
// no mod provides it. The shell owns the layout; a panel owns its content.
func (s *Shell) renderPanel(c *mui.Context, id string, props map[string]any) {
	if p, ok := s.panels[id]; ok && p.Render != nil {
		p.Render(&api.RenderContext{C: c, App: s.app, Props: props})
		return
	}
	mui.Text(c, "Panel "+id+" is unavailable").
		FontSize(c.Theme().FontSize - 1).
		TextColor(c.Theme().TextMuted).
		Padding(12)
}

func (s *Shell) connectionLabel() string {
	switch s.app.Connection() {
	case "connected":
		return "connected"
	case "disconnected":
		return "offline"
	default:
		return "starting"
	}
}

func (s *Shell) selectSpace(id string) {
	if id == s.app.ActiveSpace() {
		return
	}
	if s.focus != "" {
		s.zoom = ""
		s.focus = ""
		s.focusPending = false
	}
	s.run(api.CmdSpaceSelect, map[string]any{"space": id, "id": id})
}

func (s *Shell) selectTab(id string) {
	s.zoom = ""
	s.focus = ""
	s.focusPending = false
	s.run(api.CmdTabSelect, map[string]any{"tab": id, "id": id})
}

// focusAgent opens the agent's terminal in its space and tab. An agent whose
// terminal is not a pane of the active tab is peeked at instead.
func (s *Shell) focusAgent(ag api.Agent) {
	if ag.Address == "" {
		return
	}
	shown := false
	for _, sp := range s.app.Spaces() {
		if ag.Space != "" && sp.ID != ag.Space {
			continue
		}
		for _, tab := range sp.Tabs {
			if ag.Tab != "" && tab.ID != ag.Tab {
				continue
			}
			if tabHasTerminal(&tab, ag.Address) {
				shown = true
				break
			}
		}
	}
	if !shown {
		s.peek = ag.Address
		return
	}
	if ag.Space != "" {
		s.run(api.CmdSpaceSelect, map[string]any{"space": ag.Space, "id": ag.Space})
	}
	if ag.Tab != "" {
		s.run(api.CmdTabSelect, map[string]any{"tab": ag.Tab, "id": ag.Tab})
	}
	s.run(api.CmdAgentFocus, map[string]any{"agent": ag.Address, "address": ag.Address})
	if st := s.app.UIState(); st.Focus != "" {
		s.focus = st.Focus
		s.focusPending = true
		s.zoom = ""
	}
}

// syncModelFocus adopts a pane focus set by a panel or command. Those call
// agent.focus directly, so without this the model moves but the shell keeps
// its old pane. Only a changed model focus is adopted; shell-local focus moves
// flow the other way, through persist.
func (s *Shell) syncModelFocus(tab *api.Tab) {
	st := s.app.UIState()
	if st.Focus == s.lastModelFocus {
		return
	}
	s.lastModelFocus = st.Focus
	if st.Focus == "" || tab == nil || findNode(tab.Root, st.Focus) == nil {
		return
	}
	s.focus = st.Focus
	s.focusPending = true
	s.zoom = st.Zoomed
}

// tabHasTerminal reports whether the tab shows the terminal address.
func tabHasTerminal(tab *api.Tab, address string) bool {
	if tab == nil || tab.Root == nil || address == "" {
		return false
	}
	visible := make(map[string]bool)
	leaves(tab.Root, visible)
	return visible[address]
}

func tabsOf(sp *api.Space) []api.Tab {
	if sp == nil {
		return nil
	}
	return sp.Tabs
}

func activeTabIndex(sp *api.Space) int {
	if sp == nil || len(sp.Tabs) == 0 {
		return -1
	}
	n := sp.Active
	if n < 0 || n >= len(sp.Tabs) {
		return -1
	}
	return n
}

func statusColor(c *mui.Context, status string) mui.Color {
	t := c.Theme()
	switch status {
	case "working":
		return t.Accent
	case "blocked":
		return t.Danger
	case "done":
		return t.Success
	case "error":
		return t.Danger
	default:
		return t.TextMuted
	}
}

// paletteChord is the key the keymap binds to the command palette, as a
// short label, or "" when the action is unbound.
func (s *Shell) paletteChord() string {
	for _, b := range s.app.Keymap() {
		if b.Command != api.CmdPalette || (b.Section != "" && b.Section != "global") {
			continue
		}
		chord := strings.TrimSpace(b.Chord)
		if chord != "" {
			if keys.Unbound(chord) {
				return ""
			}
			return chordLabel(chord)
		}
		// The core lists every command, including one the shell binds by
		// default and the person has not overridden. An explicit empty
		// chord is an unbound action; no chord falls through to the
		// shell's own binding instead of showing "commands".
		if b.Explicit {
			return ""
		}
	}
	for _, shortcut := range defaultShortcuts(s) {
		if shortcut.action == api.CmdPalette {
			return chordLabel(shortcut.chord)
		}
	}
	return ""
}

// paletteHint is the status bar's palette hint: the real binding, followed
// by what it opens. Without a binding it still says "commands".
func (s *Shell) paletteHint() string {
	if key := s.paletteChord(); key != "" {
		return key + "  commands"
	}
	return "commands"
}

// chordLabel formats a canonical chord as the platform shows it.
func chordLabel(chord string) string {
	return api.FormatChord(chord)
}

// shortcutMods is the modifier of the app's shortcuts, as keys.Parse reads
// Mod.
func shortcutMods() mui.Modifiers { return keys.ShortcutMods() }

// registerShortcuts binds the app's keys for this frame. The terminal keeps
// every key the app does not bind. The keys setting replaces the chord of an
// action by its ID; an empty, "none" or "-" value leaves the action unbound.
func (s *Shell) registerShortcuts(c *mui.Context, settings map[string]any) {
	overrides := make(map[string]string)
	for _, b := range s.app.Keymap() {
		if b.Explicit {
			overrides[b.Command] = b.Chord
		}
	}
	for _, sc := range defaultShortcuts(s) {
		mods, key, ok, overridden := resolveShortcut(sc, overrides)
		if !ok {
			if overridden {
				s.toast = fmt.Sprintf("keymap: %q is not a key for %s", overrides[sc.action], sc.action)
			}
			continue
		}
		c.OnWindowShortcut(mods, key, func() { sc.run(s) })
	}
}

func (s *Shell) selectSpaceAt(index int) {
	spaces := s.app.Spaces()
	if index < 0 || index >= len(spaces) {
		return
	}
	s.selectSpace(spaces[index].ID)
}

func (s *Shell) openPalette() {
	s.paletteQuery = ""
	s.paletteSel = 0
	s.paletteList = mui.ListState{}
	s.paletteOpen = true
}

func (s *Shell) openSettings() {
	s.settingsOpen = true
}

func (s *Shell) renameTab() {
	_, tab := s.activeTab()
	if tab == nil {
		return
	}
	name := tab.Name
	s.askRename("Rename tab", name, func(v string) {
		s.run(api.CmdTabRename, map[string]any{"tab": tab.ID, "id": tab.ID, "name": v})
	})
}

func (s *Shell) confirmCloseTab() {
	_, tab := s.activeTab()
	if tab == nil {
		return
	}
	s.askConfirm("Close tab", "Close this tab?", func() {
		s.run(api.CmdTabClose, map[string]any{"tab": tab.ID, "id": tab.ID})
	})
}

func (s *Shell) toggleZoom() {
	if s.zoom != "" {
		s.zoom = ""
	} else {
		s.zoom = s.focus
	}
	s.focusPending = true
}

// askRename opens the shell's rename prompt.
func (s *Shell) askRename(title, initial string, fn func(string)) {
	s.dialog = dialog{kind: dialogRename, title: title, value: initial, rename: fn}
}

// askConfirm opens the shell's confirmation prompt.
func (s *Shell) askConfirm(title, message string, fn func()) {
	s.dialog = dialog{kind: dialogConfirm, title: title, message: message, confirm: fn}
}

// paletteHidden lists commands the palette does not show: they need a
// target, a JSON tree or an action the UI itself offers, so choosing them
// from a search field could only fail. They stay available to keys, mods and
// the CLI.
var paletteHidden = map[string]bool{
	"agent.focus": true, "agent.peek": true, "agent.pin": true,
	"app.feedback": true, "app.hide_others": true, "app.show_all": true,
	"app.set": true, "app.theme": true,
	"layout.clone": true, "layout.hide": true, "layout.resize": true,
	"layout.select": true, "layout.set": true, "layout.show": true,
	"layout.toggle": true,
	"machine.add":   true, "machine.connect": true, "machine.rename": true,
	"machine.rm": true,
	"mod.float":  true, "mod.window": true,
	"pane.focus": true, "pane.go": true, "pane.split": true, "pane.swap": true,
	"space.move": true, "space.rename": true, "space.select": true,
	"tab.move": true, "tab.rename": true, "tab.select": true,
	"terminal.copy": true, "terminal.keep": true, "terminal.kill": true,
	"terminal.label": true, "terminal.park": true, "terminal.pin": true,
	"terminal.right_click": true, "terminal.scroll": true,
	"terminal.show": true,
}

// filteredCommands returns visible commands whose title or ID contains the
// query.
func filteredCommands(cmds []api.Command, query string) []api.Command {
	visible := make([]api.Command, 0, len(cmds))
	for _, cmd := range cmds {
		if !paletteHidden[cmd.ID] {
			visible = append(visible, cmd)
		}
	}
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return visible
	}
	out := make([]api.Command, 0, len(visible))
	for _, cmd := range visible {
		if strings.Contains(strings.ToLower(cmd.Title), q) ||
			strings.Contains(strings.ToLower(cmd.ID), q) {
			out = append(out, cmd)
		}
	}
	return out
}

// shortcutTime is a small helper for tests and retries.
func retryAfter(d time.Duration) time.Time { return time.Now().Add(d) }
