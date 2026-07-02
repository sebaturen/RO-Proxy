package analyzer

import (
	"context"
	"fmt"
	"sync"
	"time"

	"roproxy/internal/common"
	"roproxy/internal/config"
	"roproxy/internal/ipc"
	"roproxy/internal/packets"
)

// ConnectionInfo tracks metadata about an active connection
type ConnectionInfo struct {
	ID         uint64
	ClientAddr string
	ServerAddr string
	StartTime  time.Time
}

// Processor receives frames from IPC and dispatches them to workers.
// Each unique ConnectionID gets its own worker.
type Processor struct {
	frameChan   <-chan *ipc.Frame
	workers     map[uint64]*workerHandle
	connections map[uint64]*ConnectionInfo
	workersMu   sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	cfg         *config.Config
	startTime   time.Time
	cleanedUp   bool
}

// workerHandle tracks a worker and its input channel
type workerHandle struct {
	worker    *Worker
	chunkChan chan *packets.RawChunk
}

// NewProcessor creates a processor that reads from the given frame channel
func NewProcessor(frameChan <-chan *ipc.Frame, cfg *config.Config) *Processor {
	return &Processor{
		frameChan:   frameChan,
		workers:     make(map[uint64]*workerHandle),
		connections: make(map[uint64]*ConnectionInfo),
		cfg:         cfg,
		startTime:   time.Now(),
	}
}

// Start begins processing frames in the background
func (p *Processor) Start(ctx context.Context) {
	p.ctx, p.cancel = context.WithCancel(ctx)

	p.wg.Add(1)
	go p.processLoop()
}

// Stop shuts down the processor and all workers
func (p *Processor) Stop() {
	if p.cancel != nil {
		p.cancel()
	}

	p.workersMu.Lock()
	for _, handle := range p.workers {
		close(handle.chunkChan)
		handle.worker.Close()
	}
	p.workersMu.Unlock()

	p.wg.Wait()
}

// processLoop reads frames and dispatches them to workers
func (p *Processor) processLoop() {
	defer p.wg.Done()

	for {
		select {
		case <-p.ctx.Done():
			p.cleanupAllWorkers()
			return

		case frame, ok := <-p.frameChan:
			if !ok {
				p.cleanupAllWorkers()
				return
			}

			p.handleFrame(frame)
		}
	}
}

// handleFrame processes a frame based on its type
func (p *Processor) handleFrame(frame *ipc.Frame) {
	switch frame.Type {
	case ipc.FrameTypeConnOpen:
		p.handleConnOpen(frame)
	case ipc.FrameTypeConnClose:
		p.handleConnClose(frame)
	case ipc.FrameTypeData:
		p.handleData(frame)
	case ipc.FrameTypeIPCDisconnect:
		p.handleIPCDisconnect()
	}
}

// handleConnOpen registers a new connection
func (p *Processor) handleConnOpen(frame *ipc.Frame) {
	connID := frame.ConnectionID

	p.workersMu.Lock()
	defer p.workersMu.Unlock()

	// Store connection info
	p.connections[connID] = &ConnectionInfo{
		ID:         connID,
		ClientAddr: frame.ClientAddr,
		ServerAddr: frame.ServerAddr,
		StartTime:  time.Unix(frame.Timestamp, 0),
	}

	// Create worker
	chunkChan := make(chan *packets.RawChunk, 100000)
	worker := NewWorker(connID, frame.ClientAddr, frame.ServerAddr, chunkChan)
	worker.Start(p.ctx)

	p.workers[connID] = &workerHandle{
		worker:    worker,
		chunkChan: chunkChan,
	}

	common.Log(common.LogPacket, common.LogInfo, "Connection #%d opened (%s → %s)", connID, frame.ClientAddr, frame.ServerAddr)
}

// handleConnClose cleans up a closed connection
func (p *Processor) handleConnClose(frame *ipc.Frame) {
	connID := frame.ConnectionID

	p.workersMu.Lock()
	connInfo := p.connections[connID]
	handle := p.workers[connID]

	if handle != nil {
		close(handle.chunkChan)
		handle.worker.Close()
		delete(p.workers, connID)
	}
	delete(p.connections, connID)
	p.workersMu.Unlock()

	// Send Discord notification if configured
	if connInfo != nil {
		duration := time.Since(connInfo.StartTime)
		mapName, _ := packets.GetConnectionMap(connID)
		packets.ClearConnectionMap(connID)

		common.Log(common.LogPacket, common.LogInfo, "Connection #%d closed (duration: %s, map: %s)", connID, duration.Round(time.Second), mapName)

		p.sendDiscordNotification(connInfo, duration, mapName)
	}
}

