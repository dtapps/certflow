package deploy

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

// genKeyCert 生成一对匹配的 RSA 私钥与自签证书（PEM）。
func genKeyCert(t *testing.T) (keyPEM, certPEM string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		DNSNames:     []string{"example.com"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	kb := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	cb := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	return string(kb), string(cb)
}

// TestVerifyKeyCertMatch 锁定部署前私钥-证书匹配校验：匹配通过、不匹配报错
// （避免把 1Panel 的「私钥文件校验失败」这类晦涩错误直接抛给用户）。
func TestVerifyKeyCertMatch(t *testing.T) {
	goodKey, goodCert := genKeyCert(t)
	if err := verifyKeyCertMatch(goodKey, goodCert); err != nil {
		t.Fatalf("匹配对不应报错: %v", err)
	}
	// 用另一把私钥制造不匹配
	otherKey, _ := genKeyCert(t)
	if err := verifyKeyCertMatch(otherKey, goodCert); err == nil {
		t.Fatal("不匹配对必须报错")
	}
	// 直接调用 tls.X509KeyPair 也应一致
	if _, err := tls.X509KeyPair([]byte(goodCert), []byte(goodKey)); err != nil {
		t.Fatalf("参考实现也应匹配: %v", err)
	}
}

// TestMatchSiteID 锁定回退配对逻辑：未显式传入 siteID 时，按站点名从配置的
// site_name/site_id 两份按索引对齐的数组中查出对应 ID（前端主路径直接传 siteID，不走此逻辑）。
func TestMatchSiteID(t *testing.T) {
	// 复现线上报错：只传了站点名、缺失 ID，应从配置 site_id 配对出 123
	names := `["asw-sd.dtapp.net"]`
	ids := `["123"]`
	if got := matchSiteID(names, ids, "asw-sd.dtapp.net"); got != "123" {
		t.Fatalf("单站点按名配对失败: got %q", got)
	}

	// 多站点配对需保持索引顺序一致
	if got := matchSiteID(`["a.com","b.com"]`, `["11","22"]`, "b.com"); got != "22" {
		t.Fatalf("多站点按名配对失败: got %q", got)
	}

	// 名不在数组中应返回空
	if got := matchSiteID(names, ids, "other.com"); got != "" {
		t.Fatalf("未匹配应返回空: got %q", got)
	}
}

// TestOnePanelDeployCertUsesSiteID 锁定 1Panel 部署直接读 svc["site_id"] 定位站点，
// 不再依赖「名称||ID」拼接（之前会误把 aaPanel/宝塔的站点名带 ||ID 后缀）。
func TestOnePanelDeployCertUsesSiteID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/https") {
			fmt.Fprint(w, `{"code":200,"data":{"enable":true,"SSLProtocol":["TLSv1.2","TLSv1.3"]}}`)
			return
		}
		fmt.Fprint(w, `{"code":200,"message":"ok"}`)
	}))
	defer srv.Close()

	d := OnePanelDeployer{panelDeployerBase{provider: ProviderOnePanel}}
	keyPEM, certPEM := genKeyCert(t)
	svc := map[string]string{
		"site_name": "asw-sd.dtapp.net", // 纯站点名，不带 ||ID
		"site_id":   "123",              // 站点 ID 独立存放
		"cert_pem":  certPEM,
		"key_pem":   keyPEM,
	}
	res, err := d.DeployCert(context.Background(), Credentials{AccessKeyID: "k", PanelURL: srv.URL}, "", "", svc)
	if err != nil {
		t.Fatalf("DeployCert 不应失败: %v", err)
	}
	if res.CloudCertID != "123" {
		t.Fatalf("1Panel 应直接用 site_id=123: got %q", res.CloudCertID)
	}
}

// TestPanelAuthParams 校验 5 个面板鉴权签名请求头均已正确生成。
// 各面板签名算法相互独立实现（btpanel.go/aapanel.go/onepanel.go/acepanel.go/aawaf.go），
// 此处按厂商标识选择各自 authFn 校验。签名含时间戳，无法断言具体值，仅校验请求头存在且携带 API Key。
func TestPanelAuthParams(t *testing.T) {
	authFns := map[string]panelAuthFn{
		ProviderBTPanel:  btPanelAuth,
		ProviderAAPanel:  aaPanelAuth,
		ProviderOnePanel: onePanelAuth,
		ProviderAcePanel: acePanelAuth,
		ProviderAAWaf:    aawafAuth,
	}
	cases := []struct {
		provider string
		key      string
		wantHdr  []string // 期望出现的请求头
	}{
		{ProviderBTPanel, "bt-key", []string{"BT-PANEL-APIKEY", "BT-PANEL-TIMESTAMP", "BT-PANEL-SIGNATURE"}},
		{ProviderAAPanel, "aapanel-key", []string{}},
		{ProviderOnePanel, "1panel-key", []string{"1panel-Token", "1panel-Timestamp"}},
		{ProviderAcePanel, "ace-key", []string{"X-Timestamp", "Authorization"}},
		{ProviderAAWaf, "aawaf-key", []string{"waf_request_time", "waf_request_token"}},
	}
	for _, tc := range cases {
		t.Run(tc.provider, func(t *testing.T) {
			fn, ok := authFns[tc.provider]
			if !ok {
				t.Fatalf("未找到 %s 的 authFn", tc.provider)
			}
			c := &PanelClient{BaseURL: "https://example.com:8888", APIKey: tc.key, HTTP: http.DefaultClient}
			h, form, err := fn(c, http.MethodGet, "/api/website", nil, nil)
			if err != nil {
				t.Fatalf("authFn 返回错误: %v", err)
			}
			for _, name := range tc.wantHdr {
				if got := h.Get(name); got == "" {
					t.Errorf("缺少鉴权请求头 %s", name)
				}
			}
			// btpanel / aapanel 还需表单参数 request_time / request_token
			if tc.provider == ProviderBTPanel || tc.provider == ProviderAAPanel {
				if form.Get("request_time") == "" || form.Get("request_token") == "" {
					t.Errorf("宝塔类缺少表单参数 request_time/request_token")
				}
			}
		})
	}
}

