# RO-Proxy Development Guide

## Project Overview

RO-Proxy is a transparent TCP proxy for Ragnarok Online that intercepts and analyzes game network traffic in real-time. It uses iptables/ipset for packet redirection and provides a terminal-based UI (TUI) dashboard for monitoring.

**Hot-Swappable Architecture** — The project uses a two-process IPC design that allows restarting packet analysis logic without disconnecting game clients.

## Build & Run

```powershell
# Build both binaries for Linux (from Windows)
$env:GOOS="linux"; $env:GOARCH="amd64"; go build -o proxy .\cmd\proxy
$env:GOOS="linux"; $env:GOARCH="amd64"; go build -o analyzer .\cmd\analyzer

# Run on Linux
# Terminal 1 - Proxy (requires sudo for iptables, run once, never restart)
sudo ./proxy

# Terminal 2 - Analyzer (can restart anytime for hot-swapping)
./analyzer
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

### Two-Process IPC Design

```
┌──────────────────────────────────────┐    Unix Domain Socket     ┌──────────────────────────────────────┐
│           PROXY PROCESS              │    ./roproxy.sock         │         ANALYZER PROCESS             │
│                                      │                           │                                      │
│  Client → iptables → Listener        │   ┌─────────────────┐     │    IPC Server                        │
│              ↓                       │   │  IPC Frame      │     │        ↓                             │
│       Connection Handler             │══>│  (RawChunk)     │════>│    Processor                         │
│      ┌──────┴──────┐                 │   │  Buffer: 10K    │     │        ↓                             │
│      ↓             ↓                 │   └─────────────────┘     │    Worker (per connection)           │
│  relay C→S    relay S→C              │                           │        ↓                             │
│      ↓             ↓                 │                           │    Parse packets → Deserialize       │
│   Recording   IPC Client             │                           │        ↓                             │
│                                      │                           │    Handler.Deserialize() → API       │
│  NEVER RESTARTS                      │                           │    TUI Dashboard                     │
│  (keeps connections alive)           │                           │    HOT-SWAPPABLE (restart anytime)   │
└──────────────────────────────────────┘                           └──────────────────────────────────────┘
```

### IPC Frame Protocol

Binary frame format for communication between Proxy and Analyzer:

```
┌────────────┬─────────┬──────────┬──────────┬──────────┬───────────┬───────────┬─────────────┬─────────────┬───────────┬──────┐
│ MagicNum   │ Version │ FrameLen │ Type     │ ConnID   │ Timestamp │ Direction │ ClientAddr  │ ServerAddr  │ DataLen   │ Data │
│ 2B (0x524F)│ 1B      │ 4B       │ 1B       │ 8B       │ 8B        │ 1B        │ 1B+N bytes  │ 1B+M bytes  │ 4B        │ N B  │
└────────────┴─────────┴──────────┴──────────┴──────────┴───────────┴───────────┴─────────────┴─────────────┴───────────┴──────┘
```

- **MagicNumber**: `0x524F` ("RO" in ASCII) for validation
- **Type**: Frame type - `0x00` DATA (packet), `0x01` CONN_OPEN, `0x02` CONN_CLOSE
- **Direction**: `0x00` = Client→Server, `0x01` = Server→Client
- **ClientAddr/ServerAddr**: UTF-8 encoded "IP:port" strings (prefixed with length byte)

### Package Structure

- **`cmd/proxy`**: Proxy entry point (never restarts)
- **`cmd/analyzer`**: Analyzer entry point (hot-swappable)
- **`internal/ipc`**: 
  - `frame.go`: Binary frame encoding/decoding
  - `client.go`: Non-blocking sender with ring buffer (used by Proxy)
  - `server.go`: Unix socket listener (used by Analyzer)
- **`internal/proxy`**: 
  - `proxy.go`: Listener and connection lifecycle
  - `connection.go`: Per-connection state, dual relay goroutines, sends to IPC
  - `iptables.go`: Sets up traffic redirection using ipset/iptables
  - `recording.go`: Packet capture (always in Proxy for continuity)
  - `monitoring.go`: Stats and diagnostics
- **`internal/analyzer`**: 
  - `processor.go`: Receives frames from IPC, manages workers
  - `worker.go`: Packet parsing pipeline, manages deserialization goroutines
- **`internal/packets`**: 
  - `parser.go`: Packet structures (RawChunk, ParsedPacket)
  - `context.go`: Map location tracking (state lives in Analyzer, rebuilds on restart)
  - `receive/`: Server→Client packet handlers
  - `send/`: Client→Server packet handlers
- **`internal/tui`**: Terminal UI using tview/tcell (runs in Analyzer)
- **`internal/config`**: JSON config loader with validation
- **`internal/common`**: Shared utilities (logging, API client, formatting)

### Key Design Patterns

#### Hot-Swap Workflow
1. **Proxy** starts first, sets up iptables, listens for connections
2. **Analyzer** connects to `./roproxy.sock`, starts receiving frames
3. To hot-swap: stop Analyzer, recompile with changes, restart Analyzer
4. Proxy buffers up to 10,000 frames while Analyzer is down (drops oldest if full)
5. Analyzer rebuilds state (MapLocation, etc.) by listening to packets

#### Connection Lifecycle (Proxy)
Each client connection spawns:
1. **2 relay goroutines**: `relayClientToServer` and `relayServerToClient` - forward raw bytes
2. **IPC sender**: Copies raw bytes to IPC frame and sends to Analyzer (non-blocking)
3. **Recording**: If enabled, writes raw chunks to `recordings/` directory
4. **Connection events**: Sends `CONN_OPEN` frame on start, `CONN_CLOSE` on close (with timestamp)

All goroutines share a context that cancels on connection close.

#### Packet Processing (Analyzer)
The Analyzer tracks active connections via `CONN_OPEN`/`CONN_CLOSE` frames:
1. **Processor**: Routes frames to appropriate worker, tracks connection info (start time, addresses)
2. **Worker goroutine** (per connection): Parses raw bytes into complete packets
3. **N deserializer goroutines**: One per packet, semaphore-limited to 100 concurrent per connection
4. **Discord notifications**: Sends webhook on connection close with duration and map info

#### Packet Handling
1. **RawChunk**: Raw TCP data from IPC, not yet parsed
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
The Analyzer tracks which in-game map each connection is on:
- `packets.SetConnectionMap(connID, mapName)`: Store map for a connection
- `packets.GetConnectionMap(connID)`: Retrieve current map
- State rebuilds automatically when Analyzer restarts by listening to map-change packets

#### Recording System
Recording lives in the Proxy process (`proxy/recording.go`) for continuity:
- `IsRecording()`: Check if recording is active
- Raw chunks are written to `recordings/` directory
- **File Flag Control**: Analyzer creates/deletes `.recording_enabled` file to toggle recording
- Proxy watches this file every 500ms to sync state
- This design allows hot-swapping the Analyzer without losing recording continuity

#### IPC Buffer Strategy
The Proxy uses a ring buffer (10,000 frames) when sending to Analyzer:
- If Analyzer is down or slow, frames are buffered
- If buffer fills, oldest frames are dropped (analysis is best-effort)
- This ensures Proxy never blocks and game connections stay alive

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
- In the Analyzer process, logs are queued and consumed by the TUI
- The Proxy process logs to stdout (no TUI)

### Linux-Specific Requirements

- **iptables/ipset**: Required for packet redirection (verified in `iptables.go`)
- **SO_ORIGINAL_DST**: Socket option to retrieve pre-NAT destination (defined in `proxy/syscall_linux.go`)
- **Root privileges**: Needed for iptables manipulation (only Proxy requires sudo)
- **Unix Domain Sockets**: Used for IPC (`./roproxy.sock`)
- Cleanup handled via defer in Proxy's `main.go`

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

- **Panic Recovery**: Worker loop (in Analyzer) has auto-recovery to prevent crashes - use this pattern for critical goroutines
- **Context Propagation**: Always pass `ctx` from Connection.Start() to goroutines for proper cleanup
- **IPC Buffer**: Proxy's IPC ring buffer is sized at 10,000 frames - oldest dropped if Analyzer can't keep up
- **Analyzer Buffer**: Worker's RawChunkBuffer is sized at 100,000 - adjust if high-traffic scenarios drop packets
- **Byte Order**: All packet integers are **little-endian** (use `binary.LittleEndian`)
- **String Parsing**: Use `common.HexStringToString()` for null-terminated strings in packets
- **Semaphore Limits**: Worker uses weighted semaphore (100) to prevent goroutine explosion
- **Connection IDs**: Use atomic counter (`atomic.Uint64`) for thread-safe ID generation

## TUI Dashboard Controls (runs in Analyzer)

- **V**: Cycle verbosity (Info → Verbose → Very Verbose)
- **F**: Filter logs by connection ID (input prompt)
- **R**: Toggle recording (creates/deletes `.recording_enabled` file for Proxy)
- **L**: Clear logs
- **Q**: Quit Analyzer (Proxy keeps running)
- **Ctrl+C**: Force quit

## Windows Compatibility

This project is **Linux-only** at runtime due to:
- iptables/ipset dependencies
- SO_ORIGINAL_DST socket option (see `syscall_linux.go`)
- Unix Domain Sockets for IPC

**Development workflow**: Code is typically written on Windows and cross-compiled for Linux using `GOOS=linux GOARCH=amd64`.
