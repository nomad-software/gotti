package ui

import (
	"bytes"
	"fmt"
	"os"

	"github.com/nomad-software/qute/qtcore"
	"github.com/nomad-software/qute/qtgui"
	"github.com/nomad-software/qute/qtsvg"

	"termite/internal/icons"
)

// icon returns termite's own icon with the name. It is drawn each time it is
// shown, in the colours of the palette as it then is, so that it follows a
// change of window theme: in the colour of the text, of disabled text where
// it is disabled, and of selected text where it is selected.
func icon(name string) *qtgui.QIcon {
	data, err := icons.SVG(name)
	if err != nil {
		fmt.Fprintln(os.Stderr, "termite:", err)
		return qtgui.NewQIcon()
	}
	return qtgui.NewQIconWithIconEngine(newIconEngine(name, data))
}

// iconEngine draws the icon with the name, whose SVG is data.
type iconEngine struct {
	qtgui.QIconEngineBase
	name string
	data []byte
}

// newIconEngine returns the engine that draws the icon with the name, whose
// SVG is data. The icon it is given to owns it.
func newIconEngine(name string, data []byte) *iconEngine {
	e := &iconEngine{name: name, data: data}
	qtgui.NewQIconEngineWith(e)
	return e
}

// Paint overrides QIconEngine::paint.
func (e *iconEngine) Paint(p *qtgui.QPainter, rect qtcore.QRect, mode qtgui.QIconMode, _ qtgui.QIconState) {
	e.paint(p, qtcore.QRectF{X: float64(rect.X), Y: float64(rect.Y), Width: float64(rect.Width), Height: float64(rect.Height)}, mode)
}

// Pixmap overrides QIconEngine::pixmap. Qt's own pixmap of an engine is not
// cleared before it is painted, so the icon is drawn on a transparent one.
func (e *iconEngine) Pixmap(size qtcore.QSize, mode qtgui.QIconMode, _ qtgui.QIconState) *qtgui.QPixmap {
	img := qtgui.NewQImageWithSize(size, qtgui.QImageFormat_ARGB32_Premultiplied)
	img.FillColor(qtgui.QColor{})
	p := qtgui.NewQPainterOn(img)
	e.paint(p, qtcore.QRectF{Width: float64(size.Width), Height: float64(size.Height)}, mode)
	p.End()
	return qtgui.QPixmapFromImage(img)
}

// Clone overrides QIconEngine::clone, which Qt calls where an icon is
// changed.
func (e *iconEngine) Clone() *qtgui.QIconEngine {
	return newIconEngine(e.name, e.data).QIconEngine
}

func (e *iconEngine) paint(p *qtgui.QPainter, rect qtcore.QRectF, mode qtgui.QIconMode) {
	p.SetRenderHint(qtgui.QPainterAntialiasing)
	svgRenderer(e.name, e.data, iconColour(mode)).RenderWithRectF(p, rect)
}

// iconColour returns the colour an icon is drawn in, in the mode, from the
// application's palette as it is.
func iconColour(mode qtgui.QIconMode) string {
	palette := qtgui.QGuiApplicationPalette()
	switch mode {
	case qtgui.QIconDisabled:
		return palette.GroupColor(qtgui.QPaletteDisabled, qtgui.QPaletteWindowText).Name()
	case qtgui.QIconSelected:
		return palette.GroupColor(qtgui.QPaletteActive, qtgui.QPaletteHighlightedText).Name()
	}
	return palette.GroupColor(qtgui.QPaletteActive, qtgui.QPaletteWindowText).Name()
}

// svgRenderers hold each icon's SVG read for each colour it has been drawn
// in, as reading it each time it is drawn would be slow.
var svgRenderers = map[string]*qtsvg.QSvgRenderer{}

// svgRenderer returns the renderer of the icon with the name, whose SVG is
// data, in the colour.
func svgRenderer(name string, data []byte, colour string) *qtsvg.QSvgRenderer {
	key := name + " " + colour
	r, ok := svgRenderers[key]
	if !ok {
		r = qtsvg.NewQSvgRendererWithBytes(bytes.ReplaceAll(data, []byte("currentColor"), []byte(colour)), nil)
		svgRenderers[key] = r
	}
	return r
}
