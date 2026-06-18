package receive

import (
    "log"
    "roproxy/internal/packets"
)

type VenderFound struct {
    BaseDeserializer
}

func (v *VenderFound) Deserialize() error {
    if len(v.Payload) < 6 {
        return nil
    }

    vendorID := ReadUint32LE(v.Payload, 2)
    shopName := ReadNullTerminatedString(v.Payload, 6)

    shopMap, hasMap := packets.GetConnectionMap(v.ConnID)
    if !hasMap {
        log.Printf("[%d] Vendor found but no map info yet: %s (ID:%d)", v.ConnID, shopName, vendorID)
        return nil
    }

    data := map[string]interface{}{
        "vendor_id": vendorID,
        "shop_name": StringToHex(shopName),
        "shop_map":  StringToHex(shopMap),
        "PID": fmt.Sprintf("%d", v.ConnID),
        "timestamp": v.Timestamp
    }

    packets.SendToAPI("vending/shop", data)
    log.Printf("[%d] Vendor: %s on map %s (ID:%d)", v.ConnID, shopName, shopMap, vendorID)
    return nil
}
