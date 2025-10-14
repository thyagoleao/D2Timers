// Package timer contains the domain logic for Dota timers: the TimerConfig
// definition and the DotaTimer runtime state machine.
//
// Maintenance notes:
//   - Mutable fields (State, Remaining, mode, etc) are accessed by multiple
//     goroutines (the command loop and the tick goroutine). To avoid data
//     races when changing or reading these fields, protect them with a mutex
//     (add `mu sync.Mutex` to DotaTimer) or ensure that all mutations happen
//     on a single goroutine.
//   - The DotaTimer exposes minimal methods for state transitions (Start,
//     Pause, Resume, Reset, Tick). Prefer calling these through the centralized
//     application command loop to keep behavior deterministic.
package timer

import (
	"sync"

	"fyne.io/fyne/v2"
)

// App defines the interface that the timer package needs to communicate back to the main application.
type App interface {
	AddActiveTimer(*DotaTimer)
	RemoveActiveTimer(*DotaTimer)
	PlaySound(string)
}

// TimerUI is the minimal interface the timer logic expects from the UI side.
type TimerUI interface {
	GetCanvasObject() fyne.CanvasObject
	UpdateDisplay()
}

// DotaTimer represents a single timer's state and logic.
type DotaTimer struct {
	*TimerConfig

	// mutable state - protect with mu
	mu                sync.RWMutex
	State             TimerState
	mode              TimerMode
	Remaining         int
	cycleDuration     int
	CustomDurationSec int

	// original config values, stored for turbo mode toggling
	originalNormal_Auto_Initial   int
	originalNormal_Auto_Repeat    int
	originalTurbo_Auto_Initial      int
	originalTurbo_Auto_Repeat       int
	originalNormal_Manual_Initial     int
	originalNormal_Manual_Repeat      int
	originalTurbo_Manual_Initial  int
	originalTurbo_Manual_Repeat   int

	// UI components are stored here but managed by the UI package.
	UI TimerUI // Holds the associated UI component, managed by the ui package
}

// GetMode returns the current mode of the timer.
func (t *DotaTimer) GetMode() TimerMode {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.mode
}

// NewDotaTimer creates a new timer based on a config.
func NewDotaTimer(c *TimerConfig) *DotaTimer {
	t := &DotaTimer{
		TimerConfig:                 c,
		State:                       StateInactive,
		mode:                        ModeAuto,
		originalNormal_Auto_Initial:   c.Normal_Auto_Initial,
		originalNormal_Auto_Repeat:    c.Normal_Auto_Repeat,
		originalTurbo_Auto_Initial:      c.Turbo_Auto_Initial,
		originalTurbo_Auto_Repeat:       c.Turbo_Auto_Repeat,
		originalNormal_Manual_Initial:     c.Normal_Manual_Initial,
		originalNormal_Manual_Repeat:      c.Normal_Manual_Repeat,
		originalTurbo_Manual_Initial:  c.Turbo_Manual_Initial,
		originalTurbo_Manual_Repeat:   c.Turbo_Manual_Repeat,
	}
	if c.Name == "Custom Timer" {
		t.State = StateUnconfigured
	}
	return t
}

func (t *DotaTimer) changeState(a App, newState TimerState, newMode TimerMode) {
	t.mu.Lock()
	t.State = newState
	if newMode != 0 {
		t.mode = newMode
	}
	t.mu.Unlock()

	switch newState {
	case StateActiveAuto, StateActiveManual:
		a.AddActiveTimer(t)
	case StatePaused, StateInactive, StateUnconfigured:
		a.RemoveActiveTimer(t)
	}
}

// Start begins the timer's countdown.
func (t *DotaTimer) Start(a App, mode TimerMode) {
	t.mu.Lock()
	if t.Name == "Custom Timer" && t.CustomDurationSec == 0 {
		t.mu.Unlock()
		return
	}

	var newState TimerState
	switch mode {
	case ModeAuto:
		newState = StateActiveAuto
	case ModeManual:
		newState = StateActiveManual
	default:
		newState = StateActiveManual
	}

	if t.State != StatePaused {
		isAuto := mode == ModeAuto
		if t.Name == "Custom Timer" {
			t.Remaining = t.CustomDurationSec
			t.cycleDuration = t.CustomDurationSec
		} else if isAuto {
			t.Remaining = t.Normal_Auto_Initial
			t.cycleDuration = t.Normal_Auto_Repeat
		} else {
			t.Remaining = t.Normal_Manual_Initial
			t.cycleDuration = t.Normal_Manual_Repeat
		}
	}
	t.mu.Unlock()

	t.changeState(a, newState, mode)
}

// Pause stops the timer's countdown.
func (t *DotaTimer) Pause(a App) {
	t.mu.RLock()
	state := t.State
	t.mu.RUnlock()
	switch state {
	case StateActiveManual:
		t.changeState(a, StatePaused, ModeManual)
	case StateActiveAuto:
		t.changeState(a, StatePaused, ModeAuto)
	}
}

