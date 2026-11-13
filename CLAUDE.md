# Termite — notes for Claude

GPU-rendered terminal emulator for Linux only: Go + Qt 6.12+ via qute
(github.com/nomad-software/qute, the user's own project, checked out beside
this one in ../qute) + OpenGL 3.3 core. See README.md for features, config
and architecture.

## Build and check
- `make build` (./termite, test/termite-headless), `make test` (race), `make vet`,
  `make fuzz`, `make bench` (parser + reflow), `make guibench` (GUI throughput,
  frame costs and a CPU profile under Xvfb; pass two builds to
  `test/guibench/run.sh` to compare), `make icons` (SVG → embedded PNG),
  `make screenshot` (docs/screenshot.png via Xvfb; needs Xvfb, ImageMagick, vim).
- First build compiles qute's shims (minutes). Always run `gofmt -l .`,
  `go vet ./...`, `staticcheck ./...` (in `$GOPATH/bin`) and
  `go run github.com/nomad-software/qute/cmd/qute@<version in go.mod> vet ./...`.
- GUI tests (`internal/ui/gui_test.go`: splits and focus, window title, toolbar,
  options dialog, closing panes and windows, shells that fail to start)
  need an X display, as offscreen gives no GL
  context: `DISPLAY=:99 go test ./internal/ui` under Xvfb (they skip without
  `DISPLAY`). Also run them with `GOGC=5` after ownership changes. Tests
  leave their windows open: closing the last window quits the app.
- Run `make build` after every change so `./termite` is current.
- GUI checks: run under `Xvfb :99` with `QT_QPA_PLATFORM=xcb`, set
  `XDG_CONFIG_HOME`/`XDG_DATA_HOME` to a scratch dir (start-up writes a desktop
  entry otherwise), screenshot with `import -window root`. Start Xvfb with `&`
  and kill it by PID: `pkill -f "Xvfb :99"` also matches (and kills) the
  shell running that command.
- Measure before and after any performance change (`make bench`,
  `make guibench`, `TERMITE_DEBUG=frames`, `--cpuprofile`), and report the
  numbers whichever way they go.
- `make appimage` runs `packaging/appimage.sh`, which works in a temporary
  folder outside the project (removed on exit) and leaves only
  `Termite-<version>-x86_64.AppImage` in the project folder. Never create a
  `build/` folder in the project. Releases come from
  `.github/workflows/appimage.yml`: `v` tags or by hand,
  Ubuntu 22.04 container (glibc 2.35) with aqtinstall's Qt 6.12.0, tests
  first (under `xvfb-run`, so the GUI tests run), then the script. The
  script refuses to make an AppImage missing the xcb/Wayland/GL plugins or
  the Qt OpenGL and SVG modules, or containing libGL/libEGL
  (the system's must be used), and starts the result under `xvfb-run` with
  Mesa's software GL (offscreen can't give termite a GL context).
- `test/termite-headless -size 80x24 -script '...' -- prog` prints the screen of a
  program run through the emulator core (vim, vttest, …) without a GUI.

## Layout
- `internal/term` emulator core, no GUI deps: 16-byte packed `Cell`, flat ring
  `Buffer` (scrollback), VT500 parser with fast paths (ASCII runs, CSI params,
  OSC/DCS payloads), two-pass reflow, OSC/DCS handling. `Event`s exist only
  for what the UI acts on; the renderer notices screen, colour and
  alternate-screen changes itself.
- `internal/render` atlas, procedural box drawing, instanced GL renderer (two
  draw calls; per-row instance slots rebuilt only when damaged, and the used
  instances packed each frame so empty slots are never drawn).
- `internal/qtgl` the terminal's GL widget: a Go subclass of qute's
  QOpenGLWidget (GL callbacks, input events, IME queries); go-gl gets its GL
  functions through `QOpenGLContext.GetProcAddress`.
- `internal/ui` windows/tabs/splits/panes, menu, toolbar, options dialog
  (General, User interface, Colours, Keyboard Shortcuts), pane headers,
  layouts.
  `qtutil.go` has helpers for throwaway Qt values. Overrides of Qt virtuals
  are Go subclasses (`headerLabel`, `searchBar`, `windowWidget`).
- Tabs (`tabbar.go`): a QTabWidget subclass so it can take a QTabBar
  subclass that asks for the window's whole width and gives every tab the
  same base width; with expanding on (after document mode, which turns it
  off) the tabs share the bar equally. They are flat, styled by object name
  from the window theme's palette (`palette(...)` in `applyTheme`), and Qt
  elides their titles from the left. Their close buttons are termite's own
  (`tabCloseButton`, the close icon, `tabCloseMargin` from the edge): Qt's
  sits at the tab's edge whatever the stylesheet says, and styling it drops
  its icon.
