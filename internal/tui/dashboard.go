package tui

import (
	"fmt"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"roproxy/internal/common"
	"roproxy/internal/proxy"
)

type Dashboard struct {
	app               *tview.Application
	proxy             *proxy.Proxy
	stats             *Stats
	
	// UI components
	statsView         *tview.TextView
	logsView          *tview.TextView
	connectionsView   *tview.TextView
	controlsView      *tview.TextView
	statusBar         *tview.TextView
	
	// Settings
	debugMode         bool
	showWarnings      bool
	captureServer     bool
	captureClient     bool
	
	// Log buffer
	logBuffer         []string
	logMutex          sync.Mutex
	maxLogs           int
	
	updateTicker      *time.Ticker
	stopChan          chan struct{}
}

func NewDashboard(p *proxy.Proxy, captureServer, captureClient bool) *Dashboard {
	app := tview.NewApplication()
	
	d := &Dashboard{
		app:           app,
		proxy:         p,
		stats:         NewStats(),
		debugMode:     false,
		showWarnings:  true,
		captureServer: captureServer,
		captureClient: captureClient,
		logBuffer:     make([]string, 0),
		maxLogs:       100,
		stopChan:      make(chan struct{}),
	}
	
	d.buildUI()
	return d
}

func (d *Dashboard) buildUI() {
	// Stats panel
	d.statsView = tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(false)
	d.statsView.SetBorder(true).SetTitle(" Statistics ").SetTitleAlign(tview.AlignLeft)
	
	// Logs panel
	d.logsView = tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetChangedFunc(func() {
			d.app.Draw()
		})
	d.logsView.SetBorder(true).SetTitle(" Logs ").SetTitleAlign(tview.AlignLeft)
	
	// Connections panel
	d.connectionsView = tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true)
	d.connectionsView.SetBorder(true).SetTitle(" Active Connections ").SetTitleAlign(tview.AlignLeft)
	
	// Controls panel
	d.controlsView = tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(false)
	d.controlsView.SetBorder(true).SetTitle(" Controls ").SetTitleAlign(tview.AlignLeft)
	d.updateControlsView()
	
	// Status bar
	d.statusBar = tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter)
	d.statusBar.SetBackgroundColor(tcell.ColorDarkGreen)
	d.updateStatusBar()
	
	// Layout
	leftColumn := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(d.statsView, 12, 0, false).
		AddItem(d.controlsView, 10, 0, false).
		AddItem(d.connectionsView, 0, 1, false)
	
	mainFlex := tview.NewFlex().
		AddItem(leftColumn, 0, 1, false).
		AddItem(d.logsView, 0, 2, false)
	
	rootFlex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(mainFlex, 0, 1, false).
		AddItem(d.statusBar, 1, 0, false)
	
	// Key bindings
	d.app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Rune() {
		case 'd', 'D':
			d.toggleDebugMode()
			return nil
		case 'w', 'W':
			d.toggleWarnings()
			return nil
		case 's', 'S':
			d.toggleCaptureServer()
			return nil
		case 'c', 'C':
			d.toggleCaptureClient()
			return nil
		case 'l', 'L':
			d.clearLogs()
			return nil
		case 'q', 'Q':
			d.app.Stop()
			return nil
		}
		
		if event.Key() == tcell.KeyCtrlC {
			d.app.Stop()
			return nil
		}
		
		return event
	})
	
	d.app.SetRoot(rootFlex, true)
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
			d.updateStats()
			d.updateConnections()
			d.app.Draw()
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
	
	d.statsView.SetText(content)
}