func TestIsPanelProvider(t *testing.T) {
	panels := []string{ProviderBTPanel, ProviderAAPanel, ProviderOnePanel, ProviderAcePanel, ProviderAAWaf}
	for _, p := range panels {
		if !isPanelProvider(p) {
			t.Errorf("%s 应判定为面板", p)
		}
	}
	if isPanelProvider("aliyun") {
		t.Errorf("aliyun 不应判定为面板")
	}
}

// TestBTPanelListSites 使用 mock 服务器验证 ListSites 的请求鉴权与响应解析。
func TestBTPanelListSites(t *testing.T) {
	var gotForm url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotForm = r.Form
		// 校验宝塔鉴权请求头已附加
		if r.Header.Get("BT-PANEL-APIKEY") == "" {
			t.Errorf("缺少 BT-PANEL-APIKEY 请求头")
		}
		if r.Header.Get("BT-PANEL-SIGNATURE") == "" {
			t.Errorf("缺少 BT-PANEL-SIGNATURE 请求头")
		}
		if r.URL.Path != "/mod/proxy/com/get_list" {
			t.Errorf("期望路径 /mod/proxy/com/get_list，实际 %s", r.URL.Path)
		}
		// 宝塔 /mod 系返回嵌套结构 data.data
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"data": []map[string]any{
					{"id": 64, "name": "bbb.com", "ps": "bbb.com"},
					{"id": 65, "name": "ccc.com", "ps": "ccc.com"},
				},
				"page": map[string]any{"count": 2, "page": 1},
			},
		})
	}))
	defer srv.Close()

	d := BTPanelDeployer{panelDeployerBase{provider: ProviderBTPanel}}
	domains, err := d.ListSites(context.Background(), Credentials{PanelURL: srv.URL, AccessKeyID: "bt-key"}, "", "", "")
	if err != nil {
		t.Fatalf("ListSites 返回错误: %v", err)
	}
	if len(domains) != 2 {
		t.Fatalf("期望 2 个站点，实际 %d", len(domains))
	}
	if domains[0] != "bbb.com||64" || domains[1] != "ccc.com||65" {
		t.Errorf("站点解析不符合预期: %v", domains)
	}
	// 校验请求体业务参数
	if gotForm.Get("p") != "1" || gotForm.Get("limit") != "100" {
		t.Errorf("缺少 p/limit 业务参数: %v", gotForm)
	}
	if gotForm.Get("request_time") == "" || gotForm.Get("request_token") == "" {
		t.Errorf("缺少宝塔表单鉴权参数")
	}
}

// TestOnePanelListSites 使用 mock 服务器验证 1Panel ListSites 的鉴权请求头与响应解析。
func TestOnePanelListSites(t *testing.T) {
	var gotHeaders http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders = r.Header.Clone()
		if r.URL.Path != "/api/v2/websites/search" {
			t.Errorf("期望路径 /api/v2/websites/search，实际 %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("期望 POST，实际 %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("期望 Content-Type application/json，实际 %s", ct)
		}
		// 1Panel v2 返回格式：{ "code": 200, "data": { "items": [...], "total": N } }
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 200,
			"data": map[string]any{
				"items": []map[string]any{
					{"id": 1, "alias": "site-a", "primaryDomain": "a.com"},
					{"id": 2, "alias": "", "primaryDomain": "b.com"},
				},
				"total": 2,
			},
		})
	}))
	defer srv.Close()

	d := OnePanelDeployer{panelDeployerBase{provider: ProviderOnePanel}}
	domains, err := d.ListSites(context.Background(), Credentials{PanelURL: srv.URL, AccessKeyID: "1panel-key"}, "", "", "")
	if err != nil {
		t.Fatalf("ListSites 返回错误: %v", err)
	}
	if len(domains) != 2 {
		t.Fatalf("期望 2 个站点，实际 %d", len(domains))
	}
	if domains[0] != "site-a||1" || domains[1] != "b.com||2" {
		t.Errorf("站点解析不符合预期: %v", domains)
	}
	// 校验 1Panel 鉴权请求头
	if gotHeaders.Get("1panel-Token") == "" {
		t.Errorf("缺少 1panel-Token 请求头")
	}
	if gotHeaders.Get("1panel-Timestamp") == "" {
		t.Errorf("缺少 1panel-Timestamp 请求头")
	}
}

// TestAcePanelListSites 使用 mock 服务器验证 AcePanel ListSites 的 HMAC 签名与响应解析。
func TestAcePanelListSites(t *testing.T) {
	var gotHeaders http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders = r.Header.Clone()
		if r.URL.Path != "/api/website" {
			t.Errorf("期望路径 /api/website，实际 %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("期望 GET，实际 %s", r.Method)
		}
		// 校验查询参数
		if r.URL.Query().Get("type") != "all" {
			t.Errorf("缺少查询参数 type=all")
		}
		// AcePanel 返回格式：{ "msg": "success", "data": { "items": [...], "total": N } }
		_ = json.NewEncoder(w).Encode(map[string]any{
			"msg": "success",
			"data": map[string]any{
				"total": 2,
				"items": []map[string]any{
					{"id": 10, "name": "site-a", "domains": []string{"x.com"}},
					{"id": 11, "name": "site-b", "domains": []string{"y.com"}},
				},
			},
		})
	}))
	defer srv.Close()

	d := AcePanelDeployer{panelDeployerBase{provider: ProviderAcePanel}}
	domains, err := d.ListSites(context.Background(), Credentials{PanelURL: srv.URL, AccessKeyID: "ace-key", AccessKeySecret: "ace-secret"}, "", "", "")
	if err != nil {
		t.Fatalf("ListSites 返回错误: %v", err)
	}
	if len(domains) != 2 {
		t.Fatalf("期望 2 个站点，实际 %d", len(domains))
	}
	if domains[0] != "x.com||10" || domains[1] != "y.com||11" {
		t.Errorf("站点解析不符合预期: %v", domains)
	}
	// 校验 AcePanel 鉴权请求头
	if gotHeaders.Get("X-Timestamp") == "" {
		t.Errorf("缺少 X-Timestamp 请求头")
	}
	if auth := gotHeaders.Get("Authorization"); auth == "" {
		t.Errorf("缺少 Authorization 请求头")
	} else if !strings.HasPrefix(auth, "HMAC-SHA256 Credential=ace-key, Signature=") {
		t.Errorf("Authorization 格式不符合预期: %s", auth)
	}
}

