package receive

import (
    "log"
    "roproxy/internal/common"
)

type OfflineCloneFound struct {
    common.BaseDeserializer
}

func (o *OfflineCloneFound) Deserialize() error {
    if len(o.Payload) < 59 {
        return nil
    }

    cloneID := common.ReadUint32LE(o.Payload, 2)
    jobID := common.ReadUint32LE(o.Payload, 6)
    coordX := common.ReadUint16LE(o.Payload, 10)
    coordY := common.ReadUint16LE(o.Payload, 12)
    sex := o.Payload[14]
    name := common.ReadNullTerminatedString(o.Payload, 37)

    shopMap, hasMap := GetConnectionMap(o.ConnID)
    if !hasMap {
        log.Printf("[%d] Offline clone found but no map info yet: %s (ID:%d)", o.ConnID, name, cloneID)
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
        "PID":      o.ConnID,
        "timestamp":o.Timestamp,
    }

    common.SendToAPI("vending/offline_clone", data)
    log.Printf("[%d] Offline clone: %s on map %s (ID:%d, Job:%d, Sex:%d, X:%d, Y:%d)", o.ConnID, name, shopMap, cloneID, jobID, sex, coordX, coordY)

    return nil
}
