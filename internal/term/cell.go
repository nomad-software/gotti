// Package term implements the terminal emulator core: the cell grid,
// scrollback ring, escape sequence parser and xterm-compatible emulation.
// It has no GUI dependencies and is safe to test headlessly.
package term

import "sync"

// Color is a packed colour. The top byte is the kind; the low 24 bits
// hold either a palette index or an RGB triple.
type Color uint32

const (
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
	_ // three bits holding the underline style (see Underline)
	_
	_
	AttrInverse
	AttrHidden
	AttrStrike
	AttrWide     // first half of a double-width character
	AttrWideCont // second half (placeholder) of a double-width character
	AttrCluster  // Ch is an index into the grapheme cluster table
	AttrOverline
	// AttrWrapPad marks the blank left at the end of a line when a wide
	// character did not fit and wrapped to the next line. It is layout,
	// not content: reflow, copying and search skip it.
	AttrWrapPad
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

// Extra holds rarely used per-cell data, interned so cells stay small.
type Extra struct {
	ULColor Color
	Link    uint32 // hyperlink id (0 = none)
}

// clusters is the grapheme cluster table. It is shared by every terminal
// and append-only, as the glyph cache, which every pane shares, keys
// clusters by their index. Underline colours and hyperlinks are interned
// per terminal instead (extras.go).
var clusters = struct {
	mu   sync.RWMutex
	list []string
	ix   map[string]uint32
}{list: []string{""}, ix: map[string]uint32{}}

// Link is an OSC 8 hyperlink.
type Link struct {
	ID  string
	URI string
}

func internCluster(s string) uint32 {
	clusters.mu.Lock()
	defer clusters.mu.Unlock()
	if i, ok := clusters.ix[s]; ok {
		return i
	}
	i := uint32(len(clusters.list))
	clusters.list = append(clusters.list, s)
	clusters.ix[s] = i
	return i
}

// ClusterString returns the grapheme cluster with index i.
func ClusterString(i uint32) string {
	clusters.mu.RLock()
	defer clusters.mu.RUnlock()
	if int(i) < len(clusters.list) {
		return clusters.list[i]
	}
	return ""
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
