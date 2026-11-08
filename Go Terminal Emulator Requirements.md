# Go Terminal Emulator Requirements

Nov 8, 2026 · @Gary Willoughby

## Scope and non-goals

Target modern xterm-compatible behaviour (`TERM=xterm-256color`) on Linux. Anything a current shell, vim, tmux or htop doesn't use is out.

**In scope**

- UTF-8 only, with wide characters and emoji
- Truecolor and 256 colours
- Tabs, split panes and saved layouts

**Dropped (non-goals)**

- VT52 mode and 8-bit C1 control codes
- Legacy charsets other than DEC line drawing (G0–G3 switching, NRCS)
- Double-width and double-height lines (`DECDWL`, `DECDHL`)
- 132-column mode (`DECCOLM`)
- Sixel and ReGIS graphics, and Tektronix mode
- Printer passthrough, answerback and blink attributes
- Non-UTF-8 encodings
- macOS and Windows support (v1)

## Stack and architecture

Go for everything, with Qt (via miqt) for windows and OpenGL for the grid.

- **Language:** Go, with CGO for miqt
- **UI:** miqt (Qt 6): `QTabWidget`, `QSplitter`, `QOpenGLWidget`
- **GPU:** OpenGL 3.3 core via `go-gl`
- **PTY:** `creack/pty`
- **Fonts:** shaping and fallback via Qt (`QRawFont`) or HarfBuzz
- **Threads:** one reader and parser goroutine per pane; all Qt and GL calls on the main thread

&#91;embedded content: per-pane data flow · output and input paths\]

The parser locks the screen state, then asks Qt for a redraw; the renderer reads a snapshot at the next vsync.

## Terminal emulation

Implement the xterm subset that real programs use; nothing else.

- [ ] Control characters: CR, LF, BS, HT, BEL
- [ ] Cursor movement and positioning (CUU/CUD/CUF/CUB, CUP, save/restore)
- [ ] Erase in display and line (ED, EL), insert/delete lines and chars
- [ ] Scroll regions (DECSTBM)
- [ ] SGR: bold, dim, italic, underline (incl. curly), inverse, strikethrough
- [ ] Colours: 16, 256 and truecolor
- [ ] Alternate screen (`?1049`), cursor visibility (`?25`), cursor shape (DECSCUSR)
- [ ] Application cursor keys (`?1`) and bracketed paste (`?2004`)
- [ ] Mouse reporting: SGR mode only (`?1000`, `?1002`, `?1006`)
- [ ] Focus events (`?1004`) and synchronized output (`?2026`)
- [ ] OSC 0/2 title, OSC 8 hyperlinks, OSC 52 clipboard, OSC 7 working directory
- [ ] DEC line-drawing charset
- [ ] Scrollback with reflow on resize
- [ ] Device attribute replies (DA1, DA2) and cursor position report

## Input

Encode keys and mouse the way xterm does, plus the modern kitty keyboard protocol.

- [ ] xterm key encoding, with modifiers (Ctrl, Alt, Shift)
- [ ] Kitty keyboard protocol (progressive enhancement)
- [ ] Input methods (IME) for CJK text via Qt
- [ ] Bracketed paste, with a warning on multi-line paste
- [ ] Mouse selection, copy on select, middle-click paste
- [ ] Configurable keybindings

## Rendering

Draw the grid on the GPU with OpenGL 3.3 core inside a `QOpenGLWidget`.

- [ ] Glyph atlas: rasterize each glyph once, draw cells as instanced quads
- [ ] One or two draw calls per frame (backgrounds, then glyphs)
- [ ] Damage tracking: re-upload only changed rows
- [ ] Render at most once per vsync, never per byte received
- [ ] Font fallback for missing glyphs and emoji (colour emoji)
- [ ] Ligatures optional, off by default
- [ ] Box-drawing and block characters drawn procedurally, so they join cleanly
- [ ] HiDPI and per-monitor scaling
- [ ] Cursor styles: block, beam, underline, with optional blink

## UI features

Terminator-style layouts built from Qt widgets via miqt.

- [ ] Split panes horizontally and vertically (`QSplitter`), resizable
- [ ] Tabs (`QTabWidget`), reorderable
- [ ] Keyboard navigation between panes
- [ ] Broadcast input to a group of panes
- [ ] Save and restore named layouts
- [ ] Search in scrollback
- [ ] Clickable URLs and OSC 8 links
- [ ] Config file (TOML) with live reload
- [ ] Colour themes: Solarized Light, Solarized Dark and Ubuntu Ambiance

## Optimisations

Keep the parser and memory layout allocation-free; rendering optimisations are listed under Rendering.

- [ ] Read the PTY in large buffered chunks (64 KB or more per syscall)
- [ ] ASCII fast path: write runs of printable characters in bulk, skipping the state machine
- [ ] Flat, preallocated `[]Cell` grid instead of per-row slices or pointers
- [ ] Ring buffer for scrollback, so scrolling never copies lines
- [ ] Packed cell attributes (colours and flags in integers), no strings or pointers per cell
- [ ] Reuse buffers with `sync.Pool` where allocations can't be avoided
- [ ] Profile with `pprof` before and after each optimisation

## Performance targets and testing

Proposed targets, to be tuned once a prototype exists.

| Metric | Target |
| --- | --- |
| Keypress-to-screen latency | under 10 ms |
| `cat` of a 100 MB file | under 2 s |
| Idle CPU | \~0% (no redraw without changes) |
| Memory per pane, 10k lines scrollback | under 20 MB |
| Startup time | under 200 ms |

**Testing**

- Unit tests for the parser, fed recorded byte streams
- Golden-screen tests: bytes in, expected grid out
- vttest for the sequences in scope
- vtebench for throughput
- Manual checks with vim, tmux, htop, less and fzf
- Fuzz the parser with Go's built-in fuzzing
