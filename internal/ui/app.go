// Package ui builds termite's Qt user interface: windows, tabs, split panes
// and the terminal widgets.
package ui

import (
	"fmt"
	"math"
	"os"

	"github.com/nomad-software/qute/qtcore"
	"github.com/nomad-software/qute/qtgui"
	"github.com/nomad-software/qute/qtwidgets"

	"termite/internal/config"
	"termite/internal/desktop"
	"termite/internal/render"
	"termite/internal/term"
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
	configErr  string // the config file's latest problem, shown in pane headers
	defaultCmd []string
}

// NewApp creates the application controller. loadErr is the problem
// config.Load found with the file, if any, which the pane headers show.
func NewApp(cfg *config.Config, cfgPath string, loadErr error, cmd []string) *App {
	a := &App{cfg: cfg, cfgPath: cfgPath, caches: map[float64]*render.GlyphCache{}, fontScale: 1, defaultCmd: cmd}
	if loadErr != nil {
		a.configErr = loadErr.Error()
	}
	a.bindings = cfg.ParseBindings()
	a.theme = cfg.ResolveTheme()
	applyWindowTheme(cfg.WindowTheme)
	return a
}

// NewApplication creates the QApplication, with every GL context shared
// (glyph atlases are shared by the panes) and OpenGL 3.3 core as the default
// format. Importing qute locks the main goroutine to the main OS thread,
// where every Qt and GL call is made.
func NewApplication() {
	qtcore.QCoreApplicationSetAttribute(qtcore.AA_ShareOpenGLContexts)
	f := qtgui.NewQSurfaceFormat()
	f.SetRenderableType(qtgui.QSurfaceFormatOpenGL)
	f.SetVersion(3, 3)
	f.SetProfile(qtgui.QSurfaceFormatCoreProfile)
	f.SetDepthBufferSize(0)
	f.SetStencilBufferSize(0)
	f.SetSwapInterval(1) // vsync: at most one frame per refresh
	qtgui.QSurfaceFormatSetDefaultFormat(f)
	qtwidgets.NewQApplication(os.Args)
}

// ---------------------------------------------------------------- config

func (a *App) reload(c *config.Config, err error) {
	// Problems are shown in every pane's header until the file is fixed;
	// whatever did load is still applied (Load falls back to defaults).
	a.configErr = ""
	if err != nil {
		a.configErr = err.Error()
		fmt.Fprintln(os.Stderr, "termite:", err)
	}
	if c == nil {
		return
	}
	old := a.cfg
	a.cfg = c
	a.bindings = c.ParseBindings()
	a.theme = c.ResolveTheme()
	if old.WindowTheme != c.WindowTheme {
		applyWindowTheme(c.WindowTheme)
	}
	fontChanged := old.FontFamily != c.FontFamily || old.FontSize != c.FontSize || old.LineHeight != c.LineHeight
	if fontChanged {
		a.rebuildFonts()
	}
	// Restyle first: the header font comes from the stylesheet, and
	// layoutHeader elides with it.
	for _, w := range a.windows {
		w.applyTheme()
		w.toolbar.update()
	}
	for _, p := range a.allPanes() {
		a.applyTerminalSettings(p)
		p.layoutHeader()
		p.scrollbar.SetVisible(c.Scrollbar)
		if p.R != nil {
			p.R.Style = a.styleFor(p)
			p.R.Invalidate()
		}
		p.cols = 0 // padding or font may have changed
		p.updateGridSize()
		p.updateBlinkTimer()
		p.invalidate()
	}
}

// applyTerminalSettings gives a pane's terminal the settings it holds
// itself: theme colours, the default cursor shape and clipboard access.
func (a *App) applyTerminalSettings(p *Pane) {
	th := a.theme
	cursor := term.CursorBlock
	switch a.cfg.CursorStyle {
	case "beam":
		cursor = term.CursorBeam
	case "underline":
		cursor = term.CursorUnderline
	}
	p.T.Mu.Lock()
	p.T.SetColors(th.Palette256(), th.Foreground, th.Background, th.Cursor)
	p.T.SetDefaultCursor(cursor)
	p.T.AllowClipboardRead = a.cfg.OSC52 == "read-write"
	p.T.Mu.Unlock()
}

