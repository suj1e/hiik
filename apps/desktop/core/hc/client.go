package hc

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Dial starts `hc connect` and performs the framed handshake.
func Dial(ctx context.Context, opts Options) (Client, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	binary := opts.Binary
	if binary == "" {
		binary = "hc"
	}
	path, err := exec.LookPath(binary)
	if err != nil {
		return nil, fmt.Errorf("hc: find %s: %w", binary, err)
	}
	args := []string{"connect"}
	if opts.Session != "" {
		args = append(args, "--session", opts.Session)
	}

	cmd := exec.Command(path, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("hc: stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("hc: stdout: %w", err)
	}
	var stderr syncBuffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("hc: start %s: %w", path, err)
	}

	p := &process{cmd: cmd, stdin: stdin, stdout: stdout, done: make(chan struct{})}
	br := bufio.NewReader(stdout)
	hello, err := handshake(ctx, br, stdin)
	if err != nil {
		p.stop()
		msg := stderr.String()
		if msg != "" {
			return nil, fmt.Errorf("%w (core said: %s)", err, firstLine(msg))
		}
		return nil, err
	}

	// Reap the process in the background. Closing stdin makes hc connect
	// exit; Wait then closes the stdout pipe and the reader loop loses.
	go func() {
		p.waitErr = cmd.Wait()
		close(p.done)
	}()

	return newConn(br, stdin, hello, p.Close), nil
}

// process owns the hc connect child and its pipes.
type process struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  io.ReadCloser
	done    chan struct{}
	waitErr error
	once    sync.Once
}

// Close ends hc connect. The core itself keeps running.
func (p *process) Close() error {
	p.once.Do(func() {
		if p.stdin != nil {
			_ = p.stdin.Close()
		}
		// If hc connect does not leave on EOF, end it. The core is a
		// separate process and is unaffected.
		select {
		case <-p.done:
		case <-time.After(500 * time.Millisecond):
			if p.cmd.Process != nil {
				_ = p.cmd.Process.Kill()
			}
			<-p.done
		}
		if p.stdout != nil {
			_ = p.stdout.Close()
		}
	})
	return nil
}

// stop ends the process during a failed handshake.
func (p *process) stop() {
	if p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
	_ = p.stdin.Close()
	_ = p.cmd.Wait()
}

// syncBuffer is a bytes.Buffer safe for a child process and a reader.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func firstLine(s string) string {
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		return s[:i]
	}
	return s
}

// applicationEnv is the terminal identity this app gives programs it starts.
var applicationEnv = map[string]string{
	"TERM":         "xterm-256color",
	"COLORTERM":    "truecolor",
	"TERM_PROGRAM": "hiik",
}

// List returns the core's terminals, optionally including finished ones.
func (c *conn) List(ctx context.Context, all bool) ([]Terminal, error) {
	var res struct {
		Terminals []Terminal `json:"terminals"`
	}
	if err := c.Call(ctx, "terminal.list", map[string]any{"all": all}, &res); err != nil {
		return nil, err
	}
	if res.Terminals == nil {
		return []Terminal{}, nil
	}
	return res.Terminals, nil
}

// Run starts a program in a new terminal and returns its record.
func (c *conn) Run(ctx context.Context, req RunRequest) (Terminal, error) {
	argv := req.Argv
	if argv == nil {
		argv = []string{}
	}
	params := map[string]any{
		"argv": argv,
		"env":  applicationEnv,
	}
	if req.Cwd != "" {
		params["cwd"] = req.Cwd
	}
	if req.On != "" {
		params["on"] = req.On
	}
	if req.Labels != nil {
		params["labels"] = req.Labels
	}
	if req.Cols > 0 || req.Rows > 0 {
		params["size"] = map[string]int{
			"cols": req.Cols,
			"rows": req.Rows,
		}
	}
	var term Terminal
	if err := c.Call(ctx, "terminal.run", params, &term); err != nil {
		return Terminal{}, err
	}
	return term, nil
}

// Kill ends a terminal and its sub-agents.
func (c *conn) Kill(ctx context.Context, address string) error {
	return c.Call(ctx, "terminal.kill", map[string]any{"terminal": address}, nil)
}

// Label sets and removes labels on a terminal; an empty value removes one.
func (c *conn) Label(ctx context.Context, address string, labels map[string]string) error {
	if labels == nil {
		labels = map[string]string{}
	}
	params := map[string]any{
		"terminal": address,
		"labels":   labels,
	}
	return c.Call(ctx, "terminal.label", params, nil)
}

var _ Client = (*conn)(nil)
