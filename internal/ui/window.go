package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/nomad-software/qute/qtcore"
	"github.com/nomad-software/qute/qtgui"
	"github.com/nomad-software/qute/qtwidgets"

	"termite/internal/config"
	"termite/internal/desktop"
)

// Window is a top-level window with tabs.
type Window struct {
	app     *App
	w       *qtwidgets.QWidget
	toolbar *toolbar
	tabs    *qtwidgets.QTabWidget
	list    []*Tab
}

func newWindow(a *App) *Window {
	w := &Window{app: a}
	w.w = qtwidgets.NewQWidgetWith(&windowWidget{win: w}, nil)
	w.w.SetObjectName("termiteWindow")
	w.w.SetWindowTitle("")
	w.w.SetWindowIcon(AppIcon())
	width, height := a.cfg.WindowWidth, a.cfg.WindowHeight
	if width < 200 || height < 100 {
		width, height = 900, 560
	}
	w.w.Resize(width, height)
	lay := qtwidgets.NewQVBoxLayout(w.w)
	lay.SetContentsMargins(qtcore.QMargins{})
	lay.SetSpacing(0)
	w.tabs = newTabWidget(w.w)
	w.tabs.SetMovable(true)
	w.tabs.SetTabBarAutoHide(true)
	w.tabs.SetFocusPolicy(qtcore.NoFocus)
	w.tabs.TabBar().SetFocusPolicy(qtcore.NoFocus)
	w.toolbar = newToolbar(w)
	lay.AddWidget(w.toolbar.bar)
	lay.AddWidget(w.tabs)
	w.tabs.OnCurrentChanged(func(i int) {
		if t := w.tabAt(i); t != nil && t.focused != nil {
			t.focused.GL.SetFocus()
		}
		w.updateTitles()
		w.toolbar.update()
	})
	w.applyTheme()
	return w
}

// windowWidget is the top-level QWidget of a Window.
type windowWidget struct {
	qtwidgets.QWidgetBase
	win *Window
}

// CloseEvent overrides QWidget::closeEvent: the window's panes end with it.
func (ww *windowWidget) CloseEvent(ev *qtgui.QCloseEvent) {
	w := ww.win
	w.saveGeometry()
	for _, p := range w.allPanes() {
		p.exited = true
		p.releaseGL()
		p.S.Close()
	}
	ww.Super().CloseEvent(ev)
	w.app.windowClosed(w)
	// A closed window is not used again: delete it, and with it its
	// panes' widgets, which hold the last references to their terminals.
	w.w.DeleteLater()
}

func (w *Window) applyTheme() {
	th := w.app.theme
	family, size := w.app.cfg.HeaderFont()
	hex := func(c uint32) string { return fmt.Sprintf("#%06x", c) }
	// The rules name termite's own widgets only, so menus and dialogs keep
	// the desktop's native look. The tabs are flat, in the colours of the
	// window theme's palette.
	w.w.SetStyleSheet(strings.NewReplacer(
		"$bg", hex(th.Background),
		"$fg", hex(th.Foreground),
		"$sel", hex(th.Selection),
		"$headerBg", hex(th.HeaderBg),
		"$headerFg", hex(th.HeaderFg),
		"$focusBg", hex(th.HeaderFocusBg),
		"$focusFg", hex(th.HeaderFocusFg),
		"$font", strconv.Quote(family),
		"$size", strconv.FormatFloat(size, 'f', 1, 64),
	).Replace(`
		QWidget#termitePane { background: $bg; }
		QTabBar#termiteTabBar { background: palette(window); }
		QTabBar#termiteTabBar::tab {
			background: palette(window); color: palette(window-text);
			border: none; border-bottom: 2px solid palette(mid);
			padding: 6px 8px; margin: 0;
		}
		QTabBar#termiteTabBar::tab:hover:!selected { background: palette(button); }
		QTabBar#termiteTabBar::tab:selected { background: palette(base); border-bottom-color: palette(highlight); }
		QLineEdit#termiteSearch { background: $bg; color: $fg; border: 1px solid $sel; padding: 3px; }
		QLineEdit#termiteSearch[notFound="true"] { color: #dc322f; }
		QLabel#termiteHeader { background: $headerBg; color: $headerFg; padding: 2px 10px; font-family: $font; font-size: $sizept; }
		QLabel#termiteHeader[focused="true"] { background: $focusBg; color: $focusFg; }
		QLabel#termiteHeader[problem="true"] { color: #dc322f; }
	`))
}

