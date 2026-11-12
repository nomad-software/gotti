package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	c, err := Load(p)
	if err != nil || c.Theme != "solarized-dark" {
		t.Fatal(c, err)
	}
	os.WriteFile(p, []byte(`{"theme": "ubuntu-ambiance", "font_size": 14, "keybindings": {"copy": "ctrl+alt+c"}}`), 0o644)
	c, err = Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.Theme != "ubuntu-ambiance" || c.FontSize != 14 || c.Keybindings["copy"] != "ctrl+alt+c" || c.Keybindings["paste"] != "ctrl+shift+v" {
		t.Fatalf("%+v", c)
	}
	if th := c.ResolveTheme(); th.Background != 0x300a24 || th.Foreground != 0xffffff {
		t.Fatalf("%x", th.Background)
	}
	b := c.ParseBindings()
	if b[Chord{Ctrl: true, Alt: true, Key: "c"}] != "copy" || b[Chord{Ctrl: true, Key: "equal"}] != "font_bigger" {
		t.Fatal("bindings", b)
	}
	os.WriteFile(p, []byte(`{"theme": "nope"}`), 0o644)
	if _, err := Load(p); err == nil {
		t.Fatal("expected validation error")
	}
	os.WriteFile(p, []byte("{\n  \"theme\": \"solarized-light\",\n  \"font_sise\": 12\n}"), 0o644)
	if c, err := Load(p); err == nil || c.Theme != "solarized-light" {
		t.Fatalf("unknown key should warn but still apply the rest: %v %v", c.Theme, err)
	}
	os.WriteFile(p, []byte("{\n  \"theme\": \"solarized-light\"\n  \"x\": 1\n}"), 0o644)
	if _, err := Load(p); err == nil || !strings.Contains(err.Error(), "line 3") {
		t.Fatalf("syntax error should name the line: %v", err)
	}
}

func TestSetValues(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	if err := SetValues(p, map[string]any{"theme": "ubuntu-ambiance"}); err != nil {
		t.Fatal(err)
	}
	if c, err := Load(p); err != nil || c.Theme != "ubuntu-ambiance" || c.FontSize != 11 {
		t.Fatalf("new file from defaults: %+v %v", c, err)
	}
	os.WriteFile(p, []byte(`{"font_size": 12, "keybindings": {"copy": "ctrl+c"}}`), 0o644)
	SetValues(p, map[string]any{"ligatures": true})
	SetValues(p, map[string]any{"font_size": 14.0})
	SetValues(p, map[string]any{"theme": "solarized-light"})
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if !c.Ligatures || c.FontSize != 14 || c.Theme != "solarized-light" || c.Keybindings["copy"] != "ctrl+c" {
		t.Fatalf("%+v", c)
	}
}

func TestThemes(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	c, _ := Load(p)
	if len(c.Themes) != len(BuiltinThemes) || c.ResolveTheme().Background != 0x002b36 {
		t.Fatalf("defaults: %d themes, bg %x", len(c.Themes), c.ResolveTheme().Background)
	}
	for id, spec := range BuiltinThemes {
		if _, err := spec.Resolve(); err != nil {
			t.Errorf("built-in %s: %v", id, err)
		}
	}
	// A themes list in the file replaces the built-in one.
	os.WriteFile(p, []byte(`{"theme": "mine", "themes": {"mine": {"name": "Mine", "foreground": "#eeeeee",
		"background": "#101820", "cursor": "#ff8800", "palette": ["#000000","#aa0000","#00aa00","#aa5500",
		"#0000aa","#aa00aa","#00aaaa","#aaaaaa","#555555","#ff5555","#55ff55","#ffff55","#5555ff","#ff55ff",
		"#55ffff","#ffffff"]}}}`), 0o644)
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Themes) != 1 || c.ThemeIDs()[0] != "mine" {
		t.Fatalf("themes not replaced: %v", c.ThemeIDs())
	}
	th := c.ResolveTheme()
	if th.Background != 0x101820 || th.Cursor != 0xff8800 || th.Palette[9] != 0xff5555 {
		t.Fatalf("resolved %+v", th)
	}
	// Optional colours get defaults derived from the others.
	if th.CursorText != th.Background || th.Selection == 0 {
		t.Fatalf("derived colours %+v", th)
	}
	// A broken theme is reported, and the default is used meanwhile.
	os.WriteFile(p, []byte(`{"theme": "bad", "themes": {"bad": {"name": "Bad", "foreground": "nope", "background": "#000000", "palette": []}}}`), 0o644)
	c, err = Load(p)
	if err == nil || !strings.Contains(err.Error(), "not a colour") || !strings.Contains(err.Error(), "palette has 0") {
		t.Fatalf("broken theme not reported: %v", err)
	}
	if c.ResolveTheme().Background != 0x002b36 {
		t.Fatal("broken theme should fall back to the default")
	}
	if id := NewThemeID(map[string]ThemeSpec{"my-theme": {}}, "My Theme!"); id != "my-theme-2" {
		t.Fatalf("NewThemeID = %q", id)
	}
}

// A config file that is a symlink stays one: the settings are written to
// the file it points at.
func TestSetValuesKeepsSymlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "dotfiles", "termite.json")
	if err := os.MkdirAll(filepath.Dir(real), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(real, []byte(`{"font_size": 12}`), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "config.json")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if err := SetValues(link, map[string]any{"padding": 7}); err != nil {
		t.Fatal(err)
	}
	if st, err := os.Lstat(link); err != nil || st.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("the config file is no longer a symlink (%v)", err)
	}
	c, err := Load(link)
	if err != nil {
		t.Fatal(err)
	}
	if c.Padding != 7 || c.FontSize != 12 {
		t.Fatalf("padding %d, font size %v: want 7 and 12", c.Padding, c.FontSize)
	}
}

// WriteSample creates the folder of the path it is given (not necessarily
// the default one) and never overwrites a file.
func TestWriteSample(t *testing.T) {
	path := filepath.Join(t.TempDir(), "elsewhere", "termite.json")
	if err := WriteSample(path); err != nil {
		t.Fatal(err)
	}
	if c, err := Load(path); err != nil || c.FontSize != Default().FontSize {
		t.Fatalf("the sample does not load as the defaults: %v", err)
	}
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteSample(path); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("writing over a file: %v, want fs.ErrExist", err)
	}
	if data, _ := os.ReadFile(path); string(data) != "{}" {
		t.Fatalf("the file was overwritten: %q", data)
	}
}
