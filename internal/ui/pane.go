package ui

import (
	"fmt"
	"math"
	"os"
	"strings"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/go-gl/gl/v3.3-core/gl"
	qt "github.com/mappu/miqt/qt6"
	"github.com/mappu/miqt/qt6/mainthread"

	"gotti/internal/desktop"
	"gotti/internal/input"
	"gotti/internal/pty"
	"gotti/internal/qtgl"
	"gotti/internal/render"
	"gotti/internal/term"
)

var glInitialized bool

// Debug timing hooks (GOTTI_DEBUG=startup,latency).
var (
	ProcessStart   = time.Now()
	debugStartup   = strings.Contains(os.Getenv("GOTTI_DEBUG"), "startup")
	debugLatency   = strings.Contains(os.Getenv("GOTTI_DEBUG"), "latency")
	firstFrameDone bool
)

const (
	selChar = iota
	selWord
	selLine
)

// Pane is one terminal: a GL widget, its emulator and child process.
type Pane struct {
	app    *App
	tab    *Tab
	Widget *qt.QWidget // container: search bar + GL view
	GL     *qtgl.Widget
	T      *term.Terminal
	S      *pty.Session
	R      *render.Renderer
	cache  *render.GlyphCache
	dpr    float64

	view          render.View
	viewTop       int64 // -1: follow output
	redrawPending atomic.Bool
	syncTimer     *time.Timer

	// selection
	selecting   bool
	selMode     int
	selAnchor   term.Pos
	selAnchorE  term.Pos
	lastClick   time.Time
	clickCount  int
	lastClickAt term.Pos

	// mouse reporting
	buttonsDown int
	lastMouse   [2]int
	wheelAccum  float64

	// search
	searchBar   *qt.QLineEdit
	searchQuery string

	blinkTimer *qt.QTimer
	InGroup    bool
	Title      string
	Cwd        string
	exited     bool
	cols, rows int
	command    []string
	keyTime    time.Time

	// header above the terminal: current directory
	header        *qt.QLabel
	headerDir     string
	program       string // foreground program, shown in tab and window titles
	headerPending atomic.Bool
}

// NewPane creates a pane and starts its process.
func (a *App) NewPane(tab *Tab, cwd string, command []string) (*Pane, error) {
	p := &Pane{app: a, tab: tab, viewTop: -1, dpr: 1, command: command}
	p.view.Current = -1
	cfg := a.cfg
	p.T = term.New(80, 24, cfg.Scrollback)
	p.T.AllowClipboardRead = cfg.OSC52 == "read-write"
	a.applyThemeTo(p)

	p.Widget = qt.NewQWidget(nil)
	p.Widget.SetObjectName("gottiPane")
	lay := qt.NewQVBoxLayout(p.Widget)
	lay.SetContentsMargins(0, 0, 0, 0)
	lay.SetSpacing(0)
	p.setupHeader()
	lay.AddWidget(p.header.QWidget)
	p.searchBar = qt.NewQLineEdit(p.Widget)
	p.searchBar.SetObjectName("gottiSearch")
	p.searchBar.SetPlaceholderText("Search scrollback  (Enter: older, Shift+Enter: newer, Esc: close)")
	p.searchBar.Hide()
	p.GL = qtgl.New(p.Widget)
	p.GL.SetSizePolicy2(qt.QSizePolicy__Expanding, qt.QSizePolicy__Expanding)
	p.GL.SetMinimumSize2(40, 20)
	lay.AddWidget(p.searchBar.QWidget)
	lay.AddWidget(p.GL.QWidget)
	p.GL.SetCursor(qt.NewQCursor2(qt.IBeamCursor))

	p.GL.OnInit = p.initGL
	p.GL.OnPaint = p.paint
	p.GL.OnResize = func(w, h int) { p.updateGridSize() }
	p.GL.OnEvent = p.event
	p.setupSearch()

	if command == nil {
		command = cfg.Shell
	}
	s, err := pty.Start(p.T, pty.Options{Command: command, Dir: cwd, Cols: 80, Rows: 24})
	if err != nil {
		return nil, err
	}
	p.S = s
	p.Cwd = cwd
	s.OnUpdate = p.onOutput
	s.OnExit = func() { mainthread.Start(p.onExit) }
	p.headerDir = cwd
	p.scheduleHeaderRefresh()
	s.Run()
	return p, nil
}

