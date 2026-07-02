package ipc

import (
	"context"
	"net"
	"os"
	"sync"

	"roproxy/internal/common"
)

// Server handles IPC communication from Proxy to Analyzer.
// It listens on a Unix domain socket and forwards frames to a channel.
type Server struct {
	socketPath string
	listener   net.Listener

	frameChan chan *Frame
	closeChan chan struct{}
	wg        sync.WaitGroup
}

// NewServer creates a new IPC server that listens on the given Unix socket path.
func NewServer(socketPath string) (*Server, error) {
	// Remove existing socket file if it exists
	os.Remove(socketPath)

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, err
	}

	return &Server{
		socketPath: socketPath,
		listener:   listener,
		frameChan:  make(chan *Frame, 10000),
		closeChan:  make(chan struct{}),
	}, nil
}

// Frames returns the channel where received frames are sent
func (s *Server) Frames() <-chan *Frame {
	return s.frameChan
}

// Start begins accepting connections in the background
func (s *Server) Start(ctx context.Context) {
	s.wg.Add(1)
	go s.acceptLoop(ctx)
}

// acceptLoop accepts incoming connections
func (s *Server) acceptLoop(ctx context.Context) {
	defer s.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.closeChan:
			return
		default:
		}

		conn, err := s.listener.Accept()
		if err != nil {
			// Check if server is closing
			select {
			case <-s.closeChan:
				return
			default:
				continue
			}
		}

		common.Log(common.LogProxy, common.LogInfo, "IPC client connected from Proxy")
		s.wg.Add(1)
		go s.handleConnection(ctx, conn)
	}
}

// handleConnection reads frames from a single connection
func (s *Server) handleConnection(ctx context.Context, conn net.Conn) {
	defer s.wg.Done()
	defer conn.Close()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.closeChan:
			return
		default:
		}

		frame, err := DecodeFrame(conn)
		if err != nil {
			// Connection closed or error - notify Processor
			common.Log(common.LogProxy, common.LogWarning, "IPC connection closed: %v", err)
			select {
			case s.frameChan <- &Frame{Type: FrameTypeIPCDisconnect}:
			default:
			}
			return
		}

		// Send frame to channel (non-blocking)
		select {
		case s.frameChan <- frame:
		default:
			common.Log(common.LogProxy, common.LogWarning, "IPC frame channel full, dropping frame")
		}
	}
}

// Close shuts down the server and removes the socket file
func (s *Server) Close() error {
	close(s.closeChan)
	s.listener.Close()
	os.Remove(s.socketPath)
	s.wg.Wait()
	close(s.frameChan)
	return nil
}
