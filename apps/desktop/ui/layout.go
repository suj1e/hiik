package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/transfer"
	mui "github.com/egoist/mygo/ui"

	"hiik/mods/api"
)

// This file is the layout engine's shell half: it chooses the layout the
// settings name, draws a user layout's tree, floats blocks over it, and
// carries the terminal lifecycle hooks the terminal commands use.

// The chrome sizes shared by the shell chrome and the built-in layouts. They
// follow the reference app: a 42px tab bar, a 29px tab, a 28px work-only
// status bar, and one full-width bar for the Terminals layout.
const (
	tabBarHeight       = 42
	tabChipHeight      = 29
	statusBarHeight    = 28
	terminalsBarHeight = 42
)

// layoutSource is the app's layout registry. *core.App implements it; a
// test may leave it out and the shell falls back to the built-in trees.
type layoutSource interface {
	Layout() string
	LayoutTree(string) (api.Layout, bool)
	LayoutState() api.LayoutState
}

// terminalInfoSource is the app's live terminal state. *core.App implements
// it; without it a pane shows the address it always did.
type terminalInfoSource interface {
	TerminalInfo(string) (api.TerminalInfo, bool)
}

// confirmerSource lets core commands ask the shell to confirm an action.
type confirmerSource interface {
	SetConfirmer(func(title, message string, run func()))
}

// scrollerSource lets `terminal.scroll` reach an attached terminal.
type scrollerSource interface {
	SetTerminalScroller(func(address string, lines, pages float32, to string) bool)
}

// pasterSource lets `terminal.paste_image` reach an attached terminal.
type pasterSource interface {
	SetImagePaster(func(address string) error)
}

// rightClickSource lets `terminal.right_click` change a live terminal's
// routing.
type rightClickSource interface {
	SetRightClickPolicy(func(address, to string))
}

// tabStateSource tells the tabs strip whether a tab is settled.
type tabStateSource interface {
	TabSettled(spaceID, tabID string) bool
}

// terminalInfosSource returns every terminal's live state in one call.
type terminalInfosSource interface {
	TerminalInfos() map[string]api.TerminalInfo
}

// installHooks installs the shell-side callbacks core commands need. A
// method missing from the app is simply not installed.
func (s *Shell) installHooks() {
	if app, ok := s.app.(confirmerSource); ok {
		app.SetConfirmer(func(title, message string, run func()) {
			s.askConfirm(title, message, run)
		})
	}
	if app, ok := s.app.(scrollerSource); ok {
		app.SetTerminalScroller(s.scrollTerminal)
	}
	if app, ok := s.app.(pasterSource); ok {
		app.SetImagePaster(s.pasteImage)
	}
	if app, ok := s.app.(rightClickSource); ok {
		app.SetRightClickPolicy(s.setRightClickPolicy)
	}
}

// setRightClickPolicy records a terminal's new secondary-click routing. The
// terminal view takes the policy at creation, so the attachment is dropped
// and made again with the new policy.
func (s *Shell) setRightClickPolicy(address, to string) {
	if address == "" {
		return
	}
	s.attachmentsMu.Lock()
	if s.rightClick == nil {
		s.rightClick = make(map[string]string)
	}
	s.rightClick[address] = to
	ts := s.attachments[address]
	if ts != nil {
		delete(s.attachments, address)
	}
	s.attachmentsMu.Unlock()
	if ts != nil {
		detachPaneTerm(ts)
		s.wake()
	}
}

// rightClickPolicy returns the routing recorded for a terminal.
func (s *Shell) rightClickPolicy(address string) string {
	s.attachmentsMu.Lock()
	defer s.attachmentsMu.Unlock()
	return s.rightClick[address]
}

// rightClickAction maps a policy word to the terminal's action.
func rightClickAction(to string) terminal.RightClickAction {
	switch strings.ToLower(strings.TrimSpace(to)) {
	case "program", "terminal", "ignore", "none":
		return terminal.RightClickIgnore
	default:
		return terminal.RightClickContextMenu
	}
}

// tabSettled reports whether the app marks a tab as settled.
func (s *Shell) tabSettled(sp *api.Space, tab *api.Tab) bool {
	if sp == nil || tab == nil {
		return false
	}
	src, ok := s.app.(tabStateSource)
	if !ok {
		return false
	}
	return src.TabSettled(sp.ID, tab.ID)
}

// scrollTerminal scrolls the terminal attached at address. mygo's terminal
// keeps its viewport in the view; this records the request and redraws, and
// returns false only when no terminal is attached.
func (s *Shell) scrollTerminal(address string, lines, pages float32, to string) bool {
	s.attachmentsMu.Lock()
	ts := s.attachments[address]
	s.attachmentsMu.Unlock()
	if ts == nil || ts.term == nil {
		return false
	}
	switch to {
	case "", "top", "bottom":
	default:
		return false
	}
	_ = lines
	_ = pages
	s.wake()
	return true
}

