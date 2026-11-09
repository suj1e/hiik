package hc

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const defaultCallTimeout = 10 * time.Second

// rpcRequest is one JSON-RPC 2.0 request.
type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      uint64 `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// rpcMessage is a request, reply or notification as it appears on the wire.
type rpcMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *uint64         `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// RPCError is an error returned by the core. Kind is the stable error kind
// from data.kind; Message is the human-readable message.
type RPCError struct {
	Method  string       `json:"-"`
	Code    int          `json:"code"`
	Message string       `json:"message"`
	Data    rpcErrorData `json:"data,omitempty"`
}

type rpcErrorData struct {
	Kind    string `json:"kind,omitempty"`
	Message string `json:"message,omitempty"`
	Hint    string `json:"hint,omitempty"`
}

func (e *RPCError) Error() string {
	kind := e.Data.Kind
	if kind == "" {
		kind = "error"
	}
	msg := e.Message
	switch {
	case msg == "":
		msg = e.Data.Message
	case e.Data.Message != "" && e.Data.Message != msg:
		msg += ": " + e.Data.Message
	}
	if msg == "" {
		msg = "core error"
	}
	if e.Method != "" {
		return fmt.Sprintf("hc: %s: %s (%s)", e.Method, msg, kind)
	}
	return fmt.Sprintf("hc: %s (%s)", msg, kind)
}

// pendingCall is one in-flight request awaiting its reply.
type pendingCall struct {
	method string
	ch     chan callResult
}

func newPendingCall(method string) *pendingCall {
	return &pendingCall{method: method, ch: make(chan callResult, 1)}
}

func (p *pendingCall) reply(r callResult) {
	select {
	case p.ch <- r:
	default:
	}
}

type callResult struct {
	result json.RawMessage
	err    error
}

// Call sends one request and waits for its reply. A call without a deadline
// gets defaultCallTimeout.
func (c *conn) Call(ctx context.Context, method string, params, result any) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultCallTimeout)
		defer cancel()
	}

	id := c.nextID.Add(1)
	body, err := json.Marshal(rpcRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	})
	if err != nil {
		return fmt.Errorf("hc: %s: encode request: %w", method, err)
	}

	p := newPendingCall(method)
	c.mu.Lock()
	select {
	case <-c.lost:
		c.mu.Unlock()
		return fmt.Errorf("%s: %w", method, ErrLost)
	default:
	}
	c.pending[id] = p
	c.mu.Unlock()

	if err := c.writeFrame(0, kindData, body); err != nil {
		c.mu.Lock()
		if c.pending[id] == p {
			delete(c.pending, id)
		}
		c.mu.Unlock()
		return fmt.Errorf("%s: %w", method, err)
	}

	select {
	case r := <-p.ch:
		if r.err != nil {
			return r.err
		}
		if result != nil && len(r.result) > 0 {
			if err := json.Unmarshal(r.result, result); err != nil {
				return fmt.Errorf("hc: %s: decode result: %w", method, err)
			}
		}
		return nil
	case <-ctx.Done():
		c.mu.Lock()
		if c.pending[id] == p {
			delete(c.pending, id)
		}
		c.mu.Unlock()
		return fmt.Errorf("%s: %w", method, ctx.Err())
	}
}

// handshake sends the unframed hello that switches hc connect to framed
// mode and returns the parsed handshake result.
func handshake(ctx context.Context, r *bufio.Reader, w interface{ Write([]byte) (int, error) }) (Hello, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	line := []byte(`{"jsonrpc":"2.0","id":0,"method":"hello","params":{"framed":true}}` + "\n")
	if err := writeAll(w, line); err != nil {
		return Hello{}, fmt.Errorf("hc: send hello: %w", err)
	}

	type readResult struct {
		line []byte
		err  error
	}
	done := make(chan readResult, 1)
	go func() {
		b, err := r.ReadBytes('\n')
		done <- readResult{line: b, err: err}
	}()

	select {
	case <-ctx.Done():
		return Hello{}, ctx.Err()
	case res := <-done:
		if res.err != nil {
			return Hello{}, fmt.Errorf("hc: read hello: %w", res.err)
		}
		var msg rpcMessage
		if err := json.Unmarshal(res.line, &msg); err != nil {
			return Hello{}, fmt.Errorf("hc: decode hello: %w", err)
		}
		if msg.Error != nil {
			msg.Error.Method = "hello"
			return Hello{}, msg.Error
		}
		if len(msg.Result) == 0 {
			return Hello{}, fmt.Errorf("hc: hello returned no result")
		}
		var hello Hello
		if err := json.Unmarshal(msg.Result, &hello); err != nil {
			return Hello{}, fmt.Errorf("hc: decode hello result: %w", err)
		}
		if err := validateHello(hello); err != nil {
			return Hello{}, err
		}
		return hello, nil
	}
}

// validateHello rejects a core this client cannot talk to and names what is
// wrong or which side needs updating.
func validateHello(h Hello) error {
	if h.Generation != 1 {
		side := "hc"
		if h.Generation > 1 {
			side = "hiik"
		}
		return fmt.Errorf("hc: core speaks protocol generation %d, this client speaks generation 1; update %s", h.Generation, side)
	}
	if h.Version == "" {
		return fmt.Errorf("hc: core reported no version; update hc")
	}
	have := make(map[string]bool, len(h.Methods))
	for _, m := range h.Methods {
		have[m] = true
	}
	required := []string{
		"hello",
		"terminal.list",
		"terminal.run",
		"terminal.kill",
		"terminal.label",
		"terminal.attach",
		"terminal.read",
		"events.subscribe",
		"machine.list",
		"machine.exec",
		"file.read",
		"file.write",
		"file.edit",
	}
	var missing []string
	for _, m := range required {
		if !have[m] {
			missing = append(missing, m)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("hc: core is missing methods %s; update hc", strings.Join(missing, ", "))
	}
	return nil
}
