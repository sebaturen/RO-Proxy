package proxy

import (
    "bufio"
    "bytes"
    "context"
    "encoding/binary"
    "fmt"
    "net"
    "os"
    "reflect"
    "strconv"
    "strings"
    "sync"
    "time"

    "golang.org/x/sync/semaphore"
    "roproxy/internal/common"
    "roproxy/internal/packets"
    "roproxy/internal/packets/receive"
    "roproxy/internal/packets/send"
)

type Connection struct {
    ID           uint64
    ClientConn   net.Conn
    ServerConn   net.Conn
    ClientAddr   string
    ServerAddr   string
    StartTime    time.Time

    // Processing pipeline
    RawChunkBuffer chan *packets.RawChunk
    cancel         context.CancelFunc
    semaphore      *semaphore.Weighted
    wg             sync.WaitGroup
}

func NewConnection(id uint64, clientConn net.Conn, serverAddr string) (*Connection, error) {
    serverConn, err := net.DialTimeout("tcp", serverAddr, 10*time.Second)
    if err != nil {
        return nil, err
    }

    conn := &Connection{
        ID:         id,
        ClientConn: clientConn,
        ServerConn: serverConn,
        ClientAddr: clientConn.RemoteAddr().String(),
        ServerAddr: serverAddr,
        StartTime:  time.Now(),
        
        RawChunkBuffer: make(chan *packets.RawChunk, 100000),
        semaphore:      semaphore.NewWeighted(100),
    }

    return conn, nil
}

func (c *Connection) Start(ctx context.Context) {
    connCtx, cancel := context.WithCancel(ctx)
    c.cancel = cancel
    
    // Debug log connection addresses
    common.Log(common.LogProxy, common.LogVeryVerbose, "Connection #%d started: ClientAddr='%s', ServerAddr='%s'", c.ID, c.ClientAddr, c.ServerAddr)
    
    c.wg.Add(3)
    go c.relayClientToServer(connCtx)
    go c.relayServerToClient(connCtx)
    go c.workerLoop(connCtx)
}

func (c *Connection) Wait() {
    c.wg.Wait()
}

func (c *Connection) Close() {
    // Cancel all goroutines (relays will stop reading/writing)
    if c.cancel != nil {
        c.cancel()
    }
    
    // Close network connections (forces relays to exit if still blocked on Read)
    c.ClientConn.Close()
    c.ServerConn.Close()
    
    // Close recording file if open
    c.closeRecordFile()
    
    // Clear connection-specific data
    receive.ClearConnectionMap(c.ID)
}

func (c *Connection) relayClientToServer(ctx context.Context) {
    defer c.wg.Done()
    defer c.cancel() // Cancel all goroutines when this relay exits

    buf := make([]byte, 4096)
    for {
        select {
        case <-ctx.Done():
            return
        default:
        }

        c.ClientConn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
        n, err := c.ClientConn.Read(buf)
        if err != nil {
            if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
                continue
            }
            return
        }

        // CRITICAL: Write IMMEDIATELY to maintain packet sequentiality
        _, err = c.ServerConn.Write(buf[:n])
        if err != nil {
            return
        }

        // Now copy for worker (async processing, order doesn't matter here)
        rawData := make([]byte, n)
        copy(rawData, buf[:n])

        chunk := &packets.RawChunk{
            ConnectionID: c.ID,
            Timestamp:    time.Now().Unix(),
            Direction:    common.ClientToServer,
            Data:         rawData,
        }

        select {
        case c.RawChunkBuffer <- chunk:
        default:
            common.Log(common.LogProxy, common.LogError, "CRITICAL: Connection #%d buffer overflow - worker cannot keep up with network traffic (capacity: 100,000)", c.ID)
            panic(fmt.Sprintf("Connection #%d: RawChunkBuffer overflow", c.ID))
        }
    }
}

func (c *Connection) relayServerToClient(ctx context.Context) {
    defer c.wg.Done()
    defer c.cancel() // Cancel all goroutines when this relay exits

    buf := make([]byte, 4096)
    for {
        select {
        case <-ctx.Done():
            return
        default:
        }

        c.ServerConn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
        n, err := c.ServerConn.Read(buf)
        if err != nil {
            if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
                continue
            }
            return
        }

        // CRITICAL: Write IMMEDIATELY to maintain packet sequentiality
        _, err = c.ClientConn.Write(buf[:n])
        if err != nil {
            return
        }

        // Now copy for worker (async processing, order doesn't matter here)
        rawData := make([]byte, n)
        copy(rawData, buf[:n])

        chunk := &packets.RawChunk{
            ConnectionID: c.ID,
            Timestamp:    time.Now().Unix(),
            Direction:    common.ServerToClient,
            Data:         rawData,
        }

        select {
        case c.RawChunkBuffer <- chunk:
        default:
            common.Log(common.LogProxy, common.LogError, "CRITICAL: Connection #%d buffer overflow - worker cannot keep up with network traffic (capacity: 100,000)", c.ID)
            panic(fmt.Sprintf("Connection #%d: RawChunkBuffer overflow", c.ID))
        }
    }
}

