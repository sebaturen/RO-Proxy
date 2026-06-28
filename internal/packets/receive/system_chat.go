package receive

import (
	"bytes"
	"roproxy/internal/common"
	"roproxy/internal/packets"
)

type SystemChat struct {
	packets.ParsedPacket
}

func (sc *SystemChat) Deserialize() map[string]any {
	pktData := sc.Payload
	var data map[string]any

	if pktData[0] == 0x1c { // multi params
		var params[]string
		startPosition := 1
		endPosition := startPosition

		for endPosition < len(pktData) {
			if pktData[endPosition] == 0x1c || pktData[endPosition] == 0x1d {
				paramBytes := pktData[startPosition:endPosition]
				param := string(paramBytes)

				params = append(params, common.StringToHex(param))
				startPosition = endPosition + 1
			}

			if pktData[endPosition] == 0x1c {
				break
			}
			endPosition++
		}

		data = map[string]any {
			"params": params,
		}
	} else {
		message := bytes.Split(pktData, []byte{0})
		var msgs []string

		for _, part := range message {
			if len(part) > 0 {
				msgs = append(msgs, common.HexToHexString(part))
			}
		}
		data = map[string]any{
			"messages": msgs,
		}
	}

	common.Log(common.LogPacket, common.LogVeryVerbose, "[%d] Send System chat %s", sc.ConnectionID, data)
	packets.SendToAPI(&sc.ParsedPacket, "system/chat", data)

	return data
}