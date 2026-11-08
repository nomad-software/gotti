// Package input encodes keyboard, mouse and paste events into the byte
// sequences xterm-compatible applications expect, including the kitty
// keyboard protocol. It is independent of the GUI toolkit.
package input

import (
	"strconv"
	"strings"
	"unicode"
)

// Mods is a modifier bitmask, numbered as in xterm / kitty.
type Mods uint8

const (
	Shift Mods = 1 << iota
	Alt
	Ctrl
	Super
)

// Key identifies a non-text key. Text keys use KeyRune.
type Key int

const (
	KeyNone Key = iota
	KeyRune
	KeyEnter
	KeyTab
	KeyBackspace
	KeyEscape
	KeyInsert
	KeyDelete
	KeyHome
	KeyEnd
	KeyPageUp
	KeyPageDown
	KeyUp
	KeyDown
	KeyLeft
	KeyRight
	KeyF1
	KeyF2
	KeyF3
	KeyF4
	KeyF5
	KeyF6
	KeyF7
	KeyF8
	KeyF9
	KeyF10
	KeyF11
	KeyF12
	KeyShift
	KeyControl
	KeyAlt
	KeySuper
	KeyCapsLock
	KeyMenu
)

// Actions.
const (
	Press = iota + 1
	Repeat
	Release
)

// KeyEvent describes one key event in toolkit-neutral terms.
type KeyEvent struct {
	Key     Key
	Rune    rune   // for KeyRune: the unshifted base key, lowercase
	Shifted rune   // the shifted key (when Shift is held), if known
	Text    string // text the key produces with the current modifiers
	Mods    Mods
	Action  int
	Keypad  bool // key is on the numeric keypad
}

// Modes are the terminal modes that affect key encoding.
type Modes struct {
	AppCursor  bool
	AppKeypad  bool
	KittyFlags int
}

// Kitty keyboard protocol flags.
const (
	KittyDisambiguate = 1
	KittyEventTypes   = 2
	KittyAlternate    = 4
	KittyAllKeys      = 8
	KittyText         = 16
)

func modParam(m Mods) int { return 1 + int(m&(Shift|Alt|Ctrl|Super)) }

// legacy CSI letter / tilde codes for functional keys.
type fkey struct {
	num    int  // number for CSI num ~ (0 if letter form)
	letter byte // final letter for CSI 1;m X form
	ss3    bool // unmodified form uses SS3 (F1-F4)
}

var fkeys = map[Key]fkey{
	KeyUp: {letter: 'A'}, KeyDown: {letter: 'B'}, KeyRight: {letter: 'C'}, KeyLeft: {letter: 'D'},
	KeyHome: {letter: 'H'}, KeyEnd: {letter: 'F'},
	KeyInsert: {num: 2}, KeyDelete: {num: 3}, KeyPageUp: {num: 5}, KeyPageDown: {num: 6},
	KeyF1: {letter: 'P', ss3: true}, KeyF2: {letter: 'Q', ss3: true}, KeyF3: {letter: 'R', ss3: true}, KeyF4: {letter: 'S', ss3: true},
	KeyF5: {num: 15}, KeyF6: {num: 17}, KeyF7: {num: 18}, KeyF8: {num: 19},
	KeyF9: {num: 20}, KeyF10: {num: 21}, KeyF11: {num: 23}, KeyF12: {num: 24},
}

// Encode returns the bytes to send for ev, or nil.
func Encode(ev KeyEvent, m Modes) []byte {
	if m.KittyFlags != 0 {
		return encodeKitty(ev, m)
	}
	if ev.Action == Release {
		return nil
	}
	return encodeLegacy(ev, m)
}

