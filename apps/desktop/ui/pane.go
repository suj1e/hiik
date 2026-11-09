package ui

import (
	"context"
	"sync"
	"time"

	"github.com/egoist/mygo/plugins/terminal"
	mui "github.com/egoist/mygo/ui"

	"hiik/core/hc"
	"hiik/mods/api"
)

// attachSnapshot is what the view knows of one terminal attachment.
type attachSnapshot struct {
	term *terminal.Terminal
	err  string
}

// paneTerm is one terminal the shell attached, or the state of attaching it.
type paneTerm struct {
	loading bool
	err     string
	term    *terminal.Terminal
	at      *hc.Attachment
	cols    int
	rows    int
	retryAt time.Time
}

// panes draws the active tab's pane tree.
func (s *Shell) panes(c *mui.Context, tab *api.Tab) {
	if s.zoom != "" {
		if n := findNode(tab.Root, s.zoom); n != nil && n.Terminal != "" {
			s.pane(c, n)
			return
		}
		s.zoom = ""
	}
	s.paneNode(c, tab.ID, tab.Root)
}

// paneNode draws a split or one pane of the tree.
func (s *Shell) paneNode(c *mui.Context, tabID string, n *api.Node) {
	if n == nil {
		return
	}
	if n.A == nil || n.B == nil {
		s.pane(c, n)
		return
	}
	pct := s.splitPercent(tabID, n)
	var el mui.Element
	if n.Vertical {
		el = mui.SplitVerticalPercent(c, pct, func() { s.paneNode(c, tabID, n.A) }, func() { s.paneNode(c, tabID, n.B) })
	} else {
		el = mui.SplitPercent(c, pct, func() { s.paneNode(c, tabID, n.A) }, func() { s.paneNode(c, tabID, n.B) })
	}
	el.Fill()
	if el.Changed() {
		s.run(api.CmdPaneResize, map[string]any{"pane": n.ID, "id": n.ID, "ratio": *pct / 100})
	}
}

// splitPercent returns the split's first-share in percent, kept across
// frames so a dragged divider retains its size. The model keeps the ratio
// (0 to 1), which the split writes back after the frame.
func (s *Shell) splitPercent(tabID string, n *api.Node) *float32 {
	key := tabID + "/" + n.ID
	if p := s.splitPct[key]; p != nil {
		return p
	}
	v := n.Ratio * 100
	if v <= 0 || v >= 100 {
		v = 50
	}
	p := new(float32)
	*p = v
	s.splitPct[key] = p
	return p
}

// pane draws one terminal card.
func (s *Shell) pane(c *mui.Context, n *api.Node) {
	t := c.Theme()
	focused := n.ID == s.focus && !s.overlayOpen()
	border := t.Border
	if focused {
		border = t.Accent
	}
	card := mui.Column(c).
		Fill().
		Background(t.Surface).
		Border(1, border).
		Radius(6).
		Clip()
	card.Children(func() {
		head := mui.Row(c).
			Height(28).
			Padding(0, 8).
			Gap(6).
			AlignItems(mui.Center).
			OnClick(func() { s.setFocus(n.ID) })
		head.ContextMenu(func(m *mui.Menu) { s.paneMenu(m, n) })
		if focused {
			head.Background(t.SurfaceHover)
		}
		head.Children(func() {
			props := map[string]any{
				"node":     n,
				"id":       n.ID,
				"address":  n.Terminal,
				"terminal": n.Terminal,
				"focused":  focused,
				"zoomed":   s.zoom == n.ID,
			}
			if info, ok := s.terminalInfo(n.Terminal); ok {
				props["title"] = info.Title
				props["cwd"] = info.Cwd
				props["state"] = info.State
				props["pinned"] = info.Pinned
				if info.ExitCode != nil {
					props["exit_code"] = *info.ExitCode
				}
			}
			s.renderPanel(c, api.PanelPaneHead, props)
		})
		body := mui.Box(c).Fill().OnClick(func() { s.setFocus(n.ID) })
		body.Children(func() {
			if n.Mod != "" {
				if !renderModPane(c, n) {
					s.modPaneNotice(c, n)
				}
				return
			}
			s.paneBody(c, n)
		})
	})
}

