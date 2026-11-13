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
		if _, ok := b.LinkAt(p); ok {
			t.Errorf("LinkAt col %d found a link", col)
		}
	}
}
