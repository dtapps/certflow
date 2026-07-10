package logging

import (
	"sync"
	"testing"
)

// TestConcurrentLoggerWrite 并发写日志
func TestConcurrentLoggerWrite(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLogger(dir, DEBUG, 10, 3)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer l.Close()

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := range goroutines {
		go func(idx int) {
			defer wg.Done()
			l.Debug("goroutine %d: debug message", idx)
			l.Info("goroutine %d: info message", idx)
			l.Warn("goroutine %d: warn message", idx)
			l.Error("goroutine %d: error message", idx)
		}(i)
	}

	wg.Wait()
}

// TestConcurrentSetLevel 并发切换日志级别
func TestConcurrentSetLevel(t *testing.T) {
	dir := t.TempDir()
	l, err := NewLogger(dir, INFO, 10, 3)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer l.Close()

	const goroutines = 30
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := range goroutines {
		go func(idx int) {
			defer wg.Done()
			levels := []Level{DEBUG, INFO, WARN, ERROR}
			l.SetLevel(levels[idx%len(levels)])
			_ = l.GetLevel()
			l.Info("message from goroutine %d", idx)
		}(i)
	}

	wg.Wait()
}

// TestConcurrentGlobalLogger 并发使用全局日志
func TestConcurrentGlobalLogger(t *testing.T) {
	dir := t.TempDir()
	if err := InitGlobalLogger(dir, "INFO", 10, 3); err != nil {
		t.Fatalf("InitGlobalLogger: %v", err)
	}

	const goroutines = 30
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := range goroutines {
		go func(idx int) {
			defer wg.Done()
			Debug("global debug %d", idx)
			Info("global info %d", idx)
			Warn("global warn %d", idx)
			Error("global error %d", idx)
		}(i)
	}

	wg.Wait()
}
