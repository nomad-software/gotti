package ui

import (
	"testing"

	"github.com/nomad-software/qute/qtcore"
	"github.com/nomad-software/qute/qtgui"
	"github.com/nomad-software/qute/qutetest"
)

func TestEdgeAt(t *testing.T) {
	r := rect{100, 50, 400, 100} // wide: fractions, not pixels, decide
	for _, c := range []struct {
		x, y float64
		want dropEdge
	}{
		{110, 100, dropLeft},
		{490, 100, dropRight},
		{300, 55, dropTop},
		{300, 145, dropBottom},
		{150, 60, dropTop},  // 12.5% across, 10% down
		{120, 90, dropLeft}, // 5% across, 40% down
	} {
		if got := edgeAt(r, c.x, c.y); got != c.want {
			t.Errorf("edgeAt(%v, %v) = %v, want %v", c.x, c.y, got, c.want)
		}
	}
}

func TestDropHalf(t *testing.T) {
	r := rect{10, 20, 100, 60}
	for e, want := range map[dropEdge]rect{
		dropLeft:   {10, 20, 50, 60},
		dropRight:  {60, 20, 50, 60},
		dropTop:    {10, 20, 100, 30},
		dropBottom: {10, 50, 100, 30},
	} {
		if got := e.half(r); got != want {
			t.Errorf("%v.half = %v, want %v", e, got, want)
		}
		if got := e.orient() == qtcore.Horizontal; got != (e == dropLeft || e == dropRight) {
			t.Errorf("%v.orient is wrong", e)
		}
	}
}

// dragHeader drags p's header to the screen point (x, y) as the mouse
// would: press, a move past the drag distance, a move to the point, and
// the release there. beforeRelease, if any, runs before the release.
func dragHeader(p *Pane, x, y float64, beforeRelease ...func()) {
	h := p.header
	gx, gy := mapToGlobal(h, 5, 5)
	send := func(typ qtcore.QEventType, x, y float64, button, buttons qtcore.MouseButtons) {
		lx, ly := mapToGlobal(h, 0, 0)
		local := qtcore.QPointF{X: x - float64(lx), Y: y - float64(ly)}
		ev := qtgui.NewQMouseEvent(typ, local, qtcore.QPointF{X: x, Y: y}, button, buttons, qtcore.NoModifier)
		qtcore.QCoreApplicationSendEvent(h, ev)
	}
	send(qtcore.QEventMouseButtonPress, float64(gx), float64(gy), qtcore.LeftButton, qtcore.LeftButton)
	send(qtcore.QEventMouseMove, float64(gx+40), float64(gy+40), qtcore.NoButton, qtcore.LeftButton)
	send(qtcore.QEventMouseMove, x, y, qtcore.NoButton, qtcore.LeftButton)
	for _, f := range beforeRelease {
		f()
	}
	send(qtcore.QEventMouseButtonRelease, x, y, qtcore.LeftButton, qtcore.NoButton)
}

// layoutShape describes a split tree: pane numbers, with "|" for side by
// side and "/" for stacked.
func layoutShape(n *Node, num map[*Pane]int) string {
	if n.pane != nil {
		return string(rune('A' + num[n.pane]))
	}
	sep := "|"
	if n.split.Orientation() == qtcore.Vertical {
		sep = "/"
	}
	s := "("
	for i, c := range n.children {
		if i > 0 {
			s += sep
		}
		s += layoutShape(c, num)
	}
	if got := n.split.Count(); got != len(n.children) {
		s += "!" // the splitter and the tree disagree
	}
	return s + ")"
}

