package api

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Layout is a chrome layout read from layouts/<name>.json. A layout is a
// tree: a row or column of children, a stack showing one child, or a block
// with content.
type Layout struct {
	ID       string         `json:"id,omitempty"`
	Type     string         `json:"type"`
	Size     *LayoutSize    `json:"size,omitempty"`
	Min      float32        `json:"min,omitempty"`
	Max      float32        `json:"max,omitempty"`
	Hidden   bool           `json:"hidden,omitempty"`
	Active   string         `json:"active,omitempty"`
	Header   string         `json:"header,omitempty"`
	Content  *LayoutContent `json:"content,omitempty"`
	Children []Layout       `json:"children,omitempty"`
	Floating []LayoutFloat  `json:"floating,omitempty"`
	Gap      float32        `json:"gap,omitempty"`
	Padding  LayoutPadding  `json:"padding,omitempty"`
	Radius   float32        `json:"radius,omitempty"`
	Border   *LayoutBorder  `json:"border,omitempty"`
	// Background names a theme color or holds "#rrggbb".
	Background string `json:"background,omitempty"`
}

// LayoutContent is what a layout block shows: a mod panel with props, one
// terminal, the active tab, or one agent's terminal.
type LayoutContent struct {
	Mod      string         `json:"mod,omitempty"`
	Props    map[string]any `json:"props,omitempty"`
	Terminal string         `json:"terminal,omitempty"`
	Tab      string         `json:"tab,omitempty"`
	Agent    string         `json:"agent,omitempty"`
}

// LayoutFloat is a block drawn over its node.
type LayoutFloat struct {
	ID      string         `json:"id,omitempty"`
	Content *LayoutContent `json:"content,omitempty"`
	Edge    string         `json:"edge,omitempty"`
	Align   string         `json:"align,omitempty"`
	Peek    int            `json:"peek,omitempty"`
	Width   float32        `json:"width,omitempty"`
	Height  float32        `json:"height,omitempty"`
}

// LayoutSize is a node's share: fixed pixels or a weight of the room the
// fixed siblings leave.
type LayoutSize struct {
	Px     *float32
	Weight *float32
}

