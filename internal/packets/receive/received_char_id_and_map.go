package receive

import (
    "log"
    "net"
    "strconv"
    "strings"
    "roproxy/internal/common"
)

type ReceivedCharIdAndMap struct {
    common.BaseDeserializer
}

func (r *ReceivedCharIdAndMap) Deserialize() error {
    charID := common.ReadUint32LE(r.Payload, 2)
    mapName := common.ReadNullTerminatedString(r.Payload, 6)
    mapURL := common.ReadNullTerminatedString(r.Payload, 28)

    SetConnectionMap(r.ConnID, mapName)
    
    parts := strings.Split(mapURL, ":")
    hostname := parts[0]
    port, _ := strconv.Atoi(parts[1])
    
    ips, _ := net.LookupIP(hostname)
    destIP := ips[0].String()
    SetPendingMapByDestination(destIP, port, mapName)
    log.Printf("[%d] Received char ID %d - Map: %s - Next: %s:%d (%s) - Pending match", r.ConnID, charID, mapName, hostname, port, destIP)

    return nil
}
