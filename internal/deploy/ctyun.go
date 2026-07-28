package deploy

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
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
	"cnb.cool/dtapp/certflow/internal/logging"
	"github.com/google/uuid"
)

// CtyunDeployer 天翼云部署器：支持天翼云 CDN/全站加速/边缘安全加速平台证书部署
type CtyunDeployer struct{}

func init() { RegisterDeployer(&CtyunDeployer{}) }

func (d *CtyunDeployer) Provider() string { return "ctyun" }

// ctyunServiceConfig 天翼云服务配置
type ctyunServiceConfig struct {
	BaseURL       string // API 基础 URL
	CertPath      string // 证书创建路径
	DomainPath    string // 域名配置路径
	ListPath      string // 域名列表路径
	QueryCertPath string // 查询域名证书详情路径（为空表示该服务暂不支持按域名查证书）
}

// ctyunServiceProductCode 各部署服务对应的产品编码，用于按产品过滤域名列表。
// 取自天翼云域名列表 returnObj.result[].product_code：
//
//	006 = 全站加速（icdn），008 = CDN加速（ctcdn），020 = 边缘安全与加速服务（accessone）。
//
// 天翼云域名列表接口返回账号下全部产品的域名，不会按所选服务自动过滤，
// 因此列表需在客户端按 product_code 筛出当前服务对应的产品域名。
// 该映射独立于 ctyunServices，避免改动已确认的路径/方法配置。
var ctyunServiceProductCode = map[string]string{
	"ctcdn":     "008",
	"icdn":      "006",
	"accessone": "020",
}

// ctyunServices 天翼云服务配置表
var ctyunServices = map[string]ctyunServiceConfig{
	"ctcdn": {
		BaseURL:       "https://ctcdn-global.ctapi.ctyun.cn",
		CertPath:      "/v1/cert/creat-cert",               // 创建证书 POST https://eop.ctyun.cn/ebp/ctapiDocument/search?sid=108&api=10893&data=161&isNormal=1&vid=154
		DomainPath:    "/v1/domain/update-domain",          // 修改域名配置 POST https://eop.ctyun.cn/ebp/ctapiDocument/search?sid=108&api=11308&data=161&isNormal=1&vid=154
		ListPath:      "/v1/domain/query-domain-list",      // 查询域名列表 GET 域名列表在 returnObj.result[] https://eop.ctyun.cn/ebp/ctapiDocument/search?sid=108&api=11307&data=161&isNormal=1&vid=154
		QueryCertPath: "/v1/domain/query-domain-cert-info", // 查询指定加速域名证书详情 POST https://eop.ctyun.cn/ebp/ctapiDocument/search?sid=108&api=10906&data=161&isNormal=1&vid=154
	},
	"icdn": {
		BaseURL:       "https://icdn-global.ctapi.ctyun.cn",
		CertPath:      "/v1/cert/creat-cert",               // 创建证书 POST https://eop.ctyun.cn/ebp/ctapiDocument/search?sid=112&api=10835&data=173&isNormal=1&vid=166
		DomainPath:    "/v1/domain/update-domain",          // 增量修改域名配置 POST https://eop.ctyun.cn/ebp/ctapiDocument/search?sid=112&api=10853&data=173&isNormal=1&vid=166
		ListPath:      "/v1/domain/query-domain-list",      // 查询域名列表 GET 域名列表在 returnObj.result[] https://eop.ctyun.cn/ebp/ctapiDocument/search?sid=112&api=10852&data=173&isNormal=1&vid=166
		QueryCertPath: "/v1/domain/query-domain-cert-info", // 查询指定加速域名证书详情 POST https://eop.ctyun.cn/ebp/ctapiDocument/search?sid=112&api=10847&data=173&isNormal=1&vid=166
	},
	"accessone": {
		BaseURL:    "https://accessone-global.ctapi.ctyun.cn",
		CertPath:   "/ctapi/v1/accessone/cert/create",     // 创建证书 POST https://eop.ctyun.cn/ebp/ctapiDocument/search?sid=113&api=13014&data=174&isNormal=1&vid=167
		DomainPath: "/ctapi/v1/scdn/domain/modify_config", // 域名基础及加速配置修改 POST https://eop.ctyun.cn/ebp/ctapiDocument/search?sid=113&api=13413&data=174&isNormal=1&vid=167
		ListPath:   "/ctapi/v2/domain/query",              // 查询域名列表基础信息 GET 域名列表在 returnObj.result[] https://eop.ctyun.cn/ebp/ctapiDocument/search?sid=113&api=13816&data=174&isNormal=1&vid=167
	},
}

