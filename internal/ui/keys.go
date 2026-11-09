package ui

import (
	"unicode"

	qt "github.com/mappu/miqt/qt6"

	"termite/internal/config"
	"termite/internal/input"
)

var qtKeys = map[qt.Key]input.Key{
	qt.Key_Escape: input.KeyEscape, qt.Key_Tab: input.KeyTab, qt.Key_Backtab: input.KeyTab,
	qt.Key_Backspace: input.KeyBackspace, qt.Key_Return: input.KeyEnter, qt.Key_Enter: input.KeyEnter,
	qt.Key_Insert: input.KeyInsert, qt.Key_Delete: input.KeyDelete, qt.Key_Home: input.KeyHome,
	qt.Key_End: input.KeyEnd, qt.Key_Left: input.KeyLeft, qt.Key_Up: input.KeyUp,
	qt.Key_Right: input.KeyRight, qt.Key_Down: input.KeyDown, qt.Key_PageUp: input.KeyPageUp,
	qt.Key_PageDown: input.KeyPageDown,
	qt.Key_F1:       input.KeyF1, qt.Key_F2: input.KeyF2, qt.Key_F3: input.KeyF3, qt.Key_F4: input.KeyF4,
	qt.Key_F5: input.KeyF5, qt.Key_F6: input.KeyF6, qt.Key_F7: input.KeyF7, qt.Key_F8: input.KeyF8,
	qt.Key_F9: input.KeyF9, qt.Key_F10: input.KeyF10, qt.Key_F11: input.KeyF11, qt.Key_F12: input.KeyF12,
	qt.Key_Shift: input.KeyShift, qt.Key_Control: input.KeyControl, qt.Key_Alt: input.KeyAlt,
	qt.Key_Meta: input.KeySuper, qt.Key_Super_L: input.KeySuper, qt.Key_Super_R: input.KeySuper,
	qt.Key_CapsLock: input.KeyCapsLock, qt.Key_Menu: input.KeyMenu,
}

var keyNames = map[input.Key]string{
	input.KeyEscape: "escape", input.KeyTab: "tab", input.KeyBackspace: "backspace", input.KeyEnter: "enter",
	input.KeyInsert: "insert", input.KeyDelete: "delete", input.KeyHome: "home", input.KeyEnd: "end",
	input.KeyLeft: "left", input.KeyUp: "up", input.KeyRight: "right", input.KeyDown: "down",
	input.KeyPageUp: "pageup", input.KeyPageDown: "pagedown",
	input.KeyF1: "f1", input.KeyF2: "f2", input.KeyF3: "f3", input.KeyF4: "f4", input.KeyF5: "f5",
	input.KeyF6: "f6", input.KeyF7: "f7", input.KeyF8: "f8", input.KeyF9: "f9", input.KeyF10: "f10",
	input.KeyF11: "f11", input.KeyF12: "f12", input.KeyMenu: "menu",
}

var symbolNames = map[rune]string{
	'=': "equal", '+': "plus", '-': "minus", ',': "comma", '.': "period", '/': "slash", '\\': "backslash",
	';': "semicolon", '\'': "apostrophe", '[': "bracketleft", ']': "bracketright", '`': "grave", ' ': "space",
}

// US-layout shifted symbols, used to recover the base key that kitty's
// protocol and key bindings refer to.
var unshift = map[rune]rune{
	'!': '1', '@': '2', '#': '3', '$': '4', '%': '5', '^': '6', '&': '7', '*': '8', '(': '9', ')': '0',
	'_': '-', '+': '=', '{': '[', '}': ']', '|': '\\', ':': ';', '"': '\'', '<': ',', '>': '.', '?': '/', '~': '`',
}

func qtMods(m qt.KeyboardModifier) input.Mods {
	var out input.Mods
	if m&qt.ShiftModifier != 0 {
		out |= input.Shift
	}
	if m&qt.ControlModifier != 0 {
		out |= input.Ctrl
	}
	if m&qt.AltModifier != 0 {
		out |= input.Alt
	}
	if m&qt.MetaModifier != 0 {
		out |= input.Super
	}
	return out
}

// translateKey converts a Qt key event to the toolkit-neutral form.
func translateKey(ev *qt.QKeyEvent, release bool) input.KeyEvent {
	k := qt.Key(ev.Key())
	mods := ev.Modifiers()
	out := input.KeyEvent{Mods: qtMods(mods), Action: input.Press, Keypad: mods&qt.KeypadModifier != 0}
	if release {
		out.Action = input.Release
	} else if ev.IsAutoRepeat() {
		out.Action = input.Repeat
	}
	if ik, ok := qtKeys[k]; ok {
		out.Key = ik
		if k == qt.Key_Backtab {
			out.Mods |= input.Shift
		}
		return out
	}
	text := ev.Text()
	out.Key = input.KeyRune
	// Qt reports letters as upper case key codes and symbols as the
	// shifted character; derive the unshifted base key.
	if k > 0 && k < 0x110000 {
		r := rune(k)
		base := unicode.ToLower(r)
		if b, ok := unshift[r]; ok {
			base = b
		}
		out.Rune = base
		if out.Mods&input.Shift != 0 {
			if _, ok := unshift[r]; ok {
				out.Shifted = r
			} else {
				out.Shifted = unicode.ToUpper(r)
			}
		}
	}
	// Control characters in text are ignored: the encoder derives them.
	if text != "" {
		rs := []rune(text)
		if len(rs) == 1 && (rs[0] < 0x20 || rs[0] == 0x7f) {
			text = ""
		}
	}
	out.Text = text
	if out.Rune == 0 && text != "" {
		out.Rune = unicode.ToLower([]rune(text)[0])
	}
	return out
}

// chordOf returns the key-binding chord for an event.
func chordOf(ev input.KeyEvent) (config.Chord, bool) {
	c := config.Chord{
		Ctrl: ev.Mods&input.Ctrl != 0, Shift: ev.Mods&input.Shift != 0,
		Alt: ev.Mods&input.Alt != 0, Super: ev.Mods&input.Super != 0,
	}
	if ev.Key == input.KeyRune {
		r := ev.Rune
		if r == 0 {
			return c, false
		}
		if n, ok := symbolNames[r]; ok {
			c.Key = n
		} else {
			c.Key = string(r)
		}
		return c, true
	}
	n, ok := keyNames[ev.Key]
	c.Key = n
	return c, ok
}