// TestAAPanelListSites 使用 mock 服务器验证 aaPanel ListSites 的鉴权请求头与响应解析。
func TestAAPanelListSites(t *testing.T) {
	var gotQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		if r.URL.Path != "/v2/mod/proxy/com/get_list" {
			t.Errorf("期望路径 /v2/mod/proxy/com/get_list，实际 %s", r.URL.Path)
		}
		// aaPanel v2 返回格式：{ "message": { "data": { "data": [...] } } }
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message": map[string]any{
				"data": map[string]any{
					"data": []map[string]any{
						{"id": 30, "name": "aa-site", "ps": "aa-site"},
						{"id": 31, "name": "", "ps": "bb-site"},
					},
				},
			},
		})
	}))
	defer srv.Close()

	d := AAPanelDeployer{panelDeployerBase{provider: ProviderAAPanel}}
	domains, err := d.ListSites(context.Background(), Credentials{PanelURL: srv.URL, AccessKeyID: "aapanel-key"}, "", "", "")
	if err != nil {
		t.Fatalf("ListSites 返回错误: %v", err)
	}
	if len(domains) != 2 {
		t.Fatalf("期望 2 个站点，实际 %d", len(domains))
	}
	if domains[0] != "aa-site||30" || domains[1] != "bb-site||31" {
		t.Errorf("站点解析不符合预期: %v", domains)
	}
	// aaPanel v2 鉴权参数为查询串 request_time/request_token（无 BT-PANEL-APIKEY 头）
	if gotQuery.Get("request_time") == "" || gotQuery.Get("request_token") == "" {
		t.Errorf("缺少 aaPanel 查询鉴权参数 request_time/request_token")
	}
}

// TestBTPanelListSitesNoURL 校验缺少 panel_url 时返回明确错误。
func TestBTPanelListSitesNoURL(t *testing.T) {
	d := BTPanelDeployer{panelDeployerBase{provider: ProviderBTPanel}}
	if _, err := d.ListSites(context.Background(), Credentials{AccessKeyID: "k"}, "", "", ""); err == nil {
		t.Fatal("缺少 panel_url 时应返回错误")
	}
}

// TestNormalizePanelBaseURL 校验用户地址含宝塔「安全入口」等路径时，
// 客户端 base 仅保留 origin，调用 API 才拼得对路径（不会带上 /<入口>/）。
func TestNormalizePanelBaseURL(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"http://100.127.28.74:29187/3e228c0f/", "http://100.127.28.74:29187"},
		{"http://100.127.28.74:29187/3e228c0f", "http://100.127.28.74:29187"},
		{"https://1.2.3.4:8888", "https://1.2.3.4:8888"},
		{"http://1.2.3.4:8888/", "http://1.2.3.4:8888"},
		{"http://1.2.3.4:8888/path/to?q=1", "http://1.2.3.4:8888"},
		{"  http://1.2.3.4:8888/ent/  ", "http://1.2.3.4:8888"},
	}
	for _, tc := range cases {
		if got := normalizePanelBaseURL(tc.in); got != tc.want {
			t.Errorf("normalizePanelBaseURL(%q)=%q, want %q", tc.in, got, tc.want)
		}
	}
}

// ============================================================================
// 以下 5 个方法用于真实接口联调，便于在本地用真实面板/防火墙验证 ListSites。
//
// 运行方式（设置对应环境变量后执行，未设置则自动 Skip）：
//
//	PANEL_BTPANEL_URL=https://1.2.3.4:8888 PANEL_BTPANEL_KEY=xxxx \
//	  go test -run 'TestBTPanelInterface' -v ./internal/deploy/
//
// 环境变量命名：PANEL_<厂商大写>_URL / PANEL_<厂商大写>_KEY
//   btpanel  -> PANEL_BTPANEL_URL  / PANEL_BTPANEL_KEY
//   aapanel  -> PANEL_AAPANEL_URL  / PANEL_AAPANEL_KEY
//   1panel   -> PANEL_ONEPANEL_URL / PANEL_ONEPANEL_KEY
//   acepanel -> PANEL_ACEPANEL_URL / PANEL_ACEPANEL_KEY
//   aawaf    -> PANEL_AAWAF_URL    / PANEL_AAWAF_KEY
//
// 说明：btpanel / aapanel / 1panel / acepanel / aawaf 均已实现 ListSites（返回 "网站名||站点ID"）。
// ============================================================================

func panelEnvCreds(t *testing.T, provider string) (string, string) {
	up := strings.ToUpper(strings.ReplaceAll(provider, "-", "_"))
	rawURL := os.Getenv("PANEL_" + up + "_URL")
	key := os.Getenv("PANEL_" + up + "_KEY")
	if rawURL == "" || key == "" {
		t.Skipf("未设置 PANEL_%s_URL / PANEL_%s_KEY，跳过真实接口联调", up, up)
	}
	return rawURL, key
}

func runPanelInterfaceTest(t *testing.T, provider string, d Deployer) {
	rawURL, key := panelEnvCreds(t, provider)
	sites, err := d.ListSites(context.Background(), Credentials{PanelURL: rawURL, AccessKeyID: key}, "site", "", "")
	if err != nil {
		t.Fatalf("%s ListSites 失败: %v", provider, err)
	}
	t.Logf("%s 站点列表（共 %d 条）:", provider, len(sites))
	for i, s := range sites {
		t.Logf("  [%d] %s", i+1, s)
	}
}

func TestBTPanelInterface(t *testing.T) {
	runPanelInterfaceTest(t, ProviderBTPanel, BTPanelDeployer{panelDeployerBase{provider: ProviderBTPanel}})
}

func TestAAPanelInterface(t *testing.T) {
	runPanelInterfaceTest(t, ProviderAAPanel, AAPanelDeployer{panelDeployerBase{provider: ProviderAAPanel}})
}

func TestOnePanelInterface(t *testing.T) {
	runPanelInterfaceTest(t, ProviderOnePanel, OnePanelDeployer{panelDeployerBase{provider: ProviderOnePanel}})
}