// ---------------------------------------------------------------- output

// onOutput runs on the reader goroutine after each parsed chunk.
func (p *Pane) onOutput() {
	t := p.T
	t.Mu.Lock()
	ev := t.TakeEvents()
	sync := t.SyncOutput
	since := t.SyncSince
	var title, cwd string
	var clips []term.ClipboardWrite
	if ev&EvUI != 0 {
		title, cwd = t.Title, t.Cwd
		clips = t.ClipboardWrites
		t.ClipboardWrites = nil
	}
	t.Mu.Unlock()
	if ev&EvUI != 0 {
		mainthread.Start(func() { p.handleEvents(ev, title, cwd, clips) })
	}
	p.scheduleHeaderRefresh()
	// Synchronized output: hold redraws until the update completes (or
	// a safety timeout passes).
	if sync && ev&term.EvSync == 0 {
		const timeout = 150 * time.Millisecond
		if wait := timeout - time.Since(since); wait > 0 {
			if p.syncTimer == nil {
				p.syncTimer = time.AfterFunc(wait, p.requestRedraw)
			}
			return
		}
	}
	if p.syncTimer != nil {
		p.syncTimer.Stop()
		p.syncTimer = nil
	}
	p.requestRedraw()
}

// EvUI are the events that need main-thread handling.
const EvUI = term.EvTitle | term.EvBell | term.EvClipboard | term.EvClipboardQuery | term.EvCwd | term.EvMouseMode | term.EvCursorStyle

// requestRedraw schedules a repaint; repaints are coalesced and Qt limits
// them to one per vsync, so a flood of output never renders per byte.
func (p *Pane) requestRedraw() {
	if p.redrawPending.Swap(true) {
		return
	}
	mainthread.Start(func() {
		if !p.exited {
			p.GL.Update()
		}
	})
}

func (p *Pane) handleEvents(ev term.Event, title, cwd string, clips []term.ClipboardWrite) {
	if p.exited {
		return
	}
	if ev&term.EvTitle != 0 {
		p.Title = title
		p.app.paneTitleChanged(p)
	}
	if ev&term.EvCwd != 0 {
		p.Cwd = cwd
		p.refreshHeader()
	}
	if ev&term.EvBell != 0 {
		p.app.bell(p)
	}
	if p.app.cfg.OSC52 != "off" {
		cb := qt.QGuiApplication_Clipboard()
		for _, c := range clips {
			if c.Primary {
				cb.SetText2(c.Text, qt.QClipboard__Selection)
			} else {
				cb.SetText(c.Text)
			}
		}
	}
	if ev&term.EvClipboardQuery != 0 {
		text := qt.QGuiApplication_Clipboard().Text()
		p.T.Mu.Lock()
		p.T.ReplyClipboard(text)
		r := p.T.TakeReplies()
		p.T.Mu.Unlock()
		p.S.Write(r)
	}
	if ev&term.EvMouseMode != 0 {
		p.updatePointer(false)
	}
	if ev&term.EvCursorStyle != 0 {
		p.updateBlinkTimer()
	}
}

func (p *Pane) onExit() {
	if p.exited {
		return
	}
	p.exited = true
	if p.blinkTimer != nil {
		p.blinkTimer.Stop()
	}
	p.app.closePane(p)
}

// ---------------------------------------------------------------- GL

func (p *Pane) initGL() {
	if !glInitialized {
		if err := gl.InitWithProcAddrFunc(qtgl.ProcAddress); err != nil {
			fmt.Fprintln(os.Stderr, "gotti: OpenGL init failed:", err)
			os.Exit(1)
		}
		glInitialized = true
	}
	p.dpr = p.GL.DevicePixelRatioF()
	p.cache = p.app.cacheFor(p.dpr)
	if p.R == nil {
		p.R = render.NewRenderer(p.cache)
	} else {
		p.R.Cache = p.cache
	}
	p.R.Style = p.app.styleFor(p)
	if err := p.R.InitGL(); err != nil {
		fmt.Fprintln(os.Stderr, "gotti:", err)
		os.Exit(1)
	}
	p.updateGridSize()
}

