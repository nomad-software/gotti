package config

import (
	"sort"

	"gotti/internal/term"
)

// Theme is a colour scheme. Colours are 0xRRGGBB.
type Theme struct {
	Name       string
	Foreground uint32
	Background uint32
	Cursor     uint32
	CursorText uint32
	Selection  uint32 // selection background
	SelText    uint32 // selection foreground
	Palette    [16]uint32
	Dark       bool
}

// Solarized base colours.
const (
	base03  = 0x002b36
	base02  = 0x073642
	base01  = 0x586e75
	base00  = 0x657b83
	base0   = 0x839496
	base1   = 0x93a1a1
	base2   = 0xeee8d5
	base3   = 0xfdf6e3
	yellow  = 0xb58900
	orange  = 0xcb4b16
	red     = 0xdc322f
	magenta = 0xd33682
	violet  = 0x6c71c4
	blue    = 0x268bd2
	cyan    = 0x2aa198
	green   = 0x859900
)

var solarizedPalette = [16]uint32{
	base02, red, green, yellow, blue, magenta, cyan, base2,
	base03, orange, base01, base00, base0, violet, base1, base3,
}

// Themes lists the built-in themes.
var Themes = map[string]Theme{
	"solarized-dark": {
		Name: "Solarized Dark", Foreground: base0, Background: base03,
		Cursor: base1, CursorText: base03, Selection: base02, SelText: base1,
		Palette: solarizedPalette, Dark: true,
	},
	"solarized-light": {
		Name: "Solarized Light", Foreground: base00, Background: base3,
		Cursor: base01, CursorText: base3, Selection: base2, SelText: base01,
		Palette: solarizedPalette,
	},
	// Ubuntu's terminal look: Tango palette on the aubergine background
	// used with the Ambiance desktop theme.
	"ubuntu-ambiance": {
		Name: "Ubuntu Ambiance", Foreground: 0xffffff, Background: 0x300a24,
		Cursor: 0xffffff, CursorText: 0x300a24, Selection: 0x5e2750, SelText: 0xffffff,
		Palette: [16]uint32{
			0x2e3436, 0xcc0000, 0x4e9a06, 0xc4a000, 0x3465a4, 0x75507b, 0x06989a, 0xd3d7cf,
			0x555753, 0xef2929, 0x8ae234, 0xfce94f, 0x729fcf, 0xad7fa8, 0x34e2e2, 0xeeeeec,
		},
		Dark: true,
	},
}

// ThemeNames returns the sorted theme identifiers.
func ThemeNames() []string {
	var n []string
	for k := range Themes {
		n = append(n, k)
	}
	sort.Strings(n)
	return n
}

// ResolveTheme returns the configured theme with overrides applied.
func (c *Config) ResolveTheme() Theme {
	th, ok := Themes[c.Theme]
	if !ok {
		th = Themes["solarized-dark"]
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
