package term

// Buffer is a screen plus its scrollback, stored as one flat []Cell used
// as a ring of lines. Scrolling the full screen into scrollback only moves
// the ring's start index; no cells are copied.
//
// Logical line 0 is the oldest scrollback line; the visible screen is the
// last Rows logical lines.
type Buffer struct {
	Cols, Rows int
	maxLines   int // Rows + scrollback capacity

	cells   []Cell // alloc*Cols cells
	wrapped []bool // per physical line: soft-wrapped into the next line
	start   int    // physical index of logical line 0
	count   int    // logical lines in use, always >= Rows

	// Evicted counts lines that have fallen off the top of the ring, so
	// absolute line numbers (Evicted + logical index) stay stable.
	Evicted int64

	dirty    []bool // per screen row
	allDirty bool
}

// NewBuffer allocates a buffer with the given scrollback capacity.
func NewBuffer(cols, rows, scrollback int) *Buffer {
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	b := &Buffer{Cols: cols, Rows: rows, maxLines: rows + scrollback}
	// The whole ring is allocated up front. Large allocations are backed
	// by untouched zero pages, so memory only becomes resident as lines
	// are written, and the ring never has to grow or copy.
	b.cells = make([]Cell, b.maxLines*cols)
	b.wrapped = make([]bool, b.maxLines)
	b.count = rows
	b.dirty = make([]bool, rows)
	b.allDirty = true
	return b
}

// ScrollbackLen returns the number of lines above the screen.
func (b *Buffer) ScrollbackLen() int { return b.count - b.Rows }

// Count returns the number of logical lines (scrollback + screen).
func (b *Buffer) Count() int { return b.count }

func (b *Buffer) phys(i int) int {
	p := b.start + i
	if n := len(b.wrapped); p >= n {
		p -= n
	}
	return p
}

// LogicalLine returns logical line i (0 = oldest).
func (b *Buffer) LogicalLine(i int) []Cell {
	p := b.phys(i) * b.Cols
	return b.cells[p : p+b.Cols : p+b.Cols]
}

// Line returns screen row y.
func (b *Buffer) Line(y int) []Cell {
	return b.LogicalLine(b.count - b.Rows + y)
}

// Wrapped reports whether logical line i soft-wraps into i+1.
func (b *Buffer) Wrapped(i int) bool { return b.wrapped[b.phys(i)] }

// SetWrapped sets the soft-wrap flag of screen row y.
func (b *Buffer) SetWrapped(y int, w bool) {
	b.wrapped[b.phys(b.count-b.Rows+y)] = w
}

// RowWrapped reports the soft-wrap flag of screen row y.
func (b *Buffer) RowWrapped(y int) bool {
	return b.wrapped[b.phys(b.count-b.Rows+y)]
}

// MarkAllDirty flags every row.
func (b *Buffer) MarkAllDirty() { b.allDirty = true }

// IsDirty reports whether screen row y changed since ClearDirty.
func (b *Buffer) IsDirty(y int) bool { return b.allDirty || b.dirty[y] }

// ClearDirty resets all damage.
func (b *Buffer) ClearDirty() {
	b.allDirty = false
	clear(b.dirty)
}

// AnyDirty reports whether anything changed since ClearDirty.
func (b *Buffer) AnyDirty() bool {
	if b.allDirty {
		return true
	}
	for _, d := range b.dirty {
		if d {
			return true
		}
	}
	return false
}

func fillCells(line []Cell, fill Cell) {
	if fill == (Cell{}) {
		clear(line) // compiles to memclr
		return
	}
	for i := range line {
		line[i] = fill
	}
}

// pushLine appends a new blank logical line at the bottom, moving the top
// screen line into scrollback (or evicting the oldest line when full).
func (b *Buffer) pushLine(fill Cell) {
	if b.count < b.maxLines {
		b.count++
	} else {
		b.start++
		if b.start == len(b.wrapped) {
			b.start = 0
		}
		b.Evicted++
	}
	y := b.Rows - 1
	fillCells(b.Line(y), fill)
	b.SetWrapped(y, false)
}

// ScrollUp scrolls rows [top,bottom] up by n, filling new rows with fill.
// When the region is the whole screen and toScrollback is set, lines move
// into the scrollback ring without copying.
func (b *Buffer) ScrollUp(n, top, bottom int, fill Cell, toScrollback bool) {
	h := bottom - top + 1
	if n <= 0 || h <= 0 {
		return
	}
	if n > h {
		n = h
	}
	if top == 0 && bottom == b.Rows-1 && toScrollback {
		for i := 0; i < n; i++ {
			b.pushLine(fill)
		}
		b.allDirty = true
		return
	}
	for y := top; y+n <= bottom; y++ {
		copy(b.Line(y), b.Line(y+n))
		b.SetWrapped(y, b.RowWrapped(y+n))
		b.dirty[y] = true
	}
	for y := bottom - n + 1; y <= bottom; y++ {
		fillCells(b.Line(y), fill)
		b.SetWrapped(y, false)
		b.dirty[y] = true
	}
}

// ScrollDown scrolls rows [top,bottom] down by n.
func (b *Buffer) ScrollDown(n, top, bottom int, fill Cell) {
	h := bottom - top + 1
	if n <= 0 || h <= 0 {
		return
	}
	if n > h {
		n = h
	}
	for y := bottom; y-n >= top; y-- {
		copy(b.Line(y), b.Line(y-n))
		b.SetWrapped(y, b.RowWrapped(y-n))
		b.dirty[y] = true
	}
	for y := top; y < top+n; y++ {
		fillCells(b.Line(y), fill)
		b.SetWrapped(y, false)
		b.dirty[y] = true
	}
}

// ClearScrollback drops all scrollback lines.
func (b *Buffer) ClearScrollback() {
	sb := b.ScrollbackLen()
	if sb == 0 {
		return
	}
	// Move the screen to the start of the ring.
	tmp := make([]Cell, b.Rows*b.Cols)
	tw := make([]bool, b.Rows)
	for y := 0; y < b.Rows; y++ {
		copy(tmp[y*b.Cols:], b.Line(y))
		tw[y] = b.RowWrapped(y)
	}
	b.start, b.count = 0, b.Rows
	b.Evicted += int64(sb)
	for y := 0; y < b.Rows; y++ {
		copy(b.Line(y), tmp[y*b.Cols:(y+1)*b.Cols])
		b.SetWrapped(y, tw[y])
	}
	b.allDirty = true
}
