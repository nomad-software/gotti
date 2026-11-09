package render

import "math"

// Procedurally drawn characters: box drawing, block elements, braille,
// powerline separators and DEC scan lines. Drawing them ourselves (rather
// than using font glyphs) makes adjacent cells join seamlessly.

// box line weights per direction: up, right, down, left.
// 0 none, 1 light, 2 heavy, 3 double.
var boxLines = map[rune][4]uint8{}

func init() {
	// Weights up, right, down, left for U+2500..U+257F, four digits per
	// character; "0000" marks one drawn some other way (arcs, diagonals).
	const table = "" +
		"0101020210102020010102021010202001010202101020200110021001200220" + // 2500
		"0011001200210022110012002100220010011002200120021110121021101120" + // 2510
		"2120221012202220101110122011102120212012102220220111011202110212" + // 2520
		"0121012202210222110111021201120221012102220122021111111212111212" + // 2530
		"2111112121212112221111221221221212222122222122220101020210102020" + // 2540
		"0303303003100130033000130031003313003100330010033001300313103130" + // 2550
		"3330101330313033031301310333130331013303131331313333000000000000" + // 2560
		"0000000000000000000110000100001000022000020000200201102001022010" // 2570
	for i := 0; i < 128; i++ {
		s := table[i*4 : i*4+4]
		if s == "0000" {
			continue
		}
		boxLines[0x2500+rune(i)] = [4]uint8{s[0] - '0', s[1] - '0', s[2] - '0', s[3] - '0'}
	}
}

// dash counts for the dashed line characters.
var boxDashes = map[rune]int{
	0x2504: 3, 0x2505: 3, 0x2506: 3, 0x2507: 3,
	0x2508: 4, 0x2509: 4, 0x250a: 4, 0x250b: 4,
	0x254c: 2, 0x254d: 2, 0x254e: 2, 0x254f: 2,
}

// IsProcedural reports whether r is drawn by DrawProcedural.
func IsProcedural(r rune) bool {
	switch {
	case r >= 0x2500 && r <= 0x259f:
		return true
	case r >= 0x2800 && r <= 0x28ff:
		return true
	case r >= 0xe0b0 && r <= 0xe0b3:
		return true
	case r >= 0x23ba && r <= 0x23bd:
		return true
	}
	return false
}

// mask is an alpha bitmap.
type mask struct {
	w, h int
	a    []byte
}

func newMask(w, h int) *mask { return &mask{w: w, h: h, a: make([]byte, w*h)} }

func (m *mask) rect(x0, y0, x1, y1 int, v byte) {
	x0, y0 = max(x0, 0), max(y0, 0)
	x1, y1 = min(x1, m.w), min(y1, m.h)
	for y := y0; y < y1; y++ {
		row := m.a[y*m.w : (y+1)*m.w]
		for x := x0; x < x1; x++ {
			if row[x] < v {
				row[x] = v
			}
		}
	}
}

// coverage fills pixels by 4×4 supersampling an inside test.
func (m *mask) coverage(inside func(x, y float64) bool) {
	const n = 4
	for y := 0; y < m.h; y++ {
		for x := 0; x < m.w; x++ {
			c := 0
			for sy := 0; sy < n; sy++ {
				for sx := 0; sx < n; sx++ {
					if inside(float64(x)+(float64(sx)+0.5)/n, float64(y)+(float64(sy)+0.5)/n) {
						c++
					}
				}
			}
			if c > 0 {
				v := byte(c * 255 / (n * n))
				if i := y*m.w + x; m.a[i] < v {
					m.a[i] = v
				}
			}
		}
	}
}

// Metrics needed for procedural drawing.
type boxMetrics struct {
	w, h  int
	light int // light line thickness
	heavy int
}

func newBoxMetrics(w, h int) boxMetrics {
	light := max(1, int(math.Round(float64(w)/9)))
	return boxMetrics{w: w, h: h, light: light, heavy: light * 2}
}

func (bm boxMetrics) thick(weight uint8) int {
	switch weight {
	case 1, 3:
		return bm.light
	case 2:
		return bm.heavy
	}
	return 0
}

