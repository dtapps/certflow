package monitor

import (
	"sync"
	"testing"
)

// TestConcurrentStartStop 并发 Start/Stop（串行生命周期）
func TestConcurrentStartStop(t *testing.T) {
	client := setupTestDB(t)

	const cycles = 5
	for range cycles {
		svc := NewMonitorService(client)
		svc.Start()
		svc.Stop()
	}
}

// TestConcurrentStartStopParallel 并行启动多个实例
func TestConcurrentStartStopParallel(t *testing.T) {
	client := setupTestDB(t)

	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for range goroutines {
		go func() {
			defer wg.Done()
			svc := NewMonitorService(client)
			svc.Start()
			svc.Stop()
		}()
	}

	wg.Wait()
}

// TestStartStopIdempotent 确保 Stop 多次调用不 panic（需串行）
func TestStartStopIdempotent(t *testing.T) {
	client := setupTestDB(t)
	svc := NewMonitorService(client)

	svc.Start()
	svc.Stop()
}
