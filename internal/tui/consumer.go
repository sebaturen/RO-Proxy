package tui

import (
    "reflect"
    "strings"

    "roproxy/internal/common"
    "roproxy/internal/packets"
    "roproxy/internal/packets/receive"
    "roproxy/internal/packets/send"
)

// StartUIConsumer starts the global UI consumer goroutine.
// This goroutine reads from GlobalUIQueue and processes packets for display.
// It replaces the legacy PacketProcessor per-connection approach.
func StartUIConsumer(dashboard *Dashboard, captureSettings CaptureSettings) {
    go uiConsumerLoop(dashboard, captureSettings)
    go logConsumerLoop(dashboard)
}

// logConsumerLoop reads from GlobalLogQueue and displays logs in UI.
// This allows any component to log to UI without needing a dashboard reference.
func logConsumerLoop(dashboard *Dashboard) {
    for msg := range common.GlobalLogQueue {
        // Filter verbose logs - only show in VERY VERBOSE mode
        if strings.Contains(msg, "[DEBUG") || 
           strings.Contains(msg, "[DRAIN") || 
           strings.Contains(msg, "[RECORD") || 
           strings.Contains(msg, "[MONITOR") {
            if dashboard.debugMode != DebugVeryVerbose {
                continue // Skip verbose logs unless in very verbose mode
            }
        }
        dashboard.Log(msg)
    }
}

// CaptureSettings interface matches the one in packets package
type CaptureSettings interface {
    GetCaptureServer() bool
    GetCaptureClient() bool
}

func uiConsumerLoop(dashboard *Dashboard, captureSettings CaptureSettings) {
    for pktInterface := range common.GlobalUIQueue {
        // Cast from interface{} to *packets.ParsedPacket
        pkt, ok := pktInterface.(*packets.ParsedPacket)
        if !ok {
            continue // Invalid type, skip
        }
        processPacketForUI(pkt, dashboard, captureSettings)
    }
}

func processPacketForUI(pkt *packets.ParsedPacket, dashboard *Dashboard, captureSettings CaptureSettings) {
    // Filter by direction based on capture settings
    if pkt.Direction == common.ServerToClient && !captureSettings.GetCaptureServer() {
        return
    }
    if pkt.Direction == common.ClientToServer && !captureSettings.GetCaptureClient() {
        return
    }

    // Look up packet spec
    var spec *common.PacketSpec
    if pkt.Direction == common.ServerToClient {
        spec = receive.PacketDatabase[pkt.Opcode]
    } else {
        spec = send.PacketDatabase[pkt.Opcode]
    }

    if spec == nil {
        // Unknown packet
        if dashboard.IsShowWarnings() {
            dashboard.LogUnknown(pkt.ConnectionID, pkt.Direction, pkt.Opcode, len(pkt.Payload), pkt.Payload, pkt.Checksum)
        }
        return
    }

    // Known packet - log to UI if debug mode is on
    desc := spec.Desc
    if desc == "" {
        desc = "Unknown"
    }

    if dashboard.IsDebugMode() {
        dashboard.LogPacket(pkt.ConnectionID, pkt.Direction, pkt.Opcode, len(pkt.Payload), desc, pkt.Payload, pkt.Checksum)
    }

    // Call deserializer handler if exists
    if spec.Handler != nil {
        handlerValue := reflect.ValueOf(spec.Handler)
        if handlerValue.Kind() == reflect.Ptr {
            handlerValue = handlerValue.Elem()
        }

        baseField := handlerValue.FieldByName("BaseDeserializer")
        if baseField.IsValid() && baseField.CanSet() {
            baseField.Set(reflect.ValueOf(common.BaseDeserializer{
                ConnID:     pkt.ConnectionID,
                Timestamp:  pkt.Timestamp.Unix(),
                Payload:    pkt.Payload,
                SourceIP:   pkt.SourceIP,
                SourcePort: pkt.SourcePort,
                DestIP:     pkt.DestIP,
                DestPort:   pkt.DestPort,
            }))
        }

        spec.Handler.Deserialize()
    }
}
