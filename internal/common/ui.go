package common

// GlobalUIQueue receives all deserialized packets for display.
// Type is interface{} to avoid import cycle (packets imports common).
// Actual type is *packets.ParsedPacket but typed as interface{} here.
var GlobalUIQueue chan interface{}

// InitializeGlobalUIQueue creates the global UI queue.
// Must be called once at startup (from main).
func InitializeGlobalUIQueue() {
    GlobalUIQueue = make(chan interface{}, 100000)
}

// SendPacketToUI sends a packet to the UI queue (non-blocking).
// CRITICAL: Crashes with log.Fatal() if queue is full (fail-fast philosophy).
// pkt should be *packets.ParsedPacket but typed as interface{} to avoid import cycle.
func SendPacketToUI(pkt interface{}) {
    select {
    case GlobalUIQueue <- pkt:
        // Success
    default:
        LogToUI("[red]CRITICAL: UI queue overflow (capacity: 100,000) - UI consumer cannot keep up[-]")
        // Give UI 100ms to display the error message
        // time.Sleep(100 * time.Millisecond)
        panic("CRITICAL: UI queue overflow - system cannot process packets fast enough")
    }
}
