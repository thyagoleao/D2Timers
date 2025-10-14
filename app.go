package main

import (
	"D2Timers/control"
	"D2Timers/i18n"
	"D2Timers/timer"
	"D2Timers/ui"
	"context"
	"embed"
	"fmt"
	"image/color"
	"log"
	"sort"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/gopxl/beep"
	"github.com/gopxl/beep/speaker"
	"github.com/gopxl/beep/vorbis"
)

type AppManager struct {
	mainWindow   fyne.Window
	allTimers    []*timer.DotaTimer
	activeTimers []*timer.DotaTimer
	activeLock   sync.Mutex
	cmdCh        chan control.Command
	cmdCtx       context.Context
	cmdCancel    context.CancelFunc

	autoButton  *widget.Button
	stopButton  *widget.Button
	startButton *widget.Button
	resetButton *widget.Button

	audioBuffers map[string]*beep.Buffer
	speakerLock  sync.Mutex
	content      embed.FS

	turboMode bool

	turboCheck *widget.Check
}

func NewAppManager(content embed.FS) *AppManager {
	a := &AppManager{audioBuffers: make(map[string]*beep.Buffer), content: content}
	timer.LoadTimerConfigs(content)
	log.Printf("Loaded %d timer configs.", len(timer.TimerConfigs))
	a.loadAudioFiles()
	a.cmdCh = make(chan control.Command, 256)
	a.cmdCtx, a.cmdCancel = context.WithCancel(context.Background())
	go a.commandLoop()

	for _, cfg := range timer.TimerConfigs {
		t := timer.NewDotaTimer(cfg)
		ui.NewTimerWidget(a, t)
		a.allTimers = append(a.allTimers, t)
	}

	return a
}

func (a *AppManager) EnqueueCommand(cmd control.Command) {
	select {
	case a.cmdCh <- cmd:
	case <-time.After(150 * time.Millisecond):
		log.Printf("EnqueueCommand timeout: dropping command")
	}
}

func (a *AppManager) commandLoop() {
	for {
		select {
		case <-a.cmdCtx.Done():
			return
		case cmd := <-a.cmdCh:
			t := cmd.Target
			if t != nil {
				switch cmd.Type {
				case control.CmdStart:
					t.Start(a, cmd.Mode)
				case control.CmdPause:
					t.Pause(a)
				case control.CmdResume:
					t.Resume(a)
				case control.CmdReset:
					t.Reset(a)
				}
			}
			if cmd.Reply != nil {
				select {
				case cmd.Reply <- nil:
				default:
				}
			}
		}
	}
}

func (a *AppManager) AllTimers() []*timer.DotaTimer {
	return a.allTimers
}

func (a *AppManager) AddActiveTimer(t *timer.DotaTimer) {
	a.activeLock.Lock()
	defer a.activeLock.Unlock()
	for _, at := range a.activeTimers {
		if at == t {
			return
		}
	}
	a.activeTimers = append(a.activeTimers, t)
}

func (a *AppManager) RemoveActiveTimer(t *timer.DotaTimer) {
	a.activeLock.Lock()
	defer a.activeLock.Unlock()
	for i, at := range a.activeTimers {
		if at == t {
			a.activeTimers = append(a.activeTimers[:i], a.activeTimers[i+1:]...)
			return
		}
	}
}

func (a *AppManager) loadAudioFiles() {
	if err := speaker.Init(44100, 44100/10); err != nil {
		log.Printf("Audio disabled: Failed to initialize speaker: %v\n", err)
	}

	audioFiles := []string{
		"audio_timer1.ogg",
		"audio_timer2.ogg",
		"audio_timer3.ogg",
		"audio_timer4.ogg",
	}

	for _, filename := range audioFiles {
		if _, ok := a.audioBuffers[filename]; ok {
			continue
		}

		filepath := fmt.Sprintf("assets/%s", filename)
		log.Printf("Attempting to open audio file: %s", filepath)
		data, err := a.content.Open(filepath)
		if err != nil {
			log.Printf("Failed to open audio %s: %v", filepath, err)
			continue
		}
		log.Printf("Successfully opened audio file: %s", filepath)

		streamer, format, err := vorbis.Decode(data)
		if err != nil {
			log.Printf("Failed to decode audio %s: %v", filepath, err)
			data.Close()
			continue
		}
		log.Printf("Successfully decoded audio file: %s", filepath)

		buffer := beep.NewBuffer(format)
		buffer.Append(streamer)
		a.audioBuffers[filename] = buffer

		streamer.Close()
		data.Close()
	}
}

