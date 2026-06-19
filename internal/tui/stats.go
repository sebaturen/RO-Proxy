package tui

import (
	"sync"
	"time"

	"roproxy/internal/common"
)

type StatsData struct {
	StartTime           time.Time
	TotalPackets        uint64
	ClientToServer      uint64
	ServerToClient      uint64
	UnknownPackets      uint64
	BytesClientToServer uint64
	BytesServerToClient uint64
}

type Stats struct {
	data  StatsData
	mutex sync.RWMutex
}

func NewStats() *Stats {
	return &Stats{
		data: StatsData{
			StartTime: time.Now(),
		},
	}
}

func (s *Stats) AddPacket(direction common.PacketDirection, size int, unknown bool) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	
	s.data.TotalPackets++
	
	if direction == common.ClientToServer {
		s.data.ClientToServer++
		s.data.BytesClientToServer += uint64(size)
	} else {
		s.data.ServerToClient++
		s.data.BytesServerToClient += uint64(size)
	}
	
	if unknown {
		s.data.UnknownPackets++
	}
}

func (s *Stats) Get() StatsData {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.data
}
