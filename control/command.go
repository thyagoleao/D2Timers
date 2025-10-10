// Package control defines lightweight command messages used by the UI to
// request actions from the application command loop. The command-loop
// centralizes state changes to avoid races and to simplify synchronization.
package control

import "D2Timers/timer"

// CommandType enumerates supported command operations.
type CommandType int

const (
	CmdStart CommandType = iota
	CmdPause
	CmdResume
	CmdReset
)

// Command is the message sent from UI to AppManager.commandLoop. The
// optional Reply channel can be used by the commandLoop to confirm
// completion back to the sender (useful for keeping UI state in sync).
type Command struct {
	Type   CommandType
	Target *timer.DotaTimer // target timer
	Mode   timer.TimerMode
	Reply  chan error // optional reply channel
}
