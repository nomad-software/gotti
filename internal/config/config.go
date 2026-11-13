// Package config loads termite's JSON configuration, writes settings back
// to it and defines the built-in colour themes.
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"unicode"
)

// Config is the user configuration.
type Config struct {
	FontFamily string  `json:"font_family"`
	FontSize   float64 `json:"font_size"`
	// The pane headers' font: "" and 0 follow the terminal's font, at
	// HeaderFontScale of its size.
	HeaderFontFamily string            `json:"header_font_family"`
	HeaderFontSize   float64           `json:"header_font_size"`
	LineHeight       float64           `json:"line_height"`
	Theme            string            `json:"theme"`
	WindowTheme      string            `json:"window_theme"` // system, light, dark
	Scrollback       int               `json:"scrollback"`
	CursorStyle      string            `json:"cursor_style"` // block, beam, underline
	CursorBlink      bool              `json:"cursor_blink"`
	CopyOnSelect     bool              `json:"copy_on_select"`
	PasteWarning     string            `json:"paste_warning"` // always, unbracketed, never
	Ligatures        bool              `json:"ligatures"`
	Shell            []string          `json:"shell"`
	Padding          int               `json:"padding"`
	OSC52            string            `json:"osc52"` // off, write, read-write
	WordChars        string            `json:"word_chars"`
	BoldIsBright     bool              `json:"bold_is_bright"`
	Scrollbar        bool              `json:"scrollbar"`
	Toolbar          bool              `json:"toolbar"`
	Keybindings      map[string]string `json:"keybindings"`
	// Themes are the available colour themes by id; Theme picks one. When
	// the file has a "themes" object it replaces the built-in one entirely.
	Themes map[string]ThemeSpec `json:"themes"`

	// Window geometry, saved when a window closes and used for new ones.
	WindowWidth     int  `json:"window_width"`
	WindowHeight    int  `json:"window_height"`
	WindowMaximized bool `json:"window_maximized"`
}

// DefaultKeybindings maps actions to key chords.
var DefaultKeybindings = map[string]string{
	"copy":             "ctrl+shift+c",
	"paste":            "ctrl+shift+v",
	"new_tab":          "ctrl+shift+t",
	"close_pane":       "ctrl+shift+q",
	"exit":             "ctrl+w",
	"split_horizontal": "ctrl+shift+o", // stacked: one above the other
	"split_vertical":   "ctrl+shift+e", // side by side
	"focus_up":         "alt+up",
	"focus_down":       "alt+down",
	"focus_left":       "alt+left",
	"focus_right":      "alt+right",
	"next_tab":         "ctrl+pagedown",
	"prev_tab":         "ctrl+pageup",
	"move_tab_left":    "ctrl+shift+pageup",
	"move_tab_right":   "ctrl+shift+pagedown",
	"search":           "ctrl+shift+f",
	"scroll_page_up":   "shift+pageup",
	"scroll_page_down": "shift+pagedown",
	"scroll_top":       "shift+home",
	"scroll_bottom":    "shift+end",
	"font_bigger":      "ctrl+equal",
	"font_smaller":     "ctrl+minus",
	"font_reset":       "ctrl+0",
	"broadcast_all":    "ctrl+shift+b",
	"broadcast_group":  "",
	"group_toggle":     "ctrl+shift+g",
	"save_layout":      "ctrl+shift+s",
	"load_layout":      "ctrl+shift+l",
	"reload_config":    "ctrl+shift+r",
	"new_window":       "ctrl+n",
	"context_menu":     "menu",
	"clear_scrollback": "",
	"reset_terminal":   "",
	"edit_config":      "",
	"options":          "ctrl+comma",
}

// removedActions are keybinding actions termite no longer has. Config files
// saved by the Options dialog list every action, so these are dropped
// quietly rather than reported as unknown.
var removedActions = []string{"paste_selection"}

// Default returns the built-in configuration.
func Default() *Config {
	kb := map[string]string{}
	for k, v := range DefaultKeybindings {
		kb[k] = v
	}
	return &Config{
		FontFamily:       "Iosevka Term Curly",
		FontSize:         15,
		HeaderFontFamily: "Iosevka Term Curly",
		HeaderFontSize:   12,
		LineHeight:       1.0,
		Theme:            DefaultTheme,
		WindowTheme:      "system",
		Themes:           CloneThemes(BuiltinThemes),
		Scrollback:       10000,
		CursorStyle:      "block",
		CopyOnSelect:     true,
		Scrollbar:        true,
		Toolbar:          true,
		PasteWarning:     "unbracketed",
		Padding:          0,
		OSC52:            "write",
		WordChars:        "-_./~:@%+#=?&",
		Shell:            []string{},
		WindowWidth:      900,
		WindowHeight:     560,
		Keybindings:      kb,
	}
}

// HeaderFontScale sizes the pane headers' font relative to the terminal's,
// when header_font_size is not set.
const HeaderFontScale = 0.85

// HeaderFont returns the pane headers' font family and size in points.
func (c *Config) HeaderFont() (family string, size float64) {
	family, size = c.HeaderFontFamily, c.HeaderFontSize
	if family == "" {
		family = c.FontFamily
	}
	if size <= 0 {
		size = math.Round(c.FontSize*HeaderFontScale*10) / 10
	}
	return family, size
}

