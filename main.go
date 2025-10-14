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

	mediumFontData, _ := content.ReadFile("assets/Quicksand-Medium.ttf")
	boldFontData, _ := content.ReadFile("assets/Quicksand-Bold.ttf")
	mediumFontRes := fyne.NewStaticResource("Quicksand-Medium.ttf", mediumFontData)
	boldFontRes := fyne.NewStaticResource("Quicksand-Bold.ttf", boldFontData)

	fyneApp.Settings().SetTheme(ui.NewCustomTheme(mediumFontRes, boldFontRes))

	a := NewAppManager(content)

	w := ui.CreateMainWindow(a, fyneApp, content)
	a.mainWindow = w

	ctx, cancel := context.WithCancel(context.Background())
	w.SetOnClosed(func() {
		cancel()
	})

	go a.tick(ctx)

	w.ShowAndRun()
}
