package proxy

import (
    "context"
    "fmt"
    "net"
    "sync"
    "sync/atomic"
    "syscall"
    "time"
    "unsafe"

    "roproxy/internal/common"
    "roproxy/internal/config"
)

const SO_ORIGINAL_DST = 80

type Proxy struct {
    cfg           *config.Config
    listener      net.Listener
    connections   map[uint64]*Connection
    connMutex     sync.RWMutex
    nextConnID    atomic.Uint64
}

func New(cfg *config.Config) *Proxy {
    return &Proxy{
        cfg:           cfg,
        connections:   make(map[uint64]*Connection),
    }
}

func Start(p *Proxy, ctx context.Context) {
    SetProxy(p)
    SetRecording(false)
    go func ()  {
        p.startListen(ctx)
    }()
}

func (p *Proxy) startListen(ctx context.Context) error {
    addr := fmt.Sprintf(":%d", p.cfg.ListenPort)
    listener, err := net.Listen("tcp", addr)
    if err != nil {
        common.Log(common.LogProxy, common.LogError, "failed to listen on %s: %w", addr, err)
        return fmt.Errorf("failed to listen on %s: %w", addr, err)
    }
    p.listener = listener
    defer listener.Close()

    go func() {
        <-ctx.Done()
        listener.Close()
    }()

    for {
        clientConn, err := listener.Accept()
        if err != nil {
            select {
            case <-ctx.Done():
                return nil
            default:
                continue
            }
        }

        go p.handleConnection(ctx, clientConn)
    }
}

func (p *Proxy) handleConnection(ctx context.Context, clientConn net.Conn) {
    startTime := time.Now()
    connID := p.nextConnID.Add(1)
    
    common.Log(common.LogProxy, common.LogVeryVerbose, "Connection #%d: Client connected from %s (handleConnection started)", connID, clientConn.RemoteAddr().String())

    originalDest, err := getOriginalDest(clientConn)
    if err != nil {
        common.Log(common.LogProxy, common.LogError, "Connection #%d: getOriginalDest FAILED: %v", connID, err)
        clientConn.Close()
        return
    }
    afterGetDest := time.Now()
    common.Log(common.LogProxy, common.LogVeryVerbose, "Connection #%d: getOriginalDest=%dms, dest=%s", connID, afterGetDest.Sub(startTime).Milliseconds(), originalDest)

    conn, err := NewConnection(connID, clientConn, originalDest)
    if err != nil {
        common.Log(common.LogProxy, common.LogError, "Connection #%d: NewConnection FAILED: %v", connID, err)
        clientConn.Close()
        return
    }
    afterNewConn := time.Now()
    common.Log(common.LogProxy, common.LogVeryVerbose, "Connection #%d: NewConnection=%dms (dial to %s)", connID, afterNewConn.Sub(afterGetDest).Milliseconds(), originalDest)

    p.connMutex.Lock()
    p.connections[connID] = conn
    p.connMutex.Unlock()

    defer func() {
        p.connMutex.Lock()
        delete(p.connections, connID)
        p.connMutex.Unlock()

        conn.Close()
        
        // Close channel AFTER Close() and Wait() complete
        // This ensures worker has finished draining
        close(conn.RawChunkBuffer)
    }()

    conn.Start(ctx)
    conn.Wait()
}

func getOriginalDest(conn net.Conn) (string, error) {
    tcpConn, ok := conn.(*net.TCPConn)
    if !ok {
        return "", fmt.Errorf("not a TCP connection")
    }

    file, err := tcpConn.File()
    if err != nil {
        return "", fmt.Errorf("failed to get connection file: %w", err)
    }
    defer file.Close()

    fd := int(file.Fd())

    var addr syscall.RawSockaddrInet4
    size := uint32(unsafe.Sizeof(addr))

    if err := getsockopt(fd, syscall.IPPROTO_IP, SO_ORIGINAL_DST, unsafe.Pointer(&addr), &size); err != nil {
        return "", fmt.Errorf("getsockopt SO_ORIGINAL_DST failed: %w", err)
    }

    ip := net.IPv4(addr.Addr[0], addr.Addr[1], addr.Addr[2], addr.Addr[3])
    port := int(addr.Port>>8) | int(addr.Port&0xff)<<8

    return fmt.Sprintf("%s:%d", ip.String(), port), nil
}

func (p *Proxy) GetActiveConnections() []*Connection {
    p.connMutex.RLock()
    defer p.connMutex.RUnlock()

    conns := make([]*Connection, 0, len(p.connections))
    for _, conn := range p.connections {
        conns = append(conns, conn)
    }
    return conns
}
