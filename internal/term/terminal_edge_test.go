package term

import (
	"testing"
)

// wideIntact checks that every wide character's halves are together.
func wideIntact(tb testing.TB, t *Terminal, y int) {
	tb.Helper()
	line := t.buf.Line(y)
	for x, c := range line {
		if c.Attr&AttrWide != 0 && (x+1 >= len(line) || line[x+1].Attr&AttrWideCont == 0) {
			tb.Errorf("wide character at %d has no right half", x)
		}
		if c.Attr&AttrWideCont != 0 && (x == 0 || line[x-1].Attr&AttrWide == 0) {
			tb.Errorf("right half at %d has no wide character", x)
		}
	}
}

// TestWideInsertDelete checks that inserting and deleting characters next
// to wide ones never leaves half of one behind.
func TestWideInsertDelete(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{"delete before it", "ab中cd\x1b[1G\x1b[2P", "中cd"},
		{"delete its left half", "ab中cd\x1b[3G\x1b[1P", "ab cd"},
		{"delete onto its right half", "a中cd\x1b[1G\x1b[2P", " cd"},
		{"insert before it", "a中cd\x1b[2G\x1b[1@", "a 中cd"},
		{"insert at its right half", "中abcd\x1b[2G\x1b[1@", "   abc"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tm := feed(New(6, 2, 10), tc.in)
			wideIntact(t, tm, 0)
			if got := tm.buf.LineString(tm.buf.Count() - 2); got != tc.want {
				t.Errorf("%q, want %q", got, tc.want)
			}
		})
	}
}

// TestEraseBelowUnwraps checks that ED 0 ends the cursor's row as EL 0
// does: copy and reflow must not join it to the row below.
func TestEraseBelowUnwraps(t *testing.T) {
	tm := feed(New(5, 3, 10), "abcdefgh\x1b[1;3H\x1b[J")
	if tm.buf.Wrapped(tm.buf.Count() - 3) {
		t.Error("the cursor's row is still wrapped")
	}
}

// TestEraseScrollbackFromAltScreen checks that CSI 3 J clears the normal
// screen's scrollback while a full-screen program runs, as xterm does.
func TestEraseScrollbackFromAltScreen(t *testing.T) {
	tm := feed(New(10, 2, 10), "1\r\n2\r\n3\r\n4\r\n\x1b[?1049h\x1b[3J")
	if n := tm.primary.ScrollbackLen(); n != 0 {
		t.Errorf("%d lines of scrollback left", n)
	}
}

// TestReflowCursorOnWideRightHalf checks that a cursor on the right half
// of a wide character stays with it when the lines are rewrapped.
func TestReflowCursorOnWideRightHalf(t *testing.T) {
	tm := feed(New(10, 3, 10), "中abcdef\x1b[1;2H")
	tm.Resize(12, 3)
	expectCursor(t, tm, 1, 0)
}

// TestSearchLowercaseChangesLength checks that a case-insensitive search
// still matches in a line where lowercasing changes the text's length.
func TestSearchLowercaseChangesLength(t *testing.T) {
	tm := feed(New(20, 2, 10), "Foo K") // the Kelvin sign lowercases to 3 bytes fewer... or more
	if m := tm.buf.Search("foo", false); len(m) != 1 || m[0].Start.Col != 0 || m[0].End.Col != 3 {
		t.Errorf("%v", m)
	}
}

// TestNewTinyTerminal checks that a terminal made one column wide takes a
// wide character without panicking, as one resized that small does.
func TestNewTinyTerminal(t *testing.T) {
	feed(New(1, 1, 0), "中x")
}

// TestDefaultCursorKeepsProgramChoice checks that changing the cursor
// setting leaves a shape a program chose alone, even when it was the
// setting's, and that a program's reset goes back to the setting.
func TestDefaultCursorKeepsProgramChoice(t *testing.T) {
	tm := New(10, 2, 0)
	tm.SetDefaultCursor(CursorBlock)
	feed(tm, "\x1b[2 q") // the program asks for a block
	tm.SetDefaultCursor(CursorBeam)
	if tm.CursorStyle != CursorBlock {
		t.Errorf("the program's block became %d", tm.CursorStyle)
	}
	feed(tm, "\x1b[0 q")
	if tm.CursorStyle != CursorBeam {
		t.Errorf("after a reset: %d, want the setting's beam", tm.CursorStyle)
	}
	tm.SetDefaultCursor(CursorUnderline)
	if tm.CursorStyle != CursorUnderline {
		t.Errorf("the setting is not followed: %d", tm.CursorStyle)
	}
}