// Resume continues a paused timer.
func (t *DotaTimer) Resume(a App) {
	t.mu.RLock()
	isPaused := t.State == StatePaused
	mode := t.mode
	t.mu.RUnlock()
	if isPaused {
		switch mode {
		case ModeManual:
			t.changeState(a, StateActiveManual, ModeManual)
		case ModeAuto:
			t.changeState(a, StateActiveAuto, ModeAuto)
		}
	}
}

// Reset puts the timer back to its initial state.
func (t *DotaTimer) Reset(a App) {
	t.mu.Lock()
	t.Remaining = 0
	t.cycleDuration = 0
	if t.Name == "Custom Timer" {
		t.CustomDurationSec = 0 // Reset custom duration to 0
		t.mu.Unlock()
		t.changeState(a, StateUnconfigured, 0)
		return
	}
	t.mode = ModeAuto
	t.mu.Unlock()
	t.changeState(a, StateInactive, t.mode)
}

// Alert plays the timer's sound.
func (t *DotaTimer) Alert(a App) {
	a.PlaySound(t.AudioFilename)
}

// Tick processes one second of time passing.
func (t *DotaTimer) Tick(a App) {
	t.mu.Lock()
	t.Remaining--
	remaining := t.Remaining
	name := t.Name
	state := t.State
	custom := t.CustomDurationSec
	cycle := t.cycleDuration
	t.mu.Unlock()

	if remaining <= 0 {
		t.Alert(a)
		if name == "Custom Timer" && state == StateActiveManual {
			t.mu.Lock()
			t.Remaining = custom
			t.mu.Unlock()
			t.changeState(a, StateInactive, 0)
		} else {
			t.mu.Lock()
			t.Remaining = cycle
			t.mu.Unlock()
		}
	}
}

// GetState returns the current state in a thread-safe manner.
func (t *DotaTimer) GetState() TimerState {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.State
}

// GetRemaining returns the remaining seconds in a thread-safe manner.
func (t *DotaTimer) GetRemaining() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.Remaining
}

// GetCustomDurationSec returns the custom duration in a thread-safe manner.
func (t *DotaTimer) GetCustomDurationSec() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.CustomDurationSec
}

// SetCustomDuration sets the custom duration and marks the timer as inactive.
func (t *DotaTimer) SetCustomDuration(val int) {
	t.mu.Lock()
	t.CustomDurationSec = val
	t.State = StateInactive
	t.mu.Unlock()
}

// TimerSnapshot is an atomic snapshot of timer fields that the UI needs to
// render a consistent view. Call GetSnapshot() to obtain a coherent set of
// values under the timer lock.
type TimerSnapshot struct {
	State             TimerState
	Remaining         int
	Mode              TimerMode
	CustomDurationSec int
	Name              string
	Normal_Auto_Initial   int
	Normal_Manual_Initial int
	Normal_Auto_Repeat    int
	Normal_Manual_Repeat  int
}

// GetSnapshot returns a consistent snapshot of the timer's state for UI use.
func (t *DotaTimer) GetSnapshot() TimerSnapshot {
	t.mu.RLock()
	snap := TimerSnapshot{
		State:             t.State,
		Remaining:         t.Remaining,
		Mode:              t.mode,
		CustomDurationSec: t.CustomDurationSec,
		Name:              t.Name,
		Normal_Auto_Initial:   t.Normal_Auto_Initial,
		Normal_Manual_Initial: t.Normal_Manual_Initial,
		Normal_Auto_Repeat:    t.Normal_Auto_Repeat,
		Normal_Manual_Repeat:  t.Normal_Manual_Repeat,
	}
	t.mu.RUnlock()
	return snap
}

// SetNormal_Auto_InitialRepeat safely updates the Normal_Auto_Initial and Normal_Auto_Repeat values.
func (t *DotaTimer) SetNormal_Auto_InitialRepeat(initial, repeat int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Normal_Auto_Initial = initial
	t.Normal_Auto_Repeat = repeat
}

// SetNormal_Manual_InitialRepeat safely updates the Normal_Manual_Initial and Normal_Manual_Repeat values.
func (t *DotaTimer) SetNormal_Manual_InitialRepeat(initial, repeat int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Normal_Manual_Initial = initial
	t.Normal_Manual_Repeat = repeat
}

// GetOriginals returns the original timer configuration values.
func (t *DotaTimer) GetOriginals() (int, int, int, int, int, int, int, int) {
	return t.originalNormal_Auto_Initial, t.originalNormal_Auto_Repeat, t.originalTurbo_Auto_Initial, t.originalTurbo_Auto_Repeat, t.originalNormal_Manual_Initial, t.originalNormal_Manual_Repeat, t.originalTurbo_Manual_Initial, t.originalTurbo_Manual_Repeat
}