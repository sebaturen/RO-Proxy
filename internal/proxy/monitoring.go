package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"

	"roproxy/internal/common"
	"roproxy/internal/config"
)

type MonitoringStats struct {
    APIQueueSize      int
    ActiveGoroutines  int
    MemoryUsageMB     uint64
    MemoryAllocMB     uint64
}

type PacketStat struct {
    Direction common.PacketDirection
    Size int
    Unknown bool
}

type GlobalStats struct {
    StartTime           time.Time
    TotalPackets        uint64
    ClientToServer      uint64
    ServerToClient      uint64
    UnknownPackets      uint64
    BytesClientToServer uint64
    BytesServerToClient uint64
    StatQueue           chan *PacketStat
}
var globalStats *GlobalStats
var cfg *config.Config

const (
    bufferCapacity       = 100000
    bufferWarningPercent = 50
    memoryWarningMB      = 8192 // 8GB
    monitoringInterval   = 1 * time.Minute
)

type DiscordMessage struct {
    Content   string `json:"content"`
    Username  string `json:"username,omitempty"`
    AvatarURL string `json:"avatar_url,omitempty"`
}

// StartMonitoring starts a background goroutine that monitors system health.
// Checks buffer sizes, memory usage, and goroutine count every 1 minute.
// Logs warnings when thresholds are exceeded but does NOT crash (crash happens on overflow).
func StartMonitoring(inCfg *config.Config) {
    globalStats = &GlobalStats{
        StartTime: time.Now(),
        StatQueue: make(chan *PacketStat, 1000),
    }
    cfg = inCfg
    go monitoringStatusLoop()
    go monitoringLoop()
    common.Log(common.LogMonitor, common.LogInfo, "Monitoring started (interval: 1 minute)")
}

func monitoringStatusLoop() {
    for pkstat := range globalStats.StatQueue {
        globalStats.TotalPackets++
        if pkstat.Direction == common.ClientToServer {
            globalStats.ClientToServer++
            globalStats.BytesClientToServer += uint64(pkstat.Size)
        } else {
            globalStats.ServerToClient++
            globalStats.BytesServerToClient += uint64(pkstat.Size)
        }
        
        if pkstat.Unknown {
            globalStats.UnknownPackets++
        }
    }
}

func GetGlobalStats() *GlobalStats {
    return globalStats
}

func monitoringLoop() {
    ticker := time.NewTicker(monitoringInterval)
    defer ticker.Stop()
    
    for range ticker.C {
        stats := collectStats()
        checkThresholds(stats)
    }
}

func collectStats() MonitoringStats {
    var memStats runtime.MemStats
    runtime.ReadMemStats(&memStats)
    
    apiQueueSize := 0
    apiConsumer := common.GetAPIConsumer()
    if apiConsumer != nil {
        apiQueueSize = apiConsumer.QueueSize()
    }
    
    return MonitoringStats{
        APIQueueSize:     apiQueueSize,
        ActiveGoroutines: runtime.NumGoroutine(),
        MemoryUsageMB:    memStats.Sys / 1024 / 1024,
        MemoryAllocMB:    memStats.Alloc / 1024 / 1024,
    }
}

func checkThresholds(stats MonitoringStats) {
    warningThreshold := (bufferCapacity * bufferWarningPercent) / 100
    
    // Check API queue
    if stats.APIQueueSize > warningThreshold {
        percent := (stats.APIQueueSize * 100) / bufferCapacity
        common.Log(common.LogMonitor, common.LogWarning, "API queue at %d%% capacity (%d/%d items)", percent, stats.APIQueueSize, bufferCapacity)
    }
    
    // Check memory
    if stats.MemoryUsageMB > memoryWarningMB {
        common.Log(common.LogMonitor, common.LogWarning, "Memory usage at %d MB (threshold: %d MB)", stats.MemoryUsageMB, memoryWarningMB)
    }
    
    // Log stats (verbose info)
    common.Log(common.LogMonitor, common.LogVerbose, "Stats - Goroutines: %d, Memory: %d MB (Alloc: %d MB), API Queue: %d", stats.ActiveGoroutines, stats.MemoryUsageMB, stats.MemoryAllocMB, stats.APIQueueSize)
}

func AddPacket(direction common.PacketDirection, size int, unknown bool) {
    pk := &PacketStat{
        Direction: direction,
        Size: size,
        Unknown: unknown,
    }

    select {
    case globalStats.StatQueue <- pk:
        // success
    default:
        // queue full
    }
}

func ReportCloseConnection(c *Connection) {
    if strings.TrimSpace(cfg.DiscordWebhook) == "" {
        common.Log(common.LogMonitor, common.LogVeryVerbose, "Discord configuration not set %s", cfg)
        return
    }

    duration := time.Since(c.StartTime)
    msg := DiscordMessage {
        Content: fmt.Sprintf("Connection %d was close [Duration: %s]", c.ID, duration),
    }
    payload, err := json.Marshal(msg)
    if err != nil {
        common.Log(common.LogMonitor, common.LogError, fmt.Sprintf("Error to parse JSON on connection close discord notification %v"), err)
        return
    }

    req, err := http.NewRequest("POST", cfg.DiscordWebhook, bytes.NewBuffer(payload))
    if err != nil {
        common.Log(common.LogMonitor, common.LogError, "Error on create discord notification request %v", err)
        return
    }

    req.Header.Set("Content-Type", "application/json")
    client := &http.Client{Timeout: 5 * time.Second}
    resp, err := client.Do(req)
    if err != nil {
        common.Log(common.LogMonitor, common.LogError, "Error on send discord notification %v", err)
        return
    }
    defer resp.Body.Close()

    // Discord responde con un Status 204 No Content si todo salió bien
    if resp.StatusCode == http.StatusNoContent {
        common.Log(common.LogMonitor, common.LogVeryVerbose, "MSG sended successfull")
    } else {
        common.Log(common.LogMonitor, common.LogVeryVerbose, "Error send notification: %d", resp.StatusCode)
    }
}