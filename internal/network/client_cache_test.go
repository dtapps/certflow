package network

import (
	"sync"
	"testing"

	"cnb.cool/dtapp/certflow/internal/settings"
)

func TestClientCache_ReusesClientWhenSettingsUnchanged(t *testing.T) {
	s := settings.DefaultSettings()
	cache := NewClientCache(func() settings.Settings { return s })

	first := cache.Get()
	if first == nil {
		t.Fatal("expected non-nil client")
	}
	// 设置未变：必须返回同一实例（连接池复用的前提）
	if again := cache.Get(); again != first {
		t.Error("expected the same cached client when settings unchanged")
	}
}

func TestClientCache_RebuildsWhenProxyChanges(t *testing.T) {
	s := settings.DefaultSettings()
	cache := NewClientCache(func() settings.Settings { return s })

	first := cache.Get()

	s.Proxy.Enabled = true
	s.Proxy.Host = "127.0.0.1"
	s.Proxy.Port = 8080
	s.Proxy.Protocol = "http"

	second := cache.Get()
	if second == first {
		t.Error("expected a rebuilt client after proxy settings change")
	}
}

func TestClientCache_RebuildsWhenDNSChanges(t *testing.T) {
	s := settings.DefaultSettings()
	cache := NewClientCache(func() settings.Settings { return s })

	first := cache.Get()

	// 启用一个自定义 DNS：解析行为变化，必须重建客户端
	s.DNSConfigs = append(s.DNSConfigs, settings.DNSConfig{
		ID:      "aliyun",
		Name:    "阿里云 DNS",
		Enabled: true,
		Servers: []string{"223.5.5.5"},
	})

	if second := cache.Get(); second == first {
		t.Error("expected a rebuilt client after DNS settings change")
	}
}

func TestClientCache_IgnoresIrrelevantSettings(t *testing.T) {
	s := settings.DefaultSettings()
	cache := NewClientCache(func() settings.Settings { return s })

	first := cache.Get()

	// 与客户端构建无关的字段变化不应触发重建
	s.MonitorHistoryDays = 7
	s.Language = "en-US"

	if second := cache.Get(); second != first {
		t.Error("expected the same client when only unrelated settings changed")
	}
}

func TestClientCache_NilProvider(t *testing.T) {
	cache := NewClientCache(nil)
	client := cache.Get()
	if client == nil {
		t.Fatal("expected non-nil fallback client")
	}
	if client.Timeout == 0 {
		t.Error("expected non-zero timeout on fallback client")
	}
}

func TestClientCache_ConcurrentGet(t *testing.T) {
	s := settings.DefaultSettings()
	cache := NewClientCache(func() settings.Settings { return s })

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)
	clients := make([]any, goroutines)
	for i := range goroutines {
		go func(i int) {
			defer wg.Done()
			clients[i] = cache.Get()
		}(i)
	}
	wg.Wait()

	// 并发 Get 必须全部返回同一实例（无竞态导致的重复构建）
	for i := 1; i < goroutines; i++ {
		if clients[i] != clients[0] {
			t.Fatalf("concurrent Get returned different clients at index %d", i)
		}
	}
}

func TestSettingsFingerprint_DistinguishesProxyCredentials(t *testing.T) {
	base := settings.DefaultSettings()

	withPass := base
	withPass.Proxy.Enabled = true
	withPass.Proxy.Host = "127.0.0.1"
	withPass.Proxy.Port = 8080
	withPass.Proxy.Username = "user"
	withPass.Proxy.Password = "secret"

	otherPass := withPass
	otherPass.Proxy.Password = "different"

	if SettingsFingerprint(withPass) == SettingsFingerprint(otherPass) {
		t.Error("expected different fingerprints for different proxy passwords")
	}
}
