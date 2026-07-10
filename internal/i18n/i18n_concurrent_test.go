package i18n

import (
	"sync"
	"testing"
)

// TestConcurrentSetLocale 并发切换语言环境
func TestConcurrentSetLocale(t *testing.T) {
	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := range goroutines {
		go func(idx int) {
			defer wg.Done()
			if idx%2 == 0 {
				SetLocale("en-US")
			} else {
				SetLocale("zh-CN")
			}
			locale := GetLocale()
			if locale != "en-US" && locale != "zh-CN" {
				t.Errorf("GetLocale() = %q, unexpected value", locale)
			}
		}(i)
	}

	wg.Wait()
}

// TestConcurrentT 并发翻译
func TestConcurrentT(t *testing.T) {
	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)

	keys := []string{
		"error.ca_name_required",
		"error.password_too_short",
		"app.description",
	}

	for i := range goroutines {
		go func(idx int) {
			defer wg.Done()
			key := keys[idx%len(keys)]
			result := T(key)
			if result == "" {
				t.Errorf("T(%q) returned empty string", key)
			}
		}(i)
	}

	wg.Wait()
}

// TestConcurrentTWithLocale 并发指定语言翻译
func TestConcurrentTWithLocale(t *testing.T) {
	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := range goroutines {
		go func(idx int) {
			defer wg.Done()
			if idx%2 == 0 {
				result := TWithLocale("en-US", "error.ca_name_required")
				_ = result
			} else {
				result := TWithLocale("zh-CN", "error.ca_name_required")
				_ = result
			}
		}(i)
	}

	wg.Wait()

	// 验证 locale 恢复正确
	locale := GetLocale()
	if locale != "zh-CN" && locale != "en-US" {
		t.Errorf("locale not restored: %q", locale)
	}
}

// TestConcurrentSetLocaleAndT 并发切换语言+翻译混合
func TestConcurrentSetLocaleAndT(t *testing.T) {
	const goroutines = 30
	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	// 一半 goroutine 切换 locale
	for i := range goroutines {
		go func(idx int) {
			defer wg.Done()
			if idx%2 == 0 {
				SetLocale("en-US")
			} else {
				SetLocale("zh-CN")
			}
		}(i)
	}

	// 另一半做翻译
	for i := range goroutines {
		go func(idx int) {
			defer wg.Done()
			_ = T("error.ca_name_required")
			_ = TWithLocale("en-US", "error.password_too_short")
			_ = GetLocale()
			_ = GetCurrentLocale()
		}(i)
	}

	wg.Wait()
}