func (a *AppManager) CreateBackgroundImage(filename string) fyne.CanvasObject {
	if filename == "" {
		return canvas.NewRectangle(color.Transparent)
	}

	filepath := fmt.Sprintf("assets/%s", filename)
	data, err := a.content.ReadFile(filepath)
	if err != nil {
		log.Printf("Failed to load image %s: %v\n", filename, err)
		return canvas.NewRectangle(color.Transparent)
	}

	res := fyne.NewStaticResource(filename, data)
	img := canvas.NewImageFromResource(res)
	img.FillMode = canvas.ImageFillStretch

	return img
}

func (a *AppManager) PlaySound(filename string) {
	b, ok := a.audioBuffers[filename]
	if !ok {
		log.Printf("Sound buffer not found for %s", filename)
		return
	}

	a.speakerLock.Lock()
	defer a.speakerLock.Unlock()

	speaker.Play(b.Streamer(0, b.Len()))
}

func (a *AppManager) UpdateControlButtonState() {
	isAnyActive := false
	isAnyPaused := false
	isAnyUnconfigured := false

	for _, t := range a.allTimers {
		switch t.State {
		case timer.StateActiveAuto, timer.StateActiveManual:
			isAnyActive = true
		case timer.StatePaused:
			isAnyPaused = true
		case timer.StateUnconfigured:
			isAnyUnconfigured = true
		}
	}

	fyne.Do(func() {
		if a.autoButton != nil {
			if isAnyActive {
				a.autoButton.Hide()
				a.stopButton.Show()
				a.startButton.Hide()
			} else if isAnyPaused {
				a.autoButton.Hide()
				a.stopButton.Hide()
				a.startButton.Show()
			} else {
				a.autoButton.Show()
				a.stopButton.Hide()
				a.startButton.Hide()
			}

			if isAnyUnconfigured && !isAnyActive && !isAnyPaused {
				allAreUnconfiguredOrInactive := true
				for _, t := range a.allTimers {
					if t.State != timer.StateUnconfigured && t.State != timer.StateInactive {
						allAreUnconfiguredOrInactive = false
						break
					}
				}
				if allAreUnconfiguredOrInactive {
					a.resetButton.Disable()
				} else {
					a.resetButton.Enable()
				}
			} else {
				a.resetButton.Enable()
			}

			a.resetButton.Refresh()
			a.autoButton.Refresh()
			a.stopButton.Refresh()
			a.startButton.Refresh()

			allInitial := true
			for _, t := range a.allTimers {
				if t.GetState() != timer.StateInactive && t.GetState() != timer.StateUnconfigured {
					allInitial = false
					break
				}
			}

			if a.turboCheck != nil {
				fyne.Do(func() {
					a.turboCheck.SetChecked(a.turboMode)
					if allInitial {
						a.turboCheck.Enable()
					} else {
						a.turboCheck.Disable()
					}
				})
			}
		}
	})
}

func (a *AppManager) SetTurboCheck(c *widget.Check) {
	a.turboCheck = c
}

func (a *AppManager) IsTurboEnabled() bool {
	return a.turboMode
}

func (a *AppManager) ToggleTurboMode(enable bool) error {
	if enable {
		for _, t := range a.allTimers {
			st := t.GetState()
			if st != timer.StateInactive && st != timer.StateUnconfigured {
				fyne.Do(func() {
					dialog.ShowInformation(i18n.T("Turbo Mode"), i18n.T("Turbo Mode can only be enabled when all timers are in their initial state."), a.mainWindow)
				})
				return fmt.Errorf("cannot enable turbo: not all timers in initial state")
			}
		}
	}

	for _, t := range a.allTimers {
		oNormal_Auto_Initial, oNormal_Auto_Repeat, oTurbo_Auto_Initial, oTurbo_Auto_Repeat, oNormal_Manual_Initial, oNormal_Manual_Repeat, oTurbo_Manual_Initial, oTurbo_Manual_Repeat := t.GetOriginals()
		if enable {
			if oTurbo_Auto_Initial != 0 {
				t.SetNormal_Auto_InitialRepeat(oTurbo_Auto_Initial, oTurbo_Auto_Repeat)
			}
			if oTurbo_Manual_Initial != 0 {
				t.SetNormal_Manual_InitialRepeat(oTurbo_Manual_Initial, oTurbo_Manual_Repeat)
			}
		} else {
			t.SetNormal_Auto_InitialRepeat(oNormal_Auto_Initial, oNormal_Auto_Repeat)
			t.SetNormal_Manual_InitialRepeat(oNormal_Manual_Initial, oNormal_Manual_Repeat)
		}
	}

	a.turboMode = enable

	fyne.Do(func() {
		for _, t := range a.allTimers {
			if t.UI != nil {
				t.UI.UpdateDisplay()
			}
		}
	})

	return nil
}

