package deploy

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"

	esaclient "github.com/alibabacloud-go/esa-20240910/v3/client"
	"github.com/alibabacloud-go/tea/tea"
)

// TestSplitCertChain 覆盖证书链拆分（百度云要求叶子证书与中间链分开提交）。
func TestSplitCertChain(t *testing.T) {
	keyPEM, certPEM := genKeyCert(t)
	_ = keyPEM

	leaf, chain := splitCertChain(certPEM)
	if !strings.Contains(leaf, "BEGIN CERTIFICATE") {
		t.Fatalf("叶子证书解析失败: %q", leaf)
	}
	if chain != "" {
		t.Fatalf("单证书不应有中间链: %q", chain)
	}

	// 两张拼接：第二张进入中间链
	two := certPEM + certPEM
	leaf, chain = splitCertChain(two)
	if leaf == "" || chain == "" {
		t.Fatalf("链式证书拆分失败: leaf=%q chain=%q", leaf, chain)
	}
	if strings.Count(leaf, "BEGIN CERTIFICATE") != 1 || strings.Count(chain, "BEGIN CERTIFICATE") != 1 {
		t.Fatal("链式证书拆分数量不符")
	}

	// 非法输入：两者均为空
	leaf, chain = splitCertChain("not a pem")
	if leaf != "" || chain != "" {
		t.Fatalf("非法输入应返回空: leaf=%q chain=%q", leaf, chain)
	}
}

// TestPrivateKeyPublicKey 覆盖私钥解析的三种格式与不支持的格式。
func TestPrivateKeyPublicKey(t *testing.T) {
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("生成 RSA 密钥失败: %v", err)
	}
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("生成 EC 密钥失败: %v", err)
	}

	pkcs1 := x509.MarshalPKCS1PrivateKey(rsaKey)
	pkcs8RSA, err := x509.MarshalPKCS8PrivateKey(rsaKey)
	if err != nil {
		t.Fatalf("编码 PKCS8(RSA) 失败: %v", err)
	}
	sec1, err := x509.MarshalECPrivateKey(ecKey)
	if err != nil {
		t.Fatalf("编码 SEC1 失败: %v", err)
	}
	pkcs8EC, err := x509.MarshalPKCS8PrivateKey(ecKey)
	if err != nil {
		t.Fatalf("编码 PKCS8(EC) 失败: %v", err)
	}

	cases := []struct {
		name string
		der  []byte
		want any
	}{
		{"PKCS1", pkcs1, &rsaKey.PublicKey},
		{"SEC1", sec1, &ecKey.PublicKey},
		{"PKCS8-RSA", pkcs8RSA, &rsaKey.PublicKey},
		{"PKCS8-EC", pkcs8EC, &ecKey.PublicKey},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pub, err := privateKeyPublicKey(c.der)
			if err != nil {
				t.Fatalf("解析失败: %v", err)
			}
			if !publicKeysEqual(pub, c.want) {
				t.Fatal("提取的公钥与原公钥不一致")
			}
		})
	}

	if _, err := privateKeyPublicKey([]byte("garbage")); err == nil {
		t.Fatal("非法私钥应报错")
	}
}

// TestPublicKeysEqual 覆盖公钥比较的类型分支。
func TestPublicKeysEqual(t *testing.T) {
	rsaA, _ := rsa.GenerateKey(rand.Reader, 2048)
	rsaB, _ := rsa.GenerateKey(rand.Reader, 2048)
	ecA, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	ecB, _ := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)

	if !publicKeysEqual(&rsaA.PublicKey, &rsaA.PublicKey) {
		t.Fatal("同一 RSA 公钥应相等")
	}
	if publicKeysEqual(&rsaA.PublicKey, &rsaB.PublicKey) {
		t.Fatal("不同 RSA 公钥不应相等")
	}
	if publicKeysEqual(&rsaA.PublicKey, &ecA.PublicKey) {
		t.Fatal("RSA 与 ECDSA 公钥不应相等")
	}
	if !publicKeysEqual(&ecA.PublicKey, &ecA.PublicKey) {
		t.Fatal("同一 ECDSA 公钥应相等")
	}
	if publicKeysEqual(&ecA.PublicKey, &ecB.PublicKey) {
		t.Fatal("不同 ECDSA 公钥不应相等")
	}
}

