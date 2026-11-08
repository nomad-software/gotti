package term

import (
	"fmt"
	"runtime"
	"testing"
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
