package common

import "fmt"

// GlobalLogQueue receives log messages for display in UI.
// Allows any component to log to UI without direct dashboard reference.
var GlobalLogQueue chan string

// InitializeGlobalLogQueue creates the global log queue.
// Must be called once at startup (from main).
func InitializeGlobalLogQueue() {
    GlobalLogQueue = make(chan string, 10000)
}

// LogToUI sends a log message to the UI (non-blocking).
// This allows any component to log to the UI without needing a dashboard reference.
// Uses fmt.Sprintf formatting.
func LogToUI(format string, args ...interface{}) {
    msg := fmt.Sprintf(format, args...)
    select {
    case GlobalLogQueue <- msg:
    default:
        // Queue full, drop message
    }
}
