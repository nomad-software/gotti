package ui

import (
	"github.com/nomad-software/qute/qtcore"
	"github.com/nomad-software/qute/qtgui"
	"github.com/nomad-software/qute/qtwidgets"

	"termite/internal/config"
	"termite/internal/desktop"
	"termite/internal/term"
)

// shortcutLabel returns the display form of the key bound to action
// ("Ctrl+Shift+C"), or "" when unbound.
func (a *App) shortcutLabel(action string) string {
	return chordText(a.cfg.Keybindings[action], "Super")
}

// menuBuilder adds actions with shortcut hints.
type menuBuilder struct {
	a *App
	m *qtwidgets.QMenu
}

func (b menuBuilder) item(label, action string, fn func()) *qtgui.QAction {
	text := label
	if sc := b.a.shortcutLabel(action); sc != "" {
		text += "\t" + sc
	}
	act := b.m.AddActionText(text)
	act.OnTriggered(func(bool) { fn() })
	return act
}

func (b menuBuilder) check(label string, on bool, fn func(bool)) *qtgui.QAction {
	act := b.m.AddActionText(label)
	act.SetCheckable(true)
	act.SetChecked(on)
	act.OnTriggered(fn)
	return act
}

func (b menuBuilder) sub(title string) menuBuilder {
	return menuBuilder{a: b.a, m: b.m.AddMenuTitle(title)}
}

// radio adds an exclusive group of checkable items.
func (b menuBuilder) radio(labels []string, values []int, current int, fn func(int)) {
	g := qtgui.NewQActionGroup(b.m)
	g.SetExclusive(true)
	for i, l := range labels {
		v := values[i]
		act := b.m.AddActionText(l)
		act.SetCheckable(true)
		act.SetChecked(v == current)
		act.SetActionGroup(g)
		act.OnTriggered(func(bool) { fn(v) })
	}
}

// showContextMenu opens the pane's context menu at global position (x, y).
// at is the cell that was clicked (nil when opened from the keyboard).
func (p *Pane) showContextMenu(x, y int, at *term.Pos) {
	a := p.app
	run := func(action string) func() { return func() { a.runAction(p, action) } }
	menu := qtwidgets.NewQMenu(p.Widget)
	b := menuBuilder{a: a, m: menu}

	// clipboard
	b.item("Copy", "copy", run("copy")).SetEnabled(p.view.Sel.Active)
	b.item("Paste", "paste", run("paste"))
	if at != nil {
		if url := p.linkAt(*at); url != "" {
			menu.AddSeparator()
			b.item("Open Link", "", func() { openURL(url) })
			b.item("Copy Link Address", "", func() { qtgui.QGuiApplicationClipboard().SetText(url) })
		}
	}
	menu.AddSeparator()

	// panes
	b.item("Split Horizontally", "split_horizontal", run("split_horizontal"))
	b.item("Split Vertically", "split_vertical", run("split_vertical"))
	menu.AddSeparator()

	// broadcast
	bc := b.sub("Broadcast Input")
	var labels []string
	var modes []int
	for _, m := range broadcastModes {
		labels = append(labels, m.label)
		modes = append(modes, m.mode)
	}
	bc.radio(labels, modes, a.broadcast, a.setBroadcast)
	bc.m.AddSeparator()
	bc.check("This Pane in Group", p.InGroup, func(on bool) {
		p.InGroup = on
		a.refreshBroadcast()
	})

	// layouts
	lm := b.sub("Layouts")
	lm.item("Save…", "save_layout", run("save_layout"))
	if names := ListLayouts(); len(names) > 0 {
		w := p.tab.win
		rm := lm.sub("Remove")
		for _, n := range names {
			rm.item(n, "", func() {
				if qtwidgets.QMessageBoxQuestion(w.w, "Remove Layout", "Remove the layout \""+n+"\"?") != qtwidgets.QMessageBoxYes {
					return
				}
				if err := RemoveLayout(n); err != nil {
					qtwidgets.QMessageBoxWarning(w.w, desktop.DisplayName, err.Error())
				}
			})
		}
		lm.m.AddSection("Open")
		for _, n := range names {
			lm.item(n, "", func() {
				lf, err := LoadLayout(n)
				if err != nil {
					qtwidgets.QMessageBoxWarning(w.w, desktop.DisplayName, err.Error())
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

	menu.ExecAt(qtcore.QPoint{X: x, Y: y})
	// The menu goes with the pane if its shell exits while it is open.
	if menu.Alive() {
		menu.DeleteLater()
	}
	if p.exited {
		return
	}
	// Focus goes back to the tab's focused pane, which is a new pane after
	// a split.
	if f := p.tab.focused; f != nil && !f.exited {
		f.refocus()
	} else {
		p.refocus()
	}
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
	gx, gy := mapToGlobal(p.GL, lx, ly)
	p.showContextMenu(gx, gy, nil)
}

// editConfig opens the config file in the desktop's editor, creating it
// with the defaults first if needed.
func (a *App) editConfig(p *Pane) {
	_ = config.WriteSample(a.cfgPath) // fails harmlessly if it exists
	if !qtgui.QDesktopServicesOpenUrl(qtcore.QUrlFromLocalFile(a.cfgPath)) {
		qtwidgets.QMessageBoxInformation(p.Widget, desktop.DisplayName, "Config file: "+a.cfgPath)
		p.refocus()
	}
}
