package ui

import (
	"sort"
	"strings"
	"unicode/utf8"

	qt "github.com/mappu/miqt/qt6"

	"termite/internal/config"
)

// shortcutActions lists the bindable actions in the order the options
// dialog shows them, with their labels.
var shortcutActions = []struct{ action, label string }{
	{"copy", "Copy"},
	{"paste", "Paste"},
	{"paste_selection", "Paste selection"},
	{"search", "Find"},
	{"split_horizontal", "Split horizontally"},
	{"split_vertical", "Split vertically"},
	{"close_pane", "Close pane"},
	{"focus_left", "Focus pane to the left"},
	{"focus_right", "Focus pane to the right"},
	{"focus_up", "Focus pane above"},
	{"focus_down", "Focus pane below"},
	{"new_tab", "New tab"},
	{"next_tab", "Next tab"},
	{"prev_tab", "Previous tab"},
	{"move_tab_left", "Move tab left"},
	{"move_tab_right", "Move tab right"},
	{"new_window", "New window"},
	{"scroll_page_up", "Scroll page up"},
	{"scroll_page_down", "Scroll page down"},
	{"scroll_top", "Scroll to top"},
	{"scroll_bottom", "Scroll to bottom"},
	{"font_bigger", "Zoom in"},
	{"font_smaller", "Zoom out"},
	{"font_reset", "Reset zoom"},
	{"broadcast_all", "Broadcast to all panes in tab"},
	{"broadcast_group", "Broadcast to group"},
	{"group_toggle", "Add or remove pane from group"},
	{"save_layout", "Save layout"},
	{"load_layout", "Load layout"},
	{"clear_scrollback", "Clear scrollback"},
	{"reset_terminal", "Reset terminal"},
	{"context_menu", "Open the context menu"},
	{"options", "Options"},
	{"edit_config", "Edit config file"},
	{"reload_config", "Reload config"},
}

// Key names in Qt's portable key-sequence text and termite's chord names.
var qtKeyToChord = map[string]string{
	"pgup": "pageup", "pgdown": "pagedown", "del": "delete", "ins": "insert", "esc": "escape",
	"return": "enter", "enter": "enter", "backtab": "tab",
	"=": "equal", "+": "plus", "-": "minus", ",": "comma", ".": "period", "/": "slash", "\\": "backslash",
	";": "semicolon", "'": "apostrophe", "[": "bracketleft", "]": "bracketright", "`": "grave",
}

var chordKeyToQt = map[string]string{
	"pageup": "PgUp", "pagedown": "PgDown", "delete": "Del", "insert": "Ins", "escape": "Esc",
	"enter": "Return", "space": "Space", "menu": "Menu", "tab": "Tab", "backspace": "Backspace",
	"home": "Home", "end": "End", "left": "Left", "right": "Right", "up": "Up", "down": "Down",
	"equal": "=", "plus": "+", "minus": "-", "comma": ",", "period": ".", "slash": "/", "backslash": "\\",
	"semicolon": ";", "apostrophe": "'", "bracketleft": "[", "bracketright": "]", "grave": "`",
}

var modOrder = map[string]int{"ctrl": 0, "shift": 1, "alt": 2, "super": 3}

// qtToChord converts Qt portable text ("Ctrl+Shift+PgUp") to a chord
// string ("ctrl+shift+pageup"). Only the first key of a sequence is used.
// Shifted symbols are recorded by their base key, as termite matches them.
func qtToChord(s string) string {
	s, _, _ = strings.Cut(s, ", ")
	if s == "" {
		return ""
	}
	key := s
	var mods []string
	for {
		i := strings.Index(key, "+")
		if i <= 0 || i == len(key)-1 { // "+" itself is the key
			break
		}
		mod := strings.ToLower(key[:i])
		switch mod {
		case "ctrl", "shift", "alt":
		case "meta":
			mod = "super"
		default:
			return "" // not a modifier: malformed
		}
		mods = append(mods, mod)
		key = key[i+1:]
	}
	shift := false
	for _, m := range mods {
		shift = shift || m == "shift"
	}
	if r, n := utf8.DecodeRuneInString(key); n == len(key) {
		if b, ok := unshift[r]; ok && shift {
			key = string(b)
		}
	}
	k := strings.ToLower(key)
	if n, ok := qtKeyToChord[k]; ok {
		k = n
	}
	// keep config order: ctrl, shift, alt, super
	sort.Slice(mods, func(i, j int) bool { return modOrder[mods[i]] < modOrder[mods[j]] })
	return strings.Join(append(mods, k), "+")
}

