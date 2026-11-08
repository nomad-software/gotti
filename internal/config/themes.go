package config

import (
	"fmt"
	"maps"
	"regexp"
	"sort"
	"strings"

	"gotti/internal/term"
)

// ThemeSpec is a colour theme as stored in the config file. Colours are
// "#rrggbb" (or any X11 spec term.ParseColor accepts). CursorText,
// Selection and SelectionText may be empty for sensible defaults.
type ThemeSpec struct {
	Name          string   `json:"name"`
	Foreground    string   `json:"foreground"`
	Background    string   `json:"background"`
	Cursor        string   `json:"cursor"`
	CursorText    string   `json:"cursor_text,omitempty"`
	Selection     string   `json:"selection,omitempty"`
	SelectionText string   `json:"selection_text,omitempty"`
	Palette       []string `json:"palette"` // 16 colours: 8 normal, then 8 bright
}

// Theme is a resolved theme, with colours as 0xRRGGBB.
type Theme struct {
	Name       string
	Foreground uint32
	Background uint32
	Cursor     uint32
	CursorText uint32
	Selection  uint32 // selection background
	SelText    uint32 // selection foreground (0 keeps text colours)
	Palette    [16]uint32
}

// Solarized base colours.
const (
	base03  = "#002b36"
	base02  = "#073642"
	base01  = "#586e75"
	base00  = "#657b83"
	base0   = "#839496"
	base1   = "#93a1a1"
	base2   = "#eee8d5"
	base3   = "#fdf6e3"
	yellow  = "#b58900"
	orange  = "#cb4b16"
	red     = "#dc322f"
	magenta = "#d33682"
	violet  = "#6c71c4"
	blue    = "#268bd2"
	cyan    = "#2aa198"
	green   = "#859900"
)

var solarizedPalette = []string{
	base02, red, green, yellow, blue, magenta, cyan, base2,
	base03, orange, base01, base00, base0, violet, base1, base3,
}

// BuiltinThemes are the themes Gotti ships with. They seed the config's
// theme list and can be restored from the options dialog.
var BuiltinThemes = map[string]ThemeSpec{
	"solarized-dark": {
		Name: "Solarized Dark", Foreground: base0, Background: base03,
		Cursor: base1, CursorText: base03, Selection: base02, SelectionText: base1,
		Palette: solarizedPalette,
	},
	"solarized-light": {
		Name: "Solarized Light", Foreground: base00, Background: base3,
		Cursor: base01, CursorText: base3, Selection: base2, SelectionText: base01,
		Palette: solarizedPalette,
	},
	// Ubuntu's terminal look: Tango palette on the aubergine background
	// used with the Ambiance desktop theme.
	"ubuntu-ambiance": {
		Name: "Ubuntu Ambiance", Foreground: "#ffffff", Background: "#300a24",
		Cursor: "#ffffff", CursorText: "#300a24", Selection: "#5e2750", SelectionText: "#ffffff",
		Palette: []string{
			"#2e3436", "#cc0000", "#4e9a06", "#c4a000", "#3465a4", "#75507b", "#06989a", "#d3d7cf",
			"#555753", "#ef2929", "#8ae234", "#fce94f", "#729fcf", "#ad7fa8", "#34e2e2", "#eeeeec",
		},
	},
}

// DefaultTheme is the theme used when none is configured.
const DefaultTheme = "solarized-dark"

// builtinThemes returns a copy of the built-in theme list.
func builtinThemes() map[string]ThemeSpec {
	m := make(map[string]ThemeSpec, len(BuiltinThemes))
	for id, t := range BuiltinThemes {
		t.Palette = append([]string(nil), t.Palette...)
		m[id] = t
	}
	return m
}

// Resolve parses the theme's colours.
func (s ThemeSpec) Resolve() (Theme, error) {
	t := Theme{Name: s.Name}
	var errs []string
	parse := func(field, v string, dst *uint32, optional bool) {
		if v == "" && optional {
			return
		}
		c, ok := term.ParseColor(v)
		if !ok {
			errs = append(errs, fmt.Sprintf("%s %q is not a colour", field, v))
			return
		}
		*dst = c
	}
	parse("foreground", s.Foreground, &t.Foreground, false)
	parse("background", s.Background, &t.Background, false)
	t.Cursor = t.Foreground
	parse("cursor", s.Cursor, &t.Cursor, true)
	t.CursorText = t.Background
	parse("cursor_text", s.CursorText, &t.CursorText, true)
	t.Selection = mix(t.Background, t.Foreground, 0.25)
	parse("selection", s.Selection, &t.Selection, true)
	parse("selection_text", s.SelectionText, &t.SelText, true)
	if len(s.Palette) != 16 {
		errs = append(errs, fmt.Sprintf("palette has %d colours, not 16", len(s.Palette)))
	}
	for i := 0; i < len(s.Palette) && i < 16; i++ {
		parse(fmt.Sprintf("palette[%d]", i), s.Palette[i], &t.Palette[i], false)
	}
	if len(errs) > 0 {
		return t, fmt.Errorf("theme %q: %s", s.Name, strings.Join(errs, "; "))
	}
	return t, nil
}

func mix(a, b uint32, f float64) uint32 {
	ch := func(s uint) uint32 {
		return uint32(float64(a>>s&0xff)*(1-f)+float64(b>>s&0xff)*f) << s
	}
	return ch(16) | ch(8) | ch(0)
}

// ThemeIDs returns the configured theme ids, ordered by name.
func (c *Config) ThemeIDs() []string {
	ids := make([]string, 0, len(c.Themes))
	for id := range c.Themes {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		a, b := strings.ToLower(c.Themes[ids[i]].Name), strings.ToLower(c.Themes[ids[j]].Name)
		if a != b {
			return a < b
		}
		return ids[i] < ids[j]
	})
	return ids
}

// ResolveTheme returns the selected theme with colour overrides applied. A
// missing or broken theme falls back to the built-in default.
func (c *Config) ResolveTheme() Theme {
	th, err := c.Themes[c.Theme].Resolve()
	if _, ok := c.Themes[c.Theme]; !ok || err != nil {
		th, _ = BuiltinThemes[DefaultTheme].Resolve()
	}
	if o := c.Colors; o != nil {
		set := func(dst *uint32, s string) {
			if v, ok := term.ParseColor(s); ok {
				*dst = v
			}
		}
		set(&th.Foreground, o.Foreground)
		set(&th.Background, o.Background)
		set(&th.Cursor, o.Cursor)
		set(&th.Selection, o.Selection)
		for i, s := range o.Palette {
			if i < 16 {
				set(&th.Palette[i], s)
			}
		}
	}
	return th
}

// Palette256 expands the theme to a full 256-colour palette.
func (t Theme) Palette256() [256]uint32 { return term.Palette256(t.Palette) }

// HexColor formats 0xRRGGBB as "#rrggbb".
func HexColor(c uint32) string { return fmt.Sprintf("#%06x", c&0xffffff) }

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// NewThemeID returns an unused id for a theme called name.
func NewThemeID(themes map[string]ThemeSpec, name string) string {
	base := strings.Trim(slugRe.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if base == "" {
		base = "theme"
	}
	id := base
	for n := 2; ; n++ {
		if _, taken := themes[id]; !taken {
			return id
		}
		id = fmt.Sprintf("%s-%d", base, n)
	}
}

// CloneThemes deep-copies a theme map.
func CloneThemes(m map[string]ThemeSpec) map[string]ThemeSpec {
	out := maps.Clone(m)
	for id, t := range out {
		t.Palette = append([]string(nil), t.Palette...)
		out[id] = t
	}
	return out
}
