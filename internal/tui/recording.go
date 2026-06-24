package tui

import (
	"roproxy/internal/common"
	"roproxy/internal/proxy"
)

// toggleRecording toggles the global recording flag.
// Worker threads will start/stop recording raw chunks based on this flag.
func (d *Dashboard) toggleRecording() {
	d.recordMutex.Lock()
	d.recording = !d.recording
	newState := d.recording
	d.recordMutex.Unlock()
	
	// Update global recording flag (worker threads check this)
	proxy.SetRecording(newState)
	
	d.app.QueueUpdateDraw(func() {
		d.updateControlsView()
	})
	
	if newState {
		common.Log(common.LogProxy, common.LogInfo, "[green]Recording started (raw chunks will be saved per connection)[-]")
	} else {
		common.Log(common.LogProxy, common.LogInfo, "[yellow]Recording stopped[-]")
	}
}
