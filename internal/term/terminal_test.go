package term

import (
	"fmt"
	"strings"
	"testing"
)

// LineString returns the text of logical line i, trailing blanks trimmed.
func (b *Buffer) LineString(i int) string {
	return strings.TrimRight(string(appendLine(nil, b.LogicalLine(i), 0, b.Cols)), " ")
}

func newT(cols, rows int) *Terminal { return New(cols, rows, 100) }

func feed(t *Terminal, s string) *Terminal {
	t.Write([]byte(s))
	return t
}

func screen(t *Terminal) string {
	var lines []string
	for y := 0; y < t.rows; y++ {
		lines = append(lines, t.buf.LineString(t.buf.Count()-t.rows+y))
	}
	return strings.Join(lines, "\n")
}

func expectScreen(tb testing.TB, t *Terminal, want ...string) {
	tb.Helper()
	got := screen(t)
	w := strings.Join(want, "\n")
	if got != w {
		tb.Fatalf("screen mismatch\n--- got ---\n%s\n--- want ---\n%s", got, w)
	}
}

func expectCursor(tb testing.TB, t *Terminal, x, y int) {
	tb.Helper()
	if t.cur.X != x || t.cur.Y != y {
		tb.Fatalf("cursor at %d,%d want %d,%d", t.cur.X, t.cur.Y, x, y)
	}
}

