// Package term implements the terminal emulator core: the cell grid,
// scrollback ring, escape sequence parser and xterm-compatible emulation.
// It has no GUI dependencies and is safe to test headlessly.
package term

import "sync"

// Color is a packed colour. The top byte is the kind; the low 24 bits
// hold either a palette index or an RGB triple.
type Color uint32

const (
	ColorDefault Color = 0
	colorIndexed Color = 1 << 24
	colorRGB     Color = 2 << 24
	colorKind    Color = 0xff << 24
)

// Indexed returns a palette colour (0-255).
func Indexed(i uint8) Color { return colorIndexed | Color(i) }

// RGB returns a truecolour value.
func RGB(r, g, b uint8) Color { return colorRGB | Color(r)<<16 | Color(g)<<8 | Color(b) }

func (c Color) IsDefault() bool { return c&colorKind == 0 }
func (c Color) IsIndexed() bool { return c&colorKind == colorIndexed }
func (c Color) IsRGB() bool     { return c&colorKind == colorRGB }
func (c Color) Index() uint8    { return uint8(c) }
func (c Color) RGB() (r, g, b uint8) {
	return uint8(c >> 16), uint8(c >> 8), uint8(c)
}

// Attr holds packed cell flags.
type Attr uint16

const (
	AttrBold Attr = 1 << iota
	AttrDim
	AttrItalic
	attrUL0 // underline style, 3 bits
	attrUL1
	attrUL2
	AttrInverse
	AttrHidden
	AttrStrike
	AttrWide     // first half of a double-width character
	AttrWideCont // second half (placeholder) of a double-width character
	AttrCluster  // Ch is an index into the grapheme cluster table
	AttrOverline
)

const (
	attrULShift = 3
	attrULMask  = 7 << attrULShift
	// AttrStyleMask covers the attributes that are part of the "pen".
	AttrStyleMask = AttrBold | AttrDim | AttrItalic | attrULMask | AttrInverse | AttrHidden | AttrStrike | AttrOverline
)

// Underline styles.
const (
	ULNone = iota
	ULSingle
	ULDouble
	ULCurly
	ULDotted
	ULDashed
)

func (a Attr) Underline() int { return int(a&attrULMask) >> attrULShift }
func (a Attr) WithUnderline(u int) Attr {
	return a&^attrULMask | Attr(u<<attrULShift)&attrULMask
}

// Cell is a single grid cell: 16 bytes, no pointers.
type Cell struct {
	Ch    rune  // code point, 0 for empty; cluster index when AttrCluster
	Fg    Color // foreground
	Bg    Color // background
	Attr  Attr
	Extra uint16 // index into the extras table (underline colour, hyperlink)
}

// Blank reports whether the cell holds no visible content.
func (c *Cell) Blank() bool { return c.Ch == 0 || c.Ch == ' ' }

// Extra holds rarely used per-cell data, interned so cells stay small.
type Extra struct {
	ULColor Color
	Link    uint32 // hyperlink id (0 = none)
}

// intern tables shared by every terminal (cells from any pane may be
// rendered by the same renderer, and the tables are append-only).
var tables = struct {
	mu        sync.RWMutex
	clusters  []string
	clusterIx map[string]uint32
	extras    []Extra
	extraIx   map[Extra]uint16
	links     []Link
	linkIx    map[Link]uint32
}{
	clusters:  []string{""},
	clusterIx: map[string]uint32{},
	extras:    []Extra{{}},
	extraIx:   map[Extra]uint16{{}: 0},
	links:     []Link{{}},
	linkIx:    map[Link]uint32{},
}

// Link is an OSC 8 hyperlink.
type Link struct {
	ID  string
	URI string
}

func internCluster(s string) uint32 {
	tables.mu.Lock()
	defer tables.mu.Unlock()
	if i, ok := tables.clusterIx[s]; ok {
		return i
	}
	i := uint32(len(tables.clusters))
	tables.clusters = append(tables.clusters, s)
	tables.clusterIx[s] = i
	return i
}

// ClusterString returns the grapheme cluster with index i.
func ClusterString(i uint32) string {
	tables.mu.RLock()
	defer tables.mu.RUnlock()
	if int(i) < len(tables.clusters) {
		return tables.clusters[i]
	}
	return ""
}

func internExtra(e Extra) uint16 {
	if e == (Extra{}) {
		return 0
	}
	tables.mu.Lock()
	defer tables.mu.Unlock()
	if i, ok := tables.extraIx[e]; ok {
		return i
	}
	if len(tables.extras) >= 0xffff {
		return 0 // table full; degrade gracefully
	}
	i := uint16(len(tables.extras))
	tables.extras = append(tables.extras, e)
	tables.extraIx[e] = i
	return i
}

// ExtraOf returns the extras record for index i.
func ExtraOf(i uint16) Extra {
	if i == 0 {
		return Extra{}
	}
	tables.mu.RLock()
	defer tables.mu.RUnlock()
	return tables.extras[i]
}

func internLink(l Link) uint32 {
	if l.URI == "" {
		return 0
	}
	tables.mu.Lock()
	defer tables.mu.Unlock()
	if i, ok := tables.linkIx[l]; ok {
		return i
	}
	i := uint32(len(tables.links))
	tables.links = append(tables.links, l)
	tables.linkIx[l] = i
	return i
}

// LinkOf returns the hyperlink with id i.
func LinkOf(i uint32) Link {
	tables.mu.RLock()
	defer tables.mu.RUnlock()
	if int(i) < len(tables.links) {
		return tables.links[i]
	}
	return Link{}
}

// Text returns the text content of a cell ("" for empty or wide continuation).
func (c *Cell) Text() string {
	if c.Attr&AttrWideCont != 0 {
		return ""
	}
	if c.Attr&AttrCluster != 0 {
		return ClusterString(uint32(c.Ch))
	}
	if c.Ch == 0 {
		return ""
	}
	return string(c.Ch)
}
