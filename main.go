// Package main wires the application: it initializes Fyne, sets the theme,
// creates the AppManager and the main window, and starts the ticker loop.
//
// Maintenance tips:
//   - The embedded `content` FS contains the assets used by the app. When
//     adding assets, include them in the `//go:embed` directive above.
//   - The ticker context is canceled on window close to ensure goroutines
//     exit cleanly.
package main

import (
	"D2Timers/ui"
	"context"
	"embed"
	"log"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
)

//go:embed assets/*
var content embed.FS

func main() {
	fyneApp := app.New()

	if iconBytes, err := content.ReadFile("assets/icon.png"); err == nil {
		fyneApp.SetIcon(fyne.NewStaticResource("icon.png", iconBytes))
	} else {
		log.Printf("Failed to load icon. %v", err)
	}

	// Load fonts for custom theme
	mediumFontData, _ := content.ReadFile("assets/Quicksand-Medium.ttf")
	boldFontData, _ := content.ReadFile("assets/Quicksand-Bold.ttf")
	mediumFontRes := fyne.NewStaticResource("Quicksand-Medium.ttf", mediumFontData)
	boldFontRes := fyne.NewStaticResource("Quicksand-Bold.ttf", boldFontData)

	fyneApp.Settings().SetTheme(ui.NewCustomTheme(mediumFontRes, boldFontRes))

	a := NewAppManager(content) // Pass embed.FS content to AppManager

	w := ui.CreateMainWindow(a, fyneApp, content)
	a.mainWindow = w // Assign created window back to AppManager

	// Setup context for ticker management
	ctx, cancel := context.WithCancel(context.Background())
	w.SetOnClosed(func() {
		cancel() // Cancel context when window is closed
	})

	go a.tick(ctx) // Pass context to the tick goroutine

	w.ShowAndRun()
}
