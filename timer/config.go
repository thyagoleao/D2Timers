package timer

import (
	"encoding/json"
	"image/color"
	"log"
)

// AppContentReader defines the interface for reading content from the embedded file system.
type AppContentReader interface {
	ReadFile(name string) ([]byte, error)
}

// TimerState defines the possible states of a timer.
type TimerState int

const (
	StateInactive TimerState = iota
	StateActiveAuto
	StateActiveManual
	StatePaused
	StateUnconfigured
)

// TimerMode defines whether a timer is in auto or manual mode.
type TimerMode int

const (
	ModeAuto TimerMode = iota
	ModeManual
)

// TimerIndex defines the index of each timer for key bindings.
type TimerIndex int

const (
	TimerIndexStackNeutrals TimerIndex = iota
	TimerIndexPowerRunes
	TimerIndexShrinesOfWisdom
	TimerIndexCustomTimer
)

// UI constants
const (
	FontSize     float32 = 25.0 // Title
	FontSizeTime float32 = 24.0 // Time display

	// Dimensions
	TimerWidth        = 320
	TimerHeight       = 96
	GapButton         = 5
	TimerSpacing      = 1
	ControlButtonsGap = 5
	CornerRadius      = 10.0
	CustomInputWidth  = 155
)

var (
	// BackgroundColor is the base background color for timers.
	BackgroundColor = color.NRGBA{R: 0x1e, G: 0x1e, B: 0x1e, A: 0xff}
)

// TimerConfig holds the static configuration for a timer.
type TimerConfig struct {
	Name          string
	AudioFilename string
	Priority      int

	AutoInitial         int
	AutoRepeat          int
	ManualInitial       int
	ManualRepeat        int
	BackgroundImageName string
}

// TimerConfigs holds the configuration for all default timers.
var TimerConfigs []*TimerConfig

// LoadTimerConfigs loads timer configurations from a JSON file.
func LoadTimerConfigs(reader AppContentReader) {
	data, err := reader.ReadFile("assets/timers_config.json")
	if err != nil {
		log.Fatalf("Failed to read timer configs: %v", err)
	}

	err = json.Unmarshal(data, &TimerConfigs)
	if err != nil {
		log.Fatalf("Failed to unmarshal timer configs: %v", err)
	}
}
