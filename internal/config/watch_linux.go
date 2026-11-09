package config

import (
	"bytes"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/unix"
)

// watchNotify watches the folder holding path with inotify and calls
// changed when the file is written, replaced (as editors and SetValues do,
// by renaming a new file over it) or removed. If path is a symbolic link,
// as in a dotfiles checkout, the folder of the file it points to is watched
// as well.
func watchNotify(path string, changed func()) (func(), error) {
	fd, err := unix.InotifyInit1(unix.IN_NONBLOCK | unix.IN_CLOEXEC)
	if err != nil {
		return nil, err
	}
	// A non-blocking descriptor joins Go's poller, so Close wakes the
	// blocked Read and the goroutine ends.
	f := os.NewFile(uintptr(fd), "inotify")
	const mask = unix.IN_CLOSE_WRITE | unix.IN_MOVED_TO | unix.IN_MOVED_FROM | unix.IN_DELETE
	names := map[int32]string{} // watch descriptor → file name to look for
	files := []string{path}
	if target, err := filepath.EvalSymlinks(path); err == nil && target != path {
		files = append(files, target)
	}
	for _, file := range files {
		wd, err := unix.InotifyAddWatch(fd, filepath.Dir(file), mask)
		if err != nil {
			if len(names) == 0 {
				f.Close()
				return nil, err
			}
			continue
		}
		names[int32(wd)] = filepath.Base(file)
	}
	go func() {
		buf := make([]byte, 64*(unix.SizeofInotifyEvent+unix.NAME_MAX+1))
		for {
			n, err := f.Read(buf)
			if err != nil {
				return
			}
			hit := false
			for off := 0; off+unix.SizeofInotifyEvent <= n; {
				ev := (*unix.InotifyEvent)(unsafe.Pointer(&buf[off]))
				name := buf[off+unix.SizeofInotifyEvent : off+unix.SizeofInotifyEvent+int(ev.Len)]
				if i := bytes.IndexByte(name, 0); i >= 0 {
					name = name[:i]
				}
				if want, ok := names[ev.Wd]; ok && string(name) == want {
					hit = true
				}
				off += unix.SizeofInotifyEvent + int(ev.Len)
			}
			if hit { // one reload for a burst of events
				changed()
			}
		}
	}()
	return func() { f.Close() }, nil
}
