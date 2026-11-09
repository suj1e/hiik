// The app's embedded program table. The data is programs.json in this
// directory; core merges a person's file over it.
package main

import (
	_ "embed"

	"hiik/core"
)

//go:embed programs.json
var bundledProgramsJSON []byte

func init() {
	core.SetBundledPrograms(bundledProgramsJSON)
}
