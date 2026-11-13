package ui

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nomad-software/qute"
	"github.com/nomad-software/qute/qtcore"
	"github.com/nomad-software/qute/qtgui"
	"github.com/nomad-software/qute/qtwidgets"
	"github.com/nomad-software/qute/qutetest"

	"termite/internal/config"
	"termite/internal/icons"
	"termite/internal/term"
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
	a := NewApp(cfg, path, nil, []string{"sh"})
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
		for _, key := range []string{`"font_family"`, `"header_font_family"`, `"header_font_size"`, `"keybindings"`, `"themes"`, `"window_theme"`} {
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
			if !qutetest.WaitFor(func() bool { return len(w.allPanes()) == n-1 }, 2*time.Second) {
				got := len(w.allPanes())
				t.Fatalf("%d panes after closing one of %d", got, n)
			}
			focusedPane(t, w) // focus moved to a pane that is left
		}
	})
}

// TestFailedStart opens a tab, then a window, whose shell cannot start:
// each shows the problem and leaves nothing behind, and the window that
// was already open keeps its one tab.
func TestFailedStart(t *testing.T) {
	needGUI(t)
	qutetest.Run(t, func() {
		a, w := newTestApp(t)
		a.defaultCmd = []string{filepath.Join(t.TempDir(), "no-such-shell")}
		warnings := 0
		dismiss := func() {
			whenModal(func(dlg *qtwidgets.QDialog) {
				warnings++
				dlg.Reject()
			})
		}
		dismiss()
		if tab := w.NewTab("", a.defaultCmd); tab != nil {
			t.Fatal("NewTab returned a tab for a shell that failed")
		}
		if n, m := w.tabs.Count(), len(w.list); n != 1 || m != 1 {
			t.Fatalf("%d tabs shown and %d kept, want 1 of each", n, m)
		}
		dismiss()
		if nw := a.NewWindow(nil); nw != nil {
			t.Fatal("NewWindow returned a window with no tab")
		}
		if n := len(a.windows); n != 1 {
			t.Fatalf("%d windows open, want 1", n)
		}
		if warnings != 2 {
			t.Fatalf("%d problems shown, want 2", warnings)
		}
	})
}

// TestCloseWindow closes a second window, which is then deleted, and
// carries on working in the first.
func TestCloseWindow(t *testing.T) {
	needGUI(t)
	qutetest.Run(t, func() {
		a, w := newTestApp(t)
		w2 := a.NewWindow(nil)
		qutetest.Wait(300)
		a.runAction(w2.list[0].focused, "split_vertical")
		qutetest.Wait(100)
		w2.w.Close()
		qutetest.Wait(300)
		if n := len(a.windows); n != 1 || a.windows[0] != w {
			t.Fatalf("%d windows open after closing one of 2", n)
		}
		// Xvfb has no window manager to activate w again: use its pane.
		a.runAction(w.list[0].focused, "split_vertical")
		qutetest.Wait(100)
		if n := len(w.allPanes()); n != 2 {
			t.Fatalf("%d panes after splitting, want 2", n)
		}
	})
}

