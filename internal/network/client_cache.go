package network

import (
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"cnb.cool/dtapp/certflow/internal/settings"
)

// ClientCache 按设置缓存 HTTP 客户端。
//
// 为什么需要它：BuildHTTPClient 每次调用都会新建 http.Transport，而 Transport
// 是连接池的载体——不复用就意味着每次请求都做完整 TCP+TLS 握手、无 keep-alive，
// 检查/扫描类高频路径会持续付出握手开销。本缓存让同一份设置（代理 + 自定义 DNS）
// 共享一个客户端及其连接池；设置变化时按指纹自动重建。
//
// Transport 本身可安全用于并发请求，多个 goroutine 共享同一客户端是标准用法。
type ClientCache struct {
	provider func() settings.Settings

	mu     sync.Mutex
	key    string
	client *http.Client
}

// NewClientCache 创建客户端缓存。provider 返回当前设置，可为 nil（Get 回退默认客户端）。
func NewClientCache(provider func() settings.Settings) *ClientCache {
	return &ClientCache{provider: provider}
}

// Get 返回当前设置对应的 HTTP 客户端。
// 设置未变时返回缓存的同一实例（连接池复用）；设置变化时重建。
func (c *ClientCache) Get() *http.Client {
	if c.provider == nil {
		return &http.Client{Timeout: 60 * time.Second}
	}
	s := c.provider()
	key := SettingsFingerprint(s)

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.client == nil || c.key != key {
		c.client = BuildHTTPClient(s)
		c.key = key
	}
	return c.client
}

// SettingsFingerprint 提取影响 BuildHTTPClient 构建结果的设置指纹：
// 代理全字段 + 所有启用的 DNS 服务器列表。与客户端构建无关的字段
// （日志、保留天数等）不参与指纹，避免其变化触发无意义的重建。
func SettingsFingerprint(s settings.Settings) string {
	var b strings.Builder
	p := s.Proxy
	// 代理：启用开关、协议、主机、端口、鉴权（密码参与指纹：变更后必须走新代理）
	b.WriteString("p=" + boolStr(p.Enabled) + "|" + p.Protocol + "|" + p.Host + "|" +
		strconv.Itoa(p.Port) + "|" + p.Username + "|" + p.Password)
	// DNS：仅启用项的服务器列表影响解析行为
	for _, d := range s.DNSConfigs {
		if !d.Enabled {
			continue
		}
		b.WriteString(";d=" + strings.Join(d.Servers, ","))
	}
	return b.String()
}

func boolStr(v bool) string {
	if v {
		return "1"
	}
	return "0"
}
