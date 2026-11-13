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
	// rc reaches the PTY's descriptor for poll and ioctl. Through it the
	// descriptor can't be closed (and its number reused) mid-call, and a
	// call after Close fails rather than touching another file.
	rc syscall.RawConn

	// Input for the child waits in queue (queued bytes in all) for the
	// writer goroutine, which wake tells of more. Write never blocks: the
	// UI thread and the reader call it, and a child that stops reading
	// would otherwise stop them both.
	mu     sync.Mutex
	queue  [][]byte
	queued int
	wake   chan struct{}
	done   chan struct{}
	closed atomic.Bool

	// OnUpdate is called from the reader goroutine after each parsed chunk
	// (without the terminal lock held). It must be cheap and thread-safe.
	OnUpdate func()
	// OnExit is called once when the child exits or the PTY closes.
	OnExit func()
}

// maxQueued is the most input held for a child that isn't reading it;
// beyond it more is dropped, as the kernel would hold no more either.
const maxQueued = 1 << 20

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
		// Set by the AppImage runtime for termite itself. A shell must not
		// inherit them: a program started from it would think it was
		// running from the AppImage (termite's own desktop entry would
		// then point at the AppImage when it was not run from it).
		case "APPIMAGE", "APPDIR", "ARGV0", "OWD":
			continue
		}
		env = append(env, e)
	}
	env = append(env, "TERM=xterm-256color", "COLORTERM=truecolor", "TERM_PROGRAM=termite", "TERM_PROGRAM_VERSION="+term.Version)
	cmd.Env = env
	f, err := cpty.StartWithSize(cmd, &cpty.Winsize{Cols: uint16(o.Cols), Rows: uint16(o.Rows)})
	if err != nil {
		return nil, fmt.Errorf("start %s: %w", args[0], err)
	}
	rc, err := f.SyscallConn()
	if err != nil {
		f.Close()
		return nil, err
	}
	s := &Session{T: t, cmd: cmd, f: f, rc: rc, wake: make(chan struct{}, 1), done: make(chan struct{})}
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
	pending := func() bool { // more output already waiting?
		ready := false
		s.rc.Control(func(fd uintptr) {
			pfd := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
			k, err := unix.Poll(pfd, 0)
			ready = err == nil && k > 0 && pfd[0].Revents&unix.POLLIN != 0
		})
		return ready
	}
	for {
		n, err := s.f.Read(buf)
		// The kernel hands out at most a few KB per read; keep reading
		// while more is already buffered so bursts are parsed in large
		// (64 KB+) chunks and the screen is locked far less often.
		for err == nil && n > 0 && n < len(buf)-4096 && pending() {
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
		case <-s.wake:
		case <-s.done:
			return
		}
		for {
			s.mu.Lock()
			if len(s.queue) == 0 {
				s.mu.Unlock()
				break
			}
			b := s.queue[0]
			s.queue[0] = nil
			s.queue = s.queue[1:]
			s.mu.Unlock()
			for data := b; len(data) > 0; {
				n, err := s.f.Write(data)
				if err != nil {
					if errors.Is(err, syscall.EINTR) || errors.Is(err, syscall.EAGAIN) {
						continue
					}
					break
				}
				data = data[n:]
			}
			s.mu.Lock()
			s.queued -= len(b)
			s.mu.Unlock()
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

// Write queues input for the child. It never blocks: input beyond
// maxQueued waiting for a child that isn't reading is dropped.
func (s *Session) Write(p []byte) {
	if s.closed.Load() || len(p) == 0 {
		return
	}
	s.mu.Lock()
	if s.queued+len(p) > maxQueued {
		s.mu.Unlock()
		return
	}
	s.queue = append(s.queue, append([]byte(nil), p...))
	s.queued += len(p)
	s.mu.Unlock()
	select {
	case s.wake <- struct{}{}:
	default: // the writer has been told already
	}
}

// WriteString queues a string.
func (s *Session) WriteString(str string) { s.Write([]byte(str)) }

// Resize changes the PTY window size (and so sends SIGWINCH).
func (s *Session) Resize(cols, rows, pxW, pxH int) {
	ws := &unix.Winsize{Col: uint16(cols), Row: uint16(rows), Xpixel: uint16(pxW), Ypixel: uint16(pxH)}
	s.rc.Control(func(fd uintptr) { _ = unix.IoctlSetWinsize(int(fd), unix.TIOCSWINSZ, ws) })
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
	var pgid int32
	var e syscall.Errno
	if err := s.rc.Control(func(fd uintptr) {
		_, _, e = syscall.Syscall(syscall.SYS_IOCTL, fd, syscall.TIOCGPGRP, uintptr(unsafe.Pointer(&pgid)))
	}); err != nil || e != 0 {
		return 0
	}
	return pgid
}
