package term

import (
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"
	"time"
)

// TestMemoryPerPane checks the emulator's memory with 10k lines of
// scrollback filled at a typical width.
func TestMemoryPerPane(t *testing.T) {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	term := New(120, 40, 10000)
	for i := 0; i < 11000; i++ {
		term.Write([]byte(fmt.Sprintf("%06d the quick brown fox jumps over the lazy dog, again and again and again\r\n", i)))
	}
	term.Resize(100, 40) // a reflow must not leave the heap bloated
	runtime.GC()
	runtime.ReadMemStats(&after)
	mb := float64(after.HeapInuse-before.HeapInuse) / (1 << 20)
	t.Logf("heap in use for one pane (120 cols, 10k scrollback, after reflow): %.1f MB", mb)
	if mb > 20 {
		t.Fatalf("pane uses %.1f MB, want < 20", mb)
	}
	runtime.KeepAlive(term)
}

// rssMB returns the process's resident memory.
func rssMB(t *testing.T) float64 {
	data, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		t.Skip("no /proc/self/statm")
	}
	var size, resident int64
	fmt.Sscan(string(data), &size, &resident)
	return float64(resident*int64(os.Getpagesize())) / (1 << 20)
}

// TestClearScrollbackMemory measures the memory Clear Scrollback gives back:
// CLEARMEM=1 go test ./internal/term -run ClearScrollbackMemory -v
func TestClearScrollbackMemory(t *testing.T) {
	if os.Getenv("CLEARMEM") == "" {
		t.Skip("set CLEARMEM=1 to measure")
	}
	const lines = 100000
	line := []byte("the quick brown fox jumps over the lazy dog, the quick brown fox jumps over the lazy dog, again\r\n")
	base := rssMB(t)
	term := New(120, 40, lines)
	for i := 0; i < lines+100; i++ {
		term.Write(line)
	}
	full := rssMB(t)
	start := time.Now()
	term.Buf().ClearScrollback()
	took := time.Since(start)
	runtime.GC()
	debug.FreeOSMemory()
	cleared := rssMB(t)
	t.Logf("RSS: start %.0f MB, scrollback full %.0f MB, after clear %.0f MB (clear took %v)", base, full, cleared, took)
	runtime.KeepAlive(term)
}

// TestClearScrollback checks that clearing keeps the screen and wipes the
// old lines, whether or not the screen wraps around the end of the ring.
func TestClearScrollback(t *testing.T) {
	for _, n := range []int{3, 25, 28, 60} { // lines written: within, at and past the ring's end
		term := New(10, 4, 20) // a ring of 24 lines
		for i := 0; i < n; i++ {
			feed(term, fmt.Sprintf("L%d\r\n", i))
		}
		feed(term, "end")
		b := term.Buf()
		before := screen(term)
		evicted := b.Evicted + int64(b.ScrollbackLen())
		b.ClearScrollback()
		if got := screen(term); got != before {
			t.Fatalf("n=%d: screen changed:\n%s\nwant\n%s", n, got, before)
		}
		if b.ScrollbackLen() != 0 || b.Evicted != evicted {
			t.Fatalf("n=%d: scrollback %d, evicted %d (want 0, %d)", n, b.ScrollbackLen(), b.Evicted, evicted)
		}
		// Every physical line outside the screen must be blank.
		onScreen := map[int]bool{}
		for y := 0; y < b.Rows; y++ {
			onScreen[b.phys(b.count-b.Rows+y)] = true
		}
		for p := 0; p < len(b.wrapped); p++ {
			if onScreen[p] {
				continue
			}
			for _, c := range b.cells[p*b.Cols : (p+1)*b.Cols] {
				if c != (Cell{}) {
					t.Fatalf("n=%d: old line %d not wiped: %+v", n, p, c)
				}
			}
		}
		// Output carries on normally afterwards.
		feed(term, "\r\nmore\r\nlines\r\nhere\r\nnow")
		if !strings.Contains(screen(term), "now") {
			t.Fatalf("n=%d: output after clearing:\n%s", n, screen(term))
		}
	}
}

// TestReleaseCells blanks ranges at many offsets and lengths, across page
// boundaries or within one page, and checks nothing outside them changes.
func TestReleaseCells(t *testing.T) {
	cells := make([]Cell, 3*os.Getpagesize()) // 48 pages
	fill := Cell{Ch: 'x', Fg: 1, Bg: 2, Attr: 3, Extra: 4}
	for _, r := range [][2]int{{0, 0}, {0, 1}, {5, 9}, {100, 400}, {255, 257}, {256, 512}, {3, 5000}, {0, len(cells)}, {1000, len(cells) - 7}} {
		for i := range cells {
			cells[i] = fill
		}
		releaseCells(cells[r[0]:r[1]])
		for i, c := range cells {
			inside := i >= r[0] && i < r[1]
			if inside && c != (Cell{}) || !inside && c != fill {
				t.Fatalf("range %v: cell %d is %+v", r, i, c)
			}
		}
	}
}
