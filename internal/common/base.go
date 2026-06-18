package common

import (
    "encoding/binary"
    "encoding/hex"
)

type PacketSizeType int

const (
    FIXED PacketSizeType = iota
    INDICATED_IN_PACKET
    UNKNOWN
    FIXED_MIN
    HTTP
)

type PacketSpec struct {
    Desc    string
    Size    int16
    Type    PacketSizeType
    Handler PacketDeserializer
    Alert   bool
}

type PacketDeserializer interface {
    Deserialize() error
}

type BaseDeserializer struct {
    ConnID    uint64
    Timestamp int64
    Payload   []byte
    SourceIP  string
    DestIP    string
    DestPort  int
}

func ReadUint32LE(data []byte, offset int) uint32 {
    if offset+4 > len(data) {
        return 0
    }
    return binary.LittleEndian.Uint32(data[offset : offset+4])
}

func ReadUint16LE(data []byte, offset int) uint16 {
    if offset+2 > len(data) {
        return 0
    }
    return binary.LittleEndian.Uint16(data[offset : offset+2])
}

func ReadNullTerminatedString(data []byte, offset int) string {
    if offset >= len(data) {
        return ""
    }
    
    end := offset
    for end < len(data) && data[end] != 0 {
        end++
    }
    
    return string(data[offset:end])
}

func StringToHex(s string) string {
    return hex.EncodeToString([]byte(s))
}