// DrawProcedural rasterizes r into a w×h alpha mask.
func DrawProcedural(r rune, w, h int) ([]byte, bool) {
	if !IsProcedural(r) {
		return nil, false
	}
	m := newMask(w, h)
	bm := newBoxMetrics(w, h)
	switch {
	case r >= 0x2580 && r <= 0x259f:
		drawBlock(m, r)
	case r >= 0x2800 && r <= 0x28ff:
		drawBraille(m, r)
	case r >= 0xe0b0 && r <= 0xe0b3:
		drawPowerline(m, r, bm)
	case r >= 0x23ba && r <= 0x23bd:
		frac := [...]float64{0.1, 0.3, 0.7, 0.9}[r-0x23ba]
		y := int(float64(h) * frac)
		m.rect(0, y-bm.light/2, w, y-bm.light/2+bm.light, 255)
	case r >= 0x256d && r <= 0x2570:
		drawArc(m, r, bm)
	case r >= 0x2571 && r <= 0x2573:
		drawDiagonal(m, r, bm)
	default:
		lines, ok := boxLines[r]
		if !ok {
			return nil, false
		}
		drawLines(m, lines, boxDashes[r], bm)
	}
	return m.a, true
}

func drawLines(m *mask, l [4]uint8, dashes int, bm boxMetrics) {
	u, rt, d, lf := l[0], l[1], l[2], l[3]
	w, h := m.w, m.h
	cx, cy := w/2, h/2
	off := bm.light // distance of double lines from the centre line
	if dashes > 0 {
		drawDashes(m, l, dashes, bm)
		return
	}
	hline := func(x0, x1, yc, t int) { m.rect(x0, yc-t/2, x1, yc-t/2+t, 255) }
	vline := func(y0, y1, xc, t int) { m.rect(xc-t/2, y0, xc-t/2+t, y1, 255) }
	maxT := func(a, b uint8) int { return max(bm.thick(a), bm.thick(b)) }

	// singleStart: where a single/heavy segment heading away from the
	// centre begins, given the two perpendicular weights.
	singleStart := func(pa, pb uint8, own int) int {
		switch {
		case pa == 3 && pb == 3:
			return -off // stop at the near double line
		case pa == 3 || pb == 3:
			return off + bm.light/2 // corner: reach the far double line
		case pa > 0 || pb > 0:
			return (maxT(pa, pb) + 1) / 2
		}
		return (own + 1) / 2
	}
	// doubleStart: start offset for the double line on side "same".
	doubleStart := func(same, other uint8) int {
		switch {
		case same == 3:
			return -off
		case same > 0:
			return (bm.thick(same) + 1) / 2
		case other == 3:
			return off + bm.light/2
		case other > 0:
			return (bm.thick(other) + 1) / 2
		}
		return off
	}

	// horizontal arms
	for _, arm := range []struct {
		weight uint8
		dir    int // +1 right, -1 left
	}{{rt, 1}, {lf, -1}} {
		if arm.weight == 0 {
			continue
		}
		seg := func(start int, yc, t int) {
			if arm.dir > 0 {
				hline(cx-start, w, yc, t)
			} else {
				hline(0, cx+start+(w&1), yc, t)
			}
		}
		if arm.weight == 3 {
			seg(doubleStart(u, d), cy-off, bm.light)
			seg(doubleStart(d, u), cy+off, bm.light)
		} else {
			t := bm.thick(arm.weight)
			seg(singleStart(u, d, t), cy, t)
		}
	}
	// vertical arms
	for _, arm := range []struct {
		weight uint8
		dir    int // +1 down, -1 up
	}{{d, 1}, {u, -1}} {
		if arm.weight == 0 {
			continue
		}
		seg := func(start int, xc, t int) {
			if arm.dir > 0 {
				vline(cy-start, h, xc, t)
			} else {
				vline(0, cy+start+(h&1), xc, t)
			}
		}
		if arm.weight == 3 {
			seg(doubleStart(lf, rt), cx-off, bm.light)
			seg(doubleStart(rt, lf), cx+off, bm.light)
		} else {
			t := bm.thick(arm.weight)
			seg(singleStart(lf, rt, t), cx, t)
		}
	}
}

func drawDashes(m *mask, l [4]uint8, n int, bm boxMetrics) {
	w, h := m.w, m.h
	if l[1] > 0 { // horizontal
		t := bm.thick(l[1])
		seg := float64(w) / float64(n)
		for i := 0; i < n; i++ {
			x0 := int(math.Round(float64(i) * seg))
			x1 := int(math.Round(float64(i)*seg + seg*0.6))
			m.rect(x0, h/2-t/2, max(x1, x0+1), h/2-t/2+t, 255)
		}
		return
	}
	t := bm.thick(l[0])
	seg := float64(h) / float64(n)
	for i := 0; i < n; i++ {
		y0 := int(math.Round(float64(i) * seg))
		y1 := int(math.Round(float64(i)*seg + seg*0.6))
		m.rect(w/2-t/2, y0, w/2-t/2+t, max(y1, y0+1), 255)
	}
}

