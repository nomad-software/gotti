package term

import (
	"strconv"
	"sync"
	"time"
)

// Event flags raised by the terminal for the UI layer.
type Event uint32

const (
	EvTitle Event = 1 << iota
	EvBell
	EvClipboard // ClipboardWrites has entries
	EvClipboardQuery
	EvCwd
	EvAltScreen
	EvMouseMode
	EvCursorStyle
	EvPalette
	EvSync // synchronized output ended
)

// Cursor styles (DECSCUSR).
const (
	CursorBlock = iota
	CursorUnderline
	CursorBeam
)

const (
	csASCII byte = iota
	csDECGraphics
)

// Mouse tracking modes.
const (
	MouseOff    = iota
	MouseClick  // ?1000
	MouseButton // ?1002
	MouseAny    // ?1003
)

type cursor struct {
	X, Y        int
	pen         Cell // Fg, Bg, Attr (style bits), Extra
	wrapPending bool
	origin      bool
	charsets    [4]byte
	gl          int
}

// ClipboardWrite is an OSC 52 clipboard request.
type ClipboardWrite struct {
	Primary bool // 'p'/'s' selection rather than 'c' clipboard
	Text    string
}

// Terminal is a complete virtual terminal. All exported methods that
// touch screen state require the caller to hold Mu, except where noted.
type Terminal struct {
	Mu sync.Mutex
	parser

	cols, rows int
	scrollback int

	primary, alt *Buffer
	buf          *Buffer
	altActive    bool

	cur       cursor
	savedMain cursor
	savedAlt  cursor
	top, bot  int // scroll region, inclusive
	tabs      []bool
	ulColor   Color
	link      uint32
	lastPrint struct {
		x, y   int
		ok, ri bool // ri: the cell holds a lone regional indicator
	}
	lastChar rune

	// modes
	AppCursor      bool
	AppKeypad      bool
	Autowrap       bool
	Insert         bool
	NewlineMode    bool // LNM
	ReverseVideo   bool
	CursorVisible  bool
	CursorStyle    int  // shape to draw: the program's choice, else defaultCursor
	CursorBlink    bool // the program asked for a blinking cursor
	defaultCursor  int  // the user's preferred shape (cursor_style)
	BracketedPaste bool
	MouseMode      int
	MouseSGR       bool
	FocusEvents    bool
	AltScroll      bool // ?1007
	SyncOutput     bool // ?2026
	SyncSince      time.Time
	kittyMain      []int
	kittyAlt       []int

	// state for the UI
	Title              string
	titleStack         []string
	Cwd                string
	ClipboardWrites    []ClipboardWrite
	ClipboardQuery     bool // OSC 52 read request pending (target clipboard)
	AllowClipboardRead bool
	events             Event

	// colours (RGB 0xRRGGBB); set by the UI from the theme, modified by OSC
	Palette                    [256]uint32
	basePalette                [256]uint32
	DefaultFg                  uint32
	DefaultBg                  uint32
	CursorColor                uint32
	baseFg, baseBg, baseCursor uint32

	// pixel geometry for CSI t reports, set by the UI
	CellW, CellH int

	replies []byte
	oscBEL  bool
}

// New creates a terminal of the given size.
func New(cols, rows, scrollback int) *Terminal {
	t := &Terminal{cols: cols, rows: rows, scrollback: scrollback}
	t.primary = NewBuffer(cols, rows, scrollback)
	t.alt = NewBuffer(cols, rows, 0)
	t.buf = t.primary
	t.parser.cur = -1
	t.SetColors(defaultPalette(), 0xd0d0d0, 0x101010, 0xd0d0d0)
	t.reset()
	return t
}

func (t *Terminal) reset() {
	t.cur = cursor{}
	t.savedMain = cursor{}
	t.savedAlt = cursor{}
	t.top, t.bot = 0, t.rows-1
	t.resetTabs()
	t.ulColor, t.link = 0, 0
	t.AppCursor, t.AppKeypad = false, false
	t.Autowrap = true
	t.Insert, t.NewlineMode, t.ReverseVideo = false, false, false
	t.CursorVisible = true
	t.CursorStyle, t.CursorBlink = t.defaultCursor, false
	t.BracketedPaste = false
	t.MouseMode, t.MouseSGR, t.FocusEvents = MouseOff, false, false
	t.AltScroll, t.SyncOutput = false, false
	t.kittyMain, t.kittyAlt = nil, nil
	t.Palette = t.basePalette
	t.DefaultFg, t.DefaultBg, t.CursorColor = t.baseFg, t.baseBg, t.baseCursor
	t.lastPrint.ok = false
}

func (t *Terminal) resetTabs() {
	t.tabs = make([]bool, t.cols)
	for i := 8; i < t.cols; i += 8 {
		t.tabs[i] = true
	}
}

// SetDefaultCursor sets the cursor shape used until a program chooses one
// with DECSCUSR, and again after it resets it (CSI 0 SP q).
func (t *Terminal) SetDefaultCursor(style int) {
	if t.CursorStyle == t.defaultCursor {
		t.CursorStyle = style // the program hasn't chosen: follow the setting
	}
	t.defaultCursor = style
}

