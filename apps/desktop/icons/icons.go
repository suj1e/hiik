// Package icons holds the small SVG set the shell and the built-in panels
// draw with ui.Icon. The shapes follow Lucide (ISC); see THIRD-PARTY-NOTICES.
package icons

import (
	_ "embed"

	"github.com/egoist/mygo/ui"
)

var (
	//go:embed svg/search.svg
	searchData []byte
	//go:embed svg/panel-left.svg
	panelLeftData []byte
	//go:embed svg/monitor.svg
	monitorData []byte
	//go:embed svg/server.svg
	serverData []byte
	//go:embed svg/branch.svg
	branchData []byte
	//go:embed svg/clock.svg
	clockData []byte
	//go:embed svg/layers.svg
	layersData []byte
	//go:embed svg/x.svg
	xData []byte
	//go:embed svg/folder.svg
	folderData []byte
	//go:embed svg/pin.svg
	pinData []byte
	//go:embed svg/terminal.svg
	terminalData []byte
	//go:embed svg/file.svg
	fileData []byte
	//go:embed svg/more.svg
	moreData []byte
	//go:embed svg/chevron-down.svg
	chevronDownData []byte
	//go:embed svg/chevron-right.svg
	chevronRightData []byte
	//go:embed svg/mark.svg
	markData []byte
)

var (
	Search       = ui.MustParseSVG(searchData)
	PanelLeft    = ui.MustParseSVG(panelLeftData)
	Monitor      = ui.MustParseSVG(monitorData)
	Server       = ui.MustParseSVG(serverData)
	Branch       = ui.MustParseSVG(branchData)
	Clock        = ui.MustParseSVG(clockData)
	Layers       = ui.MustParseSVG(layersData)
	X            = ui.MustParseSVG(xData)
	Folder       = ui.MustParseSVG(folderData)
	Pin          = ui.MustParseSVG(pinData)
	Terminal     = ui.MustParseSVG(terminalData)
	File         = ui.MustParseSVG(fileData)
	More         = ui.MustParseSVG(moreData)
	ChevronDown  = ui.MustParseSVG(chevronDownData)
	ChevronRight = ui.MustParseSVG(chevronRightData)
	Mark         = ui.MustParseSVG(markData)
)
