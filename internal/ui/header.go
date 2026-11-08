package ui

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	qt "github.com/mappu/miqt/qt6"
	"github.com/mappu/miqt/qt6/mainthread"
)

// The pane header is a slim bar above the terminal showing the directory
// the pane is in. It is refreshed after output (at most every
// headerRefresh), so an idle pane costs nothing.
const headerRefresh = 300 * time.Millisecond

func (p *Pane) setupHeader() {
	p.header = qt.NewQLabel(p.Widget)
	p.header.SetObjectName("gottiHeader")
	// The label must never widen the pane: long paths are elided instead.
	p.header.SetMinimumWidth(1)
	p.header.SetSizePolicy2(qt.QSizePolicy__Ignored, qt.QSizePolicy__Fixed)
	p.header.SetProperty("focused", qt.NewQVariant8(false))
	p.header.OnResizeEvent(func(super func(*qt.QResizeEvent), ev *qt.QResizeEvent) {
		super(ev)
		p.layoutHeader()
	})
}

// scheduleHeaderRefresh is called from the reader goroutine after output.
func (p *Pane) scheduleHeaderRefresh() {
	if p.headerPending.Swap(true) {
		return
	}
	time.AfterFunc(headerRefresh, func() {
		mainthread.Start(func() {
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
	}
	if prog := p.S.ForegroundName(); prog != p.program {
		p.program = prog
		p.app.paneTitleChanged(p)
	}
}

// layoutHeader sets the header text, elided to fit. While the config
// file has a problem, the header shows it (in red) instead of the path.
func (p *Pane) layoutHeader() {
	text, tip, mode := abbreviateHome(p.headerDir), p.headerDir, qt.ElideLeft
	problem := p.app.configErr != ""
	if problem {
		text, tip, mode = "⚠ "+p.app.configErr, p.app.configErr, qt.ElideRight
	}
	if text == "" {
		text = " "
	}
	fm := qt.NewQFontMetrics(p.header.Font())
	defer fm.Delete()
	// leave room for the label's horizontal padding
	if w := p.header.Width() - 20; w > 0 {
		text = fm.ElidedText(text, mode, w)
	}
	p.header.SetText(text)
	p.header.SetToolTip(tip)
	if problem != p.headerProblem {
		p.headerProblem = problem
		p.header.SetProperty("problem", qt.NewQVariant8(problem))
		p.repolishHeader()
	}
}

func (p *Pane) repolishHeader() {
	st := p.header.Style()
	st.Unpolish(p.header.QWidget)
	st.Polish(p.header.QWidget)
}

// setHeaderFocused highlights the header of the focused pane.
func (p *Pane) setHeaderFocused(on bool) {
	p.header.SetProperty("focused", qt.NewQVariant8(on))
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