// SetColors installs the theme colours (resetting OSC overrides).
func (t *Terminal) SetColors(pal [256]uint32, fg, bg, cursor uint32) {
	t.basePalette = pal
	t.Palette = pal
	t.baseFg, t.baseBg, t.baseCursor = fg, bg, cursor
	t.DefaultFg, t.DefaultBg, t.CursorColor = fg, bg, cursor
	t.primary.MarkAllDirty()
	t.alt.MarkAllDirty()
}

// Size returns the grid size.
func (t *Terminal) Size() (cols, rows int) { return t.cols, t.rows }

// Buf returns the active buffer.
func (t *Terminal) Buf() *Buffer { return t.buf }

// AltScreen reports whether the alternate screen is active.
func (t *Terminal) AltScreen() bool { return t.altActive }

// CursorPos returns the cursor position.
func (t *Terminal) CursorPos() (x, y int) { return t.cur.X, t.cur.Y }

// TakeEvents returns and clears pending UI events.
func (t *Terminal) TakeEvents() Event {
	e := t.events
	t.events = 0
	return e
}

// TakeReplies returns bytes to be written back to the PTY.
func (t *Terminal) TakeReplies() []byte {
	if len(t.replies) == 0 {
		return nil
	}
	r := make([]byte, len(t.replies))
	copy(r, t.replies)
	t.replies = t.replies[:0]
	return r
}

func (t *Terminal) reply(s string) { t.replies = append(t.replies, s...) }

// KittyFlags returns the active kitty keyboard protocol flags.
func (t *Terminal) KittyFlags() int {
	st := t.kittyMain
	if t.altActive {
		st = t.kittyAlt
	}
	if len(st) == 0 {
		return 0
	}
	return st[len(st)-1]
}

func (t *Terminal) kittyStack() *[]int {
	if t.altActive {
		return &t.kittyAlt
	}
	return &t.kittyMain
}

// Resize changes the grid size, reflowing the primary screen.
func (t *Terminal) Resize(cols, rows int) {
	if cols < 2 {
		cols = 2
	}
	if rows < 1 {
		rows = 1
	}
	if cols == t.cols && rows == t.rows {
		return
	}
	// Reflow the primary buffer, tracking the cursor that belongs to it.
	pcx, pcy := t.cur.X, t.cur.Y
	if t.altActive {
		pcx, pcy = t.savedMain.X, t.savedMain.Y
	}
	np, ncx, ncy := t.primary.Resize(cols, rows, true, pcx, pcy)
	na, acx, acy := t.alt.Resize(cols, rows, false, t.cur.X, t.cur.Y)
	t.primary, t.alt = np, na
	if t.altActive {
		t.buf = na
		t.cur.X, t.cur.Y = acx, acy
		t.savedMain.X, t.savedMain.Y = ncx, ncy
	} else {
		t.buf = np
		t.cur.X, t.cur.Y = ncx, ncy
	}
	t.cols, t.rows = cols, rows
	t.top, t.bot = 0, rows-1
	oldTabs := t.tabs
	t.resetTabs()
	copy(t.tabs, oldTabs)
	t.cur.wrapPending = false
	clampSaved := func(c *cursor) {
		c.X = min(c.X, cols-1)
		c.Y = min(c.Y, rows-1)
	}
	clampSaved(&t.savedAlt)
	clampSaved(&t.savedMain)
	t.lastPrint.ok = false
}

// fill returns the blank cell used for erasing (background colour erase).
func (t *Terminal) fill() Cell { return Cell{Bg: t.cur.pen.Bg} }

// ---------------------------------------------------------------- printing

func (t *Terminal) printASCII(s []byte) {
	if t.Insert || t.cur.charsets[t.cur.gl] != csASCII || !t.Autowrap {
		for _, b := range s {
			t.printRune(rune(b))
		}
		return
	}
	pen := t.cur.pen
	b := t.buf
	cols := t.cols
	for len(s) > 0 {
		if t.cur.wrapPending {
			t.wrapLine()
		}
		x := t.cur.X
		n := min(len(s), cols-x)
		line := b.Line(t.cur.Y)
		// Fix up wide characters cut by the run's edges.
		if line[x].Attr&AttrWideCont != 0 && x > 0 {
			line[x-1] = Cell{Fg: line[x-1].Fg, Bg: line[x-1].Bg}
		}
		if e := x + n - 1; line[e].Attr&AttrWide != 0 && e+1 < cols {
			line[e+1] = Cell{Fg: line[e+1].Fg, Bg: line[e+1].Bg}
		}
		src := s[:n]
		dst := line[x : x+n]
		dst = dst[:len(src)]
		tpl := Cell{Fg: pen.Fg, Bg: pen.Bg, Attr: pen.Attr, Extra: pen.Extra}
		for i, c := range src {
			tpl.Ch = rune(c)
			dst[i] = tpl
		}
		b.dirty[t.cur.Y] = true
		t.lastChar = rune(s[n-1])
		x += n
		s = s[n:]
		if x >= cols {
			t.cur.X = cols - 1
			t.cur.wrapPending = true
			t.lastPrint.x = cols - 1
		} else {
			t.cur.X = x
			t.lastPrint.x = x - 1
		}
	}
	t.lastPrint.y = t.cur.Y
	t.lastPrint.ok, t.lastPrint.ri = true, false
}

func (t *Terminal) wrapLine() {
	t.buf.SetWrapped(t.cur.Y, true)
	t.cur.wrapPending = false
	t.cur.X = 0
	t.index()
}

