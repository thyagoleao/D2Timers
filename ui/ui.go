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
	ToggleTurboMode(enable bool) error
	IsTurboEnabled() bool
	SetTurboCheck(*widget.Check)
}

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

func NewTimerWidget(a App, t *timer.DotaTimer) *TimerWidget {
	w := &TimerWidget{DotaTimer: t}
	t.UI = w

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
				return
			}
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
			reply := make(chan error, 1)
			a.EnqueueCommand(control.Command{Type: control.CmdPause, Target: t, Reply: reply})
			select {
			case <-reply:
			case <-time.After(200 * time.Millisecond):
			}
		case timer.StatePaused:
			reply := make(chan error, 1)
			a.EnqueueCommand(control.Command{Type: control.CmdResume, Target: t, Reply: reply})
			select {
			case <-reply:
			case <-time.After(200 * time.Millisecond):
			}
		case timer.StateInactive:
			reply := make(chan error, 1)
			a.EnqueueCommand(control.Command{Type: control.CmdStart, Target: t, Mode: timer.ModeManual, Reply: reply})
			select {
			case <-reply:
			case <-time.After(200 * time.Millisecond):
			}
		case timer.StateUnconfigured:
			if t.Name == "Custom Timer" {
				w.customTimeEntry.SetText("")
				w.customContentContainer.Hide()
				w.customInputContainer.Show()
			}
		}

		if state != timer.StateUnconfigured {
			w.UpdateDisplay()
		}
		a.UpdateControlButtonState()
	}

	w.tappableContainer.OnTappedSecondary = func(e *fyne.PointEvent) {
		reply := make(chan error, 1)
		a.EnqueueCommand(control.Command{Type: control.CmdReset, Target: t, Reply: reply})
		select {
		case <-reply:
		case <-time.After(200 * time.Millisecond):
		}
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
		var displayDuration int = s.Normal_Auto_Initial
		return timer.FormatTime(displayDuration)
	}
	return "--:--"
}

func (tw *TimerWidget) UpdateDisplay() {
	s := tw.DotaTimer.GetSnapshot()
	fyne.Do(func() {
		var opacity float64 = 0.65

		switch s.State {
		case timer.StateActiveAuto, timer.StateActiveManual:
			opacity = 0.25
		case timer.StatePaused, timer.StateInactive, timer.StateUnconfigured:
			opacity = 0.65
		}

		timeStr := tw.getTimeDisplayStringFromSnapshot(s)

		alpha := uint8(opacity * 255)
		tw.colorFilterRect.FillColor = withAlpha(timer.BackgroundColor, alpha)
		tw.timeText.Text = timeStr

		tw.colorFilterRect.Refresh()
		tw.timeText.Refresh()
	})
}

func (tw *TimerWidget) ForceShowInitial() {
	tw.UpdateDisplay()
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

	aboutIcon := widget.NewIcon(theme.QuestionIcon())
	helpButton := NewTappableContainer(aboutIcon, func() {
		a.ShowInfoDialog(i18n.T("Help"), "assets/timers_help.yaml", fyne.NewSize(500, 400))
	}, nil)

	leftContent := container.NewVBox(
		layout.NewSpacer(),
		helpButton,
	)

	turboCheck := widget.NewCheck(i18n.T("Turbo"), nil)
	if a.IsTurboEnabled() {
		turboCheck.SetChecked(true)
	}
	turboCheck.OnChanged = func(checked bool) {
		if err := a.ToggleTurboMode(checked); err != nil {
			fyne.Do(func() {
				turboCheck.SetChecked(!checked)
			})
		}
		w.Canvas().Focus(nil)
	}
	a.SetTurboCheck(turboCheck)

	controlButtons := container.NewHBox(controlStack, buttonsSpacer, resetButton)

	centeredCheckbox := container.NewHBox(
		layout.NewSpacer(),
		turboCheck,
		layout.NewSpacer(),
	)

	centeredControlButtons := container.NewHBox(
		layout.NewSpacer(),
		controlButtons,
		layout.NewSpacer(),
	)

	vboxSpacer := canvas.NewRectangle(color.Transparent)
	vboxSpacer.SetMinSize(fyne.NewSize(0, 1))

	centralContentBlock := container.NewVBox(
		centeredCheckbox,
		vboxSpacer,
		centeredControlButtons,
	)

	centeredCentralContentBlock := container.New(layout.NewCenterLayout(), centralContentBlock)

	footer := container.New(
		layout.NewBorderLayout(nil, nil, leftContent, nil),
		leftContent,
		centeredCentralContentBlock,
	)

	return autoButton, startButton, stopButton, resetButton, footer
}

func CreateMainWindow(a App, fyneApp fyne.App, content embed.FS) fyne.Window {
	title := fyneApp.Metadata().Name
	if title == "" {
		title = "D2Timers"
	}
	w := fyneApp.NewWindow(title)

	listContainer := BuildTimersList(a)
	autoButton, startButton, stopButton, resetButton, footerLayout := BuildFooter(a, w)

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
	w.Resize(fyne.NewSize(timer.TimerWidth, 469))
	w.SetFixedSize(true)
	return w
}

type TappableContainer struct {
	widget.BaseWidget
	Content           fyne.CanvasObject
	OnTappedPrimary   func()
	OnTappedSecondary func(e *fyne.PointEvent)
}

func NewTappableContainer(c fyne.CanvasObject, onP func(), onS func(e *fyne.PointEvent)) *TappableContainer {
	t := &TappableContainer{
		Content:           c,
		OnTappedPrimary:   onP,
		OnTappedSecondary: onS,
	}
	t.ExtendBaseWidget(t)
	return t
}

func (t *TappableContainer) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(container.NewHBox(t.Content, layout.NewSpacer()))
}

func (t *TappableContainer) Tapped(_ *fyne.PointEvent) {
	if t.OnTappedPrimary != nil {
		t.OnTappedPrimary()
	}
}

func (t *TappableContainer) TappedSecondary(e *fyne.PointEvent) {
	if t.OnTappedSecondary != nil {
		t.OnTappedSecondary(e)
	}
}

func withAlpha(c color.Color, alpha uint8) color.NRGBA {
	r, g, b, _ := c.RGBA()
	return color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: alpha}
}
