# Gotti — notes for Claude

GPU-rendered terminal emulator: Go + Qt 6 via miqt + OpenGL 3.3 core. See
README.md for features, config and architecture.

## Build and check
- `make build` (bin/gotti, bin/gotti-headless), `make test` (race), `make vet`,
  `make fuzz`, `make bench`, `make icons` (SVG → embedded PNG),
  `make screenshot` (docs/screenshot.png via Xvfb; needs Xvfb, ImageMagick, vim).
- First build compiles miqt (minutes). Always run `gofmt -l .` and `go vet ./...`.
- GUI checks: run under `Xvfb :99` with `QT_QPA_PLATFORM=xcb`, set
  `XDG_CONFIG_HOME`/`XDG_DATA_HOME` to a scratch dir (start-up writes a desktop
  entry otherwise), screenshot with `import -window root`.
- `bin/gotti-headless -size 80x24 -script '...' -- prog` prints the screen of a
  program run through the emulator core (vim, vttest, …) without a GUI.

## Layout
- `internal/term` emulator core, no GUI deps: 16-byte packed `Cell`, flat ring
  `Buffer` (scrollback), VT500 parser with ASCII fast path, reflow, OSC/DCS.
- `internal/render` atlas, procedural box drawing, instanced GL renderer (two
  draw calls, per-row instance slots re-uploaded only when damaged).
- `internal/qtgl` C++ QOpenGLWidget shim (miqt has no Qt 6 OpenGLWidgets).
- `internal/ui` windows/tabs/splits/panes, menu, options dialog (General,
  Themes, Keyboard Shortcuts), pane headers, layouts.
- `internal/config` JSON config, live reload (1 s poll), `SetValues` writes,
  themes (`themes` in the file replaces the built-ins), key chords.

## Rules learnt the hard way
- All Qt/GL calls on the main thread; PTY reader goroutines only lock the
  terminal and call `mainthread.Start`.
- miqt: never `Delete()` objects Qt returns by value (e.g. `CurrentFont()`,
  `QColorDialog_GetColor`): miqt's finalizers free them; double frees crash
  later at GC time (reproduce with `GOGC=5`). Delete only `qt.NewXxx` objects.
- Character widths follow glibc `wcwidth`; don't join ZWJ/skin-tone emoji.
- `AttrWrapPad` marks the blank left when a wide char wraps; reflow, copy and
  search must skip it.
- Stylesheets target gotti's own widgets by object name only, so menus and
  dialogs keep the native style.
- Idle must stay at ~0% CPU: no polling timers; refresh work happens after
  output.
- The user handles git; don't commit or suggest commits.
