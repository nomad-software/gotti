package ui

import (
	"math"
	"strings"

	qt "github.com/mappu/miqt/qt6"

	"termite/internal/config"
	"termite/internal/desktop"
)

// choice is a combo box over a fixed list of config values.
type choice struct {
	box    *qt.QComboBox
	values []string
}

func newChoice(parent *qt.QWidget, labels, values []string, current string) choice {
	c := choice{box: qt.NewQComboBox(parent), values: values}
	for i, l := range labels {
		c.box.AddItem(l)
		if values[i] == current {
			c.box.SetCurrentIndex(i)
		}
	}
	return c
}

func (c choice) value() string {
	if i := c.box.CurrentIndex(); i >= 0 && i < len(c.values) {
		return c.values[i]
	}
	return ""
}

// optionsSection adds a titled group box to layout, with an optional note
// above its form. Fields stretch to the box's width.
func optionsSection(layout *qt.QVBoxLayout, title, note string) (*qt.QGroupBox, *qt.QFormLayout) {
	box := qt.NewQGroupBox3(title)
	form := qt.NewQFormLayout(box.QWidget)
	form.SetFieldGrowthPolicy(qt.QFormLayout__AllNonFixedFieldsGrow)
	if note != "" {
		label := qt.NewQLabel3(note)
		label.SetWordWrap(true)
		form.AddRowWithWidget(label.QWidget)
	}
	layout.AddWidget(box.QWidget)
	return box, form
}

