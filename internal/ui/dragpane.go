package ui

import (
	"github.com/nomad-software/qute/qtcore"
	"github.com/nomad-software/qute/qtgui"
	"github.com/nomad-software/qute/qtwidgets"
)

// Dragging a pane's header onto another pane of the same tab moves the
// pane there, splitting the target: the edge of the target nearest the
// pointer is where it goes, shaded while dragging (termiteDropZone, a bare
// label in the focused header's colours). The header keeps the mouse from
// the press to the release (Qt grabs it implicitly), so no QDrag is needed.

// dropEdge is the side of a target pane a dragged pane is dropped against.
type dropEdge int

const (
	dropLeft dropEdge = iota
	dropRight
	dropTop
	dropBottom
)

// orient is the splitter orientation that puts a pane against e.
func (e dropEdge) orient() qtcore.Orientation {
	if e == dropLeft || e == dropRight {
		return qtcore.Horizontal
	}
	return qtcore.Vertical
}

// before reports whether the dropped pane goes ahead of the target.
func (e dropEdge) before() bool { return e == dropLeft || e == dropTop }

// edgeAt picks the edge of r nearest (x, y), measured in fractions of r's
// width and height so tall and wide panes split as readily either way.
func edgeAt(r rect, x, y float64) dropEdge {
	fx, fy := (x-r.x)/r.w, (y-r.y)/r.h
	e, d := dropLeft, fx
	for _, c := range []struct {
		e dropEdge
		d float64
	}{{dropRight, 1 - fx}, {dropTop, fy}, {dropBottom, 1 - fy}} {
		if c.d < d {
			e, d = c.e, c.d
		}
	}
	return e
}

// half is the part of r a pane dropped against e takes.
func (e dropEdge) half(r rect) rect {
	switch e {
	case dropLeft:
		r.w /= 2
	case dropRight:
		r.x += r.w / 2
		r.w /= 2
	case dropTop:
		r.h /= 2
	case dropBottom:
		r.y += r.h / 2
		r.h /= 2
	}
	return r
}

// widgetRect is w's rectangle in screen coordinates.
func widgetRect(w qtwidgets.QWidgetLike) rect {
	x, y := mapToGlobal(w, 0, 0)
	ww := w.AsQWidget()
	return rect{float64(x), float64(y), float64(ww.Width()), float64(ww.Height())}
}

// dropTarget finds the pane of p's tab, other than p, under the screen
// point (x, y), and the edge of it nearest the point.
func (p *Pane) dropTarget(x, y float64) (*Pane, dropEdge, rect) {
	for _, q := range p.tab.Panes() {
		r := widgetRect(q.Widget)
		if q == p || x < r.x || y < r.y || x >= r.x+r.w || y >= r.y+r.h {
			continue
		}
		e := edgeAt(r, x, y)
		return q, e, e.half(r)
	}
	return nil, 0, rect{}
}

// MousePressEvent overrides QWidget::mousePressEvent: a left press may
// start dragging the pane.
func (h *headerLabel) MousePressEvent(ev *qtgui.QMouseEvent) {
	if ev.Button() != qtcore.LeftButton {
		h.Super().MousePressEvent(ev)
		return
	}
	h.pressAt, h.pressed = ev.GlobalPosition(), true
}

// MouseMoveEvent overrides QWidget::mouseMoveEvent.
func (h *headerLabel) MouseMoveEvent(ev *qtgui.QMouseEvent) {
	p := h.p
	if !h.pressed || p.exited {
		return
	}
	g := ev.GlobalPosition()
	if p.tab.dragging != p {
		dx, dy := g.X-h.pressAt.X, g.Y-h.pressAt.Y
		lim := float64(qtwidgets.QApplicationStartDragDistance())
		if dx*dx+dy*dy < lim*lim || len(p.tab.Panes()) < 2 {
			return
		}
		p.tab.dragging = p
		setCursorShape(h, qtcore.ClosedHandCursor)
	}
	p.tab.showDropZone(p, g.X, g.Y)
}

// MouseReleaseEvent overrides QWidget::mouseReleaseEvent: the drop.
func (h *headerLabel) MouseReleaseEvent(ev *qtgui.QMouseEvent) {
	if !h.pressed || ev.Button() != qtcore.LeftButton {
		h.Super().MouseReleaseEvent(ev)
		return
	}
	h.pressed = false
	p := h.p
	t := p.tab
	if t.dragging != p || p.exited { // a press, not a drag, or cancelled
		return
	}
	t.cancelDrag()
	g := ev.GlobalPosition()
	// The target is found again here, not remembered from the last move:
	// it may have closed since.
	target, e, _ := p.dropTarget(g.X, g.Y)
	if target == nil {
		return
	}
	t.Move(p, target, e)
	t.focused = p
	p.GL.SetFocus()
	t.win.updateTitles()
}

// cancelDrag ends a drag in the tab, if there is one, leaving the panes
// where they are (Escape, or the drop, which then moves the pane itself).
func (t *Tab) cancelDrag() bool {
	p := t.dragging
	if p == nil {
		return false
	}
	t.dragging = nil
	p.header.UnsetCursor()
	t.hideDropZone()
	return true
}

// showDropZone shows where dragged pane p would go if dropped at the
// screen point (x, y), or nothing when it would stay put.
func (t *Tab) showDropZone(p *Pane, x, y float64) {
	target, _, r := p.dropTarget(x, y)
	if target == nil {
		t.hideDropZone()
		return
	}
	if t.dropZone == nil {
		t.dropZone = qtwidgets.NewQLabel(t.container)
		t.dropZone.SetObjectName("termiteDropZone")
		t.dropZone.SetAttribute(qtcore.WA_TransparentForMouseEvents)
	}
	o := t.container.MapFromGlobal(qtcore.QPoint{X: int(r.x), Y: int(r.y)})
	t.dropZone.SetGeometryRect(qtcore.QRect{X: o.X, Y: o.Y, Width: int(r.w), Height: int(r.h)})
	t.dropZone.Show()
	t.dropZone.Raise()
}

func (t *Tab) hideDropZone() {
	if t.dropZone != nil {
		t.dropZone.Hide()
	}
}
