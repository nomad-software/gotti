#!/bin/bash
# Draws docs/screenshot.png: Termite in a virtual X server (Xvfb) with a demo
# layout of split panes running real programs, captured with ImageMagick.
# Needs Xvfb, ImageMagick (import) and vim. Nothing is typed into the
# window, so the result depends only on this script and the code.
set -eu
cd "$(dirname "$0")/.."
make build >/dev/null

tmp=$(mktemp -d)
trap 'kill $termite $xvfb 2>/dev/null; rm -rf "$tmp"' EXIT
mkdir -p "$tmp/config/termite/layouts"
width=1180 height=700
cat > "$tmp/config/termite/config.json" <<JSON
{"font_size": 11, "padding": 6, "window_width": $width, "window_height": $height}
JSON
cat > "$tmp/config/termite/layouts/demo.json" <<JSON
{"tabs": [
  {"split": "columns", "sizes": [52, 48], "children": [
    {"cwd": "$PWD", "command": ["vim", "-u", "NONE", "-N", "-c", "syntax on", "-c", "set number", "internal/render/atlas.go"]},
    {"split": "rows", "sizes": [58, 42], "children": [
      {"cwd": "$PWD", "command": ["bash", "packaging/screenshot-demo.sh", "show"]},
      {"cwd": "$PWD", "command": ["bash", "packaging/screenshot-demo.sh", "ls"]}
    ]}
  ]},
  {"cwd": "$PWD", "command": ["bash", "packaging/screenshot-demo.sh", "ls"]}
]}
JSON

display=:97
Xvfb $display -screen 0 $((width + 40))x$((height + 40))x24 >/dev/null 2>&1 &
xvfb=$!
sleep 1
DISPLAY=$display QT_QPA_PLATFORM=xcb XDG_CONFIG_HOME="$tmp/config" XDG_DATA_HOME="$tmp/data" \
	TERMITE_NO_DESKTOP_ENTRY=1 ./termite --layout demo >"$tmp/log" 2>&1 &
termite=$!
sleep 3
if ! kill -0 $termite 2>/dev/null; then
	echo "termite exited:" >&2
	cat "$tmp/log" >&2
	exit 1
fi
mkdir -p docs
DISPLAY=$display import -window root -crop ${width}x${height}+0+0 +repage docs/screenshot.png
echo "wrote docs/screenshot.png"