// paneBody draws the terminal, or why there is none yet.
func (s *Shell) paneBody(c *mui.Context, n *api.Node) {
	t := c.Theme()
	if n.Terminal == "" {
		mui.Column(c).Fill().Center().Children(func() {
			mui.Text(c, "No terminal").FontSize(12).TextColor(t.TextMuted)
		})
		return
	}
	ts := s.attachment(n.Terminal)
	info, hasInfo := s.terminalInfo(n.Terminal)
	if hasInfo && terminalFinished(info, ts) {
		s.endedTerminal(c, n, info, ts)
		return
	}
	if ts.term == nil {
		msg := "starting"
		if ts.err != "" {
			msg = ts.err
		}
		mui.Column(c).Fill().Center().Padding(12).Children(func() {
			mui.Text(c, msg).FontSize(12).TextColor(t.TextMuted).SingleLine()
		})
		return
	}
	if terminalFinished(api.TerminalInfo{}, ts) {
		s.endedTerminal(c, n, info, ts)
		return
	}
	applyMinimumContrast(ts.term, s.minimumContrast())
	e := terminal.View(c, ts.term).Fill().Key(n.ID)
	if !paneScrollbars(s.app.Settings()) {
		if b := e.Bounds(); b.W > 12 && b.H > 12 {
			mui.Overlay(c, func() {
				mui.Box(c).
					Absolute().
					Left(b.X + b.W - 8).
					Top(b.Y).
					Width(8).
					Height(b.H).
					Background(t.Background)
			})
		}
	}
	switch {
	case s.focus == n.ID && s.focusPending && !s.overlayOpen():
		e = e.Focus()
		s.focusPending = false
	case e.Focused() && s.focus != n.ID && !s.overlayOpen():
		s.adoptFocus(n.ID)
	}
}

// findNode returns the node with id in the tree.
func findNode(n *api.Node, id string) *api.Node {
	if n == nil {
		return nil
	}
	if n.ID == id {
		return n
	}
	if got := findNode(n.A, id); got != nil {
		return got
	}
	return findNode(n.B, id)
}

// leaves collects the terminal addresses of a tree's leaves.
func leaves(n *api.Node, out map[string]bool) {
	if n == nil {
		return
	}
	if n.Terminal != "" && (n.A == nil || n.B == nil) {
		out[n.Terminal] = true
		return
	}
	leaves(n.A, out)
	leaves(n.B, out)
}

// firstLeaf returns the first terminal address and node ID of a tree.
func firstLeaf(n *api.Node) (string, string) {
	if n == nil {
		return "", ""
	}
	if n.Terminal != "" && (n.A == nil || n.B == nil) {
		return n.Terminal, n.ID
	}
	if addr, id := firstLeaf(n.A); addr != "" {
		return addr, id
	}
	return firstLeaf(n.B)
}

// visibleLeaves returns the addresses of the panes the active tab shows.
func visibleLeaves(tab *api.Tab, zoom string) map[string]bool {
	out := make(map[string]bool)
	if tab == nil || tab.Root == nil {
		return out
	}
	if zoom != "" {
		if n := findNode(tab.Root, zoom); n != nil {
			leaves(n, out)
			return out
		}
	}
	leaves(tab.Root, out)
	return out
}

// ensureFocus keeps the shell's focus on a pane of the active tab.
func (s *Shell) ensureFocus(tab *api.Tab) {
	if tab == nil || tab.Root == nil {
		return
	}
	if s.zoom != "" {
		if z := findNode(tab.Root, s.zoom); z != nil {
			if z.A == nil || z.B == nil {
				if s.focus != z.ID {
					s.focus = z.ID
					s.focusPending = true
				}
				return
			}
		} else {
			s.zoom = ""
		}
	}
	if s.focus != "" && findNode(tab.Root, s.focus) != nil {
		return
	}
	id := tab.Focus
	if id == "" || findNode(tab.Root, id) == nil {
		_, id = firstLeaf(tab.Root)
	}
	s.focus = id
	s.focusPending = true
}