// Dir returns the configuration directory.
func Dir() string {
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "termite")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "termite")
}

// Path returns the configuration file path.
func Path() string { return filepath.Join(Dir(), "config.json") }

// Load reads the configuration file, falling back to defaults for
// anything missing. A missing file is not an error.
func Load(path string) (*Config, error) {
	c := Default()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	// Decoding over the defaults keeps unset fields, and merges the
	// keybindings map with the default bindings.
	out := *c
	out.Keybindings = c.Keybindings
	out.Themes = nil // a themes object in the file replaces the built-in one
	// A value of the wrong type is reported, but the rest of the file still
	// applies: the decoder carries on past it, leaving that field at its
	// default. Only a file that isn't JSON falls back to the defaults.
	var typeErr error
	if err := json.Unmarshal(data, &out); err != nil {
		var te *json.UnmarshalTypeError
		if !errors.As(err, &te) {
			return c, fmt.Errorf("%s: %w", path, describeJSONError(data, err))
		}
		typeErr = fmt.Errorf("%s: %w", path, err)
	}
	if out.Themes == nil {
		out.Themes = c.Themes
	}
	if out.Keybindings == nil { // "keybindings": null
		out.Keybindings = Default().Keybindings
	}
	// A strict second pass reports misspelt keys without rejecting the file.
	var strictErr error
	strict := json.NewDecoder(bytes.NewReader(data))
	strict.DisallowUnknownFields()
	var probe Config
	if err := strict.Decode(&probe); err != nil {
		strictErr = fmt.Errorf("%s: %w", path, err)
	}
	out.normalize()
	if typeErr != nil {
		return &out, typeErr
	}
	if err := out.Validate(); err != nil {
		return &out, err
	}
	return &out, strictErr
}

// describeJSONError adds a line number to JSON syntax errors.
func describeJSONError(data []byte, err error) error {
	var se *json.SyntaxError
	if errors.As(err, &se) {
		line := 1 + bytes.Count(data[:min(int(se.Offset), len(data))], []byte("\n"))
		return fmt.Errorf("line %d: %w", line, err)
	}
	return err
}

// The ranges of the settings with limits, which the Options dialog offers
// too. Values outside them are brought within: a huge padding or font
// would overflow the renderer's 16-bit coordinates and the glyph atlas.
const (
	MinFontSize, MaxFontSize     = 5.0, 72.0
	MinLineHeight, MaxLineHeight = 0.8, 2.5
	MaxPadding                   = 50
	MaxScrollback                = 1000000
)

func (c *Config) normalize() {
	if c.FontSize <= 0 {
		c.FontSize = 11
	}
	c.FontSize = min(max(c.FontSize, MinFontSize), MaxFontSize)
	if c.LineHeight <= 0 {
		c.LineHeight = 1
	}
	c.LineHeight = min(max(c.LineHeight, MinLineHeight), MaxLineHeight)
	if c.HeaderFontSize > 0 { // 0 follows the terminal's font
		c.HeaderFontSize = min(max(c.HeaderFontSize, MinFontSize), MaxFontSize)
	} else {
		c.HeaderFontSize = 0
	}
	c.Scrollback = min(max(c.Scrollback, 0), MaxScrollback)
	c.Padding = min(max(c.Padding, 0), MaxPadding)
	if c.WindowTheme == "" {
		c.WindowTheme = "system"
	}
	for _, action := range removedActions {
		delete(c.Keybindings, action)
	}
}

// Validate reports configuration problems.
func (c *Config) Validate() error {
	var errs []string
	if len(c.Themes) == 0 {
		errs = append(errs, "themes is empty")
	} else if _, ok := c.Themes[c.Theme]; !ok {
		errs = append(errs, fmt.Sprintf("unknown theme %q (have %s)", c.Theme, strings.Join(c.ThemeIDs(), ", ")))
	}
	for _, id := range c.ThemeIDs() {
		spec := c.Themes[id]
		if spec.Name == "" {
			spec.Name = id // so a problem says which theme
		}
		if _, err := spec.Resolve(); err != nil {
			errs = append(errs, err.Error())
		}
	}
	switch c.CursorStyle {
	case "block", "beam", "underline":
	default:
		errs = append(errs, fmt.Sprintf("cursor_style must be block, beam or underline, not %q", c.CursorStyle))
	}
	switch c.WindowTheme {
	case "system", "light", "dark":
	default:
		errs = append(errs, fmt.Sprintf("window_theme must be system, light or dark, not %q", c.WindowTheme))
	}
	switch c.PasteWarning {
	case "always", "unbracketed", "never":
	default:
		errs = append(errs, fmt.Sprintf("paste_warning must be always, unbracketed or never, not %q", c.PasteWarning))
	}
	switch c.OSC52 {
	case "off", "write", "read-write":
	default:
		errs = append(errs, fmt.Sprintf("osc52 must be off, write or read-write, not %q", c.OSC52))
	}
	byChord := map[Chord][]string{}
	for action, chord := range c.Keybindings {
		if _, ok := DefaultKeybindings[action]; !ok {
			errs = append(errs, fmt.Sprintf("unknown keybinding action %q", action))
			continue
		}
		if chord == "" || chord == "none" {
			continue
		}
		ch, err := ParseChord(chord)
		if err != nil {
			errs = append(errs, fmt.Sprintf("keybinding %s: %v", action, err))
			continue
		}
		byChord[ch] = append(byChord[ch], action)
	}
	for ch, actions := range byChord {
		if len(actions) > 1 {
			sort.Strings(actions)
			errs = append(errs, fmt.Sprintf("keybindings %s all use %s", strings.Join(actions, ", "), ch))
		}
	}
	if len(errs) > 0 {
		sort.Strings(errs)
		return fmt.Errorf("config: %s", strings.Join(errs, "; "))
	}
	return nil
}