- Toolbar (`toolbar.go`, hidden by `"toolbar": false`): a
  QToolBar of 32px icons with text under them, a line along its bottom
  painted in the text colour (`toolbarWidget`; styles draw none outside a
  main window), buttons at least
  `toolButtonWidth` (80) wide, kept in the desktop's style.
  Its buttons run actions on the window's current pane, never take the
  focus and show no tooltips (`tipFilter` takes the event); Exit (`exit` action) types `exit` + Enter into that pane alone, not broadcast. Icons are `internal/icons/svg/*.svg`, all in one outline style
  (24 grid, `fill="none"`, `stroke="currentColor"` 1.5 wide, round caps and
  joins, set on the root only; a test checks), drawn by `ui/icons.go`'s
  icon engine in the palette's text colour each time, so they follow
  `window_theme`. Draw new icons in the same style. Broadcast state
  changes go through `App.setBroadcast` (or `refreshBroadcast`), which
  updates the toolbars and the pane headers: a pane typing reaches has
  the focused header colours (`updateHeaderActive`; there is no outline).
- `internal/config` JSON config, `SetValues` writes, themes (`themes` in the
  file replaces the built-ins), key chords. The file is read at start-up
  only (and on `reload_config` or Options → OK/Apply); it is deliberately
  not watched.

## Rules learnt the hard way
- All Qt/GL calls on the main thread; PTY reader goroutines only lock the
  terminal and call `qute.Post`.
- qute: Owned values (QVariant, QCursor, QUrl, QKeySequence, QFont…) are
  freed by GC cleanup, so throwaway values need no `Delete()`. Inside
  handlers delete QObjects with `DeleteLater`, never `Delete`. Pass widgets
  themselves where a `…Like` is wanted, never an embedded struct field
  (`label.QWidget` copies the handle). Event arguments are dead once the
  handler returns. `QApplication` is made before `qute.Main` (in
  `ui.NewApplication`), as GL attributes must be set before it exists.
- The window title is left without an application display name: Qt would
  append it to every title. Splitting sets `tab.focused` to the new pane,
  and the context menu gives focus back to `tab.focused`, not to the pane
  it was opened on.
- Hot loops writing cells: store the pen whole (`line[x] = pen`), then patch
  `Ch`. Building a `Cell` field by field and then copying it goes through
  the stack and stalls store forwarding, which halved parser throughput.
  Check `go tool objdump` when a cell loop looks slow.
- Reflow changes need the fuzzer (`make fuzz`) and tests with small
  scrollback, where the ring overflows.
- Character widths follow glibc `wcwidth`; don't join ZWJ/skin-tone emoji.
- `AttrWrapPad` marks the blank left when a wide char wraps; reflow, copy and
  search must skip it.
- Stylesheets target termite's own widgets by object name only, so menus and
  dialogs keep the style `window_theme` gives them (the desktop's, or
  Fusion with termite's light/dark palette).
- Idle must stay at ~0% CPU: no polling timers; refresh work happens after
  output.
- Redraws after output are paced to the refresh rate (`Pane.requestRedraw`).
  Xvfb has no vsync, so without pacing cheap frames run unthrottled and
  llvmpipe's threads steal CPU from the parser (cat got 30% slower).
- Shells must not inherit the AppImage runtime's `APPIMAGE`, `APPDIR`,
  `ARGV0`, `OWD` (stripped in `pty.Start`), and `packaging/AppRun` must not
  add anything to the environment: everything in it reaches the shells.
- `Session.Write` never blocks: the UI thread and the reader call it, and a
  child that stops reading (raw mode) once filled a bounded channel and
  froze every window. Input waits in a queue capped at `maxQueued`; beyond
  it, it is dropped (`TestWriteNeverBlocks`).
- Mouse reports follow the program's choice: SGR with ?1006, else X10
  (`EncodeMouse(…, sgr)`, through `Pane.mouseReport`).
- Ligatures are drawn only over cells with no cursor and no decorations
  (`plainRun`): the ligature path skips the per-cell drawing.
- Config: `Load` keeps what is valid when a value has the wrong type (only
  a syntax error falls back to the defaults); `normalize` clamps sizes to
  the ranges the Options dialog uses (`config.Min…/Max…`); `Validate`
  reports unknown key names and actions sharing a chord. Start-up problems
  reach the pane headers through `NewApp`'s `loadErr`.
- Raw syscalls on the PTY (poll, ioctl) go through `Session.rc.Control`,
  never a saved descriptor number: Close may run concurrently and the
  number could be reused.
- `ClearScrollback` releases the old lines with `madvise(MADV_DONTNEED)`
  (`release_linux.go`); that's safe because cells hold no pointers and zero
  is a blank cell.
- Bracketed paste must remove paste markers until none is left: a single
  pass lets "ESC[20" + marker + "1~" reassemble ESC[201~ and end the paste
  early (`input.Paste`, tested).
- Ownership of closed things: a closed window is deleted (`DeleteLater` in
  its `CloseEvent`), and a pane or tab whose shell failed to start deletes
  its widgets; a window whose first tab failed closes again. Posted
  callbacks check `p.exited`, which closing sets, before touching widgets.
- Clear Scrollback goes through `Terminal.ClearScrollback`, which clears
  the primary screen's scrollback: `Buf()` is the alternate screen (with
  none) while a full-screen program runs.
- Config writes: `config.SetValues` follows a symlinked config file, and
  `config.WriteSample` (`--write-config`, Edit Config) creates the folder
  of the path it is given, never `config.Dir()` regardless.
- `DISPLAY` is usually set in the user's shell, so a plain `go test ./...`
  runs the GUI tests on the real display; use `DISPLAY=:99` for Xvfb.
- The user handles git; don't commit or suggest commits.
