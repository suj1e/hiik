package api

// UIState is the shell's persisted view state.
type UIState struct {
	SidebarW float32 `json:"sidebarW,omitempty"`
	Zoomed   string  `json:"zoomed,omitempty"`
	Focus    string  `json:"focus,omitempty"`
}
