package receive

import (
    "log"
    "roproxy/internal/common"
)

type MapLoaded struct {
    common.BaseDeserializer
}

func (m *MapLoaded) Deserialize() error {
    coordsCompressed := uint32(m.Payload[2])<<16 | uint32(m.Payload[3])<<8 | uint32(m.Payload[4])
    
    coordX := uint16(coordsCompressed >> 14)
    coordY := uint16((coordsCompressed >> 4) & 0x3FF)

    mapName, foundByCoords := GetPendingMapChange(m.SourceIP, coordX, coordY)
    if foundByCoords {
        SetConnectionMap(m.ConnID, mapName)
        log.Printf("[%d] Map loaded: %s (X:%d Y:%d) - Matched by coords from %s", m.ConnID, mapName, coordX, coordY, m.SourceIP)
        return nil
    }

    mapName, foundByDest := GetPendingMapByDestination(m.DestIP, m.DestPort)
    if foundByDest {
        SetConnectionMap(m.ConnID, mapName)
        log.Printf("[%d] Map loaded: %s (X:%d Y:%d) - Matched by destination %s:%d", m.ConnID, mapName, coordX, coordY, m.DestIP, m.DestPort)
        return nil
    }

    log.Printf("[%d] Map loaded at (X:%d Y:%d) but no pending match (source: %s, dest: %s:%d)", m.ConnID, coordX, coordY, m.SourceIP, m.DestIP, m.DestPort)

    return nil
}
