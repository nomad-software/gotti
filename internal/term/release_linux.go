package term

import (
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

var pageSize = uintptr(os.Getpagesize())

// releaseCells blanks cells and hands the whole pages they cover back to
// the kernel (MADV_DONTNEED), which maps them to zero pages again: they
// read back as blank cells and stop counting towards resident memory.
// Cells hold no pointers, so the garbage collector never looks inside, and
// a later write simply faults a fresh zero page in. The partial pages at
// either end are cleared by hand.
func releaseCells(cells []Cell) {
	if len(cells) == 0 {
		return
	}
	raw := unsafe.Slice((*byte)(unsafe.Pointer(&cells[0])), uintptr(len(cells))*unsafe.Sizeof(Cell{}))
	addr := uintptr(unsafe.Pointer(&raw[0]))
	first := (addr + pageSize - 1) &^ (pageSize - 1)     // first whole page
	last := (addr + uintptr(len(raw))) &^ (pageSize - 1) // end of the last
	if last <= first {
		clear(cells) // no whole page inside
		return
	}
	lo, hi := first-addr, last-addr
	if unix.Madvise(raw[lo:hi], unix.MADV_DONTNEED) != nil {
		clear(cells) // just blank them
		return
	}
	clear(raw[:lo])
	clear(raw[hi:])
}