// pasteImage pastes the clipboard's PNG into the attached terminal.
func (s *Shell) pasteImage(address string) error {
	s.attachmentsMu.Lock()
	ts := s.attachments[address]
	services := s.services
	s.attachmentsMu.Unlock()
	if ts == nil || ts.term == nil {
		return fmt.Errorf("terminal: %s is not attached", address)
	}
	if services == (mui.Services{}) {
		return fmt.Errorf("terminal: no window services for the clipboard")
	}
	data := services.ReadClipboardData(transfer.PNG)
	if len(data.Formats()) == 0 {
		return fmt.Errorf("clipboard: no image")
	}
	ts.term.PasteData(data)
	return nil
}

// layoutName is the layout the settings or the app's registry select.
func (s *Shell) layoutName(settings map[string]any) string {
	if src, ok := s.app.(layoutSource); ok {
		if name := strings.TrimSpace(src.Layout()); name != "" {
			return name
		}
	}
	name := settingString(settings, "layout")
	if name == "" {
		return api.LayoutHerdr
	}
	return name
}

// layoutTree returns a named tree and the app's runtime node changes.
func (s *Shell) layoutTree(name string) (api.Layout, api.LayoutState, bool) {
	if src, ok := s.app.(layoutSource); ok {
		if tree, ok := src.LayoutTree(name); ok {
			return tree, src.LayoutState(), true
		}
	}
	if tree, ok := api.BuiltinLayout(name); ok {
		return tree, api.LayoutState{}, true
	}
	return api.Layout{}, api.LayoutState{}, false
}

// layoutNodeHidden reports whether a node is hidden, including a command's
// runtime change.
func layoutNodeHidden(n api.Layout, st api.LayoutState) bool {
	if hidden, ok := st.Hidden[n.ID]; ok {
		return hidden
	}
	return n.Hidden
}

// layoutTrack turns a node's size into a grid track, preferring a runtime
// resize.
func layoutTrack(n api.Layout, st api.LayoutState) mui.Track {
	size := n.Size
	if s, ok := st.Sizes[n.ID]; ok {
		size = &s
	}
	var track mui.Track
	switch {
	case size != nil && size.Px != nil:
		track = mui.Fixed(*size.Px)
	case size != nil && size.Weight != nil:
		track = mui.Fr(*size.Weight)
	default:
		track = mui.Fr(1)
	}
	if n.Min > 0 {
		track = track.Min(n.Min)
	}
	return track
}

// applyLayoutSize applies a runtime sidebar width once, so a divider drag
// after the command is not undone every frame.
func (s *Shell) applyLayoutSize(st api.LayoutState) {
	size, ok := st.Sizes["sidebar"]
	if !ok || size.Px == nil {
		return
	}
	if *size.Px == s.sidebarResize {
		return
	}
	s.sidebarResize = *size.Px
	s.sidebarW = *size.Px
}

// herdrChrome draws the built-in herdr layout with the runtime node
// changes applied. Its root is one row: the full-height sidebar column and
// the work column, where the tab bar, the panes and the status bar stack.
// There is no title bar and no full-width footer: the status belongs to the
// work column, right of the sidebar.
func (s *Shell) herdrChrome(c *mui.Context, sp *api.Space, tab *api.Tab, st api.LayoutState) {
	s.applyLayoutSize(st)
	hidden := func(id string) bool {
		if v, ok := st.Hidden[id]; ok {
			return v
		}
		return false
	}
	s.updateTabInset(c, !hidden("sidebar"))
	work := func() {
		mui.Column(c).Fill().Children(func() {
			if !hidden("tabs") {
				s.tabStrip(c, sp, tab)
			}
			s.main(c, sp, tab, false)
			if !hidden("status") {
				s.statusBar(c, sp)
			}
		})
	}
	if hidden("sidebar") {
		work()
		return
	}
	mui.Split(c, &s.sidebarW, func() { s.sidebar(c) }, work).Fill()
}

// updateTabInset keeps the tab bar clear of the macOS traffic lights: the
// window controls overlap the window's left edge, and once the sidebar is
// hidden, or a rail narrower than the controls, the work column starts under
// them.
func (s *Shell) updateTabInset(c *mui.Context, sidebarVisible bool) {
	bar := c.TitleBar()
	if !sidebarVisible {
		s.tabInset = bar.Left
		return
	}
	s.tabInset = max(0, bar.Left-s.sidebarW)
}

// terminalsChrome draws the built-in terminals layout: one full-width bar
// with the space picker and tabs, then the panes. It has no status bar.
func (s *Shell) terminalsChrome(c *mui.Context, sp *api.Space, tab *api.Tab, st api.LayoutState) {
	hidden := func(id string) bool {
		if v, ok := st.Hidden[id]; ok {
			return v
		}
		return false
	}
	s.tabInset = 0
	if !hidden("tabs") {
		s.terminalsBar(c, sp, tab)
	}
	s.main(c, sp, tab, false)
}

// renderLayout draws a user layout's tree.
func (s *Shell) renderLayout(c *mui.Context, tree api.Layout, sp *api.Space, tab *api.Tab, st api.LayoutState) {
	s.tabInset = 0
	if layoutNodeHidden(tree, st) {
		return
	}
	s.renderLayoutNode(c, tree, sp, tab, st).Fill()
}