// TestToolbar checks the toolbar's buttons and separators, that its buttons
// act on the current pane without taking the focus, that Broadcast shows
// the mode however it was set, and that the config can hide it.
func TestToolbar(t *testing.T) {
	needGUI(t)
	qutetest.Run(t, func() {
		a, w := newTestApp(t)
		var got []string
		for _, act := range w.toolbar.bar.Actions() {
			if act.IsSeparator() {
				got = append(got, "|")
			} else {
				got = append(got, act.Text())
				if act.Icon().IsNull() {
					t.Errorf("%s has no icon", act.Text())
				}
				button := w.toolbar.bar.WidgetForAction(act)
				if width := button.Width(); width < toolButtonWidth {
					t.Errorf("%s is %d wide, want at least %d", act.Text(), width, toolButtonWidth)
				}
				// Its tooltip is never shown: the event asking for it is taken.
				at := qtcore.QPoint{X: button.Width() / 2, Y: button.Height() / 2}
				tip := qtgui.NewQHelpEventWithType(qtcore.QEventToolTip, at, button.MapToGlobal(at))
				if qtcore.QCoreApplicationSendEvent(button, tip) && qtwidgets.QToolTipIsVisible() {
					t.Errorf("%s shows a tooltip", act.Text())
				}
			}
		}
		want := "New Window,New Tab,|,Split Horizontally,Split Vertically,|,Broadcast,|,Options,|,Exit"
		// A line runs along its bottom, in the text's colour over the
		// window's, in every theme.
		img := w.toolbar.bar.Grab().ToImage()
		x := img.Width() - 5
		if line, above := img.Pixel(x, img.Height()-1), img.Pixel(x, img.Height()-3); line&0xffffff == above&0xffffff {
			t.Errorf("no line along the toolbar's bottom (%06x)", line&0xffffff)
		}
		if strings.Join(got, ",") != want {
			t.Errorf("toolbar %q, want %q", strings.Join(got, ","), want)
		}

		w.toolbar.actions["split_vertical"].Trigger()
		qutetest.Wait(200)
		if n := len(w.list[0].Panes()); n != 2 {
			t.Fatalf("%d panes after Split Vertically, want 2", n)
		}
		if p := focusedPane(t, w); p != w.list[0].focused {
			t.Error("the new pane does not have the focus")
		}
		w.toolbar.actions["new_tab"].Trigger()
		qutetest.Wait(200)
		if len(w.list) != 2 || w.currentPane() != focusedPane(t, w) {
			t.Fatalf("New Tab: %d tabs, focus on the current pane %v", len(w.list), w.currentPane() == focusedPane(t, w))
		}

		bc := w.toolbar.broadcast
		bc.Trigger()
		if a.broadcast != BroadcastAll || !bc.IsChecked() {
			t.Errorf("Broadcast on: mode %d, checked %v", a.broadcast, bc.IsChecked())
		}
		bc.Trigger()
		if a.broadcast != BroadcastOff || bc.IsChecked() {
			t.Errorf("Broadcast off: mode %d, checked %v", a.broadcast, bc.IsChecked())
		}
		a.runAction(w.currentPane(), "broadcast_group")
		if !bc.IsChecked() || !w.toolbar.modes[2].IsChecked() {
			t.Error("Broadcast does not show the mode a shortcut set")
		}
		w.toolbar.modes[0].Trigger()
		if a.broadcast != BroadcastOff || bc.IsChecked() {
			t.Error("Off in Broadcast's menu did not turn it off")
		}
		w.toolbar.inGroup.Trigger()
		if !w.currentPane().InGroup {
			t.Error("Current Pane in Group did not put the pane in the group")
		}

		// Exit types exit into the current pane only, whose shell then ends
		// and closes it, though input is broadcast to the tab.
		a.setBroadcast(BroadcastAll)
		n := len(w.allPanes())
		exiting := w.currentPane()
		w.toolbar.actions["exit"].Trigger()
		for range 50 {
			if len(w.allPanes()) < n {
				break
			}
			qutetest.Wait(100)
		}
		if got := w.allPanes(); len(got) != n-1 || containsPane(got, exiting) {
			t.Errorf("Exit: %d panes, want %d without the current one", len(got), n-1)
		}
		a.setBroadcast(BroadcastOff)

		// Its shortcut does the same.
		n = len(w.allPanes())
		exiting = focusedPane(t, w)
		qutetest.Key(exiting.GL, qtcore.Key_W, qtcore.ControlModifier)
		for range 50 {
			if len(w.allPanes()) < n {
				break
			}
			qutetest.Wait(100)
		}
		if got := w.allPanes(); len(got) != n-1 || containsPane(got, exiting) {
			t.Errorf("Ctrl+W: %d panes, want %d without the focused one", len(got), n-1)
		}

		if err := config.SetValues(a.cfgPath, map[string]any{"toolbar": false}); err != nil {
			t.Fatal(err)
		}
		a.runAction(w.currentPane(), "reload_config")
		if w.toolbar.bar.IsVisible() {
			t.Error("the toolbar is shown with \"toolbar\": false")
		}
	})
}