// setFocus focuses a pane and records it.
func (s *Shell) setFocus(id string) {
	if id == "" || id == s.focus {
		return
	}
	s.focus = id
	s.focusPending = true
}

// adoptFocus records a pane the user focused by clicking its terminal.
func (s *Shell) adoptFocus(id string) {
	if id == "" {
		return
	}
	s.focus = id
	s.focusPending = false
}

// attachment returns a copy of the attachment state for an address.
func (s *Shell) attachment(address string) attachSnapshot {
	s.attachmentsMu.Lock()
	defer s.attachmentsMu.Unlock()
	ts := s.attachments[address]
	if ts == nil {
		return attachSnapshot{}
	}
	return attachSnapshot{term: ts.term, err: ts.err}
}

// minimumContrast is the least contrast terminal text has with its
// background, from the settings.
func (s *Shell) minimumContrast() float32 {
	return settingFloat(s.app.Settings(), "minimum_contrast", 1)
}

// terminalContrast remembers the contrast last given to each terminal, so
// a live change is applied once and not every frame.
var terminalContrast sync.Map // *terminal.Terminal -> float32

// applyMinimumContrast changes an attached terminal's least contrast when
// the setting changed. New terminals get it in terminal.Options.
func applyMinimumContrast(term *terminal.Terminal, ratio float32) {
	if term == nil {
		return
	}
	if ratio < 1 || ratio > 21 {
		ratio = 1
	}
	if last, ok := terminalContrast.Load(term); ok && last.(float32) == ratio {
		return
	}
	terminalContrast.Store(term, ratio)
	term.SetMinimumContrast(ratio)
}

// terminalClipboardRead turns the settings into the terminal's clipboard
// read policy: reads stay refused unless the user opts in.
func terminalClipboardRead(settings map[string]any) terminal.ClipboardAccess {
	if allowed, _ := settings["clipboard_read"].(bool); allowed {
		return terminal.ClipboardAllow
	}
	return terminal.ClipboardDeny
}

// syncAttachments attaches visible terminals and detaches the rest. It never
// waits for the core: each attach runs in its own goroutine.
func (s *Shell) syncAttachments(visible map[string]bool) {
	s.attachmentsMu.Lock()
	var starts []*paneTerm
	var addrs []string
	for addr, ts := range s.attachments {
		if !visible[addr] {
			if ts.term != nil {
				if cols, rows := ts.term.Size(); cols > 0 && rows > 0 {
					s.termSizes[addr] = [2]int{cols, rows}
				}
			}
			delete(s.attachments, addr)
			go detachPaneTerm(ts)
		}
	}
	for addr := range visible {
		ts := s.attachments[addr]
		if ts == nil {
			size := s.termSizes[addr]
			ts = &paneTerm{loading: true, cols: size[0], rows: size[1]}
			s.attachments[addr] = ts
			starts = append(starts, ts)
			addrs = append(addrs, addr)
			continue
		}
		if ts.loading || ts.term != nil {
			continue
		}
		if time.Now().Before(ts.retryAt) {
			continue
		}
		ts.loading = true
		starts = append(starts, ts)
		addrs = append(addrs, addr)
	}
	s.attachmentsMu.Unlock()
	for i, ts := range starts {
		go s.attach(addrs[i], ts)
	}
}