// TestCertKeyMatch 覆盖证书/私钥配对校验的各条错误分支。
func TestCertKeyMatch(t *testing.T) {
	keyPEM, certPEM := genKeyCert(t)

	if err := certKeyMatch(certPEM, keyPEM); err != nil {
		t.Fatalf("配对证书应通过校验: %v", err)
	}

	// 另一张证书 + 原私钥：不配对
	otherKey, otherCert := genKeyCert(t)
	_ = otherKey
	if err := certKeyMatch(otherCert, keyPEM); err == nil {
		t.Fatal("证书与私钥不配对应报错")
	}

	// 证书 PEM 非法
	if err := certKeyMatch("garbage", keyPEM); err == nil {
		t.Fatal("证书 PEM 非法应报错")
	}
	// 私钥 PEM 非法
	if err := certKeyMatch(certPEM, "garbage"); err == nil {
		t.Fatal("私钥 PEM 非法应报错")
	}
	// 私钥类型不支持（用证书块充当私钥块）
	if err := certKeyMatch(certPEM, certPEM); err == nil {
		t.Fatal("不支持的私钥格式应报错")
	}
}

// TestBaiduCertName 覆盖百度云证书命名：优先配置名，否则净化域名 + 指纹后缀。
func TestBaiduCertName(t *testing.T) {
	cert := CertContent{Domain: "*.example.com", CertPEM: "c", KeyPEM: "k"}
	if got := baiduCertName(cert, map[string]string{"cert_name": "自定义"}); got != "自定义" {
		t.Fatalf("应优先使用配置名，实际 %q", got)
	}

	got := baiduCertName(cert, nil)
	if !strings.HasPrefix(got, "certflow-wildcard-example-com-") {
		t.Fatalf("通配域名未净化: %q", got)
	}
	if len(got) != len("certflow-wildcard-example-com-")+8 {
		t.Fatalf("指纹后缀长度不符: %q", got)
	}
	// 同内容幂等、不同内容不同名
	if again := baiduCertName(cert, nil); again != got {
		t.Fatal("同内容命名应稳定")
	}
	cert.KeyPEM = "k2"
	if diff := baiduCertName(cert, nil); diff == got {
		t.Fatal("不同内容应生成不同名称")
	}
}

// TestHostMatchesPattern 覆盖域名通配匹配。
func TestHostMatchesPattern(t *testing.T) {
	cases := []struct {
		domain, pattern string
		want            bool
	}{
		{"a.example.com", "a.example.com", true},
		{"A.Example.com", "a.example.com", true},
		{"a.example.com", "*.example.com", true},
		{"example.com", "*.example.com", true},
		{"deep.a.example.com", "*.example.com", true},
		{"example.com.evil.com", "*.example.com", false},
		{"b.example.org", "*.example.com", false},
		{"", "*.example.com", false},
		{"a.example.com", "", false},
		{"a.example.com", "b.example.com", false},
	}
	for _, c := range cases {
		if got := hostMatchesPattern(c.domain, c.pattern); got != c.want {
			t.Fatalf("hostMatchesPattern(%q, %q) = %v, want %v", c.domain, c.pattern, got, c.want)
		}
	}
}

// TestSplitCommaList 覆盖证书 SAN 字符串拆分。
func TestSplitCommaList(t *testing.T) {
	if got := splitCommaList(""); got != nil {
		t.Fatalf("空串应返回 nil，实际 %v", got)
	}
	got := splitCommaList(" a.com, b.com ,, c.com ")
	if len(got) != 3 || got[0] != "a.com" || got[2] != "c.com" {
		t.Fatalf("拆分结果不符: %v", got)
	}
}

// TestParseESACertDate 覆盖 ESA 日期格式化。
func TestParseESACertDate(t *testing.T) {
	if got := parseESACertDate(""); got != "" {
		t.Fatalf("空串应返回空，实际 %q", got)
	}
	if got := parseESACertDate("2026-01-02 03:04:05"); got != "2026-01-02T03:04:05Z" {
		t.Fatalf("UTC 时间格式化不符: %q", got)
	}
	if got := parseESACertDate("不是时间"); got != "不是时间" {
		t.Fatalf("解析失败应原样返回，实际 %q", got)
	}
}

