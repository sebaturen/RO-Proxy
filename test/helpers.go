package test

import (
	"fmt"
	"roproxy/internal/common"
	"roproxy/internal/packets"
)

// Deserializer interface for any packet that implements Deserialize
type Deserializer interface {
	Deserialize() map[string]any
}

// testPacketDeserialize is a generic helper function to test packet deserialization
func testPacketDeserialize(deserializer Deserializer) (map[string]any, error) {
	fmt.Printf("\n=== Testing Deserialize ===\n")

	data := deserializer.Deserialize()
	if data == nil {
		return nil, fmt.Errorf("Deserialize returned nil")
	}

	fmt.Printf("\n=== Test completed ===\n")
	fmt.Printf("Check the logs above for the parsed data\n\n")
	fmt.Printf("Output data %s\n", data)
	return data, nil
}

// createMockParsedPacket creates a mock ParsedPacket for testing
func createMockParsedPacket(hexString string) (packets.ParsedPacket, error) {
	payload, err := common.HexStringToBytes(hexString)
	if err != nil {
		return packets.ParsedPacket{}, fmt.Errorf("Failed to convert hex string to bytes: %v", err)
	}
	return packets.ParsedPacket{
		ConnectionID: 12345,
		Timestamp:    1234567890,
		Direction:    common.ServerToClient,
		Opcode:       0x000,
		Payload:      payload,
		Checksum:     nil,
		SourceIP:     "127.0.0.1",
		SourcePort:   6900,
	}, nil
}
