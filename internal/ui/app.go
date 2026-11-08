// Package ui builds gotti's Qt user interface: windows, tabs, split panes
// and the terminal widgets.
package ui

import (
	"fmt"
	"math"
	"os"
	"time"

	qt "github.com/mappu/miqt/qt6"
	"github.com/mappu/miqt/qt6/mainthread"

	"gotti/internal/config"
	"gotti/internal/desktop"
	"gotti/internal/render"
	"gotti/internal/term"
)

// Broadcast modes.
const (
	BroadcastOff   = iota
	BroadcastAll   // all panes in the current tab
	BroadcastGroup // all panes in the group, across tabs
)

// App owns all windows and global state.
type App struct {
	cfg        *config.Config
	cfgPath    string
	bindings   config.Bindings
	theme      config.Theme
	windows    []*Window
	caches     map[float64]*render.GlyphCache
	fontScale  float64
	focused    *Pane
	broadcast  int
	stopWatch  func()
	configErr  string
	defaultCmd []string
}

// NewApp creates the application controller.
func NewApp(cfg *config.Config, cfgPath string, cmd []string) *App {
	a := &App{cfg: cfg, cfgPath: cfgPath, caches: map[float64]*render.GlyphCache{}, fontScale: 1, defaultCmd: cmd}
	a.bindings = cfg.ParseBindings()
	a.theme = cfg.ResolveTheme()
	a.stopWatch = config.Watch(cfgPath, time.Second, func(c *config.Config, err error) {
		mainthread.Start(func() { a.reload(c, err) })
	})
	return a
}

// ---------------------------------------------------------------- config

func (a *App) reload(c *config.Config, err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "gotti:", err)
		if a.focused != nil {
			a.focused.Widget.SetToolTip(err.Error())
		}
		// keep running with whatever parsed; Load returns defaults on syntax errors
		if c == nil {
			return
		}
	}
	old := a.cfg
	a.cfg = c
	a.bindings = c.ParseBindings()
	a.theme = c.ResolveTheme()
	fontChanged := old.FontFamily != c.FontFamily || old.FontSize != c.FontSize || old.LineHeight != c.LineHeight
	if fontChanged {
		a.rebuildFonts()
	}
	for _, p := range a.allPanes() {
		a.applyThemeTo(p)
		p.T.Mu.Lock()
		p.T.AllowClipboardRead = c.OSC52 == "read-write"
		p.T.Mu.Unlock()
		if p.R != nil {
			p.R.Style = a.styleFor(p)
			p.R.Invalidate()
		}
		p.cols = 0 // padding or font may have changed
		p.updateGridSize()
		p.updateBlinkTimer()
		p.invalidate()
	}
	for _, w := range a.windows {
		w.applyTheme()
	}
}

func (a *App) applyThemeTo(p *Pane) {
	th := a.theme
	p.T.Mu.Lock()
	p.T.SetColors(th.Palette256(), th.Foreground, th.Background, th.Cursor)
	p.T.Mu.Unlock()
}

func (a *App) style() render.Style {
	th := a.theme
	cs := map[string]int{"block": term.CursorBlock, "beam": term.CursorBeam, "underline": term.CursorUnderline}[a.cfg.CursorStyle]
	return render.Style{
		SelBg: th.Selection, SelFg: th.SelText, CursorText: th.CursorText,
		MatchBg: mixRGB(th.Background, 0xb58900, 0.55), CurrentMatchBg: 0xcb4b16,
		BoldIsBright: a.cfg.BoldIsBright, Ligatures: a.cfg.Ligatures,
		CursorStyle: cs, CursorBlink: a.cfg.CursorBlink,
	}
}

func (a *App) styleFor(p *Pane) render.Style {
	s := a.style()
	pad := int(math.Round(float64(a.cfg.Padding) * p.dpr))
	s.PadX, s.PadY = pad, pad
	return s
}

func mixRGB(x, y uint32, t float64) uint32 {
	f := func(s uint) uint32 {
		return uint32(float64(x>>s&0xff)*(1-t)+float64(y>>s&0xff)*t) << s
	}
	return f(16) | f(8) | f(0)
}

