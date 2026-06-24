package proxy

import (
    "sync/atomic"
)

var (
    GlobalRecording atomic.Bool
    globalProxy     *Proxy
)

// GetProxy returns the global proxy instance.
func GetProxy() *Proxy {
    return globalProxy
}

// SetProxy sets the global proxy instance.
func SetProxy(p *Proxy) {
    globalProxy = p
}

func IsRecording() bool {
    return GlobalRecording.Load()
}

func SetRecording(enabled bool) {
    GlobalRecording.Store(enabled)
}