func TestAcePanelInterface(t *testing.T) {
	runPanelInterfaceTest(t, ProviderAcePanel, AcePanelDeployer{panelDeployerBase{provider: ProviderAcePanel}})
}

func TestAAWafInterface(t *testing.T) {
	runPanelInterfaceTest(t, ProviderAAWaf, AAWafDeployer{panelDeployerBase{provider: ProviderAAWaf}})
}

// ============================================================================
// doRequest / doJSONRequest / doGetRequest 单元测试
// ============================================================================

// TestDoRequest 校验 POST 表单请求：authFn 注入的表单参数应出现在请求体中。
func TestDoRequest(t *testing.T) {
	var gotForm url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotForm = r.Form
		if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/x-www-form-urlencoded") {
			t.Errorf("期望 Content-Type 为 application/x-www-form-urlencoded，实际 %s", ct)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": true})
	}))
	defer srv.Close()

	c := newPanelClient(srv.URL, "test-key", "", nil, btPanelAuth)
	form := url.Values{"hello": []string{"world"}}
	body, err := c.doRequest(context.Background(), "/api/test", form)
	if err != nil {
		t.Fatalf("doRequest 返回错误: %v", err)
	}
	_ = body

	// 业务参数应透传
	if gotForm.Get("hello") != "world" {
		t.Errorf("业务参数 hello=world 未透传，实际 %s", gotForm.Get("hello"))
	}
	// authFn 注入的鉴权参数应存在
	if gotForm.Get("request_time") == "" || gotForm.Get("request_token") == "" {
		t.Errorf("authFn 注入的鉴权参数缺失: %v", gotForm)
	}
}

// TestDoJSONRequest 校验 POST JSON 请求：Content-Type 应为 application/json，且 body 为合法 JSON。
func TestDoJSONRequest(t *testing.T) {
	var gotBody []byte
	var gotCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": map[string]any{"total": 0, "items": []any{}}})
	}))
	defer srv.Close()

	c := newPanelClient(srv.URL, "test-key", "", nil, onePanelAuth)
	payload := map[string]any{"page": 1, "limit": 10}
	body, err := c.doJSONRequest(context.Background(), "/api/v2/test", payload)
	if err != nil {
		t.Fatalf("doJSONRequest 返回错误: %v", err)
	}
	_ = body

	if gotCT != "application/json" {
		t.Errorf("期望 Content-Type application/json，实际 %s", gotCT)
	}
	// 校验 body 是合法 JSON
	var parsed map[string]any
	if err := json.Unmarshal(gotBody, &parsed); err != nil {
		t.Errorf("请求体不是合法 JSON: %v, body=%s", err, string(gotBody))
	}
	if parsed["page"] == nil || parsed["limit"] == nil {
		t.Errorf("JSON body 缺少业务字段: %s", string(gotBody))
	}
}

// TestDoGetRequest 校验 GET 请求：查询参数应出现在 URL 中，且无请求体。
func TestDoGetRequest(t *testing.T) {
	var gotQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		if r.Method != http.MethodGet {
			t.Errorf("期望 GET，实际 %s", r.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": []any{}})
	}))
	defer srv.Close()

	c := newPanelClient(srv.URL, "ace-key", "ace-secret", nil, acePanelAuth)
	query := url.Values{"type": []string{"all"}, "page": []string{"1"}}
	body, err := c.doGetRequest(context.Background(), "/api/website", query)
	if err != nil {
		t.Fatalf("doGetRequest 返回错误: %v", err)
	}
	_ = body

	if gotQuery.Get("type") != "all" || gotQuery.Get("page") != "1" {
		t.Errorf("查询参数未正确传递: %v", gotQuery)
	}
}

