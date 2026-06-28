package receive

import (
	"roproxy/internal/common"
	"roproxy/internal/packets"
)

type OfflineCloneFound struct {
	packets.ParsedPacket
}

func (o *OfflineCloneFound) Deserialize() map[string]any {
    cloneID := common.ReadUint32LE(o.Payload, 0)
    jobID := common.ReadUint32LE(o.Payload, 4)
    coordX := common.ReadUint16LE(o.Payload, 8)
    coordY := common.ReadUint16LE(o.Payload, 10)
    sex := o.Payload[12]
    name := common.ReadNullTerminatedString(o.Payload, 35)

    shopMap, hasMap := packets.GetConnectionMap(o.ConnectionID)
    if !hasMap {
        common.Log(common.LogPacket, common.LogWarning, "[%d] Offline clone found but no map info yet: %s (ID:%d)", o.ConnectionID, name, cloneID)
        return nil
    }

    charInfo := map[string]interface{}{
        "job_id": jobID,
        "sex":    sex,
        "name":   common.StringToHex(name),
    }

    data := map[string]interface{}{
        "clone_id": cloneID,
        "info":     charInfo,
        "map":      common.StringToHex(shopMap),
        "coord_x":  coordX,
        "coord_y":  coordY,
    }

    common.Log(common.LogPacket, common.LogVeryVerbose, "[%d] Sending offline clone to API: %s on map %s (ID:%d, Job:%d, Sex:%d, X:%d, Y:%d)", o.ConnectionID, name, shopMap, cloneID, jobID, sex, coordX, coordY)
    packets.SendToAPI(&o.ParsedPacket, "vending/offline_clone", data)

    return nil
}
