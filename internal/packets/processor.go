package packets

import (
    "fmt"
    "log"
    "reflect"
    
    "roproxy/internal/common"
    "roproxy/internal/packets/receive"
    "roproxy/internal/packets/send"
)

type PacketLogger interface {
    LogPacket(direction common.PacketDirection, opcode uint16, size int)
    LogUnknown(direction common.PacketDirection, opcode uint16, size int)
    IsDebugMode() bool
    IsShowWarnings() bool
}

type PacketProcessor struct {
    connID      uint64
    packetChan  <-chan *CapturedPacket
    stopChan    chan struct{}
    verbose     bool
    captureServer   bool
    captureClient   bool
    logger        PacketLogger
}

func NewPacketProcessor(connID uint64, packetChan <-chan *CapturedPacket, verbose, captureServer, captureClient bool) *PacketProcessor {
    return &PacketProcessor{
        connID:     connID,
        packetChan: packetChan,
        stopChan:   make(chan struct{}),
        verbose:    verbose,
        captureServer: captureServer,
        captureClient: captureClient,
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
    // Filter by direction
    if packet.Direction == common.ServerToClient && !pp.captureServer {
        return
    }
    if packet.Direction == common.ClientToServer && !pp.captureClient {
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
            pp.logger.LogUnknown(packet.Direction, packet.Opcode, int(packet.Size))
        } else if pp.verbose {
            dirStr := "S->C"
            if packet.Direction == common.ClientToServer {
                dirStr = "C->S"
            }
            log.Printf("[%d] [%s] Unknown packet: opcode=0x%04X size=%d", packet.ConnectionID, dirStr, packet.Opcode, packet.Size)
        }
        return
    }

    // Known packet
    if pp.logger != nil && pp.logger.IsDebugMode() {
        pp.logger.LogPacket(packet.Direction, packet.Opcode, int(packet.Size))
    } else if pp.verbose {
        dirStr := "S->C"
        if packet.Direction == common.ClientToServer {
            dirStr = "C->S"
        }
        logMsg := fmt.Sprintf("[%d] [%s] [0x%04X][%s] size=%d payload=%X", packet.ConnectionID, dirStr, packet.Opcode, spec.Desc, packet.Size, packet.Payload)
        if packet.Checksum != nil {
            logMsg += fmt.Sprintf(" checksum=0x%02X", *packet.Checksum)
        }
        log.Println(logMsg)
    } else if !pp.verbose && pp.logger == nil {
        dirStr := "S->C"
        if packet.Direction == common.ClientToServer {
            dirStr = "C->S"
        }
        fmt.Printf("[%d] [%s] [0x%04X][%s]\n", packet.ConnectionID, dirStr, packet.Opcode, spec.Desc)
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
        
        err := spec.Handler.Deserialize()
        if err != nil {
            if pp.verbose {
                dirStr := "S->C"
                if packet.Direction == common.ClientToServer {
                	dirStr = "C->S"
                }
                log.Printf("[%d] [%s] Deserialization error for 0x%04X: %v", packet.ConnectionID, dirStr, packet.Opcode, err)
            }
        }
    }
}
