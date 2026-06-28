package receive

import (
	"roproxy/internal/common"
	"roproxy/internal/packets"
)

type VenderFound struct {
	packets.ParsedPacket
}

func (v *VenderFound) Deserialize() map[string]any {

    vendorID := common.ReadUint32LE(v.Payload, 0)
    shopName := common.ReadNullTerminatedString(v.Payload, 4)

    shopMap, hasMap := packets.GetConnectionMap(v.ConnectionID)
    if !hasMap {
        common.Log(common.LogPacket, common.LogWarning, "[%d] Vendor found but no map info yet: '%s' (ID:%d)", v.ConnectionID, shopName, vendorID)
        return nil
    }

    data := map[string]interface{}{
        "vendor_id": vendorID,
        "shop_name": common.StringToHex(shopName),
        "shop_map":  common.StringToHex(shopMap),
    }

    common.Log(common.LogPacket, common.LogVeryVerbose, "[%d] Sending vendor to API: '%s' on map '%s' (ID:%d)", v.ConnectionID, shopName, shopMap, vendorID)
    packets.SendToAPI(&v.ParsedPacket, "vending/shop", data)
    return data
}
