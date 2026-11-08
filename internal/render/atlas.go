// Package render draws terminal grids with OpenGL 3.3 core: a glyph atlas
// and instanced quads, two draw calls per frame.
package render

// Atlas is a simple shelf packer for glyph sprites.
type Atlas struct {
	W, H       int
	x, y, rowH int
}

// NewAtlas creates an empty atlas of the given size.
func NewAtlas(w, h int) *Atlas { return &Atlas{W: w, H: h, x: 1, y: 1} }

// Alloc reserves a w×h region (with a 1px gutter) and returns its origin,
// or ok=false when the atlas is full.
func (a *Atlas) Alloc(w, h int) (x, y int, ok bool) {
	if w <= 0 || h <= 0 {
		return 0, 0, true
	}
	if w+2 > a.W || h+2 > a.H {
		return 0, 0, false
	}
	if a.x+w+1 > a.W {
		a.y += a.rowH + 1
		a.x, a.rowH = 1, 0
	}
	if a.y+h+1 > a.H {
		return 0, 0, false
	}
	x, y = a.x, a.y
	a.x += w + 1
	if h > a.rowH {
		a.rowH = h
	}
	return x, y, true
}

// Reset empties the atlas.
func (a *Atlas) Reset() { a.x, a.y, a.rowH = 1, 1, 0 }
