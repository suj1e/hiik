package hc

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestCallRoutesByID(t *testing.T) {
	c, fc := newTestConn(t, func(f *fakeCore, fr frame) {
		req, ok := requestID(t, fr)
		if !ok {
			return
		}
		switch req.Method {
		case "echo":
			f.result(req.ID, map[string]int{"value": 7})
		case "fail":
			f.rpcError(req.ID, -32000, "no terminal t9", "not_found")
		case "never":
			// Leave the call pending.
		}
	})

	var got struct {
		Value int `json:"value"`
	}
	if err := c.Call(context.Background(), "echo", map[string]any{"x": 1}, &got); err != nil {
		t.Fatal(err)
	}
	if got.Value != 7 {
		t.Fatalf("value = %d, want 7", got.Value)
	}

	err := c.Call(context.Background(), "fail", nil, nil)
	var rpcErr *RPCError
	if !errors.As(err, &rpcErr) {
		t.Fatalf("error = %v, want RPCError", err)
	}
	if rpcErr.Data.Kind != "not_found" || rpcErr.Message != "no terminal t9" {
		t.Fatalf("rpc error = %+v", rpcErr)
	}
	if !strings.Contains(err.Error(), "not_found") || !strings.Contains(err.Error(), "no terminal t9") {
		t.Fatalf("error text = %q", err)
	}

	// A timeout removes the pending entry.
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	if err := c.Call(ctx, "never", nil, nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout error = %v", err)
	}
	c.mu.Lock()
	n := len(c.pending)
	c.mu.Unlock()
	if n != 0 {
		t.Fatalf("%d pending calls after timeout", n)
	}

	// Closing the pipe fails pending calls and closes Lost exactly once.
	_ = fc.srv.Close()
	select {
	case <-c.Lost():
	case <-time.After(3 * time.Second):
		t.Fatal("Lost did not close")
	}
	if err := c.Call(context.Background(), "echo", nil, nil); !errors.Is(err, ErrLost) {
		t.Fatalf("after loss: %v", err)
	}
}

func TestClientMethods(t *testing.T) {
	type captured struct {
		method string
		params map[string]any
	}
	capturedCh := make(chan captured, 8)
	c, _ := newTestConn(t, func(f *fakeCore, fr frame) {
		req, ok := requestID(t, fr)
		if !ok {
			return
		}
		var params map[string]any
		_ = json.Unmarshal(req.Params, &params)
		capturedCh <- captured{method: req.Method, params: params}
		switch req.Method {
		case "terminal.list":
			f.result(req.ID, map[string]any{"terminals": []Terminal{{Address: "t1", State: "live"}}})
		case "terminal.run":
			f.result(req.ID, Terminal{Address: "t2", State: "live"})
		default:
			f.result(req.ID, map[string]any{})
		}
	})

	terms, err := c.List(context.Background(), true)
	if err != nil || len(terms) != 1 || terms[0].Address != "t1" {
		t.Fatalf("List = %+v, %v", terms, err)
	}
	listCall := <-capturedCh
	if listCall.params["all"] != true {
		t.Fatalf("terminal.list params = %+v", listCall.params)
	}

	term, err := c.Run(context.Background(), RunRequest{
		Argv:   []string{"/bin/sh"},
		Cwd:    "/tmp",
		On:     "vps",
		Labels: map[string]string{"hiik.space": "s1"},
		Cols:   80,
		Rows:   24,
	})
	if err != nil || term.Address != "t2" {
		t.Fatalf("Run = %+v, %v", term, err)
	}
	runCall := <-capturedCh
	if runCall.params["cwd"] != "/tmp" || runCall.params["on"] != "vps" {
		t.Fatalf("run params = %+v", runCall.params)
	}
	env, _ := runCall.params["env"].(map[string]any)
	if env["TERM"] != "xterm-256color" || env["COLORTERM"] != "truecolor" || env["TERM_PROGRAM"] != "hiik" {
		t.Fatalf("run env = %+v", env)
	}
	size, _ := runCall.params["size"].(map[string]any)
	if size["cols"] != float64(80) || size["rows"] != float64(24) {
		t.Fatalf("run size = %+v", size)
	}
	labels, _ := runCall.params["labels"].(map[string]any)
	if labels["hiik.space"] != "s1" {
		t.Fatalf("run labels = %+v", labels)
	}
	if _, ok := runCall.params["task"]; ok {
		t.Fatal("terminal.run must not get a task param")
	}

	if err := c.Kill(context.Background(), "t2"); err != nil {
		t.Fatal(err)
	}
	killCall := <-capturedCh
	if killCall.method != "terminal.kill" || killCall.params["terminal"] != "t2" {
		t.Fatalf("kill = %+v", killCall)
	}

	if err := c.Label(context.Background(), "t2", map[string]string{"a": "b"}); err != nil {
		t.Fatal(err)
	}
	labelCall := <-capturedCh
	gotLabels, _ := labelCall.params["labels"].(map[string]any)
	if labelCall.params["terminal"] != "t2" || gotLabels["a"] != "b" {
		t.Fatalf("label = %+v", labelCall)
	}
}

func TestValidateHello(t *testing.T) {
	good := Hello{Generation: 1, Version: "1.2.3", Methods: helloMethods()}
	if err := validateHello(good); err != nil {
		t.Fatalf("valid hello: %v", err)
	}

	newer := good
	newer.Generation = 2
	if err := validateHello(newer); err == nil || !strings.Contains(err.Error(), "update hiik") {
		t.Fatalf("generation 2 error = %v", err)
	}
	older := good
	older.Generation = 0
	if err := validateHello(older); err == nil || !strings.Contains(err.Error(), "update hc") {
		t.Fatalf("generation 0 error = %v", err)
	}
	noVersion := good
	noVersion.Version = ""
	if err := validateHello(noVersion); err == nil || !strings.Contains(err.Error(), "version") {
		t.Fatalf("empty version error = %v", err)
	}
	missing := good
	missing.Methods = []string{"hello"}
	err := validateHello(missing)
	if err == nil || !strings.Contains(err.Error(), "terminal.attach") || !strings.Contains(err.Error(), "update hc") {
		t.Fatalf("missing method error = %v", err)
	}
}

func TestRPCMessageIDs(t *testing.T) {
	// IDs are allocated atomically, increase and are never reused.
	ids := make(chan uint64, 8)
	c, _ := newTestConn(t, func(f *fakeCore, fr frame) {
		req, ok := requestID(t, fr)
		if !ok {
			return
		}
		ids <- req.ID
		f.result(req.ID, nil)
	})
	seen := map[uint64]bool{}
	var last uint64
	for range 5 {
		if err := c.Call(context.Background(), "hello", nil, nil); err != nil {
			t.Fatal(err)
		}
		id := <-ids
		if seen[id] {
			t.Fatalf("id %d reused", id)
		}
		if id <= last {
			t.Fatalf("id %d did not increase after %d", id, last)
		}
		last = id
		seen[id] = true
	}
}
