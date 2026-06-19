package tui

import (
	"fmt"
	"os"
	"time"

	"roproxy/internal/common"
)

func (d *Dashboard) writeToRecording(connID uint64, direction common.PacketDirection, opcode uint16, payload []byte, checksum *uint8) {
	d.recordMutex.Lock()
	defer d.recordMutex.Unlock()
	
	if !d.recording || d.recordFile == nil {
		return
	}
	
	unixTime := time.Now().Unix()
	dirStr := "C->S"
	if direction == common.ServerToClient {
		dirStr = "S->C"
	}
	
	fullPacket := buildPacketForRecording(opcode, payload, checksum)
	payloadHex := fmt.Sprintf("%X", fullPacket)
	recordLine := fmt.Sprintf("%d | %d | %s | %s\n", unixTime, connID, dirStr, payloadHex)
	d.recordFile.WriteString(recordLine)
}

func (d *Dashboard) toggleRecording() {
	var filename string
	var stopped bool
	
	d.recordMutex.Lock()
	if d.recording {
		if d.recordFile != nil {
			d.recordFile.Close()
			d.recordFile = nil
		}
		d.recording = false
		stopped = true
	} else {
		filename = fmt.Sprintf("roproxy_recording_%s.txt", time.Now().Format("20060102_150405"))
		file, err := os.Create(filename)
		if err != nil {
			d.recordMutex.Unlock()
			d.Log("[red]Failed to create recording file: %v[-]", err)
			return
		}
		d.recordFile = file
		d.recording = true
		stopped = false
	}
	d.recordMutex.Unlock()
	
	d.app.QueueUpdateDraw(func() {
		d.updateControlsView()
	})
	
	if stopped {
		d.Log("[red]Recording stopped[-]")
	} else {
		d.Log("[green]Recording started: %s[-]", filename)
	}
}
