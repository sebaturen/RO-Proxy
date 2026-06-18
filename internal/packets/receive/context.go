package receive

import (
    "sync"
)

var (
    connectionMaps      = make(map[uint64]string)
    connectionMapsMutex sync.RWMutex
)

func SetConnectionMap(connID uint64, mapName string) {
    connectionMapsMutex.Lock()
    defer connectionMapsMutex.Unlock()
    connectionMaps[connID] = mapName
}

func GetConnectionMap(connID uint64) (string, bool) {
    connectionMapsMutex.RLock()
    defer connectionMapsMutex.RUnlock()
    mapName, exists := connectionMaps[connID]
    return mapName, exists
}

func ClearConnectionMap(connID uint64) {
    connectionMapsMutex.Lock()
    defer connectionMapsMutex.Unlock()
    delete(connectionMaps, connID)
}