// cacheFor returns the glyph cache for a device pixel ratio. A GL context
// must be current.
func (a *App) cacheFor(dpr float64) *render.GlyphCache {
	if c, ok := a.caches[dpr]; ok {
		return c
	}
	fs := render.NewFontSet(a.cfg.FontFamily, a.cfg.FontSize*a.fontScale, a.cfg.LineHeight, dpr)
	c := render.NewGlyphCache(fs)
	a.caches[dpr] = c
	return c
}

func (a *App) rebuildFonts() {
	for dpr, c := range a.caches {
		c.SetFonts(render.NewFontSet(a.cfg.FontFamily, a.cfg.FontSize*a.fontScale, a.cfg.LineHeight, dpr))
	}
	for _, p := range a.allPanes() {
		p.cols = 0
		p.updateGridSize()
		p.invalidate()
	}
}

// ---------------------------------------------------------------- windows

// NewWindow opens a window with one tab (or the given layout).
func (a *App) NewWindow(layout *LayoutFile) *Window {
	w := newWindow(a)
	a.windows = append(a.windows, w)
	if layout != nil && len(layout.Tabs) > 0 {
		for _, ln := range layout.Tabs {
			w.addLayoutTab(ln)
		}
	} else {
		w.NewTab("", a.defaultCmd)
	}
	if a.cfg.WindowMaximized {
		w.w.ShowMaximized()
	} else {
		w.w.Show()
	}
	w.w.ActivateWindow()
	return w
}

func (a *App) allPanes() []*Pane {
	var out []*Pane
	for _, w := range a.windows {
		for i := 0; i < w.tabs.Count(); i++ {
			if t := w.tabAt(i); t != nil {
				out = append(out, t.Panes()...)
			}
		}
	}
	return out
}

func (a *App) windowClosed(w *Window) {
	for i, x := range a.windows {
		if x == w {
			a.windows = append(a.windows[:i], a.windows[i+1:]...)
			break
		}
	}
	if len(a.windows) == 0 {
		a.stopWatch()
		qt.QCoreApplication_Quit()
	}
}

func (a *App) paneFocused(p *Pane) {
	a.focused = p
	p.tab.focused = p
	p.tab.win.updateTitles()
	if a.broadcast != BroadcastOff {
		for _, q := range a.allPanes() {
			q.GL.Update()
		}
	}
}

func (a *App) paneTitleChanged(p *Pane) { p.tab.win.updateTitles() }

func (a *App) bell(p *Pane) {
	w := p.tab.win.w
	if !w.IsActiveWindow() || a.focused != p {
		qt.QApplication_Alert(w)
	}
}

func (a *App) closePane(p *Pane) {
	if a.focused == p {
		a.focused = nil
	}
	t := p.tab
	empty := t.Remove(p)
	if empty {
		t.win.removeTab(t)
		return
	}
	if t.focused == p || t.focused == nil {
		ps := t.Panes()
		t.focused = ps[0]
	}
	t.focused.GL.SetFocus()
	t.win.updateTitles()
}

// ---------------------------------------------------------------- broadcast

func (a *App) inputTargets(p *Pane) []*Pane {
	switch a.broadcast {
	case BroadcastAll:
		return p.tab.Panes()
	case BroadcastGroup:
		if !p.InGroup {
			return []*Pane{p}
		}
		var out []*Pane
		for _, q := range a.allPanes() {
			if q.InGroup {
				out = append(out, q)
			}
		}
		return out
	}
	return []*Pane{p}
}

// borderFor returns the indicator border colour for a pane (RGBA).
func (a *App) borderFor(p *Pane) uint32 {
	red := uint32(0xdc) | 0x32<<8 | 0x2f<<16 | 0xff<<24
	orange := uint32(0xcb) | 0x4b<<8 | 0x16<<16 | 0xff<<24
	dimOrange := uint32(0xcb) | 0x4b<<8 | 0x16<<16 | 0x70<<24
	switch {
	case a.broadcast == BroadcastAll && a.focused != nil && a.focused.tab == p.tab:
		return red
	case a.broadcast == BroadcastGroup && p.InGroup:
		return orange
	case p.InGroup:
		return dimOrange
	}
	return 0
}

func (a *App) refreshBorders() {
	for _, q := range a.allPanes() {
		q.invalidate()
	}
}

// ---------------------------------------------------------------- actions