// showOptions opens the options dialog. Settings are written to the
// config file on OK or Apply and applied at once.
func (a *App) showOptions(p *Pane) {
	cfg := a.cfg
	dlg := qt.NewQDialog(p.tab.win.w)
	dlg.SetWindowTitle("Options")
	root := qt.NewQVBoxLayout(dlg.QWidget)
	tabs := qt.NewQTabWidget(dlg.QWidget)
	general := qt.NewQWidget(nil)
	gl := qt.NewQVBoxLayout(general)
	uiPage := qt.NewQWidget(nil)
	ul := qt.NewQVBoxLayout(uiPage)

	// ---- window theme
	win, wf := optionsSection(ul, "Window", "")
	windowTheme := newChoice(win.QWidget, []string{"Light", "Dark", "Follow the system"},
		[]string{"light", "dark", "system"}, cfg.WindowTheme)
	wf.AddRow3("Theme:", windowTheme.box.QWidget)

	// ---- appearance
	look, lf := optionsSection(ul, "Appearance", "")

	// Every scalable font is offered: Qt's monospace filter relies on the
	// font's declared spacing, which hides fonts like Iosevka Term.
	font := qt.NewQFontComboBox(look.QWidget)
	font.SetFontFilters(qt.QFontComboBox__ScalableFonts)
	initial := qt.NewQFont2(cfg.FontFamily)
	font.SetCurrentFont(initial)
	initial.Delete()
	size := qt.NewQDoubleSpinBox(look.QWidget)
	size.SetRange(5, 72)
	size.SetDecimals(1)
	size.SetSingleStep(0.5)
	size.SetSuffix(" pt")
	size.SetValue(cfg.FontSize)
	fontRow := qt.NewQHBoxLayout(nil)
	fontRow.AddWidget2(font.QWidget, 1)
	fontRow.AddWidget(size.QWidget)
	lf.AddRow4("Font:", fontRow.QLayout)
	monoWarn := qt.NewQLabel3("This font does not look monospaced; text may be spaced unevenly.")
	monoWarn.SetWordWrap(true)
	lf.AddRow3("", monoWarn.QWidget)
	checkMono := func() {
		// CurrentFont returns a copy that miqt frees itself; deleting it
		// here as well would free it twice.
		monoWarn.SetVisible(!isMonospaced(font.CurrentFont()))
	}
	checkMono()
	font.OnCurrentFontChanged(func(*qt.QFont) { checkMono() })

	lineHeight := qt.NewQDoubleSpinBox(look.QWidget)
	lineHeight.SetRange(0.8, 2.5)
	lineHeight.SetDecimals(2)
	lineHeight.SetSingleStep(0.05)
	lineHeight.SetValue(cfg.LineHeight)
	lf.AddRow3("Line height:", lineHeight.QWidget)
	padding := qt.NewQSpinBox(look.QWidget)
	padding.SetRange(0, 50)
	padding.SetSuffix(" px")
	padding.SetValue(cfg.Padding)
	lf.AddRow3("Padding:", padding.QWidget)
	ligatures := qt.NewQCheckBox3("Ligatures (needs a font with them)")
	ligatures.SetChecked(cfg.Ligatures)
	lf.AddRow3("", ligatures.QWidget)
	boldBright := qt.NewQCheckBox3("Show bold text in bright colours")
	boldBright.SetChecked(cfg.BoldIsBright)
	lf.AddRow3("", boldBright.QWidget)
	scrollbar := qt.NewQCheckBox3("Show a scrollbar")
	scrollbar.SetChecked(cfg.Scrollbar)
	lf.AddRow3("", scrollbar.QWidget)

	// ---- cursor
	curBox, cf := optionsSection(ul, "Cursor", "")
	cursor := newChoice(curBox.QWidget, []string{"Block", "Beam", "Underline"}, []string{"block", "beam", "underline"}, cfg.CursorStyle)
	cf.AddRow3("Style:", cursor.box.QWidget)
	blink := qt.NewQCheckBox3("Allow blinking when a program asks for it")
	blink.SetChecked(cfg.CursorBlink)
	cf.AddRow3("", blink.QWidget)

	// ---- behaviour
	beh, bf := optionsSection(gl, "Behaviour", "")
	scrollback := qt.NewQSpinBox(beh.QWidget)
	scrollback.SetRange(0, 1000000)
	scrollback.SetSingleStep(1000)
	scrollback.SetSuffix(" lines")
	scrollback.SetValue(cfg.Scrollback)
	scrollback.SetToolTip("Applies to panes opened from now on.")
	bf.AddRow3("Scrollback:", scrollback.QWidget)
	copySel := qt.NewQCheckBox3("Copy selected text to the clipboard")
	copySel.SetChecked(cfg.CopyOnSelect)
	bf.AddRow3("", copySel.QWidget)
	paste := newChoice(beh.QWidget, []string{"Always", "Unless the program uses bracketed paste", "Never"},
		[]string{"always", "unbracketed", "never"}, cfg.PasteWarning)
	bf.AddRow3("Warn on multi-line paste:", paste.box.QWidget)
	osc52 := newChoice(beh.QWidget, []string{"Off", "Programs may set it", "Programs may set and read it"},
		[]string{"off", "write", "read-write"}, cfg.OSC52)
	bf.AddRow3("Clipboard access (OSC 52):", osc52.box.QWidget)

	gl.AddStretch()
	ul.AddStretch()
	tabs.AddTab(general, "General")
	tabs.AddTab(uiPage, "User interface")
	themesPage, readThemes := a.themesTab(dlg)
	tabs.AddTab(themesPage, "Colours")
	keysPage, readKeys := a.shortcutsTab()
	tabs.AddTab(keysPage, "Keyboard Shortcuts")
	root.AddWidget(tabs.QWidget)

	// ---- buttons
	buttons := qt.NewQDialogButtonBox4(qt.QDialogButtonBox__Ok | qt.QDialogButtonBox__Cancel | qt.QDialogButtonBox__Apply)
	edit := buttons.AddButton2("Edit Config File…", qt.QDialogButtonBox__ResetRole)
	root.AddWidget(buttons.QWidget)

	apply := func() bool {
		bindings, clashes := readKeys()
		if len(clashes) > 0 {
			tabs.SetCurrentIndex(3)
			qt.QMessageBox_Warning(dlg.QWidget, desktop.DisplayName, "These shortcuts are used by more than one action:\n\n"+
				strings.Join(clashes, "\n")+"\n\nChange or clear them, then try again.")
			return false
		}
		family := font.CurrentFont().Family()
		values := map[string]any{
			"window_theme":   windowTheme.value(),
			"font_family":    family,
			"font_size":      math.Round(size.Value()*10) / 10,
			"line_height":    math.Round(lineHeight.Value()*100) / 100,
			"padding":        padding.Value(),
			"ligatures":      ligatures.IsChecked(),
			"bold_is_bright": boldBright.IsChecked(),
			"scrollbar":      scrollbar.IsChecked(),
			"cursor_style":   cursor.value(),
			"cursor_blink":   blink.IsChecked(),
			"scrollback":     scrollback.Value(),
			"copy_on_select": copySel.IsChecked(),
			"paste_warning":  paste.value(),
			"osc52":          osc52.value(),
			"keybindings":    bindings,
		}
		values["themes"], values["theme"] = readThemes()
		if family != a.cfg.FontFamily || values["font_size"] != a.cfg.FontSize {
			a.fontScale = 1 // an explicit size replaces any zoom
		}
		if err := config.SetValues(a.cfgPath, values); err != nil {
			qt.QMessageBox_Warning(dlg.QWidget, desktop.DisplayName, "The settings could not be saved:\n"+err.Error())
			return false
		}
		c, err := config.Load(a.cfgPath)
		a.reload(c, err)
		return true
	}
	buttons.OnAccepted(func() {
		if apply() {
			dlg.Accept()
		}
	})
	buttons.OnRejected(dlg.Reject)
	buttons.Button(qt.QDialogButtonBox__Apply).OnClicked(func() { apply() })
	edit.OnClicked(func() {
		dlg.Reject()
		a.editConfig(p)
	})

	dlg.Exec()
	dlg.DeleteLater()
	if !p.exited {
		p.refocus()
	}
}
