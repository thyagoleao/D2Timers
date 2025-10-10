// Package ui contains the Fyne user interface for D2Timers. It wires the
// visual widgets to the timer domain via the App interface and uses a
// command-based approach to interact with the application manager.
//
// Maintenance tips:
//   - The UI must never mutate timer domain fields directly. Use the
//     `App.EnqueueCommand` mechanism to request state changes. The command
//     loop will apply changes and optionally reply on the provided Reply
//     channel to confirm completion.
//   - Use `fyne.Do` when updating UI objects from background goroutines.
//   - The `TimerWidget` implements `timer.TimerUI` and keeps a back reference
//     from the domain Timer (t.UI = widget) so the command loop or tick can
//     call `UpdateDisplay` safely. Avoid making UI assumptions inside the
//     timer package.
package ui

import (
	"D2Timers/control"
	"D2Timers/i18n"
	"D2Timers/timer"
	"embed"
	"fmt"
	"image/color"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// App defines the interface the UI needs to communicate with the main app.
type App interface {
	timer.App
	AllTimers() []*timer.DotaTimer
	UpdateControlButtonState()
	HandleKeyRune(rune)
	ShowInfoDialog(title, contentFile string, minSize fyne.Size)
	CreateBackgroundImage(string) fyne.CanvasObject
	SetAutoButton(*widget.Button)
	SetStartButton(*widget.Button)
	SetStopButton(*widget.Button)
	SetResetButton(*widget.Button)
	EnqueueCommand(cmd control.Command)
}

// TimerWidget holds the UI components for a single timer.
type TimerWidget struct {
	*timer.DotaTimer

	nameText               *canvas.Text
	timeText               *canvas.Text
	backgroundImage        fyne.CanvasObject
	colorFilterRect        *canvas.Rectangle
	borderRect             *canvas.Rectangle
	tappableContainer      *TappableContainer
	customContentContainer *fyne.Container
	customInputContainer   *fyne.Container
	customTimeEntry        *widget.Entry
	customSetButton        *widget.Button
}

// NewTimerWidget creates all the UI for a single timer.
func NewTimerWidget(a App, t *timer.DotaTimer) *TimerWidget {
	w := &TimerWidget{DotaTimer: t}
	t.UI = w // Link back to the UI widget from the logic struct

	w.nameText = canvas.NewText(t.Name, color.White)
	w.nameText.TextStyle.Bold = false
	w.nameText.TextSize = timer.FontSize

	w.timeText = canvas.NewText("--:--", color.White)
	w.timeText.TextStyle.Monospace = false
	w.timeText.TextStyle.Symbol = true
	w.timeText.TextSize = timer.FontSizeTime

	w.backgroundImage = a.CreateBackgroundImage(t.BackgroundImageName)

	w.colorFilterRect = canvas.NewRectangle(color.Transparent)
	w.colorFilterRect.CornerRadius = timer.CornerRadius

	w.customContentContainer = container.New(layout.NewVBoxLayout(),
		layout.NewSpacer(),
		container.New(layout.NewCenterLayout(), w.nameText),
		container.New(layout.NewCenterLayout(), w.timeText),
		layout.NewSpacer(),
	)

	w.borderRect = canvas.NewRectangle(color.Transparent)
	w.borderRect.SetMinSize(fyne.NewSize(timer.TimerWidth, timer.TimerHeight))
	w.borderRect.CornerRadius = timer.CornerRadius

	if t.Name == "Custom Timer" {
		w.customTimeEntry = widget.NewEntry()
		w.customTimeEntry.SetPlaceHolder(i18n.T("mm:ss or seconds"))
		w.customTimeEntry.Text = ""

		setCustomTime := func() {
			val, err := parseTime(w.customTimeEntry.Text)
			if err != nil {
				return // Optionally show error dialog
			}
			// Use thread-safe setter to avoid races with ticker/command loop.
			t.SetCustomDuration(val)

			w.customInputContainer.Hide()
			w.customContentContainer.Show()
			w.UpdateDisplay()
			a.UpdateControlButtonState()
		}

		w.customSetButton = widget.NewButton("Set", setCustomTime)
		w.customTimeEntry.OnSubmitted = func(text string) { setCustomTime() }

		sizeEnforcer := canvas.NewRectangle(color.Transparent)
		sizeEnforcer.SetMinSize(fyne.NewSize(timer.CustomInputWidth, 0))
		paddedEntry := container.New(layout.NewPaddedLayout(), w.customTimeEntry)
		inputWrapper := container.New(layout.NewStackLayout(), sizeEnforcer, paddedEntry)
		w.customInputContainer = container.New(layout.NewHBoxLayout(), inputWrapper, w.customSetButton)

		inputCentered := container.New(layout.NewVBoxLayout(), layout.NewSpacer(), container.New(layout.NewCenterLayout(), w.customInputContainer), layout.NewSpacer())
		w.customInputContainer.Hide()

		contentStack := container.NewStack(w.customContentContainer, inputCentered)
		w.tappableContainer = NewTappableContainer(container.NewStack(w.backgroundImage, w.colorFilterRect, contentStack, w.borderRect), nil, nil)
	} else {
		w.tappableContainer = NewTappableContainer(container.NewStack(w.backgroundImage, w.colorFilterRect, w.customContentContainer, w.borderRect), nil, nil)
	}

	w.tappableContainer.OnTappedPrimary = func() {
		state := t.GetState()
		switch state {
		case timer.StateActiveManual, timer.StateActiveAuto:
			// send pause command and wait for confirmation
			reply := make(chan error, 1)
			a.EnqueueCommand(control.Command{Type: control.CmdPause, Target: t, Reply: reply})
			select {
			case <-reply:
			case <-time.After(200 * time.Millisecond):
			}
		case timer.StatePaused:
			// send resume command and wait for confirmation
			reply := make(chan error, 1)
			a.EnqueueCommand(control.Command{Type: control.CmdResume, Target: t, Reply: reply})
			select {
			case <-reply:
			case <-time.After(200 * time.Millisecond):
			}
		case timer.StateInactive:
			// send start (manual) command and wait for confirmation
			reply := make(chan error, 1)
			a.EnqueueCommand(control.Command{Type: control.CmdStart, Target: t, Mode: timer.ModeManual, Reply: reply})
			select {
			case <-reply:
			case <-time.After(200 * time.Millisecond):
			}
		case timer.StateUnconfigured:
			if t.Name == "Custom Timer" {
				// Show the input and clear previous text. Do NOT call UpdateDisplay here
				// because UpdateDisplay hides the input for StateUnconfigured. This mirrors
				// the behavior from the old code.
				w.customTimeEntry.SetText("")
				w.customContentContainer.Hide()
				w.customInputContainer.Show()
			}
		}

		// Only refresh display for states other than the unconfigured custom timer
		if state != timer.StateUnconfigured {
			w.UpdateDisplay()
		}
		// It's safe to update control buttons in all cases
		a.UpdateControlButtonState()
	}

	w.tappableContainer.OnTappedSecondary = func(e *fyne.PointEvent) {
		// send reset command and wait for confirmation
		reply := make(chan error, 1)
		a.EnqueueCommand(control.Command{Type: control.CmdReset, Target: t, Reply: reply})
		select {
		case <-reply:
			// confirmed
		case <-time.After(200 * time.Millisecond):
			// timeout - still proceed to update UI
		}
		// Hide custom input when resetting, to match old behavior.
		if t.Name == "Custom Timer" {
			w.customInputContainer.Hide()
			w.customContentContainer.Show()
		}
		if t.UI != nil {
			t.UI.UpdateDisplay()
		}
		a.UpdateControlButtonState()
	}

	w.UpdateDisplay()
	return w
}

func (tw *TimerWidget) GetCanvasObject() fyne.CanvasObject {
	return tw.tappableContainer
}

func parseTime(input string) (int, error) {
	var val int
	var err error
	if strings.Contains(input, ":") {
		parts := strings.Split(input, ":")
		if len(parts) == 2 {
			var min, sec int
			min, err = strconv.Atoi(parts[0])
			if err == nil {
				sec, err = strconv.Atoi(parts[1])
				if err == nil && sec >= 0 && sec < 60 {
					val = min*60 + sec
				} else {
					err = fmt.Errorf("invalid seconds (must be 0-59)")
				}
			}
		} else {
			err = fmt.Errorf("invalid time format")
		}
	} else {
		val, err = strconv.Atoi(input)
	}

	if err != nil || val <= 0 || val > 1800 {
		return 0, fmt.Errorf("invalid value")
	}
	return val, nil
}

func (tw *TimerWidget) getTimeDisplayStringFromSnapshot(s timer.TimerSnapshot) string {
	switch s.State {
	case timer.StateActiveAuto, timer.StateActiveManual, timer.StatePaused:
		return timer.FormatTime(s.Remaining)
	case timer.StateUnconfigured:
		if s.Name == "Custom Timer" && s.CustomDurationSec == 0 {
			return "+"
		}
		fallthrough
	case timer.StateInactive:
		if s.Name == "Custom Timer" {
			return timer.FormatTime(s.CustomDurationSec)
		}
		var displayDuration int
		if s.Mode == timer.ModeAuto {
			displayDuration = s.AutoInitial
		} else {
			displayDuration = s.ManualInitial
		}
		return timer.FormatTime(displayDuration)
	}
	return "--:--"
}

func (tw *TimerWidget) UpdateDisplay() {
	// Capture an atomic snapshot of the timer state to avoid races and
	// inconsistent UI updates while other goroutines may mutate the timer.
	s := tw.DotaTimer.GetSnapshot()
	fyne.Do(func() {
		// Default to the 'inactive/paused/unconfigured' look used in the old UI.
		var opacity float64 = 0.65

		switch s.State {
		case timer.StateActiveAuto, timer.StateActiveManual:
			// active timers have a darker overlay
			opacity = 0.25
		case timer.StatePaused, timer.StateInactive, timer.StateUnconfigured:
			// paused/inactive/unconfigured share the same lighter overlay
			opacity = 0.65
			// Do not change visibility of custom input here; visibility is controlled
			// explicitly by user actions (OnTappedPrimary, Set button, Reset).
		}

		timeStr := tw.getTimeDisplayStringFromSnapshot(s)

		alpha := uint8(opacity * 255)
		tw.colorFilterRect.FillColor = withAlpha(timer.BackgroundColor, alpha)
		tw.timeText.Text = timeStr

		tw.colorFilterRect.Refresh()
		tw.timeText.Refresh()
	})
}

func BuildTimersList(a App) *fyne.Container {
	listContainer := container.NewVBox()
	for _, t := range a.AllTimers() {
		if t.UI != nil {
			listContainer.Add(t.UI.GetCanvasObject())
		}
		spacer := canvas.NewRectangle(color.Transparent)
		spacer.SetMinSize(fyne.NewSize(0, timer.TimerSpacing))
		listContainer.Add(spacer)
	}
	return listContainer
}

func BuildFooter(a App, w fyne.Window) (*widget.Button, *widget.Button, *widget.Button, *widget.Button, fyne.CanvasObject) {
	autoButton := widget.NewButton("Auto", func() {
		// Enqueue start(auto) for all timers and wait for confirmations so the
		// control buttons reflect the new state immediately (behaviour from old UI).
		var replies []chan error
		for _, t := range a.AllTimers() {
			reply := make(chan error, 1)
			a.EnqueueCommand(control.Command{Type: control.CmdStart, Target: t, Mode: timer.ModeAuto, Reply: reply})
			replies = append(replies, reply)
		}
		for _, r := range replies {
			select {
			case <-r:
			case <-time.After(200 * time.Millisecond):
			}
		}
		for _, t := range a.AllTimers() {
			if t.UI != nil {
				t.UI.UpdateDisplay()
			}
		}
		a.UpdateControlButtonState()
	})

	stopButton := widget.NewButton(i18n.T("Stop"), func() {
		var replies []chan error
		for _, t := range a.AllTimers() {
			reply := make(chan error, 1)
			a.EnqueueCommand(control.Command{Type: control.CmdPause, Target: t, Reply: reply})
			replies = append(replies, reply)
		}
		for _, r := range replies {
			select {
			case <-r:
			case <-time.After(200 * time.Millisecond):
			}
		}
		for _, t := range a.AllTimers() {
			if t.UI != nil {
				t.UI.UpdateDisplay()
			}
		}
		a.UpdateControlButtonState()
	})
	stopButton.Hide()

	startButton := widget.NewButton(i18n.T("Start"), func() {
		var replies []chan error
		for _, t := range a.AllTimers() {
			reply := make(chan error, 1)
			a.EnqueueCommand(control.Command{Type: control.CmdResume, Target: t, Reply: reply})
			replies = append(replies, reply)
		}
		for _, r := range replies {
			select {
			case <-r:
			case <-time.After(200 * time.Millisecond):
			}
		}
		for _, t := range a.AllTimers() {
			if t.UI != nil {
				t.UI.UpdateDisplay()
			}
		}
		a.UpdateControlButtonState()
	})
	startButton.Hide()

	resetButton := widget.NewButton(i18n.T("Reset"), func() {
		var replies []chan error
		for _, t := range a.AllTimers() {
			reply := make(chan error, 1)
			a.EnqueueCommand(control.Command{Type: control.CmdReset, Target: t, Reply: reply})
			replies = append(replies, reply)
		}
		for _, r := range replies {
			select {
			case <-r:
			case <-time.After(200 * time.Millisecond):
			}
		}
		for _, t := range a.AllTimers() {
			if t.UI != nil {
				t.UI.UpdateDisplay()
			}
		}
		a.UpdateControlButtonState()
	})

	controlStack := container.NewStack(autoButton, stopButton, startButton)
	buttonsSpacer := canvas.NewRectangle(color.Transparent)
	buttonsSpacer.SetMinSize(fyne.NewSize(timer.ControlButtonsGap, 0))
	centerControlContainer := container.NewHBox(controlStack, buttonsSpacer, resetButton)

	aboutIcon := widget.NewIcon(theme.QuestionIcon())
	aboutButton := NewTappableContainer(aboutIcon, func() {
		a.ShowInfoDialog(i18n.T("About D2Timers"), "assets/dialogue_about(eng).txt", fyne.NewSize(500, 400))
	}, nil)

	iconButtonsContainer := container.New(layout.NewBorderLayout(nil, nil, aboutButton, nil), aboutButton)

	topIconSpacer := canvas.NewRectangle(color.Transparent)
	topIconSpacer.SetMinSize(fyne.NewSize(0, 12))
	paddedIconContainer := container.NewVBox(topIconSpacer, iconButtonsContainer)

	footer := container.NewStack(paddedIconContainer, container.NewCenter(centerControlContainer))
	return autoButton, startButton, stopButton, resetButton, footer
}

func CreateMainWindow(a App, fyneApp fyne.App, content embed.FS) fyne.Window {
	title := fyneApp.Metadata().Name
	if title == "" {
		title = "D2Timers"
	}
	w := fyneApp.NewWindow(title)
	// a.mainWindow = w // Cannot set mainWindow here, as App is an interface

	listContainer := BuildTimersList(a)
	autoButton, startButton, stopButton, resetButton, footerLayout := BuildFooter(a, w)

	// Set buttons in AppManager
	a.SetAutoButton(autoButton)
	a.SetStartButton(startButton)
	a.SetStopButton(stopButton)
	a.SetResetButton(resetButton)

	w.Canvas().SetOnTypedRune(a.HandleKeyRune)

	bottomSpacer := canvas.NewRectangle(color.Transparent)
	bottomSpacer.SetMinSize(fyne.NewSize(0, timer.GapButton))

	contentVBox := container.NewVBox(
		listContainer,
		bottomSpacer,
		footerLayout,
	)

	a.UpdateControlButtonState()

	w.SetContent(contentVBox)
	// Use a single Resize with the intended width (TimerWidth) and a reasonable height.
	w.Resize(fyne.NewSize(timer.TimerWidth, 469))
	w.SetFixedSize(true)
	return w
}

// TappableContainer is a wrapper for canvas objects to handle primary and secondary taps.
type TappableContainer struct {
	widget.BaseWidget
	Content           fyne.CanvasObject
	OnTappedPrimary   func()
	OnTappedSecondary func(e *fyne.PointEvent)
}

// NewTappableContainer creates a new TappableContainer.
func NewTappableContainer(c fyne.CanvasObject, onP func(), onS func(e *fyne.PointEvent)) *TappableContainer {
	t := &TappableContainer{
		Content:           c,
		OnTappedPrimary:   onP,
		OnTappedSecondary: onS,
	}
	t.ExtendBaseWidget(t)
	return t
}

// CreateRenderer is a standard Fyne method.
func (t *TappableContainer) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(t.Content)
}

// Tapped is a standard Fyne method.
func (t *TappableContainer) Tapped(_ *fyne.PointEvent) {
	if t.OnTappedPrimary != nil {
		t.OnTappedPrimary()
	}
}

// TappedSecondary is a standard Fyne method.
func (t *TappableContainer) TappedSecondary(e *fyne.PointEvent) {
	if t.OnTappedSecondary != nil {
		t.OnTappedSecondary(e)
	}
}

func withAlpha(c color.Color, alpha uint8) color.NRGBA {
	r, g, b, _ := c.RGBA()
	return color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: alpha}
}
