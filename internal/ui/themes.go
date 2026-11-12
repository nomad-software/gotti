package ui

import (
	"fmt"
	"strings"

	"github.com/nomad-software/qute/qtwidgets"

	"termite/internal/config"
)

// themeColorFields are the editable non-palette colours of a theme.
var themeColorFields = []struct {
	label    string
	header   bool // shown under Pane header
	get      func(*config.ThemeSpec) *string
	resolved func(config.Theme) uint32
}{
	{"Foreground", false, func(s *config.ThemeSpec) *string { return &s.Foreground }, func(t config.Theme) uint32 { return t.Foreground }},
	{"Background", false, func(s *config.ThemeSpec) *string { return &s.Background }, func(t config.Theme) uint32 { return t.Background }},
	{"Cursor", false, func(s *config.ThemeSpec) *string { return &s.Cursor }, func(t config.Theme) uint32 { return t.Cursor }},
	{"Cursor text", false, func(s *config.ThemeSpec) *string { return &s.CursorText }, func(t config.Theme) uint32 { return t.CursorText }},
	{"Selection", false, func(s *config.ThemeSpec) *string { return &s.Selection }, func(t config.Theme) uint32 { return t.Selection }},
	{"Selected text", false, func(s *config.ThemeSpec) *string { return &s.SelectionText }, func(t config.Theme) uint32 { return t.SelText }},
	{"Background", true, func(s *config.ThemeSpec) *string { return &s.HeaderBackground }, func(t config.Theme) uint32 { return t.HeaderBg }},
	{"Focused background", true, func(s *config.ThemeSpec) *string { return &s.HeaderFocusedBackground }, func(t config.Theme) uint32 { return t.HeaderFocusBg }},
	{"Text", true, func(s *config.ThemeSpec) *string { return &s.HeaderText }, func(t config.Theme) uint32 { return t.HeaderFg }},
	{"Focused text", true, func(s *config.ThemeSpec) *string { return &s.HeaderFocusedText }, func(t config.Theme) uint32 { return t.HeaderFocusFg }},
}

var paletteNames = [8]string{"Black", "Red", "Green", "Yellow", "Blue", "Magenta", "Cyan", "White"}

// swatchStyle colours a button with c, with readable text on it.
func swatchStyle(c uint32) string {
	r, g, b := c>>16&0xff, c>>8&0xff, c&0xff
	text := "#000000"
	if 299*r+587*g+114*b < 128000 {
		text = "#ffffff"
	}
	return fmt.Sprintf("QPushButton { background: %s; color: %s; border: 1px solid #808080; border-radius: 3px; padding: 2px 8px; }",
		config.HexColor(c), text)
}