func encodeLegacy(ev KeyEvent, m Modes) []byte {
	mods := ev.Mods
	alt := mods&Alt != 0
	esc := func(b []byte) []byte {
		if alt {
			return append([]byte{0x1b}, b...)
		}
		return b
	}
	switch ev.Key {
	case KeyRune:
		if ev.Keypad && m.AppKeypad && mods == 0 {
			if s, ok := appKeypad[ev.Rune]; ok {
				return []byte("\x1bO" + s)
			}
		}
		if mods&Ctrl != 0 {
			if c, ok := ctrlByte(ev.Rune); ok {
				return esc([]byte{c})
			}
			if ev.Shifted != 0 {
				if c, ok := ctrlByte(ev.Shifted); ok {
					return esc([]byte{c})
				}
			}
		}
		text := ev.Text
		if text == "" && ev.Rune != 0 && mods&Ctrl == 0 {
			text = string(ev.Rune)
		}
		if text == "" {
			return nil
		}
		return esc([]byte(text))
	case KeyEnter:
		if ev.Keypad && m.AppKeypad {
			return []byte("\x1bOM")
		}
		return esc([]byte{'\r'})
	case KeyTab:
		if mods&Shift != 0 {
			return esc([]byte("\x1b[Z"))
		}
		return esc([]byte{'\t'})
	case KeyBackspace:
		if mods&Ctrl != 0 {
			return esc([]byte{0x08})
		}
		return esc([]byte{0x7f})
	case KeyEscape:
		return esc([]byte{0x1b})
	}
	f, ok := fkeys[ev.Key]
	if !ok {
		return nil
	}
	mp := modParam(mods)
	if f.num != 0 {
		if mp == 1 {
			return []byte("\x1b[" + strconv.Itoa(f.num) + "~")
		}
		return []byte("\x1b[" + strconv.Itoa(f.num) + ";" + strconv.Itoa(mp) + "~")
	}
	if mp == 1 {
		if f.ss3 || (m.AppCursor && ev.Key >= KeyHome && ev.Key <= KeyRight) {
			return []byte{0x1b, 'O', f.letter}
		}
		return []byte{0x1b, '[', f.letter}
	}
	return []byte("\x1b[1;" + strconv.Itoa(mp) + string(f.letter))
}

var appKeypad = map[rune]string{
	'0': "p", '1': "q", '2': "r", '3': "s", '4': "t", '5': "u", '6': "v", '7': "w", '8': "x", '9': "y",
	'+': "k", '-': "m", '*': "j", '/': "o", '.': "n",
}

// ctrlByte maps a key to its C0 control code as xterm does.
func ctrlByte(r rune) (byte, bool) {
	switch {
	case r >= 'a' && r <= 'z':
		return byte(r-'a') + 1, true
	case r >= 'A' && r <= 'Z':
		return byte(r-'A') + 1, true
	}
	switch r {
	case '@', ' ', '2':
		return 0, true
	case '[', '3':
		return 0x1b, true
	case '\\', '4':
		return 0x1c, true
	case ']', '5':
		return 0x1d, true
	case '^', '6', '~':
		return 0x1e, true
	case '_', '7', '/', '-':
		return 0x1f, true
	case '8', '?':
		return 0x7f, true
	}
	return 0, false
}

// kitty functional key numbers.
var kittyNum = map[Key]int{
	KeyEscape: 27, KeyEnter: 13, KeyTab: 9, KeyBackspace: 127,
	KeyInsert: 2, KeyDelete: 3, KeyPageUp: 5, KeyPageDown: 6,
	KeyF3: 13, KeyF5: 15, KeyF6: 17, KeyF7: 18, KeyF8: 19, KeyF9: 20, KeyF10: 21, KeyF11: 23, KeyF12: 24,
	KeyShift: 57441, KeyControl: 57442, KeyAlt: 57443, KeySuper: 57444, KeyCapsLock: 57358, KeyMenu: 57363,
}

var tildeKeys = map[Key]bool{
	KeyInsert: true, KeyDelete: true, KeyPageUp: true, KeyPageDown: true, KeyF3: true,
	KeyF5: true, KeyF6: true, KeyF7: true, KeyF8: true, KeyF9: true, KeyF10: true, KeyF11: true, KeyF12: true,
}

var kittyLetter = map[Key]byte{
	KeyUp: 'A', KeyDown: 'B', KeyRight: 'C', KeyLeft: 'D', KeyHome: 'H', KeyEnd: 'F',
	KeyF1: 'P', KeyF2: 'Q', KeyF4: 'S',
}

