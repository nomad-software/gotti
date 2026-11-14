package term

// Resize returns a new buffer of the given size. When reflow is set the
// soft-wrapped lines are re-joined and re-split at the new width; the
// cursor (cx, cy in screen coordinates) is tracked through the reflow and
// its new position returned. pending is a wrap pending at the cursor (it
// follows a character in the last column, at cols-1); the result says
// whether one still is, so the next character does not overwrite the last.
//
// Reflow runs twice: a first pass only measures the result (how many
// lines, where the cursor and the last content land), then a second pass
// writes the lines that are kept straight into the new ring. Nothing else
// is allocated, which matters because a window being resized reflows every
// pane on each step.
func (b *Buffer) Resize(cols, rows int, reflow bool, cx, cy int, pending bool) (*Buffer, int, int, bool) {
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	if pending {
		cx++ // the cursor really follows the character it sits on
	}
	if !reflow {
		return b.resizeClip(cols, rows, cx, cy, pending)
	}
	nb := &Buffer{Cols: cols, Rows: rows, maxLines: rows + b.maxLines - b.Rows, Evicted: b.Evicted}

	st := b.reflow(cols, cx, cy, 0, 0, nil, nil)
	// Drop empty lines below both the cursor and the last content line.
	n := max(st.curLine, st.last) + 1
	curLine := st.curLine
	if curLine < 0 {
		curLine = n - 1
	}
	// Keep the cursor on screen even if content below it overflows.
	if n-curLine > rows {
		n = curLine + rows
	}
	total := max(n, rows) // short output is padded with blank lines
	first := max(0, total-nb.maxLines)

	nb.cells = make([]Cell, nb.maxLines*cols)
	nb.wrapped = make([]bool, nb.maxLines)
	b.reflow(cols, cx, cy, first, n, nb.cells, nb.wrapped)
	nb.Evicted += int64(first)
	nb.count = total - first
	nb.dirty = make([]bool, rows)
	nb.allDirty = true

	ncy := clamp(curLine-(total-rows), 0, rows-1)
	// A cursor past the last column follows a character there: a wrap is
	// pending.
	return nb, min(st.curX, cols-1), ncy, st.curX >= cols
}

// reflowStats describes the output of a reflow pass.
type reflowStats struct {
	lines   int // output lines
	curLine int // output line of the cursor, or -1
	curX    int
	last    int // last output line with content or a soft wrap, or -1
}

// reflow re-splits b's paragraphs (logical lines joined by soft wraps) at
// width cols. Output lines lo to hi-1 are written to dst and dstWrap,
// starting at row 0; the others are only counted, so a pass with lo == hi
// measures the result without writing anything. dst must be zeroed.
func (b *Buffer) reflow(cols, cx, cy, lo, hi int, dst []Cell, dstWrap []bool) reflowStats {
	st := reflowStats{curLine: -1, last: -1}
	cursorLogical := b.count - b.Rows + cy

	var line []Cell // the output line being built, nil when not kept
	lx := 0
	content := false
	begin := func() {
		line, lx, content = nil, 0, false
		if st.lines >= lo && st.lines < hi {
			k := (st.lines - lo) * cols
			line = dst[k : k+cols : k+cols]
		}
	}
	emit := func(wrapped bool) {
		if line != nil {
			dstWrap[st.lines-lo] = wrapped
		}
		if wrapped || content {
			st.last = st.lines
		}
		st.lines++
		begin()
	}
	begin()

	var para []Cell
	for i := 0; i < b.count; {
		// Collect one paragraph.
		para = para[:0]
		cursorOff := -1
		j := i
		for {
			l := b.LogicalLine(j)
			wrapped := b.Wrapped(j)
			n := len(l)
			if !wrapped {
				n = contentLen(l)
			}
			if j == cursorLogical {
				cursorOff = len(para) + cx
				if cursorOff > len(para)+n {
					// cursor beyond content: pad the paragraph up to it
					n = min(cx, len(l))
				}
			}
			para = append(para, l[:n]...)
			j++
			if !wrapped || j >= b.count {
				break
			}
		}
		i = j
		// Right halves and pads are skipped below, so a cursor on one is
		// placed by its head (one column right of it), or by what follows
		// the pad.
		rightHalf := false
		if cursorOff >= 0 && cursorOff < len(para) {
			switch a := para[cursorOff].Attr; {
			case a&AttrWideCont != 0 && cursorOff > 0:
				cursorOff--
				rightHalf = true
			case a&AttrWrapPad != 0:
				cursorOff++
			}
		}
		// Re-split it at the new width.
		for k := range para {
			c := para[k]
			if c.Attr&(AttrWideCont|AttrWrapPad) != 0 {
				continue // re-generated after its head / layout only
			}
			w := 1
			if c.Attr&AttrWide != 0 {
				w = 2
			}
			if lx+w > cols {
				if line != nil && lx < cols {
					line[lx].Attr |= AttrWrapPad // the wide char wraps
				}
				emit(true)
			}
			if k == cursorOff {
				st.curLine, st.curX = st.lines, lx
			}
			if w == 2 && cols < 2 {
				c.Attr &^= AttrWide
				w = 1
			}
			if line != nil {
				line[lx] = c
				if w == 2 {
					line[lx+1] = Cell{Fg: c.Fg, Bg: c.Bg, Attr: AttrWideCont | c.Attr&AttrStyleMask}
				}
			}
			content = content || isContent(&c)
			lx += w
			if k+1 == cursorOff && k == len(para)-1 {
				// cursor right after the last character, cols when that
				// is in the last column (a pending wrap)
				st.curLine, st.curX = st.lines, lx
			}
		}
		if cursorOff >= 0 && st.curLine < 0 {
			st.curLine, st.curX = st.lines, min(lx, cols-1)
		}
		if rightHalf {
			st.curX = min(st.curX+1, cols-1)
		}
		emit(false)
	}
	return st
}

// isContent reports whether a cell is more than a blank: trailing cells
// that are not content are trimmed from hard-wrapped lines.
func isContent(c *Cell) bool {
	return c.Ch != 0 || !c.Bg.IsDefault() || c.Attr&^(AttrWideCont|AttrWrapPad) != 0 || c.Extra != 0
}

// contentLen returns the length of l without trailing blank cells.
func contentLen(l []Cell) int {
	n := len(l)
	for n > 0 && !isContent(&l[n-1]) {
		n--
	}
	return n
}

// resizeClip resizes without reflow (used for the alternate screen). With
// pending, cx is one past the character the cursor follows.
func (b *Buffer) resizeClip(cols, rows, cx, cy int, pending bool) (*Buffer, int, int, bool) {
	nb := NewBuffer(cols, rows, b.maxLines-b.Rows)
	nb.Evicted = b.Evicted
	// Keep the cursor row on screen: drop rows from the top if needed.
	shift := 0
	if cy >= rows {
		shift = cy - rows + 1
	}
	for y := 0; y < rows && y+shift < b.Rows; y++ {
		src := b.Line(y + shift)
		dst := nb.Line(y)
		n := copy(dst, src)
		if n > 0 && n < len(src) && dst[n-1].Attr&AttrWide != 0 {
			dst[n-1] = Cell{}
		}
	}
	cy -= shift
	if !pending {
		return nb, min(cx, cols-1), cy, false
	}
	// The character it follows is still in the last column, or was clipped
	// off; otherwise the cursor moves on to after it.
	if cx >= cols {
		return nb, cols - 1, cy, true
	}
	return nb, cx, cy, false
}
