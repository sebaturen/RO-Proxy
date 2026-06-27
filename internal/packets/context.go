package packets

import (
	"roproxy/internal/common"
	"sync"
	"time"
)

var (
    pendingMap      = make([]pendingMapChange, 0)
    pendingMapMutex sync.RWMutex 
)

type pendingMapChange struct {
    charId    uint32
    connId    uint64
    mapName   string
    x         uint16
    y         uint16
    sourceIP  string
    destIP    string
    destPort  int
    timestamp time.Time
}

func SetConnectionMap(connID uint64, mapName string) {
    SetConnectionMapChar(0, connID, mapName)
}

func SetConnectionMapChar(charId uint32, connID uint64, mapName string) {
    pendingMapMutex.Lock()
    defer pendingMapMutex.Unlock()

    for i := range pendingMap {
        if pendingMap[i].connId == connID {
            pendingMap[i].charId = charId
            pendingMap[i].mapName = mapName
            return
        }
    }

    newLocation := pendingMapChange {
        charId:    charId,
        connId:    connID,
        mapName:   mapName,
        timestamp: time.Now(),
    }
    common.Log(common.LogProxy, common.LogVeryVerbose, "Set Connection map char -> %s", newLocation)
    pendingMap = append(pendingMap, newLocation)
}

func GetConnectionMap(connID uint64) (string, bool) {
    pendingMapMutex.Lock()
    defer pendingMapMutex.Unlock()

    for _, p := range pendingMap {
        if p.connId == connID {
            return p.mapName, true
        }
    }

    return "", false
}

func SetPendingMapChange(sourceIP string, mapName string, x, y uint16) {
    pendingMapMutex.Lock()
    defer pendingMapMutex.Unlock()

    for i := range pendingMap {
        if pendingMap[i].sourceIP == sourceIP && 
            pendingMap[i].mapName == mapName && 
            pendingMap[i].x == x && 
            pendingMap[i].y == y {
                return
        }
    }

    newLocation := pendingMapChange {
        sourceIP:  sourceIP,
        mapName:   mapName,
        x:         x,
        y:         y,
        timestamp: time.Now(),
    }
    pendingMap = append(pendingMap, newLocation)
}

func GetPendingMapChange(sourceIP string, x, y uint16) (string, bool) {
    pendingMapMutex.Lock()
    defer pendingMapMutex.Unlock()

    for _, p := range pendingMap {
        if p.sourceIP == sourceIP &&
            p.x == x &&
            p.y == y {
                return p.mapName, true
        }
    }

    return "", false
}

func SetPendingMapByDestination(destIP string, destPort int, mapName string) {
    pendingMapMutex.Lock()
    defer pendingMapMutex.Unlock()

    for i := range pendingMap {
        if pendingMap[i].destIP == destIP && 
            pendingMap[i].destPort == destPort && 
            pendingMap[i].mapName == mapName {
                return
        }
    }

    newLocation := pendingMapChange {
        destIP:   destIP,
        destPort: destPort,
        mapName:  mapName,
        timestamp: time.Now(),
    }
    pendingMap = append(pendingMap, newLocation)
}

func GetPendingMapByDestination(destIP string, destPort int) (string, bool) {
    pendingMapMutex.Lock()
    defer pendingMapMutex.Unlock()

    for _, p := range pendingMap {
        if p.destIP == destIP &&
            p.destPort == destPort {
                return p.mapName, true
        }
    }

    return "", false
}

func SetPendingMapByCharId(charId uint32, connID uint64) {
    pendingMapMutex.Lock()
    defer pendingMapMutex.Unlock()

    for i := range pendingMap {
        if pendingMap[i].charId == charId {
            pendingMap[i].connId = connID
            common.Log(common.LogPacket, common.LogVeryVerbose, "Seted char id %d for conn %d", charId, connID)
        }
    }
}

func ClearConnectionMap(connID uint64) {
    pendingMapMutex.Lock()
    defer pendingMapMutex.Unlock()
    
    index := -1
    for i, p := range pendingMap {
        if p.connId == connID {
            index = i
        }
    }

    if index >= 0 {
        common.Log(common.LogProxy, common.LogVeryVerbose, "Connection Map delete for %d conn id", connID)
        pendingMap = append(pendingMap[:index], pendingMap[index+1:]...)
    }
}