//go:build !linux

package term

// releaseCells blanks cells. Only Linux gives their memory back.
func releaseCells(cells []Cell) { clear(cells) }
