package ui

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nomad-software/qute"
	"github.com/nomad-software/qute/qtcore"
	"github.com/nomad-software/qute/qtgui"
	"github.com/nomad-software/qute/qtwidgets"
)

// The pane header is a slim bar above the terminal showing the directory
// the pane is in. It is refreshed after output (at most every
// headerRefresh), so an idle pane costs nothing.
const headerRefresh = 300 * time.Millisecond

// headerLabel is the header's QLabel, which lays its text out again
// whenever it is resized.
type headerLabel struct {
	qtwidgets.QLabelBase
	p *Pane
}

// ResizeEvent overrides QWidget::resizeEvent.
func (h *headerLabel) ResizeEvent(ev *qtgui.QResizeEvent) {
	h.Super().ResizeEvent(ev)
	h.p.layoutHeader()
}

func (p *Pane) setupHeader() {
	p.header = qtwidgets.NewQLabelWith(&headerLabel{p: p}, p.Widget)
	p.header.SetObjectName("termiteHeader")
	p.header.SetAlignment(qtcore.AlignCenter)
	// The label must never widen the pane: long paths are elided instead.
	p.header.SetMinimumWidth(1)
	p.header.SetSizePolicy(qtwidgets.NewQSizePolicyWithPolicy(qtwidgets.QSizePolicyIgnored, qtwidgets.QSizePolicyFixed))
	setBoolProperty(p.header, "focused", false)
}

// scheduleHeaderRefresh is called from the reader goroutine after output.
func (p *Pane) scheduleHeaderRefresh() {
	if p.headerPending.Swap(true) {
		return
	}
	time.AfterFunc(headerRefresh, func() {
		qute.Post(func() {
			p.headerPending.Store(false)
			if !p.exited {
				p.refreshHeader()
			}
		})
	})
}

// refreshHeader re-reads the directory and foreground program.
func (p *Pane) refreshHeader() {
	dir := p.cwd()
	if dir != p.headerDir {
		p.headerDir = dir
		p.layoutHeader()
		p.tab.win.updateTitles()
	}
	if prog := p.S.ForegroundName(); prog != p.program {
		p.program = prog
		p.app.paneTitleChanged(p)
	}
}

// layoutHeader sets the header text, elided to fit. While the config
// file has a problem, the header shows it (in red) instead of the path.
func (p *Pane) layoutHeader() {
	text, tip, mode := abbreviateHome(p.headerDir), p.headerDir, qtcore.ElideLeft
	problem := p.app.configErr != ""
	if problem {
		text, tip, mode = "⚠ "+p.app.configErr, p.app.configErr, qtcore.ElideRight
	}
	if text == "" {
		text = " "
	}
	fm := qtgui.NewQFontMetricsWithFont(p.header.Font())
	// leave room for the label's horizontal padding
	if w := p.header.Width() - 20; w > 0 {
		text = fm.ElidedText(text, mode, w)
	}
	p.header.SetText(text)
	p.header.SetToolTip(tip)
	if problem != p.headerProblem {
		p.headerProblem = problem
		setBoolProperty(p.header, "problem", problem)
		p.repolishHeader()
	}
}

func (p *Pane) repolishHeader() {
	st := p.header.Style()
	st.Unpolish(p.header)
	st.Polish(p.header)
}

// setHeaderFocused notes whether the pane has the focus, and shows it.
func (p *Pane) setHeaderFocused(on bool) {
	p.headerFocused = on
	p.updateHeaderActive()
}

// updateHeaderActive gives the header the focused colours while the pane
// has the focus, or while typing in another pane reaches it.
func (p *Pane) updateHeaderActive() {
	active := p.headerFocused || p.app.broadcastsTo(p)
	if active == p.headerActive {
		return
	}
	p.headerActive = active
	setBoolProperty(p.header, "focused", active)
	p.repolishHeader()
}

// abbreviateHome shows paths under the home directory as ~/...
func abbreviateHome(dir string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" || dir == "" {
		return dir
	}
	if dir == home {
		return "~"
	}
	if rel, ok := strings.CutPrefix(dir, home+string(filepath.Separator)); ok {
		return "~/" + rel
	}
	return dir
}
