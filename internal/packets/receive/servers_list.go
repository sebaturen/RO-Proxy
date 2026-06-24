package receive

import (
	"fmt"
	"roproxy/internal/common"
	"roproxy/internal/packets"
	"strings"
)

type ServersList struct {
	packets.ParsedPacket
}

func (sl *ServersList) Deserialize() error {
	pktData := sl.Payload
	
	serverInfoSize := 165
	headerData := 56

	// accountID = common.ReadUint32LE(pktData, 4)
	var output strings.Builder
	output.WriteString("Server\tPlayers\tURL\n")
	for i := headerData; (i + serverInfoSize) <= len(pktData); i += serverInfoSize {
		serverName := common.ReadNullTerminatedString(pktData, i+10)
		currentPlayers := common.ReadUint32LE(pktData, i+30)
		urlConnection := common.ReadNullTerminatedString(pktData, i+36)
		fmt.Fprintf(&output, "%s\t%d\t%s\n", serverName, currentPlayers, urlConnection)
	}

	common.Log(common.LogMonitor, common.LogInfo, output.String())
	return nil
}