package packets

import (
    "reflect"

    "roproxy/internal/common"
    "roproxy/internal/packets/receive"
    "roproxy/internal/packets/send"
)

type PacketLogger interface {
    LogPacket(connID uint64, direction common.PacketDirection, opcode uint16, size int, desc string, payload []byte, checksum *uint8)
    LogUnknown(connID uint64, direction common.PacketDirection, opcode uint16, size int, payload []byte, checksum *uint8)
    IsDebugMode() bool
    IsShowWarnings() bool
}

type CaptureSettings interface {
    GetCaptureServer() bool
    GetCaptureClient() bool
}

type PacketProcessor struct {
    connID          uint64
    packetChan      <-chan *CapturedPacket
    stopChan        chan struct{}
    verbose         bool
    captureSettings CaptureSettings
    logger          PacketLogger
}

func NewPacketProcessor(connID uint64, packetChan <-chan *CapturedPacket, verbose bool, captureSettings CaptureSettings) *PacketProcessor {
    return &PacketProcessor{
        connID:          connID,
        packetChan:      packetChan,
        stopChan:        make(chan struct{}),
        verbose:         verbose,
        captureSettings: captureSettings,
    }
}

func (pp *PacketProcessor) SetLogger(logger PacketLogger) {
    pp.logger = logger
}

func (pp *PacketProcessor) Start() {
    go pp.processLoop()
}

func (pp *PacketProcessor) Stop() {
    close(pp.stopChan)
}

func (pp *PacketProcessor) processLoop() {
    for {
        select {
        case packet, ok := <-pp.packetChan:
            if !ok {
                return
            }
            pp.processPacket(packet)

        case <-pp.stopChan:
            return
        }
    }
}

func (pp *PacketProcessor) processPacket(packet *CapturedPacket) {
    // Filter by direction - read dynamically from proxy settings
    if packet.Direction == common.ServerToClient && !pp.captureSettings.GetCaptureServer() {
        return
    }
    if packet.Direction == common.ClientToServer && !pp.captureSettings.GetCaptureClient() {
        return
    }

    var spec *common.PacketSpec
    if packet.Direction == common.ServerToClient {
        spec = receive.PacketDatabase[packet.Opcode]
    } else {
        spec = send.PacketDatabase[packet.Opcode]
    }

    if spec == nil {
        // Unknown packet
        if pp.logger != nil && pp.logger.IsShowWarnings() {
            pp.logger.LogUnknown(packet.ConnectionID, packet.Direction, packet.Opcode, int(packet.Size), packet.Payload, packet.Checksum)
        }
        return
    }

    // Known packet
    desc := spec.Desc
    if desc == "" {
        desc = "Unknown"
    }
	
    if pp.logger != nil && pp.logger.IsDebugMode() {
        pp.logger.LogPacket(packet.ConnectionID, packet.Direction, packet.Opcode, int(packet.Size), desc, packet.Payload, packet.Checksum)
    }

    if spec.Handler != nil {
        handlerValue := reflect.ValueOf(spec.Handler)
        if handlerValue.Kind() == reflect.Ptr {
            handlerValue = handlerValue.Elem()
        }

        baseField := handlerValue.FieldByName("BaseDeserializer")
        if baseField.IsValid() && baseField.CanSet() {
            baseField.Set(reflect.ValueOf(common.BaseDeserializer{
                ConnID:    packet.ConnectionID,
                Timestamp: packet.Timestamp,
                Payload:   packet.Payload,
                SourceIP:  packet.SourceIP,
                DestIP:    packet.DestIP,
                DestPort:  packet.DestPort,
            }))
        }

        spec.Handler.Deserialize()
    }
}
