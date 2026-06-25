package receive

import (
	"roproxy/internal/common"
	"roproxy/internal/packets"
)

type UpgradeItemBroadcast struct {
	packets.ParsedPacket
}

func (ui *UpgradeItemBroadcast) Deserialize() error {
	characterName := common.ReadNullTerminatedString(ui.Payload, 0)
	itemId := common.ReadUint32LE(ui.Payload, 24)
	level := common.ReadUint16LE(ui.Payload, 28)
	unknownVal := common.ReadUint16LE(ui.Payload, 30)

	data := map[string]interface{}{
		"character_name": common.StringToHex(characterName),
		"item_id": itemId,
		"level": level,
		"unknown_val": unknownVal,
		"PID": ui.ConnectionID,
		"timestamp": ui.Timestamp,
	}

	common.Log(common.LogPacket, common.LogVeryVerbose, "Upgrade Item [%s] %d -> %d", characterName, itemId, level)
	common.SendToAPI("items/obtain/upgrade", data)
	return nil
}