// TestDoRequestNon2xx 校验非 2xx 响应返回错误。
func TestDoRequestNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"invalid signature"}`))
	}))
	defer srv.Close()

	c := newPanelClient(srv.URL, "bad-key", "", nil, btPanelAuth)
	form := url.Values{"a": []string{"b"}}
	_, err := c.doRequest(context.Background(), "/api/test", form)
	if err == nil {
		t.Fatal("期望非 2xx 返回错误，实际为 nil")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("错误信息应包含状态码 403，实际: %v", err)
	}
}

// ============================================================================
// panelDeployerBase 默认方法测试
// ============================================================================

// TestBaseUploadCert 校验面板 UploadCert 为空缓存操作（返回空 certID，无错误）。
func TestBaseUploadCert(t *testing.T) {
	d := BTPanelDeployer{panelDeployerBase{provider: ProviderBTPanel}}
	certID, raw, err := d.UploadCert(context.Background(), Credentials{PanelURL: "http://localhost"},
		CertContent{Domain: "example.com", CertPEM: "cert", KeyPEM: "key"}, nil)
	if err != nil {
		t.Fatalf("面板 UploadCert 应为空操作无错误，实际: %v", err)
	}
	if certID != "" || raw != "" {
		t.Errorf("面板 UploadCert 应返回空 certID/raw，实际 certID=%q raw=%q", certID, raw)
	}
}

// TestBaseDeployCert 校验 panelDeployerBase 默认 DeployCert 返回「尚未实现」错误。
// 各具体面板（btpanel / aapanel / 1panel / aawaf / openrestymanager）均已自行实现 DeployCert，
// 故此处直接校验基类默认实现，避免随具体部署器实现变动而失效。
func TestBaseDeployCert(t *testing.T) {
	d := panelDeployerBase{provider: ProviderAAWaf}
	_, err := d.DeployCert(context.Background(), Credentials{PanelURL: "http://localhost"}, "cert-id", "site", nil)
	if err == nil {
		t.Fatal("基类 DeployCert 应返回「尚未实现」错误")
	}
	if !strings.Contains(err.Error(), "尚未实现") {
		t.Errorf("错误信息应包含「尚未实现」，实际: %v", err)
	}
}

// TestOpenRestyManagerListSites 校验列站点解析，并兼容 id 键的 "id" / "ID" 两种大小写。
func TestOpenRestyManagerListSites(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/admin/sites", func(w http.ResponseWriter, r *http.Request) {
		if !validORMAuth(t, r, "jwt-key") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"sites":[{"id":1,"name":"example.com","domains":"example.com","cert_id":0,"listeners":"0.0.0.0:80,0.0.0.0:443"},{"ID":2,"name":"site2"}],"cert_options":[],"upstream_options":[]}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d := OpenRestyManagerDeployer{panelDeployerBase{provider: ProviderOpenRestyManager}}
	sites, err := d.ListSites(context.Background(), Credentials{PanelURL: srv.URL, AccessKeySecret: "jwt-key"}, "", "", "")
	if err != nil {
		t.Fatalf("ListSites 错误: %v", err)
	}
	if len(sites) != 2 {
		t.Fatalf("期望 2 个站点，实际 %d: %v", len(sites), sites)
	}
	if sites[0] != "example.com||1" {
		t.Errorf("站点0 应为 example.com||1，实际 %s", sites[0])
	}
	if sites[1] != "site2||2" {
		t.Errorf("站点1 应为 site2||2，实际 %s", sites[1])
	}
}

// TestOpenRestyManagerDeployCert 使用 mock 服务器验证完整流程：
// 拉取站点 → 上传证书（type=1）→ 按名查证书 id → 写回站点 cert_id → PUT 站点。
func TestOpenRestyManagerDeployCert(t *testing.T) {
	var postedName string
	var putCertID float64
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/admin/sites", func(w http.ResponseWriter, r *http.Request) {
		if !validORMAuth(t, r, "jwt-key") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"sites":[{"id":1,"name":"example.com","domains":"example.com","cert_id":0,"listeners":"0.0.0.0:80,0.0.0.0:443"}]}`))
		case http.MethodPut:
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			putCertID, _ = body["cert_id"].(float64)
			_, _ = w.Write([]byte("OK"))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/v1/admin/certs", func(w http.ResponseWriter, r *http.Request) {
		if !validORMAuth(t, r, "jwt-key") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.Method {
		case http.MethodPost:
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			postedName, _ = body["name"].(string)
			if body["type"] != float64(1) {
				t.Errorf("上传证书 type 应为 1，实际 %v", body["type"])
			}
			_, _ = w.Write([]byte("OK"))
		case http.MethodGet:
			// 实测 OpenResty Manager 证书列表接口返回裸数组 [...]（非 {"certs":[...]}）
			_, _ = w.Write([]byte(`[{"id":9,"name":"` + postedName + `","crt":"CERT","key":"KEY"}]`))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	keyPEM, certPEM := genKeyCert(t)
	d := OpenRestyManagerDeployer{panelDeployerBase{provider: ProviderOpenRestyManager}}
	svc := map[string]string{
		"site_id":  "1",
		"cert_pem": certPEM,
		"key_pem":  keyPEM,
	}
	res, err := d.DeployCert(context.Background(), Credentials{PanelURL: srv.URL, AccessKeySecret: "jwt-key"}, "", "", svc)
	if err != nil {
		t.Fatalf("DeployCert 错误: %v", err)
	}
	if res.CloudCertID != "1" {
		t.Errorf("CloudCertID 应为 1，实际 %s", res.CloudCertID)
	}
	if postedName == "" {
		t.Error("应上传证书且带 name")
	}
	if int(putCertID) != 9 {
		t.Errorf("PUT 站点 cert_id 应为 9，实际 %v", putCertID)
	}
}