// TestIcons checks that every icon is drawn, in the colour of the text, and
// lasts once the garbage collector has run.
func TestIcons(t *testing.T) {
	needGUI(t)
	qutetest.Run(t, func() {
		for _, name := range icons.Names {
			icon(name)
		}
	})
	runtime.GC()
	runtime.GC()
	qutetest.Run(t, func() {
		c := qtgui.QGuiApplicationPalette().GroupColor(qtgui.QPaletteActive, qtgui.QPaletteWindowText)
		text := uint(c.R)<<16 | uint(c.G)<<8 | uint(c.B)
		for _, name := range icons.Names {
			pixmap := icon(name).Pixmap(qtcore.QSize{Width: 32, Height: 32})
			img := pixmap.ToImage()
			drawn, other := 0, 0
			for y := range img.Height() {
				for x := range img.Width() {
					c := img.Pixel(x, y)
					switch {
					case c>>24 == 0:
					case c&0xffffff == text:
						drawn++
					case c>>24 == 0xff:
						other++ // smoothed edges are partly transparent
					}
				}
			}
			if drawn == 0 || other > 0 {
				t.Errorf("%s: %d pixels in the text's colour, %d opaque in another", name, drawn, other)
			}
		}
		if !icon("nothing").IsNull() {
			t.Error("an icon that is not there was drawn")
		}
	})
}

// TestSearchBarAtBottom checks that the search bar opens below the
// terminal, at the bottom of the pane, and takes the focus.
func TestSearchBarAtBottom(t *testing.T) {
	needGUI(t)
	qutetest.Run(t, func() {
		a, w := newTestApp(t)
		p := w.currentPane()
		a.runAction(p, "search")
		qutetest.Wait(100)
		sb := p.searchBar
		if !sb.IsVisible() || !sb.HasFocus() {
			t.Fatalf("search bar visible %v, focused %v", sb.IsVisible(), sb.HasFocus())
		}
		if sb.Y() < p.GL.Y()+p.GL.Height() || sb.Y()+sb.Height() != p.Widget.Height() {
			t.Errorf("search bar at %d..%d, terminal ends at %d, pane %d high",
				sb.Y(), sb.Y()+sb.Height(), p.GL.Y()+p.GL.Height(), p.Widget.Height())
		}

		// One match is shown at a time, the newest first: Enter steps to
		// older ones, round to the newest again, and Shift+Enter back.
		p.S.WriteString("printf 'zq%sz\\n' 1 2 3\r")
		qutetest.Wait(500)
		sb.SetText("zqz")
		sb.SetText("zq")
		n := len(p.view.Matches)
		if n < 3 || p.view.Current != n-1 {
			t.Fatalf("%d matches, current %d", n, p.view.Current)
		}
		for i := n - 2; i >= -1; i-- {
			qutetest.Key(sb, qtcore.Key_Return, qtcore.NoModifier)
			if want := (i + n) % n; p.view.Current != want {
				t.Fatalf("Enter: current %d, want %d", p.view.Current, want)
			}
		}
		qutetest.Key(sb, qtcore.Key_Return, qtcore.ShiftModifier)
		if p.view.Current != 0 {
			t.Errorf("Shift+Enter: current %d, want 0", p.view.Current)
		}
		p.closeSearch()
	})
}

// descendants returns every object below o.
func descendants(o interface{ Children() []*qtcore.QObject }) []*qtcore.QObject {
	var out []*qtcore.QObject
	for _, c := range o.Children() {
		out = append(out, c)
		out = append(out, descendants(c)...)
	}
	return out
}

// TestRenameTheme renames themes in the Colours page's list, which has no
// name field of its own: a blank name keeps the old one.
func TestRenameTheme(t *testing.T) {
	needGUI(t)
	qutetest.Run(t, func() {
		a, w := newTestApp(t)
		dlg := qtwidgets.NewQDialog(w.w)
		defer dlg.DeleteLater()
		page, read := a.themesTab(dlg)
		var list *qtwidgets.QListWidget
		for _, o := range descendants(page) {
			if o.Inherits("QLineEdit") {
				t.Error("the page has a text field")
			}
			if l := qtwidgets.QListWidgetCast(o); l != nil {
				list = l
			}
		}
		if list == nil {
			t.Fatal("no list of themes")
		}
		item := list.CurrentItem()
		if item.Flags()&qtcore.ItemIsEditable == 0 {
			t.Fatal("the theme's name cannot be edited in the list")
		}
		item.SetText("  My Theme ")
		themes, sel := read()
		if got := themes[sel].Name; got != "My Theme" {
			t.Errorf("renamed to %q", got)
		}
		if item.Text() != "My Theme" {
			t.Errorf("the list shows %q", item.Text())
		}
		item.SetText(" ")
		themes, sel = read()
		if got := themes[sel].Name; got != "My Theme" || item.Text() != "My Theme" {
			t.Errorf("a blank name gave %q, shown as %q", got, item.Text())
		}
	})
}