// UnmarshalJSON reads {"px":240} or {"weight":2}, and a bare number as px.
func (s *LayoutSize) UnmarshalJSON(data []byte) error {
	var n float32
	if err := json.Unmarshal(data, &n); err == nil {
		s.Px = new(n)
		return nil
	}
	var raw struct {
		Px     *float32 `json:"px"`
		Weight *float32 `json:"weight"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("layout size: %w", err)
	}
	if raw.Px != nil && raw.Weight != nil {
		return fmt.Errorf("layout size: give px or weight, not both")
	}
	s.Px, s.Weight = raw.Px, raw.Weight
	return nil
}

// MarshalJSON writes the size in the shape UnmarshalJSON reads.
func (s LayoutSize) MarshalJSON() ([]byte, error) {
	switch {
	case s.Px != nil && s.Weight != nil:
		return nil, fmt.Errorf("layout size: give px or weight, not both")
	case s.Px != nil:
		return json.Marshal(struct {
			Px float32 `json:"px"`
		}{*s.Px})
	case s.Weight != nil:
		return json.Marshal(struct {
			Weight float32 `json:"weight"`
		}{*s.Weight})
	default:
		return []byte("null"), nil
	}
}

// LayoutPadding is a node's inner space: one number for both sides, or
// [vertical, horizontal].
type LayoutPadding struct{ X, Y float32 }

// UnmarshalJSON reads 8 or [8, 12].
func (p *LayoutPadding) UnmarshalJSON(data []byte) error {
	var one float32
	if err := json.Unmarshal(data, &one); err == nil {
		p.X, p.Y = one, one
		return nil
	}
	var pair []float32
	if err := json.Unmarshal(data, &pair); err != nil {
		return fmt.Errorf("layout padding: %w", err)
	}
	switch len(pair) {
	case 0:
		p.X, p.Y = 0, 0
	case 1:
		p.X, p.Y = pair[0], pair[0]
	case 2:
		p.Y, p.X = pair[0], pair[1]
	default:
		return fmt.Errorf("layout padding: want one number or [vertical, horizontal]")
	}
	return nil
}

// MarshalJSON writes the padding in the shape UnmarshalJSON reads.
func (p LayoutPadding) MarshalJSON() ([]byte, error) {
	if p.X == p.Y {
		return json.Marshal(p.X)
	}
	return json.Marshal([]float32{p.Y, p.X})
}

// LayoutBorder is a node's border.
type LayoutBorder struct {
	Width float32 `json:"width,omitempty"`
	Color string  `json:"color,omitempty"`
}

// LayoutState is the shell's runtime changes to the shown layout: nodes it
// hid or resized, and the child a stack shows.
type LayoutState struct {
	Hidden map[string]bool       `json:"hidden,omitempty"`
	Sizes  map[string]LayoutSize `json:"sizes,omitempty"`
	Stacks map[string]string     `json:"stacks,omitempty"`
}

// Clone returns a copy that a caller may mutate.
func (st LayoutState) Clone() LayoutState {
	out := LayoutState{
		Hidden: make(map[string]bool, len(st.Hidden)),
		Sizes:  make(map[string]LayoutSize, len(st.Sizes)),
		Stacks: make(map[string]string, len(st.Stacks)),
	}
	for k, v := range st.Hidden {
		out.Hidden[k] = v
	}
	for k, v := range st.Sizes {
		out.Sizes[k] = v
	}
	for k, v := range st.Stacks {
		out.Stacks[k] = v
	}
	return out
}

// Layout names the app ships. Both are drawn by the shell itself rather
// than read from a file.
const (
	LayoutHerdr     = "herdr"
	LayoutTerminals = "terminals"
)

// LayoutNameInvalid reports whether a layout name is usable as a file name.
func LayoutNameInvalid(name string) error {
	name = strings.TrimSpace(name)
	switch {
	case name == "":
		return fmt.Errorf("layout name must not be empty")
	case name == "." || name == "..":
		return fmt.Errorf("layout name %q is not a file name", name)
	case strings.ContainsAny(name, `/\`):
		return fmt.Errorf("layout name %q must not contain a path separator", name)
	case strings.HasPrefix(name, "."):
		return fmt.Errorf("layout name %q must not start with a dot", name)
	}
	return nil
}

// BuiltinLayouts returns the layouts the app draws without a file, in the
// order the switcher offers them.
func BuiltinLayouts() []Layout {
	return []Layout{
		{
			ID: "root", Type: "column",
			Children: []Layout{
				{
					ID: "titlebar", Type: "block",
					Size:    &LayoutSize{Px: new(float32(38))},
					Content: &LayoutContent{Mod: "titlebar"},
				},
				{
					ID: "body", Type: "row",
					Children: []Layout{
						{
							ID: "sidebar", Type: "column",
							Size: &LayoutSize{Px: new(float32(240))},
							Children: []Layout{
								{ID: "spaces", Type: "block", Content: &LayoutContent{Mod: "spaces"}},
								{ID: "agents", Type: "block", Content: &LayoutContent{Mod: "agents"}},
							},
						},
						{
							ID: "work", Type: "column", Children: []Layout{
								{ID: "tabs", Type: "block", Size: &LayoutSize{Px: new(float32(36))}, Content: &LayoutContent{Mod: "tabs"}},
								{ID: "main", Type: "block", Content: &LayoutContent{Tab: "active"}},
							},
						},
					},
				},
				{ID: "status", Type: "block", Size: &LayoutSize{Px: new(float32(24))}, Content: &LayoutContent{Mod: "status"}},
			},
		},
		{
			ID: "root", Type: "column",
			Children: []Layout{
				{ID: "tabs", Type: "block", Size: &LayoutSize{Px: new(float32(38))}, Content: &LayoutContent{Mod: "terminals-bar"}},
				{ID: "main", Type: "block", Content: &LayoutContent{Tab: "active"}},
				{ID: "status", Type: "block", Size: &LayoutSize{Px: new(float32(24))}, Content: &LayoutContent{Mod: "status"}},
			},
		},
	}
}

// BuiltinLayout returns one shipped layout by name.
func BuiltinLayout(name string) (Layout, bool) {
	switch name {
	case LayoutHerdr:
		return BuiltinLayouts()[0], true
	case LayoutTerminals:
		return BuiltinLayouts()[1], true
	default:
		return Layout{}, false
	}
}

// BuiltinLayoutNames returns the shipped layout names in order.
func BuiltinLayoutNames() []string {
	return []string{LayoutHerdr, LayoutTerminals}
}

// ParseLayout reads and validates one layout document.
func ParseLayout(data []byte) (Layout, error) {
	var l Layout
	if err := json.Unmarshal(data, &l); err != nil {
		return Layout{}, fmt.Errorf("layout: %w", err)
	}
	if err := ValidateLayout(&l); err != nil {
		return Layout{}, err
	}
	return l, nil
}

// ValidateLayout checks a layout tree and fills the defaults a renderer
// relies on.
func ValidateLayout(l *Layout) error {
	if l == nil {
		return fmt.Errorf("layout is empty")
	}
	ids := make(map[string]bool)
	return validateLayoutNode(l, "root", ids)
}

func validateLayoutNode(n *Layout, path string, ids map[string]bool) error {
	switch n.Type {
	case "row", "column", "stack", "block":
	case "":
		return fmt.Errorf("%s: a node needs a type", path)
	default:
		return fmt.Errorf("%s: type %q is not row, column, stack or block", path, n.Type)
	}
	id := strings.TrimSpace(n.ID)
	if id == "" {
		id = path
	}
	if ids[id] {
		return fmt.Errorf("%s: duplicate node id %q", path, id)
	}
	ids[id] = true
	n.ID = id
	if n.Min < 0 || n.Max < 0 || n.Gap < 0 || n.Padding.X < 0 || n.Padding.Y < 0 || n.Radius < 0 {
		return fmt.Errorf("%s: sizes and padding must not be negative", path)
	}
	if n.Size != nil {
		if n.Size.Px != nil && n.Size.Weight != nil {
			return fmt.Errorf("%s: size needs px or weight, not both", path)
		}
		if n.Size.Px != nil && *n.Size.Px < 0 {
			return fmt.Errorf("%s: px must not be negative", path)
		}
		if n.Size.Weight != nil && *n.Size.Weight <= 0 {
			return fmt.Errorf("%s: weight must be greater than zero", path)
		}
	}
	if n.Type == "block" {
		if len(n.Children) > 0 {
			return fmt.Errorf("%s: a block has no children", path)
		}
		if err := validateLayoutContent(n.Content, path); err != nil {
			return err
		}
	} else if len(n.Children) == 0 {
		return fmt.Errorf("%s: a %s needs children", path, n.Type)
	}
	childIDs := make(map[string]bool, len(n.Children))
	for i := range n.Children {
		child := &n.Children[i]
		childPath := fmt.Sprintf("%s.%d", path, i)
		if id := strings.TrimSpace(child.ID); id != "" {
			childPath = fmt.Sprintf("%s.%s", path, id)
		}
		if err := validateLayoutNode(child, childPath, ids); err != nil {
			return err
		}
		childIDs[child.ID] = true
	}
	if n.Active != "" && !childIDs[n.Active] {
		return fmt.Errorf("%s: active %q is not a child", path, n.Active)
	}
	if n.Type == "stack" && n.Active == "" && len(n.Children) > 0 {
		n.Active = n.Children[0].ID
	}
	for i := range n.Floating {
		f := &n.Floating[i]
		if strings.TrimSpace(f.ID) == "" {
			f.ID = fmt.Sprintf("%s.float%d", path, i)
		}
		if ids[f.ID] {
			return fmt.Errorf("%s: duplicate node id %q", path, f.ID)
		}
		ids[f.ID] = true
		if err := validateLayoutContent(f.Content, path+"."+f.ID); err != nil {
			return err
		}
		switch f.Edge {
		case "", "top", "right", "bottom", "left":
		default:
			return fmt.Errorf("%s.%s: edge %q is not top, right, bottom or left", path, f.ID, f.Edge)
		}
		switch f.Align {
		case "", "start", "center", "end":
		default:
			return fmt.Errorf("%s.%s: align %q is not start, center or end", path, f.ID, f.Align)
		}
		if f.Peek < 0 || f.Width < 0 || f.Height < 0 {
			return fmt.Errorf("%s.%s: sizes must not be negative", path, f.ID)
		}
	}
	return nil
}

func validateLayoutContent(c *LayoutContent, path string) error {
	if c == nil {
		return fmt.Errorf("%s: a block needs content", path)
	}
	n := 0
	if c.Mod != "" {
		n++
	}
	if c.Terminal != "" {
		n++
	}
	if c.Tab != "" {
		n++
	}
	if c.Agent != "" {
		n++
	}
	switch {
	case n == 0:
		return fmt.Errorf("%s: content needs a mod, terminal, tab or agent", path)
	case n > 1:
		return fmt.Errorf("%s: content names more than one of mod, terminal, tab and agent", path)
	}
	if c.Tab != "" && c.Tab != "active" {
		return fmt.Errorf("%s: tab %q is not \"active\"", path, c.Tab)
	}
	return nil
}

// LoadLayouts reads every valid layouts/*.json in dir. A file that does not
// parse or validate is left out and reported in warnings.
func LoadLayouts(dir string) (map[string]Layout, []string) {
	out := make(map[string]Layout)
	var warnings []string
	layoutDir := filepath.Join(dir, "layouts")
	entries, err := os.ReadDir(layoutDir)
	if err != nil {
		return out, warnings
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".json")
		if err := LayoutNameInvalid(name); err != nil {
			warnings = append(warnings, fmt.Sprintf("layouts/%s: %v", e.Name(), err))
			continue
		}
		data, err := os.ReadFile(filepath.Join(layoutDir, e.Name()))
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("layouts/%s: %v", e.Name(), err))
			continue
		}
		l, err := ParseLayout(data)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("layouts/%s: %v", e.Name(), err))
			continue
		}
		out[name] = l
	}
	return out, warnings
}

// LayoutNamesFromDir returns the built-in names followed by the valid user
// layouts in dir, sorted by name. Invalid files are reported in warnings.
func LayoutNamesFromDir(dir string) ([]string, []string) {
	loaded, warnings := LoadLayouts(dir)
	names := BuiltinLayoutNames()
	extra := make([]string, 0, len(loaded))
	for name := range loaded {
		if _, builtin := BuiltinLayout(name); !builtin {
			extra = append(extra, name)
		}
	}
	sort.Strings(extra)
	return append(names, extra...), warnings
}
