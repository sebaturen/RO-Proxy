package receive

import (
    "roproxy/internal/common"
)

type MapChanged struct {
    common.BaseDeserializer
}

func (m *MapChanged) Deserialize() error {
    mapName := common.ReadNullTerminatedString(m.Payload, 2)
    coordX := common.ReadUint16LE(m.Payload, 18)
    coordY := common.ReadUint16LE(m.Payload, 20)

    common.LogToUI("[yellow][DEBUG MapChanged] ConnID=%d, SourceIP='%s', mapName='%s', coords=(%d,%d)[-]", m.ConnID, m.SourceIP, mapName, coordX, coordY)

    SetConnectionMap(m.ConnID, mapName)
    SetPendingMapChange(m.SourceIP, mapName, coordX, coordY)
    
    common.LogToUI("[green][%d] Map changed to: %s (X:%d Y:%d) - Pending match for %s[-]", m.ConnID, mapName, coordX, coordY, m.SourceIP)
    return nil
}