func (c *Connection) workerLoop(ctx context.Context) {
    defer c.wg.Done()

    clientBuffer := &bytes.Buffer{}
    serverBuffer := &bytes.Buffer{}
    
    // Flush recorder every 1 second
    flushTicker := time.NewTicker(1 * time.Second)
    defer flushTicker.Stop()

    for {
        select {
        case <-ctx.Done():
            // Graceful drain: process remaining packets before exit
            ReportCloseConnection(c)
            c.drainRemainingPackets(clientBuffer, serverBuffer)
            return
            
        case <-flushTicker.C:
            c.flushRecording()
            
        case chunk := <-c.RawChunkBuffer:
            // CRITICAL: Record RAW chunk BEFORE processing (for reverse engineering)
            if IsRecording() {
                c.recordRawChunk(chunk)
            }
            
            // Then process normally
            c.processChunk(chunk, clientBuffer, serverBuffer)
        }
    }
}

func (c *Connection) processChunk(chunk *packets.RawChunk, clientBuffer, serverBuffer *bytes.Buffer) {
    if chunk.Direction == common.ClientToServer {
        clientBuffer.Write(chunk.Data)
        
        // Try to parse complete packets
        parsedPackets := c.tryParsePackets(clientBuffer, chunk.Direction, chunk.Timestamp)
        for _, pkt := range parsedPackets {
            c.spawnDeserializer(pkt)
        }
    } else {
        serverBuffer.Write(chunk.Data)
        
        // Try to parse complete packets
        parsedPackets := c.tryParsePackets(serverBuffer, chunk.Direction, chunk.Timestamp)
        for _, pkt := range parsedPackets {
            c.spawnDeserializer(pkt)
        }
    }
}

func (c *Connection) drainRemainingPackets(clientBuffer, serverBuffer *bytes.Buffer) {
    common.Log(common.LogProxy, common.LogVeryVerbose, "Connection #%d draining remaining packets (buffer size: %d)", c.ID, len(c.RawChunkBuffer))
    
    processed := 0
    timeout := time.After(500 * time.Millisecond)
    
    for {
        select {
        case chunk, ok := <-c.RawChunkBuffer:
            if !ok {
                // Channel closed
                c.waitForDeserializers()
                common.Log(common.LogProxy, common.LogVeryVerbose, "Connection #%d drained %d packets", c.ID, processed)
                return
            }
            
            c.processChunk(chunk, clientBuffer, serverBuffer)
            processed++
            
        case <-timeout:
            // Timeout reached - stop draining
            c.waitForDeserializers()
            common.Log(common.LogProxy, common.LogVeryVerbose, "Connection #%d drain timeout - processed %d packets, %d remaining", 
                c.ID, processed, len(c.RawChunkBuffer))
            return
        }
    }
}

func (c *Connection) waitForDeserializers() {
    // Wait for all deserializers to finish (up to 1 second)
    deadline := time.Now().Add(1 * time.Second)
    
    for {
        // Try to acquire all 100 slots - if successful, no deserializers running
        if c.semaphore.TryAcquire(100) {
            c.semaphore.Release(100)
            common.Log(common.LogProxy, common.LogVeryVerbose, "Connection #%d - all deserializers finished", c.ID)
            return
        }
        
        if time.Now().After(deadline) {
            common.Log(common.LogProxy, common.LogVeryVerbose, "Connection #%d - timeout waiting for deserializers", c.ID)
            return
        }
        
        time.Sleep(50 * time.Millisecond)
    }
}

// recordRawChunk writes raw chunk to recording file (for reverse engineering).
func (c *Connection) recordRawChunk(chunk *packets.RawChunk) {
    r := GetRecording()
    r.recordMutex.Lock()
    defer r.recordMutex.Unlock()
    
    // Lazy init: create file on first chunk if recording is active
    if r.file == nil {
        if err := createRecordFile(r); err != nil {
            common.Log(common.LogRecord, common.LogError, "Connection #%d failed to create recording file: %v", c.ID, err)
            return
        }
    }
    
    dirStr := common.FormatDirection(chunk.Direction)
    hexData := common.FormatPayload(chunk.Data, false)
    line := fmt.Sprintf("%d;%d;%s;%d;%s\n", chunk.Timestamp, c.ID, dirStr, len(chunk.Data), hexData)
    
    r.writer.WriteString(line)
}