// TestESACertHelpers 覆盖 ESA 证书覆盖判断与最新证书挑选。
func TestESACertHelpers(t *testing.T) {
	cert := &esaclient.ListCertificatesResponseBodyResult{
		CommonName: new("a.example.com"),
		SAN:        new("b.example.com, *.cdn.example.com"),
		NotAfter:   new("2026-06-01 00:00:00"),
	}
	if !certCoversDomain(cert, "a.example.com") {
		t.Fatal("应命中 CN")
	}
	if !certCoversDomain(cert, "b.example.com") {
		t.Fatal("应命中 SAN")
	}
	if !certCoversDomain(cert, "x.cdn.example.com") {
		t.Fatal("应命中通配 SAN")
	}
	if certCoversDomain(cert, "other.example.org") {
		t.Fatal("不应命中无关域名")
	}

	older := &esaclient.ListCertificatesResponseBodyResult{
		CommonName: new("old.example.com"),
		NotAfter:   new("2025-01-01 00:00:00"),
	}
	invalid := &esaclient.ListCertificatesResponseBodyResult{
		CommonName: new("bad.example.com"),
		NotAfter:   new("不是时间"),
	}
	if best := latestByNotAfter([]*esaclient.ListCertificatesResponseBodyResult{older, cert, invalid}); best != cert {
		t.Fatalf("应挑选 NotAfter 最新的证书，实际 %v", tea.StringValue(best.CommonName))
	}
	if latestByNotAfter(nil) != nil {
		t.Fatal("空列表应返回 nil")
	}
}

// TestAcePanelHelpers 覆盖 AcePanel 站点展示名与证书 ID 解析。
func TestAcePanelHelpers(t *testing.T) {
	if got := (acePanelSite{ID: 7, Name: "site", Domains: []string{"a.com"}}).displayName(); got != "a.com" {
		t.Fatalf("应优先取域名，实际 %q", got)
	}
	if got := (acePanelSite{ID: 7, Name: "site"}).displayName(); got != "site" {
		t.Fatalf("无域名应回退站点名，实际 %q", got)
	}
	if got := (acePanelSite{ID: 7}).displayName(); got != "7" {
		t.Fatalf("无名称应回退 ID，实际 %q", got)
	}
	if got := (acePanelSite{ID: 7, Name: "site", Domains: []string{""}}).displayName(); got != "site" {
		t.Fatalf("空域名应回退站点名，实际 %q", got)
	}

	// anyToInt 各类型
	if n, ok := anyToInt(float64(12)); !ok || n != 12 {
		t.Fatal("float64 转换失败")
	}
	if n, ok := anyToInt(12); !ok || n != 12 {
		t.Fatal("int 转换失败")
	}
	if n, ok := anyToInt(int64(12)); !ok || n != 12 {
		t.Fatal("int64 转换失败")
	}
	if n, ok := anyToInt(" 12 "); !ok || n != 12 {
		t.Fatal("字符串转换失败")
	}
	if _, ok := anyToInt("abc"); ok {
		t.Fatal("非数字字符串不应转换成功")
	}
	if _, ok := anyToInt(nil); ok {
		t.Fatal("nil 不应转换成功")
	}

	// parseAcePanelCertID 各响应形态
	if id, err := parseAcePanelCertID([]byte(`{"msg":"success","data":{"id":9}}`)); err != nil || id != 9 {
		t.Fatalf("对象 id 解析失败: id=%d err=%v", id, err)
	}
	if id, err := parseAcePanelCertID([]byte(`{"msg":"success","data":{"cert_id":"11"}}`)); err != nil || id != 11 {
		t.Fatalf("对象 cert_id 解析失败: id=%d err=%v", id, err)
	}
	if id, err := parseAcePanelCertID([]byte(`{"data":13}`)); err != nil || id != 13 {
		t.Fatalf("裸数字解析失败: id=%d err=%v", id, err)
	}
	if _, err := parseAcePanelCertID([]byte(`{"msg":"failed"}`)); err == nil {
		t.Fatal("失败响应应报错")
	}
	if _, err := parseAcePanelCertID([]byte(`{"msg":"success","data":{}}`)); err == nil {
		t.Fatal("缺少 id 应报错")
	}
	if _, err := parseAcePanelCertID([]byte(`not json`)); err == nil {
		t.Fatal("非法 JSON 应报错")
	}

	// serialEqual：十进制 / 十六进制 / 带分隔符
	wantDec := "255"
	if !serialEqual("255", wantDec) {
		t.Fatal("十进制应相等")
	}
	if !serialEqual("ff", wantDec) {
		t.Fatal("十六进制应相等")
	}
	if !serialEqual("0xFF", wantDec) {
		t.Fatal("0x 前缀十六进制应相等")
	}
	if !serialEqual("00:ff", wantDec) {
		t.Fatal("带冒号分隔应相等")
	}
	if serialEqual("100", wantDec) {
		t.Fatal("不同序列号不应相等")
	}
	if serialEqual("zz", wantDec) {
		t.Fatal("非法十六进制不应相等")
	}
}

