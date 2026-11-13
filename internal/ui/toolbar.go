package ui

import (
	"github.com/nomad-software/qute/qtcore"
	"github.com/nomad-software/qute/qtgui"
	"github.com/nomad-software/qute/qtwidgets"

	"termite/internal/icons"
)

// toolbarItems are the toolbar's buttons in order, an empty one standing
// for a separator: each runs its action on the window's current pane.
var toolbarItems = []struct{ label, action, icon string }{
	{"New Window", "new_window", icons.NewWindow},
	{"New Tab", "new_tab", icons.NewTab},
	{},
	{"Split Horizontally", "split_horizontal", icons.SplitHorizontal},
	{"Split Vertically", "split_vertical", icons.SplitVertical},
	{},
	{"Broadcast", "broadcast_all", icons.Broadcast},
	{},
	{"Options", "options", icons.Options},
	{},
	{"Exit", "exit", icons.Exit},
}

// toolButtonWidth is the least width of a toolbar button, so
// that one with a short name is not thinner than the rest.
const toolButtonWidth = 80

// broadcastModes are the broadcast modes as menus offer them.
var broadcastModes = []struct {
	label string
	mode  int
}{
	{"Off", BroadcastOff},
	{"All Panes in Tab", BroadcastAll},
	{"Group", BroadcastGroup},
}

// toolbar is a window's bar of large buttons, with their text under them,
// above its tabs. It keeps the desktop's (or window_theme's) style.
type toolbar struct {
	win     *Window
	bar     *qtwidgets.QToolBar
	actions map[string]*qtgui.QAction // by the action they run
	// broadcast is checked while input is broadcast in any mode; its
	// button's menu chooses the mode and the current pane's group.
	broadcast *qtgui.QAction
	modes     []*qtgui.QAction // as broadcastModes
	inGroup   *qtgui.QAction
}

func newToolbar(w *Window) *toolbar {
	tb := &toolbar{win: w, actions: map[string]*qtgui.QAction{}}
	tb.bar = qtwidgets.NewQToolBarWithTitleWith(&toolbarWidget{}, "Toolbar", w.w)
	tb.bar.SetObjectName("termiteToolbar")
	tb.bar.SetMovable(false)
	tb.bar.SetFloatable(false)
	tb.bar.SetToolButtonStyle(qtcore.ToolButtonTextUnderIcon)
	tb.bar.SetIconSize(qtcore.QSize{Width: 32, Height: 32})
	tb.bar.SetFocusPolicy(qtcore.NoFocus)
	for _, it := range toolbarItems {
		if it.action == "" {
			tb.bar.AddSeparator()
			continue
		}
		act := qtgui.NewQActionWithText(it.label, tb.bar)
		act.SetIcon(icon(it.icon))
		tb.bar.AddAction(act)
		tb.actions[it.action] = act
		if it.action == "broadcast_all" {
			tb.broadcast = act
			act.SetCheckable(true)
			act.OnTriggered(func(on bool) {
				mode := BroadcastOff
				if on {
					mode = BroadcastAll
				}
				w.app.setBroadcast(mode)
			})
			tb.makeBroadcastMenu()
			continue
		}
		act.OnTriggered(func(bool) {
			if p := w.currentPane(); p != nil {
				w.app.runAction(p, it.action)
			}
		})
	}
	// The buttons never take the focus from the terminal, and have no
	// tooltips: Qt gives a button its action's name as one whatever is
	// set, so the event that would show it is taken.
	noTips := &tipFilter{}
	qtcore.NewQObjectWith(noTips, tb.bar)
	for _, act := range tb.actions {
		button := tb.bar.WidgetForAction(act)
		button.SetFocusPolicy(qtcore.NoFocus)
		button.SetMinimumWidth(toolButtonWidth)
		button.InstallEventFilter(noTips)
	}
	tb.update()
	return tb
}

// makeBroadcastMenu gives the broadcast button a menu, beside the button
// itself, choosing the mode and whether the current pane is in the group.
func (tb *toolbar) makeBroadcastMenu() {
	a := tb.win.app
	menu := qtwidgets.NewQMenu(tb.bar)
	group := qtgui.NewQActionGroup(menu)
	group.SetExclusive(true)
	for _, m := range broadcastModes {
		act := menu.AddActionText(m.label)
		act.SetCheckable(true)
		act.SetActionGroup(group)
		act.OnTriggered(func(bool) { a.setBroadcast(m.mode) })
		tb.modes = append(tb.modes, act)
	}
	menu.AddSeparator()
	tb.inGroup = menu.AddActionText("Current Pane in Group")
	tb.inGroup.SetCheckable(true)
	tb.inGroup.OnTriggered(func(on bool) {
		if p := tb.win.currentPane(); p != nil {
			p.InGroup = on
			a.refreshBroadcast()
		}
	})
	menu.OnAboutToShow(tb.update)
	button := qtwidgets.QToolButtonCast(tb.bar.WidgetForAction(tb.broadcast))
	button.SetMenu(menu)
	button.SetPopupMode(qtwidgets.QToolButtonMenuButtonPopup)
}

// toolbarLine is the opacity, out of 255, of the line along the toolbar's
// bottom, in the colour of the text so that it shows in every theme.
const toolbarLine = 60

// toolbarWidget is the toolbar's QToolBar, which draws a line along its
// bottom: the desktop's style draws none for a toolbar outside a main
// window.
type toolbarWidget struct {
	qtwidgets.QToolBarBase
}

// PaintEvent overrides QWidget::paintEvent.
func (b *toolbarWidget) PaintEvent(ev *qtgui.QPaintEvent) {
	b.Super().PaintEvent(ev)
	p := qtgui.NewQPainterOn(b)
	defer p.End()
	c := b.Palette().Color(qtgui.QPaletteWindowText)
	c.A = toolbarLine
	p.FillRect(qtcore.QRect{Y: b.Height() - 1, Width: b.Width(), Height: 1}, c)
}

// tipFilter is an event filter that takes tooltip events.
type tipFilter struct{ qtcore.QObjectBase }

// EventFilter overrides QObject::eventFilter: tooltip events are taken, and
// every other is let through. Qt's own filter is not called, as it only
// lets events through, and a button being destroyed as its window closes
// still has events filtered, which Super would refuse.
func (f *tipFilter) EventFilter(_ *qtcore.QObject, event *qtcore.QEvent) bool {
	return event.Type() == qtcore.QEventToolTip
}

// update shows the config's toolbar setting and the broadcast state.
func (tb *toolbar) update() {
	a := tb.win.app
	tb.bar.SetVisible(a.cfg.Toolbar)
	tb.broadcast.SetChecked(a.broadcast != BroadcastOff)
	for i, m := range broadcastModes {
		tb.modes[i].SetChecked(a.broadcast == m.mode)
	}
	p := tb.win.currentPane()
	tb.inGroup.SetEnabled(p != nil)
	tb.inGroup.SetChecked(p != nil && p.InGroup)
}