// Chord is a parsed key binding.
type Chord struct {
	Ctrl, Shift, Alt, Super bool
	Key                     string // normalized key name, e.g. "c", "pageup", "f5", "equal"
}

// keyAliases are other names for keys. A shifted symbol is matched by the
// key it is on, so plus is the = key: "ctrl+plus" fires for Ctrl and the
// key with + on it, which needs Shift too where + is shifted.
var keyAliases = map[string]string{
	"=": "equal", "plus": "equal", "-": "minus", "pgup": "pageup", "pgdn": "pagedown", "pgdown": "pagedown",
	"return": "enter", "esc": "escape", "del": "delete", "ins": "insert", "spc": "space",
	",": "comma", ".": "period", "/": "slash", "\\": "backslash", ";": "semicolon", "'": "apostrophe",
	"[": "bracketleft", "]": "bracketright", "`": "grave",
}

// KeyNames are the names of the keys a chord can use besides a single
// letter or digit: those key presses are matched by.
var KeyNames = map[string]bool{
	"escape": true, "tab": true, "backspace": true, "enter": true, "insert": true, "delete": true,
	"home": true, "end": true, "left": true, "up": true, "right": true, "down": true,
	"pageup": true, "pagedown": true, "menu": true, "space": true,
	"f1": true, "f2": true, "f3": true, "f4": true, "f5": true, "f6": true,
	"f7": true, "f8": true, "f9": true, "f10": true, "f11": true, "f12": true,
	"equal": true, "minus": true, "comma": true, "period": true, "slash": true, "backslash": true,
	"semicolon": true, "apostrophe": true, "bracketleft": true, "bracketright": true, "grave": true,
}

// ParseChord parses strings like "ctrl+shift+c".
func ParseChord(s string) (Chord, error) {
	var c Chord
	parts := strings.Split(strings.ToLower(strings.TrimSpace(s)), "+")
	// allow "ctrl++" for plus
	if strings.HasSuffix(s, "++") {
		parts = append(parts[:len(parts)-2], "plus")
	}
	for i, p := range parts {
		p = strings.TrimSpace(p)
		if i < len(parts)-1 {
			switch p {
			case "ctrl", "control":
				c.Ctrl = true
			case "shift":
				c.Shift = true
			case "alt", "meta":
				c.Alt = true
			case "super", "win", "cmd":
				c.Super = true
			default:
				return c, fmt.Errorf("unknown modifier %q in %q", p, s)
			}
			continue
		}
		if a, ok := keyAliases[p]; ok {
			p = a
		}
		if p == "" {
			return c, fmt.Errorf("missing key in %q", s)
		}
		if r := []rune(p); !KeyNames[p] && (len(r) != 1 || !unicode.IsLetter(r[0]) && !unicode.IsDigit(r[0])) {
			return c, fmt.Errorf("unknown key %q in %q", p, s)
		}
		c.Key = p
	}
	return c, nil
}

func (c Chord) String() string {
	var parts []string
	if c.Ctrl {
		parts = append(parts, "ctrl")
	}
	if c.Shift {
		parts = append(parts, "shift")
	}
	if c.Alt {
		parts = append(parts, "alt")
	}
	if c.Super {
		parts = append(parts, "super")
	}
	return strings.Join(append(parts, c.Key), "+")
}

// Bindings maps chords to actions.
type Bindings map[Chord]string

// ParseBindings converts the config map into a lookup table. Where actions
// share a chord (which Validate reports) the first by name has it, so the
// same one does each time.
func (c *Config) ParseBindings() Bindings {
	b := Bindings{}
	for _, action := range slices.Sorted(maps.Keys(c.Keybindings)) {
		s := c.Keybindings[action]
		if s == "" || s == "none" {
			continue
		}
		if ch, err := ParseChord(s); err == nil {
			if _, taken := b[ch]; !taken {
				b[ch] = action
			}
		}
	}
	return b
}

// SampleConfig returns the default configuration as indented JSON. It is
// written by --write-config and by Edit Config File when there is none.
func SampleConfig() []byte {
	b, _ := marshalIndent(Default())
	return b
}

// marshalIndent encodes readable JSON (no HTML escaping, trailing newline).
func marshalIndent(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	err := enc.Encode(v)
	return buf.Bytes(), err
}
