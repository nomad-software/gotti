#!/bin/bash
# GUI benchmark: runs termite through fixed workloads under a virtual X
# server and prints throughput, frame costs and a CPU profile.
#
#   test/guibench/run.sh [BINARY] [OUT]     (make guibench)
#
# BINARY defaults to ./termite; results (timings, frame stats, cat.pprof)
# go to OUT, a new temporary folder by default. Compare two builds by
# running it for each. Needs Xvfb and python3. The 100 MB test file is made
# once and kept in $TMPDIR.
#
# Throughput runs have no instrumentation. Frame stats come from separate
# runs with TERMITE_DEBUG=frames, which ends every paint with glFinish so
# the GPU's (or llvmpipe's) work is counted.
set -e
HERE=$(cd "$(dirname "$0")" && pwd)
BIN=$(realpath "${1:-./termite}")
OUT=${2:-$(mktemp -d "${TMPDIR:-/tmp}/termite-guibench.XXXXXX")}
mkdir -p "$OUT/xdg/config/termite" "$OUT/xdg/data"
echo '{"window_width": 1600, "window_height": 1000}' > "$OUT/xdg/config/termite/config.json"
BIG=${TMPDIR:-/tmp}/termite-guibench-100MB.txt
if [ ! -s "$BIG" ]; then
	echo "making $BIG"
	yes "The quick brown fox jumps over the lazy dog, lorem ipsum dolor sit amet" | head -c 100000000 > "$BIG"
fi

DPY=:97
Xvfb $DPY -screen 0 1920x1200x24 >/dev/null 2>&1 &
XVFB=$!
trap 'kill $XVFB' EXIT
sleep 1

term() { # TERMITE_DEBUG value, then termite's arguments
	local dbg=$1
	shift
	DISPLAY=$DPY QT_QPA_PLATFORM=xcb XDG_CONFIG_HOME="$OUT/xdg/config" XDG_DATA_HOME="$OUT/xdg/data" \
		TERMITE_DEBUG=$dbg timeout 120 "$BIN" "$@"
}
timeit() { # name, shell command to time three times
	term "" -- bash --norc -c "for i in 1 2 3; do s=\$(date +%s.%N); $2; e=\$(date +%s.%N); echo \"\$s \$e\" >> '$OUT/$1.times'; done" 2>/dev/null
}

timeit cat "cat '$BIG'"
timeit fullscreen "python3 '$HERE/fullscreen.py'"
term frames -- bash --norc -c "cat '$BIG'" 2> "$OUT/cat.frames.txt"
term frames -- bash --norc -c "python3 '$HERE/fullscreen.py'" 2> "$OUT/fullscreen.frames.txt"
term frames -- bash --norc -c "'$HERE/sparse.sh'" 2> "$OUT/sparse.frames.txt"
term "" --cpuprofile "$OUT/cat.pprof" -- bash --norc -c "cat '$BIG'" 2>/dev/null

echo "termite GUI benchmark: $BIN (1600x1000 window)"
echo "cat 100 MB:          $(awk '{printf "%.2fs ", $2-$1}' "$OUT/cat.times")"
echo "fullscreen redraws:  $(awk '{printf "%.2fs ", $2-$1}' "$OUT/fullscreen.times")"
for n in cat fullscreen sparse; do
	awk -v n=$n '/frames, avg/ {f+=$2; a+=$5*$2; inst=$(NF-1)}
		END {if (f) printf "frames, %-10s %d frames, avg %.2f ms, %s instances each\n", n":", f, a/f, inst}' "$OUT/$n.frames.txt"
done
echo "CPU profile of cat:  go tool pprof -top $BIN $OUT/cat.pprof"
