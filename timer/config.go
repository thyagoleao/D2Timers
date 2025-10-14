package timer

import (
	"fmt"
	"image/color"
	"log"

	"gopkg.in/yaml.v3"
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
	Name                string `yaml:"Name"`
	AudioFilename       string `yaml:"AudioFilename"`
	Priority            int    `yaml:"Priority"`

	Normal_Auto_Initial   int `yaml:"Normal_Auto_Initial"`
	Normal_Auto_Repeat    int `yaml:"Normal_Auto_Repeat"`
	Normal_Manual_Initial int `yaml:"Normal_Manual_Initial"`
	Normal_Manual_Repeat  int `yaml:"Normal_Manual_Repeat"`
	// Turbo* fields: if zero, timer has no turbo configuration and normal values are used
	Turbo_Auto_Initial    int `yaml:"Turbo_Auto_Initial"`
	Turbo_Auto_Repeat     int `yaml:"Turbo_Auto_Repeat"`
	Turbo_Manual_Initial  int `yaml:"Turbo_Manual_Initial"`
	Turbo_Manual_Repeat   int `yaml:"Turbo_Manual_Repeat"`
	BackgroundImageName string `yaml:"BackgroundImageName"`
}

// TimerConfigs holds the configuration for all default timers.
var TimerConfigs []*TimerConfig

// LoadTimerConfigs loads timer configurations from a YAML file.
func LoadTimerConfigs(reader AppContentReader) {
	data, err := reader.ReadFile("assets/timers_config.yaml")
	if err != nil {
		log.Fatalf("Failed to read timer configs: %v", err)
	}

	var configs map[string]TimerConfig
	err = yaml.Unmarshal(data, &configs)
	if err != nil {
		log.Fatalf("Failed to unmarshal timer configs: %v", err)
	}

	for i := 0; i < len(configs); i++ {
		key := fmt.Sprintf("Timer %d", i+1)
		config, ok := configs[key]
		if !ok {
			continue
		}
		TimerConfigs = append(TimerConfigs, &config)
	}
}
