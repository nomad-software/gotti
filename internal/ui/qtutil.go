package ui

import (
	"github.com/nomad-software/qute/qtcore"
	"github.com/nomad-software/qute/qtgui"
	"github.com/nomad-software/qute/qtwidgets"
)

// Helpers for Qt values termite makes only to hand to Qt. qute's GC cleanup
// frees each one once it is dropped.

// setBoolProperty sets a dynamic property, which stylesheets select on.
func setBoolProperty(w qtwidgets.QWidgetLike, name string, on bool) {
	w.AsQWidget().SetProperty(name, qtcore.NewQVariantOf(on))
}

// setCursorShape sets the mouse pointer shown over w.
func setCursorShape(w qtwidgets.QWidgetLike, shape qtcore.CursorShape) {
	w.AsQWidget().SetCursor(qtgui.NewQCursorWithCursorShape(shape))
}

// openURL opens url with the desktop's handler for it.
func openURL(url string) bool {
	return qtgui.QDesktopServicesOpenUrl(qtcore.NewQUrlWithUrl(url))
}

// setKeySequence shows a key sequence, in Qt's portable text form.
func setKeySequence(e *qtwidgets.QKeySequenceEdit, seq string) {
	e.SetKeySequence(qtgui.NewQKeySequenceWithKey(seq))
}

// mapToGlobal converts a point in w to screen coordinates.
func mapToGlobal(w qtwidgets.QWidgetLike, x, y int) (int, int) {
	g := w.AsQWidget().MapToGlobal(qtcore.QPoint{X: x, Y: y})
	return g.X, g.Y
}