func (a *App) runAction(p *Pane, action string) {
	w := p.tab.win
	switch action {
	case "copy":
		if s := p.selectionText(); s != "" {
			qt.QGuiApplication_Clipboard().SetText(s)
		}
	case "paste":
		p.paste(qt.QGuiApplication_Clipboard().Text())
	case "paste_selection":
		p.paste(qt.QGuiApplication_Clipboard().TextWithMode(qt.QClipboard__Selection))
	case "new_tab":
		w.NewTab(p.cwd(), a.defaultCmd)
	case "new_window":
		a.NewWindow(nil)
	case "close_pane":
		p.close()
	case "split_horizontal":
		a.split(p, qt.Vertical)
	case "split_vertical":
		a.split(p, qt.Horizontal)
	case "focus_up":
		w.focusDirection(p, 0, -1)
	case "focus_down":
		w.focusDirection(p, 0, 1)
	case "focus_left":
		w.focusDirection(p, -1, 0)
	case "focus_right":
		w.focusDirection(p, 1, 0)
	case "next_tab":
		w.cycleTab(1)
	case "prev_tab":
		w.cycleTab(-1)
	case "move_tab_left":
		w.moveTab(-1)
	case "move_tab_right":
		w.moveTab(1)
	case "search":
		p.openSearch()
	case "scroll_page_up":
		p.scrollBy(-max(1, p.rows-1))
	case "scroll_page_down":
		p.scrollBy(max(1, p.rows-1))
	case "scroll_top":
		p.scrollToTop()
	case "scroll_bottom":
		p.scrollToBottom()
	case "font_bigger":
		a.fontScale = math.Min(a.fontScale*1.1, 5)
		a.rebuildFonts()
	case "font_smaller":
		a.fontScale = math.Max(a.fontScale/1.1, 0.3)
		a.rebuildFonts()
	case "font_reset":
		a.fontScale = 1
		a.rebuildFonts()
	case "broadcast_all":
		if a.broadcast == BroadcastAll {
			a.broadcast = BroadcastOff
		} else {
			a.broadcast = BroadcastAll
		}
		a.refreshBorders()
	case "broadcast_group":
		if a.broadcast == BroadcastGroup {
			a.broadcast = BroadcastOff
		} else {
			a.broadcast = BroadcastGroup
		}
		a.refreshBorders()
	case "group_toggle":
		p.InGroup = !p.InGroup
		a.refreshBorders()
	case "save_layout":
		ok := false
		name := qt.QInputDialog_GetText4(w.w, "Save layout", "Layout name:", qt.QLineEdit__Normal, "", &ok)
		if ok && name != "" {
			if err := w.SaveLayout(name); err != nil {
				qt.QMessageBox_Warning(w.w, desktop.DisplayName, err.Error())
			}
		}
		p.refocus()
	case "load_layout":
		names := ListLayouts()
		if len(names) == 0 {
			qt.QMessageBox_Information(w.w, desktop.DisplayName, "No saved layouts in "+layoutDir())
			return
		}
		ok := false
		name := qt.QInputDialog_GetItem4(w.w, "Load layout", "Layout:", names, 0, false, &ok)
		p.refocus()
		if ok {
			lf, err := LoadLayout(name)
			if err != nil {
				qt.QMessageBox_Warning(w.w, desktop.DisplayName, err.Error())
				return
			}
			for _, ln := range lf.Tabs {
				w.addLayoutTab(ln)
			}
		}
	case "context_menu":
		p.contextMenuAtCursor()
	case "clear_scrollback":
		p.T.Mu.Lock()
		p.T.Buf().ClearScrollback()
		p.T.Mu.Unlock()
		p.viewTop = -1
		p.view.Matches = nil
		p.clearSelection()
		p.invalidate()
	case "reset_terminal":
		p.T.Mu.Lock()
		p.T.Reset()
		p.T.Mu.Unlock()
		p.viewTop = -1
		p.clearSelection()
		p.updatePointer(false)
		p.updateBlinkTimer()
		p.invalidate()
	case "options":
		a.showOptions(p)
	case "edit_config":
		a.editConfig(p)
	case "reload_config":
		c, err := config.Load(a.cfgPath)
		a.reload(c, err)
	}
}

func (a *App) split(p *Pane, orient qt.Orientation) {
	np, err := a.NewPane(p.tab, p.cwd(), a.defaultCmd)
	if err != nil {
		qt.QMessageBox_Warning(p.Widget, desktop.DisplayName, err.Error())
		return
	}
	p.tab.Split(p, orient, np)
	np.GL.SetFocus()
}
