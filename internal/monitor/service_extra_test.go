package monitor

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cnb.cool/dtapp/certflow/internal/ent"
	"cnb.cool/dtapp/certflow/internal/ent/monitorchecklog"
	"cnb.cool/dtapp/certflow/internal/ent/monitoreddomain"
	esql "entgo.io/ent/dialect/sql"
)

// extraDBCounter 为每个用例分配独立的内存库名。
var extraDBCounter atomic.Int64

// setupExtraDB 使用独立的共享内存库，避免与包内其它测试的数据互相干扰。
func setupExtraDB(t *testing.T) *ent.Client {
	t.Helper()
	name := fmt.Sprintf("file:monitor_extra_%d?mode=memory&cache=shared&_fk=1", extraDBCounter.Add(1))
	db, err := sql.Open("sqlite3", name)
	if err != nil {
		t.Fatalf("打开库失败: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	client := ent.NewClient(ent.Driver(esql.OpenDB("sqlite3", db)))
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(context.Background()); err != nil {
		t.Fatalf("建表失败: %v", err)
	}
	return client
}

// seedDomain 创建一条监控域名记录。
func seedDomain(t *testing.T, client *ent.Client, domain, checkType, rawURL string, port int) *ent.MonitoredDomain {
	t.Helper()
	m, err := client.MonitoredDomain.Create().
		SetDomain(domain).
		SetPort(port).
		SetCheckType(monitoreddomain.CheckType(checkType)).
		SetURL(rawURL).
		Save(context.Background())
	if err != nil {
		t.Fatalf("造监控域名失败: %v", err)
	}
	return m
}

// TestCheckHTTPS_StatusMapping 覆盖 HTTP 状态码到监控状态的映射，以及连接失败分支。
func TestCheckHTTPS_StatusMapping(t *testing.T) {
	cases := []struct {
		name       string
		code       int
		wantStatus string
	}{
		{"2xx 正常", http.StatusOK, "ok"},
		{"3xx 正常", http.StatusFound, "ok"},
		{"4xx 告警", http.StatusNotFound, "warning"},
		{"5xx 错误", http.StatusInternalServerError, "error"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.code)
				_, _ = w.Write([]byte("body"))
			}))
			defer srv.Close()

			svc := NewMonitorService(setupExtraDB(t))
			m := &ent.MonitoredDomain{Domain: "127.0.0.1", URL: srv.URL, CheckType: monitoreddomain.CheckTypeHTTPS}

			got := svc.checkHTTPS(m)
			if got.status != tc.wantStatus {
				t.Fatalf("状态 = %q, want %q", got.status, tc.wantStatus)
			}
			if got.httpStatusCode != tc.code {
				t.Fatalf("HTTP 状态码 = %d, want %d", got.httpStatusCode, tc.code)
			}
			if got.checkError != "" {
				t.Fatalf("不应有错误: %s", got.checkError)
			}
			if got.responseTimeMs < 0 {
				t.Fatalf("响应时间不应为负: %d", got.responseTimeMs)
			}
		})
	}

	// 连接失败：端口 1 通常无监听，立即返回错误
	svc := NewMonitorService(setupExtraDB(t))
	m := &ent.MonitoredDomain{Domain: "127.0.0.1", URL: "http://127.0.0.1:1", CheckType: monitoreddomain.CheckTypeHTTPS}
	got := svc.checkHTTPS(m)
	if got.status != "error" {
		t.Fatalf("连接失败应为 error，实际 %q", got.status)
	}
	if got.checkError == "" {
		t.Fatal("连接失败应记录错误信息")
	}
}

