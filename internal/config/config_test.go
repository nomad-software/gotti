package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadAndWatch(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	c, err := Load(p)
	if err != nil || c.Theme != "solarized-dark" {
		t.Fatal(c, err)
	}
	os.WriteFile(p, []byte(`{"theme": "ubuntu-ambiance", "font_size": 14, "keybindings": {"copy": "ctrl+alt+c"}, "colors": {"background": "#112233"}}`), 0o644)
	c, err = Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.Theme != "ubuntu-ambiance" || c.FontSize != 14 || c.Keybindings["copy"] != "ctrl+alt+c" || c.Keybindings["paste"] != "ctrl+shift+v" {
		t.Fatalf("%+v", c)
	}
	if th := c.ResolveTheme(); th.Background != 0x112233 || th.Foreground != 0xffffff {
		t.Fatalf("%x", th.Background)
	}
	b := c.ParseBindings()
	if b[Chord{Ctrl: true, Alt: true, Key: "c"}] != "copy" || b[Chord{Ctrl: true, Key: "equal"}] != "font_bigger" {
		t.Fatal("bindings", b)
	}
	got := make(chan *Config, 1)
	stop := Watch(p, 20*time.Millisecond, func(c *Config, err error) { got <- c })
	defer stop()
	time.Sleep(50 * time.Millisecond)
	os.WriteFile(p, []byte(`{"theme": "solarized-light"}`), 0o644)
	os.Chtimes(p, time.Now().Add(time.Second), time.Now().Add(time.Second))
	select {
	case c := <-got:
		if c.Theme != "solarized-light" {
			t.Fatal(c.Theme)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no reload")
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
