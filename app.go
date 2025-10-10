// Package main contains the application wiring and the AppManager which
// coordinates timers, audio and the UI. This file centralizes the shared
// application state and the command loop used to serialize timer state
// mutations.
//
// Maintenance notes / tips:
//   - Concurrency model: the application uses a single command-loop goroutine
//     (see `commandLoop`) to serialize Start/Pause/Resume/Reset operations.
//     Timers are also ticked in a separate goroutine (`tick`). Because timer
//     state (e.g. Remaining, State) can be mutated by both the commandLoop
//     and the tick goroutine, take care to avoid data races if you change
//     the model. The preferred approaches are:
//   - Add a mutex to the DotaTimer struct and protect mutable fields, or
//   - Move all mutations (including Tick) into the commandLoop goroutine.
//   - `cmdCh` is a buffered channel used to enqueue commands from the UI. The
//     current implementation drops commands when the channel is full to avoid
//     blocking the UI. If you need stronger guarantees (no dropped commands),
//     consider increasing the buffer size, switching to a blocking-with-timeout
//     send, or adding backpressure to the UI.
//   - `allTimers` is populated during startup and is treated as immutable after
//     `NewAppManager` returns. If you ever modify `allTimers` at runtime,
//     protect it with a mutex or switch to an immutable copy-on-write pattern.
package main

import (
	"D2Timers/control"
	"D2Timers/i18n"
	"D2Timers/timer"
	"D2Timers/ui"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"image/color"
	"log"
	"sort"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/gopxl/beep"
	"github.com/gopxl/beep/speaker"
	"github.com/gopxl/beep/vorbis"
)

// AppManager is the main application struct, holding all state.
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
	content      embed.FS // Embedded file system for assets
}

// NewAppManager creates a new application manager.
func NewAppManager(content embed.FS) *AppManager {
	a := &AppManager{audioBuffers: make(map[string]*beep.Buffer), content: content}
	timer.LoadTimerConfigs(content)
	log.Printf("Loaded %d timer configs.", len(timer.TimerConfigs))
	a.loadAudioFiles()

	// Use a larger buffer for the command channel to reduce drops under brief bursts.
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

// EnqueueCommand posts a command to the internal command loop.
func (a *AppManager) EnqueueCommand(cmd control.Command) {
	// Try to enqueue the command but avoid blocking UI indefinitely. If the
	// channel stays full for the configured short timeout, drop and log.
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
			// handle known command types
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
			// send reply if requested
			if cmd.Reply != nil {
				select {
				case cmd.Reply <- nil:
				default:
				}
			}
		}
	}
}

// AllTimers returns all timers.
func (a *AppManager) AllTimers() []*timer.DotaTimer {
	return a.allTimers
}

// AddActiveTimer adds a timer to the list of active timers.
func (a *AppManager) AddActiveTimer(t *timer.DotaTimer) {
	a.activeLock.Lock()
	defer a.activeLock.Unlock()
	for _, at := range a.activeTimers {
		if at == t {
			return // Already in the list
		}
	}
	a.activeTimers = append(a.activeTimers, t)
}

// RemoveActiveTimer removes a timer from the list of active timers.
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

// CreateBackgroundImage creates an image object for a timer's background.
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

// PlaySound plays a sound file.
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

// UpdateControlButtonState updates the visibility of the main control buttons.
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
		}
	})
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

			// Copy active timers under lock to avoid holding the lock while ticking
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
					// TimerUI exposes UpdateDisplay; call it directly to refresh UI.
					t.UI.UpdateDisplay()
				}
			}
		}
	}
}

// HandleKeyRune handles key presses for the application.
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
		// If the UI implements the concrete TimerWidget, trigger a tap on it.
		if uw, ok := t.UI.(*ui.TimerWidget); ok {
			uw.GetCanvasObject().(*ui.TappableContainer).Tapped(&fyne.PointEvent{})
		}
	}
}

// ShowInfoDialog shows a dialog with the given title and content.
func (a *AppManager) ShowInfoDialog(title, contentFile string, minSize fyne.Size) {
	var contentText string
	if title == i18n.T("About D2Timers") {
		bytes, err := a.content.ReadFile("assets/dialogue_about.json")
		if err != nil {
			dialog.ShowError(err, a.mainWindow)
			return
		}

		var dialogues map[string]string
		if err := json.Unmarshal(bytes, &dialogues); err != nil {
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

// SetAutoButton sets the auto button widget.
func (a *AppManager) SetAutoButton(btn *widget.Button) {
	a.autoButton = btn
}

// SetStartButton sets the start button widget.
func (a *AppManager) SetStartButton(btn *widget.Button) {
	a.startButton = btn
}

// SetStopButton sets the stop button widget.
func (a *AppManager) SetStopButton(btn *widget.Button) {
	a.stopButton = btn
}

// SetResetButton sets the reset button widget.
func (a *AppManager) SetResetButton(btn *widget.Button) {
	a.resetButton = btn
}

// Shutdown attempts to gracefully stop the AppManager command loop. It
// cancels the internal context and allows background goroutines to exit.
func (a *AppManager) Shutdown() {
	if a.cmdCancel != nil {
		a.cmdCancel()
	}
}
