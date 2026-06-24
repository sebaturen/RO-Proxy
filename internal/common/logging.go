package common

import (
	"fmt"
	"time"
)

// LogCategory represents the subsystem that generated the log
type LogCategory int

const (
	LogProxy LogCategory = iota
	LogMonitor
	LogRecord
	LogPacket
	LogAPI
	LogUI
)

func (c LogCategory) String() string {
	switch c {
	case LogProxy:
		return "Proxy"
	case LogMonitor:
		return "Monitor"
	case LogRecord:
		return "Record"
	case LogPacket:
		return "Packet"
	case LogAPI:
		return "API"
	case LogUI:
		return "UI"
	default:
		return "Unknown"
	}
}

// LogLevel represents the verbosity level of the log
type LogLevel int

const (
	LogInfo LogLevel = iota
	LogWarning
	LogError
	LogVerbose
	LogVeryVerbose
)

func (l LogLevel) String() string {
	switch l {
	case LogInfo:
		return "Info"
	case LogWarning:
		return "Warning"
	case LogError:
		return "Error"
	case LogVerbose:
		return "Verbose"
	case LogVeryVerbose:
		return "VeryVerbose"
	default:
		return "Unknown"
	}
}

// LogMessage represents a structured log entry
type LogMessage struct {
	Category  LogCategory
	Level     LogLevel
	Message   string
	Timestamp time.Time
}

// Format returns the formatted log message for display
func (lm *LogMessage) Format() string {
	time := FormatTimestamp(lm.Timestamp)
	category := FormatLogCategory(lm.Category)
	verbosity := FormatVerbosity(lm.Level)
	return fmt.Sprintf("%s %s%s %s", time, category, verbosity, lm.Message)
}

// GlobalLogQueue receives structured log messages from all components
var GlobalLogQueue chan *LogMessage

// InitializeGlobalLogQueue creates the global log queue
func InitializeGlobalLogQueue() {
	GlobalLogQueue = make(chan *LogMessage, 100000)
}

// Log sends a structured log message to the global queue
// Usage: common.Log(common.LogProxy, common.LogVerbose, "Connection #%d draining...", connID)
func Log(category LogCategory, level LogLevel, format string, args ...interface{}) {
	msg := &LogMessage{
		Category:  category,
		Level:     level,
		Message:   fmt.Sprintf(format, args...),
		Timestamp: time.Now(),
	}
	
	select {
	case GlobalLogQueue <- msg:
		// Success
	default:
		// Queue full - drop message (fail-fast philosophy)
		// Don't block on logging
	}
}
