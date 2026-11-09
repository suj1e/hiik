package core

// extraCommands are commands registered by feature files without editing
// the built-in registry. registerCommand runs from init, before buildCommands.
var extraCommands []*cmd

// registerCommand adds a command for the next App. A duplicate ID is
// ignored at build time, so a built-in keeps its place.
func registerCommand(c *cmd) {
	if c == nil || c.id == "" {
		panic("core: a registered command needs an ID")
	}
	extraCommands = append(extraCommands, c)
}