var decGraphics = [...]rune{
	0x00a0, 0x25c6, 0x2592, 0x2409, 0x240c, 0x240d, 0x240a, 0x00b0,
	0x00b1, 0x2424, 0x240b, 0x2518, 0x2510, 0x250c, 0x2514, 0x253c,
	0x23ba, 0x23bb, 0x2500, 0x23bc, 0x23bd, 0x251c, 0x2524, 0x2534,
	0x252c, 0x2502, 0x2264, 0x2265, 0x03c0, 0x2260, 0x00a3, 0x00b7,
}

func isRegional(r rune) bool { return r >= 0x1f1e6 && r <= 0x1f1ff }

func (t *Terminal) printRune(r rune) {
	if r >= 0x5f && r <= 0x7e && t.cur.charsets[t.cur.gl] == csDECGraphics {
		r = decGraphics[r-0x5f]
	}
	w := RuneWidth(r)
	lp := &t.lastPrint
	// Zero-width characters (combining marks, ZWJ, variation selectors)
	// and the second regional indicator of a flag attach to the previous
	// cell. Widths otherwise follow wcwidth so applications stay aligned.
	if lp.ok && (w == 0 || (isRegional(r) && lp.ri)) {
		joinRI := isRegional(r) && lp.ri
		t.appendToCell(lp.x, lp.y, r)
		lp.ri = false
		if joinRI {
			t.widenLast()
		}
		return
	}
	if w == 0 {
		return // nothing to combine with
	}
	if t.cur.wrapPending && t.Autowrap {
		t.wrapLine()
	}
	if w == 2 && t.cur.X == t.cols-1 {
		if t.Autowrap {
			line := t.buf.Line(t.cur.Y)
			pad := t.fill()
			pad.Attr |= AttrWrapPad
			line[t.cur.X] = pad
			t.wrapLine()
		} else {
			t.cur.X = max(0, t.cols-2)
		}
	}
	if t.Insert {
		t.insertBlanks(w)
	}
	line := t.buf.Line(t.cur.Y)
	x := t.cur.X
	t.clearWideAt(line, x)
	if w == 2 && x+1 < t.cols {
		t.clearWideAt(line, x+1)
	}
	pen := t.cur.pen
	c := Cell{Ch: r, Fg: pen.Fg, Bg: pen.Bg, Attr: pen.Attr, Extra: pen.Extra}
	if w == 2 {
		c.Attr |= AttrWide
		line[x] = c
		line[x+1] = Cell{Fg: pen.Fg, Bg: pen.Bg, Attr: pen.Attr | AttrWideCont, Extra: pen.Extra}
	} else {
		line[x] = c
	}
	t.buf.dirty[t.cur.Y] = true
	lp.x, lp.y, lp.ok = x, t.cur.Y, true
	lp.ri = isRegional(r)
	t.lastChar = r
	if x+w >= t.cols {
		t.cur.X = t.cols - 1
		if t.Autowrap {
			t.cur.wrapPending = true
		}
	} else {
		t.cur.X = x + w
	}
}

// clearWideAt removes the other half of a wide character overlapping x.
func (t *Terminal) clearWideAt(line []Cell, x int) {
	c := line[x]
	if c.Attr&AttrWideCont != 0 && x > 0 {
		line[x-1] = Cell{Fg: line[x-1].Fg, Bg: line[x-1].Bg}
	}
	if c.Attr&AttrWide != 0 && x+1 < len(line) {
		line[x+1] = Cell{Fg: line[x+1].Fg, Bg: line[x+1].Bg}
	}
}

const maxClusterBytes = 64

func (t *Terminal) appendToCell(x, y int, r rune) {
	if y >= t.rows || x >= t.cols {
		return
	}
	line := t.buf.Line(y)
	c := &line[x]
	if c.Attr&AttrWideCont != 0 && x > 0 {
		c = &line[x-1]
	}
	prev := c.Text()
	if len(prev) >= maxClusterBytes {
		return // cap pathological sequences (e.g. zalgo text)
	}
	s := prev + string(r)
	c.Ch = rune(internCluster(s))
	c.Attr |= AttrCluster
	t.buf.dirty[y] = true
}

// widenLast turns the last printed (narrow) cell into a wide one, used for
// regional indicator flags: each indicator is one column wide, so the pair
// occupies two columns, matching wcwidth-based applications.
func (t *Terminal) widenLast() {
	lp := &t.lastPrint
	line := t.buf.Line(lp.y)
	c := &line[lp.x]
	if c.Attr&AttrWide != 0 || lp.x+1 >= t.cols {
		return
	}
	// The cursor is right after the first indicator; it advances one more.
	if t.cur.wrapPending {
		return
	}
	c.Attr |= AttrWide
	t.clearWideAt(line, lp.x+1)
	line[lp.x+1] = Cell{Fg: c.Fg, Bg: c.Bg, Attr: c.Attr&AttrStyleMask | AttrWideCont, Extra: c.Extra}
	if t.cur.X+1 >= t.cols {
		t.cur.X = t.cols - 1
		t.cur.wrapPending = t.Autowrap
	} else {
		t.cur.X++
	}
}

// ---------------------------------------------------------------- controls

