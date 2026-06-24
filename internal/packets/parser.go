package packets

import (
	"roproxy/internal/common"
)

// RawChunk represents a raw TCP stream chunk (unparsed).
// Sent from relay goroutines to worker for parsing.
type RawChunk struct {
    ConnectionID uint64
    Timestamp    int64
    Direction    common.PacketDirection
    Data         []byte
}

// ParsedPacket represents a complete parsed packet ready for deserialization.
// Sent from worker to deserializer goroutines.
type ParsedPacket struct {
    ConnectionID uint64
    Timestamp    int64
    Direction    common.PacketDirection
    Opcode       uint16
    Payload      []byte
    Checksum     *uint8
    
    // Network details needed for MapLocation system
    SourceIP   string
    SourcePort int
    DestIP     string
    DestPort   int
}