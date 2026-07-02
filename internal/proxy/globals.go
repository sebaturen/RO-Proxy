package proxy

import (
    "bufio"
    "os"
    "sync"
    "time"

    "roproxy/internal/common"
    "roproxy/internal/ipc"
)

const (
    // RecordingFlagFile is checked by Proxy to determine recording state.
    // Analyzer creates/deletes this file to toggle recording.
    RecordingFlagFile = ".recording_enabled"
)

type Recording struct {
    status      bool
    file        *os.File
    writer      *bufio.Writer
    recordMutex sync.Mutex
}

var (
    globalRecording *Recording
    globalProxy     *Proxy
    globalIPCClient *ipc.Client
)

func GetRecording() *Recording {
    return globalRecording
}

// SetProxy sets the global proxy instance.
func SetProxy(p *Proxy) {
    globalProxy = p
}

// GetIPCClient returns the global IPC client instance.
func GetIPCClient() *ipc.Client {
    return globalIPCClient
}

// SetIPCClient sets the global IPC client instance.
func SetIPCClient(c *ipc.Client) {
    globalIPCClient = c
}

func IsRecording() bool {
    if globalRecording == nil {
        return false
    }
    return globalRecording.status
}

func SetRecording(enabled bool) {
    if globalRecording != nil && enabled != globalRecording.status {
        if enabled == false && globalRecording.file != nil {
            if globalRecording.writer != nil {
                globalRecording.writer.Flush()
            }
        }
    }
    if globalRecording == nil || enabled != globalRecording.status {
        globalRecording = &Recording{
            status: enabled,
            file:   nil,
            writer: nil,
        }
    }
    globalRecording.status = enabled
}

// StartRecordingFileWatcher monitors the recording flag file and updates state.
// This allows the Analyzer to control recording via file toggle.
func StartRecordingFileWatcher(stopChan <-chan struct{}) {
    ticker := time.NewTicker(500 * time.Millisecond)
    defer ticker.Stop()

    for {
        select {
        case <-stopChan:
            return
        case <-ticker.C:
            _, err := os.Stat(RecordingFlagFile)
            fileExists := err == nil

            currentStatus := IsRecording()
            if fileExists != currentStatus {
                SetRecording(fileExists)
                if fileExists {
                    common.Log(common.LogRecord, common.LogInfo, "Recording enabled via flag file")
                } else {
                    common.Log(common.LogRecord, common.LogInfo, "Recording disabled via flag file")
                }
            }
        }
    }
}
