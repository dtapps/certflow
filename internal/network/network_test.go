package network

import (
	"net/http"
	"testing"

	"cnb.cool/dtapp/certflow/internal/settings"
)

func TestBuildHTTPClient_Default(t *testing.T) {
	s := settings.DefaultSettings()
	client := BuildHTTPClient(s)

	if client == nil {
		t.Fatal("expected non-nil http.Client")
	}
	if client.Timeout == 0 {
		t.Error("expected non-zero timeout")
	}
	if client.Transport == nil {
		t.Fatal("expected non-nil Transport")
	}

	// 验证 Transport 类型为 *http.Transport
	if _, ok := client.Transport.(*http.Transport); !ok {
		t.Error("expected Transport to be *http.Transport")
	}
}

func TestBuildHTTPClient_NoProxy(t *testing.T) {
	s := settings.DefaultSettings()
	s.Proxy.Enabled = false
	client := BuildHTTPClient(s)

	if client == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestBuildHTTPClient_WithProxy(t *testing.T) {
	s := settings.DefaultSettings()
	s.Proxy.Enabled = true
	s.Proxy.Host = "127.0.0.1"
	s.Proxy.Port = 8080
	s.Proxy.Protocol = "http"

	client := BuildHTTPClient(s)
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if client.Transport == nil {
		t.Fatal("expected non-nil transport")
	}

	// Transport 应已配置代理
	tr, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatal("expected *http.Transport")
	}
	if tr.Proxy == nil {
		t.Error("expected Proxy to be set when proxy is enabled")
	}
}

func TestBuildHTTPClient_ProxyWithAuth(t *testing.T) {
	s := settings.DefaultSettings()
	s.Proxy.Enabled = true
	s.Proxy.Host = "proxy.example.com"
	s.Proxy.Port = 3128
	s.Proxy.Protocol = "socks5"
	s.Proxy.Username = "user"
	s.Proxy.Password = "pass"

	client := BuildHTTPClient(s)
	if client == nil {
		t.Fatal("expected non-nil client")
	}

	tr, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatal("expected *http.Transport")
	}
	if tr.Proxy == nil {
		t.Error("expected Proxy to be set")
	}
}

func TestBuildHTTPClient_DisabledProxyWithHost(t *testing.T) {
	s := settings.DefaultSettings()
	s.Proxy.Enabled = false
	s.Proxy.Host = "127.0.0.1"
	s.Proxy.Port = 8080

	client := BuildHTTPClient(s)
	tr, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatal("expected *http.Transport")
	}
	if tr.Proxy != nil {
		t.Error("expected nil Proxy when proxy is disabled")
	}
}
