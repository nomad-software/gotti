package ui

import (
	"github.com/nomad-software/qute/qtgui"

	"termite/internal/appicon"
)

var appIconCache *qtgui.QIcon

// AppIcon returns the program's icon, made once and shared by every window.
func AppIcon() *qtgui.QIcon {
	if appIconCache == nil {
		pixmap := qtgui.NewQPixmap()
		pixmap.LoadFromData(appicon.PNG)
		appIconCache = qtgui.NewQIconWithPixmap(pixmap) // the icon keeps a copy
	}
	return appIconCache
}
