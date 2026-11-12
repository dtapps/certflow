package network

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"cnb.cool/dtapp/certflow/internal/httplog"
	"cnb.cool/dtapp/certflow/internal/settings"
	"cnb.cool/dtapp/certflow/internal/useragent"
)

// unwrapHTTPTransport 逐层解包被 useragent.Wrap（httplog.WrapTransport 最外层）
// 包裹的 RoundTripper，还原出底层 *http.Transport。BuildHTTPClient 借此在包裹层
// 之外设置代理；测试也用它断言底层 Transport 类型。仅解包 *useragent.Transport
// 一层（测试/非 DEBUG 环境足矣）；DEBUG 下多包的 LoggingRoundTripper 不在此处理。
func unwrapHTTPTransport(rt http.RoundTripper) (*http.Transport, bool) {
	for rt != nil {
		if t, ok := rt.(*http.Transport); ok {
			return t, true
		}
		if t, ok := rt.(*useragent.Transport); ok {
			rt = t.Base
			continue
		}
		return nil, false
	}
	return nil, false
}

// BuildHTTPClient 根据设置构建带有自定义 DNS 和代理的 HTTP 客户端
func BuildHTTPClient(s settings.Settings) *http.Client {
	return &http.Client{
		Transport: buildTransport(s),
		Timeout:   60 * time.Second,
	}
}

// BuildDownloadHTTPClient 构建用于大文件下载的 HTTP 客户端。
// 不设置 http.Client.Timeout：该超时覆盖整个响应体读取，更新包动辄数十 MB，
// 网络稍慢就会被中途掐断（表现为 context deadline exceeded while reading body）。
// 这里只保留连接/响应头超时，整体取消由调用方传入的 context 控制。
func BuildDownloadHTTPClient(s settings.Settings) *http.Client {
	return &http.Client{
		Transport: buildTransport(s),
	}
}

// buildTransport 构建带自定义 DNS 与代理的 RoundTripper（供普通请求与下载共用）
func buildTransport(s settings.Settings) http.RoundTripper {
	transport := httplog.WrapTransport(&http.Transport{
		// 自定义 DNS 解析
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}

			// 使用自定义 DNS 解析
			ips, err := resolveHost(s, host)
			if err != nil || len(ips) == 0 {
				// 回退到系统默认
				d := net.Dialer{Timeout: 10 * time.Second}
				return d.DialContext(ctx, network, addr)
			}

			d := net.Dialer{Timeout: 10 * time.Second}
			return d.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
		},
		TLSHandshakeTimeout:   30 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
	})

	// 配置代理
	if s.Proxy.Enabled && s.Proxy.Host != "" {
		proxyURL := buildProxyURL(s.Proxy)
		if proxyURL != nil {
			// WrapTransport 返回的是被 useragent.Transport 包裹的 RoundTripper，
			// 故须先解包拿到底层 *http.Transport 再设置代理（直接断言 *http.Transport 必失败）。
			if t, ok := unwrapHTTPTransport(transport); ok {
				t.Proxy = http.ProxyURL(proxyURL)
			}
		}
	}

	return transport
}

// resolveHost 使用设置中启用的 DNS 服务器解析域名
func resolveHost(s settings.Settings, host string) ([]net.IP, error) {
	var servers []string
	for _, dns := range s.DNSConfigs {
		if dns.Enabled && len(dns.Servers) > 0 {
			servers = append(servers, dns.Servers...)
		}
	}

	if len(servers) == 0 {
		// 没有启用自定义 DNS，使用系统默认
		return net.DefaultResolver.LookupIP(context.Background(), "ip4", host)
	}

	// 使用第一个启用的 DNS 服务器
	for _, server := range servers {
		ips, err := queryDNSServer(server, host)
		if err == nil && len(ips) > 0 {
			return ips, nil
		}
	}

	// 全部失败，回退到系统默认
	return net.DefaultResolver.LookupIP(context.Background(), "ip4", host)
}

// queryDNSServer 向指定 DNS 服务器查询 A 记录
func queryDNSServer(server, host string) ([]net.IP, error) {
	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{Timeout: 5 * time.Second}
			return d.DialContext(ctx, "udp", net.JoinHostPort(server, "53"))
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return resolver.LookupIP(ctx, "ip4", host)
}

// buildProxyURL 根据代理配置构建 URL
func buildProxyURL(proxy settings.ProxyConfig) *url.URL {
	host := fmt.Sprintf("%s:%d", proxy.Host, proxy.Port)
	if proxy.Username != "" {
		return &url.URL{
			Scheme: proxy.Protocol,
			User:   url.UserPassword(proxy.Username, proxy.Password),
			Host:   host,
		}
	}
	return &url.URL{
		Scheme: proxy.Protocol,
		Host:   host,
	}
}
