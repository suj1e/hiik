package api

// Space is one place to work on one machine.
type Space struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Folder  string `json:"folder"`
	Machine string `json:"machine,omitempty"` // "" is this computer
	Tabs    []Tab  `json:"tabs"`
	Active  int    `json:"active"`
}

// Tab is one arrangement of panes in a space.
type Tab struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Root  *Node  `json:"root"`
	Focus string `json:"focus,omitempty"`
}

// Node is a leaf terminal or a split of two nodes.
type Node struct {
	ID       string         `json:"id"`
	Vertical bool           `json:"vertical,omitempty"` // children stacked, not side by side
	Ratio    float32        `json:"ratio,omitempty"`    // share of the first child
	A        *Node          `json:"a,omitempty"`
	B        *Node          `json:"b,omitempty"`
	Terminal string         `json:"terminal,omitempty"` // core address at a leaf
	Mod      string         `json:"mod,omitempty"`      // panel ID at a mod leaf
	Props    map[string]any `json:"props,omitempty"`    // props for the mod panel
}

// Agent is a terminal that reports status, or a sub-agent of one.
type Agent struct {
	Address string `json:"address"`
	App     string `json:"app,omitempty"`
	Title   string `json:"title,omitempty"`
	Space   string `json:"space,omitempty"`
	Tab     string `json:"tab,omitempty"`
	Status  string `json:"status,omitempty"` // working, blocked, done, idle
	Pinned  bool   `json:"pinned,omitempty"`
	Seen    bool   `json:"seen,omitempty"`
	Turn    uint64 `json:"turn,omitempty"`
	// Parent is the address of the terminal that started this sub-agent,
	// or "" for a top-level agent. The list keeps a child under its parent.
	Parent string `json:"parent,omitempty"`
	// SeenTurn is the herdr.seen label's turn as the core last reported it.
	// It is not part of the model's JSON: the label is the durable record.
	SeenTurn uint64 `json:"-"`
}

// TerminalInfo is the live state of a core terminal, for pane headers and
// exit placeholders.
type TerminalInfo struct {
	Address  string `json:"address"`
	Title    string `json:"title,omitempty"`
	Cwd      string `json:"cwd,omitempty"`
	State    string `json:"state,omitempty"` // live or finished
	ExitCode *int   `json:"exit_code,omitempty"`
	SeenTurn uint64 `json:"seen_turn,omitempty"`
	Pinned   bool   `json:"pinned,omitempty"`
}
