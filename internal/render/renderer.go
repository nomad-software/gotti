package render

import (
	"fmt"
	"strings"
	"unsafe"

	"github.com/go-gl/gl/v3.3-core/gl"

	"termite/internal/term"
)

// rectInst is one background rectangle (12 bytes).
type rectInst struct {
	X, Y, W, H int16
	Color      uint32 // RGBA, R in the low byte
}

// glyphInst is one textured quad (20 bytes).
type glyphInst struct {
	X, Y  int16
	W, H  uint16
	U, V  uint16
	Flags uint16
	_     uint16
	Color uint32
}

const (
	flagColorGlyph = 1
	overlayRects   = 16
)

// Selection is a highlighted range in absolute coordinates.
type Selection struct {
	Active     bool
	Start, End term.Pos // End exclusive
	Block      bool
}

// View is the per-frame UI state that is not part of the terminal.
type View struct {
	TopAbs      int64 // absolute line shown in row 0; <0 follows the bottom
	Sel         Selection
	Matches     []term.Match
	Current     int // index of the search match shown, -1 none
	Hover       term.Match
	HoverActive bool
	Focused     bool
	BlinkOn     bool
	Preedit     string

	Dirty       bool // selection/search/hover/focus changed: rebuild all
	CursorDirty bool // only the cursor needs redrawing (blink)
}

// Style holds theme values the terminal doesn't know about.
type Style struct {
	SelBg, SelFg uint32 // 0xRRGGBB, also of the current search match
	SelFgSet     bool   // SelFg is used; otherwise selected text keeps its colours
	CursorText   uint32
	BoldIsBright bool
	Ligatures    bool
	CursorBlink  bool // allow blinking
	PadX, PadY   int
}

// Renderer draws one terminal into a GL framebuffer.
type Renderer struct {
	Cache *GlyphCache
	Style Style

	// GL objects of this renderer's context (programs are shared).
	vaoRect, vaoGlyph        uint32
	quadVBO, rectVBO, glyVBO uint32
	glReady                  bool

	cols, rows int
	fbW, fbH   int
	cells      []term.Cell
	rowAbs     []int64
	rowDirty   []bool
	allDirty   bool
	// Each row builds into a fixed slot of rects and glyphs, rebuilt only
	// when the row is damaged; rowRects and rowGlyphs count what it used.
	// Every frame the used parts are packed into packedRects/packedGlyphs,
	// so the GPU draws only real instances, never empty slots.
	rectSlot     int
	glyphSlot    int
	rects        []rectInst
	glyphs       []glyphInst
	rowRects     []int // per row, then the overlay last
	rowGlyphs    []int
	packedRects  []rectInst
	packedGlyphs []glyphInst
	gpuRects     int // buffer capacity allocated on the GPU (instances)
	gpuGlyphs    int

	// Drawn counts the instances the last frame drew, for TERMITE_DEBUG.
	Drawn    int
	cacheGen int

	// snapshot of terminal state
	palette       [256]uint32
	defFg, defBg  uint32
	cursorColor   uint32
	reverse       bool
	curX, curY    int // cursor row in view coordinates (-1 hidden)
	curVisible    bool
	curStyle      int
	curBlink      bool
	topAbs        int64
	screenTopAbs  int64
	view          View
	lastCurY      int
	lastPreedit   string
	lastSigCursor [5]int
}

// NewRenderer creates a renderer; InitGL must be called with a current context.
func NewRenderer(cache *GlyphCache) *Renderer {
	return &Renderer{Cache: cache, allDirty: true, curY: -1, lastCurY: -1}
}

// CellSize returns the cell size in device pixels.
func (r *Renderer) CellSize() (int, int) { return r.Cache.Fonts.CellW, r.Cache.Fonts.CellH }

// GridSize returns how many cells fit in a framebuffer of w×h pixels.
func (r *Renderer) GridSize(w, h int) (cols, rows int) {
	cw, ch := r.CellSize()
	cols = max(2, (w-2*r.Style.PadX)/cw)
	rows = max(1, (h-2*r.Style.PadY)/ch)
	return
}

// SetFramebufferSize records the drawable size in device pixels.
func (r *Renderer) SetFramebufferSize(w, h int) {
	if w != r.fbW || h != r.fbH {
		r.fbW, r.fbH = w, h
		r.allDirty = true
	}
}

