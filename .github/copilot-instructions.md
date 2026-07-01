# RO-Proxy Development Guide

## Project Overview

RO-Proxy is a transparent TCP proxy for Ragnarok Online that intercepts and analyzes game network traffic in real-time. It uses iptables/ipset for packet redirection and provides a terminal-based UI (TUI) dashboard for monitoring.

## Build & Run

```powershell
# Build for Linux (from Windows)
$env:GOOS="linux"; $env:GOARCH="amd64"; go build -o roproxy .\cmd\roproxy

# Run on Linux (requires sudo for iptables)
sudo ./roproxy
```

Requires `config.json` in the working directory with:
- `listen_port`: Proxy listening port
- `target_ips`: Array of Ragnarok Online server IPs to intercept
- `api` (optional): External API configuration (url, key)
- `discord_webhook` (optional): Discord webhook URL for notifications

## Testing

```bash
# Run all tests
go test ./test/...

# Run a specific test
go test ./test -run TestSystemChatTwitch

# Run tests in verbose mode
go test -v ./test/...
```

Tests use a hex-based mock system defined in `test/helpers.go`. Use `createMockParsedPacket(hexString)` to test packet deserialization.

## Architecture

### Request Flow

```
Client → iptables REDIRECT → Proxy Listener → Connection Handler
                                                    ↓
                                    ┌───────────────┴───────────────┐
                                    ↓                               ↓
                            relayClientToServer          relayServerToClient
                                    ↓                               ↓
                                RawChunkBuffer (chan *packets.RawChunk)
                                    ↓
                                Worker (goroutine per connection)
                                    ↓
                            Parse complete packets
                                    ↓
                            Deserialize (semaphore-limited goroutines)
                                    ↓
                            Handler.Deserialize() → API/Logging
```

### Package Structure

- **`cmd/roproxy`**: Entry point, initializes services
- **`internal/proxy`**: 
  - `proxy.go`: Listener and connection lifecycle
  - `connection.go`: Per-connection state, dual relay goroutines
  - `worker.go`: Packet parsing pipeline, manages deserialization goroutines
  - `iptables.go`: Sets up traffic redirection using ipset/iptables
  - `monitoring.go`: Stats and diagnostics
- **`internal/packets`**: 
  - `parser.go`: Packet structures (RawChunk, ParsedPacket)
  - `context.go`: Map location tracking system (matches connections to in-game maps)
  - `receive/`: Server→Client packet handlers
  - `send/`: Client→Server packet handlers
- **`internal/tui`**: Terminal UI using tview/tcell
- **`internal/config`**: JSON config loader with validation
- **`internal/common`**: Shared utilities (logging, API client, formatting)

### Key Design Patterns

#### Connection Lifecycle
Each client connection spawns:
1. **2 relay goroutines**: `relayClientToServer` and `relayServerToClient` - pass raw bytes through a channel
2. **1 worker goroutine**: Parses raw bytes into complete packets
3. **N deserializer goroutines**: One per packet, semaphore-limited to 100 concurrent per connection

All goroutines share a context that cancels on connection close.

#### Packet Handling
1. **RawChunk**: Raw TCP data from relay, not yet parsed
2. **ParsedPacket**: Complete packet (opcode + payload extracted)
3. **Handler Interface**: Each packet type implements `Deserialize() map[string]any`

Packet handlers are registered in `receive/receive_packets.go` and `send/send_packets.go` as `PacketDatabase` maps (opcode → PacketSpec).

#### Packet Size Types
Defined in `common/base.go`:
- `FIXED`: Known size (e.g., 14 bytes)
- `INDICATED_IN_PACKET`: First 2 bytes after opcode contain total packet length
- `HTTP`: Special handling for HTTP responses (opcode 0x5448)
- `UNKNOWN`: Not yet implemented

#### Map Location System
The proxy tracks which in-game map each connection is on:
- `packets.SetConnectionMap(connID, mapName)`: Store map for a connection
- `packets.GetConnectionMap(connID)`: Retrieve current map
- Used for context-aware packet handling and API calls

#### Recording System
`proxy/recording.go` handles packet capture:
- `IsRecording()`: Check if recording is active
- Raw chunks are written to `recordings/` directory
- Controlled via TUI dashboard (R key)

#### API Integration
`common/api.go` provides async API calls:
- `SendToAPIInternal(endpoint, payload)`: Non-blocking API POST
- Configured via `config.json` (url + key)
- Used by packet handlers to report events

#### Logging System
`common/logging.go` provides structured logging:
- **Categories**: LogProxy, LogMonitor, LogRecord, LogPacket, LogAPI, LogUI
- **Levels**: LogInfo, LogWarning, LogError, LogVerbose, LogVeryVerbose
- Use `common.Log(category, level, format, args...)` throughout the codebase
- Logs are queued and consumed by the TUI

### Linux-Specific Requirements

- **iptables/ipset**: Required for packet redirection (verified in `iptables.go`)
- **SO_ORIGINAL_DST**: Socket option to retrieve pre-NAT destination (defined in `proxy.go`)
- **Root privileges**: Needed for iptables manipulation
- Cleanup handled via defer in `main.go`

## Adding New Packets

1. **Create packet struct** in `internal/packets/receive/` or `internal/packets/send/`:
```go
type YourPacket struct {
    ParsedPacket packets.ParsedPacket
}

func (p *YourPacket) Deserialize() map[string]any {
    // Parse p.ParsedPacket.Payload
    return map[string]any{
        "field1": value1,
    }
}
```

2. **Register in PacketDatabase**:
```go
// receive_packets.go or send_packets.go
0x1234: {
    Desc: "Your Packet",
    Size: 42,  // or -1 for INDICATED_IN_PACKET
    Type: common.FIXED,
    Handler: &YourPacket{},
    Alert: false,
},
```

3. **Add test** in `test/`:
```go
func TestYourPacket(t *testing.T) {
    hexString := "01 02 03 04"
    parsedPacket, err := createMockParsedPacket(hexString)
    // ...
    yourPacket := receive.YourPacket{ParsedPacket: parsedPacket}
    data := testPacketDeserialize(&yourPacket)
    // Assertions
}
```

## Common Gotchas

- **Panic Recovery**: Worker loop has auto-recovery to prevent proxy crashes - use this pattern for critical goroutines
- **Context Propagation**: Always pass `ctx` from Connection.Start() to goroutines for proper cleanup
- **Buffer Sizes**: `RawChunkBuffer` is sized at 100,000 - adjust if high-traffic scenarios drop packets
- **Byte Order**: All packet integers are **little-endian** (use `binary.LittleEndian`)
- **String Parsing**: Use `common.HexStringToString()` for null-terminated strings in packets
- **Semaphore Limits**: Worker uses weighted semaphore (100) to prevent goroutine explosion
- **Connection IDs**: Use atomic counter (`atomic.Uint64`) for thread-safe ID generation

## TUI Dashboard Controls

- **V**: Cycle verbosity (Info → Verbose → Very Verbose)
- **T**: Toggle timestamp format (relative vs full)
- **R**: Toggle recording
- **F**: Filter logs by connection ID (input prompt)
- **C**: Clear connection filter
- **Q**: Quit proxy

## Windows Compatibility

This project is **Linux-only** at runtime due to:
- iptables/ipset dependencies
- SO_ORIGINAL_DST socket option (see `syscall_linux.go`)

**Development workflow**: Code is typically written on Windows and cross-compiled for Linux using `GOOS=linux GOARCH=amd64`.