func (p *Pane) fbSize() (int, int) {
	return int(math.Round(float64(p.GL.Width()) * p.dpr)), int(math.Round(float64(p.GL.Height()) * p.dpr))
}

// updateGridSize resizes the terminal to fit the widget.
func (p *Pane) updateGridSize() {
	if p.R == nil {
		return
	}
	fw, fh := p.fbSize()
	p.R.SetFramebufferSize(fw, fh)
	cols, rows := p.R.GridSize(fw, fh)
	cw, ch := p.R.CellSize()
	if cols == p.cols && rows == p.rows {
		return
	}
	p.cols, p.rows = cols, rows
	p.T.Mu.Lock()
	p.T.Resize(cols, rows)
	p.T.CellW, p.T.CellH = int(float64(cw)/p.dpr), int(float64(ch)/p.dpr)
	p.T.Mu.Unlock()
	p.S.Resize(cols, rows, cols*cw, rows*ch)
	p.GL.Update()
}

func (p *Pane) paint() {
	p.redrawPending.Store(false)
	if p.R == nil {
		return
	}
	if dpr := p.GL.DevicePixelRatioF(); dpr != p.dpr {
		// moved to a screen with a different scale factor
		p.dpr = dpr
		p.cache = p.app.cacheFor(dpr)
		p.R.Cache = p.cache
		p.R.Style = p.app.styleFor(p)
		p.R.Invalidate()
		p.cols = 0
		p.updateGridSize()
	}
	fw, fh := p.fbSize()
	p.R.SetFramebufferSize(fw, fh)
	p.view.TopAbs = p.viewTop
	p.view.Border = p.app.borderFor(p)
	p.T.Mu.Lock()
	p.R.Prepare(p.T, &p.view)
	cx, cy := p.T.CursorPos()
	p.T.Mu.Unlock()
	p.view.Dirty, p.view.CursorDirty = false, false
	p.R.Render()
	if debugStartup && !firstFrameDone {
		firstFrameDone = true
		fmt.Fprintf(os.Stderr, "gotti: first frame after %v\n", time.Since(ProcessStart))
	}
	if debugLatency && !p.keyTime.IsZero() {
		gl.Finish()
		fmt.Fprintf(os.Stderr, "gotti: key-to-frame %v\n", time.Since(p.keyTime))
		p.keyTime = time.Time{}
	}
	// tell input methods where the cursor is (logical pixels)
	cw, ch := p.R.CellSize()
	st := p.R.Style
	p.GL.SetIMRect(int(float64(st.PadX+cx*cw)/p.dpr), int(float64(st.PadY+cy*ch)/p.dpr), int(float64(cw)/p.dpr), int(float64(ch)/p.dpr))
}

// invalidate redraws everything (selection, search, theme changes).
func (p *Pane) invalidate() {
	p.view.Dirty = true
	p.GL.Update()
}

// ---------------------------------------------------------------- cursor blink

func (p *Pane) updateBlinkTimer() {
	p.T.Mu.Lock()
	blink := p.T.CursorBlink
	p.T.Mu.Unlock()
	want := blink && p.app.cfg.CursorBlink && p.view.Focused
	if want {
		if p.blinkTimer == nil {
			p.blinkTimer = qt.NewQTimer2(p.Widget.QObject)
			p.blinkTimer.OnTimeout(func() {
				p.view.BlinkOn = !p.view.BlinkOn
				p.view.CursorDirty = true
				p.GL.Update()
			})
		}
		if !p.blinkTimer.IsActive() {
			p.blinkTimer.Start(530)
		}
	} else if p.blinkTimer != nil && p.blinkTimer.IsActive() {
		p.blinkTimer.Stop()
		p.view.BlinkOn = true
		p.view.CursorDirty = true
		p.GL.Update()
	}
}

