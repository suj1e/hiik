package ui

import (
	_ "embed"

	mui "github.com/egoist/mygo/ui"
)

// The Files and Changes glyphs are the reference app's assets, drawn in
// the current text color.
//
//go:embed tabicons/files.svg
var filesIconData []byte

//go:embed tabicons/changes.svg
var changesIconData []byte

var (
	filesTabIcon   = mui.MustParseSVG(filesIconData)
	changesTabIcon = mui.MustParseSVG(changesIconData)
)
