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
	if err != nil || c.Theme != DefaultTheme {
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
	if c, err := Load(p); err != nil || c.Theme != "ubuntu-ambiance" || c.FontSize != Default().FontSize {
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
	if len(c.Themes) != len(BuiltinThemes) || c.ResolveTheme().Background != 0x300a24 {
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
	if c.ResolveTheme().Background != 0x300a24 {
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

// TestRemovedAction checks that a file saved before an action was removed,
// which the Options dialog fills with every action, still loads cleanly.
func TestRemovedAction(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(p, []byte(`{"keybindings": {"paste_selection": "shift+insert", "copy": "ctrl+alt+c"}}`), 0o644)
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Keybindings["paste_selection"]; ok || c.Keybindings["copy"] != "ctrl+alt+c" {
		t.Fatalf("%v", c.Keybindings)
	}
}

// TestHeaderFont checks that the pane headers follow the terminal's font,
// a little smaller, until a font or size of their own is set.
func TestHeaderFont(t *testing.T) {
	c := Default()
	c.FontFamily, c.FontSize = "Mono", 12
	c.HeaderFontFamily, c.HeaderFontSize = "", 0
	if f, s := c.HeaderFont(); f != "Mono" || s != 10.2 {
		t.Errorf("default: %q %v", f, s)
	}
	c.HeaderFontFamily, c.HeaderFontSize = "Sans", 9
	if f, s := c.HeaderFont(); f != "Sans" || s != 9 {
		t.Errorf("set: %q %v", f, s)
	}
	p := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(p, []byte(`{"header_font_size": -3}`), 0o644)
	if c, err := Load(p); err != nil || c.HeaderFontSize != 0 {
		t.Errorf("a negative size: %v %v", c.HeaderFontSize, err)
	}
}

// TestLoadKeepsGoodValues checks that a value of the wrong type is
// reported without throwing the rest of the file away.
func TestLoadKeepsGoodValues(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(p, []byte(`{"font_size": "big", "theme": "solarized-light", "padding": 7}`), 0o644)
	c, err := Load(p)
	if err == nil || !strings.Contains(err.Error(), "font_size") {
		t.Errorf("no error for font_size: %v", err)
	}
	if c.Theme != "solarized-light" || c.Padding != 7 || c.FontSize != Default().FontSize {
		t.Errorf("theme %q, padding %d, font size %v", c.Theme, c.Padding, c.FontSize)
	}
	// A file that isn't JSON still falls back to the defaults.
	os.WriteFile(p, []byte(`{"theme": `), 0o644)
	if c, err := Load(p); err == nil || c.Theme != DefaultTheme {
		t.Errorf("broken file: theme %q, %v", c.Theme, err)
	}
}

// TestLoadNullKeybindings checks that "keybindings": null keeps the
// default bindings rather than removing them all.
func TestLoadNullKeybindings(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(p, []byte(`{"keybindings": null}`), 0o644)
	c, err := Load(p)
	if err != nil || c.Keybindings["copy"] != DefaultKeybindings["copy"] || len(c.ParseBindings()) == 0 {
		t.Errorf("%v, %v", c.Keybindings, err)
	}
}

// TestKeybindingClash checks that two actions on one chord are reported,
// and that the same one of them gets it every time.
func TestKeybindingClash(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(p, []byte(`{"keybindings": {"copy": "`+DefaultKeybindings["new_tab"]+`"}}`), 0o644)
	c, err := Load(p)
	if err == nil || !strings.Contains(err.Error(), "copy, new_tab") {
		t.Errorf("clash not reported: %v", err)
	}
	ch, _ := ParseChord(DefaultKeybindings["new_tab"])
	for range 50 {
		if got := c.ParseBindings()[ch]; got != "copy" {
			t.Fatalf("the chord went to %q", got)
		}
	}
}

// TestParseChord checks chord names, aliases and the mistakes reported.
func TestParseChord(t *testing.T) {
	for in, want := range map[string]string{
		"ctrl+shift+c":    "ctrl+shift+c",
		"Ctrl+Shift+PgUp": "ctrl+shift+pageup",
		"ctrl+=":          "ctrl+equal",
		"ctrl++":          "ctrl+equal", // + is on the = key
		"ctrl+plus":       "ctrl+equal",
		"alt+esc":         "alt+escape",
		"super+return":    "super+enter",
		"ctrl+comma":      "ctrl+comma",
		"f5":              "f5",
		"menu":            "menu",
		"ctrl+0":          "ctrl+0",
	} {
		c, err := ParseChord(in)
		if err != nil || c.String() != want {
			t.Errorf("%q: %v %v, want %s", in, c, err, want)
		}
	}
	for _, bad := range []string{"ctrl+foo", "ctrl+page_up", "hyper+c", "ctrl+", "ctrl+!"} {
		if c, err := ParseChord(bad); err == nil {
			t.Errorf("%q parsed as %v", bad, c)
		}
	}
}

// TestValidateEnums checks that each setting with a fixed set of values
// reports one outside it.
func TestValidateEnums(t *testing.T) {
	for _, set := range []func(*Config){
		func(c *Config) { c.CursorStyle = "bar" },
		func(c *Config) { c.WindowTheme = "blue" },
		func(c *Config) { c.PasteWarning = "sometimes" },
		func(c *Config) { c.OSC52 = "all" },
		func(c *Config) { c.Theme = "nope" },
		func(c *Config) { c.Keybindings["nope"] = "ctrl+x" },
		func(c *Config) { c.Keybindings["copy"] = "ctrl+foo" },
	} {
		c := Default()
		set(c)
		if c.Validate() == nil {
			t.Errorf("not reported: %+v", c)
		}
	}
	if err := Default().Validate(); err != nil {
		t.Errorf("the defaults: %v", err)
	}
}

// TestLoadClamps checks that sizes beyond what the Options dialog offers
// are brought within its ranges.
func TestLoadClamps(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(p, []byte(`{"font_size": 5000, "line_height": 0.01, "padding": 100000, "header_font_size": 1, "scrollback": -5}`), 0o644)
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.FontSize != MaxFontSize || c.LineHeight != MinLineHeight || c.Padding != MaxPadding ||
		c.HeaderFontSize != MinFontSize || c.Scrollback != 0 {
		t.Errorf("%v %v %v %v %v", c.FontSize, c.LineHeight, c.Padding, c.HeaderFontSize, c.Scrollback)
	}
}

// TestBlackSelectionText checks that black selected text is kept, not
// taken for "keep the text's colours".
func TestBlackSelectionText(t *testing.T) {
	spec := BuiltinThemes[DefaultTheme]
	spec.SelectionText = "#000000"
	th, err := spec.Resolve()
	if err != nil || !th.HasSelText || th.SelText != 0 {
		t.Errorf("%v %06x %v", th.HasSelText, th.SelText, err)
	}
	spec.SelectionText = ""
	if th, _ := spec.Resolve(); th.HasSelText {
		t.Error("no selection_text still sets the colour")
	}
}

// TestUnnamedThemeError checks that a problem in a theme with no name
// says which theme by its id.
func TestUnnamedThemeError(t *testing.T) {
	c := Default()
	spec := c.Themes[DefaultTheme]
	spec.Name, spec.Foreground = "", "not a colour"
	c.Themes["mine"] = spec
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), `theme "mine"`) {
		t.Errorf("%v", err)
	}
}

// A config file that is just null loads as the defaults, and writing a
// setting to it must not panic on the nil map it decodes to.
func TestSetValuesNullFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte("null\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SetValues(path, map[string]any{"padding": 7}); err != nil {
		t.Fatal(err)
	}
	if c, err := Load(path); err != nil || c.Padding != 7 {
		t.Fatalf("padding %d (%v): want 7", c.Padding, err)
	}
}

// A dangling symlink is followed too: the write creates its target and the
// link stays. The file's permissions are kept.
func TestSetValuesDanglingLinkAndMode(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "dotfiles", "termite.json")
	link := filepath.Join(dir, "config.json")
	if err := os.Symlink(filepath.Join("dotfiles", "termite.json"), link); err != nil { // relative
		t.Fatal(err)
	}
	if err := SetValues(link, map[string]any{"padding": 7}); err != nil {
		t.Fatal(err)
	}
	if st, err := os.Lstat(link); err != nil || st.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("the config file is no longer a symlink (%v)", err)
	}
	if err := os.Chmod(real, 0o600); err != nil {
		t.Fatalf("the link's target was not created: %v", err)
	}
	if err := SetValues(link, map[string]any{"padding": 8}); err != nil {
		t.Fatal(err)
	}
	if st, err := os.Stat(real); err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v (%v): want 0600 kept", st.Mode().Perm(), err)
	}
	if _, err := os.Stat(real + ".tmp"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("temporary file left behind (%v)", err)
	}
	if c, err := Load(link); err != nil || c.Padding != 8 {
		t.Fatalf("padding %d (%v): want 8", c.Padding, err)
	}
}