// ctyunServiceByKey 返回指定部署服务的配置；若服务名不在 ctyunServices 表中，
// 返回明确错误而非静默回退到 ctcdn，避免把证书误部署到错误的服务。
func ctyunServiceByKey(svc string) (ctyunServiceConfig, error) {
	cfg, ok := ctyunServices[svc]
	if !ok {
		return ctyunServiceConfig{}, i18n.NewError("deploy.error.ctyun_unknown_service", "Service", svc)
	}
	return cfg, nil
}

// intOrString 兼容天翼云 EOP 网关把数字字段（如证书 ID）返回成字符串的情况
// （有的网关返回 100000，有的返回 "100000"）。
// 通过实现 json.Unmarshaler，让 id 等字段在结构体里直接声明即可，不必用 json.RawMessage。
// 若值无法解析为整数（例如非预期的字符串），则静默当作 0，避免整段响应解析失败。
type intOrString int

func (n *intOrString) UnmarshalJSON(b []byte) error {
	s := bytes.TrimSpace(b)
	s = bytes.Trim(s, `"`)
	if len(s) == 0 || bytes.Equal(s, []byte("null")) {
		*n = 0
		return nil
	}
	// strconv.Atoi 解析失败时会返回 0，这里忽略错误直接采用返回值，
	// 等价于解析失败静默当作 0，避免整段响应解析失败。
	v, _ := strconv.Atoi(string(s))
	*n = intOrString(v)
	return nil
}

func (n intOrString) Int() int { return int(n) }

// ctyunStatusCode 天翼云 EOP 网关业务状态码。
// 不同网关返回形态不一致：成功可能是字符串 "CTAPI_10000"，也可能是纯数字 100000。
// 因此实现 json.Unmarshaler 同时兼容 string / number 两种形式。
type ctyunStatusCode string

// UnmarshalJSON 兼容字符串（"CTAPI_10000"）与数字（100000）两种返回。
func (s *ctyunStatusCode) UnmarshalJSON(b []byte) error {
	raw := bytes.TrimSpace(b)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		*s = ""
		return nil
	}
	// 字符串形式：去掉两端引号
	if raw[0] == '"' {
		*s = ctyunStatusCode(bytes.Trim(raw, `"`))
		return nil
	}
	// 数字形式：保留原始文本（如 100000）
	*s = ctyunStatusCode(raw)
	return nil
}

// OK 是否为成功状态码
func (s ctyunStatusCode) OK() bool {
	return string(s) == "CTAPI_10000" || string(s) == "100000"
}

// ctyunBaseResponse 天翼云 EOP 网关公共响应头（所有接口一致）。
type ctyunBaseResponse struct {
	StatusCode   ctyunStatusCode `json:"statusCode"`   // 业务状态码（成功为 CTAPI_10000）
	Message      string          `json:"message"`      // 结果简述
	Error        string          `json:"error"`        // 错误码
	ErrorMessage string          `json:"errorMessage"` // 错误详情
}

// ctyunResponse 泛型响应：ReturnObj 由各接口的具体类型 T 决定。
// 调用方用 ctyunRequestAPI[T](...) 传入自己的返回结构体，由 json 直接解析。
type ctyunResponse[T any] struct {
	ctyunBaseResponse
	ReturnObj T      `json:"returnObj"` // 返回对象（具体结构由 T 决定）
	Body      []byte `json:"-"`         // 原始响应体，便于调试
}

// OK 判断是否成功（成功业务码为 CTAPI_10000，兼容旧网关 100000）
func (r *ctyunResponse[T]) OK() bool {
	return r.StatusCode.OK()
}

