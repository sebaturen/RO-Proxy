package test

import (
	"roproxy/internal/packets/receive"
	"testing"
)

func TestSystemChatMaintenace(t *testing.T) {
	hexString := "4D 61 69 6E 74 65 6E 61 6E 63 65 20 4E 6F 74 69 63 65 20 00"
	data, err := testSystemChatDeserialize(hexString)
	if err != nil {
		t.Fatalf("Failed to process System Chat for Twitch example")
	}

	expected := []string{"Maintenance Notice "}
	rData := data["messages"].([]string)

	if len(rData) != len(expected) {
		t.Fatalf("Expected len (%d) and data (%d) its diferent", len(rData), len(expected))
	}

	for i, expVal := range expected {
		evalData, err := HexStringToString(rData[i])
		if err != nil {
			t.Errorf("Fail to parse hex to string in expected data '%s' error: '%s'", rData[i], err)
			continue
		}
		if evalData != expVal {
			t.Errorf("Mismatch element [%d] expected: '%s', got '%s'", i, expVal, evalData)
			continue
		}
		t.Logf("Success '%s' == '%s'", evalData, expVal)
	}
}

func TestSystemChatTwitch(t *testing.T) {
	hexString := "6D 69 63 63 5A 61 6E 6F 6E 70 6C 61 79 73 00 61 6E 74 33 00 00 20 43 41 45 00 00 00 46 46 46 46 30 30 1C 51 67 34 44 1D 5A 61 6E 6F 6E 70 6C 61 79 73 1D 53 4F 52 54 45 49 4F 20 4E 41 20 4C 49 56 45 20 48 4F 4A 45 21 20 5A 61 6E 6F 6E 20 6E 61 20 74 77 69 74 63 68 20 5A 61 6E 6F 6E 2D 70 6C 61 79 73 20 59 54 1C"
	data, err := testSystemChatDeserialize(hexString)
	if err != nil {
		t.Fatalf("Failed to process System Chat for Twitch example")
	}

	expected := []string{"miccZanonplays", "ant3", " CAE", "FFFF00Qg4DZanonplaysSORTEIO NA LIVE HOJE! Zanon na twitch Zanon-plays YT"}
	rData := data["messages"].([]string)

	if len(rData) != len(expected) {
		t.Fatalf("Expected len (%d) and data (%d) its diferent", len(expected), len(rData))
	}

	for i, expVal := range expected {
		evalData, err := HexStringToString(rData[i])
		if err != nil {
			t.Errorf("Fail to parse hex to string in expected data '%s' error: '%s'", rData[i], err)
			continue
		}
		if evalData != expVal {
			t.Errorf("Mismatch element [%d] expected: '%s', got '%s'", i, expVal, evalData)
			continue
		}
		t.Logf("Success '%s' == '%s'", evalData, expVal)
	}
}

func TestSystemChatWoE(t *testing.T) {
	hexString := "1C 53 37 30 67 41 67 1D 54 52 49 42 55 4E 41 4C 45 1D 4B 72 69 65 6D 68 69 6C 64 1C 00"
	data, err := testSystemChatDeserialize(hexString)
	if err != nil {
		t.Fatalf("Failed to process System Chat for Twitch example")
	}

	expected := []string{"S70gAg", "TRIBUNALE", "Kriemhild"}
	rData := data["params"].([]string)

	if len(rData) != len(expected) {
		t.Fatalf("Expected and data its diferent")
	}

	for i, expVal := range expected {
		evalData, err := HexStringToString(rData[i])
		if err != nil {
			t.Errorf("Fail to parse hex to string in expected data '%s' error: '%s'", rData[i], err)
			continue
		}
		if evalData != expVal {
			t.Errorf("Mismatch element [%d] expected: '%s', got '%s'", i, expVal, evalData)
			continue
		}
		t.Logf("Success '%s' == '%s'", evalData, expVal)
	}
}

func testSystemChatDeserialize(hexString string) (map[string]any, error) {
	parsedPacket, err := createMockParsedPacket(hexString)
	if err != nil {
		return nil, err
	}

	systemChat := receive.SystemChat{
		ParsedPacket: parsedPacket,
	}

	return testPacketDeserialize(&systemChat)
}