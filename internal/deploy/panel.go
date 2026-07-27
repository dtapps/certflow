package deploy

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"cnb.cool/dtapp/certflow/internal/httplog"
	"cnb.cool/dtapp/certflow/internal/i18n"
)

// 面板厂商标识（与 ent/schema/provider_types.go 的 DeployProviderTypes 保持一致）
const (
	ProviderBTPanel  = "btpanel"
	ProviderAAPanel  = "aapanel"
	ProviderOnePanel = "1panel"
	ProviderAcePanel = "acepanel"
	ProviderAAWaf    = "aawaf"
)

// isPanelProvider 判断厂商标识是否为面板/防火墙类（凭证仅用 API Key，无 Secret）。
func isPanelProvider(p string) bool {
	switch p {
	case ProviderBTPanel, ProviderAAPanel, ProviderOnePanel, ProviderAcePanel, ProviderAAWaf:
		return true
	}
	return false
}

// md5Hex 返回小写的 md5 hex 字符串。
func md5Hex(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

// doV2Request 适用于 aaPanel 等使用 v2 接口的面板：
// 鉴权参数（如 request_time/request_token）作为 URL 查询参数，业务参数作为表单体。
// authFn 返回的 url.Values 视为查询参数（鉴权用），业务参数由 caller 通过 form 传入。
func (c *PanelClient) doV2Request(ctx context.Context, path string, query url.Values, form url.Values) ([]byte, error) {
	fullPath := c.PathPrefix + path
	h, authQuery, err := c.authFn(c, http.MethodPost, fullPath, query, []byte(form.Encode()))
	if err != nil {
		return nil, err
	}
	merged := url.Values{}
	for k, vs := range query {
		for _, v := range vs {
			merged.Add(k, v)
		}
	}
	for k, vs := range authQuery {
		for _, v := range vs {
			merged.Add(k, v)
		}
	}
	bodyStr := form.Encode()
	reqURL := c.BaseURL + fullPath
	if len(merged) > 0 {
		reqURL += "?" + merged.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, strings.NewReader(bodyStr))
	if err != nil {
		return nil, err
	}
	req.Header = h
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s", i18n.T("error.deploy_panel_request", "Err", err.Error()))
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s", i18n.T("error.deploy_panel_http_status", "Code", strconv.Itoa(resp.StatusCode)))
	}
	return body, nil
}

// hmacSHA256Hex 返回 HMAC-SHA256 hex 字符串。
func hmacSHA256Hex(key, data string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(data))
	return hex.EncodeToString(mac.Sum(nil))
}

// sha256Hex 返回字符串的 SHA256 hex 值（AcePanel 签名用于规范化请求哈希）。
func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// PanelClient 统一封装面板 HTTP 调用（请求发送 + 日志 + 超时）。
// 面板/防火墙类仅需 API Key（api_key），无需 AppSecret / Secret。
// 各面板鉴权签名算法不同，由 newPanelClient 的 authFn 注入——
// 签名实现分散在各面板文件（btpanel.go/aapanel.go/onepanel.go/acepanel.go/aawaf.go），
// panel.go 不持有任何具体签名逻辑。
// panelAuthFn 面板鉴权签名函数。参数：客户端、HTTP 方法、路径、查询参数、请求体原始字节。
// 返回：鉴权请求头、业务表单参数（POST 表单类面板用，其它面板返回空 url.Values）。
// 签名需要请求上下文的面板（如 AcePanel）可使用方法/路径/查询/请求体；
// 仅依赖客户端状态的面板（如宝塔/1Panel）可忽略后面四个参数。
type panelAuthFn func(c *PanelClient, method, path string, query url.Values, body []byte) (http.Header, url.Values, error)

type PanelClient struct {
	BaseURL    string // 面板地址（origin），如 https://1.2.3.4:8888
	PathPrefix string // URL 路径前缀（AcePanel 安全入口路径，如 /u5GJ9L），其它面板留空
	APIKey     string // 面板 API Key / Token（对应各面板 api_key 字段）
	APISecret  string // 面板 API Secret（AcePanel 等需要 HMAC 签名的面板使用，其它面板留空）
	HTTP       *http.Client
	authFn     panelAuthFn // 由各面板注入的签名函数
}

