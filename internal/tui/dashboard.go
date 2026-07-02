package tui

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/rivo/tview"
	"roproxy/internal/analyzer"
	"roproxy/internal/common"
)

// Build version - set at compile time via ldflags
var BuildVersion = "dev"

type VerbosityLevel int

const (
	VerbosityInfo VerbosityLevel = iota
	VerbosityVerbose
	VerbosityVeryVerbose
)

func (v VerbosityLevel) String() string {
	switch v {
	case VerbosityInfo:
		return "INFO"
	case VerbosityVerbose:
		return "VERBOSE"
	case VerbosityVeryVerbose:
		return "VERY VERBOSE"
	default:
		return "UNKNOWN"
	}
}

type Dashboard struct {
	app               *tview.Application
	processor         *analyzer.Processor
	
	// UI components
	statsView         *tview.TextView
	logsView          *tview.TextView
	connectionsView   *tview.TextView
	controlsView      *tview.Flex
	controlsText      *tview.TextView
	filterInput       *tview.InputField
	statusBar         *tview.TextView
	rootFlex          *tview.Flex
	
	// Settings
	verbosityLevel    VerbosityLevel  // Controls log filtering (Info/Verbose/VeryVerbose)
	fullTimestamp     bool
	connectionFilter  uint64  // 0 = show all
	filterActive      bool
	
	// Log buffer
	logBuffer         []string
	logMutex          sync.Mutex
	maxLogs           int
	
	// Rate limiting for logs
	lastLogTime       time.Time
	logRateMutex      sync.Mutex
	minLogInterval    time.Duration
	
	updateTicker      *time.Ticker
	stopChan          chan struct{}
}

func NewDashboard(processor *analyzer.Processor) *Dashboard {
	app := tview.NewApplication()
	
	d := &Dashboard{
		app:              app,
		processor:        processor,
		verbosityLevel:   VerbosityInfo,
		fullTimestamp:    false,
		connectionFilter: 0,
		filterActive:     false,
		logBuffer:        make([]string, 0),
		maxLogs:          1000,
		minLogInterval:   10 * time.Millisecond,
		stopChan:         make(chan struct{}),
	}
	
	d.buildUI()
	return d
}

func (d *Dashboard) Start() error {
	// Start update loop
	d.updateTicker = time.NewTicker(500 * time.Millisecond)
	go d.updateLoop()
	
	// Run the app
	return d.app.Run()
}

func (d *Dashboard) Stop() {
	if d.updateTicker != nil {
		d.updateTicker.Stop()
	}
	close(d.stopChan)
	d.app.Stop()
}

func (d *Dashboard) updateLoop() {
	for {
		select {
		case <-d.stopChan:
			return
		case <-d.updateTicker.C:
			d.app.QueueUpdateDraw(func() {
				d.updateStats()
				d.updateConnections()
			})
		}
	}
}

func (d *Dashboard) updateStats() {
	stats := common.GetGlobalStats()
	queueSize := 0
	if globalAPI := common.GetAPIConsumer(); globalAPI != nil {
		queueSize = globalAPI.QueueSize()
	}
	
	uptime := time.Since(d.processor.GetStartTime()).Round(time.Second)
	totalPackets := stats.ClientToServer + stats.ServerToClient
	
	content := fmt.Sprintf(`[yellow]Uptime:[-] %s
[yellow]Total Packets:[-] %d

[green]Client → Server:[-] %d
[cyan]Server → Client:[-] %d

[yellow]Unknown Packets:[-] %d
[yellow]API Queue Size:[-] %d

[yellow]Bytes C→S:[-] %s
[yellow]Bytes S→C:[-] %s`,
		uptime,
		totalPackets,
		stats.ClientToServer,
		stats.ServerToClient,
		stats.UnknownPackets,
		queueSize,
		formatBytes(stats.BytesClientToServer),
		formatBytes(stats.BytesServerToClient),
	)
	
	d.statsView.Clear()
	d.statsView.SetText(content)
}

func (d *Dashboard) updateConnections() {
	conns := d.processor.GetActiveConnections()
	
	d.connectionsView.Clear()
	
	if len(conns) == 0 {
		d.connectionsView.SetText("[gray]No active connections[-]")
		return
	}
	
	content := ""
	for _, conn := range conns {
		duration := time.Since(conn.StartTime).Round(time.Second)
		content += fmt.Sprintf("[yellow]#%d[-] %s → %s [gray](%s)[-]\n",
			conn.ID, conn.ClientAddr, conn.ServerAddr, duration)
	}
	
	d.connectionsView.SetText(content)
}

