package proxy

import (
    "sync/atomic"
    "roproxy/internal/packets"
)

// Global queues and state for proxy operations.
var (
    // GlobalAPIQueue receives packets that need API calls (e.g., item lookups).
    // Single consumer (API goroutine) reads from this queue.
    GlobalAPIQueue chan *packets.ParsedPacket

    // GlobalRecording indicates if recording mode is active.
    // When true, deserializers write raw bytes to recording file.
    GlobalRecording atomic.Bool
)

// InitializeGlobalQueues creates and initializes the global queues.
// Must be called once at startup (from main).
func InitializeGlobalQueues() {
    GlobalAPIQueue = make(chan *packets.ParsedPacket, 100000)
    GlobalRecording.Store(false)
}

// SendToAPI sends a packet to the API queue (non-blocking).
// CRITICAL: Crashes with panic if queue is full (fail-fast philosophy).
func SendToAPI(pkt *packets.ParsedPacket) {
    select {
    case GlobalAPIQueue <- pkt:
        // Success
    default:
        panic("CRITICAL: API queue overflow (capacity: 100,000) - API consumer cannot keep up")
    }
}

// IsRecording returns true if recording mode is active.
func IsRecording() bool {
    return GlobalRecording.Load()
}

// SetRecording enables or disables recording mode.
func SetRecording(enabled bool) {
    GlobalRecording.Store(enabled)
}