// handleIPCDisconnect handles proxy disconnection - all game connections are dead
func (p *Processor) handleIPCDisconnect() {
	p.workersMu.Lock()

	// Close all workers and send notifications for each
	for connID, handle := range p.workers {
		connInfo := p.connections[connID]

		close(handle.chunkChan)
		handle.worker.Close()

		if connInfo != nil {
			duration := time.Since(connInfo.StartTime)
			mapName, _ := packets.GetConnectionMap(connID)
			packets.ClearConnectionMap(connID)

			common.Log(common.LogPacket, common.LogWarning, "Connection #%d lost (proxy died, duration: %s, map: %s)", connID, duration.Round(time.Second), mapName)

			p.sendDiscordNotification(connInfo, duration, mapName)
		}

		delete(p.workers, connID)
		delete(p.connections, connID)
	}

	p.workersMu.Unlock()

	go common.SendDiscordNotification("⚠️ IPC: Proxy disconnected - all connections lost")
}

// handleData dispatches data to the appropriate worker
func (p *Processor) handleData(frame *ipc.Frame) {
	connID := frame.ConnectionID

	p.workersMu.RLock()
	handle := p.workers[connID]
	p.workersMu.RUnlock()

	if handle == nil {
		// Worker doesn't exist, create it (in case we missed CONN_OPEN)
		p.workersMu.Lock()
		if handle = p.workers[connID]; handle == nil {
			chunkChan := make(chan *packets.RawChunk, 100000)
			worker := NewWorker(connID, frame.ClientAddr, frame.ServerAddr, chunkChan)
			worker.Start(p.ctx)

			handle = &workerHandle{
				worker:    worker,
				chunkChan: chunkChan,
			}
			p.workers[connID] = handle

			p.connections[connID] = &ConnectionInfo{
				ID:         connID,
				ClientAddr: frame.ClientAddr,
				ServerAddr: frame.ServerAddr,
				StartTime:  time.Now(),
			}
		}
		p.workersMu.Unlock()
	}

	chunk := frame.ToRawChunk()
	select {
	case handle.chunkChan <- chunk:
	default:
		common.Log(common.LogPacket, common.LogWarning, "Worker buffer full for connection #%d, dropping chunk", connID)
	}
}

// sendDiscordNotification sends a Discord webhook notification for connection close
func (p *Processor) sendDiscordNotification(conn *ConnectionInfo, duration time.Duration, mapName string) {
	if duration.Minutes() < 1 {
		return
	}

	h := int(duration.Hours())
	m := int(duration.Minutes()) % 60
	s := int(duration.Seconds()) % 60
	durationStr := fmt.Sprintf("%02d:%02d:%02d", h, m, s)

	content := fmt.Sprintf("Connection #%d closed [Duration: %s]", conn.ID, durationStr)
	if mapName != "" {
		content = fmt.Sprintf("Connection #%d closed [Duration: %s] in map %s", conn.ID, durationStr, mapName)
	}

	go common.SendDiscordNotification(content)
}

// cleanupAllWorkers stops and removes all workers
func (p *Processor) cleanupAllWorkers() {
	p.workersMu.Lock()
	defer p.workersMu.Unlock()

	if p.cleanedUp {
		return
	}
	p.cleanedUp = true

	for connID, handle := range p.workers {
		close(handle.chunkChan)
		handle.worker.Close()
		handle.worker.Wait()
		delete(p.workers, connID)
	}
}

// GetActiveConnections returns a copy of all active connections
func (p *Processor) GetActiveConnections() []*ConnectionInfo {
	p.workersMu.RLock()
	defer p.workersMu.RUnlock()

	conns := make([]*ConnectionInfo, 0, len(p.connections))
	for _, conn := range p.connections {
		conns = append(conns, conn)
	}
	return conns
}

// GetStartTime returns when the processor started
func (p *Processor) GetStartTime() time.Time {
	return p.startTime
}
