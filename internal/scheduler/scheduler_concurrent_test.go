package scheduler

import (
	"sync"
	"testing"
)

// TestConcurrentSchedulerLifecycle 并发创建和销毁调度器
func TestConcurrentSchedulerLifecycle(t *testing.T) {
	client := setupTestDB(t)

	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for range goroutines {
		go func() {
			defer wg.Done()
			s := NewScheduler(client, nil, nil, nil, "/tmp/certs")
			if s == nil {
				t.Error("NewScheduler returned nil")
			}
		}()
	}

	wg.Wait()
}

// TestNewSchedulerConcurrent 并发创建多个调度器
func TestNewSchedulerConcurrent(t *testing.T) {
	client := setupTestDB(t)

	const goroutines = 20
	schedulers := make([]*Scheduler, goroutines)
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := range goroutines {
		go func(idx int) {
			defer wg.Done()
			schedulers[idx] = NewScheduler(client, nil, nil, nil, "/tmp/certs")
		}(i)
	}

	wg.Wait()

	for i, s := range schedulers {
		if s == nil {
			t.Errorf("scheduler[%d] is nil", i)
		}
	}
}