func drawArc(m *mask, r rune, bm boxMetrics) {
	w, h := float64(m.w), float64(m.h)
	t := float64(bm.light)
	// centre lines of the vertical and horizontal strokes
	lx := float64(m.w/2-bm.light/2) + t/2
	ly := float64(m.h/2-bm.light/2) + t/2
	// direction to the arc's centre: ╭ (+,+) ╮ (-,+) ╯ (-,-) ╰ (+,-)
	dir := [...][2]float64{{1, 1}, {-1, 1}, {-1, -1}, {1, -1}}[r-0x256d]
	sx, sy := dir[0], dir[1]
	rad := math.Min(w-lx, lx)
	rad = math.Min(rad, math.Min(h-ly, ly))
	ccx, ccy := lx+sx*rad, ly+sy*rad
	m.coverage(func(x, y float64) bool {
		// arc quadrant
		dx, dy := x-ccx, y-ccy
		if dx*sx <= 0 && dy*sy <= 0 {
			d := math.Hypot(dx, dy)
			if math.Abs(d-rad) <= t/2 {
				return true
			}
		}
		// straight vertical part from the arc end to the cell edge
		if math.Abs(x-lx) <= t/2 {
			if sy > 0 && y >= ccy && y <= h || sy < 0 && y <= ccy && y >= 0 {
				return true
			}
		}
		// straight horizontal part
		if math.Abs(y-ly) <= t/2 {
			if sx > 0 && x >= ccx && x <= w || sx < 0 && x <= ccx && x >= 0 {
				return true
			}
		}
		return false
	})
}

func drawDiagonal(m *mask, r rune, bm boxMetrics) {
	w, h := float64(m.w), float64(m.h)
	t := float64(bm.light) * 0.9
	dist := func(x, y, x0, y0, x1, y1 float64) float64 {
		dx, dy := x1-x0, y1-y0
		return math.Abs(dy*x-dx*y+x1*y0-y1*x0) / math.Hypot(dx, dy)
	}
	m.coverage(func(x, y float64) bool {
		if (r == 0x2571 || r == 0x2573) && dist(x, y, w, 0, 0, h) <= t/2 {
			return true
		}
		if (r == 0x2572 || r == 0x2573) && dist(x, y, 0, 0, w, h) <= t/2 {
			return true
		}
		return false
	})
}

func drawBlock(m *mask, r rune) {
	w, h := m.w, m.h
	eighthY := func(n int) int { return int(math.Round(float64(h) * float64(n) / 8)) }
	eighthX := func(n int) int { return int(math.Round(float64(w) * float64(n) / 8)) }
	hw, hh := w/2, h/2
	quad := func(ul, ur, ll, lr bool) {
		if ul {
			m.rect(0, 0, hw, hh, 255)
		}
		if ur {
			m.rect(hw, 0, w, hh, 255)
		}
		if ll {
			m.rect(0, hh, hw, h, 255)
		}
		if lr {
			m.rect(hw, hh, w, h, 255)
		}
	}
	switch {
	case r == 0x2580:
		m.rect(0, 0, w, hh, 255)
	case r >= 0x2581 && r <= 0x2588:
		m.rect(0, h-eighthY(int(r-0x2580)), w, h, 255)
	case r >= 0x2589 && r <= 0x258f:
		m.rect(0, 0, eighthX(int(0x2590-r)), h, 255)
	case r == 0x2590:
		m.rect(hw, 0, w, h, 255)
	case r == 0x2591:
		m.rect(0, 0, w, h, 64)
	case r == 0x2592:
		m.rect(0, 0, w, h, 128)
	case r == 0x2593:
		m.rect(0, 0, w, h, 192)
	case r == 0x2594:
		m.rect(0, 0, w, eighthY(1), 255)
	case r == 0x2595:
		m.rect(w-eighthX(1), 0, w, h, 255)
	case r == 0x2596:
		quad(false, false, true, false)
	case r == 0x2597:
		quad(false, false, false, true)
	case r == 0x2598:
		quad(true, false, false, false)
	case r == 0x2599:
		quad(true, false, true, true)
	case r == 0x259a:
		quad(true, false, false, true)
	case r == 0x259b:
		quad(true, true, true, false)
	case r == 0x259c:
		quad(true, true, false, true)
	case r == 0x259d:
		quad(false, true, false, false)
	case r == 0x259e:
		quad(false, true, true, false)
	case r == 0x259f:
		quad(false, true, true, true)
	}
}

