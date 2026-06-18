package packets

import (
    "bytes"
    "encoding/binary"
    "fmt"
    "log"
    
    "roproxy/internal/common"
    "roproxy/internal/packets/receive"
    "roproxy/internal/packets/send"
)

type CapturedPacket struct {
    ConnectionID uint64
    Timestamp    int64
    Opcode       uint16
    Size         uint16
    Payload      []byte
    SourceIP     string
    DestIP       string
    DestPort     int
    Direction    uint8  // 0 = server->client, 1 = client->server
}

type StreamParser struct {
    connID uint64
    buffer *bytes.Buffer
    sourceIP string
    destIP   string
    destPort int
    verbose  bool
}

func NewStreamParser(connID uint64, sourceIP, destIP string, destPort int, verbose bool) *StreamParser {
    return &StreamParser{
        connID: connID,
        buffer: &bytes.Buffer{},
        sourceIP: sourceIP,
        destIP:   destIP,
        destPort: destPort,
        verbose:  verbose,
    }
}

func (sp *StreamParser) AppendData(data []byte) {
    sp.buffer.Write(data)
}

func (sp *StreamParser) TryParsePackets(packetChan chan<- *CapturedPacket, timestamp int64, direction uint8) {
    for {
        if sp.buffer.Len() < 2 {
            return
        }

        bufData := sp.buffer.Bytes()
        opcode := binary.LittleEndian.Uint16(bufData[0:2])
        
        var spec *common.PacketSpec
        if direction == 0 {
            // server->client: use receive database
            spec = receive.PacketDatabase[opcode]
        } else {
            // client->server: use send database
            spec = send.PacketDatabase[opcode]
        }
        
        if spec == nil {
            if sp.verbose {
                dirStr := "S->C"
                if direction == 1 {
                    dirStr = "C->S"
                }
                contextLen := 16
                if sp.buffer.Len() < contextLen {
                    contextLen = sp.buffer.Len()
                }
                context := sp.buffer.Bytes()[:contextLen]
                log.Printf("[%d] [%s] WARN: Unknown opcode 0x%04X, discarding 1 byte. Buffer context: %X", 
                    sp.connID, dirStr, opcode, context)
            }
            sp.buffer.Next(1)
            continue
        }

        var packetSize int
        valid := false

        switch spec.Type {
        case common.FIXED, common.FIXED_MIN:
            packetSize = int(spec.Size)
            valid = sp.buffer.Len() >= packetSize

        case common.INDICATED_IN_PACKET:
            if sp.buffer.Len() >= 4 {
                packetSize = int(binary.LittleEndian.Uint16(bufData[2:4]))
                if packetSize < 4 || packetSize > 10485760 {
                    if sp.verbose {
                        dirStr := "S->C"
                        if direction == 1 {
                            dirStr = "C->S"
                        }
                        log.Printf("[%d] [%s] WARN: Invalid packet size %d for opcode 0x%04X, discarding 1 byte", 
                            sp.connID, dirStr, packetSize, opcode)
                    }
                    sp.buffer.Next(1)
                    continue
                }
                valid = sp.buffer.Len() >= packetSize
            }

        case common.HTTP:
            packetSize, valid = sp.parseHTTPPacket()

        case common.UNKNOWN:
            if sp.verbose {
                dirStr := "S->C"
                if direction == 1 {
                    dirStr = "C->S"
                }
                log.Printf("[%d] [%s] WARN: UNKNOWN packet type for opcode 0x%04X, discarding 1 byte", 
                    sp.connID, dirStr, opcode)
            }
            sp.buffer.Next(1)
            continue
        }

        if !valid {
            return
        }

        packetData := make([]byte, packetSize)
        sp.buffer.Read(packetData)

        packet := &CapturedPacket{
            ConnectionID: sp.connID,
            Timestamp:    timestamp,
            Opcode:       opcode,
            Size:         uint16(packetSize),
            Payload:      packetData,
            SourceIP:     sp.sourceIP,
            DestIP:       sp.destIP,
            DestPort:     sp.destPort,
            Direction:    direction,
        }

        packetChan <- packet
    }
}

func (sp *StreamParser) parseHTTPPacket() (int, bool) {
    bufData := sp.buffer.Bytes()
    delimiter := []byte{0x0D, 0x0A, 0x0D, 0x0A}

    headerEnd := bytes.Index(bufData, delimiter)
    if headerEnd == -1 {
        return 0, false
    }

    headerEnd += 4
    headers := string(bufData[:headerEnd])

    if bytes.Contains([]byte(headers), []byte("Transfer-Encoding: chunked")) {
        chunkEnd := bytes.Index(bufData[headerEnd:], delimiter)
        if chunkEnd != -1 {
            totalSize := headerEnd + chunkEnd + 4
            if sp.buffer.Len() >= totalSize {
                return totalSize, true
            }
        }
        return 0, false
    }

    contentLengthIdx := bytes.Index([]byte(headers), []byte("Content-Length: "))
    if contentLengthIdx != -1 {
        start := contentLengthIdx + 16
        end := start
        for end < len(headers) && headers[end] >= '0' && headers[end] <= '9' {
            end++
        }
        contentLength := 0
        fmt.Sscanf(headers[start:end], "%d", &contentLength)
        totalSize := headerEnd + contentLength
        if sp.buffer.Len() >= totalSize {
            return totalSize, true
        }
        return 0, false
    }

    return headerEnd, true
}

func PayloadToHex(payload []byte) string {
    if len(payload) == 0 {
        return ""
    }
    hex := ""
    for i, b := range payload {
        if i > 0 && i%16 == 0 {
            hex += "\n"
        }
        hex += fmt.Sprintf("%02X ", b)
    }
    return hex
}
