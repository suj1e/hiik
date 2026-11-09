package hc

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"sync/atomic"
)

// ErrLost is returned when the connection to the core ends while a request
// or stream is still live.
var ErrLost = errors.New("lost the core")

// conn is one framed connection. One reader goroutine reads frames and
// demultiplexes channel 0 replies by id and stream frames by channel.
type conn struct {
	hello   Hello
	r       *bufio.Reader
	w       io.Writer
	closeFn func() error

	lost     chan struct{}
	loseOnce sync.Once

	mu      sync.Mutex
	pending map[uint64]*pendingCall
	streams map[uint32]*stream
	// early holds stream frames that arrived between the reply naming the
	// channel and the caller installing the stream.
	early map[uint32][]frame

	nextID    atomic.Uint64
	writeMu   sync.Mutex
	closeOnce sync.Once
	closeErr  error
}

// newConn starts a framed connection over r and w. closeFn releases the
// transport and may be nil in tests.
func newConn(r io.Reader, w io.Writer, hello Hello, closeFn func() error) *conn {
	br, ok := r.(*bufio.Reader)
	if !ok {
		br = bufio.NewReader(r)
	}
	c := &conn{
		hello:   hello,
		r:       br,
		w:       w,
		closeFn: closeFn,
		lost:    make(chan struct{}),
		pending: make(map[uint64]*pendingCall),
		streams: make(map[uint32]*stream),
		early:   make(map[uint32][]frame),
	}
	go c.readLoop()
	return c
}

// Lost is closed once, when the connection ends.
func (c *conn) Lost() <-chan struct{} { return c.lost }

// Hello returns the handshake result.
func (c *conn) Hello() Hello {
	h := c.hello
	h.Methods = append([]string(nil), h.Methods...)
	h.Features = append([]string(nil), h.Features...)
	return h
}

// Close ends the connection. It is idempotent.
func (c *conn) Close() error {
	c.closeOnce.Do(func() {
		if c.closeFn != nil {
			c.closeErr = c.closeFn()
		}
		c.lose(ErrLost)
	})
	return c.closeErr
}

// lose fails every pending call and every stream, then closes Lost.
func (c *conn) lose(err error) {
	c.loseOnce.Do(func() {
		c.mu.Lock()
		pending := c.pending
		streams := c.streams
		c.pending = make(map[uint64]*pendingCall)
		c.streams = make(map[uint32]*stream)
		c.early = make(map[uint32][]frame)
		c.mu.Unlock()

		// Deliver errors before closing Lost so a waiter always sees the
		// failure in its own pending reply.
		for _, p := range pending {
			p.reply(callResult{err: err})
		}
		close(c.lost)
		for _, st := range streams {
			st.fail(err)
		}
	})
}

// readLoop demultiplexes frames until the transport ends.
func (c *conn) readLoop() {
	for {
		f, err := readFrame(c.r)
		if err != nil {
			c.lose(ErrLost)
			return
		}
		if f.channel == 0 {
			c.routeReply(f.payload)
			continue
		}
		c.routeStream(f)
	}
}

// routeReply handles one JSON-RPC message on channel 0.
func (c *conn) routeReply(payload []byte) {
	var msg rpcMessage
	if err := json.Unmarshal(payload, &msg); err != nil {
		return
	}
	if msg.ID == nil {
		// Notifications other than event streams are not used by this
		// client; ignore them.
		return
	}
	c.mu.Lock()
	p := c.pending[*msg.ID]
	if p != nil {
		delete(c.pending, *msg.ID)
	}
	c.mu.Unlock()
	if p == nil {
		return
	}
	if msg.Error != nil {
		msg.Error.Method = p.method
		p.reply(callResult{err: msg.Error})
		return
	}
	p.reply(callResult{result: msg.Result})
}

// routeStream hands a non-control frame to its stream, keeping it aside
// until the caller has installed the stream named by a reply.
func (c *conn) routeStream(f frame) {
	c.mu.Lock()
	st := c.streams[f.channel]
	if st == nil {
		// Keep the bytes in order until newStream adopts them.
		if len(c.early[f.channel]) < 1<<20 {
			f.payload = append([]byte(nil), f.payload...)
			c.early[f.channel] = append(c.early[f.channel], f)
		}
		c.mu.Unlock()
		return
	}
	c.mu.Unlock()
	st.push(f)
}

// newStream installs a stream for channel and adopts any frames that raced
// ahead of the reply.
func (c *conn) newStream(channel uint32) *stream {
	st := newStream()
	c.mu.Lock()
	st.mu.Lock()
	for _, f := range c.early[channel] {
		st.pushLocked(f)
	}
	st.mu.Unlock()
	delete(c.early, channel)
	if old := c.streams[channel]; old != nil {
		old.fail(ErrLost)
	}
	c.streams[channel] = st
	c.mu.Unlock()
	return st
}

// removeStream drops a channel from the demultiplexer.
func (c *conn) removeStream(channel uint32) {
	c.mu.Lock()
	delete(c.streams, channel)
	c.mu.Unlock()
}

// writeFrame writes one frame, honoring a lost connection.
func (c *conn) writeFrame(channel uint32, kind uint8, payload []byte) error {
	select {
	case <-c.lost:
		return ErrLost
	default:
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if err := writeFrame(c.w, frame{channel: channel, kind: kind, payload: payload}); err != nil {
		c.lose(ErrLost)
		return err
	}
	return nil
}

// sendCredit tells the core how many bytes were consumed on a stream.
func (c *conn) sendCredit(channel uint32, n int) error {
	if n <= 0 {
		return nil
	}
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], uint32(n))
	return c.writeFrame(channel, kindCredit, b[:])
}

// stream is an ordered, unbounded queue of frames for one channel.
type stream struct {
	mu     sync.Mutex
	cond   *sync.Cond
	queue  []frame
	closed bool
	err    error
}

func newStream() *stream {
	s := &stream{}
	s.cond = sync.NewCond(&s.mu)
	return s
}

func (s *stream) push(f frame) {
	s.mu.Lock()
	s.pushLocked(f)
	s.mu.Unlock()
}

func (s *stream) pushLocked(f frame) {
	if s.closed {
		return
	}
	f.payload = append([]byte(nil), f.payload...)
	s.queue = append(s.queue, f)
	if f.kind == kindClose {
		// The frame stays queued so the consumer sees it in order; the
		// next read after it returns io.EOF.
		s.closed = true
		if s.err == nil {
			s.err = io.EOF
		}
	}
	s.cond.Broadcast()
}

// next returns the next frame. A CLOSE frame is delivered in order and the
// following call returns io.EOF. A lost connection returns ErrLost.
func (s *stream) next() (frame, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for len(s.queue) == 0 && !s.closed {
		s.cond.Wait()
	}
	if len(s.queue) > 0 {
		f := s.queue[0]
		s.queue = s.queue[1:]
		return f, nil
	}
	if s.err != nil {
		return frame{}, s.err
	}
	return frame{}, io.EOF
}

// closeLocal abandons the stream: queued frames are dropped and the next
// read returns io.EOF.
func (s *stream) closeLocal() {
	s.mu.Lock()
	s.queue = nil
	s.closed = true
	if s.err == nil {
		s.err = io.EOF
	}
	s.cond.Broadcast()
	s.mu.Unlock()
}

// fail ends the stream with err.
func (s *stream) fail(err error) {
	s.mu.Lock()
	s.queue = nil
	s.closed = true
	s.err = err
	s.cond.Broadcast()
	s.mu.Unlock()
}