// TestParseAAWafSite 覆盖堡塔云 WAF 站点响应解析。
func TestParseAAWafSite(t *testing.T) {
	body := []byte(`{"code":0,"res":{"list":[
		{"site_name":"站点A","site_id":"1"},
		{"site_name":"站点B","site_id":"2","server":{"listen_ssl_port":["443"],"ssl":{"is_ssl":1}}}
	]}}`)

	site, err := parseAAWafSite(body, "2")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if site.SiteName != "站点B" || len(site.Server.ListenSSLPort) != 1 || site.Server.SSL.IsSSL != 1 {
		t.Fatalf("站点解析不符: %+v", site)
	}

	// 未命中指定 ID 时回退第一条
	site, err = parseAAWafSite(body, "not-exist")
	if err != nil {
		t.Fatalf("回退解析失败: %v", err)
	}
	if site.SiteID != "1" {
		t.Fatalf("未命中应回退第一条，实际 %+v", site)
	}

	// 空列表报错
	if _, err := parseAAWafSite([]byte(`{"code":0,"res":{"list":[]}}`), "1"); err == nil {
		t.Fatal("空列表应报错")
	}
	// 业务错误码
	if _, err := parseAAWafSite([]byte(`{"code":1,"msg":"无权限"}`), "1"); err == nil {
		t.Fatal("业务错误码应报错")
	}
	// 非法 JSON
	if _, err := parseAAWafSite([]byte(`oops`), "1"); err == nil {
		t.Fatal("非法 JSON 应报错")
	}
}

// TestCertFingerprintAndSameCert 覆盖证书指纹与同证书判断。
func TestCertFingerprintAndSameCert(t *testing.T) {
	_, certPEM := genKeyCert(t)
	fp, err := certFingerprint(certPEM)
	if err != nil {
		t.Fatalf("计算指纹失败: %v", err)
	}
	if len(fp) != 32 {
		t.Fatalf("SHA256 指纹长度应为 32，实际 %d", len(fp))
	}
	if !sameCertPEM(certPEM, certPEM) {
		t.Fatal("同一证书应判等")
	}
	_, other := genKeyCert(t)
	if sameCertPEM(certPEM, other) {
		t.Fatal("不同证书不应判等")
	}
	if sameCertPEM(certPEM, "garbage") {
		t.Fatal("非法 PEM 应判为不相等")
	}
	if _, err := certFingerprint("garbage"); err == nil {
		t.Fatal("非法 PEM 计算指纹应报错")
	}
	// 指纹按 DER 原始字节计算（用于内容比对，不做 x509 校验）
	block := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("not-der")})
	if fp, err := certFingerprint(string(block)); err != nil || len(fp) != 32 {
		t.Fatalf("非标准 DER 也应能计算指纹: fp=%d err=%v", len(fp), err)
	}
	// 只有非 CERTIFICATE 块时应报错
	keyBlock := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("x")})
	if _, err := certFingerprint(string(keyBlock)); err == nil {
		t.Fatal("无 CERTIFICATE 块应报错")
	}
}