func (d *Dashboard) updateConnections() {
	conns := d.proxy.GetActiveConnections()
	
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
	debugStatus := colorBool(d.debugMode, "ON", "OFF")
	warningsStatus := colorBool(d.showWarnings, "ON", "OFF")
	serverStatus := colorBool(d.captureServer, "ON", "OFF")
	clientStatus := colorBool(d.captureClient, "ON", "OFF")
	
	content := fmt.Sprintf(`[yellow]D[-] Debug Mode:    %s
[yellow]W[-] Show Warnings: %s
[yellow]S[-] Capture S→C:  %s
[yellow]C[-] Capture C→S:  %s
[yellow]L[-] Clear Logs
[yellow]Q[-] Quit`,
		debugStatus,
		warningsStatus,
		serverStatus,
		clientStatus,
	)
	
	d.controlsView.SetText(content)
}

func (d *Dashboard) updateStatusBar() {
	status := "[green]● ROProxy Running[-]"
	if !d.captureServer && !d.captureClient {
		status = "[red]● WARNING: All capture disabled[-]"
	}
	
	d.statusBar.SetText(status)
}

func (d *Dashboard) toggleDebugMode() {
	d.debugMode = !d.debugMode
	d.updateControlsView()
	d.updateStatusBar()
	d.Log("[yellow]Debug mode: %s[-]", colorBool(d.debugMode, "ON", "OFF"))
}

func (d *Dashboard) toggleWarnings() {
	d.showWarnings = !d.showWarnings
	d.updateControlsView()
	d.Log("[yellow]Warnings: %s[-]", colorBool(d.showWarnings, "ON", "OFF"))
}

func (d *Dashboard) toggleCaptureServer() {
	d.captureServer = !d.captureServer
	d.proxy.SetCaptureServer(d.captureServer)
	d.updateControlsView()
	d.updateStatusBar()
	d.Log("[cyan]Server→Client capture: %s[-]", colorBool(d.captureServer, "ON", "OFF"))
}

func (d *Dashboard) toggleCaptureClient() {
	d.captureClient = !d.captureClient
	d.proxy.SetCaptureClient(d.captureClient)
	d.updateControlsView()
	d.updateStatusBar()
	d.Log("[green]Client→Server capture: %s[-]", colorBool(d.captureClient, "ON", "OFF"))
}

func (d *Dashboard) clearLogs() {
	d.logMutex.Lock()
	d.logBuffer = make([]string, 0)
	d.logMutex.Unlock()
	d.logsView.Clear()
	d.Log("[gray]Logs cleared[-]")
}

func (d *Dashboard) Log(format string, args ...interface{}) {
	timestamp := time.Now().Format("15:04:05")
	msg := fmt.Sprintf("[gray]%s[-] %s", timestamp, fmt.Sprintf(format, args...))
	
	d.logMutex.Lock()
	d.logBuffer = append(d.logBuffer, msg)
	if len(d.logBuffer) > d.maxLogs {
		d.logBuffer = d.logBuffer[1:]
	}
	d.logMutex.Unlock()
	
	fmt.Fprintf(d.logsView, "%s\n", msg)
}

func (d *Dashboard) LogPacket(direction common.PacketDirection, opcode uint16, size int) {
	if !d.debugMode {
		return
	}
	
	var dirSymbol, color string
	if direction == common.ClientToServer {
		dirSymbol = "C→S"
		color = "green"
	} else {
		dirSymbol = "S→C"
		color = "cyan"
	}
	
	d.Log("[%s]%s[-] Packet [yellow]0x%04X[-] (%d bytes)", color, dirSymbol, opcode, size)
	d.stats.AddPacket(direction, size, false)
}

func (d *Dashboard) LogUnknown(direction common.PacketDirection, opcode uint16, size int) {
	if !d.showWarnings {
		return
	}
	
	var dirSymbol, color string
	if direction == common.ClientToServer {
		dirSymbol = "C→S"
		color = "green"
	} else {
		dirSymbol = "S→C"
		color = "cyan"
	}
	
	d.Log("[red]⚠[-] [%s]%s[-] Unknown packet [yellow]0x%04X[-] (%d bytes)", color, dirSymbol, opcode, size)
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
	return d.debugMode
}

func (d *Dashboard) IsShowWarnings() bool {
	return d.showWarnings
}
