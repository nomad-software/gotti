package render

import (
	"math"
	"unsafe"

	"github.com/nomad-software/qute/qtcore"
	"github.com/nomad-software/qute/qtgui"
)

// Font styles (bit flags).
const (
	styleBold   = 1
	styleItalic = 2
)

// FontSet holds the four font faces and the derived cell metrics, all in
// device pixels.
type FontSet struct {
	faces [4]*qtgui.QFont
	// metrics measures each face on img (the device glyphs are drawn on).
	metrics [4]*qtgui.QFontMetricsF
	DPR     float64

	CellW, CellH int
	Baseline     int // from cell top
	ULPos        int // underline top, from cell top
	ULThick      int
	StrikePos    int

	img     *qtgui.QImage
	imgW    int
	imgH    int
	pad     int
	painter *qtgui.QPainter
}

// NewFontSet loads family at size points for the given device pixel ratio.
func NewFontSet(family string, size, lineHeight, dpr float64) *FontSet {
	fs := &FontSet{DPR: dpr}
	for st := 0; st < 4; st++ {
		f := qtgui.NewQFont()
		f.SetFamilies([]string{family, "monospace"})
		f.SetStyleHintFull(qtgui.QFontMonospace, qtgui.QFontPreferAntialias|qtgui.QFontPreferQuality)
		f.SetFixedPitch(true)
		f.SetKerning(false)
		f.SetHintingPreference(qtgui.QFontPreferFullHinting)
		// QImage paints at 96 dpi; scale the point size for the device.
		f.SetPointSizeF(size * dpr)
		if st&styleBold != 0 {
			f.SetWeight(qtgui.QFontBold)
		}
		if st&styleItalic != 0 {
			f.SetItalic(true)
		}
		fs.faces[st] = f
	}
	probe := qtgui.NewQImageWithSize(qtcore.QSize{Width: 4, Height: 4}, qtgui.QImageFormat_ARGB32_Premultiplied)
	fm := qtgui.NewQFontMetricsFWithFontPaintDevice(fs.faces[0], probe)
	adv := fm.HorizontalAdvance("M")
	asc, desc := fm.Ascent(), fm.Descent()
	fs.CellW = max(1, int(math.Ceil(adv-0.02)))
	natural := asc + desc
	fs.CellH = max(1, int(math.Ceil(natural*lineHeight)))
	fs.Baseline = int(math.Round(asc + (float64(fs.CellH)-natural)/2))
	fs.ULThick = max(1, int(math.Round(fm.LineWidth())))
	fs.ULPos = min(fs.CellH-fs.ULThick, fs.Baseline+max(1, int(math.Round(fm.UnderlinePos()))))
	fs.StrikePos = fs.Baseline - int(math.Round(fm.XHeight()/2)) - fs.ULThick/2
	fm.Delete()
	probe.Delete()

	fs.pad = fs.CellW
	fs.imgW = fs.CellW*4 + 2*fs.pad
	fs.imgH = fs.CellH + 2*fs.pad
	fs.img = qtgui.NewQImageWithSize(qtcore.QSize{Width: fs.imgW, Height: fs.imgH}, qtgui.QImageFormat_ARGB32_Premultiplied)
	for st, f := range fs.faces {
		fs.metrics[st] = qtgui.NewQFontMetricsFWithFontPaintDevice(f, fs.img)
	}
	fs.painter = qtgui.NewQPainter()
	return fs
}

// Delete frees the Qt objects.
func (fs *FontSet) Delete() {
	for st, f := range fs.faces {
		fs.metrics[st].Delete()
		f.Delete()
	}
	fs.painter.Delete()
	fs.img.Delete()
}

// rasterized is a trimmed glyph bitmap in BGRA premultiplied format.
type rasterized struct {
	pix        []byte
	w, h       int
	offX, offY int // offset of the bitmap from the cell's top-left
	color      bool
}

// Rasterize draws text spanning cells columns with the given style.
func (fs *FontSet) Rasterize(text string, style int, cells int) rasterized {
	cells = min(max(cells, 1), 4)
	img := fs.img
	img.Fill(0)
	p := fs.painter
	p.Begin(img)
	p.SetRenderHint(qtgui.QPainterTextAntialiasing)
	p.SetRenderHint(qtgui.QPainterAntialiasing)
	p.SetFont(fs.faces[style&3])
	p.SetPenColor(qtgui.RGB(255, 255, 255))
	// Scale glyphs (typically emoji or wide symbols) that overflow their cells.
	adv := fs.metrics[style&3].HorizontalAdvance(text)
	avail := float64(cells * fs.CellW)
	x0, y0 := float64(fs.pad), float64(fs.pad+fs.Baseline)
	if adv > avail*1.05 && adv > 0 {
		s := avail / adv
		// keep the glyph vertically centred around the cell's middle
		cy := float64(fs.pad) + float64(fs.CellH)/2
		p.Translate(x0, cy)
		p.Scale(s, s)
		p.DrawTextF(qtcore.QPointF{X: 0, Y: float64(fs.Baseline) - float64(fs.CellH)/2}, text)
	} else {
		if adv < avail*0.95 && cells > 1 {
			x0 += (avail - adv) / 2 // centre narrow glyphs in wide cells
		}
		p.DrawTextF(qtcore.QPointF{X: x0, Y: y0}, text)
	}
	p.End()

	bpl := int(img.BytesPerLine())
	src := unsafe.Slice((*byte)(img.Bits()), bpl*fs.imgH)
	// bounding box of non-transparent pixels
	minX, minY, maxX, maxY := fs.imgW, fs.imgH, -1, -1
	color := false
	for y := 0; y < fs.imgH; y++ {
		row := src[y*bpl : y*bpl+fs.imgW*4]
		for x := 0; x < fs.imgW; x++ {
			a := row[x*4+3]
			if a == 0 {
				continue
			}
			if x < minX {
				minX = x
			}
			if x > maxX {
				maxX = x
			}
			if y < minY {
				minY = y
			}
			if y > maxY {
				maxY = y
			}
			if !color {
				b, g, r := row[x*4], row[x*4+1], row[x*4+2]
				if absDiff(r, a) > 12 || absDiff(g, a) > 12 || absDiff(b, a) > 12 {
					color = true
				}
			}
		}
	}
	if maxX < 0 {
		return rasterized{}
	}
	w, h := maxX-minX+1, maxY-minY+1
	pix := make([]byte, w*h*4)
	for y := 0; y < h; y++ {
		copy(pix[y*w*4:(y+1)*w*4], src[(minY+y)*bpl+minX*4:(minY+y)*bpl+(minX+w)*4])
	}
	if !color {
		// normalise to pure coverage (a,a,a,a)
		for i := 0; i < len(pix); i += 4 {
			a := pix[i+3]
			pix[i], pix[i+1], pix[i+2] = a, a, a
		}
	}
	return rasterized{pix: pix, w: w, h: h, offX: minX - fs.pad, offY: minY - fs.pad, color: color}
}

func absDiff(a, b byte) byte {
	if a > b {
		return a - b
	}
	return b - a
}