// errText 取响应中可读性最好的错误信息
func (r *ctyunResponse[T]) errText() string {
	if r.ErrorMessage != "" {
		return r.ErrorMessage
	}
	if r.Error != "" {
		return r.Error
	}
	if r.Message != "" {
		return r.Message
	}
	return "unknown error"
}

// 以下为各接口 returnObj 的具体结构

// ctyunCreateCertReturn 创建证书接口的返回（证书 ID 在 returnObj.id）
type ctyunCreateCertReturn struct {
	ID intOrString `json:"id"`
}

// ctyunListDomainReturn 域名列表接口的返回（位于 EOP 信封的 returnObj 内）。
// 业务体结构：code=100000 成功，result 为域名列表。
type ctyunListDomainReturn struct {
	Code    int               `json:"code"`
	Message string            `json:"message"`
	Result  []ctyunDomainItem `json:"result"`
}

// ctyunDomainItem 域名列表项（domain 字段在 result[i].domain）
type ctyunDomainItem struct {
	Domain      string `json:"domain"`
	ProductCode string `json:"product_code"` // 产品编码（006=全站加速 008=CDN加速 020=边缘安全与加速）
}

// ctyunDomainCertReturn 查询域名证书详情（/domain/query-domain-cert-info）的返回（位于 EOP 信封 returnObj 内）。
// 天翼云官方文档确认（2026-07-28）：返回为扁平结构，证书公钥在 certs 字段（PEM），
// 过期等时间为 Unix 秒级时间戳。
type ctyunDomainCertReturn struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Cn      string `json:"cn"`      // 证书通用名称
	Name    string `json:"name"`    // 证书备注名称
	Certs   string `json:"certs"`   // 证书公钥信息（PEM，交由 parseCertPEM 解析）
	Key     string `json:"key"`     // 证书私钥信息
	Created int64  `json:"created"` // 创建时间（Unix 秒）
	Expires int64  `json:"expires"` // 过期时间（Unix 秒）
	Issue   int64  `json:"issue"`   // 颁发时间（Unix 秒）
	Issuer  string `json:"issuer"`  // 颁发机构
}

// ctyunExtractCertPEM 从域名证书查询响应中取出证书 PEM。
// 优先取官方字段 certs；为空时再扫描原始响应体里的 PEM 块兜底（避免极端情况下字段名偏差）。
func ctyunExtractCertPEM(r ctyunDomainCertReturn, raw []byte) string {
	if p := strings.TrimSpace(r.Certs); p != "" {
		return p
	}
	return extractFirstPEM(raw)
}

// extractFirstPEM 从任意文本中提取第一个 -----BEGIN CERTIFICATE----- 块（含结束标记）。
func extractFirstPEM(b []byte) string {
	s := string(b)
	start := strings.Index(s, "-----BEGIN CERTIFICATE-----")
	if start < 0 {
		return ""
	}
	end := strings.Index(s[start:], "-----END CERTIFICATE-----")
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(s[start : start+end+len("-----END CERTIFICATE-----")])
}

// ctyunSign 天翼云 EOP 网关签名（依据官方「网关EOP签名说明」）：
//  1. 待签名串 = 已排序待签Header(各以\n结尾) + "\n" + 规范query + "\n" + hex(sha256(body))
//     当 query 为空时，串形如 "ctyun-eop-request-id:...\neop-date:...\n\n\n<hash>"
//  2. 动态密钥：ktime=HMAC(sk, eop-date) → kAk=HMAC(ktime, ak) → kdate=HMAC(kAk, eop-date前8位)
//  3. signature = Base64(HMAC(kdate, 待签名串))
//  4. Authorization: ak Headers=ctyun-eop-request-id;eop-date Signature=<sig>
func ctyunSign(body []byte, query url.Values, ak, sk string) (string, string, string) {
	tim := time.Now()
	date := tim.Format("20060102T150405Z") // eop-date: yyyyMMddTHHmmssZ
	requestID := uuid.NewString()
	stringToSign := ctyunStringToSign(requestID, date, query, body)

	// 2) 派生动态密钥
	signDate := tim.Format("20060102") // eop-date 前 8 位
	kTime := hmacSHA256([]byte(date), []byte(sk))
	kAk := hmacSHA256([]byte(ak), kTime)
	kDate := hmacSHA256([]byte(signDate), kAk)

	// 3) 计算签名
	signaSha256 := hmacSHA256([]byte(stringToSign), kDate)
	signatureBase64 := base64.StdEncoding.EncodeToString(signaSha256)

	// 4) 构造鉴权头
	authorization := ak + " Headers=ctyun-eop-request-id;eop-date Signature=" + signatureBase64
	return authorization, date, requestID
}

