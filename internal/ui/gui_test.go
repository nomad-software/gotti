package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nomad-software/qute"
	"github.com/nomad-software/qute/qtcore"
	"github.com/nomad-software/qute/qtwidgets"
	"github.com/nomad-software/qute/qutetest"

	"termite/internal/config"
)

// guiTests is set when the GUI tests can run: they need an X display (run
// them under Xvfb), as Qt's offscreen platform gives no GL context.
var guiTests bool

func TestMain(m *testing.M) {
	if os.Getenv("DISPLAY") == "" {
		os.Exit(m.Run())
	}
	os.Setenv("QT_QPA_PLATFORM", "xcb")
	os.Setenv("TERMITE_NO_DESKTOP_ENTRY", "1")
	guiTests = true
	NewApplication()
	qutetest.TestMain(m)
}

// needGUI skips a test when there is no display. It is called before
// qutetest.Run, which needs the application.
func needGUI(t *testing.T) {
	t.Helper()
	if !guiTests {
		t.Skip("needs an X display (DISPLAY), e.g. under Xvfb")
	}
}

// newTestApp opens a window running sh, with a config file in a temporary
// folder. Tests leave their windows open: closing the last one quits.
func newTestApp(t *testing.T) (*App, *Window) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	cfg, _ := config.Load(path)
	a := NewApp(cfg, path, []string{"sh"})
	w := a.NewWindow(nil)
	qutetest.Wait(300)
	return a, w
}

func focusedPane(t *testing.T, w *Window) *Pane {
	t.Helper()
	fw := qtwidgets.QApplicationFocusWidget()
	for _, p := range w.allPanes() {
		if fw != nil && fw.Ptr() == p.GL.Ptr() {
			return p
		}
	}
	t.Fatalf("keyboard focus is on no pane (%v)", fw)
	return nil
}

// splitter splits pane p the way a user would.
type splitter func(t *testing.T, p *Pane)

// byKey presses the action's shortcut in the pane.
func byKey(key qtcore.Key) splitter {
	return func(t *testing.T, p *Pane) {
		qutetest.Key(p.GL, key, qtcore.ControlModifier|qtcore.ShiftModifier)
	}
}

// byMenu chooses the item from the pane's context menu.
func byMenu(item string) splitter {
	return func(t *testing.T, p *Pane) {
		var choose func()
		choose = func() {
			menu := qtwidgets.QMenuCast(qtwidgets.QApplicationActivePopupWidget())
			if menu == nil {
				qute.Post(choose) // not open yet
				return
			}
			for _, act := range menu.Actions() {
				if strings.HasPrefix(act.Text(), item) {
					menu.Close()
					act.Trigger()
					return
				}
			}
			menu.Close()
			t.Errorf("no %q in the context menu", item)
		}
		qute.Post(choose)
		p.showContextMenu(0, 0, nil)
	}
}

func TestSplitFocusesNewPane(t *testing.T) {
	for _, tc := range []struct {
		name  string
		split splitter
	}{
		{"keys", byKey(qtcore.Key_E)},
		{"keys stacked", byKey(qtcore.Key_O)},
		{"menu", byMenu("Split Vertically")},
		{"menu stacked", byMenu("Split Horizontally")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			needGUI(t)
			qutetest.Run(t, func() {
				a, w := newTestApp(t)
				for i := range 3 {
					before := w.allPanes()
					tc.split(t, focusedPane(t, w))
					qutetest.Wait(200)
					panes := w.allPanes()
					if len(panes) != len(before)+1 {
						t.Fatalf("split %d: %d panes, want %d", i, len(panes), len(before)+1)
					}
					var added *Pane
					for _, p := range panes {
						if !containsPane(before, p) {
							added = p
						}
					}
					if got := focusedPane(t, w); got != added {
						t.Errorf("split %d: focus is not on the new pane", i)
					}
					if a.focused != added || w.list[0].focused != added {
						t.Errorf("split %d: the new pane is not the focused pane", i)
					}
				}
			})
		})
	}
}

func TestWindowTitleIsDirectoryName(t *testing.T) {
	needGUI(t)
	qutetest.Run(t, func() {
		_, w := newTestApp(t)
		p := w.list[0].focused
		p.headerDir = "/usr/share/doc"
		w.updateTitles()
		if got := w.w.WindowTitle(); got != "doc" {
			t.Errorf("window title %q, want %q", got, "doc")
		}
		p.headerDir = "/"
		w.updateTitles()
		if got := w.w.WindowTitle(); got != "/" {
			t.Errorf("window title %q, want %q", got, "/")
		}
	})
}

func containsPane(ps []*Pane, p *Pane) bool {
	for _, q := range ps {
		if q == p {
			return true
		}
	}
	return false
}

// whenModal runs f on the next modal dialog once it is open.
func whenModal(f func(*qtwidgets.QDialog)) {
	var try func()
	try = func() {
		dlg := qtwidgets.QDialogCast(qtwidgets.QApplicationActiveModalWidget())
		if dlg == nil {
			qute.Post(try)
			return
		}
		f(dlg)
	}
	qute.Post(try)
}

// TestOptionsDialog builds every page of the options dialog and saves it
// with Apply, then OK.
func TestOptionsDialog(t *testing.T) {
	needGUI(t)
	qutetest.Run(t, func() {
		a, w := newTestApp(t)
		p := w.list[0].focused
		pressed := 0
		whenModal(func(dlg *qtwidgets.QDialog) {
			for _, c := range dlg.Children() {
				if box := qtwidgets.QDialogButtonBoxCast(c); box != nil {
					box.Button(qtwidgets.QDialogButtonBoxApply).Click()
					box.Button(qtwidgets.QDialogButtonBoxOk).Click()
					pressed++
				}
			}
			if dlg.IsVisible() {
				dlg.Reject()
			}
		})
		a.showOptions(p)
		if pressed != 1 {
			t.Fatalf("found %d button boxes in the dialog, want 1", pressed)
		}
		data, err := os.ReadFile(a.cfgPath)
		if err != nil {
			t.Fatalf("the settings were not saved: %v", err)
		}
		for _, key := range []string{`"font_family"`, `"keybindings"`, `"themes"`, `"window_theme"`} {
			if !strings.Contains(string(data), key) {
				t.Errorf("saved config has no %s", key)
			}
		}
		if got := focusedPane(t, w); got != p {
			t.Errorf("focus did not return to the pane")
		}
	})
}

// TestClosePanes splits a tab and closes the panes again, down to one.
func TestClosePanes(t *testing.T) {
	needGUI(t)
	qutetest.Run(t, func() {
		a, w := newTestApp(t)
		for range 3 {
			a.runAction(focusedPane(t, w), "split_vertical")
			qutetest.Wait(100)
		}
		for n := 4; n > 1; n-- {
			a.runAction(focusedPane(t, w), "close_pane")
			for i := 0; i < 50 && len(w.allPanes()) == n; i++ {
				qutetest.Wait(20)
			}
			if got := len(w.allPanes()); got != n-1 {
				t.Fatalf("%d panes after closing one of %d", got, n)
			}
			focusedPane(t, w) // focus moved to a pane that is left
		}
	})
}
