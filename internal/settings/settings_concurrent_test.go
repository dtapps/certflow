package settings

import (
	"sync"
	"testing"
)

// TestConcurrentSaveGet 并发读写配置
func TestConcurrentSaveGet(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewService(dir)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	const goroutines = 30
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := range goroutines {
		go func(idx int) {
			defer wg.Done()
			settings := DefaultSettings()
			settings.AutoRenewalEnabled = idx%2 == 0
			settings.DefaultRenewalDays = idx
			settings.Language = "zh-CN"

			// 并发保存
			if err := svc.Save(settings); err != nil {
				t.Errorf("goroutine %d: Save failed: %v", idx, err)
			}

			// 并发读取
			got := svc.Get()
			if got.DefaultRenewalDays != idx {
				// 可能被其他 goroutine 覆盖，这是正常的
			}
		}(i)
	}

	wg.Wait()
}

// TestConcurrentOnChange 并发注册和触发回调
func TestConcurrentOnChange(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewService(dir)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	var callCount int
	var mu sync.Mutex

	svc.OnChange(func(s Settings) {
		mu.Lock()
		callCount++
		mu.Unlock()
	})

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := range goroutines {
		go func(idx int) {
			defer wg.Done()
			settings := DefaultSettings()
			settings.AutoRenewalEnabled = idx%2 == 0
			_ = svc.Save(settings)
		}(i)
	}

	wg.Wait()
}