// resetBlink makes the cursor visible immediately after typing.
func (p *Pane) resetBlink() {
	if p.blinkTimer != nil && p.blinkTimer.IsActive() {
		p.view.BlinkOn = true
		p.view.CursorDirty = true
		p.blinkTimer.Start(530)
	}
}

// ---------------------------------------------------------------- events

func (p *Pane) event(ev *qt.QEvent, raw unsafe.Pointer) bool {
	switch ev.Type() {
	case qt.QEvent__ShortcutOverride:
		return true // every key goes to the terminal or our bindings
	case qt.QEvent__KeyPress:
		return p.keyPress(qt.UnsafeNewQKeyEvent(raw), false)
	case qt.QEvent__KeyRelease:
		return p.keyPress(qt.UnsafeNewQKeyEvent(raw), true)
	case qt.QEvent__InputMethod:
		p.inputMethod(qt.UnsafeNewQInputMethodEvent(raw))
		return true
	case qt.QEvent__MouseButtonPress, qt.QEvent__MouseButtonDblClick:
		p.mousePress(qt.UnsafeNewQMouseEvent(raw))
		return true
	case qt.QEvent__MouseButtonRelease:
		p.mouseRelease(qt.UnsafeNewQMouseEvent(raw))
		return true
	case qt.QEvent__MouseMove:
		p.mouseMove(qt.UnsafeNewQMouseEvent(raw))
		return true
	case qt.QEvent__Wheel:
		p.wheel(qt.UnsafeNewQWheelEvent(raw))
		return true
	case qt.QEvent__FocusIn:
		p.focus(true)
		return false
	case qt.QEvent__FocusOut:
		p.focus(false)
		return false
	}
	return false
}

func (p *Pane) focus(in bool) {
	p.view.Focused = in
	p.view.BlinkOn = true
	p.view.CursorDirty = true
	p.T.Mu.Lock()
	report := p.T.FocusEvents
	p.T.Mu.Unlock()
	if report {
		if in {
			p.S.WriteString("\x1b[I")
		} else {
			p.S.WriteString("\x1b[O")
		}
	}
	if in {
		p.app.paneFocused(p)
	}
	p.setHeaderFocused(in)
	p.updateBlinkTimer()
	p.GL.Update()
}

func (p *Pane) modes() input.Modes {
	p.T.Mu.Lock()
	defer p.T.Mu.Unlock()
	return input.Modes{AppCursor: p.T.AppCursor, AppKeypad: p.T.AppKeypad, KittyFlags: p.T.KittyFlags()}
}

func (p *Pane) keyPress(qe *qt.QKeyEvent, release bool) bool {
	ev := translateKey(qe, release)
	if chord, ok := chordOf(ev); ok {
		if action, ok := p.app.bindings[chord]; ok {
			if !release {
				p.app.runAction(p, action)
			}
			return true
		}
	}
	data := input.Encode(ev, p.modes())
	if len(data) == 0 {
		return true
	}
	if debugLatency {
		p.keyTime = time.Now()
	}
	p.sendInput(data)
	return true
}

// sendInput writes user input to this pane and any broadcast targets.
func (p *Pane) sendInput(data []byte) {
	for _, t := range p.app.inputTargets(p) {
		t.S.Write(data)
		t.scrollToBottom()
		t.clearSelection()
	}
	p.resetBlink()
}

func (p *Pane) inputMethod(e *qt.QInputMethodEvent) {
	if s := e.CommitString(); s != "" {
		p.sendInput([]byte(s))
	}
	pre := e.PreeditString()
	if pre != p.view.Preedit {
		p.view.Preedit = pre
		p.GL.Update()
	}
}

// ---------------------------------------------------------------- mouse

// cellAt converts widget coordinates to a view cell (clamped).
func (p *Pane) cellAt(x, y float64) (col, row int, inside bool) {
	cw, ch := p.R.CellSize()
	st := p.R.Style
	fx := (x*p.dpr - float64(st.PadX)) / float64(cw)
	fy := (y*p.dpr - float64(st.PadY)) / float64(ch)
	col, row = int(math.Floor(fx)), int(math.Floor(fy))
	inside = col >= 0 && row >= 0 && col < p.cols && row < p.rows
	col = max(0, min(col, p.cols-1))
	row = max(0, min(row, p.rows-1))
	return
}