// ctyunStringToSign 构造待签名串。
// 官方示例中（query 为空、body 为空）：
//
//	ctyun-eop-request-id:27cfe4dc-e640-45f6-92ca-492ca73e8680
//	eop-date:20220525T160752Z
//	\n
//	\n
//	e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
func ctyunStringToSign(requestID, date string, query url.Values, body []byte) string {
	// 1) 请求体 SHA256（hex）
	sum := sha256.Sum256(body)
	bodyHash := hex.EncodeToString(sum[:])

	// 待签名 Header 列表（按 header 名升序，各以 \n 结尾）
	headerStr := "ctyun-eop-request-id:" + requestID + "\neop-date:" + date + "\n"
	// 规范 query（按 key 升序、URL 编码后以 & 连接；GET 接口带查询参数时必填）
	canonicalQuery := query.Encode()
	return headerStr + "\n" + canonicalQuery + "\n" + bodyHash
}

// hmacSHA256 HMAC-SHA256 加密
func hmacSHA256(data, key []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

// ctyunHTTPClient 天翼云调用统一的全局 HTTP 客户端：包裹 httplog（仅 DEBUG 落请求日志），
// 并带 30s 超时。与 baidu/aliyun/面板等部署器保持一致，不使用裸 http.Client。
// 采用懒初始化（sync.Once）——包级 var 在 import 时早于 httplog.Init()，若那时包裹会因尚未
// 进入 DEBUG 而返回未包裹日志的 transport，导致请求无日志。首次真实请求时（DEBUG 已开启）
// 才创建，确保能正确包裹。
var (
	ctyunHTTPClientOnce sync.Once
	ctyunHTTPClientVar  *http.Client
)

func ctyunHTTPClient() *http.Client {
	ctyunHTTPClientOnce.Do(func() {
		// 用独立的基础 transport（非 http.DefaultTransport），避免与 httplog.Init 对全局
		// DefaultTransport 的包裹叠加造成重复记录。天翼云为正规公有云 API，不做 TLS 跳过验证。
		ctyunHTTPClientVar = httplog.WrapClient(&http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
			},
			Timeout: 30 * time.Second,
		})
	})
	return ctyunHTTPClientVar
}

// ctyunRequestAPI 发送天翼云 API 请求，并把 returnObj 解析为具体类型 T。
// query 为 URL 查询参数（GET 接口用），body 为请求体（POST 接口用）。
func ctyunRequestAPI[T any](baseURL, method, urlPath string, query url.Values, body []byte, creds Credentials) (*ctyunResponse[T], error) {
	authorization, date, requestID := ctyunSign(body, query, creds.AccessKeyID, creds.AccessKeySecret)

	u := baseURL + urlPath
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequest(method, u, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("ctyun-eop-request-id", requestID)
	req.Header.Set("Eop-Authorization", authorization)
	req.Header.Set("Eop-date", date)

	// 用全局日志 HTTP 客户端（仅 DEBUG 落请求日志），与 baidu/aliyun/面板等部署器保持一致，
	// 不使用裸 http.Client。采用懒初始化：首次真实请求时（httplog.Init 已执行、DEBUG 已开启）
	// 才包裹，避免 import 期过早包裹导致请求无日志。
	client := ctyunHTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result ctyunResponse[T]
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, i18n.Wrap(err, "deploy.error.ctyun_parse_response")
	}
	result.Body = respBody

	return &result, nil
}

