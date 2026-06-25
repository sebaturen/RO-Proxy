package proxy

import (
    "runtime"
    "time"
    
    "roproxy/internal/common"
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

const (
    bufferCapacity       = 100000
    bufferWarningPercent = 50
    memoryWarningMB      = 8192 // 8GB
    monitoringInterval   = 1 * time.Minute
)

// StartMonitoring starts a background goroutine that monitors system health.
// Checks buffer sizes, memory usage, and goroutine count every 1 minute.
// Logs warnings when thresholds are exceeded but does NOT crash (crash happens on overflow).
func StartMonitoring() {
    globalStats = &GlobalStats{
        StartTime: time.Now(),
        StatQueue: make(chan *PacketStat, 1000),
    }
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