func (a *AppManager) tick(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			timersToAlert := make([]*timer.DotaTimer, 0)

			a.activeLock.Lock()
			activeCopy := make([]*timer.DotaTimer, len(a.activeTimers))
			copy(activeCopy, a.activeTimers)
			a.activeLock.Unlock()

			for _, t := range activeCopy {
				t.Tick(a)
				if t.Remaining <= 0 {
					timersToAlert = append(timersToAlert, t)
				}
			}

			if len(timersToAlert) > 0 {
				sort.Slice(timersToAlert, func(i, j int) bool {
					return timersToAlert[i].Priority > timersToAlert[j].Priority
				})

				timersToAlert[0].Alert(a)
			}

			for _, t := range a.allTimers {
				if t.UI != nil {
					t.UI.UpdateDisplay()
				}
			}
		}
	}
}

func (a *AppManager) HandleKeyRune(r rune) {
	var index int = -1

	switch r {
	case ' ':
		if !a.stopButton.Hidden {
			a.stopButton.Tapped(&fyne.PointEvent{})
		} else if !a.startButton.Hidden {
			a.startButton.Tapped(&fyne.PointEvent{})
		} else if !a.autoButton.Hidden {
			a.autoButton.Tapped(&fyne.PointEvent{})
		}
	case 't', 'T':
		if a.turboCheck != nil {
			a.turboCheck.SetChecked(!a.turboCheck.Checked)
		}
	case 'r', 'R':
		a.resetButton.Tapped(&fyne.PointEvent{})
	case 'z', 'Z':
		index = int(timer.TimerIndexStackNeutrals)
	case 'x', 'X':
		index = int(timer.TimerIndexPowerRunes)
	case 'c', 'C':
		index = int(timer.TimerIndexShrinesOfWisdom)
	case 'v', 'V':
		index = int(timer.TimerIndexCustomTimer)
	}

	if index >= 0 && index < len(a.allTimers) {
		t := a.allTimers[index]
		if uw, ok := t.UI.(*ui.TimerWidget); ok {
			uw.GetCanvasObject().(*ui.TappableContainer).Tapped(&fyne.PointEvent{})
		}
	}
}

func (a *AppManager) ShowInfoDialog(title, contentFile string, minSize fyne.Size) {
	var contentText string
	if contentFile == "assets/timers_help.yaml" {
		bytes, err := a.content.ReadFile("assets/timers_help.yaml")
		if err != nil {
			dialog.ShowError(err, a.mainWindow)
			return
		}

		var dialogues map[string]string
		if err := yaml.Unmarshal(bytes, &dialogues); err != nil {
			dialog.ShowError(err, a.mainWindow)
			return
		}
		contentText = dialogues[i18n.GetLang()]
	} else {
		bytes, err := a.content.ReadFile(contentFile)
		if err != nil {
			dialog.ShowError(err, a.mainWindow)
			return
		}
		contentText = string(bytes)
	}

	text := widget.NewLabel(contentText)
	text.Wrapping = fyne.TextWrapWord

	scrollableContent := container.NewVScroll(text)
	scrollableContent.SetMinSize(minSize)

	dialog.ShowCustom(title, i18n.T("Close"), scrollableContent, a.mainWindow)
}

func (a *AppManager) SetAutoButton(btn *widget.Button) {
	a.autoButton = btn
}

func (a *AppManager) SetStartButton(btn *widget.Button) {
	a.startButton = btn
}

func (a *AppManager) SetStopButton(btn *widget.Button) {
	a.stopButton = btn
}

func (a *AppManager) SetResetButton(btn *widget.Button) {
	a.resetButton = btn
}

func (a *AppManager) Shutdown() {
	if a.cmdCancel != nil {
		a.cmdCancel()
	}
}
