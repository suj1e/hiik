package hc

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/egoist/mygo/plugins/terminal"
)

// initialCredit is the window the viewer starts each attach stream with.
const initialCredit = 1 << 20

// attachResult is the reply to terminal.attach.
type attachResult struct {
	Channel             uint32   `json:"channel"`
	Terminal            Terminal `json:"terminal"`
	ResizeNotifications bool     `json:"resize_notifications"`
	AutomaticInput      bool     `json:"automatic_input"`
}

// Attach opens a live stream for one terminal.
func (c *conn) Attach(ctx context.Context, req AttachRequest) (*Attachment, error) {
	params := map[string]any{
		"terminal":             req.Terminal,
		"cols":                 req.Cols,
		"rows":                 req.Rows,
		"history":              req.History,
		"resize_notifications": true,
		"observe":              false,
	}
	if d := displayParams(req.Display); len(d) > 0 {
		params["display"] = d
	}
	var res attachResult
	if err := c.Call(ctx, "terminal.attach", params, &res); err != nil {
		return nil, err
	}
	st := c.newStream(res.Channel)
	a := &attachConn{
		c:       c,
		channel: res.Channel,
		stream:  st,
		display: req.Display,
	}
	if err := c.sendCredit(res.Channel, initialCredit); err != nil {
		c.removeStream(res.Channel)
		st.closeLocal()
		return nil, err
	}
	at := &Attachment{
		Channel:  res.Channel,
		Terminal: res.Terminal,
		Conn:     a,
		Close:    a.Close,
	}
	return at, nil
}

// attachConn is the io.ReadWriteCloser for one attach channel. The concrete
// type also has Resize, ResizePixels and UseTheme, which the terminal plugin
// detects.
type attachConn struct {
	c       *conn
	channel uint32
	stream  *stream

	mu      sync.Mutex
	buf     []byte
	display Display
	closed  bool
}

// Read returns stream bytes and credits the core for exactly the bytes
// returned.
func (a *attachConn) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	for {
		a.mu.Lock()
		if len(a.buf) > 0 {
			n := copy(p, a.buf)
			a.buf = a.buf[n:]
			a.mu.Unlock()
			_ = a.c.sendCredit(a.channel, n)
			return n, nil
		}
		if a.closed {
			a.mu.Unlock()
			return 0, io.EOF
		}
		a.mu.Unlock()

		f, err := a.stream.next()
		if err != nil {
			if errors.Is(err, ErrLost) {
				a.mu.Lock()
				a.closed = true
				a.mu.Unlock()
			}
			return 0, err
		}
		switch f.kind {
		case kindData:
			if len(f.payload) == 0 {
				continue
			}
			a.mu.Lock()
			a.buf = f.payload
			a.mu.Unlock()
		case kindClose:
			a.mu.Lock()
			a.closed = true
			a.buf = nil
			a.mu.Unlock()
			a.c.removeStream(a.channel)
			return 0, io.EOF
		}
	}
}

// Write sends keys or paste data to the terminal.
func (a *attachConn) Write(p []byte) (int, error) {
	if a.isClosed() {
		return 0, io.ErrClosedPipe
	}
	if len(p) == 0 {
		return 0, nil
	}
	if err := a.c.writeFrame(a.channel, kindData, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

// Resize tells the core the viewer's grid size.
func (a *attachConn) Resize(cols, rows int) error {
	if cols < 0 || cols > 65535 || rows < 0 || rows > 65535 {
		return fmt.Errorf("hc: resize %dx%d is outside uint16", cols, rows)
	}
	if a.isClosed() {
		return io.ErrClosedPipe
	}
	var b [4]byte
	binary.LittleEndian.PutUint16(b[0:2], uint16(cols))
	binary.LittleEndian.PutUint16(b[2:4], uint16(rows))
	return a.c.writeFrame(a.channel, kindResize, b[:])
}

// ResizePixels tells the core the grid size and the size of one cell.
func (a *attachConn) ResizePixels(cols, rows, cellW, cellH int) error {
	if err := a.Resize(cols, rows); err != nil {
		return err
	}
	a.mu.Lock()
	a.display.Cell = [2]int{cellW, cellH}
	facts := a.factsLocked()
	a.mu.Unlock()
	return a.sendFacts(facts)
}

// UseTheme tells the core the viewer's colors and cell size.
func (a *attachConn) UseTheme(theme *terminal.Theme) error {
	a.mu.Lock()
	if theme != nil {
		a.display.Bg = hexColor(theme.Background.R, theme.Background.G, theme.Background.B)
		a.display.Fg = hexColor(theme.Foreground.R, theme.Foreground.G, theme.Foreground.B)
		a.display.Cursor = hexColor(theme.Cursor.R, theme.Cursor.G, theme.Cursor.B)
	}
	facts := a.factsLocked()
	a.mu.Unlock()
	return a.sendFacts(facts)
}

// Close detaches the stream. It is idempotent and unblocks Read.
func (a *attachConn) Close() error {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return nil
	}
	a.closed = true
	a.buf = nil
	a.mu.Unlock()

	// Unblock Read before the (possibly blocking) detach write.
	a.stream.closeLocal()
	err := a.c.writeFrame(a.channel, kindClose, nil)
	a.c.removeStream(a.channel)
	return err
}

func (a *attachConn) isClosed() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.closed
}

// factsLocked returns the Display JSON sent as FACTS. It omits a zero cell
// size so the core keeps what it knew.
func (a *attachConn) factsLocked() []byte {
	b, _ := json.Marshal(displayParams(a.display))
	return b
}

// displayParams turns a Display into the JSON object the core accepts,
// omitting unset fields. A zero cell is not a cell size, so it is omitted.
func displayParams(d Display) map[string]any {
	m := make(map[string]any, 4)
	if d.Bg != "" {
		m["bg"] = d.Bg
	}
	if d.Fg != "" {
		m["fg"] = d.Fg
	}
	if d.Cursor != "" {
		m["cursor"] = d.Cursor
	}
	if d.Cell != [2]int{} {
		m["cell"] = d.Cell
	}
	return m
}

func (a *attachConn) sendFacts(facts []byte) error {
	if a.isClosed() {
		return io.ErrClosedPipe
	}
	return a.c.writeFrame(a.channel, kindFacts, facts)
}

func hexColor(r, g, b uint8) string {
	const digits = "0123456789abcdef"
	out := [7]byte{'#'}
	out[1] = digits[r>>4]
	out[2] = digits[r&0xf]
	out[3] = digits[g>>4]
	out[4] = digits[g&0xf]
	out[5] = digits[b>>4]
	out[6] = digits[b&0xf]
	return string(out[:])
}
