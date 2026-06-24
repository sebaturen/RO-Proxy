package receive

import (
	"net"
	"roproxy/internal/common"
	"roproxy/internal/packets"
	"strconv"
	"strings"
	"time"
)

type ReceivedCharIdAndMap struct {
	packets.ParsedPacket
}

func (r *ReceivedCharIdAndMap) Deserialize() error {
    charID := common.ReadUint32LE(r.Payload, 0)
    mapName := common.ReadNullTerminatedString(r.Payload, 4)
    mapURL := common.ReadNullTerminatedString(r.Payload, 26)

    common.Log(common.LogPacket, common.LogVeryVerbose, "ReceivedCharIdAndMap - ConnID=%d, mapName='%s', mapURL='%s'", r.ConnectionID, mapName, mapURL)

    SetConnectionMap(r.ConnectionID, mapName)
    
    parts := strings.Split(mapURL, ":")
    hostname := parts[0]
    port, _ := strconv.Atoi(parts[1])
    
    // DNS lookup with timeout
    startDNS := time.Now()
    ips, err := net.LookupIP(hostname)
    dnsDuration := time.Since(startDNS)
    
    if err != nil || len(ips) == 0 {
        common.Log(common.LogPacket, common.LogError, "[%d] DNS lookup failed for %s (took %dms): %v", r.ConnectionID, hostname, dnsDuration.Milliseconds(), err)
        return nil
    }
    
    destIP := ips[0].String()
    SetPendingMapByDestination(destIP, port, mapName)
    
    if dnsDuration > 100*time.Millisecond {
        common.Log(common.LogPacket, common.LogWarning, "[%d] DNS lookup SLOW for %s: %dms - Map: %s - Next: %s:%d (%s)", r.ConnectionID, hostname, dnsDuration.Milliseconds(), mapName, hostname, port, destIP)
    } else {
        common.Log(common.LogPacket, common.LogInfo, "[%d] Received char ID %d - Map: %s - Next: %s:%d (%s) - Pending match", r.ConnectionID, charID, mapName, hostname, port, destIP)
    }

    return nil
}
