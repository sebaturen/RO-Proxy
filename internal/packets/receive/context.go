package receive

import (
    "fmt"
    "sync"
    "time"
)

var (
    connectionMaps      = make(map[uint64]string)
    connectionMapsMutex sync.RWMutex
    
    pendingMapChanges      = make(map[string]*PendingMapChange)
    pendingMapChangesMutex sync.RWMutex
    
    pendingMapByDestination      = make(map[string]*PendingMapChange)
    pendingMapByDestinationMutex sync.RWMutex
)

type PendingMapChange struct {
    MapName   string
    X         uint16
    Y         uint16
    Timestamp time.Time
}

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

func SetPendingMapChange(sourceIP string, mapName string, x, y uint16) {
    pendingMapChangesMutex.Lock()
    defer pendingMapChangesMutex.Unlock()
    
    pendingMapChanges[sourceIP] = &PendingMapChange{
        MapName:   mapName,
        X:         x,
        Y:         y,
        Timestamp: time.Now(),
    }
}

func GetPendingMapChange(sourceIP string, x, y uint16) (string, bool) {
    pendingMapChangesMutex.Lock()
    defer pendingMapChangesMutex.Unlock()
    
    pending, exists := pendingMapChanges[sourceIP]
    if !exists {
        return "", false
    }
    
    if time.Since(pending.Timestamp) > 5*time.Second {
        delete(pendingMapChanges, sourceIP)
        return "", false
    }
    
    if pending.X == x && pending.Y == y {
        mapName := pending.MapName
        delete(pendingMapChanges, sourceIP)
        return mapName, true
    }
    
    return "", false
}

func SetPendingMapByDestination(destIP string, destPort int, mapName string) {
    pendingMapByDestinationMutex.Lock()
    defer pendingMapByDestinationMutex.Unlock()
    
    key := fmt.Sprintf("%s:%d", destIP, destPort)
    pendingMapByDestination[key] = &PendingMapChange{
        MapName:   mapName,
        Timestamp: time.Now(),
    }
}

func GetPendingMapByDestination(destIP string, destPort int) (string, bool) {
    pendingMapByDestinationMutex.Lock()
    defer pendingMapByDestinationMutex.Unlock()
    
    key := fmt.Sprintf("%s:%d", destIP, destPort)
    pending, exists := pendingMapByDestination[key]
    if !exists {
        return "", false
    }
    
    if time.Since(pending.Timestamp) > 10*time.Second {
        delete(pendingMapByDestination, key)
        return "", false
    }
    
    mapName := pending.MapName
    delete(pendingMapByDestination, key)
    return mapName, true
}
