package proxy

import (
    "runtime"
    "time"
    
    "roproxy/internal/common"
)

type MonitoringStats struct {
    UIQueueSize       int
    APIQueueSize      int
    ActiveGoroutines  int
    MemoryUsageMB     uint64
    MemoryAllocMB     uint64
}

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
    go monitoringLoop()
    common.LogToUI("[green][MONITOR] Monitoring started (interval: 1 minute)[-]")
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
    
    uiQueueSize := 0
    if common.GlobalUIQueue != nil {
        uiQueueSize = len(common.GlobalUIQueue)
    }
    
    apiQueueSize := 0
    if GlobalAPIQueue != nil {
        apiQueueSize = len(GlobalAPIQueue)
    }
    
    apiConsumer := common.GetAPIConsumer()
    if apiConsumer != nil {
        apiQueueSize = apiConsumer.QueueSize()
    }
    
    return MonitoringStats{
        UIQueueSize:      uiQueueSize,
        APIQueueSize:     apiQueueSize,
        ActiveGoroutines: runtime.NumGoroutine(),
        MemoryUsageMB:    memStats.Sys / 1024 / 1024,
        MemoryAllocMB:    memStats.Alloc / 1024 / 1024,
    }
}

func checkThresholds(stats MonitoringStats) {
    warningThreshold := (bufferCapacity * bufferWarningPercent) / 100
    
    // Check UI queue
    if stats.UIQueueSize > warningThreshold {
        percent := (stats.UIQueueSize * 100) / bufferCapacity
        common.LogToUI("[yellow][MONITOR] WARNING: UI queue at %d%% capacity (%d/%d packets)[-]", 
            percent, stats.UIQueueSize, bufferCapacity)
    }
    
    // Check API queue
    if stats.APIQueueSize > warningThreshold {
        percent := (stats.APIQueueSize * 100) / bufferCapacity
        common.LogToUI("[yellow][MONITOR] WARNING: API queue at %d%% capacity (%d/%d items)[-]", 
            percent, stats.APIQueueSize, bufferCapacity)
    }
    
    // Check memory
    if stats.MemoryUsageMB > memoryWarningMB {
        common.LogToUI("[red][MONITOR] WARNING: Memory usage at %d MB (threshold: %d MB)[-]", 
            stats.MemoryUsageMB, memoryWarningMB)
    }
    
    // Log stats (verbose info)
    common.LogToUI("[gray][MONITOR] Stats - Goroutines: %d, Memory: %d MB (Alloc: %d MB), UI Queue: %d, API Queue: %d[-]",
        stats.ActiveGoroutines, stats.MemoryUsageMB, stats.MemoryAllocMB, stats.UIQueueSize, stats.APIQueueSize)
}

// GetCurrentStats returns current monitoring statistics for display.
// Can be called from dashboard or other components for real-time stats.
func GetCurrentStats() MonitoringStats {
    return collectStats()
}