// Invalidate forces a full rebuild on the next frame.
func (r *Renderer) Invalidate() { r.allDirty = true }

// Prepare copies the state needed for drawing. The caller holds t.Mu.
// It reports whether anything needs to be drawn.
func (r *Renderer) Prepare(t *term.Terminal, v *View) bool {
	cols, rows := t.Size()
	if cols != r.cols || rows != r.rows {
		r.cols, r.rows = cols, rows
		r.cells = make([]term.Cell, cols*rows)
		r.rowAbs = make([]int64, rows)
		r.rowDirty = make([]bool, rows)
		r.rectSlot = cols + 1
		// Each cell can draw a glyph, an underline, a strike and an
		// overline, and the row one cursor.
		r.glyphSlot = cols*4 + 5
		r.rects = make([]rectInst, rows*r.rectSlot+overlayRects)
		r.glyphs = make([]glyphInst, rows*r.glyphSlot+cols*2+8)
		r.rowRects = make([]int, rows+1)
		r.rowGlyphs = make([]int, rows+1)
		r.allDirty = true
	}
	buf := t.Buf()
	r.screenTopAbs = buf.ScreenTopAbs()
	top := v.TopAbs
	if top < 0 || top > r.screenTopAbs {
		top = r.screenTopAbs
	}
	if minTop := buf.Evicted; top < minTop {
		top = minTop
	}
	if top != r.topAbs {
		r.topAbs = top
		r.allDirty = true
	}
	scrolled := top != r.screenTopAbs
	if v.Dirty || (scrolled && buf.AnyDirty()) {
		r.allDirty = true
	}
	if r.Cache.Gen != r.cacheGen {
		r.cacheGen = r.Cache.Gen
		r.allDirty = true
	}
	// colours
	if r.palette != t.Palette || r.defFg != t.DefaultFg || r.defBg != t.DefaultBg ||
		r.cursorColor != t.CursorColor || r.reverse != t.ReverseVideo {
		r.palette = t.Palette
		r.defFg, r.defBg, r.cursorColor = t.DefaultFg, t.DefaultBg, t.CursorColor
		r.reverse = t.ReverseVideo
		r.allDirty = true
	}
	changed := r.allDirty
	for y := 0; y < rows; y++ {
		abs := top + int64(y)
		if !r.allDirty && !(!scrolled && buf.IsDirty(y)) {
			continue
		}
		li := buf.AbsToLogical(abs)
		dst := r.cells[y*cols : (y+1)*cols]
		if li < 0 {
			clear(dst)
		} else {
			copy(dst, buf.LogicalLine(li))
		}
		r.rowAbs[y] = abs
		r.rowDirty[y] = true
		changed = true
	}
	buf.ClearDirty()

	// cursor
	cx, cy := t.CursorPos()
	vy := int(r.screenTopAbs + int64(cy) - top)
	if vy < 0 || vy >= rows {
		vy = -1
	}
	r.curX, r.curY = cx, vy
	r.curVisible = t.CursorVisible
	r.curStyle = t.CursorStyle
	r.curBlink = t.CursorBlink && r.Style.CursorBlink
	r.view = *v
	sig := [5]int{cx, vy, b2i(r.curVisible), r.curStyle, b2i(v.Focused)}
	if sig != r.lastSigCursor || v.CursorDirty || v.Preedit != r.lastPreedit {
		r.markRow(r.lastCurY)
		r.markRow(vy)
		r.lastSigCursor = sig
		r.lastCurY = vy
		r.lastPreedit = v.Preedit
		changed = true
	}
	return changed || v.CursorDirty
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (r *Renderer) markRow(y int) {
	if y >= 0 && y < r.rows {
		r.rowDirty[y] = true
	}
}

// ---------------------------------------------------------------- GL setup

const rectVS = `#version 330 core
layout(location=0) in vec2 corner;
layout(location=1) in vec4 rect;
layout(location=2) in vec4 color;
uniform vec2 viewport;
out vec4 vColor;
void main() {
	vec2 p = rect.xy + corner * rect.zw;
	gl_Position = vec4(p.x / viewport.x * 2.0 - 1.0, 1.0 - p.y / viewport.y * 2.0, 0.0, 1.0);
	vColor = color;
}`

const rectFS = `#version 330 core
in vec4 vColor;
out vec4 frag;
void main() { frag = vec4(vColor.rgb * vColor.a, vColor.a); }`

const glyphVS = `#version 330 core
layout(location=0) in vec2 corner;
layout(location=1) in vec4 rect;
layout(location=2) in vec2 uv;
layout(location=3) in uint flags;
layout(location=4) in vec4 color;
uniform vec2 viewport;
uniform vec2 atlasSize;
out vec2 vUV;
out vec4 vColor;
flat out uint vFlags;
void main() {
	vec2 p = rect.xy + corner * rect.zw;
	gl_Position = vec4(p.x / viewport.x * 2.0 - 1.0, 1.0 - p.y / viewport.y * 2.0, 0.0, 1.0);
	vUV = (uv + corner * rect.zw) / atlasSize;
	vColor = color;
	vFlags = flags;
}`

const glyphFS = `#version 330 core
in vec2 vUV;
in vec4 vColor;
flat in uint vFlags;
uniform sampler2D atlas;
out vec4 frag;
void main() {
	vec4 t = texture(atlas, vUV);
	if ((vFlags & 1u) != 0u) {
		frag = t;
	} else {
		float a = t.a * vColor.a;
		frag = vec4(vColor.rgb * a, a);
	}
}`

func compile(vs, fs string) (uint32, error) {
	sh := func(src string, kind uint32) (uint32, error) {
		s := gl.CreateShader(kind)
		cs, free := gl.Strs(src + "\x00")
		gl.ShaderSource(s, 1, cs, nil)
		free()
		gl.CompileShader(s)
		var ok int32
		gl.GetShaderiv(s, gl.COMPILE_STATUS, &ok)
		if ok == 0 {
			var n int32
			gl.GetShaderiv(s, gl.INFO_LOG_LENGTH, &n)
			log := strings.Repeat("\x00", int(n+1))
			gl.GetShaderInfoLog(s, n, nil, gl.Str(log))
			gl.DeleteShader(s)
			return 0, fmt.Errorf("shader: %s", strings.TrimRight(log, "\x00"))
		}
		return s, nil
	}
	v, err := sh(vs, gl.VERTEX_SHADER)
	if err != nil {
		return 0, err
	}
	f, err := sh(fs, gl.FRAGMENT_SHADER)
	if err != nil {
		gl.DeleteShader(v)
		return 0, err
	}
	p := gl.CreateProgram()
	gl.AttachShader(p, v)
	gl.AttachShader(p, f)
	gl.LinkProgram(p)
	gl.DeleteShader(v)
	gl.DeleteShader(f)
	var ok int32
	gl.GetProgramiv(p, gl.LINK_STATUS, &ok)
	if ok == 0 {
		var n int32
		gl.GetProgramiv(p, gl.INFO_LOG_LENGTH, &n)
		log := strings.Repeat("\x00", int(n+1))
		gl.GetProgramInfoLog(p, n, nil, gl.Str(log))
		gl.DeleteProgram(p)
		return 0, fmt.Errorf("link: %s", strings.TrimRight(log, "\x00"))
	}
	return p, nil
}

// Shader programs are shared by every renderer: all pane contexts share
// resources, and compiling once saves memory and startup time.
var shared struct {
	progRect, progGlyph       uint32
	uVPRect, uVPGlyph, uAtlas int32
	uAtlasSize                int32
}

func initPrograms() error {
	if shared.progRect != 0 && shared.progGlyph != 0 && gl.IsProgram(shared.progRect) {
		return nil
	}
	var err error
	if shared.progRect, err = compile(rectVS, rectFS); err != nil {
		return err
	}
	if shared.progGlyph, err = compile(glyphVS, glyphFS); err != nil {
		// Both or neither: the next pane tries again from the start.
		gl.DeleteProgram(shared.progRect)
		shared.progRect = 0
		return err
	}
	shared.uVPRect = gl.GetUniformLocation(shared.progRect, gl.Str("viewport\x00"))
	shared.uVPGlyph = gl.GetUniformLocation(shared.progGlyph, gl.Str("viewport\x00"))
	shared.uAtlasSize = gl.GetUniformLocation(shared.progGlyph, gl.Str("atlasSize\x00"))
	shared.uAtlas = gl.GetUniformLocation(shared.progGlyph, gl.Str("atlas\x00"))
	return nil
}

// InitGL creates GL objects in the current context. It may be called again
// if the widget's context is recreated.
func (r *Renderer) InitGL() error {
	if err := initPrograms(); err != nil {
		return err
	}

	quad := []float32{0, 0, 1, 0, 0, 1, 1, 1}
	gl.GenBuffers(1, &r.quadVBO)
	gl.BindBuffer(gl.ARRAY_BUFFER, r.quadVBO)
	gl.BufferData(gl.ARRAY_BUFFER, len(quad)*4, gl.Ptr(quad), gl.STATIC_DRAW)

	gl.GenVertexArrays(1, &r.vaoRect)
	gl.GenBuffers(1, &r.rectVBO)
	gl.BindVertexArray(r.vaoRect)
	gl.BindBuffer(gl.ARRAY_BUFFER, r.quadVBO)
	gl.EnableVertexAttribArray(0)
	gl.VertexAttribPointerWithOffset(0, 2, gl.FLOAT, false, 8, 0)
	gl.BindBuffer(gl.ARRAY_BUFFER, r.rectVBO)
	gl.EnableVertexAttribArray(1)
	gl.VertexAttribPointerWithOffset(1, 4, gl.SHORT, false, 12, 0)
	gl.VertexAttribDivisor(1, 1)
	gl.EnableVertexAttribArray(2)
	gl.VertexAttribPointerWithOffset(2, 4, gl.UNSIGNED_BYTE, true, 12, 8)
	gl.VertexAttribDivisor(2, 1)

	gl.GenVertexArrays(1, &r.vaoGlyph)
	gl.GenBuffers(1, &r.glyVBO)
	gl.BindVertexArray(r.vaoGlyph)
	gl.BindBuffer(gl.ARRAY_BUFFER, r.quadVBO)
	gl.EnableVertexAttribArray(0)
	gl.VertexAttribPointerWithOffset(0, 2, gl.FLOAT, false, 8, 0)
	gl.BindBuffer(gl.ARRAY_BUFFER, r.glyVBO)
	gl.EnableVertexAttribArray(1)
	gl.VertexAttribPointerWithOffset(1, 4, gl.SHORT, false, 20, 0)
	gl.VertexAttribDivisor(1, 1)
	gl.EnableVertexAttribArray(2)
	gl.VertexAttribPointerWithOffset(2, 2, gl.UNSIGNED_SHORT, false, 20, 8)
	gl.VertexAttribDivisor(2, 1)
	gl.EnableVertexAttribArray(3)
	gl.VertexAttribIPointerWithOffset(3, 1, gl.UNSIGNED_SHORT, 20, 12)
	gl.VertexAttribDivisor(3, 1)
	gl.EnableVertexAttribArray(4)
	gl.VertexAttribPointerWithOffset(4, 4, gl.UNSIGNED_BYTE, true, 20, 16)
	gl.VertexAttribDivisor(4, 1)
	gl.BindVertexArray(0)

	r.gpuRects, r.gpuGlyphs = 0, 0
	r.allDirty = true
	r.glReady = true
	return nil
}

// ReleaseGL deletes per-renderer GL objects (shared cache excluded).
func (r *Renderer) ReleaseGL() {
	if !r.glReady {
		return
	}
	gl.DeleteVertexArrays(1, &r.vaoRect)
	gl.DeleteVertexArrays(1, &r.vaoGlyph)
	bufs := []uint32{r.quadVBO, r.rectVBO, r.glyVBO}
	gl.DeleteBuffers(3, &bufs[0])
	r.glReady = false
}

// ---------------------------------------------------------------- frame

func rgba(c uint32, a uint8) uint32 {
	return (c>>16)&0xff | (c>>8&0xff)<<8 | (c&0xff)<<16 | uint32(a)<<24
}

func (r *Renderer) resolve(c term.Color, isFg, bold bool) uint32 {
	switch {
	case c.IsRGB():
		cr, cg, cb := c.RGB()
		return uint32(cr)<<16 | uint32(cg)<<8 | uint32(cb)
	case c.IsIndexed():
		i := c.Index()
		if isFg && bold && r.Style.BoldIsBright && i < 8 {
			i += 8
		}
		return r.palette[i]
	}
	if isFg != r.reverse {
		return r.defFg
	}
	return r.defBg
}

func mix(a, b uint32, t float64) uint32 {
	f := func(s uint) uint32 {
		x, y := float64(a>>s&0xff), float64(b>>s&0xff)
		return uint32(x*(1-t)+y*t) << s
	}
	return f(16) | f(8) | f(0)
}

func (r *Renderer) inSelection(abs int64, x int) bool {
	s := &r.view.Sel
	if !s.Active {
		return false
	}
	a, b := s.Start, s.End
	if b.Less(a) {
		a, b = b, a
	}
	if abs < a.Line || abs > b.Line {
		return false
	}
	if s.Block {
		lo, hi := min(a.Col, b.Col), max(a.Col, b.Col)
		return x >= lo && x < hi
	}
	p := term.Pos{Line: abs, Col: x}
	return !p.Less(a) && p.Less(b)
}

func inMatch(m term.Match, abs int64, x int) bool {
	p := term.Pos{Line: abs, Col: x}
	return !p.Less(m.Start) && p.Less(m.End)
}

// inCurrentMatch reports whether the cell is in the current search match,
// the only one shown: Enter in the search bar moves to the next.
func (r *Renderer) inCurrentMatch(abs int64, x int) bool {
	c := r.view.Current
	return c >= 0 && c < len(r.view.Matches) && inMatch(r.view.Matches[c], abs, x)
}

var ligatureCandidates = []string{
	"<!--", "===", "!==", "<=>", "->>", "<<-", "...", "|||", "&&&", "<->", "-->", "<--", "=>>", "|>>", "::=", "/**", "**/",
	"->", "<-", "=>", "<=", ">=", "==", "!=", "&&", "||", "::", "..", "//", "/*", "*/", "++", "--", "<<", ">>", "|>", "<|", ":=", "=<", "##", "~>", "<~", "www",
}

// ligatureStart flags the bytes a ligature candidate can begin with, so
// most cells skip the candidate scan.
var ligatureStart = func() (t [128]bool) {
	for _, c := range ligatureCandidates {
		t[c[0]] = true
	}
	return
}()

// buildRow fills the instance slots for view row y.
func (r *Renderer) buildRow(y int) {
	fs := r.Cache.Fonts
	cw, ch := fs.CellW, fs.CellH
	cols := r.cols
	rects := r.rects[y*r.rectSlot : (y+1)*r.rectSlot]
	glyphs := r.glyphs[y*r.glyphSlot : (y+1)*r.glyphSlot]
	nr, ng := 0, 0
	defer func() { r.rowRects[y], r.rowGlyphs[y] = nr, ng }()
	line := r.cells[y*cols : (y+1)*cols]
	abs := r.rowAbs[y]
	py := r.Style.PadY + y*ch
	defBg := r.defBg
	if r.reverse {
		defBg = r.defFg
	}

	cursorHere := y == r.curY && r.curVisible && (!r.curBlink || r.view.BlinkOn || !r.view.Focused)
	curStyle := r.curStyle
	preStart, preEnd := -1, -1
	if y == r.curY && r.view.Preedit != "" {
		preStart = r.curX
		preEnd = min(cols, r.curX+term.StringWidth(r.view.Preedit))
		cursorHere = false
	}
	addGlyph := func(s Sprite, x, py int, color uint32) {
		if ng >= len(glyphs) {
			return
		}
		g := &glyphs[ng]
		ng++
		g.X = int16(r.Style.PadX + x + int(s.OffX))
		g.Y = int16(py + int(s.OffY))
		g.W, g.H, g.U, g.V = s.W, s.H, s.X, s.Y
		g.Color = color
		if s.Color {
			g.Flags = flagColorGlyph
		} else {
			g.Flags = 0
		}
	}
	addRect := func(px, w int, color uint32) {
		if nr > 0 {
			last := &rects[nr-1]
			if last.Color == color && int(last.X)+int(last.W) == px {
				last.W += int16(w)
				return
			}
		}
		if nr >= len(rects) {
			return
		}
		rects[nr] = rectInst{X: int16(px), Y: int16(py), W: int16(w), H: int16(ch), Color: color}
		nr++
	}

	for x := 0; x < cols; x++ {
		c := line[x]
		if c.Attr&term.AttrWideCont != 0 {
			continue
		}
		wcells := 1
		if c.Attr&term.AttrWide != 0 && x+1 < cols {
			wcells = 2
		}
		bold := c.Attr&term.AttrBold != 0
		fg := r.resolve(c.Fg, true, bold)
		bg := r.resolve(c.Bg, false, false)
		if c.Attr&term.AttrInverse != 0 {
			fg, bg = bg, fg
		}
		if c.Attr&term.AttrDim != 0 {
			fg = mix(fg, bg, 0.45)
		}
		// The current search match looks like the selection.
		if r.inSelection(abs, x) || r.inCurrentMatch(abs, x) {
			bg = r.Style.SelBg
			if r.Style.SelFgSet {
				fg = r.Style.SelFg
			}
		}
		isCursor := cursorHere && x == r.curX
		if isCursor && curStyle == term.CursorBlock && r.view.Focused {
			bg = r.cursorColor
			fg = r.Style.CursorText
			if r.cursorColor != r.palette[0] && fg == r.cursorColor {
				fg = r.defBg
			}
		}
		px := r.Style.PadX + x*cw
		if bg != defBg {
			addRect(px, cw*wcells, rgba(bg, 255))
		}
		if x >= preStart && x < preEnd {
			x += wcells - 1
			continue
		}
		hidden := c.Attr&term.AttrHidden != 0
		// glyph
		if !hidden && c.Ch != 0 && c.Ch != ' ' {
			style := uint8(0)
			if bold {
				style |= styleBold
			}
			if c.Attr&term.AttrItalic != 0 {
				style |= styleItalic
			}
			// optional ligatures over runs of ASCII punctuation
			if r.Style.Ligatures && c.Attr&term.AttrCluster == 0 && c.Ch < 0x80 && ligatureStart[c.Ch] {
				if n, s := r.ligatureAt(line, x, style); n > 1 && r.plainRun(line, abs, x, n, cursorHere) {
					addGlyph(s, x*cw, py, rgba(fg, 255))
					// draw backgrounds for the following cells, then skip them
					for k := 1; k < n; k++ {
						cc := line[x+k]
						b2 := r.resolve(cc.Bg, false, false)
						if cc.Attr&term.AttrInverse != 0 {
							b2 = r.resolve(cc.Fg, true, bold)
						}
						if r.inSelection(abs, x+k) || r.inCurrentMatch(abs, x+k) {
							b2 = r.Style.SelBg
						}
						if b2 != defBg {
							addRect(r.Style.PadX+(x+k)*cw, cw, rgba(b2, 255))
						}
					}
					x += n - 1
					continue
				}
			}
			var k glyphKey
			text := func() string { return c.Text() }
			if c.Attr&term.AttrCluster != 0 {
				k = glyphKey{ch: c.Ch, kind: kindCluster, style: style, cells: uint8(wcells)}
			} else {
				k = glyphKey{ch: c.Ch, kind: kindRune, style: style, cells: uint8(wcells)}
				if IsProcedural(c.Ch) {
					k.style = 0
				}
			}
			if s, ok := r.Cache.get(k, text); ok {
				addGlyph(s, x*cw, py, rgba(fg, 255))
			}
		}
		// decorations
		ulColor := fg
		if c.Extra != 0 {
			if e := term.ExtraOf(c.Extra); e.ULColor != 0 {
				ulColor = r.resolve(e.ULColor, true, false)
			}
		}
		dec := func(kind int, color uint32) {
			k := glyphKey{ch: rune(kind), kind: kindDecoration, cells: uint8(wcells)}
			if s, ok := r.Cache.get(k, nil); ok {
				addGlyph(s, x*cw, py, rgba(color, 255))
			}
		}
		if !hidden {
			switch c.Attr.Underline() {
			case term.ULSingle:
				dec(decUnderline, ulColor)
			case term.ULDouble:
				dec(decDouble, ulColor)
			case term.ULCurly:
				dec(decCurly, ulColor)
			case term.ULDotted:
				dec(decDotted, ulColor)
			case term.ULDashed:
				dec(decDashed, ulColor)
			}
			if c.Attr&term.AttrStrike != 0 {
				dec(decStrike, fg)
			}
			if c.Attr&term.AttrOverline != 0 {
				dec(decOverline, fg)
			}
		}
		if r.view.HoverActive && inMatch(r.view.Hover, abs, x) && c.Attr.Underline() == 0 {
			dec(decUnderline, fg)
		} else if c.Extra != 0 && term.ExtraOf(c.Extra).Link != 0 && c.Attr.Underline() == 0 {
			dec(decLinkUnderline, mix(fg, bg, 0.4))
		}
		if isCursor {
			switch {
			case !r.view.Focused:
				dec(decHollow, r.cursorColor)
			case curStyle == term.CursorBeam:
				dec(decBeam, r.cursorColor)
			case curStyle == term.CursorUnderline:
				dec(decUnderCursor, r.cursorColor)
			}
		}
		x += wcells - 1
	}
}

// plainRun reports whether the n cells from x can be drawn as one ligature:
// a ligature skips the per-cell work, so none of them may have the cursor or
// a line drawn over or under it.
func (r *Renderer) plainRun(line []term.Cell, abs int64, x, n int, cursorHere bool) bool {
	if cursorHere && r.curX >= x && r.curX < x+n {
		return false
	}
	for k := x; k < x+n; k++ {
		c := line[k]
		if c.Attr.Underline() != 0 || c.Attr&(term.AttrStrike|term.AttrOverline) != 0 ||
			c.Extra != 0 && term.ExtraOf(c.Extra).Link != 0 ||
			r.view.HoverActive && inMatch(r.view.Hover, abs, k) {
			return false
		}
	}
	return true
}

func (r *Renderer) ligatureAt(line []term.Cell, x int, style uint8) (int, Sprite) {
	for _, cand := range ligatureCandidates {
		n := len(cand)
		if x+n > len(line) {
			continue
		}
		match := true
		for k := 0; k < n; k++ {
			c := line[x+k]
			if c.Ch != rune(cand[k]) || c.Attr&term.AttrStyleMask != line[x].Attr&term.AttrStyleMask || c.Fg != line[x].Fg {
				match = false
				break
			}
		}
		if !match {
			continue
		}
		id := r.Cache.textID(cand)
		s, ok := r.Cache.get(glyphKey{ch: id, kind: kindText, style: style, cells: uint8(n)}, func() string { return cand })
		if ok {
			return n, s
		}
	}
	return 0, Sprite{}
}

// buildOverlay fills the overlay slots: the IME preedit.
func (r *Renderer) buildOverlay() {
	fs := r.Cache.Fonts
	cw, ch := fs.CellW, fs.CellH
	base := r.rows * r.rectSlot
	ov := r.rects[base:]
	n, ng := 0, 0
	defer func() { r.rowRects[r.rows], r.rowGlyphs[r.rows] = n, ng }()
	add := func(x, y, w, h int, c uint32) {
		if n < len(ov) {
			ov[n] = rectInst{int16(x), int16(y), int16(w), int16(h), c}
			n++
		}
	}
	gbase := r.rows * r.glyphSlot
	gov := r.glyphs[gbase:]
	if r.view.Preedit != "" && r.curY >= 0 {
		py := r.Style.PadY + r.curY*ch
		x := r.curX
		w := term.StringWidth(r.view.Preedit)
		add(r.Style.PadX+x*cw, py, w*cw, ch, rgba(r.defBg, 255))
		for _, g := range graphemes(r.view.Preedit) {
			gw := max(1, term.StringWidth(g))
			if x+gw > r.cols {
				break
			}
			rs := []rune(g)
			k := glyphKey{ch: rs[0], kind: kindRune, cells: uint8(gw)}
			if len(rs) > 1 {
				k = glyphKey{ch: r.Cache.textID(g), kind: kindText, cells: uint8(gw)}
			}
			if s, ok := r.Cache.get(k, func() string { return g }); ok && ng < len(gov) {
				gov[ng] = glyphInst{X: int16(r.Style.PadX + x*cw + int(s.OffX)), Y: int16(py + int(s.OffY)), W: s.W, H: s.H, U: s.X, V: s.Y, Color: rgba(r.defFg, 255)}
				if s.Color {
					gov[ng].Flags = flagColorGlyph
				}
				ng++
			}
			if s, ok := r.Cache.get(glyphKey{ch: decUnderline, kind: kindDecoration, cells: uint8(gw)}, nil); ok && ng < len(gov) {
				gov[ng] = glyphInst{X: int16(r.Style.PadX + x*cw + int(s.OffX)), Y: int16(py + int(s.OffY)), W: s.W, H: s.H, U: s.X, V: s.Y, Color: rgba(r.cursorColor, 255)}
				ng++
			}
			x += gw
		}
	}
}

func graphemes(s string) []string {
	var out []string
	for _, r := range s {
		if term.RuneWidth(r) == 0 && len(out) > 0 {
			out[len(out)-1] += string(r)
		} else {
			out = append(out, string(r))
		}
	}
	return out
}

// Render draws a frame. The widget's context must be current and its
// framebuffer bound.
func (r *Renderer) Render() {
	if !r.glReady || r.cols == 0 {
		return
	}
	gen := r.Cache.Gen
	for pass := 0; pass < 2; pass++ {
		for y := 0; y < r.rows; y++ {
			if r.allDirty || r.rowDirty[y] {
				r.buildRow(y)
			}
		}
		// In the same pass: the preedit's glyphs can reset the atlas too.
		r.buildOverlay()
		if r.Cache.Gen == gen {
			break
		}
		// The atlas was reset while building: rebuild everything once.
		gen = r.Cache.Gen
		r.cacheGen = gen
		r.allDirty = true
	}
	r.upload()
	clear(r.rowDirty)
	r.allDirty = false

	bg := r.defBg
	if r.reverse {
		bg = r.defFg
	}
	gl.Viewport(0, 0, int32(r.fbW), int32(r.fbH))
	gl.Disable(gl.DEPTH_TEST)
	gl.Disable(gl.SCISSOR_TEST)
	gl.ClearColor(float32(bg>>16&0xff)/255, float32(bg>>8&0xff)/255, float32(bg&0xff)/255, 1)
	gl.Clear(gl.COLOR_BUFFER_BIT)
	gl.Enable(gl.BLEND)
	gl.BlendFunc(gl.ONE, gl.ONE_MINUS_SRC_ALPHA)

	gl.UseProgram(shared.progRect)
	gl.Uniform2f(shared.uVPRect, float32(r.fbW), float32(r.fbH))
	gl.BindVertexArray(r.vaoRect)
	gl.DrawArraysInstanced(gl.TRIANGLE_STRIP, 0, 4, int32(len(r.packedRects)))

	tex, size := r.Cache.Texture()
	gl.UseProgram(shared.progGlyph)
	gl.Uniform2f(shared.uVPGlyph, float32(r.fbW), float32(r.fbH))
	gl.Uniform2f(shared.uAtlasSize, float32(size), float32(size))
	gl.ActiveTexture(gl.TEXTURE0)
	gl.BindTexture(gl.TEXTURE_2D, tex)
	gl.Uniform1i(shared.uAtlas, 0)
	gl.BindVertexArray(r.vaoGlyph)
	gl.DrawArraysInstanced(gl.TRIANGLE_STRIP, 0, 4, int32(len(r.packedGlyphs)))
	r.Drawn = len(r.packedRects) + len(r.packedGlyphs)
	gl.BindVertexArray(0)
	gl.UseProgram(0)
}

// upload packs the instances in use, row by row and then the overlay, and
// sends them to the GPU. Only real instances are copied and drawn: a row
// uses a fraction of its slot, and empty slots would otherwise cost the
// GPU (and above all a software renderer) as much as full ones.
func (r *Renderer) upload() {
	r.packedRects = r.packedRects[:0]
	r.packedGlyphs = r.packedGlyphs[:0]
	for y := 0; y <= r.rows; y++ { // the overlay is row r.rows
		k := y * r.rectSlot
		r.packedRects = append(r.packedRects, r.rects[k:k+r.rowRects[y]]...)
		k = y * r.glyphSlot
		r.packedGlyphs = append(r.packedGlyphs, r.glyphs[k:k+r.rowGlyphs[y]]...)
	}
	gl.BindBuffer(gl.ARRAY_BUFFER, r.rectVBO)
	r.gpuRects = uploadInstances(r.packedRects, r.gpuRects, cap(r.packedRects))
	gl.BindBuffer(gl.ARRAY_BUFFER, r.glyVBO)
	r.gpuGlyphs = uploadInstances(r.packedGlyphs, r.gpuGlyphs, cap(r.packedGlyphs))
	gl.BindBuffer(gl.ARRAY_BUFFER, 0)
}

// uploadInstances writes data to the bound buffer, which holds gpuCap
// instances, growing it to want instances when data does not fit. It
// returns the buffer's capacity.
func uploadInstances[T any](data []T, gpuCap, want int) int {
	if len(data) == 0 {
		return gpuCap
	}
	size := int(unsafe.Sizeof(data[0]))
	if len(data) > gpuCap {
		gl.BufferData(gl.ARRAY_BUFFER, want*size, nil, gl.DYNAMIC_DRAW)
		gpuCap = want
	}
	gl.BufferSubData(gl.ARRAY_BUFFER, 0, len(data)*size, gl.Ptr(data))
	return gpuCap
}