// Golden-screen tests: bytes in, expected grid out.
var golden = []struct {
	name       string
	cols, rows int
	in         string
	want       []string
	cx, cy     int
}{
	{"plain", 10, 3, "hello", []string{"hello", "", ""}, 5, 0},
	{"crlf", 10, 3, "ab\r\ncd", []string{"ab", "cd", ""}, 2, 1},
	{"lf keeps column", 10, 3, "ab\ncd", []string{"ab", "  cd", ""}, 4, 1},
	{"wrap", 5, 3, "abcdefg", []string{"abcde", "fg", ""}, 2, 1},
	{"pending wrap at edge", 5, 3, "abcde", []string{"abcde", "", ""}, 4, 0},
	{"scroll", 5, 2, "a\r\nb\r\nc", []string{"b", "c"}, 1, 1},
	{"backspace", 10, 1, "abc\b\bX", []string{"aXc"}, 2, 0},
	{"tab", 20, 1, "a\tb", []string{"a       b"}, 9, 0},
	{"cup", 10, 3, "\x1b[2;3HX", []string{"", "  X", ""}, 3, 1},
	{"cup default", 10, 3, "abc\x1b[HX", []string{"Xbc", "", ""}, 1, 0},
	{"cursor moves", 10, 3, "\x1b[3;5H\x1b[AX\x1b[2DY\x1b[CZ", []string{"", "   YXZ", ""}, 6, 1},
	{"el right", 10, 1, "abcdef\x1b[3G\x1b[K", []string{"ab"}, 2, 0},
	{"el left", 10, 1, "abcdef\x1b[3G\x1b[1K", []string{"   def"}, 2, 0},
	{"el all", 10, 1, "abcdef\x1b[2K", []string{""}, 6, 0},
	{"ed below", 5, 3, "aaaaa\r\nbbbbb\r\nccccc\x1b[2;3H\x1b[J", []string{"aaaaa", "bb", ""}, 2, 1},
	{"ed above", 5, 3, "aaaaa\r\nbbbbb\r\nccccc\x1b[2;3H\x1b[1J", []string{"", "   bb", "ccccc"}, 2, 1},
	{"ed all", 5, 2, "aa\r\nbb\x1b[2J", []string{"", ""}, 2, 1},
	{"ich", 10, 1, "abcdef\x1b[3G\x1b[2@", []string{"ab  cdef"}, 2, 0},
	{"dch", 10, 1, "abcdef\x1b[3G\x1b[2P", []string{"abef"}, 2, 0},
	{"ech", 10, 1, "abcdef\x1b[3G\x1b[2X", []string{"ab  ef"}, 2, 0},
	{"il", 5, 4, "a\r\nb\r\nc\r\nd\x1b[2H\x1b[L", []string{"a", "", "b", "c"}, 0, 1},
	{"dl", 5, 4, "a\r\nb\r\nc\r\nd\x1b[2H\x1b[M", []string{"a", "c", "d", ""}, 0, 1},
	{"decstbm scroll", 5, 4, "1\r\n2\r\n3\r\n4\x1b[2;3r\x1b[3H\n\nX", []string{"1", "", "X", "4"}, 1, 2},
	{"reverse index top", 5, 3, "a\r\nb\x1b[H\x1bMX", []string{"X", "a", "b"}, 1, 0},
	{"reverse index region", 5, 4, "1\r\n2\r\n3\r\n4\x1b[2;3r\x1b[2H\x1bM", []string{"1", "", "2", "4"}, 0, 1},
	{"origin mode", 5, 4, "\x1b[2;3r\x1b[?6h\x1b[HX\x1b[9;1HY", []string{"", "X", "Y", ""}, 1, 2},
	{"save restore", 10, 2, "ab\x1b7\x1b[2;5Hcd\x1b8X", []string{"abX", "    cd"}, 3, 0},
	{"rep", 10, 1, "a\x1b[3b", []string{"aaaa"}, 4, 0},
	{"dec graphics", 10, 1, "\x1b(0lqk\x1b(Bx", []string{"┌─┐x"}, 4, 0},
	{"shift out", 10, 1, "\x1b)0a\x0eq\x0fq", []string{"a─q"}, 3, 0},
	{"wide char", 10, 1, "a世b", []string{"a世b"}, 4, 0},
	{"wide wrap", 5, 2, "abcd世", []string{"abcd", "世"}, 2, 1},
	{"combining", 10, 1, "éx", []string{"éx"}, 2, 0},
	{"emoji zwj", 10, 1, "👨‍👩‍👧!", []string{"👨‍👩‍👧!"}, 7, 0},
	{"skin tone", 10, 1, "👍🏻!", []string{"👍🏻!"}, 5, 0},
	{"flag", 10, 1, "🇬🇧!", []string{"🇬🇧!"}, 3, 0},
	{"overwrite wide half", 10, 1, "世\x1b[2GX", []string{" X"}, 2, 0},
	{"utf8", 10, 1, "héllo", []string{"héllo"}, 5, 0},
	{"invalid utf8", 10, 1, "a\xffb", []string{"a�b"}, 3, 0},
	{"autowrap off", 5, 1, "\x1b[?7labcdefg", []string{"abcdg"}, 4, 0},
	{"insert mode", 10, 1, "abc\x1b[2G\x1b[4hX", []string{"aXbc"}, 2, 0},
	{"decaln", 3, 2, "\x1b#8", []string{"EEE", "EEE"}, 0, 0},
	{"cnl cpl", 10, 3, "abc\x1b[2Ex\x1b[1Fy", []string{"abc", "y", "x"}, 1, 1},
	{"hts tbc", 20, 1, "\x1b[3g\x1b[4G\x1bH\r\tX", []string{"   X"}, 4, 0},
	{"cbt", 20, 1, "\x1b[12G\x1b[ZX", []string{"        X"}, 9, 0},
	{"vpa hpa", 10, 3, "\x1b[3d\x1b[4`X", []string{"", "", "   X"}, 4, 2},
	{"bs at pending wrap", 5, 1, "abcde\bX", []string{"abcXe"}, 4, 0},
	{"split utf8 is fine", 10, 1, "\xe4\xb8", []string{""}, 0, 0},
	{"can aborts csi", 10, 1, "\x1b[2\x18X", []string{"X"}, 1, 0},
	{"osc title ignored on screen", 10, 1, "\x1b]0;hi\x07ok", []string{"ok"}, 2, 0},
	{"dcs swallowed", 10, 1, "\x1bPqabc\x1b\\ok", []string{"ok"}, 2, 0},
	{"apc swallowed", 10, 1, "\x1b_Gabc\x1b\\ok", []string{"ok"}, 2, 0},
}

func TestGolden(t *testing.T) {
	for _, g := range golden {
		t.Run(g.name, func(t *testing.T) {
			term := feed(newT(g.cols, g.rows), g.in)
			expectScreen(t, term, g.want...)
			expectCursor(t, term, g.cx, g.cy)
		})
	}
}

func TestSplitUTF8AcrossWrites(t *testing.T) {
	term := newT(10, 1)
	b := []byte("世界")
	for i := range b {
		term.Write(b[i : i+1])
	}
	expectScreen(t, term, "世界")
}

func TestSplitEscapeAcrossWrites(t *testing.T) {
	term := newT(10, 2)
	for _, c := range []byte("\x1b[2;3HX") {
		term.Write([]byte{c})
	}
	expectScreen(t, term, "", "  X")
}

