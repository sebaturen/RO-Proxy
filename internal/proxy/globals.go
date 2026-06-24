package proxy

import (
    "os"
    "bufio"
    "sync"
)
type Recording struct {
    status      bool
    file        *os.File
    writer      *bufio.Writer
    recordMutex  sync.Mutex
}

var (
    globalRecording *Recording
    globalProxy     *Proxy
)

// GetProxy returns the global proxy instance.
func GetProxy() *Proxy {
    return globalProxy
}

func GetRecording() *Recording {
    return globalRecording
}

// SetProxy sets the global proxy instance.
func SetProxy(p *Proxy) {
    globalProxy = p
}

func IsRecording() bool {
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
            file: nil,
            writer: nil,
        }
    }
    globalRecording.status = enabled
}
