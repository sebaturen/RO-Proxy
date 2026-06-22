package receive

import (
    "net"
    "strconv"
    "strings"
    "time"
    "roproxy/internal/common"
)

type ReceivedCharIdAndMap struct {
    common.BaseDeserializer
}

func (r *ReceivedCharIdAndMap) Deserialize() error {
    charID := common.ReadUint32LE(r.Payload, 2)
    mapName := common.ReadNullTerminatedString(r.Payload, 6)
    mapURL := common.ReadNullTerminatedString(r.Payload, 28)

    common.LogToUI("[yellow][DEBUG ReceivedCharIdAndMap] ConnID=%d, mapName='%s', mapURL='%s'[-]", r.ConnID, mapName, mapURL)

    SetConnectionMap(r.ConnID, mapName)
    
    parts := strings.Split(mapURL, ":")
    hostname := parts[0]
    port, _ := strconv.Atoi(parts[1])
    
    // DNS lookup with timeout
    startDNS := time.Now()
    ips, err := net.LookupIP(hostname)
    dnsDuration := time.Since(startDNS)
    
    if err != nil || len(ips) == 0 {
        common.LogToUI("[red][%d] DNS lookup failed for %s (took %dms): %v[-]", r.ConnID, hostname, dnsDuration.Milliseconds(), err)
        return nil
    }
    
    destIP := ips[0].String()
    SetPendingMapByDestination(destIP, port, mapName)
    
    if dnsDuration > 100*time.Millisecond {
        common.LogToUI("[yellow][%d] DNS lookup SLOW for %s: %dms - Map: %s - Next: %s:%d (%s)[-]", r.ConnID, hostname, dnsDuration.Milliseconds(), mapName, hostname, port, destIP)
    } else {
        common.LogToUI("[green][%d] Received char ID %d - Map: %s - Next: %s:%d (%s) - Pending match[-]", r.ConnID, charID, mapName, hostname, port, destIP)
    }

    return nil
}
