package ui

import (
	"fmt"
	"os"

	qt "github.com/mappu/miqt/qt6"

	"gotti/internal/config"
	"gotti/internal/desktop"
)

// Window is a top-level window with tabs.
type Window struct {
	app  *App
	w    *qt.QWidget
	tabs *qt.QTabWidget
	list []*Tab
}

func newWindow(a *App) *Window {
	w := &Window{app: a}
	w.w = qt.NewQWidget(nil)
	w.w.SetObjectName("gottiWindow")
	w.w.SetWindowTitle("")
	w.w.SetWindowIcon(appIcon())
	width, height := a.cfg.WindowWidth, a.cfg.WindowHeight
	if width < 200 || height < 100 {
		width, height = 900, 560
	}
	w.w.Resize(width, height)
	lay := qt.NewQVBoxLayout(w.w)
	lay.SetContentsMargins(0, 0, 0, 0)
	w.tabs = qt.NewQTabWidget(w.w)
	w.tabs.SetObjectName("gottiTabs")
	w.tabs.TabBar().SetObjectName("gottiTabBar")
	w.tabs.SetMovable(true)
	w.tabs.SetTabsClosable(true)
	w.tabs.SetDocumentMode(true)
	w.tabs.SetTabBarAutoHide(true)
	w.tabs.SetFocusPolicy(qt.NoFocus)
	w.tabs.TabBar().SetFocusPolicy(qt.NoFocus)
	lay.AddWidget(w.tabs.QWidget)
	w.tabs.OnTabCloseRequested(func(i int) {
		if t := w.tabAt(i); t != nil {
			for _, p := range t.Panes() {
				p.close()
			}
		}
	})
	w.tabs.OnCurrentChanged(func(i int) {
		if t := w.tabAt(i); t != nil && t.focused != nil {
			t.focused.GL.SetFocus()
		}
		w.updateTitles()
	})
	w.w.OnCloseEvent(func(super func(*qt.QCloseEvent), ev *qt.QCloseEvent) {
		w.saveGeometry()
		for _, p := range w.allPanes() {
			p.exited = true
			p.releaseGL()
			p.S.Close()
		}
		super(ev)
		w.app.windowClosed(w)
	})
	w.applyTheme()
	return w
}

// headerFontScale sizes the pane header font relative to the terminal's.
const headerFontScale = 0.85

func (w *Window) applyTheme() {
	th := w.app.theme
	bg := fmt.Sprintf("#%06x", th.Background)
	fg := fmt.Sprintf("#%06x", th.Foreground)
	sel := fmt.Sprintf("#%06x", th.Selection)
	// The rules name gotti's own widgets only, so menus and dialogs keep
	// the desktop's native look.
	w.w.SetStyleSheet(fmt.Sprintf(`
		QWidget#gottiTab, QWidget#gottiPane { background: %[1]s; }
		QSplitter#gottiSplit::handle { background: %[3]s; }
		QLineEdit#gottiSearch { background: %[1]s; color: %[2]s; border: 1px solid %[3]s; padding: 3px; }
		QLineEdit#gottiSearch[notFound="true"] { color: #dc322f; }
		QLabel#gottiHeader { background: %[4]s; color: %[5]s; padding: 2px 10px; font-family: %[6]q; font-size: %.1[7]fpt; }
		QLabel#gottiHeader[focused="true"] { color: %[2]s; }
		QLabel#gottiHeader[problem="true"] { color: #dc322f; }
	`, bg, fg, sel, fmt.Sprintf("#%06x", config.Mix(th.Background, 0x000000, 0.4)), // 40% black over the background
		fmt.Sprintf("#%06x", config.Mix(th.Background, th.Foreground, 0.55)),
		w.app.cfg.FontFamily, w.app.cfg.FontSize*headerFontScale))
}

func (w *Window) allPanes() []*Pane {
	var out []*Pane
	for _, t := range w.list {
		out = append(out, t.Panes()...)
	}
	return out
}

func (w *Window) tabAt(i int) *Tab {
	if i < 0 || i >= w.tabs.Count() {
		return nil
	}
	wd := w.tabs.Widget(i)
	for _, t := range w.list {
		if t.container.UnsafePointer() == wd.UnsafePointer() {
			return t
		}
	}
	return nil
}

func (w *Window) indexOf(t *Tab) int { return w.tabs.IndexOf(t.container) }

