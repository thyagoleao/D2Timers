package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// CustomTheme defines the custom font theme for the application.
type CustomTheme struct {
	fyne.Theme
	medium fyne.Resource
	bold   fyne.Resource
}

// NewCustomTheme creates a new instance of the custom theme.
func NewCustomTheme(mediumFont, boldFont fyne.Resource) fyne.Theme {
	return &CustomTheme{Theme: theme.DefaultTheme(), medium: mediumFont, bold: boldFont}
}

// Font returns the font for the given style.
func (t *CustomTheme) Font(style fyne.TextStyle) fyne.Resource {
	if style.Bold {
		return t.bold
	}
	// If not bold, or if symbol, use medium font
	return t.medium
}
