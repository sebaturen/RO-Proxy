package receive

import (
	"roproxy/internal/common"
	"roproxy/internal/packets"
	"strings"
)

const (
	ItemFromItem = 0
	ItemFromMonster = 1
)

type SpecialItemObtain struct {
	packets.ParsedPacket
	itemType byte
	receivedItemId uint32
	characterName string
	itemId uint32
	monsterCode string
}

func (sio *SpecialItemObtain) Deserialize() error {
	sio.itemType = sio.Payload[0]

	sio.receivedItemId = common.ReadUint32LE(sio.Payload, 1)
	sio.characterName = common.ReadNullTerminatedString(sio.Payload, 6)

	if sio.itemType == ItemFromItem {
		itemIdLocation := 6 + sio.Payload[5] + 1 // offset + name space reserverd + next space reserver len
		sio.itemId = common.ReadUint32LE(sio.Payload, int(itemIdLocation))
	} 
	if sio.itemType == ItemFromMonster {
		monsterNameLocation := 6 + sio.Payload[5] + 1 // offset + name space reserverd + next space reserver len
		monsterCode := common.ReadNullTerminatedString(sio.Payload, int(monsterNameLocation))
		sio.monsterCode = strings.ReplaceAll(monsterCode, "\x1C", "")
	}

	data := map[string]interface{}{
		"type": sio.itemId,
		"received_item_id": sio.receivedItemId,
		"from_item_id": sio.itemId,
		"from_monster_code": common.StringToHex(sio.monsterCode),
		"character_name": common.StringToHex(sio.characterName),
		"PID": sio.ConnectionID,
		"timestamp": sio.Timestamp,
	}

	common.Log(common.LogPacket, common.LogVeryVerbose, "Special Item received [%d] %s -> %s", sio.itemId, sio.characterName, sio.receivedItemId)
	common.SendToAPI("items/obtain/special", data)
	return nil
}