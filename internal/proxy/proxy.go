package proxy

import (
    "context"
    "fmt"
    "net"
    "sync"
    "sync/atomic"
    "syscall"
    "unsafe"

    "roproxy/internal/config"
    "roproxy/internal/packets"
)

const SO_ORIGINAL_DST = 80

type Proxy struct {
    cfg           *config.Config
    listener      net.Listener
    connections   map[uint64]*Connection
    connMutex     sync.RWMutex
    nextConnID    atomic.Uint64
    allowedIPsMap map[string]bool
    verbose       bool
    captureServer bool
    captureClient bool
    packetLogger  packets.PacketLogger
}

func New(cfg *config.Config, verbose, captureServer, captureClient bool) *Proxy {
    allowedIPs := make(map[string]bool)
    for _, ip := range cfg.TargetIPs {
        allowedIPs[ip] = true
    }

    return &Proxy{
        cfg:           cfg,
        connections:   make(map[uint64]*Connection),
        allowedIPsMap: allowedIPs,
        verbose:       verbose,
        captureServer: captureServer,
        captureClient: captureClient,
    }
}

func (p *Proxy) SetPacketLogger(logger packets.PacketLogger) {
	p.packetLogger = logger
}

func (p *Proxy) Start(ctx context.Context) error {
    addr := fmt.Sprintf(":%d", p.cfg.ListenPort)
    listener, err := net.Listen("tcp", addr)
    if err != nil {
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
    connID := p.nextConnID.Add(1)

    originalDest, err := getOriginalDest(clientConn)
    if err != nil {
        clientConn.Close()
        return
    }

    destIP, _, err := net.SplitHostPort(originalDest)
    if err != nil {
        clientConn.Close()
        return
    }

    if !p.allowedIPsMap[destIP] {
        clientConn.Close()
        return
    }

    conn, err := NewConnection(connID, clientConn, originalDest, p.verbose, p)
    if err != nil {
        clientConn.Close()
        return
    }

    if p.packetLogger != nil {
        conn.SetLogger(p.packetLogger)
    }

    p.connMutex.Lock()
    p.connections[connID] = conn
    p.connMutex.Unlock()

    defer func() {
        p.connMutex.Lock()
        delete(p.connections, connID)
        p.connMutex.Unlock()

        conn.Close()
    }()

    conn.Start(ctx, p.verbose)
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

func (p *Proxy) GetCaptureServer() bool {
	return p.captureServer
}

func (p *Proxy) GetCaptureClient() bool {
	return p.captureClient
}

func (p *Proxy) SetCaptureServer(enabled bool) {
    p.captureServer = enabled
}

func (p *Proxy) SetCaptureClient(enabled bool) {
    p.captureClient = enabled
}