func encodeKitty(ev KeyEvent, m Modes) []byte {
	flags := m.KittyFlags
	all := flags&KittyAllKeys != 0
	reportEvents := flags&KittyEventTypes != 0
	if ev.Action == Release && !reportEvents {
		return nil
	}
	if ev.Action == 0 {
		ev.Action = Press
	}
	mods := ev.Mods
	isModKey := ev.Key >= KeyShift && ev.Key <= KeyMenu
	if isModKey && !all {
		return nil
	}

	// Keys that keep their legacy encoding unless flag 8 is set.
	if !all {
		switch ev.Key {
		case KeyRune:
			// Text producing keys without Ctrl/Alt/Super stay plain text.
			if mods&^Shift == 0 {
				if ev.Action == Release {
					return nil
				}
				if ev.Text != "" {
					return []byte(ev.Text)
				}
			}
		case KeyEnter, KeyTab, KeyBackspace:
			if mods == 0 {
				if ev.Action == Release {
					return nil
				}
				return encodeLegacy(ev, Modes{AppCursor: m.AppCursor})
			}
		}
	}

	var code int
	var letter byte
	switch {
	case ev.Key == KeyRune:
		r := ev.Rune
		if r == 0 {
			rs := []rune(ev.Text)
			if len(rs) == 0 {
				return nil
			}
			r = unicode.ToLower(rs[0])
		}
		code = int(r)
	case kittyNum[ev.Key] != 0:
		code = kittyNum[ev.Key]
	case kittyLetter[ev.Key] != 0:
		letter = kittyLetter[ev.Key]
		code = 1
	default:
		return nil
	}

	var sb strings.Builder
	sb.WriteString("\x1b[")
	if letter == 0 || mods != 0 || ev.Action != Press {
		sb.WriteString(strconv.Itoa(code))
	}
	if flags&KittyAlternate != 0 && ev.Key == KeyRune && mods&Shift != 0 && ev.Shifted != 0 && ev.Shifted != rune(code) {
		sb.WriteByte(':')
		sb.WriteString(strconv.Itoa(int(ev.Shifted)))
	}
	mp := modParam(mods)
	text := ""
	if flags&KittyText != 0 && all && ev.Key == KeyRune && ev.Action != Release {
		text = ev.Text
		if text != "" && (len(text) == 1 && text[0] < 0x20 || text == "\x7f") {
			text = ""
		}
	}
	needEvent := reportEvents && ev.Action != Press
	if mp != 1 || needEvent || text != "" {
		sb.WriteByte(';')
		if mp != 1 || needEvent {
			sb.WriteString(strconv.Itoa(mp))
		}
		if needEvent {
			sb.WriteByte(':')
			sb.WriteString(strconv.Itoa(ev.Action))
		}
	}
	if text != "" {
		sb.WriteByte(';')
		for i, r := range text {
			if i > 0 {
				sb.WriteByte(':')
			}
			sb.WriteString(strconv.Itoa(int(r)))
		}
	}
	switch {
	case letter != 0:
		sb.WriteByte(letter)
	case tildeKeys[ev.Key]:
		sb.WriteByte('~')
	default:
		sb.WriteByte('u')
	}
	return []byte(sb.String())
}

// ----------------------------------------------------------------- mouse

// Mouse buttons.
const (
	MouseLeft = iota
	MouseMiddle
	MouseRight
	MouseNone // motion without buttons
	MouseWheelUp
	MouseWheelDown
	MouseWheelLeft
	MouseWheelRight
)

// Mouse actions.
const (
	MousePress = iota
	MouseRelease
	MouseMotion
)

// EncodeMouse returns an SGR (?1006) mouse report. x and y are 0-based
// cell coordinates.
func EncodeMouse(button, action int, x, y int, mods Mods) []byte {
	var b int
	switch button {
	case MouseLeft, MouseMiddle, MouseRight:
		b = button
	case MouseNone:
		b = 3
	case MouseWheelUp:
		b = 64
	case MouseWheelDown:
		b = 65
	case MouseWheelLeft:
		b = 66
	case MouseWheelRight:
		b = 67
	}
	if action == MouseMotion {
		b += 32
	}
	if mods&Shift != 0 {
		b += 4
	}
	if mods&Alt != 0 {
		b += 8
	}
	if mods&Ctrl != 0 {
		b += 16
	}
	final := "M"
	if action == MouseRelease {
		final = "m"
	}
	return []byte("\x1b[<" + strconv.Itoa(b) + ";" + strconv.Itoa(x+1) + ";" + strconv.Itoa(y+1) + final)
}

// ----------------------------------------------------------------- paste

// Paste prepares clipboard text for sending. With bracketed paste it is
// wrapped in the paste markers (and any embedded end marker removed, so
// pasted text cannot escape the bracket); otherwise newlines become CR.
func Paste(text string, bracketed bool) []byte {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if bracketed {
		text = strings.ReplaceAll(text, "\x1b[201~", "")
		text = strings.ReplaceAll(text, "\x1b[200~", "")
		return []byte("\x1b[200~" + strings.ReplaceAll(text, "\n", "\r") + "\x1b[201~")
	}
	// Strip control characters that could act as commands, except tab/newline.
	clean := strings.Map(func(r rune) rune {
		if r == '\n' {
			return '\r'
		}
		if r < 0x20 && r != '\t' || r == 0x7f {
			return -1
		}
		return r
	}, text)
	return []byte(clean)
}

// IsMultiLine reports whether pasting text would submit more than one line.
func IsMultiLine(text string) bool {
	return strings.ContainsAny(strings.TrimRight(text, "\r\n"), "\r\n")
}