func (p *Pane) topAbs() int64 {
	if p.viewTop >= 0 {
		return p.viewTop
	}
	p.T.Mu.Lock()
	defer p.T.Mu.Unlock()
	return p.T.Buf().ScreenTopAbs()
}

func (p *Pane) mouseReporting(mods qt.KeyboardModifier) bool {
	if mods&qt.ShiftModifier != 0 {
		return false // shift overrides application mouse mode
	}
	p.T.Mu.Lock()
	defer p.T.Mu.Unlock()
	return p.T.MouseMode != term.MouseOff
}

func mouseButton(b qt.MouseButton) int {
	switch b {
	case qt.LeftButton:
		return input.MouseLeft
	case qt.MiddleButton:
		return input.MouseMiddle
	case qt.RightButton:
		return input.MouseRight
	}
	return -1
}

func (p *Pane) mousePress(e *qt.QMouseEvent) {
	pos := e.Position()
	col, row, _ := p.cellAt(pos.X(), pos.Y())
	mods := e.Modifiers()
	btn := mouseButton(e.Button())
	p.GL.SetFocus()
	if btn < 0 {
		return
	}
	if p.mouseReporting(mods) {
		p.buttonsDown |= 1 << btn
		p.S.Write(input.EncodeMouse(btn, input.MousePress, col, row, qtMods(mods)))
		p.lastMouse = [2]int{col, row}
		return
	}
	abs := p.topAbs() + int64(row)
	at := term.Pos{Line: abs, Col: col}
	switch btn {
	case input.MouseLeft:
		if mods&qt.ControlModifier != 0 {
			if url := p.linkAt(at); url != "" {
				qt.QDesktopServices_OpenUrl(qt.NewQUrl3(url))
				return
			}
		}
		now := time.Now()
		if now.Sub(p.lastClick) < 400*time.Millisecond && p.lastClickAt == at {
			p.clickCount = p.clickCount%3 + 1
		} else {
			p.clickCount = 1
		}
		p.lastClick, p.lastClickAt = now, at
		p.selecting = true
		p.selMode = []int{selChar, selChar, selWord, selLine}[p.clickCount]
		p.view.Sel = render.Selection{Block: mods&qt.AltModifier != 0}
		p.selAnchor, p.selAnchorE = at, term.Pos{Line: abs, Col: col + 1}
		if p.selMode != selChar {
			p.T.Mu.Lock()
			b := p.T.Buf()
			if p.selMode == selWord {
				p.selAnchor, p.selAnchorE = b.WordAt(at, p.app.cfg.WordChars)
			} else {
				p.selAnchor, p.selAnchorE = b.LineExtent(at)
			}
			p.T.Mu.Unlock()
			p.view.Sel = render.Selection{Active: true, Start: p.selAnchor, End: p.selAnchorE}
		}
		p.invalidate()
	case input.MouseMiddle:
		text := qt.QGuiApplication_Clipboard().TextWithMode(qt.QClipboard__Selection)
		p.paste(text)
	case input.MouseRight:
		g := e.GlobalPosition()
		p.showContextMenu(int(g.X()), int(g.Y()), &at)
	}
}

