package input

import (
	"strings"
	"testing"
)

func TestLegacy(t *testing.T) {
	cases := []struct {
		ev   KeyEvent
		m    Modes
		want string
	}{
		{KeyEvent{Key: KeyRune, Rune: 'a', Text: "a"}, Modes{}, "a"},
		{KeyEvent{Key: KeyRune, Rune: 'a', Text: "A", Mods: Shift}, Modes{}, "A"},
		{KeyEvent{Key: KeyRune, Rune: 'c', Mods: Ctrl}, Modes{}, "\x03"},
		{KeyEvent{Key: KeyRune, Rune: 'c', Mods: Ctrl | Shift}, Modes{}, "\x03"},
		{KeyEvent{Key: KeyRune, Rune: 'x', Text: "x", Mods: Alt}, Modes{}, "\x1bx"},
		{KeyEvent{Key: KeyRune, Rune: 'x', Mods: Alt | Ctrl}, Modes{}, "\x1b\x18"},
		{KeyEvent{Key: KeyRune, Rune: ' ', Mods: Ctrl}, Modes{}, "\x00"},
		{KeyEvent{Key: KeyRune, Rune: '[', Mods: Ctrl}, Modes{}, "\x1b"},
		{KeyEvent{Key: KeyRune, Rune: 'é', Text: "é"}, Modes{}, "é"},
		{KeyEvent{Key: KeyEnter}, Modes{}, "\r"},
		{KeyEvent{Key: KeyEnter, Mods: Alt}, Modes{}, "\x1b\r"},
		{KeyEvent{Key: KeyTab, Mods: Shift}, Modes{}, "\x1b[Z"},
		{KeyEvent{Key: KeyBackspace}, Modes{}, "\x7f"},
		{KeyEvent{Key: KeyBackspace, Mods: Ctrl}, Modes{}, "\x08"},
		{KeyEvent{Key: KeyUp}, Modes{}, "\x1b[A"},
		{KeyEvent{Key: KeyUp}, Modes{AppCursor: true}, "\x1bOA"},
		{KeyEvent{Key: KeyUp, Mods: Ctrl}, Modes{AppCursor: true}, "\x1b[1;5A"},
		{KeyEvent{Key: KeyLeft, Mods: Shift | Alt}, Modes{}, "\x1b[1;4D"},
		{KeyEvent{Key: KeyHome}, Modes{}, "\x1b[H"},
		{KeyEvent{Key: KeyEnd, Mods: Ctrl}, Modes{}, "\x1b[1;5F"},
		{KeyEvent{Key: KeyDelete}, Modes{}, "\x1b[3~"},
		{KeyEvent{Key: KeyPageUp, Mods: Shift}, Modes{}, "\x1b[5;2~"},
		{KeyEvent{Key: KeyF1}, Modes{}, "\x1bOP"},
		{KeyEvent{Key: KeyF1, Mods: Ctrl}, Modes{}, "\x1b[1;5P"},
		{KeyEvent{Key: KeyF5}, Modes{}, "\x1b[15~"},
		{KeyEvent{Key: KeyF12, Mods: Shift}, Modes{}, "\x1b[24;2~"},
		{KeyEvent{Key: KeyRune, Rune: '5', Text: "5", Keypad: true}, Modes{AppKeypad: true}, "\x1bOu"},
		{KeyEvent{Key: KeyRune, Rune: 'a', Action: Release}, Modes{}, ""},
	}
	for _, c := range cases {
		if got := string(Encode(c.ev, c.m)); got != c.want {
			t.Errorf("%+v: got %q want %q", c.ev, got, c.want)
		}
	}
}

