package receive

import (
	"encoding/hex"
	"roproxy/internal/common"
	"roproxy/internal/packets"
)

const (
	VendorPacketItemList4 = 0x0b62
	VendorPacketItemList3 = 0x0b3d
)

type VenderItemsLists struct {
	packets.ParsedPacket
}

type VenderItem struct {
    Price           uint32
    Quantity        uint16
    Position        uint16
    Type            uint8
    ItemID          uint32
    UnknownValue    uint16
    CardSlot1       uint32
    CardSlot2       uint32
    CardSlot3       uint32
    CardSlot4       uint32
    EnchantSlot1    uint16
    EnchantSlot1Val uint32
    EnchantSlot2    uint16
    EnchantSlot2Val uint32
    EnchantSlot3    uint16
    EnchantSlot3Val uint32
    EnchantSlot4    uint16
    EnchantSlot4Val uint32
    UnknownPart     string
    Refine          uint16
}

func (v *VenderItemsLists) Deserialize() error {
    opcode := v.Opcode
    
    var vendorID, vendorCID uint32
    var flag, expiredDate uint32
    var itemListStart int

    if opcode == VendorPacketItemList4 {
        vendorID = common.ReadUint32LE(v.Payload, 4)
        vendorCID = common.ReadUint32LE(v.Payload, 8)
        flag = uint32(v.Payload[12])
        expiredDate = common.ReadUint32LE(v.Payload, 13)
        itemListStart = 17
    } else if opcode == VendorPacketItemList3 {
        vendorID = common.ReadUint32LE(v.Payload, 4)
        vendorCID = common.ReadUint32LE(v.Payload, 8)
        itemListStart = 12
    } else {
        return nil
    }

    items := []VenderItem{}
    i := itemListStart

    for i+63 <= len(v.Payload) {
        item := VenderItem{}
        item.Price = common.ReadUint32LE(v.Payload, i)
        item.Quantity = common.ReadUint16LE(v.Payload, i+4)
        item.Position = common.ReadUint16LE(v.Payload, i+6)
        item.Type = v.Payload[i+8]
        item.ItemID = common.ReadUint32LE(v.Payload, i+9)
        item.UnknownValue = common.ReadUint16LE(v.Payload, i+13)

        cs := i + 14
        item.CardSlot1 = common.ReadUint32LE(v.Payload, cs+1)
        item.CardSlot2 = common.ReadUint32LE(v.Payload, cs+5)
        item.CardSlot3 = common.ReadUint32LE(v.Payload, cs+9)
        item.CardSlot4 = common.ReadUint32LE(v.Payload, cs+13)

        es := cs + 16
        item.EnchantSlot1 = common.ReadUint16LE(v.Payload, es+1)
        item.EnchantSlot1Val = uint32(v.Payload[es+3]) | (uint32(v.Payload[es+4]) << 8) | (uint32(v.Payload[es+5]) << 16)
        item.EnchantSlot2 = common.ReadUint16LE(v.Payload, es+6)
        item.EnchantSlot2Val = uint32(v.Payload[es+8]) | (uint32(v.Payload[es+9]) << 8) | (uint32(v.Payload[es+10]) << 16)
        item.EnchantSlot3 = common.ReadUint16LE(v.Payload, es+11)
        item.EnchantSlot3Val = uint32(v.Payload[es+13]) | (uint32(v.Payload[es+14]) << 8) | (uint32(v.Payload[es+15]) << 16)
        item.EnchantSlot4 = common.ReadUint16LE(v.Payload, es+16)
        item.EnchantSlot4Val = uint32(v.Payload[es+18]) | (uint32(v.Payload[es+19]) << 8) | (uint32(v.Payload[es+20]) << 16)

        uk := es + 20
        ukEnd := uk + 11
        unknownBytes := []byte{}
        for u := uk; u < ukEnd && u+1 < len(v.Payload); u++ {
            unknownBytes = append(unknownBytes, v.Payload[u+1])
        }
        item.UnknownPart = hex.EncodeToString(unknownBytes)

        r := ukEnd
        if r+2 < len(v.Payload) {
            item.Refine = common.ReadUint16LE(v.Payload, r+1)
        }
        i = r + 3

        items = append(items, item)
    }

    shopMap, hasMap := GetConnectionMap(v.ConnectionID)
    if !hasMap {
        common.Log(common.LogPacket, common.LogWarning, "[%d] Vender items list received but no map info yet (vendor:%d, items:%d)", v.ConnectionID, vendorID, len(items))
        return nil
    }

    shopItems := []map[string]interface{}{}
    for _, item := range items {
        apiItem := map[string]interface{}{
            "item_id":           item.ItemID,
            "type":              item.Type,
            "refine":            item.Refine,
            "card_slot_1":       item.CardSlot1,
            "card_slot_2":       item.CardSlot2,
            "card_slot_3":       item.CardSlot3,
            "card_slot_4":       item.CardSlot4,
            "enchant_slot_1":    item.EnchantSlot1,
            "enchant_slot_1_val": item.EnchantSlot1Val,
            "enchant_slot_2":    item.EnchantSlot2,
            "enchant_slot_2_val": item.EnchantSlot2Val,
            "enchant_slot_3":    item.EnchantSlot3,
            "enchant_slot_3_val": item.EnchantSlot3Val,
            "enchant_slot_4":    item.EnchantSlot4,
            "enchant_slot_4_val": item.EnchantSlot4Val,
            "unknown_part":      item.UnknownPart,
            "unknown_part_val":  item.UnknownValue,
            "price":             item.Price,
            "quantity":          item.Quantity,
            "position":          item.Position,
        }
        shopItems = append(shopItems, apiItem)
    }

    data := map[string]interface{}{
        "vendor_id":    vendorID,
        "vendor_cid":   vendorCID,
        "flag":         flag,
        "expired_date": expiredDate,
        "shop_items":   shopItems,
        "PID":          v.ConnectionID,
        "timestamp":    v.Timestamp,
    }

    common.Log(common.LogPacket, common.LogVeryVerbose, "[%d] Sending vender items to API: vendor %d on map %s (%d items)", v.ConnectionID, vendorID, shopMap, len(items))
    common.SendToAPI("vending/items", data)

    return nil
}
