package ui

import (
	qt "github.com/mappu/miqt/qt6"

	"termite/internal/appicon"
)

var appIconCache *qt.QIcon

// AppIcon returns the program's icon, made once and shared by every window.
func AppIcon() *qt.QIcon {
	if appIconCache == nil {
		pixmap := qt.NewQPixmap()
		// The icon keeps a copy of the picture.
		defer pixmap.Delete()
		pixmap.LoadFromDataWithData(appicon.PNG)
		appIconCache = qt.NewQIcon2(pixmap)
	}
	return appIconCache
}