func TestKitty(t *testing.T) {
	d := Modes{KittyFlags: KittyDisambiguate}
	ev := Modes{KittyFlags: KittyDisambiguate | KittyEventTypes}
	all := Modes{KittyFlags: KittyDisambiguate | KittyAllKeys | KittyText | KittyEventTypes}
	alt := Modes{KittyFlags: KittyDisambiguate | KittyAlternate}
	cases := []struct {
		ev   KeyEvent
		m    Modes
		want string
	}{
		{KeyEvent{Key: KeyRune, Rune: 'a', Text: "a"}, d, "a"},
		{KeyEvent{Key: KeyRune, Rune: 'a', Text: "A", Mods: Shift}, d, "A"},
		{KeyEvent{Key: KeyRune, Rune: 'c', Mods: Ctrl}, d, "\x1b[99;5u"},
		{KeyEvent{Key: KeyRune, Rune: 'i', Mods: Ctrl}, d, "\x1b[105;5u"},
		{KeyEvent{Key: KeyEscape}, d, "\x1b[27u"},
		{KeyEvent{Key: KeyEnter}, d, "\r"},
		{KeyEvent{Key: KeyEnter, Mods: Shift}, d, "\x1b[13;2u"},
		{KeyEvent{Key: KeyTab, Mods: Ctrl}, d, "\x1b[9;5u"},
		{KeyEvent{Key: KeyUp}, d, "\x1b[A"},
		{KeyEvent{Key: KeyUp, Mods: Shift}, d, "\x1b[1;2A"},
		{KeyEvent{Key: KeyF3}, d, "\x1b[13~"},
		{KeyEvent{Key: KeyF1}, d, "\x1b[P"},
		{KeyEvent{Key: KeyDelete, Mods: Ctrl}, d, "\x1b[3;5~"},
		{KeyEvent{Key: KeyRune, Rune: 'a', Mods: Ctrl, Action: Release}, ev, "\x1b[97;5:3u"},
		{KeyEvent{Key: KeyRune, Rune: 'a', Mods: Ctrl, Action: Repeat}, ev, "\x1b[97;5:2u"},
		{KeyEvent{Key: KeyUp, Action: Release}, ev, "\x1b[1;1:3A"},
		{KeyEvent{Key: KeyRune, Rune: 'a', Text: "a", Action: Release}, ev, ""},
		{KeyEvent{Key: KeyRune, Rune: 'a', Text: "a"}, all, "\x1b[97;;97u"},
		{KeyEvent{Key: KeyRune, Rune: 'a', Text: "A", Shifted: 'A', Mods: Shift}, all, "\x1b[97;2;65u"},
		{KeyEvent{Key: KeyEnter}, all, "\x1b[13u"},
		{KeyEvent{Key: KeyShift, Mods: Shift}, all, "\x1b[57441;2u"},
		{KeyEvent{Key: KeyRune, Rune: 'a', Shifted: 'A', Mods: Shift | Ctrl}, alt, "\x1b[97:65;6u"},
	}
	for _, c := range cases {
		if got := string(Encode(c.ev, c.m)); got != c.want {
			t.Errorf("%+v flags=%d: got %q want %q", c.ev, c.m.KittyFlags, got, c.want)
		}
	}
}

func TestMouse(t *testing.T) {
	if got := string(EncodeMouse(MouseLeft, MousePress, 0, 0, 0, true)); got != "\x1b[<0;1;1M" {
		t.Error(got)
	}
	if got := string(EncodeMouse(MouseLeft, MouseRelease, 9, 4, Ctrl, true)); got != "\x1b[<16;10;5m" {
		t.Error(got)
	}
	if got := string(EncodeMouse(MouseLeft, MouseMotion, 1, 1, 0, true)); got != "\x1b[<32;2;2M" {
		t.Error(got)
	}
	if got := string(EncodeMouse(MouseWheelDown, MousePress, 1, 1, Shift, true)); got != "\x1b[<69;2;2M" {
		t.Error(got)
	}
}

// TestMouseX10 checks the original form of report, for programs that
// don't ask for SGR: CSI M and three bytes, each 32 more than its value.
func TestMouseX10(t *testing.T) {
	for _, tc := range []struct {
		button, action, x, y int
		mods                 Mods
		want                 string
	}{
		{MouseLeft, MousePress, 0, 0, 0, "\x1b[M !!"},
		{MouseRight, MousePress, 9, 4, Ctrl, "\x1b[M2*%"},       // 32+2+16, 32+10, 32+5
		{MouseRight, MouseRelease, 9, 4, Ctrl, "\x1b[M3*%"},     // release: button 3
		{MouseLeft, MouseMotion, 1, 1, 0, "\x1b[M@\"\""},        // 32+32
		{MouseWheelUp, MousePress, 0, 0, Shift, "\x1b[Md!!"},    // 32+64+4
		{MouseLeft, MousePress, 222, 222, 0, "\x1b[M \xff\xff"}, // the last cell it reaches
	} {
		if got := string(EncodeMouse(tc.button, tc.action, tc.x, tc.y, tc.mods, false)); got != tc.want {
			t.Errorf("%+v: %q, want %q", tc, got, tc.want)
		}
	}
	if got := EncodeMouse(MouseLeft, MousePress, 223, 0, 0, false); got != nil {
		t.Errorf("past column 223: %q", got)
	}
}

func TestPaste(t *testing.T) {
	if got := string(Paste("a\nb\x1b[201~c", true)); got != "\x1b[200~a\rbc\x1b[201~" {
		t.Errorf("%q", got)
	}
	if got := string(Paste("a\r\nb\x1bc", false)); got != "a\rbc" {
		t.Errorf("%q", got)
	}
	if !IsMultiLine("a\nb") || IsMultiLine("ab\n") {
		t.Error("multiline detection")
	}
}

// TestPasteCannotEscapeBrackets pastes text built so that removing a paste
// marker once leaves another behind. Nothing between the brackets may end
// the paste early, or the rest would run as typed commands.
func TestPasteCannotEscapeBrackets(t *testing.T) {
	for _, in := range []string{
		"\x1b[20\x1b[201~1~rm -rf ~\n",
		"\x1b[20\x1b[200~1~x",
		"\x1b[2\x1b[20\x1b[201~1~1~x",
		"\x1b[201~\x1b[201~",
	} {
		out := string(Paste(in, true))
		inner := strings.TrimSuffix(strings.TrimPrefix(out, "\x1b[200~"), "\x1b[201~")
		if strings.Contains(inner, "\x1b[201~") || strings.Contains(inner, "\x1b[200~") {
			t.Errorf("%q pastes as %q, which ends the paste early", in, out)
		}
	}
}
