package ui

import (
	"testing"

	"github.com/nomad-software/qute/qtcore"
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

// dragHeader drags p's header to the screen point (x, y) with the left
// button. beforeRelease, if any, runs while it is still held there.
func dragHeader(p *Pane, x, y float64, beforeRelease ...func()) {
	h := p.header
	lx, ly := mapToGlobal(h, 0, 0)
	from, to := qtcore.QPoint{X: 5, Y: 5}, qtcore.QPoint{X: int(x) - lx, Y: int(y) - ly}
	if len(beforeRelease) == 0 {
		qutetest.Drag(h, qtcore.LeftButton, from, to)
		return
	}
	qutetest.Press(h, qtcore.LeftButton, qtcore.NoModifier, from)
	qutetest.Move(h, qtcore.QPoint{X: from.X + 40, Y: from.Y + 40})
	qutetest.Move(h, to)
	for _, f := range beforeRelease {
		f()
	}
	qutetest.Release(h, qtcore.LeftButton, qtcore.NoModifier, to)
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
				// Moving on with the button still held must not start it again.
				lx, ly := mapToGlobal(c.drag.header, 0, 0)
				qutetest.Move(c.drag.header, qtcore.QPoint{X: int(x) - lx + 3, Y: int(y) - ly + 3})
				if tab.dragging != nil || tab.dropZone.IsVisible() {
					t.Error("the drag started again after Escape")
				}
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

// TestLayoutRoundTrip saves nested layouts with uneven sizes and opens
// them again in a new window: the same tree, in the same proportions.
func TestLayoutRoundTrip(t *testing.T) {
	needGUI(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	qutetest.Run(t, func() {
		// C goes below B, then below A: the first child split as well.
		for _, onto := range []int{1, 0} {
			roundTrip(t, onto)
		}
	})
}

func roundTrip(t *testing.T, onto int) {
	a, w := newTestApp(t)
	for range 2 {
		a.runAction(focusedPane(t, w), "split_vertical")
		qutetest.Wait(100)
	}
	tab := w.tabAt(0)
	ps := tab.Panes()
	r := widgetRect(ps[onto].Widget)
	dragHeader(ps[2], r.x+r.w/2, r.y+r.h*0.9) // (A|(B/C)) or ((A/C)|B)
	qutetest.Wait(100)
	total := func(s []int) (n int) {
		for _, v := range s {
			n += v
		}
		return n
	}
	outer, inner := tab.root.split, tab.root.children[onto].split
	o, i := total(outer.Sizes()), total(inner.Sizes())
	outer.SetSizes([]int{o / 4, o - o/4})
	inner.SetSizes([]int{i * 2 / 3, i - i*2/3})
	qutetest.Wait(100)
	if err := w.SaveLayout("round trip"); err != nil {
		t.Fatal(err)
	}
	lf, err := LoadLayout("round trip")
	if err != nil {
		t.Fatal(err)
	}
	w2 := a.NewWindow(lf)
	qutetest.Wait(300)
	var compare func(path string, g, w LayoutNode)
	compare = func(path string, g, w LayoutNode) {
		if g.Split != w.Split || len(g.Children) != len(w.Children) {
			t.Fatalf("%s: %q with %d children, want %q with %d", path, g.Split, len(g.Children), w.Split, len(w.Children))
		}
		gt, wt := total(g.Sizes), total(w.Sizes)
		for k := range g.Sizes {
			gf, wf := float64(g.Sizes[k])/float64(gt), float64(w.Sizes[k])/float64(wt)
			if gf-wf > 0.03 || wf-gf > 0.03 {
				t.Errorf("%s: sizes %v, want the proportions of %v", path, g.Sizes, w.Sizes)
				break
			}
		}
		for k := range g.Children {
			compare(path+"/"+string(rune('0'+k)), g.Children[k], w.Children[k])
		}
	}
	compare(string(rune('0'+onto)), w2.tabAt(0).root.save(), tab.root.save())
}