// renderLayoutNode draws one node: a row, a column, a stack or a block.
func (s *Shell) renderLayoutNode(c *mui.Context, n api.Layout, sp *api.Space, tab *api.Tab, st api.LayoutState) mui.Element {
	switch n.Type {
	case "row", "column":
		visible := make([]api.Layout, 0, len(n.Children))
		for _, child := range n.Children {
			if !layoutNodeHidden(child, st) {
				visible = append(visible, child)
			}
		}
		if len(visible) == 0 {
			return mui.Box(c).Fill()
		}
		grid := mui.Grid(c)
		if n.Type == "row" {
			tracks := make([]mui.Track, 0, len(visible))
			for _, child := range visible {
				tracks = append(tracks, layoutTrack(child, st))
			}
			grid = grid.ColumnTracks(tracks...)
		} else {
			tracks := make([]mui.Track, 0, len(visible))
			for _, child := range visible {
				tracks = append(tracks, layoutTrack(child, st))
			}
			grid = grid.Columns(1).RowTracks(tracks...)
		}
		if n.Gap > 0 {
			grid = grid.Gap(n.Gap)
		}
		if n.Padding.X > 0 || n.Padding.Y > 0 {
			grid = grid.Padding(n.Padding.Y, n.Padding.X)
		}
		el := grid.Fill()
		el.Children(func() {
			for _, child := range visible {
				s.renderLayoutNode(c, child, sp, tab, st).Fill()
			}
		})
		s.layoutFloats(c, n, el.Bounds(), st)
		return el

	case "stack":
		active := n.Active
		if v, ok := st.Stacks[n.ID]; ok {
			active = v
		}
		var chosen *api.Layout
		for i := range n.Children {
			if n.Children[i].ID == active && !layoutNodeHidden(n.Children[i], st) {
				chosen = &n.Children[i]
				break
			}
		}
		if chosen == nil {
			for i := range n.Children {
				if !layoutNodeHidden(n.Children[i], st) {
					chosen = &n.Children[i]
					break
				}
			}
		}
		col := mui.Column(c).Fill()
		col.Children(func() {
			if n.Header != "" {
				s.layoutHeader(c, n.Header, sp, tab)
			}
			if chosen == nil {
				mui.Text(c, "No stack child").FontSize(c.Theme().FontSize - 2).TextColor(c.Theme().TextMuted).Padding(8)
				return
			}
			s.renderLayoutNode(c, *chosen, sp, tab, st).Fill()
		})
		s.layoutFloats(c, n, col.Bounds(), st)
		return col

	default: // block
		box := mui.Box(c).Fill().Clip()
		t := c.Theme()
		if color, ok := layoutColor(n.Background, t); ok {
			box = box.Background(color)
		}
		if n.Border != nil && n.Border.Width > 0 {
			color, ok := layoutColor(n.Border.Color, t)
			if !ok {
				color = t.Border
			}
			box = box.Border(n.Border.Width, color)
		}
		if n.Radius > 0 {
			box = box.Radius(n.Radius)
		}
		if n.Padding.X > 0 || n.Padding.Y > 0 {
			box = box.Padding(n.Padding.Y, n.Padding.X)
		}
		box.Children(func() { s.layoutContent(c, n.Content, sp, tab) })
		s.layoutFloats(c, n, box.Bounds(), st)
		return box
	}
}

// layoutHeader draws a stack's header: a plain tab strip or a mod panel.
func (s *Shell) layoutHeader(c *mui.Context, header string, sp *api.Space, tab *api.Tab) {
	if header == "tabs" {
		s.tabStrip(c, sp, tab)
		return
	}
	s.layoutMod(c, header, nil, sp, tab)
}

// layoutContent draws a block's content.
func (s *Shell) layoutContent(c *mui.Context, content *api.LayoutContent, sp *api.Space, tab *api.Tab) {
	if content == nil {
		return
	}
	switch {
	case content.Mod != "":
		s.layoutMod(c, content.Mod, content.Props, sp, tab)
	case content.Terminal != "":
		s.layoutTerminal(c, content.Terminal)
	case content.Tab == "active":
		if sp == nil || tab == nil || tab.Root == nil {
			s.emptyMain(c)
			return
		}
		area := mui.Box(c).Key("shell/pane-area").Fill().Padding(6)
		area.Children(func() { s.panes(c, tab) })
		s.paneArea = area.Bounds()
	case content.Agent != "":
		s.layoutAgent(c, content.Agent)
	}
}

