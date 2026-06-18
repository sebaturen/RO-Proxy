package packets

import (
    "fmt"
    "log"
    "reflect"
    
    "roproxy/internal/common"
    "roproxy/internal/packets/receive"
)

type PacketProcessor struct {
    connID      uint64
    packetChan  <-chan *CapturedPacket
    stopChan    chan struct{}
    verbose     bool
}

func NewPacketProcessor(connID uint64, packetChan <-chan *CapturedPacket, verbose bool) *PacketProcessor {
    return &PacketProcessor{
        connID:     connID,
        packetChan: packetChan,
        stopChan:   make(chan struct{}),
        verbose:    verbose,
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
    spec := receive.PacketDatabase[packet.Opcode]
    if spec == nil {
        if pp.verbose {
            log.Printf("[%d] Unknown packet: opcode=0x%04X size=%d", 
                packet.ConnectionID, packet.Opcode, packet.Size)
        }
        return
    }

    if pp.verbose {
        log.Printf("[%d] [0x%04X][%s] size=%d payload=%X", packet.ConnectionID, packet.Opcode, spec.Desc, packet.Size, packet.Payload)
    } else {
        fmt.Printf("[%d] [0x%04X][%s]\n", packet.ConnectionID, packet.Opcode, spec.Desc)
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
        if err != nil && pp.verbose {
            log.Printf("[%d] Deserialization error for 0x%04X: %v", packet.ConnectionID, packet.Opcode, err)
        }
    }
}
