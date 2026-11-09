package ui

import (
	qt "github.com/mappu/miqt/qt6"
)

// Helpers for Qt values termite makes only to hand to Qt. miqt never frees
// an object made by qt.NewXxx (only those Qt returns by value), so each is
// deleted as soon as Qt has taken its copy.

// setBoolProperty sets a dynamic property, which stylesheets select on.
func setBoolProperty(w *qt.QWidget, name string, on bool) {
	v := qt.NewQVariant8(on)
	w.SetProperty(name, v)
	v.Delete()
}

// setCursorShape sets the mouse pointer shown over w.
func setCursorShape(w *qt.QWidget, shape qt.CursorShape) {
	c := qt.NewQCursor2(shape)
	w.SetCursor(c)
	c.Delete()
}

// openURL opens url with the desktop's handler for it.
func openURL(url string) bool {
	u := qt.NewQUrl3(url)
	defer u.Delete()
	return qt.QDesktopServices_OpenUrl(u)
}

// setKeySequence shows a key sequence, in Qt's portable text form.
func setKeySequence(e *qt.QKeySequenceEdit, seq string) {
	ks := qt.NewQKeySequence2(seq)
	e.SetKeySequence(ks)
	ks.Delete()
}

// mapToGlobal converts a point in w to screen coordinates.
func mapToGlobal(w *qt.QWidget, x, y int) (int, int) {
	pt := qt.NewQPoint2(x, y)
	defer pt.Delete()
	g := w.MapToGlobalWithQPoint(pt)
	return g.X(), g.Y()
}
