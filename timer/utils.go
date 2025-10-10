package timer

import (
	"fmt"
)

// FormatTime converts a number of seconds into a mm:ss string format.
func FormatTime(sec int) string {
	if sec < 0 {
		sec = 0
	}
	return fmt.Sprintf("%02d:%02d", sec/60, sec%60)
}
