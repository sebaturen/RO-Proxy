package proxy

import (
    "context"
    "net"
    "strconv"
    "sync"
    "time"

    "roproxy/internal/common"
    "roproxy/internal/packets"
    "roproxy/internal/packets/receive"
)

type Connection struct {
    ID           uint64
    ClientConn   net.Conn
    ServerConn   net.Conn
    ClientAddr   string
    ServerAddr   string
    StartTime    time.Time

    parser       *packets.StreamParser
    processor    *packets.PacketProcessor
    packetChan   chan *packets.CapturedPacket
    wg           sync.WaitGroup
    proxy        packets.CaptureSettings
}

func NewConnection(id uint64, clientConn net.Conn, serverAddr string, verbose bool, proxy packets.CaptureSettings) (*Connection, error) {
    serverConn, err := net.DialTimeout("tcp", serverAddr, 10*time.Second)
    if err != nil {
        return nil, err
    }

    clientAddr := clientConn.RemoteAddr().String()
    clientIP, _, _ := net.SplitHostPort(clientAddr)
    
    serverIP, serverPortStr, _ := net.SplitHostPort(serverAddr)
    serverPort, _ := strconv.Atoi(serverPortStr)
    
    packetChan := make(chan *packets.CapturedPacket, 1000)
    parser := packets.NewStreamParser(id, clientIP, serverIP, serverPort, verbose)
    processor := packets.NewPacketProcessor(id, packetChan, verbose, proxy)

    conn := &Connection{
        ID:         id,
        ClientConn: clientConn,
        ServerConn: serverConn,
        ClientAddr: clientAddr,
        ServerAddr: serverAddr,
        StartTime:  time.Now(),
        parser:     parser,
        processor:  processor,
        packetChan: packetChan,
        proxy:      proxy,
    }

    processor.Start()

    return conn, nil
}

func (c *Connection) SetLogger(logger packets.PacketLogger) {
    c.processor.SetLogger(logger)
}

func (c *Connection) Start(ctx context.Context, verbose bool) {
    c.wg.Add(2)
    go c.relayClientToServer(ctx, verbose)
    go c.relayServerToClient(ctx, verbose)
}

func (c *Connection) Wait() {
    c.wg.Wait()
}

func (c *Connection) Close() {
    c.ClientConn.Close()
    c.ServerConn.Close()
    close(c.packetChan)
    c.processor.Stop()
    receive.ClearConnectionMap(c.ID)
}

func (c *Connection) relayClientToServer(ctx context.Context, verbose bool) {
    defer c.wg.Done()

    buf := make([]byte, 32*1024)
    for {
        select {
        case <-ctx.Done():
            return
        default:
            c.ClientConn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
            n, err := c.ClientConn.Read(buf)
            
            if n > 0 {
                c.parser.AppendData(buf[:n], common.ClientToServer)
                
                timestamp := time.Now().Unix()
                c.parser.TryParsePackets(c.packetChan, timestamp, common.ClientToServer)
                
                _, writeErr := c.ServerConn.Write(buf[:n])
                if writeErr != nil {
                    return
                }
            }

            if err != nil {
                if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
                    continue
                }
                return
            }
        }
    }
}

func (c *Connection) relayServerToClient(ctx context.Context, verbose bool) {
    defer c.wg.Done()

    buf := make([]byte, 32*1024)
    for {
        select {
        case <-ctx.Done():
            return
        default:
            c.ServerConn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
            n, err := c.ServerConn.Read(buf)
            
            if n > 0 {
                c.parser.AppendData(buf[:n], common.ServerToClient)
                
                timestamp := time.Now().Unix()
                c.parser.TryParsePackets(c.packetChan, timestamp, common.ServerToClient)
                
                _, writeErr := c.ClientConn.Write(buf[:n])
                if writeErr != nil {
                    return
                }
            }

            if err != nil {
                if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
                    continue
                }
                return
            }
        }
    }
}
