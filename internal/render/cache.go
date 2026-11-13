package render

import (
	"unsafe"

	"github.com/go-gl/gl/v3.3-core/gl"
)

type glyphKind uint8

const (
	kindRune glyphKind = iota
	kindCluster
	kindDecoration
	kindText // a string drawn as one sprite: a ligature, or IME preedit text
)

type glyphKey struct {
	ch    rune
	kind  glyphKind
	style uint8
	cells uint8
}

// Sprite locates a glyph in the atlas.
type Sprite struct {
	X, Y, W, H uint16
	OffX, OffY int16
	Color      bool
}

// GlyphCache rasterizes glyphs once into a shared atlas texture. GL
// contexts of all panes share resources, so one cache serves them all.
type GlyphCache struct {
	Fonts *FontSet
	atlas *Atlas
	tex   uint32
	size  int
	m     map[glyphKey]Sprite
	// Gen changes whenever cached sprites are invalidated.
	Gen       int
	textIndex map[string]rune // ids of strings drawn as kindText sprites
	maxSize   int
}

// NewGlyphCache creates a cache for fonts. A GL context must be current.
func NewGlyphCache(fonts *FontSet) *GlyphCache {
	c := &GlyphCache{Fonts: fonts, m: map[glyphKey]Sprite{}, textIndex: map[string]rune{}}
	var maxTex int32
	gl.GetIntegerv(gl.MAX_TEXTURE_SIZE, &maxTex)
	c.maxSize = int(min(maxTex, 8192))
	c.size = min(2048, c.maxSize)
	c.atlas = NewAtlas(c.size, c.size)
	gl.GenTextures(1, &c.tex)
	c.allocTexture()
	return c
}

func (c *GlyphCache) allocTexture() {
	gl.BindTexture(gl.TEXTURE_2D, c.tex)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA8, int32(c.size), int32(c.size), 0, gl.BGRA, gl.UNSIGNED_BYTE, nil)
}

// Texture returns the atlas texture and its size.
func (c *GlyphCache) Texture() (uint32, int) { return c.tex, c.size }

// SetFonts replaces the fonts, invalidating every sprite.
func (c *GlyphCache) SetFonts(f *FontSet) {
	if c.Fonts != nil {
		c.Fonts.Delete()
	}
	c.Fonts = f
	c.reset()
}

func (c *GlyphCache) reset() {
	c.m = map[glyphKey]Sprite{}
	c.atlas.Reset()
	c.Gen++
}

// textID returns the id of a kindText sprite for s.
func (c *GlyphCache) textID(s string) rune {
	if id, ok := c.textIndex[s]; ok {
		return id
	}
	id := rune(len(c.textIndex))
	c.textIndex[s] = id
	return id
}

// get returns the sprite for key, rasterizing it on first use. It returns
// ok=false if the glyph has no visible pixels, or doesn't fit in the atlas.
func (c *GlyphCache) get(k glyphKey, text func() string) (Sprite, bool) {
	if s, ok := c.m[k]; ok {
		return s, s.W > 0
	}
	fs := c.Fonts
	var r rasterized
	switch k.kind {
	case kindDecoration:
		w := fs.CellW * int(k.cells)
		a := drawDecoration(int(k.ch), w, fs.CellH, fs.ULPos, fs.ULThick, fs.StrikePos)
		r = maskToRaster(a, w, fs.CellH)
	default:
		if k.kind == kindRune && IsProcedural(k.ch) {
			w := fs.CellW * int(k.cells)
			a, _ := DrawProcedural(k.ch, w, fs.CellH)
			r = maskToRaster(a, w, fs.CellH)
		} else {
			r = fs.Rasterize(text(), int(k.style), int(k.cells))
		}
	}
	if r.w == 0 {
		c.m[k] = Sprite{}
		return Sprite{}, false
	}
	x, y, ok := c.atlas.Alloc(r.w, r.h)
	if !ok {
		// Grow the atlas if possible, otherwise start over.
		if c.size < c.maxSize {
			c.size *= 2
			c.atlas = NewAtlas(c.size, c.size)
			c.allocTexture()
		}
		c.reset()
		x, y, ok = c.atlas.Alloc(r.w, r.h)
		if !ok {
			// Too big even for an empty atlas: remember that, or every
			// frame would clear the atlas again trying.
			c.m[k] = Sprite{}
			return Sprite{}, false
		}
	}
	gl.BindTexture(gl.TEXTURE_2D, c.tex)
	gl.PixelStorei(gl.UNPACK_ALIGNMENT, 4)
	gl.TexSubImage2D(gl.TEXTURE_2D, 0, int32(x), int32(y), int32(r.w), int32(r.h), gl.BGRA, gl.UNSIGNED_BYTE, unsafe.Pointer(&r.pix[0]))
	s := Sprite{X: uint16(x), Y: uint16(y), W: uint16(r.w), H: uint16(r.h), OffX: int16(r.offX), OffY: int16(r.offY), Color: r.color}
	c.m[k] = s
	return s, true
}

// maskToRaster converts an alpha mask to a trimmed BGRA raster.
func maskToRaster(a []byte, w, h int) rasterized {
	minX, minY, maxX, maxY := w, h, -1, -1
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if a[y*w+x] != 0 {
				minX, maxX = min(minX, x), max(maxX, x)
				minY, maxY = min(minY, y), max(maxY, y)
			}
		}
	}
	if maxX < 0 {
		return rasterized{}
	}
	rw, rh := maxX-minX+1, maxY-minY+1
	pix := make([]byte, rw*rh*4)
	for y := 0; y < rh; y++ {
		for x := 0; x < rw; x++ {
			v := a[(minY+y)*w+minX+x]
			i := (y*rw + x) * 4
			pix[i], pix[i+1], pix[i+2], pix[i+3] = v, v, v, v
		}
	}
	return rasterized{pix: pix, w: rw, h: rh, offX: minX, offY: minY}
}
