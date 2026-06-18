package packets

import (
	"fmt"
	"log"
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
	spec := PacketDatabase[packet.Opcode]
	if spec == nil {
		if pp.verbose {
			log.Printf("[%d] Unknown packet: opcode=0x%04X size=%d", 
				packet.ConnectionID, packet.Opcode, packet.Size)
		}
		return
	}

	if pp.verbose {
		hexPayload := PayloadToHex(packet.Payload)
		log.Printf("[%d] [0x%04X][%s] Size:%d\n%s",
			packet.ConnectionID,
			packet.Opcode,
			spec.Desc,
			packet.Size,
			hexPayload)
	} else {
		fmt.Printf("[%d] [0x%04X][%s]\n", 
			packet.ConnectionID, packet.Opcode, spec.Desc)
	}
}
