// Package qtgl provides an OpenGL widget for miqt (Qt 6) applications.
package qtgl

/*
#cgo CXXFLAGS: -std=c++17 -fPIC
#cgo pkg-config: Qt6OpenGLWidgets Qt6OpenGL Qt6Widgets
#include <stdlib.h>
#include "glwidget.h"
*/
import "C"

import (
	"runtime/cgo"
	"unsafe"

	qt "github.com/mappu/miqt/qt6"
)

// Widget is a QOpenGLWidget with Go callbacks. All callbacks run on the
// Qt main thread with the widget's GL context current (except OnEvent).
type Widget struct {
	*qt.QWidget
	ptr    unsafe.Pointer
	handle cgo.Handle

	OnInit   func()
	OnPaint  func()
	OnResize func(w, h int)
	// OnEvent receives input events; return true when handled.
	OnEvent func(ev *qt.QEvent, raw unsafe.Pointer) bool
}

// New creates the widget as a child of parent (which may be nil).
func New(parent *qt.QWidget) *Widget {
	w := &Widget{}
	w.handle = cgo.NewHandle(w)
	var p unsafe.Pointer
	if parent != nil {
		p = parent.UnsafePointer()
	}
	w.ptr = C.termite_glwidget_new(C.uintptr_t(w.handle), p)
	w.QWidget = qt.UnsafeNewQWidget(w.ptr)
	w.QWidget.OnDestroyed(func() { w.handle.Delete() })
	return w
}

// SetIMRect tells input methods where the text cursor is (widget coords).
func (w *Widget) SetIMRect(x, y, width, height int) {
	C.termite_glwidget_set_im_rect(w.ptr, C.int(x), C.int(y), C.int(width), C.int(height))
}

// MakeCurrent makes the widget's context current (outside paint).
func (w *Widget) MakeCurrent() { C.termite_glwidget_make_current(w.ptr) }

// DoneCurrent releases the context.
func (w *Widget) DoneCurrent() { C.termite_glwidget_done_current(w.ptr) }

// ProcAddress resolves a GL function through the current Qt context.
func ProcAddress(name string) unsafe.Pointer {
	cs := C.CString(name)
	defer C.free(unsafe.Pointer(cs))
	return C.termite_get_proc_address(cs)
}

func fromHandle(h C.uintptr_t) *Widget {
	return cgo.Handle(h).Value().(*Widget)
}

//export termiteInitializeGL
func termiteInitializeGL(h C.uintptr_t) {
	if w := fromHandle(h); w.OnInit != nil {
		w.OnInit()
	}
}

//export termitePaintGL
func termitePaintGL(h C.uintptr_t) {
	if w := fromHandle(h); w.OnPaint != nil {
		w.OnPaint()
	}
}

//export termiteResizeGL
func termiteResizeGL(h C.uintptr_t, width, height C.int) {
	if w := fromHandle(h); w.OnResize != nil {
		w.OnResize(int(width), int(height))
	}
}

//export termiteEvent
func termiteEvent(h C.uintptr_t, ev unsafe.Pointer) C.int {
	w := fromHandle(h)
	if w.OnEvent != nil && w.OnEvent(qt.UnsafeNewQEvent(ev), ev) {
		return 1
	}
	return 0
}
