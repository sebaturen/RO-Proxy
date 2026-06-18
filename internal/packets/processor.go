package packets

import (
    "fmt"
    "log"
    "reflect"
    
    "roproxy/internal/common"
    "roproxy/internal/packets/receive"
    "roproxy/internal/packets/send"
)

type PacketProcessor struct {
    connID      uint64
    packetChan  <-chan *CapturedPacket
    stopChan    chan struct{}
    verbose     bool
    captureServer   bool
    captureClient   bool
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
        if pp.verbose {
            dirStr := "S->C"
            if packet.Direction == common.ClientToServer {
                dirStr = "C->S"
            }
            log.Printf("[%d] [%s] Unknown packet: opcode=0x%04X size=%d", packet.ConnectionID, dirStr, packet.Opcode, packet.Size)
        }
        return
    }

    dirStr := "S->C"
    if packet.Direction == common.ClientToServer {
        dirStr = "C->S"
    }

    if pp.verbose {
        log.Printf("[%d] [%s] [0x%04X][%s] size=%d payload=%X", packet.ConnectionID, dirStr, packet.Opcode, spec.Desc, packet.Size, packet.Payload)
    } else {
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
                log.Printf("[%d] [%s] Deserialization error for 0x%04X: %v", packet.ConnectionID, dirStr, packet.Opcode, err)
            }
        }
    }
}