func (t *Terminal) execute(b byte) {
	switch b {
	case 0x07:
		t.events |= EvBell
	case 0x08:
		if t.cur.wrapPending {
			t.cur.wrapPending = false
		}
		if t.cur.X > 0 {
			t.cur.X--
		}
	case 0x09:
		t.tabForward(1)
	case 0x0a, 0x0b, 0x0c:
		t.index()
		if t.NewlineMode {
			t.cur.X = 0
		}
		t.cur.wrapPending = false
	case 0x0d:
		t.cur.X = 0
		t.cur.wrapPending = false
	case 0x0e:
		t.cur.gl = 1
	case 0x0f:
		t.cur.gl = 0
	}
	t.lastPrint.ok = false
}

func (t *Terminal) tabForward(n int) {
	x := t.cur.X
	for ; n > 0; n-- {
		x++
		for x < t.cols-1 && !t.tabs[x] {
			x++
		}
	}
	t.cur.X = min(x, t.cols-1)
	t.cur.wrapPending = false
}

func (t *Terminal) tabBackward(n int) {
	x := t.cur.X
	for ; n > 0 && x > 0; n-- {
		x--
		for x > 0 && !t.tabs[x] {
			x--
		}
	}
	t.cur.X = x
	t.cur.wrapPending = false
}

// index moves the cursor down, scrolling at the bottom margin.
func (t *Terminal) index() {
	if t.cur.Y == t.bot {
		t.scrollUp(1)
	} else if t.cur.Y < t.rows-1 {
		t.cur.Y++
	}
}

func (t *Terminal) reverseIndex() {
	if t.cur.Y == t.top {
		t.buf.ScrollDown(1, t.top, t.bot, t.fill())
	} else if t.cur.Y > 0 {
		t.cur.Y--
	}
}

func (t *Terminal) scrollUp(n int) {
	t.buf.ScrollUp(n, t.top, t.bot, t.fill(), !t.altActive)
	t.lastPrint.ok = false
}

// ---------------------------------------------------------------- cursor

