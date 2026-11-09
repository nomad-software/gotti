package pty

import (
	"strings"
	"testing"
	"time"

	"termite/internal/term"
)

func TestSessionRoundTrip(t *testing.T) {
	tm := term.New(40, 5, 100)
	s, err := Start(tm, Options{Command: []string{"bash", "-c", "read x; printf 'got:%s\\n' \"$x\"; printf '\\033[6n'; read -r -s -d R r; echo done"}, Cols: 40, Rows: 5})
	if err != nil {
		t.Fatal(err)
	}
	exited := make(chan struct{})
	s.OnExit = func() { close(exited) }
	s.Run()
	s.WriteString("hello\r")
	select {
	case <-exited:
	case <-time.After(5 * time.Second):
		t.Fatal("child did not exit (cursor position report not answered?)")
	}
	tm.Mu.Lock()
	defer tm.Mu.Unlock()
	b := tm.Buf()
	top := b.ScreenTopAbs()
	screen := b.TextRange(term.Pos{Line: top}, term.Pos{Line: top + 4, Col: 40}, false)
	if !strings.Contains(screen, "got:hello") || !strings.Contains(screen, "done") {
		t.Fatalf("unexpected screen:\n%s", screen)
	}
}
