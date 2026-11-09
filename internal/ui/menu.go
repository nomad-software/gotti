package ui

import (
	"math"
	"os"
	"strings"

	qt "github.com/mappu/miqt/qt6"

	"termite/internal/config"
	"termite/internal/desktop"
	"termite/internal/term"
)

// shortcutLabel returns the display form of the key bound to action
// ("Ctrl+Shift+C"), or "" when unbound.
func (a *App) shortcutLabel(action string) string {
	s := a.cfg.Keybindings[action]
	if s == "" || s == "none" {
		return ""
	}
	ch, err := config.ParseChord(s)
	if err != nil {
		return ""
	}
	names := map[string]string{
		"equal": "=", "minus": "-", "plus": "+", "pageup": "PgUp", "pagedown": "PgDown",
		"comma": ",", "period": ".", "slash": "/", "backslash": "\\", "semicolon": ";",
		"apostrophe": "'", "bracketleft": "[", "bracketright": "]", "grave": "`",
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
		parts = append(parts, "Super")
	}
	k := ch.Key
	if n, ok := names[k]; ok {
		k = n
	} else if len(k) > 0 {
		k = strings.ToUpper(k[:1]) + k[1:]
	}
	return strings.Join(append(parts, k), "+")
}

// menuBuilder adds actions with shortcut hints.
type menuBuilder struct {
	a *App
	m *qt.QMenu
}

func (b menuBuilder) item(label, action string, fn func()) *qt.QAction {
	text := label
	if sc := b.a.shortcutLabel(action); sc != "" {
		text += "\t" + sc
	}
	act := b.m.AddActionWithText(text)
	act.OnTriggered(fn)
	return act
}

func (b menuBuilder) check(label string, on bool, fn func(bool)) *qt.QAction {
	act := b.m.AddActionWithText(label)
	act.SetCheckable(true)
	act.SetChecked(on)
	act.OnTriggeredWithChecked(fn)
	return act
}

func (b menuBuilder) sub(title string) menuBuilder {
	return menuBuilder{a: b.a, m: b.m.AddMenuWithTitle(title)}
}

// radio adds an exclusive group of checkable items.
func (b menuBuilder) radio(labels, values []string, current string, fn func(string)) {
	g := qt.NewQActionGroup(b.m.QObject)
	g.SetExclusive(true)
	for i, l := range labels {
		v := values[i]
		act := b.m.AddActionWithText(l)
		act.SetCheckable(true)
		act.SetChecked(v == current)
		act.SetActionGroup(g)
		act.OnTriggered(func() { fn(v) })
	}
}

// showContextMenu opens the pane's context menu at global position (x, y).
// at is the cell that was clicked (nil when opened from the keyboard).
func (p *Pane) showContextMenu(x, y int, at *term.Pos) {
	a := p.app
	run := func(action string) func() { return func() { a.runAction(p, action) } }
	menu := qt.NewQMenu(p.Widget)
	b := menuBuilder{a: a, m: menu}

	// clipboard
	b.item("Copy", "copy", run("copy")).SetEnabled(p.view.Sel.Active)
	b.item("Paste", "paste", run("paste"))
	b.item("Paste Selection", "paste_selection", run("paste_selection"))
	if at != nil {
		if url := p.linkAt(*at); url != "" {
			menu.AddSeparator()
			b.item("Open Link", "", func() { openURL(url) })
			b.item("Copy Link Address", "", func() { qt.QGuiApplication_Clipboard().SetText(url) })
		}
	}
	menu.AddSeparator()

	// panes
	b.item("Split Horizontally", "split_horizontal", run("split_horizontal"))
	b.item("Split Vertically", "split_vertical", run("split_vertical"))
	menu.AddSeparator()

	// broadcast
	bc := b.sub("Broadcast Input")
	mode := map[int]string{BroadcastOff: "off", BroadcastAll: "all", BroadcastGroup: "group"}[a.broadcast]
	bc.radio([]string{"Off", "All Panes in Tab", "Group"}, []string{"off", "all", "group"}, mode, func(v string) {
		a.broadcast = map[string]int{"off": BroadcastOff, "all": BroadcastAll, "group": BroadcastGroup}[v]
		a.refreshBorders()
	})
	bc.m.AddSeparator()
	bc.check("This Pane in Group", p.InGroup, func(on bool) {
		p.InGroup = on
		a.refreshBorders()
	})

	// layouts
	lm := b.sub("Layouts")
	lm.item("Save…", "save_layout", run("save_layout"))
	if names := ListLayouts(); len(names) > 0 {
		w := p.tab.win
		rm := lm.sub("Remove")
		for _, n := range names {
			rm.item(n, "", func() {
				if qt.QMessageBox_Question(w.w, "Remove Layout", "Remove the layout \""+n+"\"?") != qt.QMessageBox__Yes {
					return
				}
				if err := RemoveLayout(n); err != nil {
					qt.QMessageBox_Warning(w.w, desktop.DisplayName, err.Error())
				}
			})
		}
		lm.m.AddSection("Open")
		for _, n := range names {
			lm.item(n, "", func() {
				lf, err := LoadLayout(n)
				if err != nil {
					qt.QMessageBox_Warning(w.w, desktop.DisplayName, err.Error())
					return
				}
				w.loadLayout(lf)
			})
		}
	}
	menu.AddSeparator()

	// view
	findChosen := false
	b.item("Find…", "search", func() { findChosen = true; a.runAction(p, "search") })
	b.item("Clear Scrollback", "clear_scrollback", run("clear_scrollback"))
	b.item("Reset Terminal", "reset_terminal", run("reset_terminal"))
	menu.AddSeparator()

	b.item("Options…", "options", run("options"))

	pos := qt.NewQPoint2(x, y)
	menu.ExecWithPos(pos)
	pos.Delete()
	menu.DeleteLater()
	if p.exited {
		return
	}
	p.refocus()
	if findChosen {
		p.searchBar.SetFocus() // refocus gave it back to the terminal
	}
}

// contextMenuAtCursor opens the menu at the text cursor (keyboard).
func (p *Pane) contextMenuAtCursor() {
	p.T.Mu.Lock()
	cx, cy := p.T.CursorPos()
	p.T.Mu.Unlock()
	cw, ch := p.R.CellSize()
	st := p.R.Style
	lx := int(float64(st.PadX+cx*cw) / p.dpr)
	ly := int(float64(st.PadY+(cy+1)*ch) / p.dpr)
	gx, gy := mapToGlobal(p.GL.QWidget, lx, ly)
	p.showContextMenu(gx, gy, nil)
}

// editConfig opens the config file in the desktop's editor, creating it
// with the defaults first if needed.
func (a *App) editConfig(p *Pane) {
	if _, err := os.Stat(a.cfgPath); os.IsNotExist(err) {
		if err := os.MkdirAll(config.Dir(), 0o755); err == nil {
			_ = os.WriteFile(a.cfgPath, config.SampleConfig(), 0o644)
		}
	}
	if !qt.QDesktopServices_OpenUrl(qt.QUrl_FromLocalFile(a.cfgPath)) {
		qt.QMessageBox_Information(p.Widget, desktop.DisplayName, "Config file: "+a.cfgPath)
		p.refocus()
	}
}

// isMonospaced checks whether narrow and wide letters have equal advances.
func isMonospaced(f *qt.QFont) bool {
	fm := qt.NewQFontMetricsF(f)
	defer fm.Delete()
	i, m, w := fm.HorizontalAdvance("i"), fm.HorizontalAdvance("M"), fm.HorizontalAdvance("W")
	return math.Abs(i-m) < 0.5 && math.Abs(m-w) < 0.5
}