func TestSGR(t *testing.T) {
	term := feed(newT(20, 1), "\x1b[1;3;4:3;31;48;5;100;58:2::1:2:3mA\x1b[0mB\x1b[38;2;10;20;30;9;7mC\x1b[22;39;49;21mD")
	l := term.buf.Line(0)
	a := l[0]
	if a.Attr&AttrBold == 0 || a.Attr&AttrItalic == 0 || a.Attr.Underline() != ULCurly {
		t.Fatalf("attrs %b", a.Attr)
	}
	if a.Fg != Indexed(1) || a.Bg != Indexed(100) {
		t.Fatalf("colours %x %x", a.Fg, a.Bg)
	}
	if ExtraOf(a.Extra).ULColor != RGB(1, 2, 3) {
		t.Fatalf("ul colour %x", ExtraOf(a.Extra).ULColor)
	}
	if l[1].Attr != 0 || l[1].Fg != 0 || l[1].Extra != 0 {
		t.Fatalf("reset failed %+v", l[1])
	}
	c := l[2]
	if c.Fg != RGB(10, 20, 30) || c.Attr&AttrStrike == 0 || c.Attr&AttrInverse == 0 {
		t.Fatalf("C %+v", c)
	}
	d := l[3]
	if d.Fg != 0 || d.Attr.Underline() != ULDouble || d.Attr&AttrInverse == 0 {
		t.Fatalf("D %+v", d)
	}
	feed(term, "\x1b[92;101mE")
	if e := term.buf.Line(0)[4]; e.Fg != Indexed(10) || e.Bg != Indexed(9) {
		t.Fatalf("bright %+v", e)
	}
}

func TestBCE(t *testing.T) {
	term := feed(newT(5, 2), "\x1b[44m\x1b[2J")
	if term.buf.Line(1)[3].Bg != Indexed(4) {
		t.Fatal("erase did not use background colour")
	}
}

