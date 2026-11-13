// Package icons holds the toolbar's and tabs' icons, drawn for termite as SVG in one
// style and built into the binary: outlines on a grid of 24, 1.5 wide with
// round ends and corners, in currentColor, which the UI replaces with the
// colour of the text they are drawn beside. It has no dependency on Qt.
package icons

import (
	"embed"
	"fmt"
)

// The names of the icons.
const (
	NewWindow       = "new-window"
	NewTab          = "new-tab"
	SplitHorizontal = "split-horizontal"
	SplitVertical   = "split-vertical"
	Broadcast       = "broadcast"
	Options         = "options"
	Exit            = "exit"
	Close           = "close"
)

// Names are every icon there is.
var Names = []string{NewWindow, NewTab, SplitHorizontal, SplitVertical, Broadcast, Options, Exit, Close}

//go:embed svg/*.svg
var files embed.FS

// SVG returns the SVG of the icon with the name.
func SVG(name string) ([]byte, error) {
	data, err := files.ReadFile("svg/" + name + ".svg")
	if err != nil {
		return nil, fmt.Errorf("there is no icon %q", name)
	}
	return data, nil
}
