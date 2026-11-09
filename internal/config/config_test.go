package config

import (
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

func TestSetValue(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	if err := SetValue(p, "theme", "ubuntu-ambiance"); err != nil {
		t.Fatal(err)
	}
	if c, err := Load(p); err != nil || c.Theme != "ubuntu-ambiance" || c.FontSize != 11 {
		t.Fatalf("new file from defaults: %+v %v", c, err)
	}
	os.WriteFile(p, []byte(`{"font_size": 12, "keybindings": {"copy": "ctrl+c"}}`), 0o644)
	SetValue(p, "ligatures", true)
	SetValue(p, "font_size", 14.0)
	SetValue(p, "theme", "solarized-light")
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