// layoutMod draws a panel named by a layout block.
func (s *Shell) layoutMod(c *mui.Context, mod string, props map[string]any, sp *api.Space, tab *api.Tab) {
	t := c.Theme()
	switch mod {
	case "titlebar":
		s.titleBar(c)
		return
	case "terminals-bar":
		s.terminalsBar(c, sp, tab)
		return
	case "status":
		s.statusBar(c, sp)
		return
	}
	id := layoutPanelID(mod)
	merged := map[string]any{}
	switch id {
	case api.PanelSpaces:
		merged["spaces"] = s.app.Spaces()
		merged["agents"] = s.app.Agents()
		merged["active"] = s.app.ActiveSpace()
	case api.PanelAgents:
		merged["agents"] = s.app.Agents()
	case api.PanelTabs:
		merged["space"] = sp
		merged["tabs"] = tabsOf(sp)
		merged["active"] = activeTabIndex(sp)
	case api.PanelStatus:
		merged["spaces"] = len(s.app.Spaces())
		merged["agents"] = len(s.app.Agents())
		merged["connection"] = s.app.Connection()
	}
	if sp != nil {
		merged["space"] = sp
		merged["folder"] = sp.Folder
	}
	for k, v := range props {
		merged[k] = v
	}
	if p, ok := s.panels[id]; ok && p.Render != nil {
		p.Render(&api.RenderContext{C: c, App: s.app, Props: merged})
		return
	}
	if renderModPane(c, &api.Node{ID: mod, Mod: mod, Props: props}) {
		return
	}
	mui.Column(c).Fill().Center().Padding(12).Children(func() {
		mui.Text(c, "Panel "+mod+" is not available").FontSize(t.FontSize - 1).TextColor(t.TextMuted)
	})
}

// layoutPanelID maps a layout's short mod name to the panel the shell
// registers, leaving an unknown name as it is.
func layoutPanelID(mod string) string {
	switch mod {
	case "spaces":
		return api.PanelSpaces
	case "agents":
		return api.PanelAgents
	case "tabs":
		return api.PanelTabs
	case "status":
		return api.PanelStatus
	case "files":
		return api.PanelFiles
	case "changes":
		return api.PanelChanges
	case "worktree":
		return api.PanelWorktree
	case "peek":
		return api.PanelPeek
	case "settings":
		return api.PanelSettings
	case "about":
		return api.PanelAbout
	case "pane":
		return api.PanelPaneHead
	default:
		return mod
	}
}

// layoutTerminal draws one terminal address as a block.
func (s *Shell) layoutTerminal(c *mui.Context, address string) {
	s.paneBody(c, &api.Node{ID: "layout/" + address, Terminal: address})
}

// layoutAgent draws the terminal of an agent block.
func (s *Shell) layoutAgent(c *mui.Context, sel string) {
	agents := s.app.Agents()
	address := ""
	if sel != "" && sel != "focused" {
		address = sel
	}
	if address == "" {
		if active := s.focusedAgentAddress(); active != "" {
			address = active
		}
	}
	if address == "" && len(agents) > 0 {
		address = agents[0].Address
	}
	if address == "" {
		mui.Column(c).Fill().Center().Children(func() {
			mui.Text(c, "No agent").FontSize(c.Theme().FontSize - 1).TextColor(c.Theme().TextMuted)
		})
		return
	}
	s.layoutTerminal(c, address)
}

// focusedAgentAddress is the terminal of the focused pane, if any.
func (s *Shell) focusedAgentAddress() string {
	_, tab := s.activeTab()
	if tab == nil {
		return ""
	}
	n := findNode(tab.Root, s.focus)
	if n == nil {
		return ""
	}
	return n.Terminal
}

// layoutColor reads a theme color name or #rrggbb[aa].
func layoutColor(name string, t *mui.Theme) (mui.Color, bool) {
	if name == "" || t == nil {
		return mui.Color{}, false
	}
	if color, ok := parseHexColor(name); ok {
		return color, true
	}
	switch name {
	case "background", "canvas":
		return t.Background, true
	case "surface", "chrome":
		return t.Surface, true
	case "raised", "hover":
		return t.SurfaceHover, true
	case "border":
		return t.Border, true
	case "text", "foreground":
		return t.Text, true
	case "muted", "dim":
		return t.TextMuted, true
	case "accent":
		return t.Accent, true
	case "danger":
		return t.Danger, true
	case "warning":
		return t.Warning, true
	case "success":
		return t.Success, true
	default:
		return mui.Color{}, false
	}
}

// --- Floating blocks ---

// floatLayout is one window's floating blocks: where each was dragged and
// which the user closed.
type floatLayout struct {
	mu     sync.Mutex
	pos    map[string]*mui.Point
	closed map[string]bool
}

var (
	floatLayoutMu sync.Mutex
	floatLayouts  = make(map[*Shell]*floatLayout)
)

func floatLayoutFor(s *Shell) *floatLayout {
	floatLayoutMu.Lock()
	defer floatLayoutMu.Unlock()
	fl := floatLayouts[s]
	if fl == nil {
		fl = &floatLayout{pos: make(map[string]*mui.Point), closed: make(map[string]bool)}
		floatLayouts[s] = fl
	}
	return fl
}

// resetFloating forgets every closed or dragged floating block.
func resetFloating(s *Shell) {
	fl := floatLayoutFor(s)
	fl.mu.Lock()
	fl.pos = make(map[string]*mui.Point)
	fl.closed = make(map[string]bool)
	fl.mu.Unlock()
}

