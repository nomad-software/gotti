package term

import (
	"bytes"
	"fmt"
	"testing"
)

func benchData(kind string) []byte {
	var b bytes.Buffer
	for i := 0; b.Len() < 4<<20; i++ {
		switch kind {
		case "ascii":
			fmt.Fprintf(&b, "%06d The quick brown fox jumps over the lazy dog; lorem ipsum dolor sit amet, consectetur.\r\n", i)
		case "sgr":
			fmt.Fprintf(&b, "\x1b[1;3%dm%06d\x1b[0m \x1b[38;2;%d;100;200mquick brown\x1b[0m \x1b[4mfox\x1b[24m jumps \x1b[48;5;%dmover\x1b[m\r\n", i%8, i, i%256, i%256)
		case "unicode":
			fmt.Fprintf(&b, "%06d 日本語のテキスト — ünïcödé ✓ ★ │ ─ ┼ 😀 text\r\n", i)
		case "cursor":
			fmt.Fprintf(&b, "\x1b[%d;%dH\x1b[2Kline %d\x1b[K", i%24+1, i%40+1, i)
		}
	}
	return b.Bytes()
}

func benchFeed(b *testing.B, kind string) {
	data := benchData(kind)
	t := New(200, 50, 10000)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for off := 0; off < len(data); off += 65536 {
			end := min(off+65536, len(data))
			t.Mu.Lock()
			t.Write(data[off:end])
			t.Mu.Unlock()
		}
	}
}

func BenchmarkASCII(b *testing.B)   { benchFeed(b, "ascii") }
func BenchmarkSGR(b *testing.B)     { benchFeed(b, "sgr") }
func BenchmarkUnicode(b *testing.B) { benchFeed(b, "unicode") }
func BenchmarkCursor(b *testing.B)  { benchFeed(b, "cursor") }
