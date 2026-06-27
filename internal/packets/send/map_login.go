package send

import (
	"roproxy/internal/common"
	"roproxy/internal/packets"
)

type MapLogin struct {
	packets.ParsedPacket
}

func (ml *MapLogin) Deserialize() error {
	// accountId := common.ReadUint32LE(ml.Payload, 0)
	characterId := common.ReadUint32LE(ml.Payload, 4)

	packets.SetPendingMapByCharId(characterId, ml.ConnectionID)
	return nil
}