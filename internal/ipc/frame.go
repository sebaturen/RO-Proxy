package ipc

import (
	"encoding/binary"
	"errors"
	"io"

	"roproxy/internal/common"
	"roproxy/internal/packets"
)

const (
	MagicNumber uint16 = 0x524F
	Version     byte   = 0x01
)

// Frame types for IPC communication
type FrameType byte

const (
	// Proxy → Analyzer frames
	FrameTypeData      FrameType = 0x00 // Regular TCP data
	FrameTypeConnOpen  FrameType = 0x01 // New connection established
	FrameTypeConnClose FrameType = 0x02 // Connection closed

	// Internal frames (Server → Processor)
	FrameTypeIPCDisconnect FrameType = 0x03 // Proxy disconnected from IPC
)

var (
	ErrInvalidMagic   = errors.New("invalid magic number")
	ErrInvalidVersion = errors.New("unsupported protocol version")
)

// Frame represents an IPC frame that encapsulates a RawChunk for transport
// between the Proxy and Analyzer processes.
type Frame struct {
	Type         FrameType
	ConnectionID uint64
	Timestamp    int64
	Direction    common.PacketDirection
	ClientAddr   string
	ServerAddr   string
	Data         []byte
}

// NewDataFrame creates a frame for regular TCP data
func NewDataFrame(connID uint64, timestamp int64, direction common.PacketDirection, clientAddr, serverAddr string, data []byte) *Frame {
	return &Frame{
		Type:         FrameTypeData,
		ConnectionID: connID,
		Timestamp:    timestamp,
		Direction:    direction,
		ClientAddr:   clientAddr,
		ServerAddr:   serverAddr,
		Data:         data,
	}
}

// NewConnOpenFrame creates a frame to notify a new connection
func NewConnOpenFrame(connID uint64, timestamp int64, clientAddr, serverAddr string) *Frame {
	return &Frame{
		Type:         FrameTypeConnOpen,
		ConnectionID: connID,
		Timestamp:    timestamp,
		ClientAddr:   clientAddr,
		ServerAddr:   serverAddr,
	}
}

// NewConnCloseFrame creates a frame to notify a connection closed
func NewConnCloseFrame(connID uint64, timestamp int64) *Frame {
	return &Frame{
		Type:         FrameTypeConnClose,
		ConnectionID: connID,
		Timestamp:    timestamp,
	}
}

// ToRawChunk converts a Frame to a RawChunk for processing in the Analyzer
func (f *Frame) ToRawChunk() *packets.RawChunk {
	return &packets.RawChunk{
		ConnectionID: f.ConnectionID,
		Timestamp:    f.Timestamp,
		Direction:    f.Direction,
		Data:         f.Data,
	}
}

// Encode serializes the frame to bytes for transmission over the socket
func (f *Frame) Encode() []byte {
	clientAddrBytes := []byte(f.ClientAddr)
	serverAddrBytes := []byte(f.ServerAddr)

	// Calculate frame body length (excluding magic + version + frameLen fields)
	frameLen := 1 + 8 + 8 + 1 + // type + connID + timestamp + direction
		1 + len(clientAddrBytes) + // clientAddrLen + clientAddr
		1 + len(serverAddrBytes) + // serverAddrLen + serverAddr
		4 + len(f.Data) // dataLen + data

	totalSize := 2 + 1 + 4 + frameLen // magic + version + frameLen + body
	buf := make([]byte, totalSize)

	offset := 0

	// Magic number (2 bytes)
	binary.LittleEndian.PutUint16(buf[offset:], MagicNumber)
	offset += 2

	// Version (1 byte)
	buf[offset] = Version
	offset += 1

	// Frame length (4 bytes)
	binary.LittleEndian.PutUint32(buf[offset:], uint32(frameLen))
	offset += 4

	// Frame type (1 byte)
	buf[offset] = byte(f.Type)
	offset += 1

	// Connection ID (8 bytes)
	binary.LittleEndian.PutUint64(buf[offset:], f.ConnectionID)
	offset += 8

	// Timestamp (8 bytes)
	binary.LittleEndian.PutUint64(buf[offset:], uint64(f.Timestamp))
	offset += 8

	// Direction (1 byte)
	if f.Direction == common.ServerToClient {
		buf[offset] = 0x01
	} else {
		buf[offset] = 0x00
	}
	offset += 1

	// Client address (1 byte length + N bytes)
	buf[offset] = byte(len(clientAddrBytes))
	offset += 1
	copy(buf[offset:], clientAddrBytes)
	offset += len(clientAddrBytes)

	// Server address (1 byte length + N bytes)
	buf[offset] = byte(len(serverAddrBytes))
	offset += 1
	copy(buf[offset:], serverAddrBytes)
	offset += len(serverAddrBytes)

	// Data length (4 bytes) + Data (N bytes)
	binary.LittleEndian.PutUint32(buf[offset:], uint32(len(f.Data)))
	offset += 4
	copy(buf[offset:], f.Data)

	return buf
}

// DecodeFrame reads and deserializes a frame from an io.Reader
func DecodeFrame(r io.Reader) (*Frame, error) {
	// Read header: magic (2) + version (1) + frameLen (4) = 7 bytes
	header := make([]byte, 7)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, err
	}

	// Validate magic number
	magic := binary.LittleEndian.Uint16(header[0:2])
	if magic != MagicNumber {
		return nil, ErrInvalidMagic
	}

	// Validate version
	if header[2] != Version {
		return nil, ErrInvalidVersion
	}

	// Read frame body
	frameLen := binary.LittleEndian.Uint32(header[3:7])
	body := make([]byte, frameLen)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}

	offset := 0

	// Frame type (1 byte)
	frameType := FrameType(body[offset])
	offset += 1

	// Connection ID (8 bytes)
	connID := binary.LittleEndian.Uint64(body[offset:])
	offset += 8

	// Timestamp (8 bytes)
	timestamp := int64(binary.LittleEndian.Uint64(body[offset:]))
	offset += 8

	// Direction (1 byte)
	var direction common.PacketDirection
	if body[offset] == 0x01 {
		direction = common.ServerToClient
	} else {
		direction = common.ClientToServer
	}
	offset += 1

	// Client address (1 byte length + N bytes)
	clientAddrLen := int(body[offset])
	offset += 1
	clientAddr := string(body[offset : offset+clientAddrLen])
	offset += clientAddrLen

	// Server address (1 byte length + N bytes)
	serverAddrLen := int(body[offset])
	offset += 1
	serverAddr := string(body[offset : offset+serverAddrLen])
	offset += serverAddrLen

	// Data (4 bytes length + N bytes)
	dataLen := binary.LittleEndian.Uint32(body[offset:])
	offset += 4
	data := make([]byte, dataLen)
	copy(data, body[offset:offset+int(dataLen)])

	return &Frame{
		Type:         frameType,
		ConnectionID: connID,
		Timestamp:    timestamp,
		Direction:    direction,
		ClientAddr:   clientAddr,
		ServerAddr:   serverAddr,
		Data:         data,
	}, nil
}
