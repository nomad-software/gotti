package term

import (
	"math/rand"
	"os"
	"runtime/pprof"
	"testing"
	"time"
)

func TestStressRandom(t *testing.T) {
	if os.Getenv("STRESS") == "" {
		t.Skip("set STRESS=1")
	}
	pieces := []string{"\x1b[", "\x1b]", "\x1bP", "\x1b", ";", ":", "?", ">", "<", "=", "$", " ", "!", "1", "2", "9", "65535", "0",
		"m", "H", "J", "K", "r", "S", "T", "L", "M", "@", "P", "X", "b", "h", "l", "u", "q", "p", "t", "\x07", "\x1b\\", "\r", "\n", "\t", "\b",
		"a", "世", "́", "\u200d", "🇬", "👨", "\x0e", "\x0f", "(0", "#8", "7", "8", "1049", "2026", "6", "47", "1047", "\x18", "\xff", "\xe4"}
	rng := rand.New(rand.NewSource(1))
	deadline := time.Now().Add(40 * time.Second)
	for n := 0; time.Now().Before(deadline); n++ {
		var b []byte
		for k := rng.Intn(60); k > 0; k-- {
			b = append(b, pieces[rng.Intn(len(pieces))]...)
		}
		done := make(chan bool)
		go func() {
			term := New(13, 7, 20)
			term.Write(b)
			term.Resize(7, 11)
			term.Write(b)
			term.Resize(20, 3)
			term.Write(b)
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			pprof.Lookup("goroutine").WriteTo(os.Stderr, 2)
			t.Fatalf("hang after %d on %q", n, b)
		}
	}
}
