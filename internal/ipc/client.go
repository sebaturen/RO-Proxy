package ipc

import (
	"fmt"
	"net"
	"sync"
	"time"

	"roproxy/internal/common"
)

const (
	BufferSize = 10000
	ReconnectDelay = 500 * time.Millisecond
	WriteTimeout = 100 * time.Millisecond
)

// Client handles IPC communication from Proxy to Analyzer.
// It maintains a ring buffer to avoid blocking when Analyzer is down.
type Client struct {
	socketPath string
	conn       net.Conn
	connMutex  sync.RWMutex

	// Ring buffer for pending frames
	buffer   [BufferSize]*Frame
	head     int
	tail     int
	count    int
	bufMutex sync.Mutex

	sendChan  chan struct{} // Signal to process buffer
	closeChan chan struct{}
	wg        sync.WaitGroup
}

// NewClient creates a new IPC client that connects to the given Unix socket path.
// The client starts a background goroutine for sending frames.
func NewClient(socketPath string) *Client {
	c := &Client{
		socketPath: socketPath,
		sendChan:   make(chan struct{}, 1),
		closeChan:  make(chan struct{}),
	}

	c.wg.Add(1)
	go c.senderLoop()

	return c
}

// Send enqueues a frame for sending to the Analyzer (non-blocking).
// If the buffer is full, the oldest frame is dropped.
func (c *Client) Send(frame *Frame) {
	c.bufMutex.Lock()

	if c.count >= BufferSize {
		// Buffer full: drop oldest frame
		c.head = (c.head + 1) % BufferSize
		c.count--
		common.Log(common.LogProxy, common.LogWarning, "IPC buffer full, dropping oldest frame")
	}

	c.buffer[c.tail] = frame
	c.tail = (c.tail + 1) % BufferSize
	c.count++

	c.bufMutex.Unlock()

	// Signal sender (non-blocking)
	select {
	case c.sendChan <- struct{}{}:
	default:
	}
}

// senderLoop processes buffered frames in the background
func (c *Client) senderLoop() {
	defer c.wg.Done()

	for {
		select {
		case <-c.closeChan:
			return
		case <-c.sendChan:
			c.processBuffer()
		}
	}
}

// processBuffer sends all buffered frames
func (c *Client) processBuffer() {
	for {
		frame := c.popFrame()
		if frame == nil {
			return
		}

		if err := c.sendFrame(frame); err != nil {
			// Re-enqueue the frame and wait for reconnection
			c.pushFront(frame)
			c.reconnect()
			return
		}
	}
}

// popFrame removes and returns the oldest frame from the buffer
func (c *Client) popFrame() *Frame {
	c.bufMutex.Lock()
	defer c.bufMutex.Unlock()

	if c.count == 0 {
		return nil
	}

	frame := c.buffer[c.head]
	c.buffer[c.head] = nil
	c.head = (c.head + 1) % BufferSize
	c.count--

	return frame
}

// pushFront puts a frame back at the front of the buffer
func (c *Client) pushFront(frame *Frame) {
	c.bufMutex.Lock()
	defer c.bufMutex.Unlock()

	// Move head back one position
	c.head = (c.head - 1 + BufferSize) % BufferSize
	c.buffer[c.head] = frame
	c.count++
}

// sendFrame writes a frame to the socket
func (c *Client) sendFrame(frame *Frame) error {
	c.connMutex.RLock()
	conn := c.conn
	c.connMutex.RUnlock()

	if conn == nil {
		return net.ErrClosed
	}

	conn.SetWriteDeadline(time.Now().Add(WriteTimeout))
	_, err := conn.Write(frame.Encode())
	return err
}

// reconnect attempts to reconnect to the Analyzer
func (c *Client) reconnect() {
	c.connMutex.Lock()
	defer c.connMutex.Unlock()

	// Close existing connection if any
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
		fmt.Printf("[Proxy] IPC disconnected - Analyzer is down, buffering frames...\n")
	}

	for {
		select {
		case <-c.closeChan:
			return
		default:
		}

		conn, err := net.DialTimeout("unix", c.socketPath, ReconnectDelay)
		if err != nil {
			time.Sleep(ReconnectDelay)
			continue
		}

		c.conn = conn
		fmt.Printf("[Proxy] IPC connected to Analyzer (%s)\n", c.socketPath)
		common.Log(common.LogProxy, common.LogInfo, "IPC connected to %s", c.socketPath)

		// Trigger buffer processing after reconnect
		select {
		case c.sendChan <- struct{}{}:
		default:
		}

		return
	}
}

// Close shuts down the client and closes the connection
func (c *Client) Close() {
	close(c.closeChan)

	c.connMutex.Lock()
	if c.conn != nil {
		c.conn.Close()
	}
	c.connMutex.Unlock()

	c.wg.Wait()
}
