package receive

import (
	"roproxy/internal/common"
	"roproxy/internal/packets"
)

type CharEmblemUpdate struct {
	packets.ParsedPacket
}

func (ceu *CharEmblemUpdate) Deserialize() map[string]any {
	pktData := ceu.Payload

	guildId := common.ReadUint32LE(pktData, 0)
	emblemId := common.ReadUint32LE(pktData, 4)
	actorId := common.ReadUint32LE(pktData, 8)

	data := map[string]any {
		"guild_id": guildId,
		"emblem_id": emblemId,
		"actor_id": actorId,
	}

	common.Log(common.LogPacket, common.LogVeryVerbose, "[%d] Char emblem update, GuildID %d, ActorId: %d", ceu.ConnectionID, guildId, actorId)
	packets.SendToAPI(&ceu.ParsedPacket, "character/update_emblem", data)
	return data
}