// TestCheckHTTPS_TLSInfo 覆盖 TLS 证书信息提取（含 URL 为空时按域名+端口拼 URL 的分支）。
func TestCheckHTTPS_TLSInfo(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// 监控客户端在未配置代理/DNS 时使用 &http.Client{}（Transport 为 nil → 走 DefaultTransport），
	// 这里临时换成测试服务端的信任链，使自签证书可通过校验。
	old := http.DefaultTransport
	http.DefaultTransport = srv.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = old })

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("解析测试地址失败: %v", err)
	}
	port := 0
	if _, p, splitErr := strings.Cut(u.Host, ":"); splitErr {
		for _, c := range p {
			port = port*10 + int(c-'0')
		}
	}

	svc := NewMonitorService(setupExtraDB(t))
	// URL 留空 → 走「按 Domain:Port 拼 URL」分支
	m := &ent.MonitoredDomain{Domain: u.Hostname(), Port: port, CheckType: monitoreddomain.CheckTypeHTTPS}

	got := svc.checkHTTPS(m)
	if got.status != "ok" {
		t.Fatalf("状态 = %q, want ok（错误: %s）", got.status, got.checkError)
	}
	if got.certFingerprint == "" || len(got.certFingerprint) != 64 {
		t.Fatalf("指纹应为 64 位 hex: %q", got.certFingerprint)
	}
	if got.certIssuer == "" {
		t.Fatal("应提取到颁发者（httptest 证书无 CN，需按 Organization 兜底）")
	}
	if got.certSubject != "example.com" {
		t.Fatalf("主体应为首个 SAN example.com，实际 %q", got.certSubject)
	}
	if got.certPublicKeyBits <= 0 {
		t.Fatalf("公钥位数应大于 0: %d", got.certPublicKeyBits)
	}
	if got.certSignatureAlgo == "" || got.certPublicKeyAlgo == "" {
		t.Fatalf("签名/公钥算法应被填充: %q %q", got.certSignatureAlgo, got.certPublicKeyAlgo)
	}
	if len(got.certSANs) == 0 {
		t.Fatal("应提取到 SAN")
	}
	if got.certRemainingDays < 0 {
		t.Fatalf("剩余天数不应为负: %d", got.certRemainingDays)
	}
}

// TestCertDisplayName 覆盖证书颁发者/主体展示名的兜底顺序。
func TestCertDisplayName(t *testing.T) {
	type fields struct {
		cn   string
		orgs []string
		sans []string
	}
	mk := func(f fields) *x509.Certificate {
		c := &x509.Certificate{}
		c.Issuer = pkix.Name{CommonName: f.cn, Organization: f.orgs}
		c.Subject = pkix.Name{CommonName: f.cn, Organization: f.orgs}
		c.DNSNames = f.sans
		return c
	}

	t.Run("颁发者", func(t *testing.T) {
		cases := []struct {
			name string
			in   fields
			want string
		}{
			{"优先 CN", fields{cn: "issuer-cn", orgs: []string{"Org"}}, "issuer-cn"},
			{"无 CN 用 Organization", fields{orgs: []string{"", "Acme Co"}}, "Acme Co"},
			{"全空返回空串", fields{orgs: []string{""}}, ""},
		}
		for _, tc := range cases {
			if got := certIssuerName(mk(tc.in)); got != tc.want {
				t.Fatalf("%s: certIssuerName = %q, want %q", tc.name, got, tc.want)
			}
		}
	})

	t.Run("主体", func(t *testing.T) {
		cases := []struct {
			name string
			in   fields
			want string
		}{
			{"优先 CN", fields{cn: "cn.example.com", orgs: []string{"Org"}, sans: []string{"san.example.com"}}, "cn.example.com"},
			{"无 CN 时 SAN 优先于组织", fields{orgs: []string{"Acme Co"}, sans: []string{"san.example.com"}}, "san.example.com"},
			{"无 SAN 用 Organization", fields{orgs: []string{"Acme Co"}}, "Acme Co"},
			{"全空返回空串", fields{orgs: []string{""}, sans: []string{""}}, ""},
		}
		for _, tc := range cases {
			if got := certSubjectName(mk(tc.in)); got != tc.want {
				t.Fatalf("%s: certSubjectName = %q, want %q", tc.name, got, tc.want)
			}
		}
	})
}

