// Package hc is the herdr-core client. The types here are the frozen
// contract; the implementation lives in client.go, frame.go, events.go and
// attach.go.
package hc

import (
	"context"
	"encoding/json"
	"io"
)

// Options selects the core to reach.
type Options struct {
	// Binary is the hc executable; "" uses PATH.
	Binary string
	// Session is the core session; "" uses the user's default.
	Session string
}

// Hello is the handshake result.
type Hello struct {
	Generation int      `json:"generation"`
	Version    string   `json:"version"`
	Methods    []string `json:"methods"`
	Features   []string `json:"features"`
}

// Client is one framed connection to a core.
type Client interface {
	Close() error
	Hello() Hello
	Call(ctx context.Context, method string, params, result any) error
	Subscribe(ctx context.Context) (*Subscription, error)
	List(ctx context.Context, all bool) ([]Terminal, error)
	Run(ctx context.Context, req RunRequest) (Terminal, error)
	Kill(ctx context.Context, address string) error
	Label(ctx context.Context, address string, labels map[string]string) error
	Attach(ctx context.Context, req AttachRequest) (*Attachment, error)
	Lost() <-chan struct{}
}

// Terminal is the core's record of one terminal.
type Terminal struct {
	Address    string            `json:"address"`
	State      string            `json:"state"` // live, finished
	Title      string            `json:"title"`
	Cwd        string            `json:"cwd"`
	Agent      string            `json:"agent"`
	Parent     string            `json:"parent,omitempty"` // the terminal that started it, `t3`
	Labels     map[string]string `json:"labels"`
	Status     *Status           `json:"status"`
	Turn       uint64            `json:"turn"`
	ExitCode   *int              `json:"exit_code"`
	FinishedMS *uint64           `json:"finished_ms"`
	Cols       int               `json:"cols"`
	Rows       int               `json:"rows"`
	Foreground *Foreground       `json:"foreground"`
}

// Status is an agent's report.
type Status struct {
	State string `json:"state"` // working, blocked, idle, done, error
}

// Foreground is the program running in a terminal.
type Foreground struct {
	PID  int      `json:"pid"`
	Name string   `json:"name"`
	Argv []string `json:"argv"`
	Cwd  string   `json:"cwd"`
}

// RunRequest starts a program in a new terminal.
type RunRequest struct {
	Argv   []string          `json:"argv"`
	Cwd    string            `json:"cwd,omitempty"`
	On     string            `json:"on,omitempty"`
	Task   string            `json:"task,omitempty"`
	Labels map[string]string `json:"labels,omitempty"`
	Cols   int               `json:"cols,omitempty"`
	Rows   int               `json:"rows,omitempty"`
}

// AttachRequest opens a terminal stream.
type AttachRequest struct {
	Terminal string  `json:"terminal"`
	Cols     int     `json:"cols"`
	Rows     int     `json:"rows"`
	History  int     `json:"history,omitempty"`
	Display  Display `json:"display"`
}

// Display is what the viewer's terminal looks like.
type Display struct {
	Bg     string `json:"bg,omitempty"`
	Fg     string `json:"fg,omitempty"`
	Cursor string `json:"cursor,omitempty"`
	Cell   [2]int `json:"cell,omitempty"`
}

// Attachment is a live terminal stream.
type Attachment struct {
	Channel  uint32
	Terminal Terminal
	// Conn is fed to terminal.Options.Conn. The concrete type also has
	// Resize, ResizePixels and UseTheme, which the terminal plugin detects.
	Conn io.ReadWriteCloser
	// Close detaches and releases the stream.
	Close func() error
}

// Snapshot is the state a subscription starts from.
type Snapshot struct {
	Seq       uint64     `json:"seq"`
	Terminals []Terminal `json:"terminals"`
}

// Event is one ordered change from the core.
type Event struct {
	Seq      uint64          `json:"seq"`
	Type     string          `json:"type"`
	Terminal string          `json:"terminal"`
	Data     json.RawMessage `json:"data"`
}

// Subscription is a snapshot followed by events.
type Subscription struct {
	Snapshot Snapshot
	Events   <-chan Event
	// Close ends the subscription and its channel.
	Close func()
}
