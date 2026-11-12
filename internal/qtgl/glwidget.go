// Package qtgl provides the OpenGL widget the terminal is drawn in: a Go
// subclass of qute's QOpenGLWidget whose GL callbacks and input events go to
// Go functions.
package qtgl

import (
	"unsafe"

	"github.com/nomad-software/qute/qtcore"
	"github.com/nomad-software/qute/qtgui"
	"github.com/nomad-software/qute/qtopenglwidgets"
	"github.com/nomad-software/qute/qtwidgets"
)

// Widget is a QOpenGLWidget with Go callbacks. All callbacks run on the
// GUI thread with the widget's GL context current (except OnEvent).
type Widget struct {
	qtopenglwidgets.QOpenGLWidgetBase
	imRect qtcore.QRect

	OnInit   func()
	OnPaint  func()
	OnResize func(w, h int)
	// OnEvent receives input events; return true when handled. The event
	// is only valid until it returns.
	OnEvent func(ev *qtcore.QEvent) bool
}

// New creates the widget as a child of parent.
func New(parent qtwidgets.QWidgetLike) *Widget {
	w := &Widget{}
	qtopenglwidgets.NewQOpenGLWidgetWith(w, parent)
	w.SetAttribute(qtcore.WA_InputMethodEnabled)
	w.SetAttributeFull(qtcore.WA_KeyCompression, false)
	w.SetFocusPolicy(qtcore.StrongFocus)
	w.SetMouseTracking(true)
	w.SetUpdateBehavior(qtopenglwidgets.QOpenGLWidgetNoPartialUpdate)
	return w
}

// SetIMRect tells input methods where the text cursor is (widget coords).
func (w *Widget) SetIMRect(x, y, width, height int) {
	r := qtcore.QRect{X: x, Y: y, Width: width, Height: height}
	if r != w.imRect {
		w.imRect = r
		w.UpdateMicroFocus()
	}
}

// ProcAddress resolves a GL function through the current Qt context.
func ProcAddress(name string) unsafe.Pointer {
	ctx := qtgui.QOpenGLContextCurrentContext()
	if ctx == nil {
		return nil
	}
	return ctx.GetProcAddress(name)
}

// InitializeGL overrides QOpenGLWidget::initializeGL.
func (w *Widget) InitializeGL() {
	if w.OnInit != nil {
		w.OnInit()
	}
}

// PaintGL overrides QOpenGLWidget::paintGL.
func (w *Widget) PaintGL() {
	if w.OnPaint != nil {
		w.OnPaint()
	}
}

// ResizeGL overrides QOpenGLWidget::resizeGL.
func (w *Widget) ResizeGL(width, height int) {
	if w.OnResize != nil {
		w.OnResize(width, height)
	}
}

// FocusNextPrevChild keeps Tab and Shift+Tab for the terminal.
func (w *Widget) FocusNextPrevChild(bool) bool { return false }

// Event passes input events to OnEvent before Qt sees them.
func (w *Widget) Event(e *qtcore.QEvent) bool {
	switch e.Type() {
	case qtcore.QEventKeyPress, qtcore.QEventKeyRelease, qtcore.QEventShortcutOverride,
		qtcore.QEventMouseButtonPress, qtcore.QEventMouseButtonRelease,
		qtcore.QEventMouseButtonDblClick, qtcore.QEventMouseMove, qtcore.QEventWheel,
		qtcore.QEventFocusIn, qtcore.QEventFocusOut, qtcore.QEventInputMethod:
		if w.OnEvent != nil && w.OnEvent(e) {
			e.Accept()
			return true
		}
	}
	return w.Super().Event(e)
}

// InputMethodQuery tells input methods the widget takes text, and where
// the cursor is so that their windows open beside it.
func (w *Widget) InputMethodQuery(q qtcore.InputMethodQueries) *qtcore.QVariant {
	switch q {
	case qtcore.ImEnabled:
		return qtcore.NewQVariantOf(true)
	case qtcore.ImCursorRectangle, qtcore.ImAnchorRectangle:
		return qtcore.NewQVariantWithRect(w.imRect)
	case qtcore.ImCursorPosition:
		return qtcore.NewQVariantOf(0)
	case qtcore.ImSurroundingText:
		return qtcore.NewQVariantOf("")
	case qtcore.ImHints:
		return qtcore.NewQVariantOf(int(qtcore.ImhNoPredictiveText | qtcore.ImhNoAutoUppercase))
	}
	return w.Super().InputMethodQuery(q)
}