func (p *Pane) mouseMove(e *qt.QMouseEvent) {
	pos := e.Position()
	col, row, _ := p.cellAt(pos.X(), pos.Y())
	mods := e.Modifiers()
	if p.mouseReporting(mods) {
		p.T.Mu.Lock()
		mode := p.T.MouseMode
		p.T.Mu.Unlock()
		if [2]int{col, row} == p.lastMouse {
			return
		}
		p.lastMouse = [2]int{col, row}
		switch {
		case p.buttonsDown != 0 && mode >= term.MouseButton:
			btn := input.MouseLeft
			for b := 0; b < 3; b++ {
				if p.buttonsDown&(1<<b) != 0 {
					btn = b
					break
				}
			}
			p.S.Write(input.EncodeMouse(btn, input.MouseMotion, col, row, qtMods(mods)))
		case p.buttonsDown == 0 && mode == term.MouseAny:
			p.S.Write(input.EncodeMouse(input.MouseNone, input.MouseMotion, col, row, qtMods(mods)))
		}
		return
	}
	// hover links with Ctrl held
	if mods&qt.ControlModifier != 0 {
		p.updateHover(term.Pos{Line: p.topAbs() + int64(row), Col: col})
	} else if p.view.HoverActive {
		p.view.HoverActive = false
		p.updatePointer(false)
		p.invalidate()
	}
	if !p.selecting {
		return
	}
	// auto-scroll while dragging outside the view
	if pos.Y() < 0 {
		p.scrollBy(-1)
	} else if pos.Y() > float64(p.GL.Height()) {
		p.scrollBy(1)
	}
	at := term.Pos{Line: p.topAbs() + int64(row), Col: col}
	p.extendSelection(at, e.Modifiers()&qt.AltModifier != 0)
}

func (p *Pane) extendSelection(at term.Pos, block bool) {
	start, end := p.selAnchor, p.selAnchorE
	p.T.Mu.Lock()
	b := p.T.Buf()
	switch p.selMode {
	case selChar:
		if at.Less(p.selAnchor) {
			start, end = at, p.selAnchorE
			if block {
				start, end = at, term.Pos{Line: p.selAnchor.Line, Col: p.selAnchor.Col + 1}
			}
		} else {
			start, end = p.selAnchor, term.Pos{Line: at.Line, Col: at.Col + 1}
		}
		if block && at.Col < p.selAnchor.Col {
			start.Col, end.Col = at.Col, p.selAnchor.Col+1
		}
	case selWord:
		ws, we := b.WordAt(at, p.app.cfg.WordChars)
		if ws.Less(p.selAnchor) {
			start = ws
		}
		if p.selAnchorE.Less(we) {
			end = we
		}
	case selLine:
		ls, le := b.LineExtent(at)
		if ls.Less(p.selAnchor) {
			start = ls
		}
		if p.selAnchorE.Less(le) {
			end = le
		}
	}
	p.T.Mu.Unlock()
	sel := render.Selection{Active: true, Start: start, End: end, Block: block}
	if sel != p.view.Sel {
		p.view.Sel = sel
		p.invalidate()
	}
}

func (p *Pane) mouseRelease(e *qt.QMouseEvent) {
	pos := e.Position()
	col, row, _ := p.cellAt(pos.X(), pos.Y())
	btn := mouseButton(e.Button())
	if btn >= 0 && p.buttonsDown&(1<<btn) != 0 {
		p.buttonsDown &^= 1 << btn
		p.S.Write(input.EncodeMouse(btn, input.MouseRelease, col, row, qtMods(e.Modifiers())))
		return
	}
	if btn == input.MouseLeft && p.selecting {
		p.selecting = false
		if p.view.Sel.Active {
			text := p.selectionText()
			if text != "" {
				cb := qt.QGuiApplication_Clipboard()
				cb.SetText2(text, qt.QClipboard__Selection)
				if p.app.cfg.CopyOnSelect {
					cb.SetText(text)
				}
			}
		}
	}
}

func (p *Pane) selectionText() string {
	s := p.view.Sel
	if !s.Active {
		return ""
	}
	p.T.Mu.Lock()
	defer p.T.Mu.Unlock()
	return p.T.Buf().TextRange(s.Start, s.End, s.Block)
}

func (p *Pane) clearSelection() {
	if p.view.Sel.Active {
		p.view.Sel = render.Selection{}
		p.invalidate()
	}
}

