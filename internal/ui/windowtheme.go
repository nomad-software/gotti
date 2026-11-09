package ui

import (
	qt "github.com/mappu/miqt/qt6"
)

// systemStyle is the desktop's widget style, captured before any window
// theme replaces it.
var systemStyle string

// windowPalettes are the colours of the light and dark window themes.
// Disabled text uses windowDisabledText.
var windowPalettes = map[string]map[qt.QPalette__ColorRole]string{
	"light": {
		qt.QPalette__Window: "#efefef", qt.QPalette__WindowText: "#1e1e1e",
		qt.QPalette__Base: "#ffffff", qt.QPalette__AlternateBase: "#f5f5f5",
		qt.QPalette__Text: "#1e1e1e", qt.QPalette__PlaceholderText: "#808080",
		qt.QPalette__Button: "#e8e8e8", qt.QPalette__ButtonText: "#1e1e1e",
		qt.QPalette__BrightText:  "#ffffff",
		qt.QPalette__ToolTipBase: "#ffffe1", qt.QPalette__ToolTipText: "#1e1e1e",
		qt.QPalette__Highlight: "#3375c4", qt.QPalette__HighlightedText: "#ffffff",
		qt.QPalette__Link: "#1a5fb4", qt.QPalette__LinkVisited: "#813d9c",
		qt.QPalette__Light: "#ffffff", qt.QPalette__Midlight: "#dddddd",
		qt.QPalette__Mid: "#b8b8b8", qt.QPalette__Dark: "#a0a0a0", qt.QPalette__Shadow: "#6e6e6e",
	},
	"dark": {
		qt.QPalette__Window: "#2a2a2a", qt.QPalette__WindowText: "#e6e6e6",
		qt.QPalette__Base: "#1e1e1e", qt.QPalette__AlternateBase: "#262626",
		qt.QPalette__Text: "#e6e6e6", qt.QPalette__PlaceholderText: "#8c8c8c",
		qt.QPalette__Button: "#353535", qt.QPalette__ButtonText: "#e6e6e6",
		qt.QPalette__BrightText:  "#ff6b6b",
		qt.QPalette__ToolTipBase: "#353535", qt.QPalette__ToolTipText: "#e6e6e6",
		qt.QPalette__Highlight: "#3070b8", qt.QPalette__HighlightedText: "#ffffff",
		qt.QPalette__Link: "#78aeff", qt.QPalette__LinkVisited: "#b894ff",
		qt.QPalette__Light: "#4a4a4a", qt.QPalette__Midlight: "#3d3d3d",
		qt.QPalette__Mid: "#262626", qt.QPalette__Dark: "#1a1a1a", qt.QPalette__Shadow: "#101010",
	},
}

var windowDisabledText = map[string]string{"light": "#a0a0a0", "dark": "#7a7a7a"}

// applyWindowTheme sets the look of menus, dialogs and tabs: "light" and
// "dark" use the Fusion style with gotti's palettes, "system" the desktop's
// style and palette.
func applyWindowTheme(mode string) {
	if systemStyle == "" {
		systemStyle = qt.QApplication_Style().Name()
	}
	colours, ok := windowPalettes[mode]
	if !ok {
		qt.QApplication_SetStyleWithStyle(systemStyle)
		pal := qt.NewQPalette() // empty: Qt uses the desktop's
		qt.QApplication_SetPalette(pal)
		pal.Delete()
		return
	}
	qt.QApplication_SetStyleWithStyle("Fusion")
	pal := qt.NewQPalette()
	for role, hex := range colours {
		c := qt.NewQColor6(hex)
		pal.SetColor2(role, c)
		c.Delete()
	}
	off := qt.NewQColor6(windowDisabledText[mode])
	for _, role := range []qt.QPalette__ColorRole{qt.QPalette__WindowText, qt.QPalette__Text, qt.QPalette__ButtonText} {
		pal.SetColor(qt.QPalette__Disabled, role, off)
	}
	off.Delete()
	qt.QApplication_SetPalette(pal)
	pal.Delete()
}