// TestHeaderFont checks that the pane headers take the font set for them.
func TestHeaderFont(t *testing.T) {
	needGUI(t)
	qutetest.Run(t, func() {
		a, w := newTestApp(t)
		p := w.currentPane()
		_, size := a.cfg.HeaderFont()
		if got := p.header.Font().PointSizeF(); got != size {
			t.Errorf("header at %v pt, want %v", got, size)
		}
		if err := config.SetValues(a.cfgPath, map[string]any{"header_font_size": 17.5}); err != nil {
			t.Fatal(err)
		}
		a.runAction(p, "reload_config")
		if got := p.header.Font().PointSizeF(); got != 17.5 {
			t.Errorf("header at %v pt after setting 17.5", got)
		}
	})
}

// TestMoveTabs moves the current tab to the end and back, as
// Ctrl+Shift+PgDn and PgUp do. Qt takes the moving tab's page out and puts
// it back, and the focus leaving its terminal in between found a tab with
// no page.
func TestMoveTabs(t *testing.T) {
	needGUI(t)
	qutetest.Run(t, func() {
		a, w := newTestApp(t)
		for range 3 {
			a.runAction(w.currentPane(), "new_tab")
			qutetest.Wait(200)
		}
		w.w.ActivateWindow()
		w.tabs.SetCurrentIndex(0)
		moving := w.list[0]
		moving.focused.GL.SetFocus()
		qutetest.Wait(200)
		for _, step := range []struct {
			key  qtcore.Key
			want int
		}{
			{qtcore.Key_PageDown, 1}, {qtcore.Key_PageDown, 2}, {qtcore.Key_PageDown, 3}, {qtcore.Key_PageDown, 3},
			{qtcore.Key_PageUp, 2}, {qtcore.Key_PageUp, 1}, {qtcore.Key_PageUp, 0},
		} {
			qutetest.Key(focusedPane(t, w).GL, step.key, qtcore.ControlModifier|qtcore.ShiftModifier)
			qutetest.Wait(50)
			if got := w.indexOf(moving); got != step.want || w.tabs.CurrentIndex() != step.want {
				t.Fatalf("tab at %d, current %d, want %d", got, w.tabs.CurrentIndex(), step.want)
			}
			if w.currentPane() != moving.focused || focusedPane(t, w) != moving.focused {
				t.Fatal("the moved tab's pane lost the focus")
			}
		}
	})
}

// TestTabsFillBar checks that the tabs share the window's width equally.
func TestTabsFillBar(t *testing.T) {
	needGUI(t)
	qutetest.Run(t, func() {
		a, w := newTestApp(t)
		for range 2 {
			a.runAction(w.currentPane(), "new_tab")
			qutetest.Wait(200)
		}
		for _, width := range []int{900, 600} {
			w.w.Resize(width, 400)
			qutetest.Wait(200)
			bar := w.tabs.TabBar()
			total := 0
			for i := range 3 {
				r := bar.TabRect(i)
				total += r.Width
				if d := r.Width - bar.TabRect(0).Width; d < -1 || d > 1 {
					t.Errorf("at %d: tab %d is %d wide, tab 0 %d", width, i, r.Width, bar.TabRect(0).Width)
				}
			}
			if total < w.w.Width()-3 {
				t.Errorf("at %d: the tabs fill %d of %d", width, total, w.w.Width())
			}
		}

		// Each tab's close button keeps off its right edge, and closes it.
		bar := w.tabs.TabBar()
		for i := range 3 {
			box := bar.TabButton(i, qtwidgets.QTabBarRightSide)
			var button *qtwidgets.QToolButton
			for _, c := range box.Children() {
				if b := qtwidgets.QToolButtonCast(c); b != nil {
					button = b
				}
			}
			if button == nil {
				t.Fatalf("tab %d has no close button", i)
			}
			r := bar.TabRect(i)
			if gap := r.X + r.Width - (box.X() + button.X() + button.Width()); gap < tabCloseMargin {
				t.Errorf("tab %d's close button is %d from its edge", i, gap)
			}
		}
		closing := w.list[1]
		for _, c := range bar.TabButton(1, qtwidgets.QTabBarRightSide).Children() {
			if b := qtwidgets.QToolButtonCast(c); b != nil {
				b.Click()
			}
		}
		for range 50 {
			if len(w.list) == 2 {
				break
			}
			qutetest.Wait(100)
		}
		if len(w.list) != 2 || slices.Contains(w.list, closing) {
			t.Errorf("closing the second tab left %d tabs", len(w.list))
		}
	})
}

