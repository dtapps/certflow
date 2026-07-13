package deploy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestCtyunStringToSign 校验待签名串与官方「网关EOP签名说明」示例完全一致。
// 官方示例（query 为空、body 为空）：
//
//	ctyun-eop-request-id:27cfe4dc-e640-45f6-92ca-492ca73e8680
//	eop-date:20220525T160752Z
//	（两个空行分隔）
//	e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
func TestCtyunStringToSign(t *testing.T) {
	requestID := "27cfe4dc-e640-45f6-92ca-492ca73e8680"
	date := "20220525T160752Z"
	emptyHash := hex.EncodeToString(sha256.New().Sum(nil)) // sha256("")

	want := "ctyun-eop-request-id:" + requestID + "\n" +
		"eop-date:" + date + "\n" +
		"\n" +
		"\n" +
		emptyHash

	got := ctyunStringToSign(requestID, date, nil, []byte{})
	if got != want {
		t.Fatalf("待签名串不匹配\n got=%q\nwant=%q", got, want)
	}

	// 非空 body：结尾应为 body 的 sha256 hex，且整体仍为 3 段换行结构
	body := []byte(`{"name":"test","certs":"x","key":"y"}`)
	got2 := ctyunStringToSign(requestID, date, nil, body)
	sum := sha256.Sum256(body)
	if !strings.HasSuffix(got2, hex.EncodeToString(sum[:])) {
		t.Fatalf("非空 body 待签名串未按 body hash 结尾: %q", got2)
	}
	if !strings.HasPrefix(got2, "ctyun-eop-request-id:"+requestID+"\neop-date:"+date+"\n\n\n") {
		t.Fatalf("非空 body 待签名串头部结构错误: %q", got2)
	}
}

// TestCtyunDeployCertUsesCertName 验证配置 HTTPS 证书时使用「证书备注名」作为 cert_name，
// 而非数字证书 id（天翼云 update-domain 以备注名关联证书，传 id 会报“不存在此证书”）。
func TestCtyunDeployCertUsesCertName(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"statusCode":100000,"message":"正确返回","returnObj":{}}`))
	}))
	defer srv.Close()

	// 临时把 ctcdn 的 BaseURL 指向测试 server（同包可访问未导出变量）。
	orig := ctyunServices["ctcdn"]
	ctyunServices["ctcdn"] = ctyunServiceConfig{
		BaseURL:    srv.URL,
		CertPath:   orig.CertPath,
		DomainPath: orig.DomainPath,
		ListPath:   orig.ListPath,
	}
	defer func() { ctyunServices["ctcdn"] = orig }()

	d := &CtyunDeployer{}
	creds := Credentials{AccessKeyID: "ak", AccessKeySecret: "sk"}
	svcConfig := map[string]string{"domain": "www.example.com"}

	// certID 即证书备注名（UploadCert 返回的 cloudCertID），DeployCert 应原样作为 cert_name 透传。
	certName := "certflow-www-example-com-abcdef12"
	res, err := d.DeployCert(context.Background(), creds, certName, "ctcdn", svcConfig)
	if err != nil {
		t.Fatalf("DeployCert 失败: %v", err)
	}
	if res == nil {
		t.Fatal("res 为 nil")
	}
	if gotBody["cert_name"] != certName {
		t.Fatalf("cert_name 应为备注名 %s，实际=%v", certName, gotBody["cert_name"])
	}
	// 部署证书不应擅自修改域名原有的 HTTPS 开关状态，故 body 中不应出现 https_status。
	if _, ok := gotBody["https_status"]; ok {
		t.Fatalf("body 不应包含 https_status，实际=%v", gotBody["https_status"])
	}
}

// TestCtyunDeployCertIcdn 验证全站加速（icdn）使用 /v1/domain/update-domain（增量修改）后，
// 请求体与 ctcdn 一致：domain 为 string、不含 product_code、cert_name 透传备注名。
// 此前 icdn 用批量接口要求 domain 为 list 报错「参数domain为list型」，切回 update-domain 后不再需要。
func TestCtyunDeployCertIcdn(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"statusCode":100000,"message":"正确返回","returnObj":{}}`))
	}))
	defer srv.Close()

	orig := ctyunServices["icdn"]
	ctyunServices["icdn"] = ctyunServiceConfig{
		BaseURL:    srv.URL,
		CertPath:   orig.CertPath,
		DomainPath: orig.DomainPath,
		ListPath:   orig.ListPath,
	}
	defer func() { ctyunServices["icdn"] = orig }()

	d := &CtyunDeployer{}
	creds := Credentials{AccessKeyID: "ak", AccessKeySecret: "sk"}
	svcConfig := map[string]string{"domain": "www.example.com"}

	certName := "icdn-certflow-www-example-com-abcdef12"
	if _, err := d.DeployCert(context.Background(), creds, certName, "icdn", svcConfig); err != nil {
		t.Fatalf("DeployCert 失败: %v", err)
	}
	// icdn 已改为 update-domain，domain 应为 string（不再是之前批量接口的 list）。
	if gotBody["domain"] != "www.example.com" {
		t.Fatalf("icdn domain 应为 string www.example.com，实际=%v", gotBody["domain"])
	}
	// update-domain 增量修改接口无需 product_code，body 不应包含。
	if _, ok := gotBody["product_code"]; ok {
		t.Fatalf("icdn body 不应包含 product_code，实际=%v", gotBody["product_code"])
	}
	if gotBody["cert_name"] != certName {
		t.Fatalf("cert_name 应为备注名 %s，实际=%v", certName, gotBody["cert_name"])
	}
	if _, ok := gotBody["https_status"]; ok {
		t.Fatalf("body 不应包含 https_status，实际=%v", gotBody["https_status"])
	}
}

// TestCtyunCertName 验证备注名生成：同证书稳定、含合法字符，且证书内容变化（续期）即不同。
func TestCtyunCertName(t *testing.T) {
	cert := CertContent{Domain: "*.example.com", CertPEM: "PEM-A", KeyPEM: "KEY-A"}
	n1 := ctyunCertName(cert)
	n2 := ctyunCertName(cert)
	if n1 != n2 {
		t.Fatalf("同证书备注名应稳定: %q != %q", n1, n2)
	}
	if !strings.HasPrefix(n1, "certflow-wildcard-example-com-") {
		t.Fatalf("通配符域名备注名前缀错误: %q", n1)
	}
	// 证书内容变化（续期）应生成不同备注名，避免云端撞名。
	other := CertContent{Domain: "*.example.com", CertPEM: "PEM-B", KeyPEM: "KEY-B"}
	if ctyunCertName(other) == n1 {
		t.Fatal("不同证书内容应生成不同备注名（续期会撞名）")
	}
}