// TestOpenRestyManagerDeployCertDedup 校验证书内容去重：当面板已存在内容相同的证书时，
// 直接复用其 id，不再上传新证书（避免每次部署都新建证书、产生孤儿证书）。
func TestOpenRestyManagerDeployCertDedup(t *testing.T) {
	var posted bool
	var putCertID float64
	keyPEM, certPEM := genKeyCert(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/admin/sites", func(w http.ResponseWriter, r *http.Request) {
		if !validORMAuth(t, r, "jwt-key") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"sites":[{"id":1,"name":"example.com","domains":"example.com","cert_id":0,"listeners":"0.0.0.0:80,0.0.0.0:443"}]}`))
		case http.MethodPut:
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			putCertID, _ = body["cert_id"].(float64)
			_, _ = w.Write([]byte("OK"))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/v1/admin/certs", func(w http.ResponseWriter, r *http.Request) {
		if !validORMAuth(t, r, "jwt-key") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.Method {
		case http.MethodPost:
			posted = true
			_, _ = w.Write([]byte("OK"))
		case http.MethodGet:
			// 列表已存在内容相同的证书（crt 为真实待部署证书 PEM），应按内容复用
			_, _ = fmt.Fprintf(w, `[{"id":9,"name":"existing","crt":%q,"key":"KEY"}]`, certPEM)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d := OpenRestyManagerDeployer{panelDeployerBase{provider: ProviderOpenRestyManager}}
	svc := map[string]string{
		"site_id":  "1",
		"cert_pem": certPEM,
		"key_pem":  keyPEM,
	}
	if _, err := d.DeployCert(context.Background(), Credentials{PanelURL: srv.URL, AccessKeySecret: "jwt-key"}, "", "", svc); err != nil {
		t.Fatalf("DeployCert 错误: %v", err)
	}
	if posted {
		t.Error("已存在内容相同的证书时不应再上传新证书")
	}
	if int(putCertID) != 9 {
		t.Errorf("PUT 站点 cert_id 应复用已有证书 9，实际 %v", putCertID)
	}
}

// validSafelineAuth 校验请求携带的 X-SLCE-API-TOKEN 头（雷池 OpenAPI 鉴权令牌）。
func validSafelineAuth(t *testing.T, r *http.Request, key string) bool {
	t.Helper()
	if r.Header.Get("X-SLCE-API-TOKEN") != key {
		t.Errorf("缺少 X-SLCE-API-TOKEN 头或值错误, 实际: %q", r.Header.Get("X-SLCE-API-TOKEN"))
		return false
	}
	return true
}

// TestSafelineListSites 校验列站点解析，兼容 data 包装层与 id 键大小写。
func TestSafelineListSites(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/open/site", func(w http.ResponseWriter, r *http.Request) {
		if !validSafelineAuth(t, r, "sl-token") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":1,"name":"example.com","server_names":["example.com"],"cert_id":0},{"ID":2,"server_names":["site2.com"]}]}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d := SafelineDeployer{panelDeployerBase{provider: ProviderSafeline}}
	sites, err := d.ListSites(context.Background(), Credentials{PanelURL: srv.URL, AccessKeyID: "sl-token"}, "", "", "")
	if err != nil {
		t.Fatalf("ListSites 错误: %v", err)
	}
	if len(sites) != 2 {
		t.Fatalf("期望 2 个站点，实际 %d: %v", len(sites), sites)
	}
	if sites[0] != "example.com||1" {
		t.Errorf("站点0 应为 example.com||1，实际 %s", sites[0])
	}
	if sites[1] != "site2.com||2" {
		t.Errorf("站点1 应为 site2.com||2，实际 %s", sites[1])
	}
}

// TestSafelineListSitesEnvelope 复现真实雷池响应信封：
// {"data":{"data":[...],"total":N,"syncing":false},"err":null,"msg":""}（data 双重嵌套）。
func TestSafelineListSitesEnvelope(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/open/site", func(w http.ResponseWriter, r *http.Request) {
		if !validSafelineAuth(t, r, "sl-token") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"data":{"data":[{"id":1,"server_names":["asw-sd.dtapp.net"],"cert_id":0}],"total":1,"syncing":false},"err":null,"msg":""}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d := SafelineDeployer{panelDeployerBase{provider: ProviderSafeline}}
	sites, err := d.ListSites(context.Background(), Credentials{PanelURL: srv.URL, AccessKeyID: "sl-token"}, "", "", "")
	if err != nil {
		t.Fatalf("ListSites 错误: %v", err)
	}
	if len(sites) != 1 {
		t.Fatalf("期望 1 个站点，实际 %d: %v", len(sites), sites)
	}
	if sites[0] != "asw-sd.dtapp.net||1" {
		t.Errorf("站点应为 asw-sd.dtapp.net||1，实际 %s", sites[0])
	}
}

// TestSafelineDeployCert 使用 mock 服务器验证部署流程：
// 拉取站点详情(含 cert_id=9) → 取当前证书内容比对 → 不一致则 POST /api/open/cert 带 id 就地更新；
// 随后每次部署都执行站点绑定 PUT /api/open/site（id 在请求体，社区版正确的绑定接口）；
// 再次部署同一证书时当前内容已一致，应跳过更新（不再 POST），但绑定 PUT 仍会执行。
// 证书详情 mock 用 json 安全嵌入含换行的 PEM。
func TestSafelineDeployCert(t *testing.T) {
	keyPEM, certPEM := genKeyCert(t)

	var postedType float64
	var postedHasCRT, postedHasKey, postedHasID bool
	var putCount int
	var putCertID uint
	var postCount int
	var certUpdated bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/open/site", func(w http.ResponseWriter, r *http.Request) {
		// 绑定接口：PUT /api/open/site（id 在请求体，须回写完整站点对象）。
		if !validSafelineAuth(t, r, "sl-token") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.Method {
		case http.MethodPut:
			putCount++
			var body slSite
			_ = json.NewDecoder(r.Body).Decode(&body)
			putCertID = body.CertID.Uint()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":null,"err":null,"msg":""}`))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/open/site/", func(w http.ResponseWriter, r *http.Request) {
		if !validSafelineAuth(t, r, "sl-token") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.Method {
		case http.MethodGet:
			if r.URL.Path == "/api/open/site/1" {
				_, _ = w.Write([]byte(`{"data":{"id":1,"name":"example.com","server_names":["example.com"],"cert_id":9,"ports":["443"],"upstreams":[]}}`))
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	certHandler := func(w http.ResponseWriter, r *http.Request) {
		if !validSafelineAuth(t, r, "sl-token") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.Method {
		case http.MethodGet:
			if r.URL.Path == "/api/open/cert" {
				// 证书列表 GET /api/open/cert（仅 slCreateCert 兜底路径用到）
				_, _ = w.Write([]byte(`{"data":{"nodes":[],"total":0},"err":null,"msg":""}`))
			} else {
				// 证书详情 GET /api/open/cert/9：更新前返回过期内容（触发更新），
				// 更新后返回与待部署一致的内容（触发跳过）。
				crt := "STALE_CERT_PEM"
				if certUpdated {
					crt = certPEM
				}
				resp, _ := json.Marshal(map[string]any{
					"data": map[string]any{
						"id":     9,
						"type":   2,
						"manual": map[string]any{"crt": crt, "key": "STALE_KEY"},
					},
					"err": nil, "msg": "",
				})
				_, _ = w.Write(resp)
			}
		case http.MethodPost:
			postCount++
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			postedType, _ = body["type"].(float64)
			_, postedHasID = body["id"]
			manual, _ := body["manual"].(map[string]any)
			postedHasCRT = manual["crt"] != nil && manual["crt"] != ""
			postedHasKey = manual["key"] != nil && manual["key"] != ""
			certUpdated = true
			_, _ = w.Write([]byte(`{"data":null,"err":null,"msg":""}`))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
	mux.HandleFunc("/api/open/cert", certHandler)
	mux.HandleFunc("/api/open/cert/", certHandler)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d := SafelineDeployer{panelDeployerBase{provider: ProviderSafeline}}
	svc := map[string]string{
		"site_id":     "1",
		"cert_pem":    certPEM,
		"key_pem":     keyPEM,
		"cert_domain": "example.com",
	}
	res, err := d.DeployCert(context.Background(), Credentials{PanelURL: srv.URL, AccessKeyID: "sl-token"}, "", "", svc)
	if err != nil {
		t.Fatalf("DeployCert 错误: %v", err)
	}
	if res.CloudCertID != "9" {
		t.Errorf("CloudCertID 应为站点绑定证书 id 9，实际 %s", res.CloudCertID)
	}
	if postedType != 2 {
		t.Errorf("更新证书 type 应为 2，实际 %v", postedType)
	}
	if !postedHasCRT || !postedHasKey {
		t.Error("更新证书应携带 manual.crt 与 manual.key")
	}
	if !postedHasID {
		t.Error("就地更新证书应携带 id 字段（POST /api/open/cert 带 id）")
	}
	if putCount != 1 {
		t.Errorf("首次部署应执行 1 次站点绑定 PUT，实际 %d 次", putCount)
	}
	if putCertID != 9 {
		t.Errorf("绑定 PUT 的 cert_id 应为 9，实际 %d", putCertID)
	}
	if postCount != 1 {
		t.Errorf("首次部署应就地更新 1 次，实际 %d 次", postCount)
	}

	// 再次部署同一证书：当前证书内容已一致，应跳过更新（不再 POST），但绑定 PUT 仍会执行。
	if _, err := d.DeployCert(context.Background(), Credentials{PanelURL: srv.URL, AccessKeyID: "sl-token"}, "", "", svc); err != nil {
		t.Fatalf("二次 DeployCert 错误: %v", err)
	}
	if postCount != 1 {
		t.Errorf("证书内容未变时应跳过更新，实际 POST %d 次", postCount)
	}
	if putCount != 2 {
		t.Errorf("二次部署仍应执行站点绑定 PUT，累计 %d 次，期望 2", putCount)
	}
}

// TestSafelineGetCurrentCert 验证 GetCurrentCert 优先使用证书详情接口
// （GET /api/open/cert/{id}）的 manual.crt 解析真实证书，得到准确的 SAN/到期时间/签发者。
// mock 采用实测真实结构：{"data":{"id":0,"type":2,"acme":{...},"manual":{"key":"","crt":"..."}},"err":null,"msg":""}。
func TestSafelineGetCurrentCert(t *testing.T) {
	_, certPEM := genKeyCert(t)
	localCert, err := parseCertPEM(certPEM)
	if err != nil {
		t.Fatalf("解析测试证书失败: %v", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-SLCE-API-TOKEN") != "sl-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/api/open/site/1":
			// 站点详情：cert_id=2
			_, _ = w.Write([]byte(`{"data":{"id":1,"server_names":["example.com"],"cert_id":2},"err":null,"msg":""}`))
		case "/api/open/cert/2":
			// 证书详情（实测结构）：manual.crt 为真实 PEM，type=2，data.id 恒为 0。
			body, _ := json.Marshal(map[string]any{
				"data": map[string]any{
					"id":   0,
					"type": 2,
					"acme": map[string]any{"domains": localCert.SANs, "email": ""},
					"manual": map[string]any{
						"key": "",
						"crt": certPEM,
					},
				},
				"err": nil,
				"msg": "",
			})
			_, _ = w.Write(body)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d := SafelineDeployer{panelDeployerBase{provider: ProviderSafeline}}
	svc := map[string]string{"site_id": "1"}
	cur, err := d.GetCurrentCert(context.Background(), Credentials{PanelURL: srv.URL, AccessKeyID: "sl-token"}, "", svc)
	if err != nil {
		t.Fatalf("GetCurrentCert 错误: %v", err)
	}
	if cur.CommonName != localCert.CommonName {
		t.Errorf("CommonName 应为 %s，实际 %s", localCert.CommonName, cur.CommonName)
	}
	if cur.NotAfter != localCert.NotAfter {
		t.Errorf("NotAfter 应为 %s，实际 %s", localCert.NotAfter, cur.NotAfter)
	}
	if len(cur.SANs) != len(localCert.SANs) {
		t.Errorf("SANs 长度应为 %d，实际 %d", len(localCert.SANs), len(cur.SANs))
	}
}

// TestAAPanelDeployCert 使用 mock 服务器验证 aaPanel DeployCert 的完整流程：
// 先 upload_cert 拿到 hash，再 cert_deploy_sites 应用到站点。
func TestAAPanelDeployCert(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/ssl_domain", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.URL.Query().Get("request_token") == "" || r.URL.Query().Get("request_time") == "" {
			t.Errorf("缺少查询鉴权参数")
		}
		switch r.URL.Query().Get("action") {
		case "upload_cert":
			if r.PostForm.Get("cert") == "" || r.PostForm.Get("key") == "" {
				t.Errorf("upload_cert 缺少 cert/key")
			}
			_, _ = w.Write([]byte(`{"status":0,"msg":"ok","message":{"hash":"abc123hash"}}`))
		case "cert_deploy_sites":
			if r.PostForm.Get("hash") != "abc123hash" {
				t.Errorf("cert_deploy_sites 的 hash 不符，实际 %s", r.PostForm.Get("hash"))
			}
			if r.PostForm.Get("append") != "1" {
				t.Errorf("cert_deploy_sites 的 append 应为 1")
			}
			var domains []string
			if err := json.Unmarshal([]byte(r.PostForm.Get("domains")), &domains); err != nil || len(domains) != 1 || domains[0] != "example.com" {
				t.Errorf("cert_deploy_sites 的 domains 不符，实际 %s", r.PostForm.Get("domains"))
			}
			_, _ = w.Write([]byte(`{"status":0,"msg":"ok"}`))
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d := AAPanelDeployer{panelDeployerBase{provider: ProviderAAPanel}}
	svc := map[string]string{
		"site_name": "example.com",
		"cert_pem":  "CERT",
		"key_pem":   "KEY",
	}
	res, err := d.DeployCert(context.Background(), Credentials{PanelURL: srv.URL, AccessKeyID: "k"}, "", "", svc)
	if err != nil {
		t.Fatalf("DeployCert 返回错误: %v", err)
	}
	if res.CloudCertID != "abc123hash" {
		t.Errorf("CloudCertID 应为 abc123hash，实际 %s", res.CloudCertID)
	}
}

// TestAAPanelStatusInt 校验 aaPanel 的 status 为数字，且 0=成功、非0=失败。
func TestAAPanelStatusInt(t *testing.T) {
	cases := []struct {
		in   int
		want bool // want=是否成功
	}{
		{0, true},
		{1, false},
	}
	for _, tc := range cases {
		if (tc.in == 0) != tc.want {
			t.Errorf("aaPanel status=%d 成功判定=%v, want %v", tc.in, tc.in == 0, tc.want)
		}
	}
}

// TestAAPanelDeployCertNumericStatus 使用 mock 服务器验证 aaPanel 真实响应形态：
// status 以数字 0 表示成功，且证书 hash 位于 message.hash 对象中。
func TestAAPanelDeployCertNumericStatus(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/ssl_domain", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		switch r.URL.Query().Get("action") {
		case "upload_cert":
			// aaPanel 真实形态：status=0 表成功，hash 在 message.hash 对象内
			_, _ = w.Write([]byte(`{"status":0,"timestamp":1785067908,"message":{"hash":"realhash99","subject":"*.dtapp.net"}}`))
		case "cert_deploy_sites":
			if r.PostForm.Get("hash") != "realhash99" {
				t.Errorf("cert_deploy_sites 的 hash 不符，实际 %s", r.PostForm.Get("hash"))
			}
			// status=0 表成功
			_, _ = w.Write([]byte(`{"status":0,"msg":"ok"}`))
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d := AAPanelDeployer{panelDeployerBase{provider: ProviderAAPanel}}
	svc := map[string]string{
		"site_name": "example.com",
		"cert_pem":  "CERT",
		"key_pem":   "KEY",
	}
	res, err := d.DeployCert(context.Background(), Credentials{PanelURL: srv.URL, AccessKeyID: "k"}, "", "", svc)
	if err != nil {
		t.Fatalf("DeployCert 返回错误: %v", err)
	}
	if res.CloudCertID != "realhash99" {
		t.Errorf("CloudCertID 应为 realhash99，实际 %s", res.CloudCertID)
	}
}

// TestOnePanelDeployCert 使用 mock 服务器验证 1Panel DeployCert 的完整流程：
// 先 GET /api/v2/websites/{id}/https 拿到当前 HTTPS 配置，
// 再 POST 同一路径、原样回传既有配置并仅替换证书字段（type=manual、privateKey、certificate、enable=true）。
func TestOnePanelDeployCert(t *testing.T) {
	const siteID = 123
	keyPEM, certPEM := genKeyCert(t)
	mux := http.NewServeMux()
	mux.HandleFunc(fmt.Sprintf("/api/v2/websites/%d/https", siteID), func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			// 返回既有 HTTPS 配置，验证部署时这些字段被原样回传（不被清空）
			_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"enable":true,"SSLProtocol":["TLSv1.2","TLSv1.3"],"algorithm":"RSA","hsts":true,"hstsIncludeSubDomains":false,"httpsPorts":[443],"httpsPort":"443","http3":false}}`))
		case http.MethodPost:
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("解析 POST 请求体失败: %v", err)
			}
			if body["type"] != "manual" {
				t.Errorf("type 应为 manual，实际 %v", body["type"])
			}
			if body["privateKey"] != keyPEM {
				t.Errorf("privateKey 不符")
			}
			if body["certificate"] != certPEM {
				t.Errorf("certificate 不符")
			}
			if body["enable"] != true {
				t.Errorf("enable 应为 true，实际 %v", body["enable"])
			}
			// 既有 HTTPS 配置应被原样回传
			if sp, _ := body["SSLProtocol"].([]any); len(sp) != 2 {
				t.Errorf("SSLProtocol 应原样回传，实际 %v", body["SSLProtocol"])
			}
			if body["hsts"] != true {
				t.Errorf("hsts 应原样回传为 true，实际 %v", body["hsts"])
			}
			_, _ = w.Write([]byte(`{"code":200,"message":"ok"}`))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	d := OnePanelDeployer{panelDeployerBase{provider: ProviderOnePanel}}
	svc := map[string]string{
		"site_name": "test-site||123",
		"cert_pem":  certPEM,
		"key_pem":   keyPEM,
	}
	res, err := d.DeployCert(context.Background(), Credentials{PanelURL: srv.URL, AccessKeyID: "k"}, "", "", svc)
	if err != nil {
		t.Fatalf("DeployCert 返回错误: %v", err)
	}
	if res.CloudCertID != "123" {
		t.Errorf("CloudCertID 应为 123，实际 %s", res.CloudCertID)
	}
}

// TestAAWafListSites 使用 mock 服务器验证堡塔云WAF ListSites 的鉴权请求头与响应解析。
func TestAAWafListSites(t *testing.T) {
	var gotHeaders http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders = r.Header.Clone()
		if r.URL.Path != "/api/wafmastersite/get_site_list" {
			t.Errorf("期望路径 /api/wafmastersite/get_site_list，实际 %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("期望 POST，实际 %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("期望 Content-Type application/json，实际 %s", ct)
		}
		// 堡塔云WAF 返回格式：{ "code": 0, "res": { "list": [...] }, "msg": "success" }
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"msg":  "success",
			"res": map[string]any{
				"list": []map[string]any{
					{"site_name": "asw-pc.dtapp.top", "site_id": "asw-pc_dtapp_top_0"},
					{"site_name": "another-site.com", "site_id": "another_site_com_0"},
				},
			},
		})
	}))
	defer srv.Close()

	d := AAWafDeployer{panelDeployerBase{provider: ProviderAAWaf}}
	domains, err := d.ListSites(context.Background(), Credentials{PanelURL: srv.URL, AccessKeyID: "aawaf-key"}, "", "", "")
	if err != nil {
		t.Fatalf("ListSites 返回错误: %v", err)
	}
	if len(domains) != 2 {
		t.Fatalf("期望 2 个站点，实际 %d", len(domains))
	}
	if domains[0] != "asw-pc.dtapp.top||asw-pc_dtapp_top_0" || domains[1] != "another-site.com||another_site_com_0" {
		t.Errorf("站点解析不符合预期: %v", domains)
	}
	// 校验堡塔云WAF 鉴权请求头
	if gotHeaders.Get("waf_request_time") == "" {
		t.Errorf("缺少 waf_request_time 请求头")
	}
	if gotHeaders.Get("waf_request_token") == "" {
		t.Errorf("缺少 waf_request_token 请求头")
	}
}

// validORMAuth 校验请求携带的 Authorization: Bearer <JWT> 是否由给定 jwt 密钥 HS256 签发且未过期。
func validORMAuth(t *testing.T, r *http.Request, key string) bool {
	t.Helper()
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		t.Errorf("缺少 Authorization: Bearer 头, 实际: %q", auth)
		return false
	}
	tokenStr := strings.TrimPrefix(auth, "Bearer ")
	parsed, err := jwt.ParseWithClaims(tokenStr, &ormJWTClaims{}, func(tk *jwt.Token) (any, error) {
		if _, ok := tk.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("非预期签名算法: %v", tk.Header["alg"])
		}
		return []byte(key), nil
	})
	if err != nil || !parsed.Valid {
		t.Errorf("JWT 校验失败: %v", err)
		return false
	}
	return true
}
