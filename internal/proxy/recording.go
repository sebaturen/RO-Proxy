package proxy

import (
    "bufio"
    "fmt"
    "os"
    "time"

    "roproxy/internal/common"
    "roproxy/internal/packets"
)

// recordRawChunkToFile writes raw chunk to recording file (for reverse engineering).
func recordRawChunkToFile(connectionID uint64, chunk *packets.RawChunk) {
    r := GetRecording()
    r.recordMutex.Lock()
    defer r.recordMutex.Unlock()

    // Lazy init: create file on first chunk if recording is active
    if r.file == nil {
        if err := createRecordFile(r); err != nil {
            common.Log(common.LogRecord, common.LogError, "Connection #%d failed to create recording file: %v", connectionID, err)
            return
        }
    }

    dirStr := common.FormatDirection(chunk.Direction)
    hexData := common.FormatPayload(chunk.Data, false)
    line := fmt.Sprintf("%d;%d;%s;%d;%s\n", chunk.Timestamp, connectionID, dirStr, len(chunk.Data), hexData)

    r.writer.WriteString(line)
}

func createRecordFile(r *Recording) error {
    // Ensure recordings directory exists
    if err := os.MkdirAll("recordings", 0755); err != nil {
        return fmt.Errorf("failed to create recordings directory: %w", err)
    }
    
    timestamp := time.Now().Format("20060102_150405")
    filename := fmt.Sprintf("recordings/%s.txt", timestamp)
    
    file, err := os.Create(filename)
    if err != nil {
        return fmt.Errorf("failed to create file: %w", err)
    }
    
    r.file = file
    r.writer = bufio.NewWriter(file)
    
    common.Log(common.LogRecord, common.LogInfo, "Started recording: %s", filename)
    return nil
}
