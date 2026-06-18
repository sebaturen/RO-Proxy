package packets

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

type CapturedPacket struct {
	ConnectionID uint64
	Timestamp    int64
	Opcode       uint16
	Size         uint16
	Payload      []byte
}

type StreamParser struct {
	connID uint64
	buffer *bytes.Buffer
}

func NewStreamParser(connID uint64) *StreamParser {
	return &StreamParser{
		connID: connID,
		buffer: &bytes.Buffer{},
	}
}

func (sp *StreamParser) AppendData(data []byte) {
	sp.buffer.Write(data)
}

func (sp *StreamParser) TryParsePackets(packetChan chan<- *CapturedPacket, timestamp int64) {
	for {
		if sp.buffer.Len() < 2 {
			return
		}

		bufData := sp.buffer.Bytes()
		opcode := binary.LittleEndian.Uint16(bufData[0:2])
		
		spec := PacketDatabase[opcode]
		if spec == nil {
			sp.buffer.Next(1)
			continue
		}

		var packetSize int
		valid := false

		switch spec.Type {
		case FIXED, FIXED_MIN:
			packetSize = int(spec.Size)
			valid = sp.buffer.Len() >= packetSize

		case INDICATED_IN_PACKET:
			if sp.buffer.Len() >= 4 {
				packetSize = int(binary.LittleEndian.Uint16(bufData[2:4]))
				if packetSize < 4 || packetSize > 10485760 {
					sp.buffer.Next(1)
					continue
				}
				valid = sp.buffer.Len() >= packetSize
			}

		case HTTP:
			packetSize, valid = sp.parseHTTPPacket()

		case UNKNOWN:
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