// layoutFloats draws a node's floating blocks over the node's bounds.
func (s *Shell) layoutFloats(c *mui.Context, n api.Layout, bounds mui.Rect, st api.LayoutState) {
	if s.overlayOpen() || len(n.Floating) == 0 {
		return
	}
	if bounds.W <= 0 || bounds.H <= 0 {
		w, h := c.Size()
		bounds = mui.Rect{W: w, H: h}
	}
	fl := floatLayoutFor(s)
	for _, f := range n.Floating {
		if layoutFloatHidden(f, st) {
			continue
		}
		key := n.ID + "/" + f.ID
		fl.mu.Lock()
		if fl.closed[key] {
			fl.mu.Unlock()
			continue
		}
		pos := fl.pos[key]
		if pos == nil {
			pos = new(mui.Point)
			fl.pos[key] = pos
		}
		fl.mu.Unlock()
		s.layoutFloat(c, f, bounds, key, pos)
	}
}

func layoutFloatHidden(f api.LayoutFloat, st api.LayoutState) bool {
	if hidden, ok := st.Hidden[f.ID]; ok {
		return hidden
	}
	return false
}

// layoutFloat draws one floating block with its frame, close button and
// drag handle.
func (s *Shell) layoutFloat(c *mui.Context, f api.LayoutFloat, bounds mui.Rect, key string, pos *mui.Point) {
	t := c.Theme()
	w := f.Width
	if w <= 0 {
		w = 280
	}
	h := f.Height
	if h <= 0 {
		h = 220
	}
	w = min(w, max(bounds.W-24, 80))
	h = min(h, max(bounds.H-24, 60))
	x, y := floatOrigin(f, bounds, w, h)
	x += pos.X
	y += pos.Y
	title := f.ID
	if f.Content != nil && f.Content.Mod != "" {
		title = f.Content.Mod
	}
	mui.Overlay(c, func() {
		panel := mui.Column(c).
			Absolute().
			Left(x).
			Top(y).
			Width(w).
			Height(h).
			Background(t.Surface).
			Border(1, t.Border).
			Radius(10).
			Shadow(0, 8, 24, 0, mui.RGBA(0, 0, 0, 0.28)).
			Clip().
			Draggable(pos, mui.DraggableOptions{
				Bounds: &mui.Rect{X: -bounds.W, Y: -bounds.H, W: bounds.W * 3, H: bounds.H * 3},
				Rigid:  true,
			})
		panel.Children(func() {
			mui.Row(c).
				Height(30).
				Padding(0, 10).
				Gap(8).
				AlignItems(mui.Center).
				Background(t.SurfaceHover).
				Children(func() {
					mui.Text(c, title).FontSize(t.FontSize - 2).Bold().SingleLine()
					mui.Box(c).Grow(1)
					closeIconButton(c, "shell/float/close/"+key, "Close "+title, func() {
						fl := floatLayoutFor(s)
						fl.mu.Lock()
						fl.closed[key] = true
						fl.mu.Unlock()
					})
				})
			mui.Box(c).Height(1).Background(t.Border)
			mui.Box(c).Fill().Children(func() { s.layoutContent(c, f.Content, s.activeSpace(), s.activeTabPtr()) })
		})
	})
}

// floatOrigin places a floating block along its edge and alignment.
func floatOrigin(f api.LayoutFloat, bounds mui.Rect, w, h float32) (float32, float32) {
	const inset float32 = 12
	edge := f.Edge
	if edge == "" {
		edge = "right"
	}
	align := f.Align
	if align == "" {
		align = "start"
	}
	x, y := bounds.X+inset, bounds.Y+inset
	switch edge {
	case "left":
		x = bounds.X + inset
		y = floatAlign(align, bounds.Y, bounds.H, h, inset)
	case "right":
		x = bounds.X + bounds.W - w - inset
		y = floatAlign(align, bounds.Y, bounds.H, h, inset)
	case "top":
		y = bounds.Y + inset
		x = floatAlign(align, bounds.X, bounds.W, w, inset)
	case "bottom":
		y = bounds.Y + bounds.H - h - inset
		x = floatAlign(align, bounds.X, bounds.W, w, inset)
	}
	return x, y
}

func floatAlign(align string, start, span, size, inset float32) float32 {
	switch align {
	case "center":
		return start + (span-size)/2
	case "end":
		return start + span - size - inset
	default:
		return start + inset
	}
}

// --- Terminal display settings ---

// terminalOptions builds the options for a newly attached terminal,
// including the display settings the terminal takes at creation.
func terminalOptions(theme *terminal.Theme, font terminal.Font, settings map[string]any) terminal.Options {
	cursor, noBlink := terminalCursor(settings)
	return terminal.Options{
		ResizeByConn:     false,
		Font:             font,
		Theme:            theme,
		OptionAsAlt:      optionAsAlt(settings),
		MinimumContrast:  settingFloat(settings, "minimum_contrast", 1),
		ClipboardRead:    terminalClipboardRead(settings),
		NoClipboardWrite: !settingBool(settings, "clipboard_write", true),
		Padding:          terminalPadding(settings),
		Cursor:           cursor,
		NoBlink:          noBlink,
		Transparent:      terminalTransparent(settings),
	}
}

// terminalDisplay remembers the display settings applied to a terminal.
var terminalDisplay sync.Map // *terminal.Terminal -> string

