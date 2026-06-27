package proxy

import (
    "context"
    "net"
    "sync"
    "time"

    "roproxy/internal/common"
    "roproxy/internal/packets"
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
    worker         *Worker
    cancel         context.CancelFunc
    wg             sync.WaitGroup
}

func NewConnection(id uint64, clientConn net.Conn, serverAddr string) (*Connection, error) {
    serverConn, err := net.DialTimeout("tcp", serverAddr, 10*time.Second)
    if err != nil {
        return nil, err
    }

    clientAddr := clientConn.RemoteAddr().String()
    buffer := make(chan *packets.RawChunk, 100000)

    conn := &Connection{
        ID:         id,
        ClientConn: clientConn,
        ServerConn: serverConn,
        ClientAddr: clientAddr,
        ServerAddr: serverAddr,
        StartTime:  time.Now(),
        RawChunkBuffer: buffer,
        worker:     NewWorker(id, clientAddr, serverAddr, buffer),
    }

    return conn, nil
}

func (c *Connection) Start(ctx context.Context) {
    connCtx, cancel := context.WithCancel(ctx)
    c.cancel = cancel

    // Debug log connection addresses
    common.Log(common.LogProxy, common.LogVeryVerbose, "Connection #%d started: ClientAddr='%s', ServerAddr='%s'", c.ID, c.ClientAddr, c.ServerAddr)

    // Start worker
    c.worker.Start(connCtx)

    // Start relays
    c.wg.Add(2)
    go c.relayClientToServer(connCtx)
    go c.relayServerToClient(connCtx)
}

func (c *Connection) Wait() {
    c.wg.Wait()
    c.worker.Wait()
}

func (c *Connection) Close() {
    // Report close connection
    ReportCloseConnection(c)

    // Cancel all goroutines (relays + worker will stop)
    if c.cancel != nil {
        c.cancel()
    }

    // Close worker
    c.worker.Close()

    // Close network connections (forces relays to exit if still blocked on Read)
    c.ClientConn.Close()
    c.ServerConn.Close()

    // Close recording file if open
    c.closeRecordFile()

    // Clear connection-specific data
    packets.ClearConnectionMap(c.ID)
}

func (c *Connection) relayClientToServer(ctx context.Context) {
    c.relay(ctx, c.ClientConn, c.ServerConn, common.ClientToServer)
}

func (c *Connection) relayServerToClient(ctx context.Context) {
    c.relay(ctx, c.ServerConn, c.ClientConn, common.ServerToClient)
}

func (c *Connection) relay(ctx context.Context, source, dest net.Conn, direction common.PacketDirection) {
    defer c.wg.Done()
    defer c.cancel() // Cancel all goroutines when this relay exits

    buf := make([]byte, 4096)
    for {
        select {
        case <-ctx.Done():
            return
        default:
        }

        source.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
        n, err := source.Read(buf)
        if err != nil {
            if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
                continue
            }
            return
        }

        // CRITICAL: Write IMMEDIATELY to maintain packet sequentiality
        _, err = dest.Write(buf[:n])
        if err != nil {
            return
        }

        // Now copy for worker (async processing, order doesn't matter here)
        rawData := make([]byte, n)
        copy(rawData, buf[:n])

        chunk := &packets.RawChunk{
            ConnectionID: c.ID,
            Timestamp:    time.Now().Unix(),
            Direction:    direction,
            Data:         rawData,
        }

        select {
        case c.RawChunkBuffer <- chunk:
        default:
            common.Log(common.LogProxy, common.LogError, "CRITICAL: Connection #%d buffer overflow - worker cannot keep up with network traffic (capacity: 100,000) - Dropping chunk", c.ID)
        }
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