// UploadCert 上传证书到天翼云 SSL 证书服务
// ctyunCertName 生成天翼云证书备注名：域名 + 证书内容 SHA256 前 8 位。
// 证书内容一变（续期）哈希即变，保证同一域名多次部署的备注名全局唯一、不冲突，
// 对齐 aliyun（域名+shortCertHash）/baidu（域名+sha256[:8]）的做法。
// 通配符 * 与特殊字符按业界惯例替换，避免备注名含非法字符。
func ctyunCertName(cert CertContent) string {
	base := strings.NewReplacer("*", "wildcard", ".", "-", " ", "-").Replace(strings.TrimSpace(cert.Domain))
	sum := sha256.Sum256([]byte(cert.CertPEM + cert.KeyPEM))
	return fmt.Sprintf("certflow-%s-%s", base, hex.EncodeToString(sum[:])[:8])
}

func (d *CtyunDeployer) UploadCert(ctx context.Context, creds Credentials, cert CertContent, svcConfig map[string]string) (string, string, error) {
	if cert.CertPEM == "" || cert.KeyPEM == "" {
		return "", "", i18n.NewError("deploy.error.ctyun_cert_empty")
	}

	// 证书备注名（天翼云 creat-cert 的 name 字段，同时也是 update-domain 绑定域名时
	// 引用的 cert_name）。为避免续期时新证书与旧证书备注名撞车，备注名需全局唯一：
	// 用户显式配置 cert_name 时优先使用；否则用「服务 + 域名 + 证书指纹哈希」生成，
	// 加服务前缀避免同一证书部署到不同 ctyun 产品（ctcdn/icdn/accessone）时备注名撞车。
	certName := svcConfig["cert_name"]
	if certName == "" {
		certName = ctyunCertName(cert)
		if svc := svcConfig["deploy_service"]; svc != "" {
			certName = svc + "-" + certName
		}
	}

	// 获取服务配置
	svc := svcConfig["deploy_service"]
	svcCfg, err := ctyunServiceByKey(svc)
	if err != nil {
		return "", "", err
	}

	requestBody := map[string]string{
		"name":  certName,
		"certs": cert.CertPEM,
		"key":   cert.KeyPEM,
	}

	body, _ := json.Marshal(requestBody)
	resp, err := ctyunRequestAPI[ctyunCreateCertReturn](svcCfg.BaseURL, "POST", svcCfg.CertPath, nil, body, creds)
	if err != nil {
		return "", "", i18n.Wrap(err, "deploy.error.ctyun_upload_cert")
	}

	if !resp.OK() {
		return "", "", fmt.Errorf("%s: %s", i18n.T("deploy.error.ctyun_upload_cert"), resp.errText())
	}

	certID := resp.ReturnObj.ID.Int()
	if certID == 0 {
		return "", "", fmt.Errorf("%s: %s", i18n.T("deploy.error.ctyun_upload_cert"), "empty cert id")
	}
	certIDStr := strconv.Itoa(certID)
	// cloudCertID 返回证书「备注名」：天翼云 update-domain 通过备注名（而非数字 id）
	// 关联证书，因此把备注名作为 cloudCertID 持久化并透传给 DeployCert，部署时即可用
	// 正确的备注名绑定域名；数字 id 仅用于日志核对。这样续期生成的新备注名也能正确关联。
	logging.Debug(i18n.T("log.deploy.ctyun_upload", "CertID", certIDStr, "CertName", certName))
	return certName, "", nil
}

