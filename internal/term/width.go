package term

import (
	"sync/atomic"
	"unicode"

	"github.com/rivo/uniseg"
)

// Rune widths are computed lazily per 256-rune page so startup stays fast.
var widthPages [0x110000 >> 8]atomic.Pointer[[256]int8]

// RuneWidth returns the column width of r: 0 for combining/zero-width
// characters, 2 for wide (East Asian wide/fullwidth, emoji presentation)
// and 1 otherwise.
func RuneWidth(r rune) int {
	if r < 0x300 {
		if r < 0x20 || (r >= 0x7f && r < 0xa0) {
			return 0
		}
		return 1 // includes the soft hyphen, as in glibc
	}
	if r > 0x10ffff {
		return 1
	}
	p := widthPages[r>>8].Load()
	if p == nil {
		p = computePage(r >> 8)
		widthPages[r>>8].Store(p)
	}
	return int(p[r&0xff])
}

func computePage(page rune) *[256]int8 {
	var p [256]int8
	base := page << 8
	for i := rune(0); i < 256; i++ {
		p[i] = int8(computeWidth(base + i))
	}
	return &p
}

func computeWidth(r rune) int {
	switch {
	case r == 0x200d || r == 0x200c || r == 0x200b || r == 0x2060 || r == 0xfeff:
		return 0
	case r >= 0xfe00 && r <= 0xfe0f: // variation selectors
		return 0
	case r >= 0xe0100 && r <= 0xe01ef:
		return 0
	case r >= 0xe0020 && r <= 0xe007f: // tag characters (flag sequences)
		return 0
	case r >= 0x1160 && r <= 0x11ff: // Hangul medial vowels / final consonants
		return 0
	case unicode.In(r, unicode.Mn, unicode.Me):
		return 0
	case unicode.Is(unicode.Cf, r):
		return 0
	case r >= 0x1f1e6 && r <= 0x1f1ff: // regional indicators: half a flag
		return 1
	case r >= 0x1f3fb && r <= 0x1f3ff: // skin tone modifiers (wide, as in glibc)
		return 2
	case r >= 0xe000 && r <= 0xf8ff: // private use (powerline, nerd fonts)
		return 1
	}
	w := uniseg.StringWidth(string(r))
	if w < 1 {
		return 1
	}
	if w > 2 {
		return 2
	}
	return w
}

// StringWidth returns the display width of s in cells, grapheme-aware.
func StringWidth(s string) int { return uniseg.StringWidth(s) }
