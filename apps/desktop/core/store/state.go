package store

import "hiik/mods/api"

// Version is the state.json schema version this app writes.
const Version = 1

// State is state.json: the layout and the persisted view state.
type State struct {
	Version     int         `json:"version"`
	ActiveSpace string      `json:"activeSpace,omitempty"`
	Spaces      []Space     `json:"spaces,omitempty"`
	UI          api.UIState `json:"ui,omitempty"`
}

// Space is one space on disk.
type Space struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Folder    string `json:"folder"`
	Machine   string `json:"machine,omitempty"`
	ActiveTab int    `json:"activeTab"`
	Tabs      []Tab  `json:"tabs"`
}

// Tab is one tab on disk.
type Tab struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Focus string `json:"focus,omitempty"`
	Root  *Node  `json:"root"`
}

// Node is a terminal leaf or a split on disk. A leaf has no A and B.
type Node struct {
	ID       string  `json:"id"`
	Vertical bool    `json:"vertical,omitempty"`
	Ratio    float32 `json:"ratio,omitempty"`
	A        *Node   `json:"a,omitempty"`
	B        *Node   `json:"b,omitempty"`
	Terminal string  `json:"terminal,omitempty"`
}

// DefaultState is an empty layout.
func DefaultState() State {
	return State{Version: Version}
}

// FromAPI converts the live layout to its on-disk shape. It copies every
// node, so the caller may keep mutating the model.
func FromAPI(spaces []api.Space, active string, ui api.UIState) State {
	st := State{Version: Version, ActiveSpace: active, UI: ui}
	if len(spaces) > 0 {
		st.Spaces = make([]Space, len(spaces))
		for i, sp := range spaces {
			st.Spaces[i] = spaceFromAPI(sp)
		}
	}
	return st
}

func spaceFromAPI(sp api.Space) Space {
	out := Space{
		ID:        sp.ID,
		Name:      sp.Name,
		Folder:    sp.Folder,
		Machine:   sp.Machine,
		ActiveTab: sp.Active,
	}
	if len(sp.Tabs) > 0 {
		out.Tabs = make([]Tab, len(sp.Tabs))
		for i, tab := range sp.Tabs {
			out.Tabs[i] = Tab{ID: tab.ID, Name: tab.Name, Focus: tab.Focus, Root: nodeFromAPI(tab.Root)}
		}
	}
	return out
}

func nodeFromAPI(n *api.Node) *Node {
	if n == nil {
		return nil
	}
	out := &Node{ID: n.ID, Terminal: n.Terminal}
	if n.A != nil || n.B != nil {
		out.Vertical = n.Vertical
		out.Ratio = n.Ratio
		out.A = nodeFromAPI(n.A)
		out.B = nodeFromAPI(n.B)
	}
	return out
}

// SpacesAPI converts the on-disk layout to live model values.
func (s State) SpacesAPI() []api.Space {
	out := make([]api.Space, len(s.Spaces))
	for i, sp := range s.Spaces {
		out[i] = api.Space{
			ID:      sp.ID,
			Name:    sp.Name,
			Folder:  sp.Folder,
			Machine: sp.Machine,
			Active:  sp.ActiveTab,
		}
		if len(sp.Tabs) > 0 {
			out[i].Tabs = make([]api.Tab, len(sp.Tabs))
			for j, tab := range sp.Tabs {
				out[i].Tabs[j] = api.Tab{ID: tab.ID, Name: tab.Name, Focus: tab.Focus, Root: nodeToAPI(tab.Root)}
			}
		}
	}
	return out
}

func nodeToAPI(n *Node) *api.Node {
	if n == nil {
		return nil
	}
	out := &api.Node{ID: n.ID, Terminal: n.Terminal}
	if n.A != nil || n.B != nil {
		out.Vertical = n.Vertical
		out.Ratio = n.Ratio
		out.A = nodeToAPI(n.A)
		out.B = nodeToAPI(n.B)
	}
	return out
}