func (w *Window) allPanes() []*Pane {
	var out []*Pane
	for _, t := range w.list {
		out = append(out, t.Panes()...)
	}
	return out
}

// tabAt returns the tab at index i, or nil. While a tab is moved Qt takes
// its page out and puts it back, and the focus leaving its terminal in
// between asks for an index that has no page.
func (w *Window) tabAt(i int) *Tab {
	if i < 0 || i >= w.tabs.Count() {
		return nil
	}
	wd := w.tabs.Widget(i)
	if wd == nil {
		return nil
	}
	for _, t := range w.list {
		if t.container.Ptr() == wd.Ptr() {
			return t
		}
	}
	return nil
}

// currentPane returns the focused pane of the current tab, or nil.
func (w *Window) currentPane() *Pane {
	if t := w.tabAt(w.tabs.CurrentIndex()); t != nil {
		return t.focused
	}
	return nil
}

func (w *Window) indexOf(t *Tab) int { return w.tabs.IndexOf(t.container) }

// NewTab opens a tab with one pane.
func (w *Window) NewTab(cwd string, cmd []string) *Tab {
	t := newTab(w)
	p, err := w.app.NewPane(t, cwd, cmd)
	if err != nil {
		t.container.DeleteLater()
		qtwidgets.QMessageBoxWarning(w.w, desktop.DisplayName, err.Error())
		return nil
	}
	t.setRoot(&Node{pane: p})
	t.focused = p
	w.list = append(w.list, t)
	idx := w.tabs.AddTab(t.container, desktop.DisplayName)
	w.tabs.TabBar().SetTabButton(idx, qtwidgets.QTabBarRightSide, tabCloseButton(func() { w.closeTab(t) }))
	w.tabs.SetCurrentIndex(idx)
	p.GL.SetFocus()
	return t
}

// closeTab closes the tab's panes, and with the last of them the tab.
func (w *Window) closeTab(t *Tab) {
	for _, p := range t.Panes() {
		p.close()
	}
}

func (w *Window) addLayoutTab(ln LayoutNode) {
	leaf := firstLeaf(ln)
	t := w.NewTab(leaf.Cwd, leaf.Command)
	if t == nil {
		return
	}
	w.app.buildLayout(t, t.root.pane, ln)
}

// loadLayout replaces the window's tabs with the layout's, opening on its
// first tab.
func (w *Window) loadLayout(lf *LayoutFile) {
	if len(lf.Tabs) == 0 {
		return
	}
	old := append([]*Tab(nil), w.list...)
	first := -1
	for _, ln := range lf.Tabs {
		n := len(w.list)
		w.addLayoutTab(ln)
		if first < 0 && len(w.list) > n {
			first = w.indexOf(w.list[n])
		}
	}
	if first < 0 {
		return // nothing opened: keep the current tabs
	}
	for _, t := range old {
		w.closeTab(t)
	}
	w.tabs.SetCurrentIndex(first)
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
	if cur := w.tabAt(w.tabs.CurrentIndex()); cur != nil && cur.focused != nil {
		w.w.SetWindowTitle(windowTitle(cur.focused))
	}
	for _, t := range w.list {
		p := t.focused
		if p == nil {
			continue
		}
		title := p.tabTitle()
		i := w.indexOf(t)
		if i < 0 {
			continue // its page is out while it moves
		}
		w.tabs.SetTabText(i, title) // the bar elides it, keeping the end
		w.tabs.SetTabToolTip(i, title)
	}
}

// windowTitle is the title of a window whose current pane is p: the last
// element of the directory its header shows, or of its shell's directory
// until the header has one.
func windowTitle(p *Pane) string {
	dir := p.headerDir
	if dir == "" {
		dir = p.cwd()
	}
	switch dir {
	case "":
		return p.tabTitle()
	case "/":
		return dir
	default:
		return filepath.Base(dir)
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
	// The move took the focus from the tab's terminal: give it back.
	if p := w.currentPane(); p != nil {
		p.GL.SetFocus()
	}
	w.updateTitles()
}

func paneRect(p *Pane) rect {
	x, y := mapToGlobal(p.GL, 0, 0)
	return rect{float64(x), float64(y), float64(p.GL.Width()), float64(p.GL.Height())}
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
	if i := neighbour(paneRect(p), probe, rects, dx, dy); i >= 0 {
		others[i].GL.SetFocus()
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
		width, height = g.Width, g.Height
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
		fmt.Fprintln(os.Stderr, "termite: the window size could not be saved:", err)
		return
	}
	c.WindowWidth, c.WindowHeight, c.WindowMaximized = width, height, maximized
}
