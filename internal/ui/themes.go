package ui

import (
	"fmt"
	"strings"

	qt "github.com/mappu/miqt/qt6"

	"gotti/internal/config"
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
func (a *App) themesTab(dlg *qt.QDialog) (page *qt.QWidget, read func() (map[string]config.ThemeSpec, string)) {
	specs := config.CloneThemes(a.cfg.Themes)
	ids := a.cfg.ThemeIDs()
	current := -1
	loading := false

	page = qt.NewQWidget(nil)
	outer := qt.NewQHBoxLayout(page)

	// ---- left: the list and its buttons
	left := qt.NewQVBoxLayout(nil)
	_, lf := optionsSection(left, "Themes", "The selected theme is used when you press OK or Apply.")
	list := qt.NewQListWidget(page)
	list.SetMinimumWidth(170)
	lf.AddRowWithWidget(list.QWidget)
	addBtn := qt.NewQPushButton3("Add")
	dupBtn := qt.NewQPushButton3("Duplicate")
	removeBtn := qt.NewQPushButton3("Remove")
	row := qt.NewQHBoxLayout(nil)
	row.AddWidget(addBtn.QWidget)
	row.AddWidget(dupBtn.QWidget)
	row.AddWidget(removeBtn.QWidget)
	lf.AddRowWithLayout(row.QLayout)
	restoreBtn := qt.NewQPushButton3("Restore Built-in Themes")
	lf.AddRowWithWidget(restoreBtn.QWidget)
	outer.AddLayout2(left.QLayout, 0)

	// ---- right: the editor
	right := qt.NewQVBoxLayout(nil)
	_, form := optionsSection(right, "Terminal", "")
	name := qt.NewQLineEdit(page)
	form.AddRow3("Name:", name.QWidget)
	colorBtns := make([]*qt.QPushButton, len(themeColorFields))
	for i, f := range themeColorFields {
		b := qt.NewQPushButton(page)
		b.SetMinimumWidth(110)
		colorBtns[i] = b
		if !f.header {
			form.AddRow3(f.label+":", b.QWidget)
		}
	}

	_, hf := optionsSection(right, "Pane header", "")
	for i, f := range themeColorFields {
		if f.header {
			hf.AddRow3(f.label+":", colorBtns[i].QWidget)
		}
	}

	palBox := qt.NewQGroupBox3("Palette")
	grid := qt.NewQGridLayout(palBox.QWidget)
	grid.AddWidget2(qt.NewQLabel3("Normal").QWidget, 0, 0)
	grid.AddWidget2(qt.NewQLabel3("Bright").QWidget, 1, 0)
	palBtns := make([]*qt.QPushButton, 16)
	for i := range palBtns {
		b := qt.NewQPushButton(palBox.QWidget)
		b.SetFixedSize2(30, 24)
		label := paletteNames[i%8]
		if i >= 8 {
			label = "Bright " + strings.ToLower(label)
		}
		b.SetAccessibleName(label)
		palBtns[i] = b
		grid.AddWidget2(b.QWidget, i/8, 1+i%8)
	}
	right.AddWidget(palBox.QWidget)
	right.AddStretch()
	outer.AddLayout2(right.QLayout, 1)

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
			list.AddItem(specs[id].Name)
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
		init := qt.NewQColor6(config.HexColor(initial))
		// GetColor returns a copy that miqt frees itself.
		c := qt.QColorDialog_GetColor3(init, dlg.QWidget, title)
		init.Delete()
		if c.IsValid() {
			set(c.Name())
			refresh()
		}
	}
	for i, f := range themeColorFields {
		f := f
		colorBtns[i].OnClicked(func() {
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
		i, b := i, b
		b.OnClicked(func() {
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
	addBtn.OnClicked(func() { addCopy("New Theme") })
	dupBtn.OnClicked(func() {
		if s := cur(); s != nil {
			addCopy(s.Name + " Copy")
		}
	})
	removeBtn.OnClicked(func() {
		if len(ids) <= 1 || cur() == nil {
			return // at least one theme must remain
		}
		delete(specs, ids[current])
		ids = append(ids[:current], ids[current+1:]...)
		next := min(current, len(ids)-1)
		rebuildList(ids[next])
	})
	restoreBtn.OnClicked(func() {
		msg := "Restore the built-in themes? Changes made to them will be lost; your own themes are kept."
		if qt.QMessageBox_Question(dlg.QWidget, "Restore Built-in Themes", msg) != qt.QMessageBox__Yes {
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
