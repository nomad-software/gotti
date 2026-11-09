# Termite — notes for Claude

GPU-rendered terminal emulator: Go + Qt 6 via miqt + OpenGL 3.3 core. See
README.md for features, config and architecture.

## Build and check
- `make build` (./termite, test/termite-headless), `make test` (race), `make vet`,
  `make fuzz`, `make bench` (parser + reflow), `make icons` (SVG → embedded PNG),
  `make screenshot` (docs/screenshot.png via Xvfb; needs Xvfb, ImageMagick, vim).
- First build compiles miqt (minutes). Always run `gofmt -l .`, `go vet ./...`
  and `staticcheck ./...` (in `$GOPATH/bin`).
- Run `make build` after every change so `./termite` is current.
- GUI checks: run under `Xvfb :99` with `QT_QPA_PLATFORM=xcb`, set
  `XDG_CONFIG_HOME`/`XDG_DATA_HOME` to a scratch dir (start-up writes a desktop
  entry otherwise), screenshot with `import -window root`. Start Xvfb with `&`
  and kill it by PID: `pkill -f "Xvfb :99"` also matches (and kills) the
  shell running that command.
- `test/termite-headless -size 80x24 -script '...' -- prog` prints the screen of a
  program run through the emulator core (vim, vttest, …) without a GUI.

## Layout
- `internal/term` emulator core, no GUI deps: 16-byte packed `Cell`, flat ring
  `Buffer` (scrollback), VT500 parser with fast paths (ASCII runs, CSI params,
  OSC/DCS payloads), two-pass reflow, OSC/DCS.
- `internal/render` atlas, procedural box drawing, instanced GL renderer (two
  draw calls, per-row instance slots re-uploaded only when damaged).
- `internal/qtgl` C++ QOpenGLWidget shim (miqt has no Qt 6 OpenGLWidgets).
- `internal/ui` windows/tabs/splits/panes, menu, options dialog (General,
  User interface, Colours, Keyboard Shortcuts), pane headers, layouts.
  `qtutil.go` has helpers for throwaway Qt values (see the miqt rule).
- `internal/config` JSON config, live reload (inotify on the file's folder
  and a symlink target's; polls only if inotify can't start), `SetValues`
  writes, themes (`themes` in the file replaces the built-ins), key chords.
  `App.reload` ignores a config equal to the current one, so termite's own
  writes coming back from the watcher cost nothing.

## Rules learnt the hard way
- All Qt/GL calls on the main thread; PTY reader goroutines only lock the
  terminal and call `mainthread.Start`.
- miqt: never `Delete()` objects Qt returns by value (e.g. `CurrentFont()`,
  `QColorDialog_GetColor`): miqt's finalizers free them; double frees crash
  later at GC time (reproduce with `GOGC=5`). Delete only `qt.NewXxx` objects,
  and always delete those that aren't parented widgets (QVariant, QCursor,
  QUrl, QPoint, QKeySequence…): miqt never frees them, so they leak. Use the
  `qtutil.go` helpers.
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
  dialogs keep the native style.
- Idle must stay at ~0% CPU: no polling timers; refresh work happens after
  output.
- The user handles git; don't commit or suggest commits.