// NewTab opens a tab with one pane.
func (w *Window) NewTab(cwd string, cmd []string) *Tab {
	t := newTab(w)
	p, err := w.app.NewPane(t, cwd, cmd)
	if err != nil {
		qt.QMessageBox_Warning(w.w, desktop.DisplayName, err.Error())
		return nil
	}
	t.setRoot(&Node{pane: p})
	t.focused = p
	w.list = append(w.list, t)
	idx := w.tabs.AddTab(t.container, desktop.DisplayName)
	w.tabs.SetCurrentIndex(idx)
	p.GL.SetFocus()
	return t
}

func (w *Window) addLayoutTab(ln LayoutNode) {
	leaf := firstLeaf(ln)
	t := w.NewTab(leaf.Cwd, leaf.Command)
	if t == nil {
		return
	}
	w.app.buildLayout(t, t.root.pane, ln)
}

func (w *Window) removeTab(t *Tab) {
	if i := w.indexOf(t); i >= 0 {
		w.tabs.RemoveTab(i)
	}
	for i, x := range w.list {
		if x == t {
			w.list = append(w.list[:i], w.list[i+1:]...)
			break
		}
	}
	t.container.DeleteLater()
	if len(w.list) == 0 {
		w.w.Close()
		return
	}
	if cur := w.tabAt(w.tabs.CurrentIndex()); cur != nil && cur.focused != nil {
		cur.focused.GL.SetFocus()
	}
}

func (w *Window) updateTitles() {
	for _, t := range w.list {
		p := t.focused
		if p == nil {
			continue
		}
		title := p.tabTitle()
		short := title
		if r := []rune(short); len(r) > 30 {
			short = "…" + string(r[len(r)-29:]) // keep the end, like the header
		}
		w.tabs.SetTabText(w.indexOf(t), short)
		w.tabs.SetTabToolTip(w.indexOf(t), title)
	}
}

func (w *Window) cycleTab(d int) {
	n := w.tabs.Count()
	if n > 1 {
		w.tabs.SetCurrentIndex((w.tabs.CurrentIndex() + d + n) % n)
	}
}

func (w *Window) moveTab(d int) {
	i := w.tabs.CurrentIndex()
	j := i + d
	if j < 0 || j >= w.tabs.Count() {
		return
	}
	w.tabs.TabBar().MoveTab(i, j)
}

func paneRect(p *Pane) rect {
	g := p.GL.MapToGlobalWithQPoint(qt.NewQPoint2(0, 0))
	return rect{float64(g.X()), float64(g.Y()), float64(p.GL.Width()), float64(p.GL.Height())}
}

// cursorPoint returns the global position of the middle of p's text cursor.
func cursorPoint(p *Pane) (float64, float64) {
	r := paneRect(p)
	if p.R == nil {
		return r.x + r.w/2, r.y + r.h/2
	}
	p.T.Mu.Lock()
	cx, cy := p.T.CursorPos()
	p.T.Mu.Unlock()
	cw, ch := p.R.CellSize()
	st := p.R.Style
	return r.x + (float64(st.PadX)+(float64(cx)+0.5)*float64(cw))/p.dpr,
		r.y + (float64(st.PadY)+(float64(cy)+0.5)*float64(ch))/p.dpr
}

// focusDirection moves focus to the neighbouring pane in direction (dx, dy).
func (w *Window) focusDirection(p *Pane, dx, dy int) {
	panes := p.tab.Panes()
	var others []*Pane
	var rects []rect
	for _, q := range panes {
		if q != p {
			others = append(others, q)
			rects = append(rects, paneRect(q))
		}
	}
	px, py := cursorPoint(p)
	probe := py
	if dy != 0 {
		probe = px
	}
	var best *Pane
	if i := neighbour(paneRect(p), probe, rects, dx, dy); i >= 0 {
		best = others[i]
	}
	if best != nil {
		best.GL.SetFocus()
	}
}

// saveGeometry stores the window's size in the config file, so new windows
// (and the next start) open the same size. A maximized window keeps the
// size it had before it was maximized.
func (w *Window) saveGeometry() {
	maximized := w.w.IsMaximized()
	width, height := w.w.Width(), w.w.Height()
	if maximized {
		g := w.w.NormalGeometry()
		width, height = g.Width(), g.Height()
	}
	if width < 200 || height < 100 {
		return
	}
	c := w.app.cfg
	if c.WindowWidth == width && c.WindowHeight == height && c.WindowMaximized == maximized {
		return
	}
	err := config.SetValues(w.app.cfgPath, map[string]any{
		"window_width": width, "window_height": height, "window_maximized": maximized,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "gotti: the window size could not be saved:", err)
		return
	}
	c.WindowWidth, c.WindowHeight, c.WindowMaximized = width, height, maximized
}