func drawBraille(m *mask, r rune) {
	bits := int(r - 0x2800)
	// dot (col,row) for bits 0..7
	pos := [8][2]int{{0, 0}, {0, 1}, {0, 2}, {1, 0}, {1, 1}, {1, 2}, {0, 3}, {1, 3}}
	cw, ch := float64(m.w)/2, float64(m.h)/4
	rad := math.Min(cw, ch) * 0.32
	m.coverage(func(x, y float64) bool {
		for i, p := range pos {
			if bits&(1<<i) == 0 {
				continue
			}
			cx := (float64(p[0]) + 0.5) * cw
			cy := (float64(p[1]) + 0.5) * ch
			if math.Hypot(x-cx, y-cy) <= rad {
				return true
			}
		}
		return false
	})
}

func drawPowerline(m *mask, r rune, bm boxMetrics) {
	w, h := float64(m.w), float64(m.h)
	t := float64(bm.light)
	switch r {
	case 0xe0b0: // solid right-pointing triangle
		m.coverage(func(x, y float64) bool { return x <= w*(1-math.Abs(y-h/2)/(h/2)) })
	case 0xe0b2: // solid left-pointing triangle
		m.coverage(func(x, y float64) bool { return w-x <= w*(1-math.Abs(y-h/2)/(h/2)) })
	case 0xe0b1, 0xe0b3: // chevrons
		m.coverage(func(x, y float64) bool {
			edge := w * (1 - math.Abs(y-h/2)/(h/2))
			if r == 0xe0b3 {
				x = w - x
			}
			return math.Abs(x-edge) <= t*0.75
		})
	}
}

// ---------------------------------------------------------------- decorations

// Decoration sprites, drawn into cell-sized masks.
const (
	decUnderline = iota + 1
	decDouble
	decCurly
	decDotted
	decDashed
	decStrike
	decOverline
	decBeam
	decUnderCursor
	decHollow
	decLinkUnderline
)

// drawDecoration renders decoration kind into a w×h mask. ulPos is the
// underline's top offset from the cell top; thick its thickness;
// strikePos the strikethrough position.
func drawDecoration(kind, w, h, ulPos, thick, strikePos int) []byte {
	m := newMask(w, h)
	thick = max(1, thick)
	switch kind {
	case decUnderline:
		m.rect(0, ulPos, w, ulPos+thick, 255)
	case decLinkUnderline:
		for x := 0; x < w; x += 2 {
			m.rect(x, ulPos, x+1, ulPos+thick, 255)
		}
	case decDouble:
		y := min(ulPos, h-3*thick)
		m.rect(0, y, w, y+thick, 255)
		m.rect(0, y+2*thick, w, y+3*thick, 255)
	case decCurly:
		amp := math.Max(1, float64(min(h-ulPos, h/6))*0.5+float64(thick)/2)
		mid := float64(ulPos) + float64(thick)/2
		if mid+amp > float64(h) {
			mid = float64(h) - amp
		}
		t := float64(thick) * 0.6
		m.coverage(func(x, y float64) bool {
			c := mid + amp*math.Sin(x/float64(w)*2*math.Pi)
			return math.Abs(y-c) <= t+0.35
		})
	case decDotted:
		step := max(2, thick*2)
		for x := 0; x < w; x += step {
			m.rect(x, ulPos, x+thick, ulPos+thick, 255)
		}
	case decDashed:
		dl := max(2, w/3)
		m.rect(0, ulPos, dl, ulPos+thick, 255)
		m.rect(w-dl, ulPos, w, ulPos+thick, 255)
	case decStrike:
		m.rect(0, strikePos, w, strikePos+thick, 255)
	case decOverline:
		m.rect(0, 0, w, thick, 255)
	case decBeam:
		bw := max(2, thick*2)
		m.rect(0, 0, bw, h, 255)
	case decUnderCursor:
		bh := max(2, thick*2)
		m.rect(0, h-bh, w, h, 255)
	case decHollow:
		t := max(1, thick)
		m.rect(0, 0, w, t, 255)
		m.rect(0, h-t, w, h, 255)
		m.rect(0, 0, t, h, 255)
		m.rect(w-t, 0, w, h, 255)
	}
	return m.a
}