func (p *Pane) wheel(e *qt.QWheelEvent) {
	pos := e.Position()
	col, row, _ := p.cellAt(pos.X(), pos.Y())
	_, ch := p.R.CellSize()
	var lines float64
	if e.HasPixelDelta() && e.PixelDelta().Y() != 0 {
		p.wheelAccum += float64(e.PixelDelta().Y()) * p.dpr / float64(ch)
	} else {
		p.wheelAccum += float64(e.AngleDelta().Y()) / 120 * 3
	}
	lines = math.Trunc(p.wheelAccum)
	p.wheelAccum -= lines
	if lines == 0 {
		return
	}
	n := int(math.Abs(lines))
	up := lines > 0
	mods := e.Modifiers()
	p.T.Mu.Lock()
	reporting := p.T.MouseMode != term.MouseOff
	alt := p.T.AltScreen()
	altScroll := p.T.AltScroll
	appCursor := p.T.AppCursor
	p.T.Mu.Unlock()
	switch {
	case reporting && mods&qt.ShiftModifier == 0:
		btn := input.MouseWheelDown
		if up {
			btn = input.MouseWheelUp
		}
		for i := 0; i < max(1, n/3); i++ {
			p.S.Write(input.EncodeMouse(btn, input.MousePress, col, row, qtMods(mods)))
		}
	case alt && altScroll:
		k := "B"
		if up {
			k = "A"
		}
		seq := "\x1b[" + k
		if appCursor {
			seq = "\x1bO" + k
		}
		p.S.WriteString(strings.Repeat(seq, n))
	default:
		if up {
			p.scrollBy(-n)
		} else {
			p.scrollBy(n)
		}
	}
}

func (p *Pane) updatePointer(link bool) {
	shape := qt.IBeamCursor
	if link {
		shape = qt.PointingHandCursor
	} else if p.mouseReporting(0) {
		shape = qt.ArrowCursor
	}
	p.GL.SetCursor(qt.NewQCursor2(shape))
}

func (p *Pane) linkAt(at term.Pos) string {
	p.T.Mu.Lock()
	defer p.T.Mu.Unlock()
	b := p.T.Buf()
	if l, ok := b.LinkAt(at); ok {
		return l.URI
	}
	if u, _, ok := b.URLAt(at); ok {
		return u
	}
	return ""
}

func (p *Pane) updateHover(at term.Pos) {
	p.T.Mu.Lock()
	b := p.T.Buf()
	var m term.Match
	ok := false
	if _, isLink := b.LinkAt(at); isLink {
		// extend over neighbouring cells with the same link
		line := b.LogicalLine(b.AbsToLogical(at.Line))
		id := line[at.Col].Extra
		s, e := at.Col, at.Col
		for s > 0 && line[s-1].Extra == id {
			s--
		}
		for e+1 < len(line) && line[e+1].Extra == id {
			e++
		}
		m, ok = term.Match{Start: term.Pos{Line: at.Line, Col: s}, End: term.Pos{Line: at.Line, Col: e + 1}}, true
	} else if _, um, found := b.URLAt(at); found {
		m, ok = um, true
	}
	p.T.Mu.Unlock()
	if ok != p.view.HoverActive || m != p.view.Hover {
		p.view.HoverActive, p.view.Hover = ok, m
		p.updatePointer(ok)
		p.invalidate()
	}
}

// ---------------------------------------------------------------- scrolling

func (p *Pane) scrollBy(n int) {
	p.T.Mu.Lock()
	b := p.T.Buf()
	bottom := b.ScreenTopAbs()
	minTop := b.Evicted
	p.T.Mu.Unlock()
	top := p.viewTop
	if top < 0 {
		top = bottom
	}
	top += int64(n)
	top = max(minTop, min(top, bottom))
	if top == bottom {
		top = -1
	}
	if top != p.viewTop {
		p.viewTop = top
		p.GL.Update()
	}
}

func (p *Pane) scrollToBottom() {
	if p.viewTop != -1 {
		p.viewTop = -1
		p.GL.Update()
	}
}

func (p *Pane) scrollToTop() {
	p.T.Mu.Lock()
	top := p.T.Buf().Evicted
	p.T.Mu.Unlock()
	p.viewTop = top
	p.GL.Update()
}

// ---------------------------------------------------------------- paste

