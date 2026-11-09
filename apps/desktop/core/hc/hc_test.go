package hc

import (
	"encoding/json"
	"net"
	"sync"
	"testing"
	"time"
)

// fakeCore serves one side of a net.Pipe as an hc core.
type fakeCore struct {
	t  *testing.T
	mu sync.Mutex

	srv net.Conn
	c   *conn

	frames chan frame
	closed chan struct{}
	once   sync.Once
}

// newTestConn wires a client conn to a fake core. handler runs for every
// frame the fake core receives, in order.
func newTestConn(t *testing.T, handler func(*fakeCore, frame)) (*conn, *fakeCore) {
	t.Helper()
	cli, srv := net.Pipe()
	fc := &fakeCore{
		t:      t,
		srv:    srv,
		frames: make(chan frame, 64),
		closed: make(chan struct{}),
	}
	fc.c = newConn(cli, cli, Hello{Generation: 1, Version: "test", Methods: helloMethods()}, cli.Close)

	go func() {
		for {
			f, err := readFrame(srv)
			if err != nil {
				fc.once.Do(func() { close(fc.closed) })
				return
			}
			select {
			case fc.frames <- f:
			case <-fc.closed:
				return
			}
			if f.channel == 0 {
				handler(fc, f)
			}
		}
	}()
	t.Cleanup(func() {
		fc.once.Do(func() { close(fc.closed) })
		_ = srv.Close()
		_ = fc.c.Close()
	})
	return fc.c, fc
}

func (f *fakeCore) write(channel uint32, kind uint8, payload []byte) {
	f.t.Helper()
	if err := writeFrame(f.srv, frame{channel: channel, kind: kind, payload: payload}); err != nil {
		f.t.Logf("fake core write: %v", err)
	}
}

func (f *fakeCore) writeJSON(v any) {
	f.t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		f.t.Fatalf("marshal: %v", err)
	}
	f.write(0, kindData, b)
}

func (f *fakeCore) result(id uint64, result any) {
	f.t.Helper()
	f.writeJSON(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	})
}

func (f *fakeCore) rpcError(id uint64, code int, message, kind string) {
	f.t.Helper()
	f.writeJSON(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]any{
			"code":    code,
			"message": message,
			"data":    map[string]any{"kind": kind},
		},
	})
}

// waitFrame waits for the next client frame matching want.
func (f *fakeCore) waitFrame(t *testing.T, want func(frame) bool) frame {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case f := <-f.frames:
			if want(f) {
				return f
			}
		case <-deadline:
			t.Fatal("timed out waiting for frame")
		}
	}
}

type testRequest struct {
	ID     uint64          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

func requestID(t *testing.T, f frame) (testRequest, bool) {
	t.Helper()
	if f.channel != 0 || f.kind != kindData {
		return testRequest{}, false
	}
	var req testRequest
	if err := json.Unmarshal(f.payload, &req); err != nil {
		return testRequest{}, false
	}
	return req, req.Method != ""
}

func helloMethods() []string {
	return []string{
		"hello", "terminal.list", "terminal.run", "terminal.kill",
		"terminal.label", "terminal.attach", "terminal.read",
		"events.subscribe", "machine.list", "machine.exec",
		"file.read", "file.write", "file.edit",
	}
}
