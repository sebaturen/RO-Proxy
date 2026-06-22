package receive

import (
    "roproxy/internal/common"
)

type MapLoaded struct {
    common.BaseDeserializer
}

func (m *MapLoaded) Deserialize() error {
    coordsCompressed := uint32(m.Payload[2])<<16 | uint32(m.Payload[3])<<8 | uint32(m.Payload[4])
    
    coordX := uint16(coordsCompressed >> 14)
    coordY := uint16((coordsCompressed >> 4) & 0x3FF)

    common.LogToUI("[yellow][DEBUG MapLoaded] ConnID=%d, SourceIP='%s:%d', DestIP='%s:%d', coords=(%d,%d)[-]", m.ConnID, m.SourceIP, m.SourcePort, m.DestIP, m.DestPort, coordX, coordY)

    // Try match by coords first (using SourceIP which is client for C→S, server for S→C)
    mapName, foundByCoords := GetPendingMapChange(m.SourceIP, coordX, coordY)
    if foundByCoords {
        SetConnectionMap(m.ConnID, mapName)
        common.LogToUI("[green][%d] Map loaded: %s (X:%d Y:%d) - Matched by coords from %s[-]", m.ConnID, mapName, coordX, coordY, m.SourceIP)
        return nil
    }

    // Try match by destination (using SourceIP:SourcePort for S→C packets, which is the current server)
    mapName, foundByDest := GetPendingMapByDestination(m.SourceIP, m.SourcePort)
    if foundByDest {
        SetConnectionMap(m.ConnID, mapName)
        common.LogToUI("[green][%d] Map loaded: %s (X:%d Y:%d) - Matched by destination %s:%d[-]", m.ConnID, mapName, coordX, coordY, m.SourceIP, m.SourcePort)
        return nil
    }

    common.LogToUI("[yellow][%d] Map loaded at (X:%d Y:%d) but no pending match (source: %s:%d, dest: %s:%d)[-]", m.ConnID, coordX, coordY, m.SourceIP, m.SourcePort, m.DestIP, m.DestPort)

    return nil
}
