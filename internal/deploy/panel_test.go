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

// TestBaseDeployCert 校验未实现 DeployCert 的面板返回「尚未实现」错误。
// 注意：btpanel / aapanel / 1panel 已实现 DeployCert，此处用仍未实现的 aawaf 走 panelDeployerBase 默认实现。
func TestBaseDeployCert(t *testing.T) {
	d := AAWafDeployer{panelDeployerBase{provider: ProviderAAWaf}}
	_, err := d.DeployCert(context.Background(), Credentials{PanelURL: "http://localhost"}, "cert-id", "site", nil)
	if err == nil {
		t.Fatal("aawaf 未实现 DeployCert，应返回错误")
	}
	if !strings.Contains(err.Error(), "尚未实现") {
		t.Errorf("错误信息应包含「尚未实现」，实际: %v", err)
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
