package receive

import (
    "log"
    "roproxy/internal/common"
)

type VenderFound struct {
    common.BaseDeserializer
}

func (v *VenderFound) Deserialize() error {

    vendorID := common.ReadUint32LE(v.Payload, 2)
    shopName := common.ReadNullTerminatedString(v.Payload, 6)

    shopMap, hasMap := GetConnectionMap(v.ConnID)
    if !hasMap {
        log.Printf("[%d] Vendor found but no map info yet: '%s' (ID:%d)", v.ConnID, shopName, vendorID)
        return nil
    }

    data := map[string]interface{}{
        "vendor_id": vendorID,
        "shop_name": common.StringToHex(shopName),
        "shop_map":  common.StringToHex(shopMap),
        "PID":       v.ConnID,
        "timestamp": v.Timestamp,
    }

    common.SendToAPI("vending/shop", data)
    log.Printf("[%d] Vendor: '%s' on map '%s' (ID:%d)", v.ConnID, shopName, shopMap, vendorID)
    return nil
}
