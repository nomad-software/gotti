// Package config loads gotti's JSON configuration, watches it for changes
// and defines the built-in colour themes.
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Config is the user configuration.
type Config struct {
	FontFamily   string            `json:"font_family"`
	FontSize     float64           `json:"font_size"`
	LineHeight   float64           `json:"line_height"`
	Theme        string            `json:"theme"`
	Scrollback   int               `json:"scrollback"`
	CursorStyle  string            `json:"cursor_style"` // block, beam, underline
	CursorBlink  bool              `json:"cursor_blink"`
	CopyOnSelect bool              `json:"copy_on_select"`
	PasteWarning string            `json:"paste_warning"` // always, unbracketed, never
	Ligatures    bool              `json:"ligatures"`
	Shell        []string          `json:"shell"`
	Padding      int               `json:"padding"`
	OSC52        string            `json:"osc52"` // off, write, read-write
	WordChars    string            `json:"word_chars"`
	BoldIsBright bool              `json:"bold_is_bright"`
	Keybindings  map[string]string `json:"keybindings"`
	Colors       *ColorOverrides   `json:"colors,omitempty"`

	// Window geometry, saved when a window closes and used for new ones.
	WindowWidth     int  `json:"window_width"`
	WindowHeight    int  `json:"window_height"`
	WindowMaximized bool `json:"window_maximized"`
}

// ColorOverrides lets users tweak the active theme.
type ColorOverrides struct {
	Foreground string   `json:"foreground,omitempty"`
	Background string   `json:"background,omitempty"`
	Cursor     string   `json:"cursor,omitempty"`
	Selection  string   `json:"selection,omitempty"`
	Palette    []string `json:"palette,omitempty"`
}

// DefaultKeybindings maps actions to key chords.
var DefaultKeybindings = map[string]string{
	"copy":             "ctrl+shift+c",
	"paste":            "ctrl+shift+v",
	"paste_selection":  "shift+insert",
	"new_tab":          "ctrl+shift+t",
	"close_pane":       "ctrl+shift+w",
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
	"broadcast_all":    "ctrl+shift+a",
	"broadcast_group":  "ctrl+shift+b",
	"group_toggle":     "ctrl+shift+g",
	"save_layout":      "ctrl+shift+s",
	"load_layout":      "ctrl+shift+l",
	"reload_config":    "ctrl+shift+r",
	"new_window":       "ctrl+shift+n",
	"context_menu":     "menu",
	"clear_scrollback": "",
	"reset_terminal":   "",
	"edit_config":      "",
	"options":          "",
}

// Default returns the built-in configuration.
func Default() *Config {
	kb := map[string]string{}
	for k, v := range DefaultKeybindings {
		kb[k] = v
	}
	return &Config{
		FontFamily:   "monospace",
		FontSize:     11,
		LineHeight:   1.0,
		Theme:        "solarized-dark",
		Scrollback:   10000,
		CursorStyle:  "block",
		CopyOnSelect: true,
		PasteWarning: "unbracketed",
		Padding:      2,
		OSC52:        "write",
		WordChars:    "-_./~:@%+#=?&",
		Shell:        []string{},
		WindowWidth:  900,
		WindowHeight: 560,
		Keybindings:  kb,
	}
}

// Dir returns the configuration directory.
func Dir() string {
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "gotti")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "gotti")
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
	if err := json.Unmarshal(data, &out); err != nil {
		return c, fmt.Errorf("%s: %w", path, describeJSONError(data, err))
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

func (c *Config) normalize() {
	if c.FontSize <= 0 {
		c.FontSize = 11
	}
	if c.LineHeight <= 0 {
		c.LineHeight = 1
	}
	if c.Scrollback < 0 {
		c.Scrollback = 0
	}
	if c.Scrollback > 1000000 {
		c.Scrollback = 1000000
	}
	if c.Padding < 0 {
		c.Padding = 0
	}
}

// Validate reports configuration problems.
func (c *Config) Validate() error {
	var errs []string
	if _, ok := Themes[c.Theme]; !ok {
		errs = append(errs, fmt.Sprintf("unknown theme %q (have %s)", c.Theme, strings.Join(ThemeNames(), ", ")))
	}
	switch c.CursorStyle {
	case "block", "beam", "underline":
	default:
		errs = append(errs, fmt.Sprintf("cursor_style must be block, beam or underline, not %q", c.CursorStyle))
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
	for action, chord := range c.Keybindings {
		if _, ok := DefaultKeybindings[action]; !ok {
			errs = append(errs, fmt.Sprintf("unknown keybinding action %q", action))
			continue
		}
		if chord == "" || chord == "none" {
			continue
		}
		if _, err := ParseChord(chord); err != nil {
			errs = append(errs, fmt.Sprintf("keybinding %s: %v", action, err))
		}
	}
	if len(errs) > 0 {
		sort.Strings(errs)
		return fmt.Errorf("config: %s", strings.Join(errs, "; "))
	}
	return nil
}

// Watch polls the file and calls fn with the new configuration whenever
// it changes. fn runs on the watcher goroutine. Stop with the returned func.
func Watch(path string, interval time.Duration, fn func(*Config, error)) func() {
	stop := make(chan struct{})
	modTime := func() time.Time {
		st, err := os.Stat(path)
		if err != nil {
			return time.Time{}
		}
		return st.ModTime()
	}
	last := modTime()
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				if m := modTime(); !m.Equal(last) {
					last = m
					c, err := Load(path)
					fn(c, err)
				}
			}
		}
	}()
	return func() { close(stop) }
}

// Chord is a parsed key binding.
type Chord struct {
	Ctrl, Shift, Alt, Super bool
	Key                     string // normalized key name, e.g. "c", "pageup", "f5", "equal"
}

var keyAliases = map[string]string{
	"=": "equal", "+": "plus", "-": "minus", "pgup": "pageup", "pgdn": "pagedown", "pgdown": "pagedown",
	"return": "enter", "esc": "escape", "del": "delete", "ins": "insert", "spc": "space", " ": "space",
	",": "comma", ".": "period", "/": "slash", "\\": "backslash", ";": "semicolon", "'": "apostrophe",
	"[": "bracketleft", "]": "bracketright", "`": "grave",
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

// ParseBindings converts the config map into a lookup table.
func (c *Config) ParseBindings() Bindings {
	b := Bindings{}
	for action, s := range c.Keybindings {
		if s == "" || s == "none" {
			continue
		}
		if ch, err := ParseChord(s); err == nil {
			b[ch] = action
		}
	}
	return b
}

// SampleConfig returns the default configuration as indented JSON. It is
// written by --write-config and when the menu first saves a setting.
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