func TestReplies(t *testing.T) {
	cases := map[string]string{
		"\x1b[c":                  "\x1b[?62;22c",
		"\x1b[>c":                 "\x1b[>1;10;0c",
		"\x1b[5n":                 "\x1b[0n",
		"\x1b[3;4H\x1b[6n":        "\x1b[3;4R",
		"\x1b[?2026$p":            "\x1b[?2026;2$y",
		"\x1b[?2004h\x1b[?2004$p": "\x1b[?2004;1$y",
		"\x1b[18t":                "\x1b[8;24;80t",
		"\x1b]11;?\x07":           "\x1b]11;rgb:1010/1010/1010\x07",
		"\x1b]4;1;?\x1b\\":        "\x1b]4;1;rgb:cdcd/0000/0000\x1b\\",
		"\x1b[?u":                 "\x1b[?0u",
		"\x1b[>5u\x1b[?u":         "\x1b[?5u",
		"\x1bP$qr\x1b\\":          "\x1bP1$r1;24r\x1b\\",
	}
	for in, want := range cases {
		term := feed(New(80, 24, 0), in)
		if got := string(term.TakeReplies()); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}

func TestKittyStack(t *testing.T) {
	term := feed(newT(10, 2), "\x1b[>1u\x1b[>3u")
	if term.KittyFlags() != 3 {
		t.Fatal(term.KittyFlags())
	}
	feed(term, "\x1b[<u")
	if term.KittyFlags() != 1 {
		t.Fatal(term.KittyFlags())
	}
	feed(term, "\x1b[=8;2u")
	if term.KittyFlags() != 9 {
		t.Fatal(term.KittyFlags())
	}
	feed(term, "\x1b[?1049h")
	if term.KittyFlags() != 0 {
		t.Fatal("alt screen should have its own stack")
	}
}

func TestModes(t *testing.T) {
	term := feed(newT(10, 2), "\x1b[?1h\x1b[?25l\x1b[?2004h\x1b[?1002h\x1b[?1006h\x1b[?1004h\x1b[5 q")
	if !term.AppCursor || term.CursorVisible || !term.BracketedPaste || term.MouseMode != MouseButton ||
		!term.MouseSGR || !term.FocusEvents || term.CursorStyle != CursorBeam || !term.CursorBlink {
		t.Fatalf("modes not set: %+v", term)
	}
	feed(term, "\x1b[?1l\x1b[?25h\x1b[?1002l\x1b[2 q")
	if term.AppCursor || !term.CursorVisible || term.MouseMode != MouseOff || term.CursorBlink {
		t.Fatal("modes not reset")
	}
}

func TestAltScreen(t *testing.T) {
	term := feed(newT(10, 3), "main\x1b[?1049h\x1b[2;2Halt")
	expectScreen(t, term, "", " alt", "")
	feed(term, "\x1b[?1049l")
	expectScreen(t, term, "main", "", "")
	expectCursor(t, term, 4, 0)
}

func TestSyncOutput(t *testing.T) {
	term := feed(newT(10, 3), "\x1b[?2026h")
	if !term.SyncOutput {
		t.Fatal("sync not set")
	}
	feed(term, "\x1b[?2026l")
	if term.SyncOutput || term.TakeEvents()&EvSync == 0 {
		t.Fatal("sync end not reported")
	}
}

func TestOSC(t *testing.T) {
	term := feed(newT(20, 2), "\x1b]2;my title\x1b\\\x1b]7;file://host/home/u%20x\x07\x1b]52;c;aGVsbG8=\x07")
	ev := term.TakeEvents()
	if term.Title != "my title" || ev&EvTitle == 0 {
		t.Fatal("title", term.Title)
	}
	if term.Cwd != "/home/u x" {
		t.Fatal("cwd", term.Cwd)
	}
	if len(term.ClipboardWrites) != 1 || term.ClipboardWrites[0].Text != "hello" {
		t.Fatal("clipboard", term.ClipboardWrites)
	}
	feed(term, "\x1b]8;id=1;https://example.com\x1b\\link\x1b]8;;\x1b\\ no")
	l, ok := term.buf.LinkAt(Pos{term.buf.ScreenTopAbs(), 1})
	if !ok || l.URI != "https://example.com" {
		t.Fatal("hyperlink", l, ok)
	}
	if _, ok := term.buf.LinkAt(Pos{term.buf.ScreenTopAbs(), 5}); ok {
		t.Fatal("link should have ended")
	}
}

func TestScrollbackRing(t *testing.T) {
	term := New(10, 3, 5)
	for i := 0; i < 20; i++ {
		feed(term, fmt.Sprintf("%d\r\n", i))
	}
	b := term.buf
	if b.ScrollbackLen() != 5 {
		t.Fatalf("scrollback %d", b.ScrollbackLen())
	}
	if got := b.LineString(0); got != "13" {
		t.Fatalf("oldest line %q", got)
	}
	expectScreen(t, term, "18", "19", "")
	if b.Evicted != 13 {
		t.Fatalf("evicted %d", b.Evicted)
	}
}

func TestReflow(t *testing.T) {
	term := New(10, 3, 100)
	feed(term, "abcdefghijklmno\r\nxy")
	expectScreen(t, term, "abcdefghij", "klmno", "xy")
	term.Resize(5, 3)
	// the 15-char paragraph becomes 3 lines; one moves into scrollback
	expectScreen(t, term, "fghij", "klmno", "xy")
	expectCursor(t, term, 2, 2)
	if term.buf.LineString(0) != "abcde" {
		t.Fatal("scrollback", term.buf.LineString(0))
	}
	term.Resize(20, 3)
	expectScreen(t, term, "abcdefghijklmno", "xy", "")
	expectCursor(t, term, 2, 1)
}

func TestReflowWide(t *testing.T) {
	term := New(6, 2, 10)
	feed(term, "ab世界cd")
	expectScreen(t, term, "ab世界", "cd")
	term.Resize(5, 3)
	expectScreen(t, term, "ab世", "界cd", "")
}

func TestReflowCursorMidLine(t *testing.T) {
	term := New(10, 2, 10)
	feed(term, "0123456789abc\x1b[1;4H")
	term.Resize(5, 4)
	expectScreen(t, term, "01234", "56789", "abc", "")
	expectCursor(t, term, 3, 0)
}

func TestRowsShrinkKeepsCursor(t *testing.T) {
	term := New(10, 5, 10)
	feed(term, "a\r\nb\r\nc\r\nd")
	term.Resize(10, 2)
	expectScreen(t, term, "c", "d")
	expectCursor(t, term, 1, 1)
	term.Resize(10, 5)
	expectScreen(t, term, "a", "b", "c", "d", "")
}

func TestSelectionText(t *testing.T) {
	term := New(5, 3, 10)
	feed(term, "helloworld\r\nab  ")
	b := term.buf
	top := b.ScreenTopAbs()
	got := b.TextRange(Pos{top, 2}, Pos{top + 1, 2}, false)
	if got != "llowo" {
		t.Fatalf("%q", got)
	}
	got = b.TextRange(Pos{top, 0}, Pos{top + 2, 5}, false)
	if got != "helloworld\nab" {
		t.Fatalf("%q", got)
	}
	s, e := b.WordAt(Pos{top, 3}, "")
	if s != (Pos{top, 0}) || e != (Pos{top + 1, 5}) {
		t.Fatalf("word %v %v", s, e)
	}
}

func TestSearch(t *testing.T) {
	term := New(8, 3, 10)
	feed(term, "Foo bar\r\nbazfoo!!x\r\n")
	m := term.buf.Search("foo", false)
	if len(m) != 2 {
		t.Fatalf("%v", m)
	}
	top := term.buf.ScreenTopAbs()
	if m[1].Start != (Pos{top, 3}) {
		t.Fatalf("%v", m[1])
	}
	m = term.buf.Search("!x", true)
	if len(m) != 1 || m[0].Start != (Pos{top, 7}) || m[0].End != (Pos{top + 1, 1}) {
		t.Fatalf("wrapped match %v", m)
	}
}

func TestURLAt(t *testing.T) {
	term := New(20, 2, 10)
	feed(term, "see https://x.org/a(b). ok")
	top := term.buf.ScreenTopAbs()
	u, _, ok := term.buf.URLAt(Pos{top + 1, 0})
	if !ok || u != "https://x.org/a(b)" {
		t.Fatalf("%q %v", u, ok)
	}
	if _, _, ok := term.buf.URLAt(Pos{top, 1}); ok {
		t.Fatal("no url at 'see'")
	}
}

func TestResizeAltNoReflow(t *testing.T) {
	term := feed(New(10, 3, 10), "\x1b[?1049habcdefghij")
	term.Resize(5, 3)
	expectScreen(t, term, "abcde", "", "")
	feed(term, "\x1b[?1049l")
	if term.altActive {
		t.Fatal("still alt")
	}
}

func FuzzParser(f *testing.F) {
	for _, g := range golden {
		f.Add([]byte(g.in))
	}
	f.Add([]byte("\x1b[38:2::1:2:3m\x1b[4:3m\x1b]8;;x\x07\x1b[?1049h\x1b[2;1r"))
	f.Fuzz(func(t *testing.T, data []byte) {
		term := New(13, 7, 20)
		term.Write(data)
		term.Resize(7, 11)
		term.Write(data)
		term.Resize(20, 3)
		term.Write(data)
		_ = screen(term)
		x, y := term.CursorPos()
		c, r := term.Size()
		if x < 0 || x >= c || y < 0 || y >= r {
			t.Fatalf("cursor out of range %d,%d", x, y)
		}
	})
}

// A wide character that does not fit at the end of a line wraps, leaving a
// padding cell; reflowing again must not turn that padding into a space.
func TestReflowWidePaddingNotContent(t *testing.T) {
	term := New(80, 4, 100)
	feed(term, " │ wide 世界 │ emoji 🚀 │\r\n")
	for _, w := range []int{9, 7, 5, 13, 80, 6, 62} {
		term.Resize(w, 4)
	}
	term.Resize(80, 4)
	expectScreen(t, term, " │ wide 世界 │ emoji 🚀 │", "", "", "")
}

func TestPrintWideWrapPaddingNotCopied(t *testing.T) {
	term := New(5, 3, 10)
	feed(term, "abcd世x")
	expectScreen(t, term, "abcd", "世x", "")
	top := term.buf.ScreenTopAbs()
	if got := term.buf.TextRange(Pos{top, 0}, Pos{top + 1, 3}, false); got != "abcd世x" {
		t.Fatalf("copied %q", got)
	}
	term.Resize(10, 3)
	expectScreen(t, term, "abcd世x", "", "")
}

func TestDefaultCursor(t *testing.T) {
	term := newT(10, 2)
	term.SetDefaultCursor(CursorBeam)
	if term.CursorStyle != CursorBeam {
		t.Fatal("default cursor not applied")
	}
	feed(term, "\x1b[4 q")
	term.SetDefaultCursor(CursorBlock) // the program's choice wins
	if term.CursorStyle != CursorUnderline || term.CursorBlink {
		t.Fatalf("program choice lost: %d", term.CursorStyle)
	}
	feed(term, "\x1b[0 q") // back to the user's default
	if term.CursorStyle != CursorBlock || term.CursorBlink {
		t.Fatalf("CSI 0 SP q: %d", term.CursorStyle)
	}
	feed(term, "\x1b[5 q\x1bc") // a full reset also returns to the default
	if term.CursorStyle != CursorBlock {
		t.Fatalf("reset: %d", term.CursorStyle)
	}
}