// DeployCert 部署证书到天翼云 CDN/全站加速/边缘安全加速平台域名
func (d *CtyunDeployer) DeployCert(ctx context.Context, creds Credentials, certID string, svc string, svcConfig map[string]string) (*DeployResult, error) {
	domain := svcConfig["domain"]
	if domain == "" {
		return &DeployResult{CloudCertID: certID, Message: i18n.T("deploy.message.ctyun_no_domain")}, nil
	}

	// 获取服务配置
	svcCfg, err := ctyunServiceByKey(svc)
	if err != nil {
		return &DeployResult{CloudCertID: certID}, err
	}

	// certID 即 UploadCert 返回的证书「备注名」（cloudCertID），天翼云通过该备注名关联证书。
	// 直接原样透传，确保部署绑定的证书与上传时完全一致。
	// 部署只绑定证书、不改动域名 HTTPS 开关：各产品的 update-domain / modify_config 均为增量修改接口，
	// domain 为 string、未传字段保持原值，故不传 https_status（与「不擅自修改开关」的诉求一致）。
	// icdn 已切换为与 ctcdn 同款的 /v1/domain/update-domain（增量修改，domain 为 string、无需 product_code），
	// 不再使用此前报「参数domain为list型」的批量接口，因此所有服务统一走 string domain + cert_name。
	requestBody := map[string]any{
		"domain":    domain,
		"cert_name": certID,
	}

	body, _ := json.Marshal(requestBody)
	logging.Debug(i18n.T("log.deploy.ctyun_deploy_start", "Svc", svc, "Domain", domain, "CertID", certID))

	resp, err := ctyunRequestAPI[struct{}](svcCfg.BaseURL, "POST", svcCfg.DomainPath, nil, body, creds)
	if err != nil {
		logging.Debug(i18n.T("log.deploy.ctyun_deploy_failed", "Domain", domain, "Err", err))
		return &DeployResult{CloudCertID: certID}, i18n.Wrap(err, "deploy.error.ctyun_set_https")
	}

	if !resp.OK() {
		logging.Debug(i18n.T("log.deploy.ctyun_deploy_failed", "Domain", domain, "Err", resp.errText()))
		return &DeployResult{CloudCertID: certID}, fmt.Errorf("%s: %s", i18n.T("deploy.error.ctyun_set_https"), resp.errText())
	}

	logging.Debug(i18n.T("log.deploy.ctyun_deploy_success", "Domain", domain))
	return &DeployResult{
		CloudCertID: certID,
		Message:     i18n.T("deploy.message.ctyun_deployed", "Domain", domain),
	}, nil
}

// ListDomains 列出天翼云 CDN/全站加速/边缘安全加速平台域名
// ListSites 云厂商默认回退到 ListDomains。
func (d *CtyunDeployer) ListSites(ctx context.Context, creds Credentials, svc, region, zoneID string) ([]string, error) {
	return d.ListDomains(ctx, creds, svc, region, zoneID)
}

func (d *CtyunDeployer) ListDomains(ctx context.Context, creds Credentials, svc, region, zoneID string) ([]string, error) {
	// 获取服务配置
	svcCfg, err := ctyunServiceByKey(svc)
	if err != nil {
		return nil, err
	}

	// 查询域名列表为 GET，分页参数走 query string（page/page_size，非 page/size）。
	query := url.Values{}
	query.Set("page", "1")
	query.Set("page_size", "100")

	resp, err := ctyunRequestAPI[ctyunListDomainReturn](svcCfg.BaseURL, "GET", svcCfg.ListPath, query, nil, creds)
	if err != nil {
		return nil, i18n.Wrap(err, "deploy.error.ctyun_list_domains")
	}

	if !resp.OK() {
		// 仅在失败时打印原始响应，便于排查；成功路径不打，避免干扰日志（与其他服务商一致）。
		logging.Debug(i18n.T("log.deploy.ctyun_list_raw", "Svc", svc, "Body", string(resp.Body)))
		return nil, fmt.Errorf("%s: %s", i18n.T("deploy.error.ctyun_list_domains"), resp.errText())
	}

	// 业务成功与否以 EOP 信封外层的 statusCode（已在 resp.OK() 中判定）为准。
	// 实测 ctcdn/icdn/accessone 的 returnObj 内不含 code 字段，故不再做内层 code 校验。

	// 域名列表在 returnObj.result[].domain。
	// 天翼云接口返回账号下全部产品的域名，需按所选服务的 product_code 过滤，
	// 只保留当前部署服务对应的产品域名（否则三种服务拉到的列表完全相同）。
	wantProduct := ctyunServiceProductCode[svc]
	domains := make([]string, 0, len(resp.ReturnObj.Result))
	for _, item := range resp.ReturnObj.Result {
		if item.Domain == "" {
			continue
		}
		if wantProduct != "" && item.ProductCode != wantProduct {
			continue
		}
		domains = append(domains, item.Domain)
	}
	logging.Debug(i18n.T("log.deploy.ctyun_list_filter", "Svc", svc, "Product", wantProduct, "Total", len(resp.ReturnObj.Result), "Matched", len(domains)))

	return domains, nil
}

