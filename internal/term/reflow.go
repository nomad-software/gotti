package term

// Resize returns a new buffer of the given size. When reflow is set the
// soft-wrapped lines are re-joined and re-split at the new width; the
// cursor (cx, cy in screen coordinates) is tracked through the reflow and
// its new position returned.
func (b *Buffer) Resize(cols, rows int, reflow bool, cx, cy int) (*Buffer, int, int) {
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	if !reflow {
		return b.resizeClip(cols, rows, cx, cy)
	}
	scrollback := b.maxLines - b.Rows
	nb := &Buffer{Cols: cols, Rows: rows, maxLines: rows + scrollback, Evicted: b.Evicted}

	var out []Cell
	var outWrap []bool
	curLine, curX := -1, 0
	cursorLogical := b.count - b.Rows + cy

	line := make([]Cell, cols)
	lx := 0
	emit := func(wrapped bool) {
		out = append(out, line...)
		outWrap = append(outWrap, wrapped)
		fillCells(line, Cell{})
		lx = 0
	}
	var para []Cell
	for i := 0; i < b.count; {
		// Collect one paragraph (lines joined by soft wraps).
		para = para[:0]
		cursorOff := -1
		j := i
		for {
			l := b.LogicalLine(j)
			wrapped := b.wrapped[b.phys(j)]
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
		// Re-split the paragraph at the new width.
		startLine := len(outWrap)
		for k := 0; k < len(para); k++ {
			c := para[k]
			if c.Attr&AttrWideCont != 0 {
				continue // re-generated after its head
			}
			w := 1
			if c.Attr&AttrWide != 0 {
				w = 2
			}
			if lx+w > cols {
				emit(true)
			}
			if k == cursorOff {
				curLine, curX = len(outWrap), lx
			}
			line[lx] = c
			if w == 2 && cols >= 2 {
				line[lx+1] = Cell{Fg: c.Fg, Bg: c.Bg, Attr: AttrWideCont | c.Attr&AttrStyleMask}
			} else if w == 2 {
				line[lx].Attr &^= AttrWide
			}
			lx += w
			if k+1 == cursorOff && k == len(para)-1 {
				// cursor right after the last character
				if lx >= cols {
					curLine, curX = len(outWrap), cols-1
				} else {
					curLine, curX = len(outWrap), lx
				}
			}
		}
		if cursorOff >= 0 && curLine < 0 {
			curLine, curX = len(outWrap), min(lx, cols-1)
		}
		emit(false)
		_ = startLine
	}

	n := len(outWrap)
	// Drop empty lines below the cursor and the last content line.
	last := curLine
	for k := n - 1; k > last; k-- {
		if contentLen(out[k*cols:(k+1)*cols]) > 0 || outWrap[k] {
			last = k
			break
		}
	}
	if last+1 < n {
		n = last + 1
	}
	if curLine < 0 {
		curLine = n - 1
	}
	// Ensure the cursor is on screen even if content below it overflows.
	if n-curLine > rows {
		n = curLine + rows
	}
	if n < rows {
		pad := rows - n
		out = append(out[:n*cols], make([]Cell, pad*cols)...)
		outWrap = append(outWrap[:n], make([]bool, pad)...)
		n = rows
	}
	first := 0
	if n > nb.maxLines {
		first = n - nb.maxLines
		nb.Evicted += int64(first)
	}
	count := n - first
	alloc := nb.maxLines
	nb.cells = make([]Cell, alloc*cols)
	copy(nb.cells, out[first*cols:n*cols])
	nb.wrapped = make([]bool, alloc)
	copy(nb.wrapped, outWrap[first:n])
	nb.count = count
	nb.dirty = make([]bool, rows)
	nb.allDirty = true
	ncy := curLine - (n - rows)
	ncx := curX
	if ncy < 0 {
		ncy = 0
	}
	if ncy >= rows {
		ncy = rows - 1
	}
	if ncx >= cols {
		ncx = cols - 1
	}
	return nb, ncx, ncy
}

// contentLen returns the length of l without trailing blank cells.
func contentLen(l []Cell) int {
	n := len(l)
	for n > 0 {
		c := &l[n-1]
		if c.Ch != 0 || !c.Bg.IsDefault() || c.Attr&^AttrWideCont != 0 || c.Extra != 0 {
			break
		}
		n--
	}
	return n
}

// resizeClip resizes without reflow (used for the alternate screen).
func (b *Buffer) resizeClip(cols, rows, cx, cy int) (*Buffer, int, int) {
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
	if cx >= cols {
		cx = cols - 1
	}
	return nb, cx, cy
}
