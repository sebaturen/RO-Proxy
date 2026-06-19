package tui

import (
	"fmt"
	"time"

	"github.com/rivo/tview"
	"roproxy/internal/common"
)

// Helper functions for formatting log messages

func formatTimestamp(fullTimestamp bool) string {
	if fullTimestamp {
		return time.Now().Format("2006-01-02 15:04:05.000")
	}
	return time.Now().Format("15:04:05.000")
}

func formatDirection(direction common.PacketDirection) (string, string) {
	if direction == common.ClientToServer {
		return "C→S", "green"
	}
	return "S→C", "cyan"
}

func formatChecksum(checksum *uint8) string {
	if checksum != nil {
		return fmt.Sprintf(" checksum=0x%02X", *checksum)
	}
	return ""
}

func formatDesc(desc string) string {
	if desc == "" {
		return "[red]Unknown[-]"
	}
	return tview.Escape("[" + desc + "]")
}

func formatPayload(payload []byte, truncate bool) string {
	hex := fmt.Sprintf("%X", payload)
	if truncate && len(hex) > 80 {
		return hex[:80] + "..."
	}
	return hex
}

func buildPacketForRecording(opcode uint16, payload []byte, checksum *uint8) []byte {
	var fullPacket []byte
	header := []byte{byte(opcode & 0xFF), byte((opcode >> 8) & 0xFF)}
	fullPacket = append(fullPacket, header...)
	fullPacket = append(fullPacket, payload...)
	if checksum != nil {
		fullPacket = append(fullPacket, *checksum)
	}
	return fullPacket
}
