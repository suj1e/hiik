package api

// Program is a program or agent the app can start in a tab.
//
// The shipped table is programs.json; a person's file in HIIK_HOME adds
// entries or changes named fields. Icon is a path in the person's folder or
// in the app's resources; Tint says the icon is a one-color glyph drawn in
// Color (or the text color), while a picture icon keeps its own colors. Tile
// is the badge's background, shown behind the icon.
type Program struct {
	Key       string   `json:"key"`
	Name      string   `json:"name"`
	Run       []string `json:"run"`
	Installed bool     `json:"installed"`
	Icon      string   `json:"icon,omitempty"`
	Tint      bool     `json:"tint,omitempty"`
	Tile      string   `json:"tile,omitempty"`
	Color     string   `json:"color,omitempty"`
	Match     []string `json:"match,omitempty"`
}
