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

// TestResizeAndForeground checks the ioctls made through the PTY: the new
// size reaches the child, and its foreground process is found.
func TestResizeAndForeground(t *testing.T) {
	tm := term.New(40, 5, 100)
	s, err := Start(tm, Options{Command: []string{"bash", "-c", "read x; stty size; sleep 2"}, Dir: "/tmp", Cols: 40, Rows: 5})
	if err != nil {
		t.Fatal(err)
	}
	s.Run()
	defer s.Close()
	s.Resize(77, 13, 0, 0)
	s.WriteString("\r")
	deadline := time.Now().Add(5 * time.Second)
	for {
		tm.Mu.Lock()
		b := tm.Buf()
		top := b.ScreenTopAbs()
		screen := b.TextRange(term.Pos{Line: top}, term.Pos{Line: top + 4, Col: 40}, false)
		tm.Mu.Unlock()
		if strings.Contains(screen, "13 77") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("size not seen by the child:\n%s", screen)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if name := s.ForegroundName(); name != "sleep" && name != "bash" {
		t.Errorf("foreground name %q", name)
	}
	if dir := s.ForegroundCwd(); dir != "/tmp" {
		t.Errorf("foreground cwd %q", dir)
	}
}

// TestCloseWhileQuerying closes a session while other goroutines query and
// resize its PTY, as the pane header refresh does. Run with -race.
func TestCloseWhileQuerying(t *testing.T) {
	for i := 0; i < 20; i++ {
		s, err := Start(term.New(20, 5, 0), Options{Command: []string{"sleep", "5"}, Cols: 20, Rows: 5})
		if err != nil {
			t.Fatal(err)
		}
		s.Run()
		stop := make(chan struct{})
		done := make(chan struct{})
		go func() {
			defer close(done)
			for {
				select {
				case <-stop:
					return
				default:
					s.ForegroundCwd()
					s.ForegroundName()
					s.Resize(30, 6, 0, 0)
				}
			}
		}()
		time.Sleep(time.Millisecond)
		s.Close()
		close(stop)
		<-done
		if s.foregroundPgid() != 0 {
			t.Fatal("a closed session still reports a foreground process")
		}
	}
}

// TestChildEnvironment checks what a shell inherits: termite's TERM, and
// none of the variables the AppImage runtime sets for termite itself.
func TestChildEnvironment(t *testing.T) {
	t.Setenv("APPIMAGE", "/x/Termite.AppImage")
	t.Setenv("APPDIR", "/tmp/.mount_x")
	t.Setenv("OWD", "/home/me")
	t.Setenv("TERM", "dumb")
	tm := term.New(80, 5, 100)
	s, err := Start(tm, Options{Command: []string{"sh", "-c", `echo "[$TERM|${APPIMAGE-}|${APPDIR-}|${OWD-}]"`}, Cols: 80, Rows: 5})
	if err != nil {
		t.Fatal(err)
	}
	exited := make(chan struct{})
	s.OnExit = func() { close(exited) }
	s.Run()
	select {
	case <-exited:
	case <-time.After(5 * time.Second):
		t.Fatal("child did not exit")
	}
	tm.Mu.Lock()
	defer tm.Mu.Unlock()
	b := tm.Buf()
	top := b.ScreenTopAbs()
	if got := b.TextRange(term.Pos{Line: top}, term.Pos{Line: top, Col: 80}, false); got != "[xterm-256color|||]" {
		t.Fatalf("child saw %q", got)
	}
}