func (p *Pane) paste(text string) {
	if text == "" {
		return
	}
	p.T.Mu.Lock()
	bracketed := p.T.BracketedPaste
	p.T.Mu.Unlock()
	warn := p.app.cfg.PasteWarning
	if input.IsMultiLine(text) && (warn == "always" || warn == "unbracketed" && !bracketed) {
		lines := strings.Count(strings.TrimRight(text, "\n"), "\n") + 1
		msg := fmt.Sprintf("You are pasting %d lines. They may run as commands.\n\nPaste anyway?", lines)
		ok := qt.QMessageBox_Question(p.Widget, "Multi-line Paste", msg) == qt.QMessageBox__Yes
		p.refocus()
		if !ok {
			return
		}
	}
	p.sendInput(input.Paste(text, bracketed))
}

// ---------------------------------------------------------------- search

func (p *Pane) setupSearch() {
	sb := p.searchBar
	sb.OnTextChanged(func(s string) {
		p.searchQuery = s
		p.runSearch(true)
	})
	sb.OnKeyPressEvent(func(super func(*qt.QKeyEvent), ev *qt.QKeyEvent) {
		switch qt.Key(ev.Key()) {
		case qt.Key_Escape:
			p.closeSearch()
		case qt.Key_Return, qt.Key_Enter:
			if ev.Modifiers()&qt.ShiftModifier != 0 {
				p.searchStep(1)
			} else {
				p.searchStep(-1)
			}
		default:
			super(ev)
		}
	})
}

func (p *Pane) openSearch() {
	p.searchBar.Show()
	p.searchBar.SetFocus()
	p.searchBar.SelectAll()
	if p.searchQuery != "" {
		p.runSearch(true)
	}
}

func (p *Pane) closeSearch() {
	p.searchBar.Hide()
	p.view.Matches = nil
	p.view.Current = -1
	p.invalidate()
	p.GL.SetFocus()
}

func (p *Pane) runSearch(jump bool) {
	q := p.searchQuery
	caseSensitive := strings.ToLower(q) != q // smart case
	p.T.Mu.Lock()
	matches := p.T.Buf().Search(q, caseSensitive)
	p.T.Mu.Unlock()
	p.view.Matches = matches
	p.view.Current = len(matches) - 1
	if len(matches) == 0 {
		p.view.Current = -1
	}
	p.searchBar.SetProperty("notFound", qt.NewQVariant8(len(matches) == 0 && q != ""))
	p.searchBar.Style().Unpolish(p.searchBar.QWidget)
	p.searchBar.Style().Polish(p.searchBar.QWidget)
	if jump {
		p.revealMatch()
	}
	p.invalidate()
}

func (p *Pane) searchStep(dir int) {
	n := len(p.view.Matches)
	if n == 0 {
		return
	}
	p.view.Current = (p.view.Current + dir + n) % n
	p.revealMatch()
	p.invalidate()
}

func (p *Pane) revealMatch() {
	if p.view.Current < 0 {
		return
	}
	m := p.view.Matches[p.view.Current]
	top := p.topAbs()
	if m.Start.Line >= top && m.Start.Line < top+int64(p.rows) {
		return
	}
	target := m.Start.Line - int64(p.rows/2)
	p.T.Mu.Lock()
	b := p.T.Buf()
	bottom := b.ScreenTopAbs()
	target = max(b.Evicted, min(target, bottom))
	p.T.Mu.Unlock()
	if target == bottom {
		target = -1
	}
	p.viewTop = target
}

// ---------------------------------------------------------------- misc

// cwd returns the best known working directory of the pane.
func (p *Pane) cwd() string {
	if d := p.S.ForegroundCwd(); d != "" {
		return d
	}
	return p.Cwd
}

// displayTitle returns the title for tabs and the window.
// It is the foreground program's name: titles set by programs (which
// shells often fill with user@host:path) are kept off the title bar; the
// directory is shown in the pane's header instead.
func (p *Pane) displayTitle() string {
	if p.program == "" {
		p.program = p.S.ForegroundName()
	}
	if p.program != "" {
		return p.program
	}
	return desktop.DisplayName
}

// refocus returns keyboard focus to the pane after a dialog.
func (p *Pane) refocus() {
	p.GL.ActivateWindow()
	p.GL.SetFocus()
}

func (p *Pane) close() {
	p.S.Close()
}
