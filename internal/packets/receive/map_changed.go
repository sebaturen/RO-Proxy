package receive

import (
    "log"
    "roproxy/internal/packets"
)

type MapChanged struct {
    BaseDeserializer
}

func (m *MapChanged) Deserialize() error {
    if len(m.Payload) < 26 {
        return nil
    }

    mapName := ReadNullTerminatedString(m.Payload, 2)
    coordX := ReadUint16LE(m.Payload, 18)
    coordY := ReadUint16LE(m.Payload, 20)

    packets.SetConnectionMap(m.ConnID, mapName)
    log.Printf("[%d] Map changed to: %s (X:%d Y:%d)", m.ConnID, mapName, coordX, coordY)
    return nil
}
