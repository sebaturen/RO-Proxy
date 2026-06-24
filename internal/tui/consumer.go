package tui

import (
    "roproxy/internal/common"
)

// StartUIConsumer starts the global UI consumer goroutine.
// This goroutine reads from GlobalLogQueue and displays logs.
func StartUIConsumer(dashboard *Dashboard) {
    go logConsumerLoop(dashboard)
}

// logConsumerLoop reads from GlobalLogQueue and displays logs in UI.
// Uses drain pattern: blocks for first message, then drains all available messages
// and updates UI in a single batch for better performance.
func logConsumerLoop(dashboard *Dashboard) {
    for {
        // 1. Wait for first message (BLOCKS)
        msg, ok := <-common.GlobalLogQueue
        if !ok {
            return // Channel closed
        }
        
        // 2. Accumulate this message if it passes filter
        batch := make([]string, 0, 100)
        if shouldDisplayLog(msg.Level, dashboard.verbosityLevel) {
            batch = append(batch, msg.Format())
        }
        
        // 3. Drain ALL additional messages that are ready (NON-BLOCKING)
        drained := false
        for !drained {
            select {
            case msg2, ok := <-common.GlobalLogQueue:
                if !ok {
                    drained = true
                    break
                }
                if shouldDisplayLog(msg2.Level, dashboard.verbosityLevel) {
                    batch = append(batch, msg2.Format())
                }
            default:
                // No more messages available, exit drain loop
                drained = true
            }
        }
        
        // 4. Write ALL logs in a single UI update
        if len(batch) > 0 {
            dashboard.LogBatch(batch)
        }
    }
}

// shouldDisplayLog determines if a log message should be displayed based on level and verbosity
func shouldDisplayLog(level common.LogLevel, verbosity VerbosityLevel) bool {
    // Always show errors, warnings, and info
    if level == common.LogError || level == common.LogWarning || level == common.LogInfo {
        return true
    }
	
    // Verbose logs require at least Verbose level
    if level == common.LogVerbose {
        return verbosity == VerbosityVerbose || verbosity == VerbosityVeryVerbose
    }
	
    // VeryVerbose logs require VeryVerbose level
    if level == common.LogVeryVerbose {
        return verbosity == VerbosityVeryVerbose
    }
	
    return true
}