// TestBroadcastHeader checks that the headers of the panes typing reaches
// have the focused colours, and only while it does.
func TestBroadcastHeader(t *testing.T) {
	needGUI(t)
	qutetest.Run(t, func() {
		a, w := newTestApp(t)
		a.runAction(w.currentPane(), "split_vertical")
		qutetest.Wait(200)
		panes := w.allPanes()
		marked := func() []bool {
			var out []bool
			for _, p := range panes {
				out = append(out, p.header.Property("focused").ToBool())
			}
			return out
		}
		panes[0].GL.SetFocus()
		qutetest.Wait(100)
		if got := marked(); !slices.Equal(got, []bool{true, false}) {
			t.Errorf("only the focused pane's header is active: %v", got)
		}
		a.runAction(panes[0], "broadcast_all")
		if got := marked(); !slices.Equal(got, []bool{true, true}) {
			t.Errorf("broadcasting to the tab: %v", got)
		}
		// Typing reaches only the focused pane's tab, so another tab's
		// headers stay plain, then change places with the first tab's.
		other := w.NewTab("", nil).Panes()[0]
		qutetest.Wait(200)
		if !other.header.Property("focused").ToBool() || panes[1].header.Property("focused").ToBool() {
			t.Errorf("broadcasting from a new tab: it %v, first tab %v",
				other.header.Property("focused").ToBool(), marked())
		}
		w.tabs.SetCurrentIndex(0)
		panes[0].GL.SetFocus()
		qutetest.Wait(100)
		if got := marked(); !slices.Equal(got, []bool{true, true}) || other.header.Property("focused").ToBool() {
			t.Errorf("back on the first tab: %v, other tab %v", got, other.header.Property("focused").ToBool())
		}
		a.runAction(panes[0], "broadcast_all")
		if got := marked(); !slices.Equal(got, []bool{true, false}) {
			t.Errorf("not broadcasting: %v", got)
		}
		a.runAction(panes[1], "group_toggle")
		a.runAction(panes[0], "broadcast_group")
		if got := marked(); !slices.Equal(got, []bool{true, true}) {
			t.Errorf("broadcasting to the group: %v", got)
		}
		a.setBroadcast(BroadcastOff)
	})
}

// TestStartupConfigError checks that a problem config.Load found before
// the app started is shown in the pane headers, not only on stderr.
func TestStartupConfigError(t *testing.T) {
	needGUI(t)
	qutetest.Run(t, func() {
		path := filepath.Join(t.TempDir(), "config.json")
		os.WriteFile(path, []byte(`{"cursor_style": "bar"}`), 0o644)
		cfg, err := config.Load(path)
		if err == nil {
			t.Fatal("no error for a bad cursor_style")
		}
		a := NewApp(cfg, path, err, []string{"sh"})
		w := a.NewWindow(nil)
		qutetest.Wait(300)
		if text := w.currentPane().header.Text(); !strings.Contains(text, "cursor_style") {
			t.Errorf("the header shows %q", text)
		}
	})
}