// defaultPanelHTTPClient 面板调用统一的全局 HTTP 客户端：包裹 httplog（仅 DEBUG 落请求日志），
// 并带 30s 超时。与现有云厂商部署器保持一致，不使用裸 http.DefaultClient。
// 注意：采用懒初始化（见 panelHTTPClient）——包级 var 在 import 时早于 httplog.Init()，
// 若那时包裹会因尚未进入 DEBUG 而返回未包裹日志的 transport，导致面板请求无日志。
// 首次真实请求时（DEBUG 已开启）才创建，确保能正确包裹。
var (
	defaultPanelHTTPClient *http.Client
	defaultPanelHTTPOnce   sync.Once
)

func panelHTTPClient() *http.Client {
	defaultPanelHTTPOnce.Do(func() {
		// 用独立的基础 transport（非 http.DefaultTransport），避免与 httplog.Init 对全局
		// DefaultTransport 的包裹叠加造成重复记录。
		defaultPanelHTTPClient = httplog.WrapClient(&http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
				// 面板/防火墙常用自签名证书，跳过 TLS 证书验证
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
				// 面板地址常位于隧道/反代入口（如 frp 类 CGNAT 入口）之后，
				// 这类入口在「复用 keep-alive 连接处理 PUT 等非 GET 方法」时
				// 偶发写出非法响应头（如缺冒号的 "Date" 行），导致 net/http
				// 严格解析器报 "malformed MIME header: missing colon"，
				// 表现为 "transport connection broken"。关闭长连接，每次请求
				// 新建 TCP 连接，可规避该入口的连接复用 bug（浏览器成功即印证
				// 服务端本身能返回合法响应，问题仅在复用连接）。
				DisableKeepAlives: true,
				// 同上，禁用自动 Accept-Encoding/gzip 解压，减少入口篡改
				// 压缩响应带来的变量。
				DisableCompression: true,
			},
			Timeout: 30 * time.Second,
		})
	})
	return defaultPanelHTTPClient
}

// normalizePanelBaseURL 将用户填写的面板地址规范化为 origin（scheme://host:port），
// 丢弃路径、查询串与锚点。宝塔等面板开启「安全入口」后，访问面板首页的 URL 形如
// http://ip:port/<随机入口>/，但面板 API（如 /mod/proxy/com/get_list）统一挂在 origin 下，
// 调用前必须把安全入口路径剥掉，否则会拼成 .../<入口>//mod/... 而 404。
// 其它面板地址本就不带路径，剥离无影响。
func normalizePanelBaseURL(raw string) string {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		// 无法解析时保守地只去尾部斜杠，交给后续请求报错
		return strings.TrimRight(raw, "/")
	}
	return u.Scheme + "://" + u.Host
}

// newPanelClient 创建面板客户端。httpClient 为 nil 时使用全局日志客户端 defaultPanelHTTPClient；
// authFn 由各面板文件提供，封装该面板自己的鉴权签名算法（见 doRequest）；调用方（含单测）可显式传入 httpClient 覆盖。
func newPanelClient(baseURL, apiKey, apiSecret string, httpClient *http.Client, authFn panelAuthFn) *PanelClient {
	return newPanelClientWithPath(baseURL, "", apiKey, apiSecret, httpClient, authFn)
}

// newPanelClientWithPath 创建面板客户端，pathPrefix 为 URL 路径前缀（AcePanel 安全入口路径）。
func newPanelClientWithPath(baseURL, pathPrefix, apiKey, apiSecret string, httpClient *http.Client, authFn panelAuthFn) *PanelClient {
	if httpClient == nil {
		httpClient = panelHTTPClient()
	}
	return &PanelClient{
		BaseURL:    normalizePanelBaseURL(baseURL),
		PathPrefix: pathPrefix,
		APIKey:     apiKey,
		APISecret:  apiSecret,
		HTTP:       httpClient,
		authFn:     authFn,
	}
}

// panelDeployerBase 面板/防火墙类部署器的公共骨架。
// 面板无独立证书库，证书直接写站点：UploadCert 为空缓存操作；
// DeployCert / ListSites 的具体接口待用户提供后由各面板文件覆盖实现，
// 当前返回「待实现」错误。
type panelDeployerBase struct {
	provider string
}

// doRequest 发送面板 API 请求：调用注入的 authFn 生成鉴权请求头与表单参数，返回原始响应体。
// path 为接口路径（如 /data）；form 为业务表单参数，鉴权参数（request_time 等）由 authFn 自动并入。
func (c *PanelClient) doRequest(ctx context.Context, path string, form url.Values) ([]byte, error) {
	fullPath := c.PathPrefix + path
	h, authForm, err := c.authFn(c, http.MethodPost, fullPath, nil, []byte(form.Encode()))
	if err != nil {
		return nil, err
	}
	for k, vs := range authForm {
		for _, v := range vs {
			form.Set(k, v)
		}
	}
	bodyStr := form.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+fullPath, strings.NewReader(bodyStr))
	if err != nil {
		return nil, err
	}
	req.Header = h
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s", i18n.T("error.deploy_panel_request", "Err", err.Error()))
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s", i18n.T("error.deploy_panel_http_status", "Code", strconv.Itoa(resp.StatusCode)))
	}
	return body, nil
}

