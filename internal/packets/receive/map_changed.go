package receive

import (
	"roproxy/internal/common"
	"roproxy/internal/packets"
)

type MapChanged struct {
	packets.ParsedPacket
}

func (m *MapChanged) Deserialize() error {
    mapName := common.ReadNullTerminatedString(m.Payload, 0)
    coordX := common.ReadUint16LE(m.Payload, 16)
    coordY := common.ReadUint16LE(m.Payload, 18)

    common.Log(common.LogPacket, common.LogVeryVerbose, "MapChanged - ConnID=%d, SourceIP='%s', mapName='%s', coords=(%d,%d)", m.ConnectionID, m.SourceIP, mapName, coordX, coordY)

    SetConnectionMap(m.ConnectionID, mapName)
    SetPendingMapChange(m.SourceIP, mapName, coordX, coordY)
    
    common.Log(common.LogPacket, common.LogInfo, "[%d] Map changed to: %s (X:%d Y:%d) - Pending match for %s", m.ConnectionID, mapName, coordX, coordY, m.SourceIP)
    return nil
}
