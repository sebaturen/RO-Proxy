package test

import (
	"roproxy/internal/packets/receive"
	"testing"
)

func TestCharEmblemUpdate1(t *testing.T) {
	hexString := "3E 0B 00 00 24 00 00 00 C7 01 00 00"
	data, err := testCharEmblemUpdateDeserialize(hexString)
	if err != nil {
		t.Fatalf("Failed to process System Chat for Twitch example")
	}

	guildId := 2878
	actorId := 455
	emblemId := 36
	if int(data["guild_id"].(uint32)) != guildId {
		t.Errorf("Guild expected '%d' guild received '%s'", guildId, data["guild_id"])
	}

	if int(data["actor_id"].(uint32)) != actorId {
		t.Errorf("Actor expected '%d' actor received '%s'", actorId, data["actor_id"])
	}

	if int(data["emblem_id"].(uint32)) != emblemId {
		t.Errorf("EmblemID expected '%d' actor received '%s'", emblemId, data["emblem_id"])
	}
}

func TestCharEmblemUpdate2(t *testing.T) {
	hexString := "CB 00 00 00 4A 00 00 00 C6 01 00 00"
	data, err := testCharEmblemUpdateDeserialize(hexString)
	if err != nil {
		t.Fatalf("Failed to process System Chat for Twitch example")
	}

	guildId := 203
	actorId := 454
	emblemId := 74
	if int(data["guild_id"].(uint32)) != guildId {
		t.Errorf("Guild expected '%d' guild received '%s'", guildId, data["guild_id"])
	}

	if int(data["actor_id"].(uint32)) != actorId {
		t.Errorf("Actor expected '%d' actor received '%s'", actorId, data["actor_id"])
	}

	if int(data["emblem_id"].(uint32)) != emblemId {
		t.Errorf("EmblemID expected '%d' actor received '%s'", emblemId, data["emblem_id"])
	}
}

func testCharEmblemUpdateDeserialize(hexString string) (map[string]any, error) {
	parsedPacket, err := createMockParsedPacket(hexString)
	if err != nil {
		return nil, err
	}

	emblemUpdate := receive.CharEmblemUpdate{
		ParsedPacket: parsedPacket,
	}

	return testPacketDeserialize(&emblemUpdate)
}