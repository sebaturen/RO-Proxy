package receive

import (
	"roproxy/internal/common"
	"roproxy/internal/packets"
)

// ActorType represents the type of actor in the game
type ActorType uint8

const (
	ActorPacketMove8      = 0x09fd
	ActorPacketConnected8 = 0x09fe
	ActorPacketExists8    = 0x09ff
	ActorPacketInfo2      = 0x0a30
)

const (
	ActorTypePlayer  ActorType = 0x00
	ActorTypeNPC     ActorType = 0x01
	ActorTypeMonster ActorType = 0x05
)

// ActorInfo handles actor information packets (players, monsters, NPCs)
// Supports multiple packet types with different structures
type ActorInfo struct {
	packets.ParsedPacket

	// Common fields
	actorType   ActorType
	actorID     uint32
	characterID uint32
	name        string

	// Extended fields (ACTOR_MOVED_8, ACTOR_CONNECTED_8, ACTOR_EXISTS_8)
	typeID         uint16
	hairStyleID    uint16
	weaponID       uint32
	shieldID       uint32
	lowHeadID      uint16
	topHeadID      uint16
	midHeadID      uint16
	hairColorID    uint16
	clothesColorID uint16
	guildID        uint32
	guildEmblemID  uint32
	sex            uint8
	coordX         uint16
	coordY         uint16
	coordMap       string
	level          uint32
	clothesStyle   uint16

	// Minimal fields (ACTOR_INFO_2)
	partyName  string
	guildName  string
	guildTitle string
}

func (a *ActorInfo) Deserialize() error {
	opcode := a.Opcode

	// Extended version
	if opcode == ActorPacketMove8 || opcode == ActorPacketConnected8 || opcode == ActorPacketExists8 {
		a.deserializeExtended(opcode)
	}

	// Minimal version (party/guild info only)
	if opcode == ActorPacketInfo2 {
		a.deserializeMinimal()
	}

	return nil
}

func (a *ActorInfo) deserializeExtended(opcode uint16) {
	// Calculate offsets based on packet type
	var offsetOne, offsetTwo int8
	if opcode == ActorPacketConnected8 {
		offsetTwo = -1
	}
	if opcode == ActorPacketMove8 {
		offsetOne = 4
		offsetTwo = 6
	}

	payload := a.Payload

	// Read basic actor info
	a.actorType = ActorType(payload[0])
	a.actorID = common.ReadUint32LE(payload, 1)
	a.characterID = common.ReadUint32LE(payload, 5)

	// Read appearance data
	a.typeID = common.ReadUint16LE(payload, 19)
	a.hairStyleID = common.ReadUint16LE(payload, 21)
	a.weaponID = common.ReadUint32LE(payload, 23)
	a.shieldID = common.ReadUint32LE(payload, 27)
	a.lowHeadID = common.ReadUint16LE(payload, 31)
	a.topHeadID = common.ReadUint16LE(payload, 33+int(offsetOne))
	a.midHeadID = common.ReadUint16LE(payload, 35+int(offsetOne))
	a.hairColorID = common.ReadUint16LE(payload, 37+int(offsetOne))
	a.clothesColorID = common.ReadUint16LE(payload, 39+int(offsetOne))

	// Read guild info
	a.guildID = common.ReadUint32LE(payload, 45+int(offsetOne))
	a.guildEmblemID = common.ReadUint32LE(payload, 49+int(offsetOne))
	a.sex = payload[58+offsetOne]

	// Read coordinates (big endian, compressed)
	coords := uint32(payload[59+offsetOne])<<16 | uint32(payload[60+offsetOne])<<8 | uint32(payload[61+offsetOne])
	a.coordX = uint16(coords >> 14)
	a.coordY = uint16((coords >> 4) & 0x3FF)

	// Get map from connection
	mapName, hasMap := packets.GetConnectionMap(a.ConnectionID)
	if hasMap {
		a.coordMap = mapName
	} else {
		a.coordMap = "unknown"
	}

	// Read level and clothes style
	a.level = common.ReadUint32LE(payload, 65+int(offsetTwo))
	a.clothesStyle = common.ReadUint16LE(payload, 78+int(offsetTwo))

	// Read name (null-terminated string at end)
	nameStart := 80 + int(offsetTwo)
	if nameStart < len(payload) {
		a.name = common.ReadNullTerminatedString(payload, nameStart)
	}

	// Report based on actor type
	switch a.actorType {
	case ActorTypePlayer:
		a.reportPlayer()
	case ActorTypeMonster:
		a.reportMonster()
	}

}