// TestWordAtLinkAtOutside checks that positions off the line are refused
// rather than read past it.
func TestWordAtLinkAtOutside(t *testing.T) {
	tm := feed(New(10, 2, 0), "hello")
	b := tm.buf
	line := int64(b.Count() - 2)
	for _, col := range []int{-1, 10, 99} {
		p := Pos{Line: line, Col: col}
		if s, e := b.WordAt(p, ""); s != p || e != p {
			t.Errorf("WordAt col %d: %v %v", col, s, e)
		}
		if _, ok := tm.LinkAt(p); ok {
			t.Errorf("LinkAt col %d found a link", col)
		}
		if _, ok := tm.LinkExtent(p); ok {
			t.Errorf("LinkExtent col %d found a link", col)
		}
	}
}

// TestLinkExtent checks that a link's extent covers all of its cells,
// also where its underline colour changes, and no others.
func TestLinkExtent(t *testing.T) {
	tm := feed(New(20, 2, 0), "ab\x1b]8;;http://x\x1b\\cd\x1b[58;5;1mef\x1b]8;;\x1b\\\x1b]8;;http://y\x1b\\g\x1b]8;;\x1b\\h")
	b := tm.buf
	line := int64(b.Count() - 2)
	want := Match{Start: Pos{Line: line, Col: 2}, End: Pos{Line: line, Col: 6}}
	for col := 2; col < 6; col++ {
		if m, ok := tm.LinkExtent(Pos{Line: line, Col: col}); !ok || m != want {
			t.Errorf("col %d: %v %v, want %v", col, m, ok, want)
		}
	}
	if m, ok := tm.LinkExtent(Pos{Line: line, Col: 6}); !ok || m.Start.Col != 6 || m.End.Col != 7 {
		t.Errorf("the next link: %v %v", m, ok)
	}
	for _, col := range []int{0, 1, 7} {
		if _, ok := tm.LinkExtent(Pos{Line: line, Col: col}); ok {
			t.Errorf("col %d has no link but has an extent", col)
		}
	}
}

// TestParseColorForms checks every X11 colour form termite takes, and
// some it refuses.
func TestParseColorForms(t *testing.T) {
	for in, want := range map[string]uint32{
		"rgb:f/0/8":          0xff0088,
		"rgb:ff/00/80":       0xff0080,
		"rgb:fff/000/800":    0xff007f,
		"rgb:ffff/0000/8000": 0xff007f,
		"#f08":               0xff0088,
		"#ff0080":            0xff0080,
		"#fff000800":         0xff0080,
		"#ffff00008000":      0xff0080,
		"  #102030 ":         0x102030,
	} {
		if got, ok := ParseColor(in); !ok || got != want {
			t.Errorf("%q: %06x %v, want %06x", in, got, ok, want)
		}
	}
	for _, bad := range []string{"", "#12", "#1234567", "rgb:1/2", "rgb:12345/0/0", "rgb:g/0/0", "red", "#gggggg"} {
		if got, ok := ParseColor(bad); ok {
			t.Errorf("%q parsed as %06x", bad, got)
		}
	}
}

// TestOSCColourResets checks that OSC 104 and 110-112 put back the
// theme's colours a program changed.
func TestOSCColourResets(t *testing.T) {
	tm := New(10, 2, 0)
	tm.SetColors(defaultPalette(), 0x111111, 0x222222, 0x333333)
	feed(tm, "\x1b]4;1;#010101\x07\x1b]4;2;#020202\x07\x1b]10;#aaaaaa\x07\x1b]11;#bbbbbb\x07\x1b]12;#cccccc\x07")
	if tm.Palette[1] != 0x010101 || tm.DefaultFg != 0xaaaaaa || tm.DefaultBg != 0xbbbbbb || tm.CursorColor != 0xcccccc {
		t.Fatal("the colours were not set")
	}
	feed(tm, "\x1b]104;1\x07")
	if tm.Palette[1] != defaultPalette()[1] || tm.Palette[2] != 0x020202 {
		t.Errorf("OSC 104;1: %06x %06x", tm.Palette[1], tm.Palette[2])
	}
	feed(tm, "\x1b]104\x07\x1b]110\x07\x1b]111\x07\x1b]112\x07")
	if tm.Palette != defaultPalette() || tm.DefaultFg != 0x111111 || tm.DefaultBg != 0x222222 || tm.CursorColor != 0x333333 {
		t.Errorf("resets: %06x %06x %06x", tm.DefaultFg, tm.DefaultBg, tm.CursorColor)
	}
}

// TestXTGETTCAP checks the terminfo capabilities termite answers, in hex,
// and the reply to one it doesn't know.
func TestXTGETTCAP(t *testing.T) {
	tm := New(10, 2, 0)
	feed(tm, "\x1bP+q544e;436f\x1b\\") // TN;Co
	if got := string(tm.TakeReplies()); got != "\x1bP1+r544e="+hexEncode("xterm-256color")+";436f="+hexEncode("256")+"\x1b\\" {
		t.Errorf("%q", got)
	}
	feed(tm, "\x1bP+q6e6f7065\x1b\\") // "nope"
	if got := string(tm.TakeReplies()); got != "\x1bP0+r\x1b\\" {
		t.Errorf("unknown: %q", got)
	}
}