// styleFor returns the renderer settings for a pane (padding depends on
// the pane's screen scale).
func (a *App) styleFor(p *Pane) render.Style {
	th := a.theme
	pad := int(math.Round(float64(a.cfg.Padding) * p.dpr))
	return render.Style{
		SelBg: th.Selection, SelFg: th.SelText, SelFgSet: th.HasSelText, CursorText: th.CursorText,
		BoldIsBright: a.cfg.BoldIsBright, Ligatures: a.cfg.Ligatures,
		CursorBlink: a.cfg.CursorBlink, PadX: pad, PadY: pad,
	}
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

// NewWindow opens a window with one tab (or the given layout). It returns
// nil when no tab could be opened: the problem has been shown, and the
// window is closed again (which quits if it was the only one).
func (a *App) NewWindow(layout *LayoutFile) *Window {
	w := newWindow(a)
	a.windows = append(a.windows, w)
	if layout != nil && len(layout.Tabs) > 0 {
		for _, ln := range layout.Tabs {
			w.addLayoutTab(ln)
		}
		w.tabs.SetCurrentIndex(0) // a layout opens on its first tab
	} else {
		w.NewTab("", a.defaultCmd)
	}
	if len(w.list) == 0 {
		w.w.Close()
		return nil
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
	if a.focused != nil && a.focused.tab.win == w {
		a.focused = nil
	}
	for i, x := range a.windows {
		if x == w {
			a.windows = append(a.windows[:i], a.windows[i+1:]...)
			break
		}
	}
	if len(a.windows) == 0 {
		qtcore.QCoreApplicationQuit()
	}
}

func (a *App) paneFocused(p *Pane) {
	moved := a.focused == nil || a.focused.tab != p.tab
	a.focused = p
	if moved && a.broadcast == BroadcastAll {
		a.refreshBroadcast() // typing reaches another tab now
	}
	p.tab.focused = p
	p.tab.win.updateTitles()
}

func (a *App) paneTitleChanged(p *Pane) { p.tab.win.updateTitles() }

func (a *App) bell(p *Pane) {
	w := p.tab.win.w
	if !w.IsActiveWindow() || a.focused != p {
		qtwidgets.QApplicationAlert(w)
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

// broadcastsTo reports whether typing in a pane reaches p as well as the
// pane typed in, which its header shows with the focused colours.
func (a *App) broadcastsTo(p *Pane) bool {
	switch a.broadcast {
	case BroadcastAll: // the focused pane's tab (inputTargets)
		return a.focused != nil && p.tab == a.focused.tab
	case BroadcastGroup:
		return p.InGroup
	}
	return false
}

// setBroadcast sets the broadcast mode.
func (a *App) setBroadcast(mode int) {
	a.broadcast = mode
	a.refreshBroadcast()
}

// toggleBroadcast turns the broadcast mode on, or off if it is on already.
func (a *App) toggleBroadcast(mode int) {
	if a.broadcast == mode {
		mode = BroadcastOff
	}
	a.setBroadcast(mode)
}

// refreshBroadcast shows the broadcast state in the pane headers and the
// toolbars.
func (a *App) refreshBroadcast() {
	for _, q := range a.allPanes() {
		q.updateHeaderActive()
	}
	for _, w := range a.windows {
		w.toolbar.update()
	}
}

// ---------------------------------------------------------------- actions

func (a *App) runAction(p *Pane, action string) {
	w := p.tab.win
	switch action {
	case "copy":
		if s := p.selectionText(); s != "" {
			qtgui.QGuiApplicationClipboard().SetText(s)
		}
	case "paste":
		p.paste(qtgui.QGuiApplicationClipboard().Text())
	case "new_tab":
		w.NewTab(p.cwd(), a.defaultCmd)
	case "new_window":
		a.NewWindow(nil)
	case "close_pane":
		p.close()
	case "exit":
		// Typed into this pane only, even while input is broadcast.
		p.S.WriteString("exit\r")
		p.scrollToBottom()
	case "split_horizontal":
		a.split(p, qtcore.Vertical)
	case "split_vertical":
		a.split(p, qtcore.Horizontal)
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
		a.toggleBroadcast(BroadcastAll)
	case "broadcast_group":
		a.toggleBroadcast(BroadcastGroup)
	case "group_toggle":
		p.InGroup = !p.InGroup
		a.refreshBroadcast()
	case "save_layout":
		name, ok := qtwidgets.QInputDialogGetTextFull(w.w, "Save layout", "Layout name:", qtwidgets.QLineEditNormal, "", 0, 0)
		if ok && name != "" {
			if err := w.SaveLayout(name); err != nil {
				qtwidgets.QMessageBoxWarning(w.w, desktop.DisplayName, err.Error())
			}
		}
		p.refocus()
	case "load_layout":
		names := ListLayouts()
		if len(names) == 0 {
			qtwidgets.QMessageBoxInformation(w.w, desktop.DisplayName, "No saved layouts in "+layoutDir())
			return
		}
		name, ok := qtwidgets.QInputDialogGetItemFull(w.w, "Load layout", "Layout:", names, 0, false, 0, 0)
		p.refocus()
		if ok {
			lf, err := LoadLayout(name)
			if err != nil {
				qtwidgets.QMessageBoxWarning(w.w, desktop.DisplayName, err.Error())
				return
			}
			w.loadLayout(lf)
		}
	case "context_menu":
		p.contextMenuAtCursor()
	case "clear_scrollback":
		p.T.Mu.Lock()
		p.T.ClearScrollback()
		p.T.Mu.Unlock()
		p.viewTop = -1
		p.view.Matches, p.view.Current = nil, -1
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

func (a *App) split(p *Pane, orient qtcore.Orientation) {
	np, err := a.NewPane(p.tab, p.cwd(), a.defaultCmd)
	if err != nil {
		qtwidgets.QMessageBoxWarning(p.Widget, desktop.DisplayName, err.Error())
		return
	}
	p.tab.Split(p, orient, np)
	// The new pane is where typing goes next, also when the split was
	// chosen from p's context menu, which gives the focus back to the tab's
	// focused pane when it closes.
	p.tab.focused = np
	np.GL.SetFocus()
}
