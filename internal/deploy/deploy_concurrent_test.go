package deploy

import (
	"encoding/json"
	"sync"
	"testing"

	"cnb.cool/dtapp/certflow/internal/config"
	"cnb.cool/dtapp/certflow/internal/deploycredential"
)

// TestConcurrentCachePutGet 并发读写部署上传缓存（uploadCache 由 uploadMu 保护）
func TestConcurrentCachePutGet(t *testing.T) {
	s := NewDeployService(nil)
	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := range goroutines {
		go func(idx int) {
			defer wg.Done()
			key := "key-" + string(rune('A'+idx))
			s.cachePut(key, "val")
			if got, ok := s.cacheGet(key); !ok || got != "val" {
				t.Errorf("cacheGet(%q) = %q,%v; want val,true", key, got, ok)
			}
		}(i)
	}
	wg.Wait()

	// 验证所有 key 都已落盘到缓存中
	for i := range goroutines {
		key := "key-" + string(rune('A'+i))
		if got, ok := s.cacheGet(key); !ok || got != "val" {
			t.Errorf("after wait cacheGet(%q) = %q,%v; want val,true", key, got, ok)
		}
	}
}

// TestConcurrentCacheStress 多 goroutine 对重叠 key 高频 put/get，验证互斥锁无竞态
func TestConcurrentCacheStress(t *testing.T) {
	s := NewDeployService(nil)
	const goroutines = 20
	const ops = 200
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := range goroutines {
		go func(idx int) {
			defer wg.Done()
			for j := range ops {
				k := "shared-" + string(rune('0'+j%10))
				s.cachePut(k, "x")
				s.cacheGet(k)
			}
		}(i)
	}
	wg.Wait()
}

// TestConcurrentPureHelpers 并发调用纯函数（各自持有独立的局部 config，无共享可变状态）
func TestConcurrentPureHelpers(t *testing.T) {
	const goroutines = 30
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := range goroutines {
		go func(idx int) {
			defer wg.Done()
			cfg := map[string]string{
				"region":            "cn-hangzhou",
				"access_key_id":     "ak",
				"access_key_secret": "sk",
				"domain":            "x.com",
			}
			if RegionFromConfig(cfg) != "cn-hangzhou" {
				t.Errorf("RegionFromConfig mismatch")
			}
			raw, _ := json.Marshal(cfg)
			c, err := deploycredential.Parse("aliyun", raw)
			if err != nil || c.AccessKeyID != "ak" || c.AccessKeySecret != "sk" {
				t.Errorf("deploycredential.Parse mismatch")
			}
			s, _ := config.StripSecrets[deploycredential.AliyunDeployCred](raw)
			var stripped map[string]string
			json.Unmarshal(s, &stripped)
			if _, ok := stripped["access_key_id"]; ok {
				t.Errorf("StripSecrets leaked access_key_id")
			}
			if stripped["domain"] != "x.com" {
				t.Errorf("StripSecrets dropped domain")
			}
			if got := parseConfig([]byte(`{"a":"b"}`)); got["a"] != "b" {
				t.Errorf("parseConfig mismatch")
			}
			if got := certName("d.com", map[string]string{}); got != "d.com" {
				t.Errorf("certName default mismatch")
			}
		}(i)
	}
	wg.Wait()
}
