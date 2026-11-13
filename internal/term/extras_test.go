package term

import (
	"fmt"
	"strings"
	"testing"
)

// floodLinks writes n links, each on a line of its own, that scroll away.
func floodLinks(t *Terminal, n int) {
	var sb strings.Builder
	for i := range n {
		fmt.Fprintf(&sb, "\x1b]8;;http://x/%d\x1b\\a\x1b]8;;\x1b\\\r\n", i)
		if sb.Len() > 1<<16 {
			feed(t, sb.String())
			sb.Reset()
		}
	}
	feed(t, sb.String())
}

// TestExtrasReclaimed writes more links than a cell can index: those that
// scrolled away are reclaimed, while those still on the screen, and in a
// saved cursor's pen, keep theirs.
func TestExtrasReclaimed(t *testing.T) {
	tm := New(20, 4, 0)
	// row 0: a link with an underline colour, kept out of the scroll region
	feed(tm, "\x1b[58;5;3m\x1b]8;id=k;http://keep\x1b\\kk\x1b]8;;\x1b\\\x1b[59m")
	// a saved cursor whose pen writes another link
	feed(tm, "\x1b]8;;http://saved\x1b\\\x1b7\x1b]8;;\x1b\\")
	feed(tm, "\x1b[2;4r\x1b[2;1H")
	floodLinks(tm, maxLinks+10)
	if tm.interned.reclaims == 0 {
		t.Fatal("nothing was reclaimed")
	}
	top := tm.buf.ScreenTopAbs()
	if l, ok := tm.LinkAt(Pos{Line: top, Col: 1}); !ok || l.URI != "http://keep" || l.ID != "k" {
		t.Errorf("the link on the screen: %+v %v", l, ok)
	}
	if e := tm.ExtraOf(tm.buf.Line(0)[0].Extra); e.ULColor != Indexed(3) {
		t.Errorf("its underline colour: %+v", e)
	}
	// the newest flood link is still there and right
	feed(tm, "\x1b[4;1H\x1b]8;;http://last\x1b\\z\x1b]8;;\x1b\\")
	if l, ok := tm.LinkAt(Pos{Line: top + 3, Col: 0}); !ok || l.URI != "http://last" {
		t.Errorf("a new link after reclaiming: %+v %v", l, ok)
	}
	// the saved pen still writes its link
	feed(tm, "\x1b8s")
	if l, ok := tm.LinkAt(Pos{Line: top, Col: 2}); !ok || l.URI != "http://saved" {
		t.Errorf("the saved cursor's link: %+v %v", l, ok)
	}
	if n := len(tm.interned.extras); n > maxExtras {
		t.Errorf("%d extras", n)
	}
	if n := len(tm.interned.links); n > maxLinks {
		t.Errorf("%d links", n)
	}
}

// TestExtrasFull fills every extras index with cells on the screen: a new
// one gets none rather than another cell's, and none is lost.
func TestExtrasFull(t *testing.T) {
	const cols, rows = 300, 220 // more cells than indexes
	tm := New(cols, rows, 0)
	var sb strings.Builder
	for i := range cols * rows {
		fmt.Fprintf(&sb, "\x1b[58;2;%d;%d;%dmx", i>>16, i>>8&0xff, i&0xff)
	}
	feed(tm, sb.String()[:sb.Len()-1]) // the last cell: no wrap, no scroll
	if tm.interned.reclaims == 0 {
		t.Fatal("the full table was not reclaimed")
	}
	for i := range cols * rows {
		c := tm.buf.Line(i / cols)[i%cols]
		want := Extra{ULColor: RGB(uint8(i>>16), uint8(i>>8), uint8(i))}
		if got := tm.ExtraOf(c.Extra); c.Extra != 0 && got != want {
			t.Fatalf("cell %d has %+v, want %+v or none", i, got, want)
		}
		if i < maxExtras-1 && c.Extra == 0 {
			t.Fatalf("cell %d lost its extras", i)
		}
	}
}