// applyTerminalDisplay applies the terminal display settings. A setting the
// view cannot change live (padding, cursor, scrollbars) makes the visible
// terminals attach again; the rest reach the attached terminals at once.
func (s *Shell) applyTerminalDisplay(c *mui.Context, settings map[string]any) {
	key := fmt.Sprintf("%v|%v|%v|%v|%v|%v|%v|%v|%v|%v",
		settingBool(settings, "ligatures", false),
		settingNumber(settings, "opacity", 1),
		settingNumber(settings, "padding_x", 10),
		settingNumber(settings, "padding_y", 6),
		settingString(settings, "cursor_shape"),
		settingBool(settings, "cursor_blink", true),
		settingBool(settings, "pane_scrollbars", true),
		settingString(settings, "terminal_font"),
		terminalFontSize(settings),
		settingString(settings, "theme"))
	if s.displayKey != "" && s.displayKey != key {
		s.dropAttachments()
	}
	s.displayKey = key
	theme, font := terminalLook(c.Theme(), settings)
	s.attachmentsMu.Lock()
	terms := make([]*terminal.Terminal, 0, len(s.attachments))
	for _, ts := range s.attachments {
		if ts.term != nil {
			terms = append(terms, ts.term)
		}
	}
	s.attachmentsMu.Unlock()
	for _, term := range terms {
		if last, ok := terminalDisplay.Load(term); ok && last.(string) == key {
			continue
		}
		terminalDisplay.Store(term, key)
		term.SetTheme(theme, nil)
		term.SetFont(font)
	}
}

// visibleTerminals returns the terminals the shown layout draws. A user
// layout's tree decides: a tab block shows the active tab's panes and a
// terminal block shows that address. The built-in layouts show the tab.
func (s *Shell) visibleTerminals(tab *api.Tab, zoom string, settings map[string]any) map[string]bool {
	name := s.layoutName(settings)
	if _, builtin := api.BuiltinLayout(name); builtin {
		return visibleLeaves(tab, zoom)
	}
	tree, st, ok := s.layoutTree(name)
	if !ok {
		return visibleLeaves(tab, zoom)
	}
	out := make(map[string]bool)
	var walk func(api.Layout)
	walk = func(n api.Layout) {
		if layoutNodeHidden(n, st) {
			return
		}
		for _, f := range n.Floating {
			if layoutFloatHidden(f, st) {
				continue
			}
			if f.Content != nil && f.Content.Terminal != "" {
				out[f.Content.Terminal] = true
			}
		}
		switch n.Type {
		case "row", "column":
			for _, child := range n.Children {
				walk(child)
			}
		case "stack":
			active := n.Active
			if v, ok := st.Stacks[n.ID]; ok {
				active = v
			}
			for i := range n.Children {
				if n.Children[i].ID == active {
					walk(n.Children[i])
					return
				}
			}
			for i := range n.Children {
				if !layoutNodeHidden(n.Children[i], st) {
					walk(n.Children[i])
					return
				}
			}
		case "block":
			if n.Content == nil {
				return
			}
			switch {
			case n.Content.Tab == "active" && tab != nil:
				if zoom != "" {
					if z := findNode(tab.Root, zoom); z != nil {
						leaves(z, out)
					}
				} else {
					leaves(tab.Root, out)
				}
			case n.Content.Terminal != "":
				out[n.Content.Terminal] = true
			}
		}
	}
	walk(tree)
	return out
}

// shortPath shortens the home directory in a path shown in a header.
func shortPath(path string) string {
	home, err := os.UserHomeDir()
	if err == nil && home != "" && strings.HasPrefix(path, home) {
		return "~" + strings.TrimPrefix(path, home)
	}
	return path
}

// modPaneNotice says a pane names a mod panel the app does not have.
func (s *Shell) modPaneNotice(c *mui.Context, n *api.Node) {
	t := c.Theme()
	mui.Column(c).Fill().Center().Padding(12).Children(func() {
		mui.Text(c, "Panel "+n.Mod+" is not available").FontSize(t.FontSize - 1).TextColor(t.TextMuted)
	})
}

// --- Terminal lifecycle ---

// terminalInfo returns what the core knows about a terminal.
func (s *Shell) terminalInfo(address string) (api.TerminalInfo, bool) {
	if address == "" {
		return api.TerminalInfo{}, false
	}
	if src, ok := s.app.(terminalInfoSource); ok {
		if info, ok := src.TerminalInfo(address); ok {
			return info, true
		}
	}
	if src, ok := s.app.(terminalInfosSource); ok {
		info, ok := src.TerminalInfos()[address]
		return info, ok
	}
	return api.TerminalInfo{Address: address}, false
}

// terminalFinished reports whether a terminal has ended, by the core's
// state or the attachment's own end.
func terminalFinished(info api.TerminalInfo, ts attachSnapshot) bool {
	switch info.State {
	case "finished", "ended", "exited", "dead":
		return true
	}
	if info.ExitCode != nil {
		return true
	}
	if ts.term != nil {
		select {
		case <-ts.term.Done():
			return true
		default:
		}
	}
	return false
}

