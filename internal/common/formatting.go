package common

import (
	"fmt"
	"time"
	"github.com/rivo/tview"
)

func FormatTimestamp(time time.Time) string {
	return fmt.Sprintf("[gray]%s[-]", time.Format("15:04:05.000"))
}

func FormatDirection(direction PacketDirection) string {
	color := "cyan"
	dirStr := "S→C"
	if direction == ClientToServer {
		color = "green"
		dirStr = "C→S"
	}
	dirEsc := tview.Escape(fmt.Sprintf("[%s]", dirStr))
	return fmt.Sprintf("[%s]%s[-]", color, dirEsc)
}

func FormatChecksum(checksum *uint8) string {
	if checksum != nil {
		return fmt.Sprintf(" checksum=0x%02X", *checksum)
	}
	return ""
}

func FormatDesc(desc string) string {
	if desc == "" {
		desc = "UNKNOWN"
	}
	desc = fmt.Sprintf("[%s]", desc)
	return tview.Escape(desc)
}

func FormatPayload(payload []byte, truncate bool) string {
	hex := fmt.Sprintf("%X", payload)
	if truncate && len(hex) > 80 {
		return hex[:80] + "..."
	}
	return hex
}

func FormatLogCategory(category LogCategory) string {
	return tview.Escape(fmt.Sprintf("[%s]", category.String()))
}

func FormatVerbosity(verbosity LogLevel) string {
	return tview.Escape(fmt.Sprintf("[%s]", verbosity.String()))
}