// TestSearchAfterResize checks that the matches follow the text when a
// resize rewraps it.
func TestSearchAfterResize(t *testing.T) {
	needGUI(t)
	qutetest.Run(t, func() {
		_, w := newTestApp(t)
		p := w.currentPane()
		p.S.WriteString("printf '%0150d zqmark\\n' 0\r")
		qutetest.Wait(500)
		p.openSearch()
		p.searchBar.SetText("zqmark")
		at := func() term.Pos {
			if p.view.Current < 0 {
				t.Fatal("no match")
			}
			return p.view.Matches[p.view.Current].Start
		}
		textAt := func(pos term.Pos) string {
			p.T.Mu.Lock()
			defer p.T.Mu.Unlock()
			return p.T.Buf().TextRange(pos, term.Pos{Line: pos.Line, Col: pos.Col + 6}, false)
		}
		before := at()
		w.w.Resize(w.w.Width()/2, w.w.Height())
		qutetest.Wait(300)
		after := at()
		if after == before || textAt(after) != "zqmark" {
			t.Errorf("match at %v, then %v showing %q", before, after, textAt(after))
		}
		p.closeSearch()
	})
}

// TestPaneExitsUnderMenu closes a pane's shell while its context menu,
// then its multi-line paste question, is open: neither may outlive the
// pane in a way that crashes when it closes.
func TestPaneExitsUnderMenu(t *testing.T) {
	needGUI(t)
	qutetest.Run(t, func() {
		a, w := newTestApp(t)
		a.cfg.PasteWarning = "always"
		for _, c := range []struct {
			when func(testing.TB, func(*qtwidgets.QWidget))
			open func(p *Pane)
		}{
			{qutetest.WhenPopup, func(p *Pane) { p.showContextMenu(100, 100, nil) }},
			{qutetest.WhenModal, func(p *Pane) { p.paste("one\ntwo\n") }},
		} {
			a.runAction(focusedPane(t, w), "split_vertical")
			qutetest.Wait(200)
			p := focusedPane(t, w)
			n := len(w.allPanes())
			c.when(t, func(m *qtwidgets.QWidget) {
				p.S.WriteString("exit\r")
				if !qutetest.WaitFor(func() bool { return len(w.allPanes()) == n-1 }, 5*time.Second) {
					t.Errorf("the pane did not close under its menu or dialog")
				}
				// It goes with the pane, unless that is only deleted later.
				qutetest.WaitFor(func() bool { return !m.Alive() }, 500*time.Millisecond)
				if m.Alive() {
					m.Close()
				}
			})
			c.open(p)
			if got := len(w.allPanes()); got != n-1 {
				t.Fatalf("%d panes after one of %d exited", got, n)
			}
		}
	})
}

// TestHeaderKeptAcrossWindowSwitch checks that the focused pane's header
// keeps its colours while the window is inactive or a menu is open (so it
// does not flash when the window comes back), and still follows the focus
// between panes.
func TestHeaderKeptAcrossWindowSwitch(t *testing.T) {
	needGUI(t)
	qutetest.Run(t, func() {
		a, w := newTestApp(t)
		a.runAction(w.currentPane(), "split_vertical")
		qutetest.Wait(200)
		panes := w.allPanes()
		lit := func() []bool {
			return []bool{panes[0].header.Property("focused").ToBool(), panes[1].header.Property("focused").ToBool()}
		}
		panes[0].GL.SetFocus()
		qutetest.Wait(100)
		for _, reason := range []qtcore.FocusReason{qtcore.ActiveWindowFocusReason, qtcore.PopupFocusReason} {
			qtcore.QCoreApplicationSendEvent(panes[0].GL, qtgui.NewQFocusEventWithTypeFull(qtcore.QEventFocusOut, reason))
			if got := lit(); !slices.Equal(got, []bool{true, false}) {
				t.Errorf("focus out (reason %d): %v", reason, got)
			}
		}
		// The focus moved while the window was away: only the new pane is lit.
		qtcore.QCoreApplicationSendEvent(panes[1].GL, qtgui.NewQFocusEventWithTypeFull(qtcore.QEventFocusIn, qtcore.ActiveWindowFocusReason))
		if got := lit(); !slices.Equal(got, []bool{false, true}) {
			t.Errorf("focus in on the other pane: %v", got)
		}
		// Qt's own focus never moved (the events were made up): move it for
		// real, there and back.
		for i, want := range [][]bool{{false, true}, {true, false}} {
			panes[1-i].GL.SetFocus()
			qutetest.WaitFor(func() bool { return slices.Equal(lit(), want) }, time.Second)
			if got := lit(); !slices.Equal(got, want) {
				t.Errorf("focus moved to pane %d: %v", 1-i, got)
			}
		}
	})
}