func (d *Dashboard) updateControlsView() {
	verbosityStatus := fmt.Sprintf("[yellow]%s[-]", d.verbosityLevel.String())
	
	filterText := "[white]ALL[-]"
	if d.connectionFilter > 0 {
		filterText = fmt.Sprintf("[yellow]#%d[-]", d.connectionFilter)
	}
	
	// Check recording status from flag file
	_, err := os.Stat(".recording_enabled")
	recordingEnabled := err == nil
	recordingText := colorBool(recordingEnabled, "ON", "OFF")
	
	content := fmt.Sprintf(`[yellow]V[-] Verbosity: %s
[yellow]F[-] Filter:    %s
[yellow]R[-] Recording: %s

[yellow]L[-] Clear Logs
[yellow]Q[-] Quit Analyzer
[gray]Ctrl+C: force quit[-]`,
		verbosityStatus,
		filterText,
		recordingText,
	)
	
	d.controlsText.SetText(content)
	
	// Show/hide filter input
	if !d.filterActive {
		d.filterInput.SetLabel("")
		d.filterInput.SetText("")
	}
}

func (d *Dashboard) updateStatusBar() {
	status := fmt.Sprintf("[white]ROProxy Analyzer - v%s [gray](Hot-Swappable)[-]", BuildVersion)
	d.statusBar.SetText(status)
}

func (d *Dashboard) toggleVerbosity() {
	// Cycle through: Info -> Verbose -> VeryVerbose -> Info
	d.verbosityLevel = (d.verbosityLevel + 1) % 3
	d.app.QueueUpdateDraw(func() {
		d.updateControlsView()
		d.updateStatusBar()
	})
	common.Log(common.LogUI, common.LogInfo, "[yellow]Verbosity level: %s[-]", d.verbosityLevel.String())
}

func (d *Dashboard) promptConnectionFilter() {
	d.filterActive = true
	// Set initial value
	if d.connectionFilter > 0 {
		d.filterInput.SetText(fmt.Sprintf("%d", d.connectionFilter))
	} else {
		d.filterInput.SetText("")
	}
	d.filterInput.SetLabel("[yellow]→ Conn ID (Enter/ESC): [-]")
	
	d.app.QueueUpdateDraw(func() {
		// Remove border to avoid "?" characters on SSH
		d.controlsView.SetBorder(false)
		d.app.SetFocus(d.filterInput)
	})
}

func (d *Dashboard) clearLogs() {
	d.logMutex.Lock()
	d.logBuffer = make([]string, 0)
	d.logMutex.Unlock()
	
	d.app.QueueUpdateDraw(func() {
		d.logsView.Clear()
	})
	
	common.Log(common.LogUI, common.LogInfo, "[gray]Logs cleared[-]")
}

// toggleRecording toggles the recording state by creating/deleting the flag file.
// The Proxy watches this file to enable/disable recording.
func (d *Dashboard) toggleRecording() {
	const recordingFlagFile = ".recording_enabled"
	
	_, err := os.Stat(recordingFlagFile)
	currentlyEnabled := err == nil
	
	if currentlyEnabled {
		// Disable: remove the flag file
		if err := os.Remove(recordingFlagFile); err != nil {
			common.Log(common.LogUI, common.LogError, "Failed to disable recording: %v", err)
			return
		}
		common.Log(common.LogUI, common.LogInfo, "[red]Recording DISABLED[-]")
	} else {
		// Enable: create the flag file
		file, err := os.Create(recordingFlagFile)
		if err != nil {
			common.Log(common.LogUI, common.LogError, "Failed to enable recording: %v", err)
			return
		}
		file.Close()
		common.Log(common.LogUI, common.LogInfo, "[green]Recording ENABLED[-]")
	}
	
	d.app.QueueUpdateDraw(func() {
		d.updateControlsView()
	})
}

// LogBatch writes multiple log messages in a single UI update for better performance.
// Used by the log consumer to batch multiple messages together.
func (d *Dashboard) LogBatch(messages []string) {
	d.logMutex.Lock()
	d.logBuffer = append(d.logBuffer, messages...)
	if len(d.logBuffer) > d.maxLogs {
		overflow := len(d.logBuffer) - d.maxLogs
		d.logBuffer = d.logBuffer[overflow:]
	}
	d.logMutex.Unlock()
	
	// Single update for ALL messages
	d.app.QueueUpdateDraw(func() {
		for _, msg := range messages {
			fmt.Fprintf(d.logsView, "%s\n", msg)
		}
	})

	d.logsView.ScrollToEnd()
}

func colorBool(val bool, trueText, falseText string) string {
	if val {
		return fmt.Sprintf("[green]%s[-]", trueText)
	}
	return fmt.Sprintf("[red]%s[-]", falseText)
}

func formatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}