// TestDragPane drags pane headers onto other panes and checks the tree,
// the splitters holding the widgets and where the focus ends up.
func TestDragPane(t *testing.T) {
	needGUI(t)
	qutetest.Run(t, func() {
		a, w := newTestApp(t)
		a.runAction(focusedPane(t, w), "split_vertical")
		qutetest.Wait(100)
		a.runAction(focusedPane(t, w), "split_vertical")
		qutetest.Wait(100)
		tab := w.tabAt(0)
		ps := tab.Panes()
		num := map[*Pane]int{}
		for i, p := range ps {
			num[p] = i
		}
		if got := layoutShape(tab.root, num); got != "(A|B|C)" {
			t.Fatalf("start: %s", got)
		}
		at := func(q *Pane, fx, fy float64) (float64, float64) {
			r := widgetRect(q.Widget)
			return r.x + r.w*fx, r.y + r.h*fy
		}
		for _, c := range []struct {
			drag, onto *Pane
			fx, fy     float64
			want       string
		}{
			{ps[0], ps[2], 0.5, 0.9, "(B|(C/A))"},
			{ps[1], ps[0], 0.1, 0.5, "(C/(B|A))"},
			{ps[2], ps[1], 0.5, 0.1, "((C/B)|A)"},
		} {
			before := layoutShape(tab.root, num)
			dragHeader(c.drag, -100, -100) // off the panes: nothing changes
			x, y := at(c.drag, 0.5, 0.9)
			dragHeader(c.drag, x, y) // onto itself: nothing changes
			if got := layoutShape(tab.root, num); got != before {
				t.Fatalf("dragging %c nowhere changed %s to %s", 'A'+num[c.drag], before, got)
			}
			x, y = at(c.onto, c.fx, c.fy)
			dragHeader(c.drag, x, y, func() { // Escape cancels the drag
				if !tab.dropZone.IsVisible() {
					t.Error("no drop zone while dragging")
				}
				qutetest.Key(focusedPane(t, w).GL, qtcore.Key_Escape, qtcore.NoModifier)
			})
			if got := layoutShape(tab.root, num); got != before || tab.dropZone.IsVisible() {
				t.Fatalf("dragging %c then Escape: %s, drop zone %v", 'A'+num[c.drag], got, tab.dropZone.IsVisible())
			}
			dragHeader(c.drag, x, y)
			qutetest.Wait(100)
			if got := layoutShape(tab.root, num); got != c.want {
				t.Errorf("dragging %c onto %c: %s, want %s", 'A'+num[c.drag], 'A'+num[c.onto], got, c.want)
			}
			if f := focusedPane(t, w); f != c.drag {
				t.Errorf("after dragging %c: focus on %c", 'A'+num[c.drag], 'A'+num[f])
			}
		}
		if tab.dropZone == nil || tab.dropZone.IsVisible() {
			t.Error("the drop zone is still showing")
		}
		for _, p := range ps {
			if !p.Widget.IsVisible() || p.Widget.Window().Ptr() != w.w.Ptr() {
				t.Errorf("pane %c is hidden or out of the window", 'A'+num[p])
			}
		}
	})
}

// TestDragPaneFlattens drags panes until a splitter is left holding only a
// splitter of its parent's orientation: its panes must join the parent
// rather than nest (columns within columns could not be saved and built
// again).
func TestDragPaneFlattens(t *testing.T) {
	needGUI(t)
	qutetest.Run(t, func() {
		a, w := newTestApp(t)
		for range 3 {
			a.runAction(focusedPane(t, w), "split_vertical")
			qutetest.Wait(100)
		}
		tab := w.tabAt(0)
		ps := tab.Panes()
		num := map[*Pane]int{}
		for i, p := range ps {
			num[p] = i
		}
		drop := func(drag, onto *Pane, fx, fy float64, want string) {
			r := widgetRect(onto.Widget)
			dragHeader(drag, r.x+r.w*fx, r.y+r.h*fy)
			qutetest.Wait(100)
			if got := layoutShape(tab.root, num); got != want {
				t.Fatalf("dragging %c onto %c: %s, want %s", 'A'+num[drag], 'A'+num[onto], got, want)
			}
		}
		drop(ps[1], ps[2], 0.5, 0.9, "(A|(C/B)|D)")
		drop(ps[3], ps[1], 0.9, 0.5, "(A|(C/(B|D)))")
		drop(ps[2], ps[0], 0.5, 0.9, "((A/C)|B|D)")
		ln := tab.root.save()
		if len(ln.Sizes) != 3 || ln.Sizes[1] == 0 || ln.Sizes[2] == 0 {
			t.Errorf("sizes after flattening: %v", ln.Sizes)
		}
	})
}