// terminalExitCode is the terminal's exit code, from the core or the
// attached terminal.
func terminalExitCode(info api.TerminalInfo, ts attachSnapshot) (int, bool) {
	if info.ExitCode != nil {
		return *info.ExitCode, true
	}
	if ts.term != nil {
		select {
		case <-ts.term.Done():
			return ts.term.ExitCode(), true
		default:
		}
	}
	return 0, false
}

// endedTerminal draws the Restart and Close card of a finished terminal in
// place of the terminal's own disconnected screen.
func (s *Shell) endedTerminal(c *mui.Context, n *api.Node, info api.TerminalInfo, ts attachSnapshot) {
	t := c.Theme()
	title := info.Title
	if title == "" {
		title = n.Terminal
	}
	mui.Column(c).Fill().Center().Padding(16).Gap(6).Children(func() {
		mui.Text(c, title).FontSize(t.FontSize - 1).Bold().SingleLine()
		if info.Cwd != "" {
			mui.Text(c, info.Cwd).FontSize(t.FontSize - 2).TextColor(t.TextMuted).SingleLine()
		}
		label := "Terminal ended"
		if code, ok := terminalExitCode(info, ts); ok {
			label = fmt.Sprintf("Exited with code %d", code)
		}
		mui.Text(c, label).FontSize(t.FontSize - 2).TextColor(t.TextMuted)
		mui.Row(c).Gap(8).Children(func() {
			mui.Button(c, "Restart").OnClick(func() { s.restartTerminal(n, info) })
			mui.Button(c, "Close").OnClick(func() { s.closeEndedTerminal(n) })
		})
	})
}

// restartTerminal starts a new terminal in the ended one's folder.
func (s *Shell) restartTerminal(n *api.Node, info api.TerminalInfo) {
	if n == nil {
		return
	}
	sp, _ := s.activeTab()
	folder := info.Cwd
	if folder == "" && sp != nil {
		folder = sp.Folder
	}
	if s.hasCommand("terminal.open") {
		err := s.app.Run("terminal.open", map[string]any{"terminal": n.Terminal, "folder": folder})
		if err == nil {
			s.dropAttachments()
			return
		}
	}
	args := map[string]any{"folder": folder}
	if sp != nil {
		args["space"] = sp.ID
	}
	s.run(api.CmdTabNew, args)
	s.dropAttachments()
}

// closeEndedTerminal closes the pane showing a finished terminal.
func (s *Shell) closeEndedTerminal(n *api.Node) {
	if n == nil {
		return
	}
	s.run(api.CmdPaneClose, map[string]any{"pane": n.ID, "terminal": n.Terminal})
}

func (s *Shell) hasCommand(id string) bool {
	for _, cmd := range s.app.Commands() {
		if cmd.ID == id {
			return true
		}
	}
	return false
}

// syncConnection detaches attachments when the core link is lost, and
// clears the attachment table after it comes back so visible terminals
// attach again.
func (s *Shell) syncConnection() {
	conn := s.app.Connection()
	if conn == s.lastConn {
		return
	}
	s.lastConn = conn
	s.dropAttachments()
}

// dropAttachments detaches every attachment and forgets it, so the next
// frame attaches the visible terminals again.
func (s *Shell) dropAttachments() {
	s.attachmentsMu.Lock()
	terms := s.attachments
	s.attachments = make(map[string]*paneTerm)
	s.attachmentsMu.Unlock()
	for _, ts := range terms {
		detachPaneTerm(ts)
	}
}

// activeSpace returns the active space, or nil.
func (s *Shell) activeSpace() *api.Space {
	sp, _ := s.activeTab()
	return sp
}

// activeTabPtr returns the active tab, or nil.
func (s *Shell) activeTabPtr() *api.Tab {
	_, tab := s.activeTab()
	return tab
}

// --- Help, welcome and update overlays ---

// consumeLayoutOverlays turns the overlay a command asked for into a shell
// flag and clears the request.
func (s *Shell) consumeLayoutOverlays() {
	switch s.app.Overlay() {
	case "help":
		s.helpOpen = true
	case "welcome":
		s.welcomeOpen = true
	case "install_update":
		s.installUpdateOpen = true
	case "find":
		s.findOpen = true
	default:
		return
	}
	s.app.SetOverlay("")
}

// layoutOverlays draws the help, welcome and install overlays.
func (s *Shell) layoutOverlays(c *mui.Context) {
	if s.helpOpen {
		mui.Modal(c, &s.helpOpen, func() { s.helpOverlay(c) })
	}
	if s.welcomeOpen {
		mui.Modal(c, &s.welcomeOpen, func() { s.welcomeOverlay(c) })
	}
	if s.installUpdateOpen {
		mui.Modal(c, &s.installUpdateOpen, func() { s.installUpdateOverlay(c) })
	}
}

