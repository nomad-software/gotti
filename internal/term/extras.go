package term

// Each terminal interns its cells' rarely used data, the underline colour
// and hyperlink (Extra), in its own tables, used under its Mu. A Cell has
// 16 bits for its extras index, so when a table fills, the entries no cell
// or pen uses any more are reclaimed (reclaim) and their indexes reused: a
// long session making many links never runs out.

const (
	maxExtras = 1 << 16 // Cell.Extra is 16 bits
	// Every link in use is in an extra in use, or the one being written
	// (Terminal.link), so this many is never too few.
	maxLinks = maxExtras + 1
)

type internTables struct {
	extras     []Extra // index 0 is no extras
	extraIx    map[Extra]uint16
	freeExtras []uint16
	links      []Link // index 0 is no link
	linkIx     map[Link]uint32
	freeLinks  []uint32
	reclaims   int // times entries were reclaimed, for tests
}

func newInternTables() internTables {
	return internTables{
		extras:  []Extra{{}},
		extraIx: map[Extra]uint16{},
		links:   []Link{{}},
		linkIx:  map[Link]uint32{},
	}
}

// internExtra returns e's index, adding it if it is new. When every index
// is in use, it returns 0: the cells lose their extras rather than show
// another cell's.
func (t *Terminal) internExtra(e Extra) uint16 {
	if e == (Extra{}) {
		return 0
	}
	x := &t.interned
	if i, ok := x.extraIx[e]; ok {
		return i
	}
	if len(x.freeExtras) == 0 && len(x.extras) >= maxExtras {
		t.reclaim()
	}
	var i uint16
	if n := len(x.freeExtras); n > 0 {
		i = x.freeExtras[n-1]
		x.freeExtras = x.freeExtras[:n-1]
		x.extras[i] = e
	} else if len(x.extras) < maxExtras {
		i = uint16(len(x.extras))
		x.extras = append(x.extras, e)
	} else {
		return 0
	}
	x.extraIx[e] = i
	return i
}

// internLink returns l's id, adding it if it is new, or 0 (no link) when
// every id is in use.
func (t *Terminal) internLink(l Link) uint32 {
	if l.URI == "" {
		return 0
	}
	x := &t.interned
	if i, ok := x.linkIx[l]; ok {
		return i
	}
	if len(x.freeLinks) == 0 && len(x.links) >= maxLinks {
		t.reclaim()
	}
	var i uint32
	if n := len(x.freeLinks); n > 0 {
		i = x.freeLinks[n-1]
		x.freeLinks = x.freeLinks[:n-1]
		x.links[i] = l
	} else if len(x.links) < maxLinks {
		i = uint32(len(x.links))
		x.links = append(x.links, l)
	} else {
		return 0
	}
	x.linkIx[l] = i
	return i
}

// ExtraOf returns the extras with index i. The caller holds Mu.
func (t *Terminal) ExtraOf(i uint16) Extra {
	if int(i) < len(t.interned.extras) {
		return t.interned.extras[i]
	}
	return Extra{}
}

// LinkOf returns the hyperlink with id i. The caller holds Mu.
func (t *Terminal) LinkOf(i uint32) Link {
	if int(i) < len(t.interned.links) {
		return t.interned.links[i]
	}
	return Link{}
}

// HasExtras reports whether any cell may have extras, so callers can skip
// looking them up. The caller holds Mu.
func (t *Terminal) HasExtras() bool { return len(t.interned.extras) > 1 }

// reclaim frees the extras no cell of either screen, nor any pen, uses,
// and the links no extra left, nor the pen being written, uses. Whole ring
// storage is scanned, rows beyond the kept lines too: keeping an entry a
// stale row holds is harmless, freeing one a live row holds is not.
func (t *Terminal) reclaim() {
	x := &t.interned
	x.reclaims++
	used := make([]bool, len(x.extras))
	mark := func(e uint16) {
		if int(e) < len(used) {
			used[e] = true
		}
	}
	for _, b := range []*Buffer{t.primary, t.alt} {
		for i := range b.cells {
			if e := b.cells[i].Extra; e != 0 {
				mark(e)
			}
		}
	}
	for _, c := range []*cursor{&t.cur, &t.savedMain, &t.savedAlt} {
		mark(c.pen.Extra)
	}
	linkUsed := make([]bool, len(x.links))
	if int(t.link) < len(linkUsed) {
		linkUsed[t.link] = true
	}
	x.freeExtras = x.freeExtras[:0]
	for i := len(x.extras) - 1; i > 0; i-- {
		e := x.extras[i]
		if used[i] {
			if int(e.Link) < len(linkUsed) {
				linkUsed[e.Link] = true
			}
			continue
		}
		if e != (Extra{}) {
			delete(x.extraIx, e)
			x.extras[i] = Extra{}
		}
		x.freeExtras = append(x.freeExtras, uint16(i))
	}
	x.freeLinks = x.freeLinks[:0]
	for i := len(x.links) - 1; i > 0; i-- {
		if linkUsed[i] {
			continue
		}
		if l := x.links[i]; l != (Link{}) {
			delete(x.linkIx, l)
			x.links[i] = Link{}
		}
		x.freeLinks = append(x.freeLinks, uint32(i))
	}
}
