package ui

import (
	"math"
	"strings"

	"github.com/nomad-software/qute/qtgui"
	"github.com/nomad-software/qute/qtwidgets"

	"termite/internal/config"
	"termite/internal/desktop"
)

// choice is a combo box over a fixed list of config values.
type choice struct {
	box    *qtwidgets.QComboBox
	values []string
}

func newChoice(parent qtwidgets.QWidgetLike, labels, values []string, current string) choice {
	c := choice{box: qtwidgets.NewQComboBox(parent), values: values}
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
func optionsSection(layout *qtwidgets.QVBoxLayout, title, note string) (*qtwidgets.QGroupBox, *qtwidgets.QFormLayout) {
	box := qtwidgets.NewQGroupBoxWithTitle(title, nil)
	form := qtwidgets.NewQFormLayout(box)
	form.SetFieldGrowthPolicy(qtwidgets.QFormLayoutAllNonFixedFieldsGrow)
	if note != "" {
		label := qtwidgets.NewQLabelWithText(note, nil)
		label.SetWordWrap(true)
		form.AddRowWidget(label)
	}
	layout.AddWidget(box)
	return box, form
}

// showOptions opens the options dialog. Settings are written to the
// config file on OK or Apply and applied at once.
func (a *App) showOptions(p *Pane) {
	cfg := a.cfg
	dlg := qtwidgets.NewQDialog(p.tab.win.w)
	dlg.SetWindowTitle("Options")
	root := qtwidgets.NewQVBoxLayout(dlg)
	tabs := qtwidgets.NewQTabWidget(dlg)
	general := qtwidgets.NewQWidget(nil)
	gl := qtwidgets.NewQVBoxLayout(general)
	uiPage := qtwidgets.NewQWidget(nil)
	ul := qtwidgets.NewQVBoxLayout(uiPage)

	// ---- window theme
	win, wf := optionsSection(ul, "Window", "")
	windowTheme := newChoice(win, []string{"Light", "Dark", "Follow the system"},
		[]string{"light", "dark", "system"}, cfg.WindowTheme)
	wf.AddRowText("Theme:", windowTheme.box)
	toolbar := qtwidgets.NewQCheckBoxWithText("Show the toolbar", nil)
	toolbar.SetChecked(cfg.Toolbar)
	wf.AddRowText("", toolbar)

	// ---- appearance
	look, lf := optionsSection(ul, "Appearance", "")

	// Every scalable font is offered: Qt's monospace filter relies on the
	// font's declared spacing, which hides fonts like Iosevka Term.
	font := qtwidgets.NewQFontComboBox(look)
	font.SetFontFilters(qtwidgets.QFontComboBoxScalableFonts)
	font.SetCurrentFont(qtgui.NewQFontWithFamily(cfg.FontFamily))
	size := qtwidgets.NewQDoubleSpinBox(look)
	size.SetRange(config.MinFontSize, config.MaxFontSize)
	size.SetDecimals(1)
	size.SetSingleStep(0.5)
	size.SetSuffix(" pt")
	size.SetValue(cfg.FontSize)
	fontRow := qtwidgets.NewQHBoxLayout(nil)
	fontRow.AddWidgetFull(font, 1, 0)
	fontRow.AddWidget(size)
	lf.AddRowTextLayout("Font:", fontRow)
	monoWarn := qtwidgets.NewQLabelWithText("This font does not look monospaced; text may be spaced unevenly.", nil)
	monoWarn.SetWordWrap(true)
	lf.AddRowText("", monoWarn)
	checkMono := func() {
		monoWarn.SetVisible(!isMonospaced(font.CurrentFont()))
	}
	checkMono()
	font.OnCurrentFontChanged(func(*qtgui.QFont) { checkMono() })

	// The pane headers' font: the terminal's, a little smaller, unless set.
	hdrFamily, hdrPoints := cfg.HeaderFont()
	hdrFont := qtwidgets.NewQFontComboBox(look)
	hdrFont.SetFontFilters(qtwidgets.QFontComboBoxScalableFonts)
	hdrFont.SetCurrentFont(qtgui.NewQFontWithFamily(hdrFamily))
	hdrSize := qtwidgets.NewQDoubleSpinBox(look)
	hdrSize.SetRange(config.MinFontSize, config.MaxFontSize)
	hdrSize.SetDecimals(1)
	hdrSize.SetSingleStep(0.5)
	hdrSize.SetSuffix(" pt")
	hdrSize.SetValue(hdrPoints)
	hdrRow := qtwidgets.NewQHBoxLayout(nil)
	hdrRow.AddWidgetFull(hdrFont, 1, 0)
	hdrRow.AddWidget(hdrSize)
	lf.AddRowTextLayout("Pane header font:", hdrRow)
	hdrSame := qtwidgets.NewQCheckBoxWithText("Use the terminal font, a little smaller", nil)
	hdrSame.SetChecked(cfg.HeaderFontFamily == "" && cfg.HeaderFontSize <= 0)
	lf.AddRowText("", hdrSame)
	hdrEnable := func(same bool) {
		hdrFont.SetEnabled(!same)
		hdrSize.SetEnabled(!same)
	}
	hdrEnable(hdrSame.IsChecked())
	hdrSame.OnToggled(hdrEnable)

	lineHeight := qtwidgets.NewQDoubleSpinBox(look)
	lineHeight.SetRange(config.MinLineHeight, config.MaxLineHeight)
	lineHeight.SetDecimals(2)
	lineHeight.SetSingleStep(0.05)
	lineHeight.SetValue(cfg.LineHeight)
	lf.AddRowText("Line height:", lineHeight)
	padding := qtwidgets.NewQSpinBox(look)
	padding.SetRange(0, config.MaxPadding)
	padding.SetSuffix(" px")
	padding.SetValue(cfg.Padding)
	lf.AddRowText("Padding:", padding)
	ligatures := qtwidgets.NewQCheckBoxWithText("Ligatures (needs a font with them)", nil)
	ligatures.SetChecked(cfg.Ligatures)
	lf.AddRowText("", ligatures)
	boldBright := qtwidgets.NewQCheckBoxWithText("Show bold text in bright colours", nil)
	boldBright.SetChecked(cfg.BoldIsBright)
	lf.AddRowText("", boldBright)
	scrollbar := qtwidgets.NewQCheckBoxWithText("Show a scrollbar", nil)
	scrollbar.SetChecked(cfg.Scrollbar)
	lf.AddRowText("", scrollbar)

	// ---- cursor
	curBox, cf := optionsSection(ul, "Cursor", "")
	cursor := newChoice(curBox, []string{"Block", "Beam", "Underline"}, []string{"block", "beam", "underline"}, cfg.CursorStyle)
	cf.AddRowText("Style:", cursor.box)
	blink := qtwidgets.NewQCheckBoxWithText("Allow blinking when a program asks for it", nil)
	blink.SetChecked(cfg.CursorBlink)
	cf.AddRowText("", blink)

	// ---- behaviour
	beh, bf := optionsSection(gl, "Behaviour", "")
	scrollback := qtwidgets.NewQSpinBox(beh)
	scrollback.SetRange(0, config.MaxScrollback)
	scrollback.SetSingleStep(1000)
	scrollback.SetSuffix(" lines")
	scrollback.SetValue(cfg.Scrollback)
	scrollback.SetToolTip("Applies to panes opened from now on.")
	bf.AddRowText("Scrollback:", scrollback)
	copySel := qtwidgets.NewQCheckBoxWithText("Copy selected text to the clipboard", nil)
	copySel.SetChecked(cfg.CopyOnSelect)
	bf.AddRowText("", copySel)
	paste := newChoice(beh, []string{"Always", "Unless the program uses bracketed paste", "Never"},
		[]string{"always", "unbracketed", "never"}, cfg.PasteWarning)
	bf.AddRowText("Warn on multi-line paste:", paste.box)
	osc52 := newChoice(beh, []string{"Off", "Programs may set it", "Programs may set and read it"},
		[]string{"off", "write", "read-write"}, cfg.OSC52)
	bf.AddRowText("Clipboard access (OSC 52):", osc52.box)

	gl.AddStretch()
	ul.AddStretch()
	tabs.AddTab(general, "General")
	tabs.AddTab(uiPage, "User interface")
	themesPage, readThemes := a.themesTab(dlg)
	tabs.AddTab(themesPage, "Colours")
	keysPage, readKeys := a.shortcutsTab()
	tabs.AddTab(keysPage, "Keyboard Shortcuts")
	root.AddWidget(tabs)

	// ---- buttons
	buttons := qtwidgets.NewQDialogButtonBoxWithStandardButton(qtwidgets.QDialogButtonBoxOk|qtwidgets.QDialogButtonBoxCancel|qtwidgets.QDialogButtonBoxApply, nil)
	edit := buttons.AddButtonText("Edit Config File…", qtwidgets.QDialogButtonBoxResetRole)
	root.AddWidget(buttons)

	apply := func() bool {
		bindings, clashes := readKeys()
		if len(clashes) > 0 {
			tabs.SetCurrentWidget(keysPage)
			qtwidgets.QMessageBoxWarning(dlg, desktop.DisplayName, "These shortcuts are used by more than one action:\n\n"+
				strings.Join(clashes, "\n")+"\n\nChange or clear them, then try again.")
			return false
		}
		family := font.CurrentFont().Family()
		headerFamily, headerSize := "", 0.0
		if !hdrSame.IsChecked() {
			headerFamily = hdrFont.CurrentFont().Family()
			headerSize = math.Round(hdrSize.Value()*10) / 10
		}
		values := map[string]any{
			"header_font_family": headerFamily,
			"header_font_size":   headerSize,
			"window_theme":       windowTheme.value(),
			"toolbar":            toolbar.IsChecked(),
			"font_family":        family,
			"font_size":          math.Round(size.Value()*10) / 10,
			"line_height":        math.Round(lineHeight.Value()*100) / 100,
			"padding":            padding.Value(),
			"ligatures":          ligatures.IsChecked(),
			"bold_is_bright":     boldBright.IsChecked(),
			"scrollbar":          scrollbar.IsChecked(),
			"cursor_style":       cursor.value(),
			"cursor_blink":       blink.IsChecked(),
			"scrollback":         scrollback.Value(),
			"copy_on_select":     copySel.IsChecked(),
			"paste_warning":      paste.value(),
			"osc52":              osc52.value(),
			"keybindings":        bindings,
		}
		values["themes"], values["theme"] = readThemes()
		if family != a.cfg.FontFamily || values["font_size"] != a.cfg.FontSize {
			a.fontScale = 1 // an explicit size replaces any zoom
		}
		if err := config.SetValues(a.cfgPath, values); err != nil {
			qtwidgets.QMessageBoxWarning(dlg, desktop.DisplayName, "The settings could not be saved:\n"+err.Error())
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
	buttons.Button(qtwidgets.QDialogButtonBoxApply).OnClicked(func(bool) { apply() })
	edit.OnClicked(func(bool) {
		dlg.Reject()
		a.editConfig(p)
	})

	dlg.Exec()
	dlg.DeleteLater()
	if !p.exited {
		p.refocus()
	}
}

// isMonospaced checks whether narrow and wide letters have equal advances.
func isMonospaced(f *qtgui.QFont) bool {
	fm := qtgui.NewQFontMetricsFWithFont(f)
	i, m, w := fm.HorizontalAdvance("i"), fm.HorizontalAdvance("M"), fm.HorizontalAdvance("W")
	return math.Abs(i-m) < 0.5 && math.Abs(m-w) < 0.5
}