func createRecordFile(r *Recording) error {
    // Ensure recordings directory exists
    if err := os.MkdirAll("recordings", 0755); err != nil {
        return fmt.Errorf("failed to create recordings directory: %w", err)
    }
    
    timestamp := time.Now().Format("20060102_150405")
    filename := fmt.Sprintf("recordings/%s.txt", timestamp)
    
    file, err := os.Create(filename)
    if err != nil {
        return fmt.Errorf("failed to create file: %w", err)
    }
    
    r.file = file
    r.writer = bufio.NewWriter(file)
    
    common.Log(common.LogRecord, common.LogInfo, "Started recording: %s", filename)
    return nil
}

func (c *Connection) flushRecording() {
    r := GetRecording()
    r.recordMutex.Lock()
    defer r.recordMutex.Unlock()
    
    if r.writer != nil {
        r.writer.Flush()
    }
}

func (c *Connection) closeRecordFile() {
    r := GetRecording()
    r.recordMutex.Lock()
    defer r.recordMutex.Unlock()
    
    if r.writer != nil {
        r.writer.Flush()
    }
}

func (c *Connection) spawnDeserializer(pkt *packets.ParsedPacket) {
    // Spawn goroutine immediately (non-blocking)
    // Semaphore acquired inside to prevent blocking the worker
    go func() {
        // Acquire semaphore (blocks if 100 deserializers already running)
        c.semaphore.Acquire(context.Background(), 1)
        defer c.semaphore.Release(1)
        
        // Look up packet spec
        var spec *common.PacketSpec
        if pkt.Direction == common.ServerToClient {
            spec = receive.PacketDatabase[pkt.Opcode]
        } else {
            spec = send.PacketDatabase[pkt.Opcode]
        }
        
        // Format packet info for logging
        dirSymbol := common.FormatDirection(pkt.Direction)
        checksumStr := common.FormatChecksum(pkt.Checksum)
        
        desc := ""
        if spec != nil && spec.Desc != "" {
            desc = spec.Desc
        }
        descDisplay := common.FormatDesc(desc)
        payloadHex := common.FormatPayload(pkt.Payload, true)
        
        // Log packet
        common.Log(common.LogPacket, common.LogVerbose, "[yellow][#%d][-]%s[yellow][0x%04X][-]%s [white]size=%d%s payload=%s[-]", pkt.ConnectionID, dirSymbol, pkt.Opcode, descDisplay, len(pkt.Payload), checksumStr, payloadHex)
        
        // Call deserializer handler if exists
        unknownPkt := true
        if spec != nil {
            unknownPkt = false // can or can't have handler but is know packet
        }
        if spec != nil && spec.Handler != nil {
            handlerValue := reflect.ValueOf(spec.Handler)
            if handlerValue.Kind() == reflect.Ptr {
                handlerValue = handlerValue.Elem()
            }

            packetField := handlerValue.FieldByName("ParsedPacket")
            if packetField.IsValid() && packetField.CanSet() {
                pktValue := reflect.ValueOf(pkt)
                if pktValue.Kind() == reflect.Ptr {
                    pktValue = pktValue.Elem()
                }
                
                packetField.Set(pktValue)
            }

            spec.Handler.Deserialize()
        }
        AddPacket(pkt.Direction, len(pkt.Payload), unknownPkt)
    }()
}