// deserializeMinimal handles ACTOR_INFO_2 (party/guild info only)
func (a *ActorInfo) deserializeMinimal() {
	payload := a.Payload

	a.actorID = common.ReadUint32LE(payload, 0)
	a.name = common.ReadNullTerminatedString(payload, 4)
	a.partyName = common.ReadNullTerminatedString(payload, 28)
	a.guildName = common.ReadNullTerminatedString(payload, 52)
	a.guildTitle = common.ReadNullTerminatedString(payload, 76)

	a.reportPlayerMinimal()
}

func (a *ActorInfo) reportPlayer() {
	data := map[string]interface{}{
		"account_id":   a.actorID,
		"character_id": a.characterID,
		"info": map[string]interface{}{
			"job_id": a.typeID,
			"sex":    a.sex,
			"level":  a.level,
			"name":   common.StringToHex(a.name),
		},
		"customization": map[string]interface{}{
			"hair_style_id":    a.hairStyleID,
			"hair_color_id":    a.hairColorID,
			"weapon_id":        a.weaponID,
			"shield_id":        a.shieldID,
			"top_head_id":      a.topHeadID,
			"mid_head_id":      a.midHeadID,
			"low_head_id":      a.lowHeadID,
			"clothes_color_id": a.clothesColorID,
			"clothes_style":    a.clothesStyle,
		},
		"guild": map[string]interface{}{
			"id":        a.guildID,
			"emblem_id": a.guildEmblemID,
		},
		"location": map[string]interface{}{
			"coord_map": common.StringToHex(a.coordMap),
			"coord_x":   a.coordX,
			"coord_y":   a.coordY,
		},
	}

	common.Log(common.LogPacket, common.LogVeryVerbose, "[%d] Player detected: '%s' (JobID:%d, Lvl:%d) at %s (%d,%d)", a.ConnectionID, a.name, a.typeID, a.level, a.coordMap, a.coordX, a.coordY)
	packets.SendToAPI(&a.ParsedPacket, "character", data)
}

func (a *ActorInfo) reportMonster() {
	data := map[string]interface{}{
		"id":        a.typeID,
		"level":     a.level,
		"coord_x":   a.coordX,
		"coord_y":   a.coordY,
		"coord_map": common.StringToHex(a.coordMap),
	}

	common.Log(common.LogPacket, common.LogVeryVerbose, "[%d] Monster detected: TypeID:%d Lvl:%d at (%d,%d)", a.ConnectionID, a.typeID, a.level, a.coordX, a.coordY)
	common.Log(common.LogPacket, common.LogInfo, "Actor Monster info -> %s", data)
	// common.SendToAPI("monster", data)
	_ = data
}

func (a *ActorInfo) reportPlayerMinimal() {
	data := map[string]interface{}{
		"account_id":  a.actorID,
		"name":        common.StringToHex(a.name),
		"party":       common.StringToHex(a.partyName),
		"guild":       common.StringToHex(a.guildName),
		"guild_title": common.StringToHex(a.guildTitle),
	}

	common.Log(common.LogPacket, common.LogVeryVerbose, "[%d] Player party/guild info: '%s' (Party:'%s', Guild:'%s')", a.ConnectionID, a.name, a.partyName, a.guildName)
	packets.SendToAPI(&a.ParsedPacket, "character/party_guild", data)
}
