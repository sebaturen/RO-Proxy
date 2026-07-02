package analyzer

import (
	"bytes"
	"context"
	"encoding/binary"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"roproxy/internal/common"
	"roproxy/internal/packets"
	"roproxy/internal/packets/receive"
	"roproxy/internal/packets/send"

	"golang.org/x/sync/semaphore"
)

type Worker struct {
	ConnectionID   uint64
	ClientAddr     string
	ServerAddr     string
	RawChunkBuffer chan *packets.RawChunk // Reference (Connection owns it)
	semaphore      *semaphore.Weighted    // 100 deserializers per worker
	cancel         context.CancelFunc
	wg             sync.WaitGroup
}

func NewWorker(connectionID uint64, clientAddr, serverAddr string, buffer chan *packets.RawChunk) *Worker {
	return &Worker{
		ConnectionID:   connectionID,
		ClientAddr:     clientAddr,
		ServerAddr:     serverAddr,
		RawChunkBuffer: buffer,
		semaphore:      semaphore.NewWeighted(100),
	}
}

func (w *Worker) Start(ctx context.Context) {
	workerCtx, cancel := context.WithCancel(ctx)
	w.cancel = cancel

	w.wg.Add(1)
	go w.workerLoop(workerCtx)
}

func (w *Worker) Wait() {
	w.wg.Wait()
}

func (w *Worker) Close() {
	if w.cancel != nil {
		w.cancel()
	}
}

func (w *Worker) workerLoop(ctx context.Context) {
	defer w.wg.Done()

	// CRITICAL: Auto-recover to prevent analyzer crash
	defer func() {
		if r := recover(); r != nil {
			common.Log(common.LogPacket, common.LogError, "PANIC RECOVERED in workerLoop (Connection #%d): %v - Worker restarting", w.ConnectionID, r)
			go w.workerLoop(ctx)
		}
	}()

	clientBuffer := &bytes.Buffer{}
	serverBuffer := &bytes.Buffer{}

	for {
		select {
		case <-ctx.Done():
			w.drainRemainingPackets(clientBuffer, serverBuffer)
			return

		case chunk, ok := <-w.RawChunkBuffer:
			if !ok {
				// Channel closed
				w.drainRemainingPackets(clientBuffer, serverBuffer)
				return
			}
			w.processChunk(chunk, clientBuffer, serverBuffer)
		}
	}
}

func (w *Worker) processChunk(chunk *packets.RawChunk, clientBuffer, serverBuffer *bytes.Buffer) {
	if chunk.Direction == common.ClientToServer {
		clientBuffer.Write(chunk.Data)
		parsedPackets := w.tryParsePackets(clientBuffer, chunk.Direction, chunk.Timestamp)
		for _, pkt := range parsedPackets {
			w.spawnDeserializer(pkt)
		}
	} else {
		serverBuffer.Write(chunk.Data)
		parsedPackets := w.tryParsePackets(serverBuffer, chunk.Direction, chunk.Timestamp)
		for _, pkt := range parsedPackets {
			w.spawnDeserializer(pkt)
		}
	}
}

func (w *Worker) drainRemainingPackets(clientBuffer, serverBuffer *bytes.Buffer) {
	common.Log(common.LogPacket, common.LogVeryVerbose, "Connection #%d draining remaining packets (buffer size: %d)", w.ConnectionID, len(w.RawChunkBuffer))

	processed := 0
	timeout := time.After(500 * time.Millisecond)

	for {
		select {
		case chunk, ok := <-w.RawChunkBuffer:
			if !ok {
				w.waitForDeserializers()
				common.Log(common.LogPacket, common.LogVeryVerbose, "Connection #%d drained %d packets", w.ConnectionID, processed)
				return
			}

			w.processChunk(chunk, clientBuffer, serverBuffer)
			processed++

		case <-timeout:
			w.waitForDeserializers()
			common.Log(common.LogPacket, common.LogVeryVerbose, "Connection #%d drain timeout - processed %d packets, %d remaining",
				w.ConnectionID, processed, len(w.RawChunkBuffer))
			return
		}
	}
}

func (w *Worker) waitForDeserializers() {
	deadline := time.Now().Add(1 * time.Second)

	for {
		if w.semaphore.TryAcquire(100) {
			w.semaphore.Release(100)
			common.Log(common.LogPacket, common.LogVeryVerbose, "Connection #%d - all deserializers finished", w.ConnectionID)
			return
		}

		if time.Now().After(deadline) {
			common.Log(common.LogPacket, common.LogVeryVerbose, "Connection #%d - timeout waiting for deserializers", w.ConnectionID)
			return
		}

		time.Sleep(50 * time.Millisecond)
	}
}

func (w *Worker) spawnDeserializer(pkt *packets.ParsedPacket) {
	go func() {
		w.semaphore.Acquire(context.Background(), 1)
		defer w.semaphore.Release(1)

		var spec *common.PacketSpec
		if pkt.Direction == common.ServerToClient {
			spec = receive.PacketDatabase[pkt.Opcode]
		} else {
			spec = send.PacketDatabase[pkt.Opcode]
		}

		// Format packet info for logging
		dirSymbol := common.FormatDirection(pkt.Direction)
		checksumStr := common.FormatChecksum(pkt.Checksum)

		desc := ""
		if spec != nil && spec.Desc != "" {
			desc = spec.Desc
		}
		descDisplay := common.FormatDesc(desc)
		payloadHex := common.FormatPayload(pkt.Payload, true)

		common.Log(common.LogPacket, common.LogVerbose, "[yellow][#%d][-]%s[yellow][0x%04X][-]%s [white]size=%d%s payload=%s[-]", pkt.ConnectionID, dirSymbol, pkt.Opcode, descDisplay, len(pkt.Payload), checksumStr, payloadHex)

		// Call deserializer handler if exists
		unknownPkt := true
		if spec != nil {
			unknownPkt = false
		}
		if spec != nil && spec.Handler != nil {
			handlerType := reflect.TypeOf(spec.Handler).Elem()
			newHandler := reflect.New(handlerType)

			packetField := newHandler.Elem().FieldByName("ParsedPacket")
			if packetField.IsValid() && packetField.CanSet() {
				pktValue := reflect.ValueOf(pkt)
				if pktValue.Kind() == reflect.Ptr {
					pktValue = pktValue.Elem()
				}

				packetField.Set(pktValue)
			}

			newHandler.Interface().(common.PacketDeserializer).Deserialize()
		}
		
		// Track packet stats
		common.AddPacket(pkt.Direction, len(pkt.Payload), unknownPkt)
	}()
}

