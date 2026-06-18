package receive

import (
    "log"
    "roproxy/internal/common"
)

type MapChanged struct {
    common.BaseDeserializer
}

func (m *MapChanged) Deserialize() error {
    if len(m.Payload) < 26 {
        return nil
    }

    mapName := common.ReadNullTerminatedString(m.Payload, 2)
    coordX := common.ReadUint16LE(m.Payload, 18)
    coordY := common.ReadUint16LE(m.Payload, 20)

    SetConnectionMap(m.ConnID, mapName)
    log.Printf("[%d] Map changed to: %s (X:%d Y:%d)", m.ConnID, mapName, coordX, coordY)
    return nil
}
