package receive

import (
	"roproxy/internal/common"
	"roproxy/internal/packets"
)

type MapLoaded struct {
	packets.ParsedPacket
}

func (m *MapLoaded) Deserialize() map[string]any {
    coordsCompressed := uint32(m.Payload[4])<<16 | uint32(m.Payload[5])<<8 | uint32(m.Payload[6])
    
    coordX := uint16(coordsCompressed >> 14)
    coordY := uint16((coordsCompressed >> 4) & 0x3FF)

    common.Log(common.LogPacket, common.LogVeryVerbose, "MapLoaded - ConnID=%d, SourceIP='%s:%d', DestIP='%s:%d', coords=(%d,%d)", m.ConnectionID, m.SourceIP, m.SourcePort, m.DestIP, m.DestPort, coordX, coordY)

    // Try match by coords first (using SourceIP which is client for C→S, server for S→C)
    mapName, foundByCoords := packets.GetPendingMapChange(m.SourceIP, coordX, coordY)
    if foundByCoords {
        packets.SetConnectionMap(m.ConnectionID, mapName)
        common.Log(common.LogPacket, common.LogInfo, "[%d] Map loaded: %s (X:%d Y:%d) - Matched by coords from %s", m.ConnectionID, mapName, coordX, coordY, m.SourceIP)
        return nil
    }

    // Try match by destination (using SourceIP:SourcePort for S→C packets, which is the current server)
    mapName, foundByDest := packets.GetPendingMapByDestination(m.SourceIP, m.SourcePort)
    if foundByDest {
        packets.SetConnectionMap(m.ConnectionID, mapName)
        common.Log(common.LogPacket, common.LogInfo, "[%d] Map loaded: %s (X:%d Y:%d) - Matched by destination %s:%d", m.ConnectionID, mapName, coordX, coordY, m.SourceIP, m.SourcePort)
        return nil
    }

    common.Log(common.LogPacket, common.LogWarning, "[%d] Map loaded at (X:%d Y:%d) but no pending match (source: %s:%d, dest: %s:%d)", m.ConnectionID, coordX, coordY, m.SourceIP, m.SourcePort, m.DestIP, m.DestPort)

    return nil
}