func (w *Worker) tryParsePackets(buffer *bytes.Buffer, direction common.PacketDirection, timestamp int64) []*packets.ParsedPacket {
	var result []*packets.ParsedPacket

	for {
		if buffer.Len() < 2 {
			return result
		}

		bufData := buffer.Bytes()
		opcode := binary.LittleEndian.Uint16(bufData[0:2])

		var spec *common.PacketSpec
		if direction == common.ServerToClient {
			spec = receive.PacketDatabase[opcode]
		} else {
			spec = send.PacketDatabase[opcode]
		}

		if spec == nil {
			buffer.Next(1)
			continue
		}

		var packetSize int
		valid := false

		switch spec.Type {
		case common.FIXED, common.FIXED_MIN:
			packetSize = int(spec.Size)
			valid = buffer.Len() >= packetSize

		case common.INDICATED_IN_PACKET:
			if buffer.Len() >= 4 {
				packetSize = int(binary.LittleEndian.Uint16(bufData[2:4]))
				if packetSize < 2 || packetSize > 10*1024*1024 {
					payload := common.FormatPayload(bufData, false)
					ptDir := common.FormatDirection(direction)
					common.Log(common.LogPacket, common.LogError, "CRITICAL: Invalid packet size %d (dir=%s, opcode=0x%04X, conn=%d) payload %s", packetSize, ptDir, opcode, w.ConnectionID, payload)
					packetSize = 2
				}
				valid = buffer.Len() >= packetSize
			}

		case common.HTTP:
			packetSize, valid = w.parseHTTPPacket(buffer)

		case common.UNKNOWN:
			buffer.Next(1)
			continue
		}

		if !valid {
			return result
		}

		packetData := make([]byte, packetSize)
		buffer.Read(packetData)

		var checksum *uint8
		if direction == common.ClientToServer && buffer.Len() > 0 {
			remainingBytes := buffer.Bytes()

			if len(remainingBytes) == 1 {
				extraByte := make([]byte, 1)
				buffer.Read(extraByte)
				checksum = &extraByte[0]
			} else if len(remainingBytes) >= 2 {
				nextOpcode := binary.LittleEndian.Uint16(remainingBytes[0:2])
				if send.PacketDatabase[nextOpcode] != nil {
					checksum = nil
				} else if len(remainingBytes) >= 3 {
					nextOpcodeAfterByte := binary.LittleEndian.Uint16(remainingBytes[1:3])
					if send.PacketDatabase[nextOpcodeAfterByte] != nil {
						extraByte := make([]byte, 1)
						buffer.Read(extraByte)
						checksum = &extraByte[0]
					}
				}
			}
		}

		sourceIP, sourcePort, destIP, destPort := w.parseNetworkAddresses(direction)

		var startData = 2
		if spec.Size == -1 {
			startData = 4
		}

		if startData > len(packetData) {
			startData = len(packetData)
		}

		result = append(result, &packets.ParsedPacket{
			ConnectionID: w.ConnectionID,
			Timestamp:    timestamp,
			Direction:    direction,
			Opcode:       opcode,
			Payload:      packetData[startData:],
			Checksum:     checksum,
			SourceIP:     sourceIP,
			SourcePort:   sourcePort,
			DestIP:       destIP,
			DestPort:     destPort,
		})
	}
}

func (w *Worker) parseNetworkAddresses(direction common.PacketDirection) (string, int, string, int) {
	var sourceIP, destIP string
	var sourcePort, destPort int

	if direction == common.ClientToServer {
		clientParts := strings.Split(w.ClientAddr, ":")
		sourceIP = clientParts[0]
		if len(clientParts) > 1 {
			sourcePort, _ = strconv.Atoi(clientParts[1])
		}

		serverParts := strings.Split(w.ServerAddr, ":")
		destIP = serverParts[0]
		if len(serverParts) > 1 {
			destPort, _ = strconv.Atoi(serverParts[1])
		}
	} else {
		serverParts := strings.Split(w.ServerAddr, ":")
		sourceIP = serverParts[0]
		if len(serverParts) > 1 {
			sourcePort, _ = strconv.Atoi(serverParts[1])
		}

		clientParts := strings.Split(w.ClientAddr, ":")
		destIP = clientParts[0]
		if len(clientParts) > 1 {
			destPort, _ = strconv.Atoi(clientParts[1])
		}
	}

	return sourceIP, sourcePort, destIP, destPort
}

func (w *Worker) parseHTTPPacket(buffer *bytes.Buffer) (int, bool) {
	bufData := buffer.Bytes()
	delimiter := []byte{0x0D, 0x0A, 0x0D, 0x0A}

	headerEnd := bytes.Index(bufData, delimiter)
	if headerEnd == -1 {
		return 0, false
	}

	headerEnd += 4
	return headerEnd, true
}