// GetCurrentCert 查询天翼云某域名当前生效证书。
//   - ctcdn / icdn：POST QueryCertPath（/v1/domain/query-domain-cert-info，按加速域名）查询当前生效证书，
//     响应扁平结构取 certs 字段 PEM（官方文档确认）。
//   - accessone：官方无查询证书详情的接口（仅有按备注名反查绑定域名的接口，无证书内容/到期时间），
//     视为不支持云端查询。
func (d *CtyunDeployer) GetCurrentCert(ctx context.Context, creds Credentials, svc string, svcConfig map[string]string) (*CurrentCert, error) {
	logging.Debug(i18n.T("log.deploy.ctyun_get_current_cert",
		"Svc", svc,
		"Domain", svcConfig["domain"]))

	domain := svcConfig["domain"]
	if strings.TrimSpace(domain) == "" {
		return nil, i18n.NewError("deploy.error.current_cert_domain_empty")
	}

	svcCfg, err := ctyunServiceByKey(svc)
	if err != nil {
		return nil, err
	}

	switch svc {
	case "ctcdn", "icdn":
		// 按加速域名查询当前生效证书详情。
		// product_code（官方文档确认必填）：ctcdn=CDN加速(008)，icdn=全站加速(006)；
		// 允许 svcConfig["product_code"] 覆盖（如静态/下载/点播加速取 001/003/004）。
		productCode := "008"
		if svc == "icdn" {
			productCode = "006"
		}
		if v := strings.TrimSpace(svcConfig["product_code"]); v != "" {
			productCode = v
		}
		body, mErr := json.Marshal(map[string]string{"product_code": productCode, "domain": domain})
		if mErr != nil {
			return nil, i18n.Wrap(mErr, "deploy.error.ctyun_query_cert")
		}
		if svcCfg.QueryCertPath == "" {
			return nil, i18n.NewError("deploy.error.current_cert_cloud_not_supported")
		}
		resp, err := ctyunRequestAPI[ctyunDomainCertReturn](svcCfg.BaseURL, "POST", svcCfg.QueryCertPath, nil, body, creds)
		if err != nil {
			return nil, i18n.Wrap(err, "deploy.error.ctyun_query_cert")
		}
		if !resp.OK() {
			logging.Debug(i18n.T("log.deploy.ctyun_list_raw", "Svc", svc, "Body", string(resp.Body)))
			return nil, fmt.Errorf("%s: %s", i18n.T("deploy.error.ctyun_query_cert"), resp.errText())
		}
		pem := ctyunExtractCertPEM(resp.ReturnObj, resp.Body)
		if pem == "" {
			// 未取到证书内容：可能该域名未配置证书，或返回结构需按真实凭证调整（已在 DEBUG 落原始体）。
			logging.Debug(i18n.T("log.deploy.ctyun_list_raw", "Svc", svc, "Body", string(resp.Body)))
			return nil, i18n.NewError("deploy.error.current_cert_not_configured")
		}
		return parseCertPEM(pem)

	default:
		// accessone（边缘安全加速）官方仅提供「按证书备注名反查绑定域名」
		// （GET /ctapi/v1/accessone/cert/list_domain_by_cert，仅返回域名列表，
		// https://eop.ctyun.cn/ebp/ctapioutDocument/queryCtApiByCapacityId/13176），
		// 不返回证书内容或到期时间等元数据，无法构造 CurrentCert，故视为不支持云端查询。
		return nil, i18n.NewError("deploy.error.current_cert_cloud_not_supported")
	}
}