// A value of the wrong type does not hide the file's other problems.
func TestLoadReportsEveryProblem(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"font_size": "big", "theme": "nope", "cursor_style": "x"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("no error")
	}
	msg := err.Error()
	for _, want := range []string{"font_size", `unknown theme "nope"`, "cursor_style"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q does not mention %s", msg, want)
		}
	}
	if strings.Contains(msg, "\n") {
		t.Errorf("error %q spans lines; pane headers show one", msg)
	}
}

// A font size of 0 means the default size.
func TestZeroFontSize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"font_size": 0}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if c, err := Load(path); err != nil || c.FontSize != Default().FontSize {
		t.Fatalf("font size %v (%v): want %v", c.FontSize, err, Default().FontSize)
	}
}

// A relative XDG_CONFIG_HOME is ignored, as the XDG spec says.
func TestDirIgnoresRelativeXDG(t *testing.T) {
	t.Setenv("HOME", "/home/someone")
	t.Setenv("XDG_CONFIG_HOME", "relative/config")
	if got := Dir(); got != "/home/someone/.config/termite" {
		t.Errorf("Dir() = %q", got)
	}
	t.Setenv("XDG_CONFIG_HOME", "/xdg")
	if got := Dir(); got != "/xdg/termite" {
		t.Errorf("Dir() = %q", got)
	}
}
