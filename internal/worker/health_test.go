package worker

import (
	"sync"
	"testing"
	"time"
)

func TestNewHealthMonitor(t *testing.T) {
	before := time.Now()
	hm := NewHealthMonitor()
	after := time.Now()

	if hm == nil {
		t.Fatal("NewHealthMonitor() returned nil")
	}
	if hm.startTime.Before(before) || hm.startTime.After(after) {
		t.Errorf("startTime = %v, want between %v and %v", hm.startTime, before, after)
	}
	if hm.activeBuilds != 0 {
		t.Errorf("activeBuilds = %d, want 0", hm.activeBuilds)
	}
}

func TestGetMetricsReturnsValidValues(t *testing.T) {
	hm := NewHealthMonitor()

	m := hm.GetMetrics()

	if m.Memory < 0 || m.Memory > 1 {
		t.Errorf("Memory = %f, want between 0 and 1", m.Memory)
	}
	if m.ActiveBuilds != 0 {
		t.Errorf("ActiveBuilds = %d, want 0", m.ActiveBuilds)
	}
	if m.Uptime < 0 {
		t.Errorf("Uptime = %v, want >= 0", m.Uptime)
	}
}

func TestGetMetricsUptimeIncreases(t *testing.T) {
	hm := NewHealthMonitor()

	m1 := hm.GetMetrics()
	time.Sleep(10 * time.Millisecond)
	m2 := hm.GetMetrics()

	if m2.Uptime <= m1.Uptime {
		t.Errorf("Uptime did not increase: m1=%v, m2=%v", m1.Uptime, m2.Uptime)
	}
}

func TestIncrementBuilds(t *testing.T) {
	hm := NewHealthMonitor()

	hm.IncrementBuilds()
	m := hm.GetMetrics()
	if m.ActiveBuilds != 1 {
		t.Errorf("ActiveBuilds after 1 increment = %d, want 1", m.ActiveBuilds)
	}

	hm.IncrementBuilds()
	m = hm.GetMetrics()
	if m.ActiveBuilds != 2 {
		t.Errorf("ActiveBuilds after 2 increments = %d, want 2", m.ActiveBuilds)
	}
}

func TestDecrementBuilds(t *testing.T) {
	hm := NewHealthMonitor()

	hm.IncrementBuilds()
	hm.IncrementBuilds()
	hm.DecrementBuilds()
	m := hm.GetMetrics()
	if m.ActiveBuilds != 1 {
		t.Errorf("ActiveBuilds = %d, want 1", m.ActiveBuilds)
	}

	hm.DecrementBuilds()
	m = hm.GetMetrics()
	if m.ActiveBuilds != 0 {
		t.Errorf("ActiveBuilds = %d, want 0", m.ActiveBuilds)
	}
}

func TestDecrementBelowZero(t *testing.T) {
	hm := NewHealthMonitor()

	hm.DecrementBuilds()
	m := hm.GetMetrics()
	if m.ActiveBuilds != -1 {
		t.Errorf("ActiveBuilds after decrement from 0 = %d, want -1", m.ActiveBuilds)
	}
}

func TestConcurrentAccess(t *testing.T) {
	hm := NewHealthMonitor()

	var wg sync.WaitGroup
	const goroutines = 100
	const iterations = 100

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				hm.IncrementBuilds()
				hm.GetMetrics()
				hm.DecrementBuilds()
			}
		}()
	}

	wg.Wait()

	m := hm.GetMetrics()
	if m.ActiveBuilds != 0 {
		t.Errorf("ActiveBuilds after concurrent ops = %d, want 0", m.ActiveBuilds)
	}
}

func TestConcurrentGetMetrics(t *testing.T) {
	hm := NewHealthMonitor()
	hm.IncrementBuilds()
	hm.IncrementBuilds()

	var wg sync.WaitGroup
	const readers = 50

	wg.Add(readers)
	for i := 0; i < readers; i++ {
		go func() {
			defer wg.Done()
			m := hm.GetMetrics()
			if m.ActiveBuilds != 2 {
				t.Errorf("ActiveBuilds = %d, want 2 during concurrent read", m.ActiveBuilds)
			}
		}()
	}

	wg.Wait()
}
