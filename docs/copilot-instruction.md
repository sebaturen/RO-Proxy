# ROProxy - Transparent TCP Proxy for Ragnarok Online with Packet Inspection

## Project Overview

ROProxy is a transparent TCP proxy written in Go, designed to intercept, inspect, and relay Ragnarok Online game traffic between clients and servers. The primary goal is to enable packet capture, analysis, and eventual packet injection while maintaining complete transparency to both client and server.

**Current Status:** Phase 2 - Packet Inspection & Analysis (capturing and decoding RO packets with security byte detection)

## Architecture

### Network Topology
```
Windows Host (Client)     Router/Gateway          VM Linux (Proxy)       Internet (Target)
192.168.10.X         →    DNAT Rules         →    192.168.10.Y:20000  →  172.65.x.x:10008
                          Forward specific IPs     iptables REDIRECT      RO Servers
                          Client connects to       Transparent relay
                          RO server IPs           + Packet capture
```

### Key Components

1. **Router DNAT (Destination NAT)**
   - Intercepts connections to specific RO server IPs (configured via ipset)
   - Forwards to VM Linux IP on proxy port (20000)
   - Transparent to client (client thinks it's connecting to real server)

2. **VM Linux + iptables**
   - Receives forwarded traffic
   - iptables REDIRECT rule sends to local proxy port
   - ipset manages list of RO server IPs dynamically
   - Kernel handles all TCP/IP rewriting automatically

3. **ROProxy Application**
   - Go application running on VM
   - Uses SO_ORIGINAL_DST to retrieve real destination
   - Establishes connection to actual target server
   - Bidirectional relay: Client ↔ Proxy ↔ Server
   - **TCP Stream Reassembly:** Handles fragmented packets
   - **Packet Parser:** Extracts complete RO packets from TCP stream
   - **Protocol Decoder:** Deserializes RO packet structures
   - **Security Byte Detection:** Identifies custom security byte appended to client packets
   - Connection tracking for future packet injection

### Transparency Mechanism

**Linux kernel netfilter (iptables) provides:**
- Automatic source/destination address rewriting
- TCP sequence number tracking
- Checksum recalculation
- Connection state management

**Proxy only needs to:**
- Accept connections on proxy port
- Read original destination with `getsockopt(SO_ORIGINAL_DST)`
- Connect to real target
- Copy data bidirectionally
- Parse packets from TCP stream (non-intrusive observation)

## Technical Requirements

### Target Game
- **Game:** Ragnarok Online Latin America (ROLatam)
- **Protocol:** TCP with custom packet format
- **Packet Structure:** Little-endian, variable length, opcode-based
- **Security:** Custom security byte appended to client→server packets
- **Ports:** Multiple ports (6900, 6951, 10008, etc.)
- **Concurrent Connections:** Supports multiple simultaneous client connections

### Proxy Specifications
- **Language:** Go (for simplicity, performance, and robust networking)
- **Platform:** Linux VM (Ubuntu/Debian recommended)
- **Kernel Requirements:** Linux 2.4+ (netfilter support)
- **Go Version:** 1.20+ recommended
- **External Dependencies:** None (uses only Go standard library)

### Packet Protocol Details

#### Packet Structure
```
[2 bytes: Opcode (little-endian)] [N bytes: Payload] [1 byte: Security Byte (client only)]
```

- **Server→Client:** No security byte
- **Client→Server:** Has 1-byte security byte appended (custom obfuscation)

#### Security Byte
- Added by ROLatam client as anti-bot/cheat measure
- Algorithm currently unknown (under investigation)
- Based on packet counter + packet data
- Required for packet injection to work
- OpenKore and GordoKore both struggled with this (OpenKore: "somekind of packet encryption")
- GordoKore uses external TCP server (port 2349) to calculate checksum

#### Packet Size Types
1. **FIXED:** Size known from opcode definition
2. **INDICATED_IN_PACKET:** Size in first 2 bytes after opcode
3. **INDICATED_SIZE_MINUS_FOUR:** Size in bytes 2-3 minus 4
4. **DYNAMIC_SIZE_PACKET:** Size from server-sent info (e.g., inventory)

## Code Style and Rules

### General Guidelines
- **Language:** English only (code, comments, variables, documentation)
- **Comments:** Only for complex logic; avoid obvious comments
- **Error Handling:** Fail fast philosophy
  - Missing config file → crash immediately
  - Invalid configuration → crash immediately
  - Network errors during startup → crash immediately
  - NO silent failures, NO excessive try/catch safety nets
  - If programmer made a mistake, program should break loudly
- **Concurrency:** Designed for unlimited concurrent connections
- **Scalability:** One goroutine pair per connection, efficient for thousands of connections

### Go-Specific
- Use standard library where possible
- Goroutines for connection handling (one per client-server pair)
- Context for cancellation and timeouts
- No external dependencies unless absolutely necessary
- Clean shutdown on SIGINT/SIGTERM

### Configuration
- JSON format for simplicity
- Single config file: `config.json`
- Required fields must exist or crash
- Validate all IPs/ports at startup

## Project Goals

### Phase 1: Transparent Proxy ✅ COMPLETED
- [x] Accept connections on configured port
- [x] Retrieve original destination with SO_ORIGINAL_DST
- [x] Connect to real target server
- [x] Bidirectional relay
- [x] Handle multiple concurrent connections
- [x] ipset integration for dynamic IP management
- [x] Graceful shutdown

### Phase 2: Packet Inspection 🔄 IN PROGRESS
- [x] TCP stream reassembly
- [x] Packet boundary detection (opcode + size parsing)
- [x] Separate client and server packet buffers
- [x] Security byte detection for client packets
- [x] PacketDirection enum (ServerToClient / ClientToServer)
- [x] 302 client→server packet definitions (from OpenKore ServerType0.pm)
- [x] 150+ server→client packet definitions
- [x] Deserializers for common packets (login, map, vending, etc.)
- [x] API consumer for sending captured data to external service
- [x] Packet logging with verbose flag (--logs)
- [ ] **Security byte algorithm reverse engineering** ← CURRENT BLOCKER
- [ ] Packet injection testing

### Phase 3: Packet Injection (BLOCKED - needs security byte algorithm)
- [ ] Calculate security byte for client packets
- [ ] API endpoint for injection (HTTP/gRPC/socket TBD)
- [ ] Inject arbitrary payload into specific connection
- [ ] Send to target server as if from client
- [ ] Handle server response validation

### Phase 4: Integration (FUTURE)
- [ ] Communication protocol with Windows Sniffer application
- [ ] List active connections
- [ ] Inject payloads remotely from Windows host

## Code Structure

### Directory Layout
```
ROProxy/
├── cmd/
│   └── roproxy/
│       └── main.go              # Entry point
├── internal/
│   ├── common/
│   │   └── base.go              # Shared types (PacketDirection enum, etc.)
│   ├── config/
│   │   └── config.go            # Configuration loading
│   ├── proxy/
│   │   ├── proxy.go             # Main proxy server
│   │   ├── connection.go        # Per-connection handler
│   │   └── iptables.go          # iptables/ipset management
│   ├── packets/
│   │   ├── parser.go            # TCP stream → RO packets (StreamParser)
│   │   ├── processor.go         # Packet processing & handler invocation
│   │   ├── receive/             # Server→Client packets
│   │   │   ├── receive_packets.go       # Packet definitions
│   │   │   ├── actor_spawn.go           # Deserializers
│   │   │   ├── vender_items_lists.go
│   │   │   └── ... (150+ packets)
│   │   └── send/                # Client→Server packets
│   │       └── send_packets.go          # 302 packet definitions
│   └── api/
│       └── consumer.go          # HTTP API for external integration
├── build/
│   └── src/
│       └── config.json          # Runtime configuration
├── docs/
│   └── copilot-instruction.md   # This file
└── tools/
    ├── generate_send_packet_db.py       # Generates send_packets.go from ServerType0.pm
    ├── ServerType0.pm                    # OpenKore packet definitions
    ├── validate_checksum.py              # Test security byte algorithm
    ├── analyze_security_byte.py          # Statistical analysis
    └── debug_crc.py                      # Step-by-step CRC debugging
```

### Key Files

#### `internal/packets/parser.go` (StreamParser)
- Manages TCP stream reassembly
- Separate buffers for client and server directions
- Detects packet boundaries using opcode definitions
- Extracts security byte from client packets using intelligent lookahead
- Handles fragmentation across multiple TCP reads

#### `internal/packets/processor.go` (PacketProcessor)
- Processes parsed packets
- Invokes deserializers based on opcode
- Filters packets by direction (--capture-client, --capture-server flags)
- Sends to API consumer
- Verbose logging

#### `internal/proxy/connection.go`
- Per-connection goroutines (relayClientToServer, relayServerToClient)
- Calls parser.AppendData() with correct direction
- Handles connection lifecycle
- **Packet injection test:** Captures 0x0130 packet and reinjects after 1 second

#### `internal/common/base.go`
- `PacketDirection` enum: `ServerToClient = 0`, `ClientToServer = 1`
- Shared packet structures (PacketSpec, PacketSizeType)
- BaseDeserializer interface

## Setup Instructions

### VM Configuration
1. Linux VM with 2 network interfaces:
   - Interface 1: Bridge/NAT to host LAN (192.168.10.x)
   - Interface 2: NAT/Bridge to internet (for reaching target servers)

2. Enable IP forwarding:
   ```bash
   echo 1 > /proc/sys/net/ipv4/ip_forward
   sysctl -w net.ipv4.ip_forward=1
   ```

3. Configure iptables REDIRECT:
   ```bash
   iptables -t nat -A PREROUTING -p tcp --dport 20000 -j REDIRECT --to-port 20000
   # Or let proxy handle all forwarded traffic directly
   ```

### Router Configuration
Configure DNAT rules for each target server IP (any port):
```
iptables -t nat -A PREROUTING -d 172.65.200.86 -p tcp -j DNAT --to-destination 192.168.10.Y:20000
iptables -t nat -A PREROUTING -d 172.65.179.43 -p tcp -j DNAT --to-destination 192.168.10.Y:20000
# Repeat for all target IPs
```

Replace `192.168.10.Y` with actual VM IP.

Note: No port filtering in DNAT rules - proxy validates target IP and forwards to any port.

### Building and Running
```bash
cd ROProxy
go build -o roproxy cmd/roproxy/main.go

# Run without logs (silent mode)
sudo ./roproxy

# Run with verbose logging
sudo ./roproxy --logs
```

## Connection Lifecycle

1. **Client initiates:** `connect(172.65.200.86:10008)` (or any port)
2. **Router DNAT:** Rewrites destination to `192.168.10.Y:20000`
3. **Proxy accepts:** Receives connection on port 20000
4. **Get original dest:** `getsockopt(fd, SOL_IP, SO_ORIGINAL_DST)` returns `172.65.200.86:10008`
5. **Validate IP:** Check if `172.65.200.86` is in allowed `target_ips` list
6. **Connect to target:** Proxy establishes connection to `172.65.200.86:10008` (original IP:port)
7. **Bidirectional relay:** Copy data between client and server using goroutines
8. **Connection tracking:** Store connection metadata for future injection
9. **Clean shutdown:** Close both sockets when either side disconnects

## Known Limitations

- Requires root/CAP_NET_ADMIN for SO_ORIGINAL_DST socket option
- Linux-only (uses Linux-specific syscalls)
- UDP not supported (RO uses TCP only)

## Future Considerations

- **Packet Inspection:** Optionally parse RO protocol for debugging
- **Statistics:** Connection count, bytes transferred, latency
- **Injection Queue:** Buffer injected packets if server socket is busy
- **Performance:** Scales efficiently with Go goroutines, can handle thousands of concurrent connections

## Security Byte Investigation (Current Focus)

### Problem Statement
ROLatam client appends a custom security byte to every client→server packet. The server validates this byte and disconnects the client if incorrect (error: "input invalido"). This blocks packet injection functionality.

### What We Know
1. **Validation is strict:** Packet injection test (connection.go) captures 0x0130 packet and reinjects 1 second later → server rejects it immediately
2. **Algorithm is unknown:** OpenKore's CRC-8 implementation doesn't match (0% validation success)
3. **GordoKore solution:** Uses external TCP server (localhost:2349) to calculate checksums
   - Protocol: Send packet_data + counter (4 bytes) → Receive checksum (1 byte) + seed (8 bytes) + counter (4 bytes)
   - Counter increments with mask 0xFFF (12 bits, max 4095)
   - External server NOT in GordoKore repository (proprietary)
4. **Detection works:** Proxy correctly identifies and extracts security byte from all packets (logs show accurate detection)

### Evidence from Logs
```
[12] [C->S] [0x0130][Send Entering Vending] size=6 payload=3001BBCB1B00 security_byte=0x79
[5]  [C->S] [0x0130][Send Entering Vending] size=6 payload=3001BBCB1B00 security_byte=0x24
```
- Same packet (opening vendor shop) sent by 2 different clients at same moment
- **Different security bytes** → Algorithm depends on per-connection state (counter, seed, session data?)

### Attempted Algorithms (All Failed)
Tools created in `ROProxy/tools/`:
- `validate_checksum.py` - Tests OpenKore's CRC-8 algorithm → 0% match
- `analyze_security_byte.py` - Statistical analysis (no patterns found)
- `test_checksum.py` - Various checksum algorithms (XOR, SUM, CRC variants) → all failed
- `debug_crc.py` - Step-by-step CRC debugging
- `find_lcg_parameters.py` - LCG (Linear Congruential Generator) parameter search → no match

### OpenKore vs GordoKore
- **OpenKore (f3b1f1b commit):** Added ROLatam support, mentions "packet encryption" not resolved
  - Implements local CRC-8 algorithm that doesn't work
- **GordoKore (fork, worked 3 months ago):** Uses external checksum server
  - File: `plugins/LatamChecksum/LatamChecksum.pl`
  - Server address: `localhost:2349` (NOT found in repository)
  - Algorithm remains proprietary/private

### Possible Next Steps
1. **Find External Server:**
   - Search user's machine for process listening on port 2349
   - Look for standalone checksum calculators in other folders
   - Check GordoKore Discord/community for server binary
   - Search GitHub/GitLab for "ROLatam checksum" or "2349 server"

2. **Reverse Engineer Client:**
   - Disassemble ROLatam client .exe (obfuscated, difficult)
   - Find packet send routine and checksum calculation
   - Port algorithm to Go
   - High difficulty, but definitive solution

3. **Capture More Data:**
   - Log 500+ packets with full session context (S->C packets too)
   - Look for seed exchange during login/map change
   - Analyze counter behavior across connection lifecycle
   - Test if algorithm changes per account/character

4. **Test in Isolation:**
   - Create minimal test: capture login sequence + first few packets
   - Build Python/Go script to replay exactly as captured
   - Modify security byte to validate server's error response patterns
   - Narrow down which session state affects the byte

### Security Byte Detection Algorithm (Already Implemented)
Located in `internal/packets/parser.go` (lines 140-167):
```go
// After parsing packet payload, check remaining bytes
if len(buffer) == 1 {
    // Must be security byte (no next packet)
    securityByte = buffer[0]
} else if len(buffer) >= 2 {
    // Check if next 2 bytes form valid opcode
    nextOpcode := binary.LittleEndian.Uint16(buffer[0:2])
    if isValidOpcode(nextOpcode) {
        // Valid opcode = no security byte (next packet starts)
        securityByte = nil
    } else if len(buffer) >= 3 {
        // Check if bytes [1:3] form valid opcode
        nextOpcode = binary.LittleEndian.Uint16(buffer[1:3])
        if isValidOpcode(nextOpcode) {
            // Byte [0] is security byte
            securityByte = buffer[0]
        }
    }
}
```

### Current Status
- ✅ Detection working perfectly (all captured packets show correct security byte extraction)
- ❌ Algorithm unknown (blocking packet injection)
- 🔍 Investigation ongoing

## Debugging

### Test Connectivity
```bash
# From Windows client
telnet 172.65.200.86 10008

# Should connect through proxy transparently
# Check proxy logs for connection details
```

### Verify iptables
```bash
# On router
iptables -t nat -L -n -v | grep DNAT

# On VM
iptables -t nat -L -n -v | grep REDIRECT
ipset list ro-servers  # Check server IPs
```

### Check SO_ORIGINAL_DST
```bash
# Test if kernel provides original destination
ss -tn | grep 20000  # Should show established connections
```

### Capture Packet Logs
```bash
# Capture all packets (client + server)
sudo ./roproxy --logs > packets.log 2>&1

# Capture only client→server packets
sudo ./roproxy --logs --capture-client > client_packets.log 2>&1

# Capture only server→client packets
sudo ./roproxy --logs --capture-server > server_packets.log 2>&1
```

## Contact / Maintenance

This is a specialized tool for RO private server testing and development. The proxy maintains minimal state and is designed to be reliable and crash on misconfiguration rather than fail silently.

**Core principle:** Simple, transparent, fail-fast.