// chordToQt converts a chord string to Qt portable text.
func chordToQt(s string) string { return chordText(s, "Meta") }

// chordText spells a chord string as Qt does ("Ctrl+Shift+PgUp"), calling
// the Super key super; "" for no chord.
func chordText(s, super string) string {
	if s == "" || s == "none" {
		return ""
	}
	ch, err := config.ParseChord(s)
	if err != nil {
		return ""
	}
	var parts []string
	if ch.Ctrl {
		parts = append(parts, "Ctrl")
	}
	if ch.Shift {
		parts = append(parts, "Shift")
	}
	if ch.Alt {
		parts = append(parts, "Alt")
	}
	if ch.Super {
		parts = append(parts, super)
	}
	k := ch.Key
	if n, ok := chordKeyToQt[k]; ok {
		k = n
	} else {
		k = strings.ToUpper(k[:1]) + k[1:]
	}
	return strings.Join(append(parts, k), "+")
}

// shortcutsTab builds the options dialog's keyboard shortcut page. read
// returns the bindings as edited, with any actions that share a key.
func (a *App) shortcutsTab() (page *qt.QWidget, read func() (map[string]string, []string)) {
	page = qt.NewQWidget(nil)
	outer := qt.NewQVBoxLayout(page)
	box, sf := optionsSection(outer, "Shortcuts", "Click a shortcut and press the new keys. Clear it to leave the action unbound.")

	scroll := qt.NewQScrollArea(page)
	scroll.SetWidgetResizable(true)
	inner := qt.NewQWidget(nil)
	form := qt.NewQFormLayout(inner)
	edits := make([]*qt.QKeySequenceEdit, len(shortcutActions))
	for i, sa := range shortcutActions {
		e := qt.NewQKeySequenceEdit(inner)
		e.SetClearButtonEnabled(true)
		setKeySequence(e, chordToQt(a.cfg.Keybindings[sa.action]))
		// One chord per action: drop anything typed after the first.
		e.OnKeySequenceChanged(func(ks *qt.QKeySequence) {
			if ks.Count() > 1 {
				first, _, _ := strings.Cut(ks.ToString(), ", ")
				setKeySequence(e, first)
			}
		})
		edits[i] = e
		form.AddRow3(sa.label+":", e.QWidget)
	}
	scroll.SetWidget(inner)
	sf.AddRowWithWidget(scroll.QWidget)

	defaults := qt.NewQPushButton3("Restore Defaults")
	defaults.OnClicked(func() {
		for i, sa := range shortcutActions {
			setKeySequence(edits[i], chordToQt(config.DefaultKeybindings[sa.action]))
		}
	})
	row := qt.NewQHBoxLayout(nil)
	row.AddStretch()
	row.AddWidget(defaults.QWidget)
	sf.AddRowWithLayout(row.QLayout)
	// The list takes what room the page has.
	outer.SetStretchFactor(box.QWidget, 1)

	read = func() (map[string]string, []string) {
		out := map[string]string{}
		byChord := map[string][]string{}
		for i, sa := range shortcutActions {
			c := qtToChord(edits[i].KeySequence().ToString())
			out[sa.action] = c
			if c != "" {
				byChord[c] = append(byChord[c], sa.label)
			}
		}
		var clashes []string
		for c, labels := range byChord {
			if len(labels) > 1 {
				clashes = append(clashes, chordToQt(c)+": "+strings.Join(labels, ", "))
			}
		}
		sort.Strings(clashes)
		return out, clashes
	}
	return page, read
}
