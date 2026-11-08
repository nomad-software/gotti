package ui

import (
	qt "github.com/mappu/miqt/qt6"

	"gotti/internal/appicon"
)

var appIconCache *qt.QIcon

// appIcon returns the icon of the program.
func appIcon() *qt.QIcon {
	if appIconCache == nil {
		pixmap := qt.NewQPixmap()
		// The icon keeps a copy of the picture.
		defer pixmap.Delete()
		pixmap.LoadFromDataWithData(appicon.PNG)
		appIconCache = qt.NewQIcon2(pixmap)
	}
	return appIconCache
}

// AppIcon returns the program's icon (for the application as a whole).
func AppIcon() *qt.QIcon { return appIcon() }
