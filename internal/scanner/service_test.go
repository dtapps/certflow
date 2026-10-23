package scanner

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"entgo.io/ent/dialect"

	"cnb.cool/dtapp/certflow/internal/ent/enttest"
	"cnb.cool/dtapp/certflow/internal/sqlite"
)

func newTestScanner(t *testing.T) *ScannerService {
	t.Helper()
	client := enttest.Open(t, dialect.SQLite, sqlite.BuildDSN(filepath.Join(t.TempDir(), "scan.db")))
	t.Cleanup(func() { _ = client.Close() })
	return NewScannerService(client)
}

// portOf 从 httptest server 的 URL（http:// 或 https://）里取端口。
func portOf(t *testing.T, ts *httptest.Server) int {
	t.Helper()
	u, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("解析 URL 失败: %v", err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatalf("解析端口失败: %v", err)
	}
	return port
}

func TestScan_EmptyDomain_NotPersisted(t *testing.T) {
	s := newTestScanner(t)
	if _, err := s.Scan(context.Background(), ScanInput{Domain: ""}); err == nil {
		t.Fatal("空域名应返回错误")
	}
	// 校验失败不应该写库
	items, err := s.ListHistory(context.Background())
	if err != nil {
		t.Fatalf("ListHistory err: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("空域名请求不应落库, 实际 %d 条", len(items))
	}
}

// TestScan_HTTP 本地起一个 HTTP 服务，跑通「请求 → 计时 → 落库」全链路。
// 用 http 类型是为了避开 TLS 校验（扫描器用的是系统信任链，自签证书必然握手失败）。
func TestScan_HTTP(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != "CertFlow/1.0" {
			t.Errorf("User-Agent 不符: %q", r.Header.Get("User-Agent"))
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer ts.Close()

	s := newTestScanner(t)
	port := portOf(t, ts)

	item, err := s.Scan(context.Background(), ScanInput{Domain: "127.0.0.1", Port: port, ScanType: "http"})
	if err != nil {
		t.Fatalf("Scan err: %v", err)
	}
	if item.ID == 0 {
		t.Fatal("应返回落库后的自增 ID")
	}
	if item.ErrorMessage != "" {
		t.Fatalf("不应有错误: %s", item.ErrorMessage)
	}
	if item.Port != port || item.ScanType != "http" {
		t.Fatalf("端口/类型不符: %+v", item)
	}
	// http 模式不提取证书
	if item.CertIssuer != "" || item.CertFingerprint != "" {
		t.Fatalf("http 模式不应有证书信息: %+v", item)
	}
	if _, err := time.Parse(time.RFC3339, item.ScannedAt); err != nil {
		t.Fatalf("scanned_at 不是 RFC3339: %q", item.ScannedAt)
	}

	items, err := s.ListHistory(context.Background())
	if err != nil {
		t.Fatalf("ListHistory err: %v", err)
	}
	if len(items) != 1 || items[0].ID != item.ID {
		t.Fatalf("历史记录不符: %+v", items)
	}
}

// TestScan_DefaultPorts 未指定端口时按 scan_type 推默认值（443 / 80）。
func TestScan_DefaultPorts(t *testing.T) {
	s := newTestScanner(t)
	ctx := context.Background()

	// 端口不填 → 走默认 443，连接失败但不是「参数错误」
	item, err := s.Scan(ctx, ScanInput{Domain: "127.0.0.1"})
	if err != nil {
		t.Fatalf("扫描失败应体现在 ErrorMessage 而不是 error: %v", err)
	}
	if item.Port != 443 || item.ScanType != "https" {
		t.Fatalf("默认值不符: port=%d type=%s", item.Port, item.ScanType)
	}
	if item.ErrorMessage == "" {
		t.Fatal("连不上 127.0.0.1:443 应记录错误信息")
	}

	// http 类型默认 80
	item, err = s.Scan(ctx, ScanInput{Domain: "127.0.0.1", ScanType: "http"})
	if err != nil {
		t.Fatalf("Scan err: %v", err)
	}
	if item.Port != 80 {
		t.Fatalf("http 默认端口应为 80, 实际 %d", item.Port)
	}
}

// TestScan_ConnectionRefused 连接失败也必须落库（用户要看得到「这个域名扫不通」）。
func TestScan_ConnectionRefused(t *testing.T) {
	s := newTestScanner(t)
	ctx := context.Background()

	// 拿一个刚关闭的端口，保证 refused
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()

	item, err := s.Scan(ctx, ScanInput{Domain: "127.0.0.1", Port: port, ScanType: "http"})
	if err != nil {
		t.Fatalf("扫描	err: %v", err)
	}
	if item.ErrorMessage == "" {
		t.Fatal("连接失败应写入 ErrorMessage")
	}
	if item.ID == 0 {
		t.Fatal("失败结果也要落库")
	}
}

// TestExtractCertInfo 直接喂一个自签 TLS 服务，验证证书字段提取（这些字段是「证书检查」
// 页面的全部内容，指纹/算法/公钥位数算错会让用户误判）。
func TestExtractCertInfo(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer ts.Close()

	// 扫描器线上用的是系统信任链（自签证书会被拒），这里为了验证提取逻辑显式放宽校验
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	s := newTestScanner(t)
	result := &ScanResultItem{
		Domain:    "127.0.0.1",
		Port:      portOf(t, ts),
		ScanType:  "https",
		ScannedAt: time.Now().Format(time.RFC3339),
	}

	s.extractCertInfo("127.0.0.1", result.Port, client, result)

	if result.ErrorMessage != "" {
		t.Fatalf("提取失败: %s", result.ErrorMessage)
	}
	if result.CertIssuer == "" || result.CertSubject == "" {
		t.Fatalf("颁发者/主题为空: %+v", result)
	}
	if len(result.CertFingerprint) != 64 {
		t.Fatalf("SHA256 指纹应是 64 位 hex, 实际 %q", result.CertFingerprint)
	}
	if result.CertSignatureAlgo == "" || result.CertPublicKeyAlgo == "" {
		t.Fatalf("签名/公钥算法为空: %+v", result)
	}
	if result.CertPublicKeyBits <= 0 {
		t.Fatalf("公钥位数应 > 0, 实际 %d", result.CertPublicKeyBits)
	}
	if result.CertSerialNumber == "" {
		t.Fatal("序列号为空")
	}
	if _, err := time.Parse(time.RFC3339, result.CertNotBefore); err != nil {
		t.Fatalf("not_before 不是 RFC3339: %q", result.CertNotBefore)
	}
	if _, err := time.Parse(time.RFC3339, result.CertNotAfter); err != nil {
		t.Fatalf("not_after 不是 RFC3339: %q", result.CertNotAfter)
	}
	// httptest 自签证书的 SAN 含 example.com，提取到 DNSNames 才算对
	if len(result.CertSANs) == 0 {
		t.Fatalf("SAN 为空: %+v", result)
	}
	// 这张测试证书没有 Subject/Issuer CN，必须走兜底
	if result.CertSubject != result.CertSANs[0] {
		t.Fatalf("CN 为空时应回退到首个 SAN, 实际 %q", result.CertSubject)
	}
}

// TestGetPubKeyBits 公钥位数：ECDSA 看曲线位宽，RSA 看模长，其它类型给 0（不能瞎报）。
func TestGetPubKeyBits(t *testing.T) {
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("生成 EC 密钥失败: %v", err)
	}
	ecCert := &x509.Certificate{PublicKey: &ecKey.PublicKey}
	if got := getPubKeyBits(ecCert); got != 256 {
		t.Fatalf("P-256 应为 256 位, 实际 %d", got)
	}

	p384, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatalf("生成 P-384 密钥失败: %v", err)
	}
	if got := getPubKeyBits(&x509.Certificate{PublicKey: &p384.PublicKey}); got != 384 {
		t.Fatalf("P-384 应为 384 位, 实际 %d", got)
	}

	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("生成 RSA 密钥失败: %v", err)
	}
	if got := getPubKeyBits(&x509.Certificate{PublicKey: &rsaKey.PublicKey}); got != 2048 {
		t.Fatalf("RSA2048 应为 2048 位, 实际 %d", got)
	}

	// Ed25519 固定 256 位
	edPub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("生成 Ed25519 密钥失败: %v", err)
	}
	if got := getPubKeyBits(&x509.Certificate{PublicKey: edPub}); got != 256 {
		t.Fatalf("Ed25519 应为 256 位, 实际 %d", got)
	}

	// 未知算法
	if got := getPubKeyBits(&x509.Certificate{PublicKey: "not-a-key"}); got != 0 {
		t.Fatalf("未知算法应返回 0, 实际 %d", got)
	}
	if got := getPubKeyBits(&x509.Certificate{}); got != 0 {
		t.Fatalf("无公钥应返回 0, 实际 %d", got)
	}
}
