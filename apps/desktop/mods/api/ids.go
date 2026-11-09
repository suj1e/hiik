package api

// Panel IDs shared by the shell and the built-in mods.
const (
	PanelSpaces   = "spaces/list"
	PanelAgents   = "agents/list"
	PanelTabs     = "tabs/strip"
	PanelStatus   = "status/bar"
	PanelFiles    = "files/panel"
	PanelChanges  = "changes/panel"
	PanelWorktree = "worktree/dialog"
	PanelPeek     = "peek/terminal"
	PanelPalette  = "palette/overlay"
	PanelSettings = "settings/panel"
	PanelAbout    = "about/panel"
	PanelPaneHead = "panes/header"
)

// Command IDs shared by the shell, keys, palette and mods.
const (
	CmdSpaceNew    = "space.new"
	CmdSpaceClose  = "space.close"
	CmdSpaceRename = "space.rename"
	CmdSpaceMove   = "space.move"
	CmdSpaceSelect = "space.select"
	CmdTabNew      = "tab.new"
	CmdTabClose    = "tab.close"
	CmdTabRename   = "tab.rename"
	CmdTabMove     = "tab.move"
	CmdTabSelect   = "tab.select"
	CmdPaneSplit   = "pane.split"
	CmdPaneClose   = "pane.close"
	CmdPaneZoom    = "pane.zoom"
	CmdPaneFocus   = "pane.focus"
	CmdPaneGo      = "pane.go"
	CmdPaneSwap    = "pane.swap"
	CmdPaneResize  = "pane.resize"
	CmdAgentNext   = "agent.next"
	CmdAgentPrev   = "agent.prev"
	CmdAgentFocus  = "agent.focus"
	CmdAgentPin    = "agent.pin"
	CmdAgentPeek   = "agent.peek"
	CmdPalette     = "app.palette"
	CmdSettings    = "app.settings"
	CmdAbout       = "app.about"
	CmdQuit        = "app.quit"
	CmdRetryCore   = "app.retry_core"
)