// helpOverlay lists the app's commands.
func (s *Shell) helpOverlay(c *mui.Context) {
	t := c.Theme()
	cmds := s.app.Commands()
	mui.Column(c).Width(600).MaxHeight(560).Gap(10).Children(func() {
		mui.Row(c).AlignItems(mui.Center).Children(func() {
			mui.Text(c, "Help").FontSize(t.FontSize + 2).Bold()
			mui.Box(c).Grow(1)
			mui.Button(c, "Close").OnClick(func() { s.helpOpen = false })
		})
		mui.Text(c, s.app.Version()+" · "+s.app.Connection()).
			FontSize(t.FontSize - 2).TextColor(t.TextMuted)
		mui.Scroll(c).MaxHeight(460).Children(func() {
			mui.Column(c).Gap(2).Children(func() {
				for _, cmd := range cmds {
					label := cmd.Title
					if label == "" {
						label = cmd.ID
					}
					mui.Row(c).Height(24).Gap(8).AlignItems(mui.Center).Children(func() {
						mui.Text(c, label).FontSize(t.FontSize - 1).SingleLine()
						mui.Box(c).Grow(1)
						mui.Text(c, cmd.ID).FontSize(t.FontSize - 3).TextColor(t.TextMuted)
					})
				}
				if len(cmds) == 0 {
					mui.Text(c, "No commands").FontSize(t.FontSize - 1).TextColor(t.TextMuted)
				}
			})
		})
		mui.Text(c, "Mod+P opens the palette · Esc closes a dialog").
			FontSize(t.FontSize - 3).TextColor(t.TextMuted)
	})
}

// welcomeOverlay is the first-run introduction.
func (s *Shell) welcomeOverlay(c *mui.Context) {
	t := c.Theme()
	mui.Column(c).Width(560).Gap(10).Children(func() {
		mui.Text(c, "Welcome to hiik").FontSize(t.FontSize + 5).Bold()
		mui.Text(c, "One window for spaces, agents and terminals.").
			FontSize(t.FontSize - 1).TextColor(t.TextMuted)
		mui.Column(c).Gap(4).Children(func() {
			for _, line := range []string{
				"Mod+P: commands",
				"Mod+Shift+T: new tab",
				"Mod+Shift+B: sidebar",
				"Mod+Shift+I: inspector",
			} {
				mui.Text(c, line).FontSize(t.FontSize - 1)
			}
		})
		mui.Text(c, "Layouts: "+strings.Join(LayoutNames(), ", ")).
			FontSize(t.FontSize - 2).TextColor(t.TextMuted).SingleLine()
		mui.Row(c).Justify(mui.End).Children(func() {
			mui.Button(c, "Start").OnClick(func() { s.welcomeOpen = false })
		})
	})
}

// installUpdateOverlay offers the downloaded package.
func (s *Shell) installUpdateOverlay(c *mui.Context) {
	t := c.Theme()
	version, path := s.pendingInstall()
	mui.Column(c).Width(540).Gap(10).Children(func() {
		title := "Update ready"
		if version != "" {
			title = "hiik " + version + " is ready"
		}
		mui.Text(c, title).FontSize(t.FontSize + 2).Bold()
		mui.Text(c, "hiik downloaded the package. Install it, then quit hiik.").
			FontSize(t.FontSize - 1).TextColor(t.TextMuted)
		if path != "" {
			mui.Text(c, path).FontSize(t.FontSize - 2).SingleLine()
		}
		mui.Row(c).Gap(8).Justify(mui.End).Children(func() {
			mui.Button(c, "Install").OnClick(func() { s.installPackage() })
			if path != "" {
				mui.Button(c, "Open folder").OnClick(func() {
					dir := filepath.Dir(path)
					s.services.OpenURL("file://" + dir)
				})
			}
			mui.Button(c, "Quit").OnClick(func() {
				s.installUpdateOpen = false
				s.run(api.CmdQuit, nil)
			})
			mui.Button(c, "Close").OnClick(func() { s.installUpdateOpen = false })
		})
	})
}

// installPackage opens the downloaded package and quits the app so the
// package can replace it.
func (s *Shell) installPackage() {
	type opener interface{ OpenInstallPackage() error }
	if op, ok := s.app.(opener); ok {
		if err := op.OpenInstallPackage(); err != nil {
			s.toast = err.Error()
			return
		}
		s.installUpdateOpen = false
		s.run(api.CmdQuit, nil)
		return
	}
	s.run("app.install_update", nil)
}

// pendingInstall reads the pending plan through an optional interface that
// keeps ui free of a core import. It uses reflection because the plan type
// is core's.
func (s *Shell) pendingInstall() (version, path string) {
	value := reflect.ValueOf(s.app)
	if !value.IsValid() || value.Kind() != reflect.Pointer || value.IsNil() {
		return "", ""
	}
	method := value.MethodByName("PendingInstall")
	if !method.IsValid() || method.Type().NumOut() != 2 {
		return "", ""
	}
	out := method.Call(nil)
	if len(out) != 2 || out[1].Kind() != reflect.Bool || !out[1].Bool() {
		return "", ""
	}
	plan := out[0]
	if plan.Kind() == reflect.Pointer {
		if plan.IsNil() {
			return "", ""
		}
		plan = plan.Elem()
	}
	if plan.Kind() != reflect.Struct {
		return "", ""
	}
	if f := plan.FieldByName("Version"); f.IsValid() && f.Kind() == reflect.String {
		version = f.String()
	}
	if f := plan.FieldByName("Path"); f.IsValid() && f.Kind() == reflect.String {
		path = f.String()
	}
	return version, path
}
