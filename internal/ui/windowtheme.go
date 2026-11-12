package ui

import (
	"github.com/nomad-software/qute/qtgui"
	"github.com/nomad-software/qute/qtwidgets"
)

// systemStyle is the desktop's widget style, captured before any window
// theme replaces it.
var systemStyle string

// windowPalettes are the colours of the light and dark window themes.
// Disabled text uses windowDisabledText.
var windowPalettes = map[string]map[qtgui.QPaletteColorRole]string{
	"light": {
		qtgui.QPaletteWindow: "#efefef", qtgui.QPaletteWindowText: "#1e1e1e",
		qtgui.QPaletteBase: "#ffffff", qtgui.QPaletteAlternateBase: "#f5f5f5",
		qtgui.QPaletteText: "#1e1e1e", qtgui.QPalettePlaceholderText: "#808080",
		qtgui.QPaletteButton: "#e8e8e8", qtgui.QPaletteButtonText: "#1e1e1e",
		qtgui.QPaletteBrightText:  "#ffffff",
		qtgui.QPaletteToolTipBase: "#ffffe1", qtgui.QPaletteToolTipText: "#1e1e1e",
		qtgui.QPaletteHighlight: "#3375c4", qtgui.QPaletteHighlightedText: "#ffffff",
		qtgui.QPaletteLink: "#1a5fb4", qtgui.QPaletteLinkVisited: "#813d9c",
		qtgui.QPaletteLight: "#ffffff", qtgui.QPaletteMidlight: "#dddddd",
		qtgui.QPaletteMid: "#b8b8b8", qtgui.QPaletteDark: "#a0a0a0", qtgui.QPaletteShadow: "#6e6e6e",
	},
	"dark": {
		qtgui.QPaletteWindow: "#2a2a2a", qtgui.QPaletteWindowText: "#e6e6e6",
		qtgui.QPaletteBase: "#1e1e1e", qtgui.QPaletteAlternateBase: "#262626",
		qtgui.QPaletteText: "#e6e6e6", qtgui.QPalettePlaceholderText: "#8c8c8c",
		qtgui.QPaletteButton: "#353535", qtgui.QPaletteButtonText: "#e6e6e6",
		qtgui.QPaletteBrightText:  "#ff6b6b",
		qtgui.QPaletteToolTipBase: "#353535", qtgui.QPaletteToolTipText: "#e6e6e6",
		qtgui.QPaletteHighlight: "#3070b8", qtgui.QPaletteHighlightedText: "#ffffff",
		qtgui.QPaletteLink: "#78aeff", qtgui.QPaletteLinkVisited: "#b894ff",
		qtgui.QPaletteLight: "#4a4a4a", qtgui.QPaletteMidlight: "#3d3d3d",
		qtgui.QPaletteMid: "#262626", qtgui.QPaletteDark: "#1a1a1a", qtgui.QPaletteShadow: "#101010",
	},
}

var windowDisabledText = map[string]string{"light": "#a0a0a0", "dark": "#7a7a7a"}

// applyWindowTheme sets the look of menus, dialogs and tabs: "light" and
// "dark" use the Fusion style with termite's palettes, "system" the desktop's
// style and palette.
func applyWindowTheme(mode string) {
	if systemStyle == "" {
		systemStyle = qtwidgets.QApplicationStyle().Name()
	}
	colours, ok := windowPalettes[mode]
	if !ok {
		qtwidgets.QApplicationSetStyleName(systemStyle)
		qtwidgets.QApplicationSetPalette(qtgui.NewQPalette()) // empty: Qt uses the desktop's
		return
	}
	qtwidgets.QApplicationSetStyleName("Fusion")
	pal := qtgui.NewQPalette()
	for role, hex := range colours {
		pal.SetColor(role, mustColor(hex))
	}
	off := mustColor(windowDisabledText[mode])
	for _, role := range []qtgui.QPaletteColorRole{qtgui.QPaletteWindowText, qtgui.QPaletteText, qtgui.QPaletteButtonText} {
		pal.SetGroupColor(qtgui.QPaletteDisabled, role, off)
	}
	qtwidgets.QApplicationSetPalette(pal)
}

// mustColor parses a "#rrggbb" colour termite itself spells.
func mustColor(hex string) qtgui.QColor {
	c, err := qtgui.ParseColor(hex)
	if err != nil {
		panic(err)
	}
	return c
}