// themesTab builds the options dialog's Colours page: a list of themes to
// choose from, with an editor for the selected one. read returns the
// edited themes and the id of the selected theme.
func (a *App) themesTab(dlg *qtwidgets.QDialog) (page *qtwidgets.QWidget, read func() (map[string]config.ThemeSpec, string)) {
	specs := config.CloneThemes(a.cfg.Themes)
	ids := a.cfg.ThemeIDs()
	current := -1
	loading := false

	page = qtwidgets.NewQWidget(nil)
	outer := qtwidgets.NewQHBoxLayout(page)

	// ---- left: the list and its buttons
	left := qtwidgets.NewQVBoxLayout(nil)
	_, lf := optionsSection(left, "Themes", "The selected theme is used when you press OK or Apply.")
	list := qtwidgets.NewQListWidget(page)
	list.SetMinimumWidth(170)
	lf.AddRowWidget(list)
	addBtn := qtwidgets.NewQPushButtonWithText("Add", nil)
	dupBtn := qtwidgets.NewQPushButtonWithText("Duplicate", nil)
	removeBtn := qtwidgets.NewQPushButtonWithText("Remove", nil)
	row := qtwidgets.NewQHBoxLayout(nil)
	row.AddWidget(addBtn)
	row.AddWidget(dupBtn)
	row.AddWidget(removeBtn)
	lf.AddRowSpanLayout(row)
	restoreBtn := qtwidgets.NewQPushButtonWithText("Restore Built-in Themes", nil)
	lf.AddRowWidget(restoreBtn)
	outer.AddLayoutFull(left, 0)

	// ---- right: the editor
	right := qtwidgets.NewQVBoxLayout(nil)
	_, form := optionsSection(right, "Terminal", "")
	name := qtwidgets.NewQLineEdit(page)
	form.AddRowText("Name:", name)
	colorBtns := make([]*qtwidgets.QPushButton, len(themeColorFields))
	for i, f := range themeColorFields {
		b := qtwidgets.NewQPushButton(page)
		b.SetMinimumWidth(110)
		colorBtns[i] = b
		if !f.header {
			form.AddRowText(f.label+":", b)
		}
	}

	_, hf := optionsSection(right, "Pane header", "")
	for i, f := range themeColorFields {
		if f.header {
			hf.AddRowText(f.label+":", colorBtns[i])
		}
	}

	palBox := qtwidgets.NewQGroupBoxWithTitle("Palette", nil)
	grid := qtwidgets.NewQGridLayout(palBox)
	grid.AddWidget(qtwidgets.NewQLabelWithText("Normal", nil), 0, 0)
	grid.AddWidget(qtwidgets.NewQLabelWithText("Bright", nil), 1, 0)
	palBtns := make([]*qtwidgets.QPushButton, 16)
	for i := range palBtns {
		b := qtwidgets.NewQPushButton(palBox)
		b.SetFixedSize(30, 24)
		label := paletteNames[i%8]
		if i >= 8 {
			label = "Bright " + strings.ToLower(label)
		}
		b.SetAccessibleName(label)
		palBtns[i] = b
		grid.AddWidget(b, i/8, 1+i%8)
	}
	right.AddWidget(palBox)
	right.AddStretch()
	outer.AddLayoutFull(right, 1)

	cur := func() *config.ThemeSpec {
		if current < 0 || current >= len(ids) {
			return nil
		}
		s := specs[ids[current]]
		return &s
	}
	store := func(s *config.ThemeSpec) { specs[ids[current]] = *s }

	refresh := func() {
		s := cur()
		if s == nil {
			return
		}
		th, _ := s.Resolve()
		for i, f := range themeColorFields {
			c := f.resolved(th)
			label := config.HexColor(c)
			if *f.get(s) == "" {
				label += " (auto)"
			}
			colorBtns[i].SetText(label)
			colorBtns[i].SetStyleSheet(swatchStyle(c))
		}
		for i, b := range palBtns {
			b.SetStyleSheet(swatchStyle(th.Palette[i]))
			b.SetToolTip(b.AccessibleName() + ": " + config.HexColor(th.Palette[i]))
		}
	}
	load := func(rowIdx int) {
		current = rowIdx
		s := cur()
		if s == nil {
			return
		}
		loading = true
		name.SetText(s.Name)
		loading = false
		refresh()
	}
	rebuildList := func(selectID string) {
		loading = true
		list.Clear()
		sel := 0
		for i, id := range ids {
			list.AddItemText(specs[id].Name)
			if id == selectID {
				sel = i
			}
		}
		loading = false
		list.SetCurrentRow(sel)
		load(sel)
	}
	list.OnCurrentRowChanged(func(r int) {
		if !loading && r >= 0 {
			load(r)
		}
	})
	name.OnTextEdited(func(text string) {
		if loading || cur() == nil {
			return
		}
		s := cur()
		s.Name = text
		store(s)
		list.Item(current).SetText(text)
	})
	pick := func(title string, initial uint32, set func(string)) {
		c := qtwidgets.QColorDialogGetColorFull(mustColor(config.HexColor(initial)), dlg, title, 0)
		if c.IsValid() {
			set(c.Name())
			refresh()
		}
	}
	for i, f := range themeColorFields {
		colorBtns[i].OnClicked(func(bool) {
			s := cur()
			if s == nil {
				return
			}
			th, _ := s.Resolve()
			pick(f.label, f.resolved(th), func(hex string) {
				s := cur()
				*f.get(s) = hex
				store(s)
			})
		})
	}
	for i, b := range palBtns {
		b.OnClicked(func(bool) {
			s := cur()
			if s == nil {
				return
			}
			th, _ := s.Resolve()
			pick(b.AccessibleName(), th.Palette[i], func(hex string) {
				s := cur()
				pal := make([]string, 16)
				for k := range pal {
					if k < len(s.Palette) {
						pal[k] = s.Palette[k]
					} else {
						pal[k] = config.HexColor(th.Palette[k])
					}
				}
				pal[i] = hex
				s.Palette = pal
				store(s)
			})
		})
	}
	addCopy := func(newName string) {
		s := cur()
		if s == nil {
			return
		}
		n := *s
		n.Name = newName
		n.Palette = append([]string(nil), s.Palette...)
		id := config.NewThemeID(specs, newName)
		specs[id] = n
		ids = append(ids, id)
		rebuildList(id)
		name.SetFocus()
		name.SelectAll()
	}
	addBtn.OnClicked(func(bool) { addCopy("New Theme") })
	dupBtn.OnClicked(func(bool) {
		if s := cur(); s != nil {
			addCopy(s.Name + " Copy")
		}
	})
	removeBtn.OnClicked(func(bool) {
		if len(ids) <= 1 || cur() == nil {
			return // at least one theme must remain
		}
		delete(specs, ids[current])
		ids = append(ids[:current], ids[current+1:]...)
		next := min(current, len(ids)-1)
		rebuildList(ids[next])
	})
	restoreBtn.OnClicked(func(bool) {
		msg := "Restore the built-in themes? Changes made to them will be lost; your own themes are kept."
		if qtwidgets.QMessageBoxQuestion(dlg, "Restore Built-in Themes", msg) != qtwidgets.QMessageBoxYes {
			return
		}
		selected := ""
		if current >= 0 && current < len(ids) {
			selected = ids[current]
		}
		for id, spec := range config.BuiltinThemes {
			if _, ok := specs[id]; !ok {
				ids = append(ids, id)
			}
			spec.Palette = append([]string(nil), spec.Palette...)
			specs[id] = spec
		}
		rebuildList(selected)
	})

	removeBtn.SetEnabled(len(ids) > 1)
	list.OnCurrentRowChanged(func(int) { removeBtn.SetEnabled(len(ids) > 1) })
	rebuildList(a.cfg.Theme)

	// read returns the edited themes. Built-in themes keep their ids; the
	// user's own are stored under ids made from their current names, so a
	// renamed copy doesn't keep the id of the theme it was copied from.
	read = func() (map[string]config.ThemeSpec, string) {
		edited := config.CloneThemes(specs)
		out := map[string]config.ThemeSpec{}
		newID := map[string]string{}
		for _, id := range ids {
			s := edited[id]
			if strings.TrimSpace(s.Name) == "" {
				s.Name = id
			}
			edited[id] = s
			if _, builtin := config.BuiltinThemes[id]; builtin {
				out[id], newID[id] = s, id
			}
		}
		for _, id := range ids {
			if _, builtin := config.BuiltinThemes[id]; !builtin {
				nid := config.NewThemeID(out, edited[id].Name)
				out[nid], newID[id] = edited[id], nid
			}
		}
		sel := config.DefaultTheme
		if current >= 0 && current < len(ids) {
			sel = newID[ids[current]]
		}
		return out, sel
	}
	return page, read
}
