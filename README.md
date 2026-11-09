# <img src="internal/appicon/appicon.png" alt="" width="40" height="40"> Gotti

A GPU-rendered terminal emulator for Linux, written in Go with Qt 6 (via
[miqt](https://github.com/mappu/miqt)) and OpenGL 3.3 core. It targets
`TERM=xterm-256color` and implements the xterm subset that shells, vim,
tmux, htop, less and fzf actually use, plus Terminator-style tabs, splits
and saved layouts.

![Gotti with two tabs: the first split into vim editing Go source with syntax colours beside two stacked shells, one showing bold, italic, underlined, curly-underlined, struck-through and inverse text, the 16-colour palette, a truecolor gradient, box drawing around wide CJK characters and an emoji, braille and shade blocks and an underlined hyperlink, the other a coloured directory listing; each pane has a header with its directory](docs/screenshot.png)

The screenshot is drawn by `make screenshot` (needs Xvfb, ImageMagick and
vim).

## Build and run

Requirements: Go 1.27+, Qt 6 development files (Widgets, OpenGL,
OpenGLWidgets), OpenGL 3.3 drivers, a C++17 compiler.

```sh
make build                     # or: go build -o bin/gotti ./cmd/gotti
bin/gotti                      # your $SHELL
bin/gotti -- htop              # run a command
bin/gotti --layout work        # open a saved layout
bin/gotti --write-config       # write the default config.json
```

On every start, Gotti writes a desktop entry
(`~/.local/share/applications/gotti.desktop`) and its icon
(`~/.local/share/icons/hicolor/256x256/apps/gotti.png`). These point at
the copy that is running, so it appears in the desktop's menus and can be
pinned to a dock. Set `GOTTI_NO_DESKTOP_ENTRY=1` to skip this.
`packaging/gotti.desktop` is a static entry for packagers. The icon is
drawn from `packaging/gotti.svg` (`make icons`) and embedded in the
binary.

The first build compiles the miqt bindings and takes a few minutes; later
builds are cached.

Profiling: `--cpuprofile FILE`, `--memprofile FILE`, or `--pprof
localhost:6060` for live `net/http/pprof`. `GOTTI_DEBUG=startup,latency`
prints time-to-first-frame and keypress-to-frame latencies.

## Configuration

`~/.config/gotti/config.json` (or `$XDG_CONFIG_HOME/gotti`). Changes are
applied live, with no restart needed. `gotti --write-config` writes every
option with its default. Problems (misspelt keys, bad values, syntax
errors with their line) are shown in red in every pane's header until the
file is fixed; whatever is valid still applies.

Most options can also be set in the **Options** dialog (right-click →
Options…). Its General tab covers font and size (any installed font,
with a warning if it isn't monospaced), line height, padding, ligatures,
bold-as-bright, cursor style and blinking, scrollback, copy on select,
paste warning and OSC 52 clipboard access. Its Themes tab lists the colour
themes: pick the one to use, edit its name, colours and 16-colour palette
with a live preview, add, duplicate or remove themes, and restore the
built-in ones (Solarized Dark, Solarized Light, Ubuntu Ambiance). Its
Keyboard Shortcuts tab rebinds every action:
click a field, press the keys, or clear it to unbind. Shortcuts used by
more than one action are reported and not saved. OK or Apply saves to
`config.json` and applies at once. The
window size and maximized state are saved when a window closes and used
for new windows.

| Key | Default | Notes |
| --- | --- | --- |
| `font_family` | `"monospace"` | fallback and colour emoji come from Qt/fontconfig |
| `font_size` | `11.0` | points |
| `line_height` | `1.0` | multiplier |
| `theme` | `"solarized-dark"` | id of the theme to use, a key of `themes` |
| `themes` | the built-in themes | `{"id": {"name": "…", "foreground": "#…", "background": "#…", "cursor": "#…", "cursor_text": …, "selection": …, "selection_text": …, "palette": [16 colours]}}`; when present it replaces the built-in list, so themes can be removed. `cursor_text`, `selection` and `selection_text` are optional |
| `scrollback` | `10000` | lines |
| `cursor_style` | `"block"` | `block`, `beam`, `underline`; programs may choose another shape (DECSCUSR) until they reset it |
| `cursor_blink` | `false` | allows blinking when an app requests it |
| `copy_on_select` | `true` | primary selection is always set |
| `paste_warning` | `"unbracketed"` | `always`, `unbracketed`, `never` |
| `ligatures` | `false` | needs a font with ligatures |
| `shell` | `$SHELL` | e.g. `["/bin/zsh", "-l"]` |
| `padding` | `2` | logical pixels |
| `osc52` | `"write"` | `off`, `write`, `read-write` |
| `bold_is_bright` | `false` | |
| `word_chars` | `"-_./~:@%+#=?&"` | extra word characters for double-click |
| `window_width`, `window_height`, `window_maximized` | `900`, `560`, `false` | saved automatically |
| `keybindings` | see below | `{"copy": "ctrl+shift+x"}`, or `"none"` to unbind |

### Default keybindings

| Action | Keys | Action | Keys |
| --- | --- | --- | --- |
| copy | Ctrl+Shift+C | paste | Ctrl+Shift+V |
| paste_selection | Shift+Insert | search | Ctrl+Shift+F |
| new_tab | Ctrl+Shift+T | new_window | Ctrl+Shift+N |
| close_pane | Ctrl+Shift+W | split_vertical (Split Vertically: side by side) | Ctrl+Shift+E |
| split_horizontal (Split Horizontally: stacked) | Ctrl+Shift+O | focus_up/down/left/right | Alt+Arrows |
| next_tab / prev_tab | Ctrl+PgDn / Ctrl+PgUp | move_tab_left/right | Ctrl+Shift+PgUp/PgDn |
| scroll_page_up/down | Shift+PgUp/PgDn | scroll_top/bottom | Shift+Home/End |
| font_bigger/smaller/reset | Ctrl+= / Ctrl+- / Ctrl+0 | reload_config | Ctrl+Shift+R |
| broadcast_all (panes in tab) | Ctrl+Shift+A | broadcast_group | Ctrl+Shift+B |
| group_toggle (pane in group) | Ctrl+Shift+G | save_layout / load_layout | Ctrl+Shift+S / Ctrl+Shift+L |
| context_menu | Menu key | options, clear_scrollback, reset_terminal, edit_config | unbound |

Right-click opens the context menu (Shift+right-click while an
application uses the mouse). It covers copy and paste, opening or copying
the link under the pointer, splitting, tabs, windows,
broadcast, layouts, find, clear scrollback, reset, zoom, and Options….
The menu and dialogs use the desktop's native Qt style; the colour theme
applies only to the terminal, tabs and splitters.

Moving focus with Alt+Arrows goes to the adjacent pane on that side that
lines up with the text cursor, as in tmux and vim, never to a pane that
only touches at a corner.

Every pane has a header showing its current directory, from the shell's
OSC 7 report or the foreground process. Each tab is named after its
focused pane's header; the window title is just the app name. Titles set
by programs are ignored.

Mouse: drag to select (Alt for block selection), double/triple-click for
word/line, middle-click pastes the primary selection, Ctrl+click opens URLs
and OSC 8 links (Ctrl+hover underlines them). Shift bypasses application
mouse reporting. In search, Enter goes to older matches, Shift+Enter to
newer ones, and Esc closes the search bar.

Layouts are saved as JSON under `~/.config/gotti/layouts/`, recording the
split tree, sizes, each pane's working directory and command.

## Architecture

```
cmd/gotti            Qt application entry point, flags, profiling
cmd/gotti-headless   runs a program through the emulator core and prints the screen (testing)
internal/term        emulator core: cells, scrollback ring, parser, xterm emulation (no GUI deps)
internal/pty         child process on a PTY; one reader+parser goroutine per pane
internal/input       key (xterm + kitty), mouse (SGR) and paste encoding
internal/render      glyph atlas, procedural box drawing, OpenGL renderer
internal/qtgl        C++ QOpenGLWidget shim (miqt has no Qt 6 OpenGLWidgets binding)
internal/ui          windows, tabs, splits, panes, layouts, actions
internal/config      JSON config, live reload, in-place setting writes, themes, key chords
internal/desktop     desktop entry and icon install (as in ApeMail)
internal/appicon     embedded program icon
```

Data flow per pane: the reader goroutine reads the PTY, locks the
terminal, parses, unlocks and requests a redraw. Requests are coalesced,
and Qt paints at most once per vsync. On paint, the main thread locks
briefly to copy only the damaged rows, then builds instances and draws
without the lock. All Qt and GL calls happen on the main thread.

Key implementation points:
- **Cells** are 16 bytes with no pointers: rune, packed fg/bg (`kind<<24 | index-or-RGB`), 16-bit flags, and a 16-bit index into an interned table for rare data (underline colour, OSC 8 link).
- **Grid and scrollback** form one flat `[]Cell`, allocated up front and used as a ring. Scrolling into scrollback moves an index; nothing is copied. Untouched pages never become resident.
- **Parser**: a VT500-style state machine with an ASCII fast path that writes runs of printable bytes in bulk. Steady-state allocations are zero.
- **Rendering**: two instanced draw calls, backgrounds then glyphs. Each row owns a fixed slot in the instance buffers, so only damaged rows are re-uploaded.
- **Glyphs** are rasterized once with QPainter, which gives Qt's font fallback, colour emoji and shaping for ligatures. They live in a shared RGBA atlas, since all pane contexts share GL resources.
- **Box drawing**, block elements, braille, Powerline separators and DEC scan lines are drawn procedurally, so they join cleanly between cells.
- **Decorations** (all underline styles, strike, overline, cursor shapes) are atlas sprites, so they need no extra draw calls.

## Testing

```sh
make test                                             # all tests, with the race detector
make vet
go test ./...                                         # unit + golden-screen tests
go test ./internal/term -fuzz FuzzParser              # parser fuzzing
go test ./internal/term -bench . -run X               # throughput
STRESS=1 go test ./internal/term -run StressRandom    # random-sequence hang check
bin/gotti-headless -size 80x24 -script '1{enter}{wait 500}' -- vttest
```

## Measured results

Measured under Xvfb with Mesa llvmpipe (software GL) on 8 cores, so a
real GPU should do as well or better:

| Metric | Target | Measured |
| --- | --- | --- |
| Keypress-to-screen latency | < 10 ms | 5–9 ms (includes PTY echo round trip and `glFinish`) |
| `cat` of a 100 MB file | < 2 s | 1.58 s in the GUI; parser alone 150 MB/s |
| Idle CPU | ~0% | 0 ticks over 5 s |
| Memory per pane, 10k scrollback | < 20 MB | 15.4 MB of emulator heap (120 cols); see note |
| Startup time | < 200 ms | 153 ms to first frame |

Memory note: every pane is a `QOpenGLWidget` with its own GL context. Under
llvmpipe each context adds ~25 MB of driver RSS on top of the emulator's
15 MB. With hardware drivers that overhead is much smaller, but I haven't
measured it.

## Deviations and known gaps

- **QOpenGLWidget**: miqt does not bind Qt 6's OpenGLWidgets module, so
  `internal/qtgl` adds a small C++ subclass that forwards GL callbacks,
  input events and IME queries to Go.
- **PTY reads**: Linux returns at most about 4 KB per `read` on a PTY. The
  reader drains what is already buffered (`poll` with zero timeout) and
  parses in chunks of up to 128 KB, so the parser sees large chunks even
  though individual syscalls are kernel-limited.
- **Character widths follow glibc `wcwidth`**, so shells and editors stay
  aligned. Combining marks and variation selectors attach to the previous
  cell, and regional-indicator pairs render as one flag. ZWJ emoji
  sequences and skin-tone modifiers are shown as separate cells, as in
  xterm.
- **Mouse reporting is SGR-only** (as specified). Legacy X10 encoding is
  never sent.
- **Not verified here**: IME composition (no input method available under
  Xvfb), tmux and htop (not installed), native Wayland (tested on X11/xcb
  only), real GPU drivers, vtebench (not installed).
- Dropped per the spec: VT52, C1 controls, NRCS and legacy charsets other
  than DEC line drawing, DECDWL/DECDHL, DECCOLM, sixel/ReGIS/Tektronix,
  printer, answerback, blink attribute, non-UTF-8 encodings.
