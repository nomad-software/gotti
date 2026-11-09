// Package pty runs a child process on a pseudo-terminal and pumps its
// output through a term.Terminal on a dedicated goroutine.
package pty

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"

	cpty "github.com/creack/pty"
	"golang.org/x/sys/unix"

	"termite/internal/term"
)

const readBufSize = 128 << 10 // read in large chunks (well above 64 KB)

// Session is a child process attached to a terminal.
type Session struct {
	T   *term.Terminal
	cmd *exec.Cmd
	f   *os.File
	fd  uintptr

	writes chan *[]byte
	done   chan struct{}
	closed atomic.Bool

	// OnUpdate is called from the reader goroutine after each parsed chunk
	// (without the terminal lock held). It must be cheap and thread-safe.
	OnUpdate func()
	// OnExit is called once when the child exits or the PTY closes.
	OnExit func()
}

var bufPool = sync.Pool{New: func() any { b := make([]byte, 0, 512); return &b }}

// Options configure a new session.
type Options struct {
	Command []string // nil: user's shell
	Dir     string
	Cols    int
	Rows    int
}

// DefaultShell returns the user's login shell.
func DefaultShell() string {
	if s := os.Getenv("SHELL"); s != "" {
		return s
	}
	return "/bin/sh"
}

// Start launches the command on a new PTY. Output processing begins when
// Run is called, so callbacks can be set first.
func Start(t *term.Terminal, o Options) (*Session, error) {
	args := o.Command
	if len(args) == 0 {
		args = []string{DefaultShell()}
	}
	cmd := exec.Command(args[0], args[1:]...)
	if o.Dir != "" {
		if st, err := os.Stat(o.Dir); err == nil && st.IsDir() {
			cmd.Dir = o.Dir
		}
	}
	env := make([]string, 0, len(os.Environ())+8)
	for _, e := range os.Environ() {
		k, _, _ := strings.Cut(e, "=")
		switch k {
		case "TERM", "COLORTERM", "TERM_PROGRAM", "TERM_PROGRAM_VERSION", "VTE_VERSION", "COLUMNS", "LINES":
			continue
		}
		env = append(env, e)
	}
	env = append(env, "TERM=xterm-256color", "COLORTERM=truecolor", "TERM_PROGRAM=termite", "TERM_PROGRAM_VERSION=1.0")
	cmd.Env = env
	f, err := cpty.StartWithSize(cmd, &cpty.Winsize{Cols: uint16(o.Cols), Rows: uint16(o.Rows)})
	if err != nil {
		return nil, fmt.Errorf("start %s: %w", args[0], err)
	}
	s := &Session{T: t, cmd: cmd, f: f, fd: f.Fd(), writes: make(chan *[]byte, 1024), done: make(chan struct{})}
	return s, nil
}

// Run starts the reader and writer goroutines.
func (s *Session) Run() {
	go s.writer()
	go s.reader()
}

func (s *Session) reader() {
	defer s.finish()
	buf := make([]byte, readBufSize)
	fd := int32(s.fd)
	pfd := []unix.PollFd{{Fd: fd, Events: unix.POLLIN}}
	for {
		n, err := s.f.Read(buf)
		// The kernel hands out at most a few KB per read; keep reading
		// while more is already buffered so bursts are parsed in large
		// (64 KB+) chunks and the screen is locked far less often.
		for err == nil && n > 0 && n < len(buf)-4096 {
			pfd[0].Revents = 0
			k, perr := unix.Poll(pfd, 0)
			if perr != nil || k <= 0 || pfd[0].Revents&unix.POLLIN == 0 {
				break
			}
			m, rerr := s.f.Read(buf[n:])
			n += m
			if rerr != nil {
				err = rerr
			}
			if m <= 0 {
				break
			}
		}
		if n > 0 {
			t := s.T
			t.Mu.Lock()
			t.Write(buf[:n])
			replies := t.TakeReplies()
			t.Mu.Unlock()
			if len(replies) > 0 {
				s.Write(replies)
			}
			if s.OnUpdate != nil {
				s.OnUpdate()
			}
		}
		if err != nil {
			return
		}
	}
}

func (s *Session) writer() {
	for {
		select {
		case b := <-s.writes:
			data := *b
			for len(data) > 0 {
				n, err := s.f.Write(data)
				if err != nil {
					if errors.Is(err, syscall.EINTR) || errors.Is(err, syscall.EAGAIN) {
						continue
					}
					break
				}
				data = data[n:]
			}
			*b = (*b)[:0]
			bufPool.Put(b)
		case <-s.done:
			return
		}
	}
}

func (s *Session) finish() {
	if s.closed.Swap(true) {
		return
	}
	close(s.done)
	_ = s.f.Close()
	go func() { _ = s.cmd.Wait() }()
	if s.OnExit != nil {
		s.OnExit()
	}
}

// Write queues input for the child. It never blocks the caller for long
// (it may block if over 1024 writes are pending).
func (s *Session) Write(p []byte) {
	if s.closed.Load() || len(p) == 0 {
		return
	}
	b := bufPool.Get().(*[]byte)
	*b = append((*b)[:0], p...)
	select {
	case s.writes <- b:
	case <-s.done:
	}
}

// WriteString queues a string.
func (s *Session) WriteString(str string) { s.Write([]byte(str)) }

// Resize changes the PTY window size (and so sends SIGWINCH).
func (s *Session) Resize(cols, rows, pxW, pxH int) {
	if s.closed.Load() {
		return
	}
	_ = cpty.Setsize(s.f, &cpty.Winsize{Cols: uint16(cols), Rows: uint16(rows), X: uint16(pxW), Y: uint16(pxH)})
}

// Close terminates the child.
func (s *Session) Close() {
	if s.cmd.Process != nil {
		_ = s.cmd.Process.Signal(syscall.SIGHUP)
	}
	s.finish()
}

// Pid returns the child process id.
func (s *Session) Pid() int {
	if s.cmd.Process == nil {
		return 0
	}
	return s.cmd.Process.Pid
}

// ForegroundCwd returns the working directory of the PTY's foreground
// process (falls back to the child's).
func (s *Session) ForegroundCwd() string {
	pid := s.Pid()
	if !s.closed.Load() {
		if pgid := s.foregroundPgid(); pgid > 0 {
			pid = int(pgid)
		}
	}
	if pid == 0 {
		return ""
	}
	dir, err := os.Readlink(fmt.Sprintf("/proc/%d/cwd", pid))
	if err != nil {
		return ""
	}
	return dir
}

// ForegroundName returns the command name of the foreground process.
func (s *Session) ForegroundName() string {
	pgid := s.foregroundPgid()
	if pgid <= 0 {
		return ""
	}
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pgid))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func (s *Session) foregroundPgid() int32 {
	if s.closed.Load() {
		return 0
	}
	var pgid int32
	_, _, e := syscall.Syscall(syscall.SYS_IOCTL, s.fd, syscall.TIOCGPGRP, uintptr(unsafe.Pointer(&pgid)))
	if e != 0 {
		return 0
	}
	return pgid
}
