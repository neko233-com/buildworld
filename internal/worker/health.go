package worker

import (
	"runtime"
	"sync"
	"time"
)

type Metrics struct {
	CPU          float32
	Memory       float32
	Disk         float32
	ActiveBuilds int
	Uptime       time.Duration
}

type HealthMonitor struct {
	mu           sync.RWMutex
	startTime    time.Time
	activeBuilds int
}

func NewHealthMonitor() *HealthMonitor {
	return &HealthMonitor{
		startTime: time.Now(),
	}
}

func (h *HealthMonitor) GetMetrics() Metrics {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return Metrics{
		CPU:          0,
		Memory:       float32(m.Alloc) / float32(m.Sys),
		Disk:         0,
		ActiveBuilds: h.activeBuilds,
		Uptime:       time.Since(h.startTime),
	}
}

func (h *HealthMonitor) IncrementBuilds() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.activeBuilds++
}

func (h *HealthMonitor) DecrementBuilds() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.activeBuilds--
}
