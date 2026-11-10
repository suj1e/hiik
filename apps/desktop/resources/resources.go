// Package resources holds assets shipped inside the binary. Program marks
// are embedded so a packaged app and a test read the same SVGs without
// guessing an install directory.
package resources

import "embed"

// Icons is the program marks under icons/, relative to this package.
//
//go:embed icons/*.svg
var Icons embed.FS