func (t *Terminal) setCursor(x, y int) {
	minY, maxY := 0, t.rows-1
	if t.cur.origin {
		y += t.top
		minY, maxY = t.top, t.bot
	}
	t.cur.X = clamp(x, 0, t.cols-1)
	t.cur.Y = clamp(y, minY, maxY)
	t.cur.wrapPending = false
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func (t *Terminal) cursorUp(n int) {
	lo := 0
	if t.cur.Y >= t.top {
		lo = t.top
	}
	t.cur.Y = max(t.cur.Y-n, lo)
	t.cur.wrapPending = false
}

func (t *Terminal) cursorDown(n int) {
	hi := t.rows - 1
	if t.cur.Y <= t.bot {
		hi = t.bot
	}
	t.cur.Y = min(t.cur.Y+n, hi)
	t.cur.wrapPending = false
}

func (t *Terminal) saveCursor() {
	if t.altActive {
		t.savedAlt = t.cur
	} else {
		t.savedMain = t.cur
	}
}

func (t *Terminal) restoreCursor() {
	c := t.savedMain
	if t.altActive {
		c = t.savedAlt
	}
	t.cur = c
	t.cur.X = min(t.cur.X, t.cols-1)
	t.cur.Y = min(t.cur.Y, t.rows-1)
	t.restorePenExtra()
}

func (t *Terminal) restorePenExtra() {
	e := ExtraOf(t.cur.pen.Extra)
	t.ulColor, t.link = e.ULColor, e.Link
}

// ---------------------------------------------------------------- erase

func (t *Terminal) eraseCells(y, from, to int) {
	if from >= to {
		return
	}
	line := t.buf.Line(y)
	t.clearWideAt(line, from)
	t.clearWideAt(line, to-1)
	fillCells(line[from:to], t.fill())
	t.buf.dirty[y] = true
}

func (t *Terminal) eraseDisplay(mode int) {
	switch mode {
	case 0:
		t.eraseCells(t.cur.Y, t.cur.X, t.cols)
		for y := t.cur.Y + 1; y < t.rows; y++ {
			t.eraseCells(y, 0, t.cols)
			t.buf.SetWrapped(y, false)
		}
	case 1:
		for y := 0; y < t.cur.Y; y++ {
			t.eraseCells(y, 0, t.cols)
			t.buf.SetWrapped(y, false)
		}
		t.eraseCells(t.cur.Y, 0, t.cur.X+1)
	case 2:
		for y := 0; y < t.rows; y++ {
			t.eraseCells(y, 0, t.cols)
			t.buf.SetWrapped(y, false)
		}
	case 3:
		t.buf.ClearScrollback()
	}
	t.cur.wrapPending = false
}

func (t *Terminal) eraseLine(mode int) {
	switch mode {
	case 0:
		t.eraseCells(t.cur.Y, t.cur.X, t.cols)
		t.buf.SetWrapped(t.cur.Y, false)
	case 1:
		t.eraseCells(t.cur.Y, 0, t.cur.X+1)
	case 2:
		t.eraseCells(t.cur.Y, 0, t.cols)
		t.buf.SetWrapped(t.cur.Y, false)
	}
	t.cur.wrapPending = false
}

func (t *Terminal) insertBlanks(n int) {
	line := t.buf.Line(t.cur.Y)
	x := t.cur.X
	n = min(n, t.cols-x)
	t.clearWideAt(line, x)
	copy(line[x+n:], line[x:t.cols-n])
	f := t.fill()
	for i := x; i < x+n; i++ {
		line[i] = f
	}
	if last := &line[t.cols-1]; last.Attr&AttrWide != 0 {
		*last = f
	}
	t.buf.dirty[t.cur.Y] = true
	t.cur.wrapPending = false
}

func (t *Terminal) deleteChars(n int) {
	line := t.buf.Line(t.cur.Y)
	x := t.cur.X
	n = min(n, t.cols-x)
	t.clearWideAt(line, x)
	if x+n < t.cols {
		t.clearWideAt(line, x+n)
	}
	copy(line[x:], line[x+n:])
	f := t.fill()
	for i := t.cols - n; i < t.cols; i++ {
		line[i] = f
	}
	t.buf.dirty[t.cur.Y] = true
	t.cur.wrapPending = false
}

func (t *Terminal) insertLines(n int) {
	if t.cur.Y < t.top || t.cur.Y > t.bot {
		return
	}
	t.buf.ScrollDown(n, t.cur.Y, t.bot, t.fill())
	t.cur.X = 0
	t.cur.wrapPending = false
}

func (t *Terminal) deleteLines(n int) {
	if t.cur.Y < t.top || t.cur.Y > t.bot {
		return
	}
	t.buf.ScrollUp(n, t.cur.Y, t.bot, t.fill(), false)
	t.cur.X = 0
	t.cur.wrapPending = false
}

// ---------------------------------------------------------------- screens

func (t *Terminal) switchScreen(alt, clearAlt bool) {
	if alt == t.altActive {
		return
	}
	t.altActive = alt
	if alt {
		t.buf = t.alt
		if clearAlt {
			for y := 0; y < t.rows; y++ {
				fillCells(t.alt.Line(y), Cell{Bg: t.cur.pen.Bg})
				t.alt.SetWrapped(y, false)
			}
		}
	} else {
		t.buf = t.primary
	}
	t.buf.MarkAllDirty()
	t.top, t.bot = 0, t.rows-1
	t.lastPrint.ok = false
	t.events |= EvAltScreen
}

// ---------------------------------------------------------------- escapes

func (t *Terminal) escDispatch(final byte) {
	p := &t.parser
	if p.ninter > 0 {
		switch p.inter[0] {
		case '(', ')', '*', '+':
			slot := int(p.inter[0] - '(')
			if p.inter[0] == ')' {
				slot = 1
			} else if p.inter[0] == '*' {
				slot = 2
			} else if p.inter[0] == '+' {
				slot = 3
			}
			if final == '0' {
				t.cur.charsets[slot] = csDECGraphics
			} else {
				t.cur.charsets[slot] = csASCII
			}
		case '#':
			if final == '8' { // DECALN
				for y := 0; y < t.rows; y++ {
					line := t.buf.Line(y)
					for x := range line {
						line[x] = Cell{Ch: 'E'}
					}
					t.buf.dirty[y] = true
				}
				t.top, t.bot = 0, t.rows-1
				t.setCursor(0, 0)
			}
		}
		return
	}
	switch final {
	case '7':
		t.saveCursor()
	case '8':
		t.restoreCursor()
	case 'D':
		t.index()
	case 'E':
		t.index()
		t.cur.X = 0
		t.cur.wrapPending = false
	case 'H':
		t.tabs[t.cur.X] = true
	case 'M':
		t.reverseIndex()
		t.cur.wrapPending = false
	case 'c':
		t.fullReset()
	case '=':
		t.AppKeypad = true
	case '>':
		t.AppKeypad = false
	}
	t.lastPrint.ok = false
}

func (t *Terminal) fullReset() {
	t.switchScreen(false, false)
	t.reset()
	t.primary = NewBuffer(t.cols, t.rows, t.scrollback)
	t.primary.Evicted = t.buf.Evicted + int64(t.buf.Count())
	t.alt = NewBuffer(t.cols, t.rows, 0)
	t.buf = t.primary
	t.Title = ""
	t.events |= EvTitle | EvMouseMode | EvCursorStyle | EvAltScreen
}

func (t *Terminal) softReset() {
	t.CursorVisible = true
	t.cur.origin = false
	t.Autowrap = true
	t.Insert = false
	t.AppCursor, t.AppKeypad = false, false
	t.top, t.bot = 0, t.rows-1
	t.cur.pen = Cell{}
	t.cur.charsets = [4]byte{}
	t.cur.gl = 0
	t.ulColor, t.link = 0, 0
	t.savedMain, t.savedAlt = cursor{}, cursor{}
	t.cur.wrapPending = false
}

// ---------------------------------------------------------------- CSI

func (t *Terminal) csiDispatch(final byte) {
	p := &t.parser
	ps := &p.params
	var inter byte
	if p.ninter > 0 {
		inter = p.inter[0]
	}
	t.lastPrint.ok = false
	switch p.private {
	case 0:
		if inter != 0 {
			t.csiInter(inter, final)
			return
		}
	case '?':
		t.csiPrivate(final, inter)
		return
	case '>':
		switch final {
		case 'c': // DA2
			if ps.Raw(0, 0) == 0 {
				t.reply("\x1b[>1;10;0c")
			}
		case 'q': // XTVERSION
			t.reply("\x1bP>|gotti 1.0\x1b\\")
		case 'u': // kitty: push flags
			st := t.kittyStack()
			*st = append(*st, ps.Raw(0, 0)&0x1f)
			if len(*st) > 64 {
				*st = (*st)[1:]
			}
		}
		return
	case '<':
		if final == 'u' { // kitty: pop flags
			st := t.kittyStack()
			n := ps.Get(0, 1)
			if n >= len(*st) {
				*st = (*st)[:0]
			} else {
				*st = (*st)[:len(*st)-n]
			}
		}
		return
	case '=':
		if final == 'u' { // kitty: set flags
			st := t.kittyStack()
			flags := ps.Raw(0, 0) & 0x1f
			cur := t.KittyFlags()
			switch ps.Get(1, 1) {
			case 1:
				cur = flags
			case 2:
				cur |= flags
			case 3:
				cur &^= flags
			}
			if len(*st) == 0 {
				*st = append(*st, cur)
			} else {
				(*st)[len(*st)-1] = cur
			}
		}
		return
	default:
		return
	}

	switch final {
	case '@':
		t.insertBlanks(ps.Get(0, 1))
	case 'A':
		t.cursorUp(ps.Get(0, 1))
	case 'B', 'e':
		t.cursorDown(ps.Get(0, 1))
	case 'C', 'a':
		t.cur.X = min(t.cur.X+ps.Get(0, 1), t.cols-1)
		t.cur.wrapPending = false
	case 'D':
		t.cur.X = max(t.cur.X-ps.Get(0, 1), 0)
		t.cur.wrapPending = false
	case 'E':
		t.cursorDown(ps.Get(0, 1))
		t.cur.X = 0
	case 'F':
		t.cursorUp(ps.Get(0, 1))
		t.cur.X = 0
	case 'G', '`':
		t.cur.X = clamp(ps.Get(0, 1)-1, 0, t.cols-1)
		t.cur.wrapPending = false
	case 'H', 'f':
		t.setCursor(ps.Get(1, 1)-1, ps.Get(0, 1)-1)
	case 'I':
		t.tabForward(ps.Get(0, 1))
	case 'J':
		t.eraseDisplay(ps.Raw(0, 0))
	case 'K':
		t.eraseLine(ps.Raw(0, 0))
	case 'L':
		t.insertLines(ps.Get(0, 1))
	case 'M':
		t.deleteLines(ps.Get(0, 1))
	case 'P':
		t.deleteChars(ps.Get(0, 1))
	case 'S':
		t.buf.ScrollUp(ps.Get(0, 1), t.top, t.bot, t.fill(), !t.altActive)
	case 'T':
		if ps.Count() <= 1 {
			t.buf.ScrollDown(ps.Get(0, 1), t.top, t.bot, t.fill())
		}
	case 'X':
		n := min(ps.Get(0, 1), t.cols-t.cur.X)
		t.eraseCells(t.cur.Y, t.cur.X, t.cur.X+n)
		t.cur.wrapPending = false
	case 'Z':
		t.tabBackward(ps.Get(0, 1))
	case 'b': // REP
		if t.lastChar != 0 {
			n := min(ps.Get(0, 1), t.cols*t.rows)
			for i := 0; i < n; i++ {
				t.printRune(t.lastChar)
			}
		}
	case 'c': // DA1
		if ps.Raw(0, 0) == 0 {
			t.reply("\x1b[?62;22c")
		}
	case 'd':
		y := ps.Get(0, 1) - 1
		if t.cur.origin {
			t.setCursor(t.cur.X, y)
		} else {
			t.cur.Y = clamp(y, 0, t.rows-1)
			t.cur.wrapPending = false
		}
	case 'g':
		switch ps.Raw(0, 0) {
		case 0:
			t.tabs[t.cur.X] = false
		case 3:
			clear(t.tabs)
		}
	case 'h', 'l':
		set := final == 'h'
		for i := 0; i < ps.Count(); i++ {
			switch ps.Raw(i, 0) {
			case 4:
				t.Insert = set
			case 20:
				t.NewlineMode = set
			}
		}
	case 'm':
		t.sgr()
	case 'n':
		switch ps.Raw(0, 0) {
		case 5:
			t.reply("\x1b[0n")
		case 6:
			y := t.cur.Y
			if t.cur.origin {
				y -= t.top
			}
			t.reply("\x1b[" + strconv.Itoa(y+1) + ";" + strconv.Itoa(t.cur.X+1) + "R")
		}
	case 'r':
		top := ps.Get(0, 1) - 1
		bot := ps.Get(1, t.rows) - 1
		bot = min(bot, t.rows-1)
		if top < bot {
			t.top, t.bot = top, bot
			t.setCursor(0, 0)
		}
	case 's':
		t.saveCursor()
	case 'u':
		t.restoreCursor()
	case 't':
		t.windowOp()
	}
}

func (t *Terminal) windowOp() {
	ps := &t.parser.params
	switch ps.Raw(0, 0) {
	case 14:
		t.reply("\x1b[4;" + strconv.Itoa(t.rows*t.CellH) + ";" + strconv.Itoa(t.cols*t.CellW) + "t")
	case 16:
		t.reply("\x1b[6;" + strconv.Itoa(t.CellH) + ";" + strconv.Itoa(t.CellW) + "t")
	case 18:
		t.reply("\x1b[8;" + strconv.Itoa(t.rows) + ";" + strconv.Itoa(t.cols) + "t")
	case 22:
		if len(t.titleStack) < 32 {
			t.titleStack = append(t.titleStack, t.Title)
		}
	case 23:
		if n := len(t.titleStack); n > 0 {
			t.Title = t.titleStack[n-1]
			t.titleStack = t.titleStack[:n-1]
			t.events |= EvTitle
		}
	}
}

func (t *Terminal) csiInter(inter, final byte) {
	ps := &t.parser.params
	switch {
	case inter == ' ' && final == 'q': // DECSCUSR
		n := ps.Raw(0, 0)
		switch n {
		case 0: // back to the user's default
			t.CursorStyle = t.defaultCursor
		case 1, 2:
			t.CursorStyle = CursorBlock
		case 3, 4:
			t.CursorStyle = CursorUnderline
		case 5, 6:
			t.CursorStyle = CursorBeam
		}
		t.CursorBlink = n%2 == 1
		t.events |= EvCursorStyle
	case inter == '!' && final == 'p':
		t.softReset()
	case inter == '$' && final == 'p': // DECRQM (ANSI)
		mode := ps.Raw(0, 0)
		st := 0
		switch mode {
		case 4:
			st = boolMode(t.Insert)
		case 20:
			st = boolMode(t.NewlineMode)
		}
		t.reply("\x1b[" + strconv.Itoa(mode) + ";" + strconv.Itoa(st) + "$y")
	}
}

func boolMode(b bool) int {
	if b {
		return 1
	}
	return 2
}

func (t *Terminal) csiPrivate(final, inter byte) {
	ps := &t.parser.params
	switch {
	case inter == '$' && final == 'p': // DECRQM (DEC private)
		mode := ps.Raw(0, 0)
		t.reply("\x1b[?" + strconv.Itoa(mode) + ";" + strconv.Itoa(t.decModeState(mode)) + "$y")
		return
	case inter != 0:
		return
	}
	switch final {
	case 'h', 'l':
		set := final == 'h'
		for i := 0; i < ps.Count(); i++ {
			t.setDECMode(ps.Raw(i, 0), set)
		}
	case 'n':
		if ps.Raw(0, 0) == 6 {
			y := t.cur.Y
			if t.cur.origin {
				y -= t.top
			}
			t.reply("\x1b[?" + strconv.Itoa(y+1) + ";" + strconv.Itoa(t.cur.X+1) + "R")
		}
	case 'J':
		t.eraseDisplay(ps.Raw(0, 0))
	case 'K':
		t.eraseLine(ps.Raw(0, 0))
	case 'u':
		t.reply("\x1b[?" + strconv.Itoa(t.KittyFlags()) + "u")
	}
}

func (t *Terminal) decModeState(mode int) int {
	switch mode {
	case 1:
		return boolMode(t.AppCursor)
	case 5:
		return boolMode(t.ReverseVideo)
	case 6:
		return boolMode(t.cur.origin)
	case 7:
		return boolMode(t.Autowrap)
	case 12:
		return boolMode(t.CursorBlink)
	case 25:
		return boolMode(t.CursorVisible)
	case 47, 1047, 1049:
		return boolMode(t.altActive)
	case 1000:
		return boolMode(t.MouseMode == MouseClick)
	case 1002:
		return boolMode(t.MouseMode == MouseButton)
	case 1003:
		return boolMode(t.MouseMode == MouseAny)
	case 1004:
		return boolMode(t.FocusEvents)
	case 1006:
		return boolMode(t.MouseSGR)
	case 1007:
		return boolMode(t.AltScroll)
	case 2004:
		return boolMode(t.BracketedPaste)
	case 2026:
		return boolMode(t.SyncOutput)
	case 3: // DECCOLM: not supported
		return 4
	}
	return 0
}

func (t *Terminal) setDECMode(mode int, set bool) {
	switch mode {
	case 1:
		t.AppCursor = set
	case 5:
		if t.ReverseVideo != set {
			t.ReverseVideo = set
			t.buf.MarkAllDirty()
		}
	case 6:
		t.cur.origin = set
		t.setCursor(0, 0)
	case 7:
		t.Autowrap = set
		if !set {
			t.cur.wrapPending = false
		}
	case 12:
		t.CursorBlink = set
		t.events |= EvCursorStyle
	case 25:
		t.CursorVisible = set
	case 47:
		t.switchScreen(set, false)
	case 1047:
		if !set && t.altActive {
			for y := 0; y < t.rows; y++ {
				fillCells(t.alt.Line(y), Cell{})
			}
		}
		t.switchScreen(set, false)
	case 1048:
		if set {
			t.saveCursor()
		} else {
			t.restoreCursor()
		}
	case 1049:
		if set {
			if !t.altActive {
				t.savedMain = t.cur
				t.switchScreen(true, true)
			}
		} else if t.altActive {
			t.switchScreen(false, false)
			t.cur = t.savedMain
			t.cur.X = min(t.cur.X, t.cols-1)
			t.cur.Y = min(t.cur.Y, t.rows-1)
			t.restorePenExtra()
		}
	case 1000, 1002, 1003:
		if set {
			t.MouseMode = map[int]int{1000: MouseClick, 1002: MouseButton, 1003: MouseAny}[mode]
		} else {
			t.MouseMode = MouseOff
		}
		t.events |= EvMouseMode
	case 1004:
		t.FocusEvents = set
	case 1006:
		t.MouseSGR = set
	case 1007:
		t.AltScroll = set
	case 2004:
		t.BracketedPaste = set
	case 2026:
		if set {
			if !t.SyncOutput {
				t.SyncSince = time.Now()
			}
		} else if t.SyncOutput {
			t.events |= EvSync
		}
		t.SyncOutput = set
	}
}

// ---------------------------------------------------------------- SGR

func (t *Terminal) sgr() {
	ps := &t.parser.params
	pen := &t.cur.pen
	if ps.N == 0 {
		*pen = Cell{}
		t.ulColor = 0
		t.updateExtra()
		return
	}
	extraChanged := false
	for k := 0; k < ps.N; k++ {
		v := int(ps.V[k])
		if v < 0 {
			v = 0
		}
		// sub-parameters belonging to this parameter
		j := k + 1
		for j < ps.N && ps.Sub[j] {
			j++
		}
		subs := ps.V[k+1 : j]
		switch {
		case v == 0:
			pen.Fg, pen.Bg, pen.Attr = 0, 0, 0
			if t.ulColor != 0 {
				t.ulColor = 0
				extraChanged = true
			}
		case v == 1:
			pen.Attr |= AttrBold
		case v == 2:
			pen.Attr |= AttrDim
		case v == 3:
			pen.Attr |= AttrItalic
		case v == 4:
			u := ULSingle
			if len(subs) > 0 {
				u = int(max(subs[0], 0))
				if u > ULDashed {
					u = ULSingle
				}
			}
			pen.Attr = pen.Attr.WithUnderline(u)
		case v == 7:
			pen.Attr |= AttrInverse
		case v == 8:
			pen.Attr |= AttrHidden
		case v == 9:
			pen.Attr |= AttrStrike
		case v == 21:
			pen.Attr = pen.Attr.WithUnderline(ULDouble)
		case v == 22:
			pen.Attr &^= AttrBold | AttrDim
		case v == 23:
			pen.Attr &^= AttrItalic
		case v == 24:
			pen.Attr = pen.Attr.WithUnderline(ULNone)
		case v == 27:
			pen.Attr &^= AttrInverse
		case v == 28:
			pen.Attr &^= AttrHidden
		case v == 29:
			pen.Attr &^= AttrStrike
		case v >= 30 && v <= 37:
			pen.Fg = Indexed(uint8(v - 30))
		case v == 38, v == 48, v == 58:
			c, next, ok := parseExtColor(ps, k, subs)
			if ok {
				switch v {
				case 38:
					pen.Fg = c
				case 48:
					pen.Bg = c
				case 58:
					t.ulColor = c
					extraChanged = true
				}
			}
			if next > j {
				j = next
			}
		case v == 39:
			pen.Fg = 0
		case v >= 40 && v <= 47:
			pen.Bg = Indexed(uint8(v - 40))
		case v == 49:
			pen.Bg = 0
		case v == 53:
			pen.Attr |= AttrOverline
		case v == 55:
			pen.Attr &^= AttrOverline
		case v == 59:
			t.ulColor = 0
			extraChanged = true
		case v >= 90 && v <= 97:
			pen.Fg = Indexed(uint8(v - 90 + 8))
		case v >= 100 && v <= 107:
			pen.Bg = Indexed(uint8(v - 100 + 8))
		}
		k = j - 1
	}
	if extraChanged {
		t.updateExtra()
	}
}

// parseExtColor parses 38/48/58 colour specs in either the colon form
// (38:2::r:g:b, 38:5:n) or the legacy semicolon form (38;2;r;g;b).
// It returns the index of the next unconsumed parameter.
func parseExtColor(ps *Params, k int, subs []int32) (Color, int, bool) {
	get := func(a []int32, i int) int {
		if i < len(a) && a[i] >= 0 {
			return int(a[i])
		}
		return 0
	}
	if len(subs) > 0 {
		switch subs[0] {
		case 5:
			if len(subs) >= 2 {
				return Indexed(uint8(get(subs, 1))), 0, true
			}
		case 2:
			if len(subs) >= 5 { // 38:2:cs:r:g:b
				return RGB(uint8(get(subs, 2)), uint8(get(subs, 3)), uint8(get(subs, 4))), 0, true
			}
			if len(subs) == 4 { // 38:2:r:g:b
				return RGB(uint8(get(subs, 1)), uint8(get(subs, 2)), uint8(get(subs, 3))), 0, true
			}
		}
		return 0, 0, false
	}
	rest := ps.V[k+1 : ps.N]
	if len(rest) == 0 {
		return 0, k + 1, false
	}
	switch rest[0] {
	case 5:
		if len(rest) >= 2 {
			return Indexed(uint8(get(rest, 1))), k + 3, true
		}
	case 2:
		if len(rest) >= 4 {
			return RGB(uint8(get(rest, 1)), uint8(get(rest, 2)), uint8(get(rest, 3))), k + 5, true
		}
	}
	return 0, ps.N, false
}

func (t *Terminal) updateExtra() {
	t.cur.pen.Extra = internExtra(Extra{ULColor: t.ulColor, Link: t.link})
}

// Reset performs a full terminal reset (RIS), as if the application had
// sent ESC c. The caller holds Mu.
func (t *Terminal) Reset() { t.fullReset() }