func (c *Connection) tryParsePackets(buffer *bytes.Buffer, direction common.PacketDirection, timestamp int64) []*packets.ParsedPacket {
    var result []*packets.ParsedPacket

    for {
        if buffer.Len() < 2 {
            return result
        }

        bufData := buffer.Bytes()
        opcode := binary.LittleEndian.Uint16(bufData[0:2])

        var spec *common.PacketSpec
        if direction == common.ServerToClient {
            spec = receive.PacketDatabase[opcode]
        } else {
            spec = send.PacketDatabase[opcode]
        }

        if spec == nil {
            buffer.Next(1)
            continue
        }

        var packetSize int
        valid := false

        switch spec.Type {
        case common.FIXED, common.FIXED_MIN:
            packetSize = int(spec.Size)
            valid = buffer.Len() >= packetSize

        case common.INDICATED_IN_PACKET:
            if buffer.Len() >= 4 {
                packetSize = int(binary.LittleEndian.Uint16(bufData[2:4]))
                if packetSize < 2 || packetSize > 10*1024*1024 {
                    payload := common.FormatPayload(bufData, false)
                    ptDir := common.FormatDirection(direction)
                    common.Log(common.LogProxy, common.LogError, "CRITICAL: Invalid packet size %d (dir=%s, opcode=0x%04X, conn=%d) payload %s", packetSize, ptDir, opcode, c.ID, payload)
                    packetSize = 2 // remove head and continue parsing next packages
                }
                valid = buffer.Len() >= packetSize
            }

        case common.HTTP:
            packetSize, valid = c.parseHTTPPacket(buffer)

        case common.UNKNOWN:
            buffer.Next(1)
            continue
        }

        if !valid {
            return result
        }

        packetData := make([]byte, packetSize)
        buffer.Read(packetData)

        var checksum *uint8
        if direction == common.ClientToServer && buffer.Len() > 0 {
            remainingBytes := buffer.Bytes()

            if len(remainingBytes) == 1 {
                extraByte := make([]byte, 1)
                buffer.Read(extraByte)
                checksum = &extraByte[0]
            } else if len(remainingBytes) >= 2 {
                nextOpcode := binary.LittleEndian.Uint16(remainingBytes[0:2])
                if send.PacketDatabase[nextOpcode] != nil {
                    checksum = nil
                } else if len(remainingBytes) >= 3 {
                    nextOpcodeAfterByte := binary.LittleEndian.Uint16(remainingBytes[1:3])
                    if send.PacketDatabase[nextOpcodeAfterByte] != nil {
                        extraByte := make([]byte, 1)
                        buffer.Read(extraByte)
                        checksum = &extraByte[0]
                    }
                }
            }
        }

        // Parse network addresses for MapLocation system
        sourceIP, sourcePort, destIP, destPort := c.parseNetworkAddresses(direction)

        var startData = 2
        if spec.Size == -1 {
            startData = 4
        }
        if startData > len(packetData) {
            common.Log(common.LogProxy, common.LogError, "Strange package... len is undeterminated len: %d - packet %s", startData, common.FormatPayload(packetData, false))
            startData = 0
        }

        result = append(result, &packets.ParsedPacket{
            ConnectionID: c.ID,
            Timestamp:    timestamp,
            Direction:    direction,
            Opcode:       opcode,
            Payload:      packetData[startData:],
            Checksum:     checksum,
            SourceIP:     sourceIP,
            SourcePort:   sourcePort,
            DestIP:       destIP,
            DestPort:     destPort,
        })
    }
}

// parseNetworkAddresses extracts IP addresses and port from connection.
// Returns: sourceIP, sourcePort, destIP, destPort based on packet direction.
func (c *Connection) parseNetworkAddresses(direction common.PacketDirection) (string, int, string, int) {
    var sourceIP, destIP string
    var sourcePort, destPort int
    
    if direction == common.ClientToServer {
        // Client → Server: source=client, dest=server
        clientParts := strings.Split(c.ClientAddr, ":")
        sourceIP = clientParts[0]
        if len(clientParts) > 1 {
            sourcePort, _ = strconv.Atoi(clientParts[1])
        }
        
        serverParts := strings.Split(c.ServerAddr, ":")
        destIP = serverParts[0]
        if len(serverParts) > 1 {
            destPort, _ = strconv.Atoi(serverParts[1])
        }
    } else {
        // Server → Client: source=server, dest=client
        serverParts := strings.Split(c.ServerAddr, ":")
        sourceIP = serverParts[0]
        if len(serverParts) > 1 {
            sourcePort, _ = strconv.Atoi(serverParts[1])
        }
        
        clientParts := strings.Split(c.ClientAddr, ":")
        destIP = clientParts[0]
        if len(clientParts) > 1 {
            destPort, _ = strconv.Atoi(clientParts[1])
        }
    }
    
    return sourceIP, sourcePort, destIP, destPort
}

func (c *Connection) parseHTTPPacket(buffer *bytes.Buffer) (int, bool) {
    bufData := buffer.Bytes()
    delimiter := []byte{0x0D, 0x0A, 0x0D, 0x0A}

    headerEnd := bytes.Index(bufData, delimiter)
    if headerEnd == -1 {
        return 0, false
    }

    headerEnd += 4
    return headerEnd, true
}
