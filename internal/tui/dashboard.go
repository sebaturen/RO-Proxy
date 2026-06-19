package tui

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/rivo/tview"
	"roproxy/internal/common"
	"roproxy/internal/proxy"
)

// Build version - set at compile time via ldflags
var BuildVersion = "dev"

type DebugMode int

const (
	DebugOff DebugMode = iota
	DebugOn
	DebugVerbose
	DebugVeryVerbose
)

func (d DebugMode) String() string {
	switch d {
	case DebugOff:
		return "OFF"
	case DebugOn:
		return "ON"
	case DebugVerbose:
		return "VERBOSE"
	case DebugVeryVerbose:
		return "VERY VERBOSE"
	default:
		return "UNKNOWN"
	}
}

type Dashboard struct {
	app               *tview.Application
	proxy             *proxy.Proxy
	stats             *Stats
	
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
	debugMode         DebugMode
	showWarnings      bool
	captureServer     bool
	captureClient     bool
	fullTimestamp     bool
	connectionFilter  uint64  // 0 = show all
	filterActive      bool
	recording         bool
	recordFile        *os.File
	recordMutex       sync.Mutex
	
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

func NewDashboard(p *proxy.Proxy, captureServer, captureClient bool) *Dashboard {
	app := tview.NewApplication()
	
	d := &Dashboard{
		app:              app,
		proxy:            p,
		stats:            NewStats(),
		debugMode:        DebugOff,
		showWarnings:     true,
		captureServer:    captureServer,
		captureClient:    captureClient,
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
	// Close recording file if open
	d.recordMutex.Lock()
	if d.recordFile != nil {
		d.recordFile.Close()
		d.recordFile = nil
	}
	d.recordMutex.Unlock()
	
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
	stats := d.stats.Get()
	queueSize := 0
	if globalAPI := common.GetAPIConsumer(); globalAPI != nil {
		queueSize = globalAPI.QueueSize()
	}
	
	uptime := time.Since(stats.StartTime).Round(time.Second)
	
	content := fmt.Sprintf(`[yellow]Uptime:[-] %s
[yellow]Total Packets:[-] %d

[green]Client → Server:[-] %d
[cyan]Server → Client:[-] %d

[yellow]Unknown Packets:[-] %d
[yellow]API Queue Size:[-] %d

[yellow]Bytes C→S:[-] %s
[yellow]Bytes S→C:[-] %s`,
		uptime,
		stats.TotalPackets,
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
	conns := d.proxy.GetActiveConnections()
	
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
	debugStatus := fmt.Sprintf("[yellow]%s[-]", d.debugMode.String())
	warningsStatus := colorBool(d.showWarnings, "ON", "OFF")
	serverStatus := colorBool(d.captureServer, "ON", "OFF")
	clientStatus := colorBool(d.captureClient, "ON", "OFF")
	timestampStatus := colorBool(d.fullTimestamp, "FULL", "TIME")
	
	d.recordMutex.Lock()
	recordingStatus := colorBool(d.recording, "ON", "OFF")
	d.recordMutex.Unlock()
	
	filterText := "[white]ALL[-]"
	if d.connectionFilter > 0 {
		filterText = fmt.Sprintf("[yellow]#%d[-]", d.connectionFilter)
	}
	
	content := fmt.Sprintf(`[yellow]D[-] Debug:    %s
[yellow]W[-] Warnings: %s
[yellow]S[-] S→C:      %s
[yellow]C[-] C→S:      %s
[yellow]T[-] Time:     %s
[yellow]F[-] Filter:   %s
[yellow]R[-] Record:   %s

[yellow]L[-] Clear Logs
[yellow]Q[-] Quit
[gray]Ctrl+C/Ctrl+D/Q: force quit[-]`,
		debugStatus,
		warningsStatus,
		serverStatus,
		clientStatus,
		timestampStatus,
		filterText,
		recordingStatus,
	)
	
	d.controlsText.SetText(content)
	
	// Show/hide filter input
	if !d.filterActive {
		d.filterInput.SetLabel("")
		d.filterInput.SetText("")
	}
}

func (d *Dashboard) updateStatusBar() {
	status := fmt.Sprintf("[white]ROProxy Running - v%s", BuildVersion)
	if !d.captureServer && !d.captureClient {
		status = "[red]● WARNING: All capture disabled[-]"
	}
	
	d.statusBar.SetText(status)
}

func (d *Dashboard) toggleDebugMode() {
	// Cycle through: OFF -> ON -> VERBOSE -> VERY VERBOSE -> OFF
	d.debugMode = (d.debugMode + 1) % 4
	d.app.QueueUpdateDraw(func() {
		d.updateControlsView()
		d.updateStatusBar()
	})
	d.Log("[yellow]Debug mode: %s[-]", d.debugMode.String())
}

func (d *Dashboard) toggleWarnings() {
	d.showWarnings = !d.showWarnings
	d.app.QueueUpdateDraw(func() {
		d.updateControlsView()
	})
	d.Log("[yellow]Warnings: %s[-]", colorBool(d.showWarnings, "ON", "OFF"))
}

func (d *Dashboard) toggleCaptureServer() {
	d.captureServer = !d.captureServer
	d.proxy.SetCaptureServer(d.captureServer)
	d.app.QueueUpdateDraw(func() {
		d.updateControlsView()
		d.updateStatusBar()
	})
	d.Log("[cyan]Server→Client capture: %s[-]", colorBool(d.captureServer, "ON", "OFF"))
}

func (d *Dashboard) toggleTimestamp() {
	d.fullTimestamp = !d.fullTimestamp
	d.app.QueueUpdateDraw(func() {
		d.updateControlsView()
	})
	if d.fullTimestamp {
		d.Log("[yellow]Timestamp: FULL (date + time)[-]")
	} else {
		d.Log("[yellow]Timestamp: TIME ONLY[-]")
	}
}

func (d *Dashboard) promptConnectionFilter() {
	d.filterActive = true
	// Set initial value
	if d.connectionFilter > 0 {
		d.filterInput.SetText(fmt.Sprintf("%d", d.connectionFilter))
	} else {
		d.filterInput.SetText("")
	}
	d.filterInput.SetLabel("[yellow]→ Conn ID (Enter=apply, ESC=cancel, Q=quit): [-]")
	
	d.app.QueueUpdateDraw(func() {
		// Remove border to avoid "?" characters on SSH
		d.controlsView.SetBorder(false)
		d.app.SetFocus(d.filterInput)
	})
}

func (d *Dashboard) toggleCaptureClient() {
	d.captureClient = !d.captureClient
	d.proxy.SetCaptureClient(d.captureClient)
	d.app.QueueUpdateDraw(func() {
		d.updateControlsView()
		d.updateStatusBar()
	})
	d.Log("[green]Client→Server capture: %s[-]", colorBool(d.captureClient, "ON", "OFF"))
}

func (d *Dashboard) clearLogs() {
	d.logMutex.Lock()
	d.logBuffer = make([]string, 0)
	d.logMutex.Unlock()
	
	d.app.QueueUpdateDraw(func() {
		d.logsView.Clear()
	})
	
	d.Log("[gray]Logs cleared[-]")
}

func (d *Dashboard) Log(format string, args ...interface{}) {
	timestamp := time.Now().Format("2006-01-02 15:04:05.000")
	msg := fmt.Sprintf("[white]%s %s[-]", timestamp, fmt.Sprintf(format, args...))
	
	d.logMutex.Lock()
	d.logBuffer = append(d.logBuffer, msg)
	if len(d.logBuffer) > d.maxLogs {
		d.logBuffer = d.logBuffer[1:]
	}
	d.logMutex.Unlock()
	
	// Queue update instead of direct write
	d.app.QueueUpdateDraw(func() {
		fmt.Fprintf(d.logsView, "%s\n", msg)
	})
}

func (d *Dashboard) LogPacket(connID uint64, direction common.PacketDirection, opcode uint16, size int, desc string, payload []byte, checksum *uint8) {
	if d.debugMode == DebugOff {
		d.stats.AddPacket(direction, size, false)
		return
	}
	
	if d.connectionFilter > 0 && d.connectionFilter != connID {
		d.stats.AddPacket(direction, size, false)
		return
	}
	
	timestamp := formatTimestamp(d.fullTimestamp)
	dirSymbol, color := formatDirection(direction)
	checksumStr := formatChecksum(checksum)
	descDisplay := formatDesc(desc)
	
	var msg string
	switch d.debugMode {
	case DebugOn:
		msg = fmt.Sprintf("[gray]%s[-][yellow][#%d][-][%s][%s][-][yellow][0x%04X][-][white]%s size=%d%s[-]",
			timestamp, connID, color, dirSymbol, opcode, descDisplay, size, checksumStr)
	case DebugVerbose:
		payloadHex := formatPayload(payload, true)
		msg = fmt.Sprintf("[gray]%s[-][yellow][#%d][-][%s][%s][-][yellow][0x%04X][-][white]%s size=%d%s payload=%s[-]",
			timestamp, connID, color, dirSymbol, opcode, descDisplay, size, checksumStr, payloadHex)
	case DebugVeryVerbose:
		payloadHex := formatPayload(payload, false)
		msg = fmt.Sprintf("[gray]%s[-][yellow][#%d][-][%s][%s][-][yellow][0x%04X][-][white]%s size=%d%s payload=%s[-]",
			timestamp, connID, color, dirSymbol, opcode, descDisplay, size, checksumStr, payloadHex)
	}
	
	d.writeToRecording(connID, direction, opcode, payload, checksum)
	
	d.logMutex.Lock()
	d.logBuffer = append(d.logBuffer, msg)
	if len(d.logBuffer) > d.maxLogs {
		d.logBuffer = d.logBuffer[1:]
	}
	d.logMutex.Unlock()
	
	d.app.QueueUpdateDraw(func() {
		fmt.Fprintf(d.logsView, "%s\n", msg)
	})
	
	d.stats.AddPacket(direction, size, false)
}

func (d *Dashboard) LogUnknown(connID uint64, direction common.PacketDirection, opcode uint16, size int, payload []byte, checksum *uint8) {
	if !d.showWarnings {
		d.stats.AddPacket(direction, size, true)
		return
	}
	
	if d.connectionFilter > 0 && d.connectionFilter != connID {
		d.stats.AddPacket(direction, size, true)
		return
	}

	timestamp := formatTimestamp(d.fullTimestamp)
	dirSymbol, color := formatDirection(direction)
	checksumStr := formatChecksum(checksum)
	payloadHex := formatPayload(payload, true)
	
	msg := fmt.Sprintf("[white][%s][#%d] [red][⚠][-][%s][%s][-][yellow][0x%04X][-][red][UNKNOWN][-][white] size=%d%s payload=%s[-]",
		timestamp, connID, color, dirSymbol, opcode, size, checksumStr, payloadHex)
	
	d.writeToRecording(connID, direction, opcode, payload, checksum)
	
	d.logMutex.Lock()
	d.logBuffer = append(d.logBuffer, msg)
	if len(d.logBuffer) > d.maxLogs {
		d.logBuffer = d.logBuffer[1:]
	}
	d.logMutex.Unlock()
	
	d.app.QueueUpdateDraw(func() {
		fmt.Fprintf(d.logsView, "%s\n", msg)
	})
	
	d.stats.AddPacket(direction, size, true)
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

func (d *Dashboard) GetStats() *Stats {
	return d.stats
}

func (d *Dashboard) IsDebugMode() bool {
	return d.debugMode != DebugOff
}

func (d *Dashboard) IsShowWarnings() bool {
	return d.showWarnings
}