// TestCursorReportOriginMode checks DSR 6 and DECXCPR, which count from
// the scrolling region's top in origin mode.
func TestCursorReportOriginMode(t *testing.T) {
	tm := feed(New(10, 6, 0), "\x1b[3;5r\x1b[?6h\x1b[2;4H\x1b[6n\x1b[?6n")
	if got := string(tm.TakeReplies()); got != "\x1b[2;4R\x1b[?2;4R" {
		t.Errorf("%q", got)
	}
}

// TestAltScreenRestoresCursor checks that leaving the 1049 screen puts
// the cursor back where it was on the normal one.
func TestAltScreenRestoresCursor(t *testing.T) {
	tm := feed(New(10, 4, 0), "\x1b[3;7H\x1b[?1049h\x1b[1;1Hxyz\x1b[?1049l")
	expectCursor(t, tm, 6, 2)
}

// TestTextRangeBlock checks copying a rectangle: each row's part of it,
// trailing blanks trimmed.
func TestTextRangeBlock(t *testing.T) {
	tm := feed(New(10, 3, 0), "abcdef\r\nghijkl\r\nmn")
	b := tm.buf
	top := int64(b.Count() - 3)
	got := b.TextRange(Pos{Line: top, Col: 1}, Pos{Line: top + 2, Col: 4}, true)
	if got != "bcd\nhij\nn" {
		t.Errorf("%q", got)
	}
}

// TestWordAtAcrossWrap checks that a word soft-wrapped onto the next row
// is selected whole from either half.
func TestWordAtAcrossWrap(t *testing.T) {
	tm := feed(New(6, 3, 0), "ab hello world")
	b := tm.buf
	top := int64(b.Count() - 3)
	for _, p := range []Pos{{Line: top, Col: 4}, {Line: top + 1, Col: 1}} {
		s, e := b.WordAt(p, "")
		if s != (Pos{Line: top, Col: 3}) || e != (Pos{Line: top + 1, Col: 2}) {
			t.Errorf("from %v: %v to %v", p, s, e)
		}
	}
}

// TestResizeKeepsPendingWrap checks that a resize leaves a pending wrap
// pending: the next character goes on the next line instead of over the
// one in the last column.
func TestResizeKeepsPendingWrap(t *testing.T) {
	for _, tc := range []struct {
		name       string
		cols, rows int
		want       []string
	}{
		{"rows only", 10, 4, []string{"0123456789", "A", "", ""}},
		{"wider", 12, 3, []string{"0123456789A", "", ""}},
		{"narrower", 5, 3, []string{"01234", "56789", "A"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tm := feed(newT(10, 3), "0123456789")
			tm.Resize(tc.cols, tc.rows)
			feed(tm, "A")
			expectScreen(t, tm, tc.want...)
		})
	}
	// The same on the alternate screen, which is clipped, not reflowed.
	tm := feed(newT(10, 3), "\x1b[?1049h0123456789")
	tm.Resize(10, 4)
	feed(tm, "A")
	expectScreen(t, tm, "0123456789", "A", "", "")
}

// TestWidePadClearsOldWide prints a wide character in the last column over
// the right half of another: the old one must go, not stay half drawn.
func TestWidePadClearsOldWide(t *testing.T) {
	tm := feed(newT(10, 3), "aaaaaaaa中\x1b[10G中")
	wideIntact(t, tm, 0)
	if l := tm.buf.Line(0); l[8].Attr&AttrWide != 0 {
		t.Error("the old wide character is still at column 8")
	}
	expectScreen(t, tm, "aaaaaaaa", "中", "")
}

// TestC1Dropped checks that C1 controls arriving as UTF-8 are dropped, as
// wcwidth gives them -1, rather than joined to the previous character.
func TestC1Dropped(t *testing.T) {
	tm := feed(newT(10, 3), "a\xc2\x85b")
	if got := tm.buf.Line(0)[0].Text(); got != "a" {
		t.Errorf("cell 0 is %q", got)
	}
	expectScreen(t, tm, "ab", "", "")
}

// TestWordAtAcrossWrapPad selects a word that a wide character's wrap
// split, leaving a pad in the last column.
func TestWordAtAcrossWrapPad(t *testing.T) {
	tm := feed(newT(5, 3), "中中中")
	b := tm.Buf()
	top := b.ScreenTopAbs()
	for _, p := range []Pos{{top, 0}, {top + 1, 0}} {
		s, e := b.WordAt(p, "")
		if got := b.TextRange(s, e, false); got != "中中中" {
			t.Errorf("word at %v: %q", p, got)
		}
	}
}
