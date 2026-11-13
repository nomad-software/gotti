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

// Message boxes. Qt's static QMessageBox functions keep the box on the
// stack, so if its parent is deleted while it is open (a pane whose shell
// exits, or the window that closes with it) the box is destroyed twice.
// These keep it on the heap, where the parent's deletion frees it once.

// messageBox shows a modal box over parent and returns the button chosen,
// or 0 when the box went with its parent.
func messageBox(parent qtwidgets.QWidgetLike, icon qtwidgets.QMessageBoxIcon, title, text string, buttons qtwidgets.QMessageBoxStandardButtons) qtwidgets.QMessageBoxStandardButtons {
	box := qtwidgets.NewQMessageBoxWithIcon(icon, title, text, buttons, parent)
	r := qtwidgets.QMessageBoxStandardButtons(box.Exec())
	if !box.Alive() {
		return 0
	}
	box.DeleteLater()
	return r
}

// ask asks a yes/no question; closing the box answers no.
func ask(parent qtwidgets.QWidgetLike, title, text string) bool {
	return messageBox(parent, qtwidgets.QMessageBoxIconQuestion, title, text, qtwidgets.QMessageBoxYes|qtwidgets.QMessageBoxNo) == qtwidgets.QMessageBoxYes
}

func warn(parent qtwidgets.QWidgetLike, title, text string) {
	messageBox(parent, qtwidgets.QMessageBoxIconWarning, title, text, qtwidgets.QMessageBoxOk)
}

func inform(parent qtwidgets.QWidgetLike, title, text string) {
	messageBox(parent, qtwidgets.QMessageBoxIconInformation, title, text, qtwidgets.QMessageBoxOk)
}
