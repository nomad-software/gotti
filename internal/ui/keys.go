package ui

import (
	"unicode"

	"github.com/nomad-software/qute/qtcore"
	"github.com/nomad-software/qute/qtgui"

	"termite/internal/config"
	"termite/internal/input"
)

var qtKeys = map[qtcore.Key]input.Key{
	qtcore.Key_Escape: input.KeyEscape, qtcore.Key_Tab: input.KeyTab, qtcore.Key_Backtab: input.KeyTab,
	qtcore.Key_Backspace: input.KeyBackspace, qtcore.Key_Return: input.KeyEnter, qtcore.Key_Enter: input.KeyEnter,
	qtcore.Key_Insert: input.KeyInsert, qtcore.Key_Delete: input.KeyDelete, qtcore.Key_Home: input.KeyHome,
	qtcore.Key_End: input.KeyEnd, qtcore.Key_Left: input.KeyLeft, qtcore.Key_Up: input.KeyUp,
	qtcore.Key_Right: input.KeyRight, qtcore.Key_Down: input.KeyDown, qtcore.Key_PageUp: input.KeyPageUp,
	qtcore.Key_PageDown: input.KeyPageDown,
	qtcore.Key_F1:       input.KeyF1, qtcore.Key_F2: input.KeyF2, qtcore.Key_F3: input.KeyF3, qtcore.Key_F4: input.KeyF4,
	qtcore.Key_F5: input.KeyF5, qtcore.Key_F6: input.KeyF6, qtcore.Key_F7: input.KeyF7, qtcore.Key_F8: input.KeyF8,
	qtcore.Key_F9: input.KeyF9, qtcore.Key_F10: input.KeyF10, qtcore.Key_F11: input.KeyF11, qtcore.Key_F12: input.KeyF12,
	qtcore.Key_Shift: input.KeyShift, qtcore.Key_Control: input.KeyControl, qtcore.Key_Alt: input.KeyAlt,
	qtcore.Key_Meta: input.KeySuper, qtcore.Key_Super_L: input.KeySuper, qtcore.Key_Super_R: input.KeySuper,
	qtcore.Key_CapsLock: input.KeyCapsLock, qtcore.Key_Menu: input.KeyMenu,
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

func qtMods(m qtcore.KeyboardModifier) input.Mods {
	var out input.Mods
	if m&qtcore.ShiftModifier != 0 {
		out |= input.Shift
	}
	if m&qtcore.ControlModifier != 0 {
		out |= input.Ctrl
	}
	if m&qtcore.AltModifier != 0 {
		out |= input.Alt
	}
	if m&qtcore.MetaModifier != 0 {
		out |= input.Super
	}
	return out
}

// translateKey converts a Qt key event to the toolkit-neutral form.
func translateKey(ev *qtgui.QKeyEvent, release bool) input.KeyEvent {
	k := ev.Key()
	mods := ev.Modifiers()
	out := input.KeyEvent{Mods: qtMods(mods), Action: input.Press, Keypad: mods&qtcore.KeypadModifier != 0}
	if release {
		out.Action = input.Release
	} else if ev.IsAutoRepeat() {
		out.Action = input.Repeat
	}
	if ik, ok := qtKeys[k]; ok {
		out.Key = ik
		if k == qtcore.Key_Backtab {
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
