package proxy

import (
    "context"
    "fmt"
    "net"
    "sync"
    "time"

    "roproxy/internal/common"
    "roproxy/internal/ipc"
    "roproxy/internal/packets"
)

type Connection struct {
    ID           uint64
    ClientConn   net.Conn
    ServerConn   net.Conn
    ClientAddr   string
    ServerAddr   string
    StartTime    time.Time

    cancel         context.CancelFunc
    wg             sync.WaitGroup
}

func NewConnection(id uint64, clientConn net.Conn, serverAddr string) (*Connection, error) {
    serverConn, err := net.DialTimeout("tcp", serverAddr, 10*time.Second)
    if err != nil {
        return nil, err
    }

    clientAddr := clientConn.RemoteAddr().String()

    conn := &Connection{
        ID:         id,
        ClientConn: clientConn,
        ServerConn: serverConn,
        ClientAddr: clientAddr,
        ServerAddr: serverAddr,
        StartTime:  time.Now(),
    }

    return conn, nil
}

func (c *Connection) Start(ctx context.Context) {
    connCtx, cancel := context.WithCancel(ctx)
    c.cancel = cancel

    fmt.Printf("[Proxy] Connection #%d opened: %s → %s\n", c.ID, c.ClientAddr, c.ServerAddr)
    common.Log(common.LogProxy, common.LogVeryVerbose, "Connection #%d started: ClientAddr='%s', ServerAddr='%s'", c.ID, c.ClientAddr, c.ServerAddr)

    // Notify Analyzer of new connection
    ipcClient := GetIPCClient()
    if ipcClient != nil {
        ipcClient.Send(ipc.NewConnOpenFrame(c.ID, time.Now().Unix(), c.ClientAddr, c.ServerAddr))
    }

    // Start relays (no local worker - data goes to IPC)
    c.wg.Add(2)
    go c.relayClientToServer(connCtx)
    go c.relayServerToClient(connCtx)
}

func (c *Connection) Wait() {
    c.wg.Wait()
}

func (c *Connection) Close() {
    duration := time.Since(c.StartTime).Round(time.Second)
    fmt.Printf("[Proxy] Connection #%d closed: %s → %s (duration: %s)\n", c.ID, c.ClientAddr, c.ServerAddr, duration)

    // Notify Analyzer of connection close
    ipcClient := GetIPCClient()
    if ipcClient != nil {
        ipcClient.Send(ipc.NewConnCloseFrame(c.ID, time.Now().Unix()))
    }

    if c.cancel != nil {
        c.cancel()
    }

    c.ClientConn.Close()
    c.ServerConn.Close()

    c.closeRecordFile()
}

func (c *Connection) relayClientToServer(ctx context.Context) {
    c.relay(ctx, c.ClientConn, c.ServerConn, common.ClientToServer)
}

func (c *Connection) relayServerToClient(ctx context.Context) {
    c.relay(ctx, c.ServerConn, c.ClientConn, common.ServerToClient)
}

func (c *Connection) relay(ctx context.Context, source, dest net.Conn, direction common.PacketDirection) {
    defer c.wg.Done()
    defer c.cancel()

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

        // Copy data for async processing
        rawData := make([]byte, n)
        copy(rawData, buf[:n])
        timestamp := time.Now().Unix()

        // Record raw chunk if recording is enabled (before IPC)
        if IsRecording() {
            c.recordRawChunk(&packets.RawChunk{
                ConnectionID: c.ID,
                Timestamp:    timestamp,
                Direction:    direction,
                Data:         rawData,
            })
        }

        // Send to IPC for Analyzer processing (non-blocking)
        ipcClient := GetIPCClient()
        if ipcClient != nil {
            ipcClient.Send(ipc.NewDataFrame(c.ID, timestamp, direction, c.ClientAddr, c.ServerAddr, rawData))
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

// recordRawChunk writes raw chunk to recording file
func (c *Connection) recordRawChunk(chunk *packets.RawChunk) {
    recordRawChunkToFile(c.ID, chunk)
}