// TestGetPubKeyBits 覆盖各类公钥的位数换算。
func TestGetPubKeyBits(t *testing.T) {
	p256, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("生成 P256 失败: %v", err)
	}
	p384, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatalf("生成 P384 失败: %v", err)
	}
	p521, err := ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
	if err != nil {
		t.Fatalf("生成 P521 失败: %v", err)
	}
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("生成 RSA 失败: %v", err)
	}
	_, edPub, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("生成 ed25519 失败: %v", err)
	}

	cases := []struct {
		name string
		key  any
		want int
	}{
		{"P256", &p256.PublicKey, 256},
		{"P384", &p384.PublicKey, 384},
		{"P521", &p521.PublicKey, 521},
		{"RSA2048", &rsaKey.PublicKey, 2048},
		{"ed25519 无 Size 方法", edPub, 0},
		{"未知类型", "not-a-key", 0},
		{"nil 公钥", nil, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := getPubKeyBits(&x509.Certificate{PublicKey: tc.key}); got != tc.want {
				t.Fatalf("位数 = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestListHistory 覆盖历史查询的天数默认值与时间过滤。
func TestListHistory(t *testing.T) {
	ctx := context.Background()
	client := setupExtraDB(t)
	svc := NewMonitorService(client)
	m := seedDomain(t, client, "history.example.com", "https", "https://history.example.com", 443)

	now := time.Now()
	// 两条近期记录 + 一条 40 天前的记录
	for i, at := range []time.Time{now.Add(-2 * time.Hour), now.Add(-1 * time.Hour), now.AddDate(0, 0, -40)} {
		if _, err := client.MonitorCheckLog.Create().
			SetDomain(m).
			SetStatus(monitorchecklog.Status("ok")).
			SetCheckedAt(at).
			SetResponseTimeMs(100 + i).
			SetHTTPStatusCode(200).
			SetLastCheckError("").
			Save(ctx); err != nil {
			t.Fatalf("造检查日志失败: %v", err)
		}
	}

	// days=0 → 默认 30 天，应只取到近两条，且按时间升序
	items, err := svc.ListHistory(ctx, m.ID, 0)
	if err != nil {
		t.Fatalf("ListHistory 失败: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("默认 30 天应返回 2 条，实际 %d", len(items))
	}
	if items[0].CheckedAt >= items[1].CheckedAt {
		t.Fatalf("应按 checked_at 升序: %q → %q", items[0].CheckedAt, items[1].CheckedAt)
	}
	if items[0].ResponseTimeMs == nil || *items[0].ResponseTimeMs != 100 {
		t.Fatalf("最早一条的响应时间应为 100: %+v", items[0].ResponseTimeMs)
	}

	// days=60 → 三条都在
	items, err = svc.ListHistory(ctx, m.ID, 60)
	if err != nil {
		t.Fatalf("ListHistory 失败: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("60 天应返回 3 条，实际 %d", len(items))
	}

	// 不存在的域名 → 空切片无错误
	items, err = svc.ListHistory(ctx, 999999, 30)
	if err != nil {
		t.Fatalf("不存在的域名不应报错: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("不应返回记录，实际 %d", len(items))
	}
}

// TestSetActive 覆盖启用/停用与不存在 ID 的错误分支。
func TestSetActive(t *testing.T) {
	ctx := context.Background()
	client := setupExtraDB(t)
	svc := NewMonitorService(client)
	m := seedDomain(t, client, "toggle.example.com", "https", "https://toggle.example.com", 443)

	// 未检查前先伪造一条告警基线，验证重新启用时会清除
	svc.notifMu.Lock()
	svc.lastNotified[m.ID] = "problem"
	svc.notifMu.Unlock()

	item, err := svc.SetActive(ctx, m.ID, false)
	if err != nil {
		t.Fatalf("停用失败: %v", err)
	}
	if item.Enabled {
		t.Fatal("停用后 Enabled 应为 false")
	}
	svc.notifMu.Lock()
	stillSet := svc.lastNotified[m.ID] != ""
	svc.notifMu.Unlock()
	if !stillSet {
		t.Fatal("停用不应清除告警基线")
	}

	item, err = svc.SetActive(ctx, m.ID, true)
	if err != nil {
		t.Fatalf("启用失败: %v", err)
	}
	if !item.Enabled {
		t.Fatal("启用后 Enabled 应为 true")
	}
	svc.notifMu.Lock()
	_, exists := svc.lastNotified[m.ID]
	svc.notifMu.Unlock()
	if exists {
		t.Fatal("重新启用应清除告警基线，使下次异常可重新通知")
	}

	if _, err := svc.SetActive(ctx, 999999, true); err == nil {
		t.Fatal("不存在的 ID 应报错")
	}
}

// TestCheckNow 覆盖立即检查的完整链路：请求 → 更新域名状态 → 写历史。
func TestCheckNow(t *testing.T) {
	ctx := context.Background()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := setupExtraDB(t)
	svc := NewMonitorService(client)
	m := seedDomain(t, client, "now.example.com", "https", srv.URL, 443)

	item, err := svc.CheckNow(ctx, m.ID)
	if err != nil {
		t.Fatalf("CheckNow 失败: %v", err)
	}
	if item.Status != "ok" {
		t.Fatalf("状态 = %q, want ok（错误: %s）", item.Status, item.LastCheckError)
	}
	if item.LastCheckAt == "" {
		t.Fatal("应记录最后检查时间")
	}
	if item.HTTPStatusCode != http.StatusOK {
		t.Fatalf("HTTP 状态码 = %d, want 200", item.HTTPStatusCode)
	}

	// 应写入一条历史记录
	n, err := client.MonitorCheckLog.Query().Count(ctx)
	if err != nil {
		t.Fatalf("统计历史失败: %v", err)
	}
	if n != 1 {
		t.Fatalf("应写入 1 条历史，实际 %d", n)
	}

	if _, err := svc.CheckNow(ctx, 999999); err == nil {
		t.Fatal("不存在的 ID 应报错")
	}
}