// doJSONRequest 发送 JSON 请求体的面板 API 请求（1Panel v2 等接口要求 JSON 而非表单）。
// payload 可为 map 或任意结构体（序列化为 JSON 作为请求体）；鉴权请求头由 authFn 生成。
func (c *PanelClient) doJSONRequest(ctx context.Context, path string, payload any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("%s", i18n.T("error.deploy_panel_list_parse", "Err", err.Error()))
	}
	fullPath := c.PathPrefix + path
	h, _, err := c.authFn(c, http.MethodPost, fullPath, nil, body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+fullPath, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header = h
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s", i18n.T("error.deploy_panel_request", "Err", err.Error()))
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s", i18n.T("error.deploy_panel_http_status", "Code", strconv.Itoa(resp.StatusCode)))
	}
	return respBody, nil
}

// doPutJSONRequest 发送 JSON 请求体的 PUT 请求（AcePanel 等接口要求完整对象 PUT 替换）。
// payload 序列化为 JSON 作为请求体；鉴权请求头由 authFn 基于 PUT 方法与请求体哈希生成。
func (c *PanelClient) doPutJSONRequest(ctx context.Context, path string, payload map[string]any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("%s", i18n.T("error.deploy_panel_list_parse", "Err", err.Error()))
	}
	fullPath := c.PathPrefix + path
	h, _, err := c.authFn(c, http.MethodPut, fullPath, nil, body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.BaseURL+fullPath, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header = h
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s", i18n.T("error.deploy_panel_request", "Err", err.Error()))
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s", i18n.T("error.deploy_panel_http_status", "Code", strconv.Itoa(resp.StatusCode)))
	}
	return respBody, nil
}

// doGetRequest 发送 GET 请求（查询参数拼在 URL 上），鉴权请求头由 authFn 生成。
// 适用于 AcePanel 等列站点接口（如 /api/website?type=all&page=1&limit=20）。
func (c *PanelClient) doGetRequest(ctx context.Context, path string, query url.Values) ([]byte, error) {
	fullPath := c.PathPrefix + path
	h, _, err := c.authFn(c, http.MethodGet, fullPath, query, nil)
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(c.BaseURL + fullPath)
	if err != nil {
		return nil, err
	}
	u.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header = h
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s", i18n.T("error.deploy_panel_request", "Err", err.Error()))
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s", i18n.T("error.deploy_panel_http_status", "Code", strconv.Itoa(resp.StatusCode)))
	}
	return respBody, nil
}

// 注意：各面板「列站点」接口不同（如 1Panel/acePanel/aaWAF 与 btpanel 接口不一致），
// 列站点逻辑不在 panel.go 统一实现，由各面板文件（btpanel.go/aapanel.go/...）各自实现 ListSites。

func (d panelDeployerBase) Provider() string { return d.provider }

// UploadCert 面板无独立证书库，上传为空缓存操作；返回空 certID，DeployCert 直接使用证书内容。
func (d panelDeployerBase) UploadCert(_ context.Context, _ Credentials, _ CertContent, _ map[string]string) (string, string, error) {
	return "", "", nil
}

// DeployCert 待用户提供「设置证书到站点」接口后由各面板文件覆盖实现。
func (d panelDeployerBase) DeployCert(_ context.Context, _ Credentials, _ string, _ string, _ map[string]string) (*DeployResult, error) {
	return nil, i18n.NewError("error.deploy_panel_pending", "Provider", d.provider)
}

// ListDomains 是云厂商接口（列出 CDN 域名）；面板类部署器实现 ListSites（列出网站）。
// panelDeployerBase 保留 ListDomains 默认实现（返回「待实现」），以满足 Deployer 接口约束。
func (d panelDeployerBase) ListDomains(_ context.Context, _ Credentials, _, _, _ string) ([]string, error) {
	return nil, i18n.NewError("error.deploy_panel_pending", "Provider", d.provider)
}

// ListSites 待用户提供「列站点」接口后由各面板文件覆盖实现。
func (d panelDeployerBase) ListSites(_ context.Context, _ Credentials, _, _, _ string) ([]string, error) {
	return nil, i18n.NewError("error.deploy_panel_pending", "Provider", d.provider)
}
