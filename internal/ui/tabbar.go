package ui

import (
	"github.com/nomad-software/qute/qtcore"
	"github.com/nomad-software/qute/qtwidgets"

	"termite/internal/icons"
)

// tabBaseWidth is the width every tab asks for. The tab bar is as wide as
// the window and shares what is left between its tabs, so they are all the
// same width and fill the bar; past that many tabs it scrolls.
const tabBaseWidth = 120

// tabCloseMargin keeps a tab's close button off the tab's right edge, where
// Qt's own button sits whatever the stylesheet says.
const tabCloseMargin = 6

// tabCloseButton returns a tab's close button, a flat one with termite's
// close icon, which calls close.
func tabCloseButton(close func()) *qtwidgets.QWidget {
	box := qtwidgets.NewQWidget(nil)
	lay := qtwidgets.NewQHBoxLayout(box)
	lay.SetContentsMargins(qtcore.QMargins{Right: tabCloseMargin})
	b := qtwidgets.NewQToolButton(box)
	b.SetAutoRaise(true)
	b.SetFocusPolicy(qtcore.NoFocus)
	b.SetIcon(icon(icons.Close))
	b.SetIconSize(qtcore.QSize{Width: 14, Height: 14})
	b.SetToolTip("Close tab")
	b.OnClicked(func(bool) { close() })
	lay.AddWidget(b)
	return box
}

// tabWidget is a window's QTabWidget, which takes a tabBar: setTabBar is
// protected in Qt.
type tabWidget struct {
	qtwidgets.QTabWidgetBase
}

// tabBar is a tab bar whose tabs fill its width, each the same width.
type tabBar struct {
	qtwidgets.QTabBarBase
	tabs *qtwidgets.QTabWidget
}

// newTabWidget returns a tab widget with a tabBar, named for the window's
// stylesheet.
func newTabWidget(parent qtwidgets.QWidgetLike) *qtwidgets.QTabWidget {
	tw := &tabWidget{}
	tabs := qtwidgets.NewQTabWidgetWith(tw, parent)
	bar := &tabBar{tabs: tabs}
	qtwidgets.NewQTabBarWith(bar, nil)
	bar.SetObjectName("termiteTabBar")
	bar.SetElideMode(qtcore.ElideLeft) // the end of a directory says most
	tw.SetTabBar(bar)
	tabs.SetObjectName("termiteTabs")
	// Document mode turns expanding off, so it goes first.
	tabs.SetDocumentMode(true)
	bar.SetExpanding(true)
	return tabs
}

// SizeHint overrides QWidget::sizeHint: the tab widget gives the bar the
// width it asks for, so it asks for all of it.
func (b *tabBar) SizeHint() qtcore.QSize {
	h := b.Super().SizeHint()
	h.Width = max(h.Width, b.tabs.Width())
	return h
}

// TabSizeHint overrides QTabBar::tabSizeHint: every tab asks for the same
// width, and the bar shares out the rest equally.
func (b *tabBar) TabSizeHint(index int) qtcore.QSize {
	h := b.Super().TabSizeHint(index)
	h.Width = tabBaseWidth
	return h
}