// attach opens one terminal stream and creates its terminal view.
func (s *Shell) attach(address string, ts *paneTerm) {
	core := s.app.Core()
	if core == nil {
		s.attachmentsMu.Lock()
		ts.loading = false
		ts.retryAt = retryAfter(time.Second)
		s.attachmentsMu.Unlock()
		s.wake()
		return
	}
	cols, rows := ts.cols, ts.rows
	if cols <= 0 || rows <= 0 {
		cols, rows = 80, 24
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	at, err := core.Attach(ctx, hc.AttachRequest{
		Terminal: address,
		Cols:     cols,
		Rows:     rows,
		History:  2000,
	})
	cancel()
	if err != nil {
		s.failAttach(address, ts, err)
		return
	}
	theme, font := s.terminalLookFor()
	settings := s.app.Settings()
	opts := terminalOptions(theme, font, settings)
	opts.Conn = at.Conn
	opts.RightClick = rightClickAction(s.rightClickPolicy(address))
	contrast := opts.MinimumContrast
	term, err := terminal.New(opts)
	if err != nil {
		if at.Close != nil {
			_ = at.Close()
		}
		s.failAttach(address, ts, err)
		return
	}
	terminalContrast.Store(term, contrast)
	s.attachmentsMu.Lock()
	current := s.attachments[address] == ts
	if current {
		ts.term = term
		ts.at = at
		ts.loading = false
		ts.err = ""
	}
	s.attachmentsMu.Unlock()
	if !current {
		_ = term.Close()
		if at.Close != nil {
			_ = at.Close()
		}
		return
	}
	s.wake()
}

func (s *Shell) failAttach(address string, ts *paneTerm, err error) {
	s.attachmentsMu.Lock()
	if s.attachments[address] == ts {
		ts.loading = false
		ts.err = err.Error()
		ts.retryAt = retryAfter(3 * time.Second)
	}
	s.attachmentsMu.Unlock()
	s.wake()
}

// detachPaneTerm closes a terminal and its stream.
func detachPaneTerm(ts *paneTerm) {
	if ts == nil {
		return
	}
	if ts.term != nil {
		terminalContrast.Delete(ts.term)
		_ = ts.term.Close()
		ts.term = nil
	}
	if ts.at != nil {
		if ts.at.Close != nil {
			_ = ts.at.Close()
		}
		ts.at = nil
	}
}

// wake redraws the window after an attachment changes; it is safe outside
// the build pass and in tests without a window.
func (s *Shell) wake() {
	if s.win != nil {
		s.win.Invalidate()
	}
}

// moveFocus focuses the nearest pane in a direction, using the tree's
// ratios as they are shown.
func (s *Shell) moveFocus(dir string) {
	_, tab := s.activeTab()
	if tab == nil || tab.Root == nil || s.zoom != "" {
		return
	}
	rects := map[string]paneRect{}
	collectRects(tab.Root, paneRect{0, 0, 1, 1}, rects)
	cur := rects[s.focus]
	if _, ok := rects[s.focus]; !ok {
		return
	}
	best := ""
	var bestDist float32
	for id, r := range rects {
		if id == s.focus || !inDirection(cur, r, dir) {
			continue
		}
		d := centerDistance(cur, r)
		if best == "" || d < bestDist {
			best, bestDist = id, d
		}
	}
	if best != "" {
		s.setFocus(best)
	}
}

// paneRect is a pane's share of the tab, from 0 to 1.
type paneRect struct{ X, Y, W, H float32 }

func (r paneRect) cx() float32 { return r.X + r.W/2 }
func (r paneRect) cy() float32 { return r.Y + r.H/2 }

func collectRects(n *api.Node, r paneRect, out map[string]paneRect) {
	if n == nil {
		return
	}
	if n.A == nil || n.B == nil {
		out[n.ID] = r
		return
	}
	ratio := n.Ratio
	if ratio <= 0 || ratio >= 1 {
		ratio = 0.5
	}
	if n.Vertical {
		collectRects(n.A, paneRect{r.X, r.Y, r.W, r.H * ratio}, out)
		collectRects(n.B, paneRect{r.X, r.Y + r.H*ratio, r.W, r.H * (1 - ratio)}, out)
		return
	}
	collectRects(n.A, paneRect{r.X, r.Y, r.W * ratio, r.H}, out)
	collectRects(n.B, paneRect{r.X + r.W*ratio, r.Y, r.W * (1 - ratio), r.H}, out)
}

func inDirection(a, b paneRect, dir string) bool {
	switch dir {
	case "left":
		return b.cx() < a.cx()
	case "right":
		return b.cx() > a.cx()
	case "up":
		return b.cy() < a.cy()
	case "down":
		return b.cy() > a.cy()
	}
	return false
}

func centerDistance(a, b paneRect) float32 {
	dx := a.cx() - b.cx()
	dy := a.cy() - b.cy()
	return dx*dx + dy*dy
}
