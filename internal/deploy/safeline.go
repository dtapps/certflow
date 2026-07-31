package deploy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"cnb.cool/dtapp/certflow/internal/i18n"
	"cnb.cool/dtapp/certflow/internal/logging"
)

// SafelineDeployer 雷池 SafeLine WAF（长亭）社区版部署器。
// OpenAPI 基础路径 /api/open，鉴权使用请求头 X-SLCE-API-TOKEN（API 令牌，映射到 AccessKeyID）。
// 参考：https://help.waf-ce.chaitin.cn 与实例 /swagger/index.html。
//
// 各接口实测结构（均为统一信封 {"data":..., "err":null, "msg":""}）：
//   - 站点列表   GET  /api/open/site       → data.data 为 slSite 数组（双层信封，另含 total/syncing）
//   - 站点详情   GET  /api/open/site/{id}  → data 为完整 slSite 对象（单层信封）
//   - 证书列表   GET  /api/open/cert       → data.nodes 为 slCertNode 数组（另含 total，不含 PEM）
//   - 证书详情   GET  /api/open/cert/{id}  → data 含 type 与 manual.crt（手动证书完整 PEM）、acme.domains
//   - 证书上传   POST /api/open/cert       → 请求体 {"manual":{"crt":"..","key":".."},"type":2}
//   - 绑定证书：将站点对象 cert_id 置为证书 id 后 PUT /api/open/site（id 在请求体，须回写完整站点对象）
//
// 部署时先按证书序列号比对站点当前证书：内容一致则复用 id、跳过上传；内容变更则就地更新该证书内容。
// 证书上传响应的 data 形态未在实测中确认，故新建证书后通过再次拉取证书列表、
// 按同一比对条件反查证书 id（列表结构已实测确认），不对响应结构做猜测。
type SafelineDeployer struct {
	panelDeployerBase
}

// 雷池 OpenAPI 路径（不同版本可能为复数 /api/open/sites、/api/open/certs，按需调整）。
const (
	slSitePath = "/api/open/site"
	slCertPath = "/api/open/cert"
)

func init() {
	RegisterDeployer(SafelineDeployer{panelDeployerBase{provider: ProviderSafeline}})
}

// slUint 兼容雷池把 id 类字段返回为数字或字符串两种情况（如 "1" 与 1）。
// 通过实现 json.Unmarshaler 直接在结构体字段声明，避免散落的 map 解析。
// 解析失败静默当作 0，避免整段响应解析失败。
type slUint uint

func (n *slUint) UnmarshalJSON(b []byte) error {
	s := bytes.TrimSpace(b)
	s = bytes.Trim(s, `"`)
	if len(s) == 0 || bytes.Equal(s, []byte("null")) {
		*n = 0
		return nil
	}
	v, _ := strconv.ParseUint(string(s), 10, 64)
	*n = slUint(v)
	return nil
}

// Uint 返回底层 uint 值。
func (n slUint) Uint() uint { return uint(n) }

// slEnvelope 雷池统一响应信封：{"data":..., "err":..., "msg":...}。
// data 形态不定（对象/数组/数字），故保留为 RawMessage，由具体接口再解析。
type slEnvelope struct {
	Data json.RawMessage `json:"data"`
	Err  *string         `json:"err"`
	Msg  string          `json:"msg"`
}

// slUnwrapInner 解开雷池信封，校验 err 字段后返回 data 字段的原始 JSON。
func slUnwrapInner(body []byte) (json.RawMessage, error) {
	var env slEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("%s", i18n.T("error.deploy_panel_list_parse", "Err", err.Error()))
	}
	if err := env.checkErr(); err != nil {
		return nil, err
	}
	if len(env.Data) == 0 {
		return nil, fmt.Errorf("%s", i18n.T("error.deploy_panel_list_parse", "Err", "响应缺少 data 字段"))
	}
	return env.Data, nil
}

// checkErr 校验信封 err 字段（err 非 null 即业务失败，msg 为失败描述）。
func (env slEnvelope) checkErr() error {
	if env.Err != nil && *env.Err != "" {
		detail := env.Msg
		if detail == "" {
			detail = *env.Err
		}
		return fmt.Errorf("%s", detail)
	}
	return nil
}

// slCheckResp 仅校验响应信封的 err 字段，不关心 data 形态（用于上传证书等写操作响应）。
func slCheckResp(body []byte) error {
	var env slEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("%s", i18n.T("error.deploy_panel_list_parse", "Err", err.Error()))
	}
	return env.checkErr()
}

// slCollect 将信封 data 字段解析为 T 数组，兼容两种形态：
//   - 双层信封：data 为 {"data":[...], ...} 对象（如站点列表实测结构）
//   - 单层信封：data 直接是 [...] 数组（如证书列表）
func slCollect[T any](data json.RawMessage) ([]T, error) {
	// 先试双层信封：data 内再嵌 data 数组
	var inner struct {
		Data []T `json:"data"`
	}
	if err := json.Unmarshal(data, &inner); err == nil {
		return inner.Data, nil
	}
	// 再试单层信封：data 直接是数组
	var direct []T
	if err := json.Unmarshal(data, &direct); err == nil {
		return direct, nil
	}
	return nil, fmt.Errorf("%s", i18n.T("error.deploy_panel_list_parse", "Err", "响应 data 既非数组也非 {data:[...]}"))
}

// slSite 雷池站点（Web 服务）对象，字段与实测 GET /api/open/site/{id} 响应一一对应。
// 复杂/可空字段（load_balance、health_state、各类 *_path、custom_location、forwarding_rules 等）
// 用 any 保留原值，便于 PUT 回写时完整还原、不丢配置。
type slSite struct {
	ID                       slUint   `json:"id"`
	CreatedAt                string   `json:"created_at"`
	UpdatedAt                string   `json:"updated_at"`
	GroupID                  uint     `json:"group_id"`
	Comment                  string   `json:"comment"`
	ServerNames              []string `json:"server_names"`
	Ports                    []string `json:"ports"`
	Upstreams                []string `json:"upstreams"`
	IsEnabled                bool     `json:"is_enabled"`
	LoadBalance              any      `json:"load_balance"`
	ExcludePaths             any      `json:"exclude_paths"`
	ExcludeContentType       any      `json:"exclude_content_type"`
	Mode                     int      `json:"mode"`
	Static                   bool     `json:"static"`
	Type                     int      `json:"type"`
	Index                    string   `json:"index"`
	StaticDefault            int      `json:"static_default"`
	Init                     bool     `json:"init"`
	RedirectStatusCode       int      `json:"redirect_status_code"`
	CertType                 int      `json:"cert_type"`
	CertID                   slUint   `json:"cert_id"`
	CertFilename             string   `json:"cert_filename"`
	KeyFilename              string   `json:"key_filename"`
	Email                    string   `json:"email"`
	Title                    string   `json:"title"`
	Icon                     string   `json:"icon"`
	ACLResponseStatusCode    int      `json:"acl_response_status_code"`
	ACLResponseHTMLPath      string   `json:"acl_response_html_path"`
	ForbiddenStatusCode      int      `json:"forbidden_status_code"`
	ForbiddenHTMLPath        string   `json:"forbidden_html_path"`
	NotFoundStatusCode       int      `json:"not_found_status_code"`
	NotFoundHTMLPath         string   `json:"not_found_html_path"`
	OfflineStatusCode        int      `json:"offline_status_code"`
	OfflineHTMLPath          string   `json:"offline_html_path"`
	BadGatewayStatusCode     int      `json:"bad_gateway_status_code"`
	BadGatewayHTMLPath       string   `json:"bad_gateway_html_path"`
	GatewayTimeoutStatusCode int      `json:"gateway_timeout_status_code"`
	GatewayTimeoutHTMLPath   string   `json:"gateway_timeout_html_path"`
	AuthDefenseID            int      `json:"auth_defense_id"`
	ChallengeID              int      `json:"challenge_id"`
	ChaosID                  int      `json:"chaos_id"`
	ChaosIsEnabled           bool     `json:"chaos_is_enabled"`
	AccessLogLimit           int      `json:"access_log_limit"`
	ErrorLogLimit            int      `json:"error_log_limit"`
	ACLEnabled               bool     `json:"acl_enabled"`
	TamperRefresh            int64    `json:"tamper_refresh"`
	TamperRefreshState       string   `json:"tamper_refresh_state"`
	WRID                     int      `json:"wr_id"`
	CustomLocation           any      `json:"custom_location"`
	HealthCheck              bool     `json:"health_check"`
	Portal                   bool     `json:"portal"`
	PortalRedirect           string   `json:"portal_redirect"`
	Position                 int      `json:"position"`
	ForwardingRules          any      `json:"forwarding_rules"`
	StatEnabled              bool     `json:"stat_enabled"`
	SPEnabled                bool     `json:"sp_enabled"`
	HealthState              any      `json:"health_state"`
	CCBot                    bool     `json:"cc_bot"`
	ReqValue                 int      `json:"req_value"`
	DeniedValue              int      `json:"denied_value"`
	Semantics                bool     `json:"semantics"`
}

// slSiteName 返回站点的展示名：优先 server_names 首项（即域名），其次 comment。
// 实测响应无顶层 name 字段，站点域名即 server_names[0]。
func (s slSite) slSiteName() string {
	if len(s.ServerNames) > 0 {
		return s.ServerNames[0]
	}
	if s.Comment != "" {
		return s.Comment
	}
	return ""
}

// slCertNode 雷池证书列表节点，字段与实测 GET /api/open/cert 响应 data.nodes[] 一一对应。
type slCertNode struct {
	ID            slUint   `json:"id"`
	Domains       []string `json:"domains"`
	Issuer        string   `json:"issuer"`
	SelfSignature bool     `json:"self_signature"`
	Trusted       bool     `json:"trusted"`
	Revoked       bool     `json:"revoked"`
	Expired       bool     `json:"expired"`
	Type          int      `json:"type"`
	ACMEMessage   string   `json:"acme_message"`
	ValidBefore   string   `json:"valid_before"`
	RelatedSites  any      `json:"related_sites"`
}

// slCertList 证书列表信封内层：{"nodes":[...], "total":N}。
type slCertList struct {
	Nodes []slCertNode `json:"nodes"`
	Total int          `json:"total"`
}

// slCertManual 手动上传证书的 crt/key 字段（请求体）。
type slCertManual struct {
	CRT string `json:"crt"`
	Key string `json:"key"`
}

// slCertUploadReq 证书上传请求体，实测为 {"manual":{"crt":"..","key":".."},"type":2}。
type slCertUploadReq struct {
	Manual slCertManual `json:"manual"`
	Type   int          `json:"type"`
}

// slCertACME 证书详情中 acme 字段（自动签发证书的域名/邮箱信息）。
type slCertACME struct {
	Domains []string `json:"domains"`
	Email   string   `json:"email"`
}

// slCertDetail 证书详情（GET /api/open/cert/{id} 响应 data）。
// 手动上传证书（type=2）的 PEM 在 manual.crt；acme 字段含自动签发证书的域名信息。
// 注：实测响应 data.id 恒为 0（与 URL 中的 id 不一致），故不依赖该字段。
type slCertDetail struct {
	ID     slUint       `json:"id"`
	Type   int          `json:"type"`
	ACME   slCertACME   `json:"acme"`
	Manual slCertManual `json:"manual"`
}

// slGetCertDetail 拉取证书详情（GET /api/open/cert/{id}），返回 manual.crt 等字段。
func slGetCertDetail(ctx context.Context, client *PanelClient, certID uint) (*slCertDetail, error) {
	body, err := client.doGetRequest(ctx, slCertPath+"/"+strconv.FormatUint(uint64(certID), 10), nil)
	if err != nil {
		return nil, err
	}
	inner, err := slUnwrapInner(body)
	if err != nil {
		return nil, err
	}
	var d slCertDetail
	if err := json.Unmarshal(inner, &d); err != nil {
		return nil, fmt.Errorf("%s", i18n.T("error.deploy_panel_list_parse", "Err", err.Error()))
	}
	return &d, nil
}

// slListCerts 拉取证书列表（GET /api/open/cert → data.nodes）。
func slListCerts(ctx context.Context, client *PanelClient) ([]slCertNode, error) {
	body, err := client.doGetRequest(ctx, slCertPath, nil)
	if err != nil {
		return nil, err
	}
	inner, err := slUnwrapInner(body)
	if err != nil {
		return nil, err
	}
	var list slCertList
	if err := json.Unmarshal(inner, &list); err != nil {
		return nil, fmt.Errorf("%s", i18n.T("error.deploy_panel_list_parse", "Err", err.Error()))
	}
	return list.Nodes, nil
}

// slSameDomainSet 判断两组域名集合是否一致（忽略大小写、顺序与首尾空白）。
// 用于把「证书列表节点的 domains」与「本地证书 SANs」做匹配，定位同一份证书。
func slSameDomainSet(a, b []string) bool {
	if len(a) == 0 || len(a) != len(b) {
		return false
	}
	set := make(map[string]struct{}, len(a))
	for _, d := range a {
		set[strings.ToLower(strings.TrimSpace(d))] = struct{}{}
	}
	for _, d := range b {
		if _, ok := set[strings.ToLower(strings.TrimSpace(d))]; !ok {
			return false
		}
	}
	return true
}

// safelineAuth 生成雷池鉴权头：X-SLCE-API-TOKEN: <API 令牌>。
func safelineAuth(c *PanelClient, _, _ string, _ url.Values, _ []byte) (http.Header, url.Values, error) {
	h := http.Header{}
	if c.APIKey != "" {
		h.Set("X-SLCE-API-TOKEN", c.APIKey)
	}
	return h, nil, nil
}

// slSameCert 判断证书列表节点与本地证书是否为同一张证书：
// 域名集合一致 且 到期时间一致（RFC3339 解析后比较时刻，兼容时区写法差异）。
func slSameCert(n slCertNode, local *CurrentCert) bool {
	if !slSameDomainSet(n.Domains, local.SANs) {
		return false
	}
	remote, err1 := time.Parse(time.RFC3339, n.ValidBefore)
	mine, err2 := time.Parse(time.RFC3339, local.NotAfter)
	if err1 != nil || err2 != nil {
		return false
	}
	return remote.Equal(mine)
}

// slCertUpdateReq 证书内容就地更新请求体。
// 当站点当前引用证书的内容与待部署证书不一致时，用此请求体 POST /api/open/cert 并带 id，
// 就地更新该证书内容（站点因持有该 cert_id 而自动生效新证书）。请求体与上传一致，仅多 id 字段。
type slCertUpdateReq struct {
	ID     slUint       `json:"id"`
	Manual slCertManual `json:"manual"`
	Type   int          `json:"type"`
}

// slUpdateCertContent 就地更新指定 id 证书的内容（POST /api/open/cert，带 id）。
func slUpdateCertContent(ctx context.Context, client *PanelClient, certID uint, certPEM, keyPEM string) error {
	req := slCertUpdateReq{
		ID:     slUint(certID),
		Manual: slCertManual{CRT: certPEM, Key: keyPEM},
		Type:   2, // 手动上传证书
	}
	resp, err := client.doJSONRequest(ctx, slCertPath, req)
	if err != nil {
		return err
	}
	return slCheckResp(resp)
}

// slEnsureSiteBind 将站点绑定到目标证书：把站点对象的 cert_id 置为 certID 后
// PUT /api/open/site（社区版正确的绑定接口，id 在请求体，须回写完整站点对象）。
func slEnsureSiteBind(ctx context.Context, client *PanelClient, site *slSite, certID uint) error {
	site.CertID = slUint(certID)
	resp, err := client.doPutJSONRequest(ctx, slSitePath, site)
	if err != nil {
		return err
	}
	return slCheckResp(resp)
}

// slCreateCert 新建证书（POST /api/open/cert，不带 id），返回新证书 id。
// 用于站点尚未绑定任何证书（cert_id=0）的场景：上传新证书后由 DeployCert 回写站点 cert_id 完成绑定。
func slCreateCert(ctx context.Context, client *PanelClient, certPEM, keyPEM string) (uint, error) {
	req := slCertUploadReq{
		Manual: slCertManual{CRT: certPEM, Key: keyPEM},
		Type:   2,
	}
	resp, err := client.doJSONRequest(ctx, slCertPath, req)
	if err != nil {
		return 0, err
	}
	if err := slCheckResp(resp); err != nil {
		return 0, err
	}
	// 反查新证书 id（与 slEnsureCert 一致，避免猜响应结构）。
	nodes, err := slListCerts(ctx, client)
	if err != nil {
		return 0, err
	}
	local, err := parseCertPEM(certPEM)
	if err != nil {
		return 0, err
	}
	for _, n := range nodes {
		if slSameCert(n, local) {
			return n.ID.Uint(), nil
		}
	}
	return 0, fmt.Errorf("%s", i18n.T("error.deploy_panel_list_parse", "Err", "上传后未在证书列表中找到该证书"))
}

// slCertContentSame 判断站点当前证书(id)的内容是否与待部署证书一致（按证书序列号比对）。
// 一致则无需更新（满足「证书存在是跳过」）；当前证书无 PEM（如 acme 证书）或解析失败时
// 保守判定为不一致，执行更新以保证生效。
func slCertContentSame(ctx context.Context, client *PanelClient, certID uint, certPEM string) (bool, error) {
	detail, err := slGetCertDetail(ctx, client, certID)
	if err != nil {
		return false, err
	}
	if detail.Manual.CRT == "" {
		return false, nil
	}
	want, err := parseCertPEM(certPEM)
	if err != nil {
		return false, err
	}
	have, err := parseCertPEM(detail.Manual.CRT)
	if err != nil {
		// 现有证书 PEM 异常，保守更新。
		//nolint:nilerr // 故意忽略解析错误：现有证书无法解析时按「不一致」处理以触发更新，更安全
		return false, nil
	}
	return want.SerialNumber == have.SerialNumber, nil
}

// ListSites 列出雷池站点，用于前端「获取网站」下拉。
func (d SafelineDeployer) ListSites(ctx context.Context, creds Credentials, _, _, _ string) ([]string, error) {
	if creds.PanelURL == "" {
		return nil, fmt.Errorf("%s", i18n.T("error.deploy_panel_no_url"))
	}
	client := newPanelClient(creds.PanelURL, creds.AccessKeyID, creds.AccessKeySecret, nil, safelineAuth)
	body, err := client.doGetRequest(ctx, slSitePath, nil)
	if err != nil {
		return nil, err
	}
	inner, err := slUnwrapInner(body)
	if err != nil {
		return nil, err
	}
	sites, err := slCollect[slSite](inner)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(sites))
	for _, s := range sites {
		name := s.slSiteName()
		if name == "" {
			name = strconv.FormatUint(uint64(s.ID.Uint()), 10)
		}
		out = append(out, name+"||"+strconv.FormatUint(uint64(s.ID.Uint()), 10))
	}
	return out, nil
}

// DeployCert 将证书部署到雷池站点（上传证书并绑定到站点）。
func (d SafelineDeployer) DeployCert(ctx context.Context, creds Credentials, _ string, _ string, svc map[string]string) (*DeployResult, error) {
	siteID := svc["site_id"]
	if siteID == "" {
		if name := svc["site_name"]; name != "" {
			if parts := strings.SplitN(name, "||", 2); len(parts) == 2 {
				siteID = strings.TrimSpace(parts[1])
			}
		}
	}
	if siteID == "" {
		return nil, fmt.Errorf("%s: %s", i18n.T("error.deploy_panel_cert_deploy"), "站点 ID 缺失")
	}
	certPEM := svc["cert_pem"]
	keyPEM := svc["key_pem"]
	if certPEM == "" || keyPEM == "" {
		return nil, fmt.Errorf("%s: %s", i18n.T("error.deploy_panel_cert_deploy"), "证书或私钥为空")
	}
	// 归一化私钥并做本地 key/cert 匹配预校验，提前暴露证书数据问题。
	keyPEM = normalizePrivateKeyPEM(keyPEM)
	if err := verifyKeyCertMatch(keyPEM, certPEM); err != nil {
		return nil, fmt.Errorf("%s: %s (%s)", i18n.T("error.deploy_panel_cert_verify"), err.Error(), keyCertMismatchDetail(keyPEM, certPEM))
	}
	if creds.PanelURL == "" {
		return nil, fmt.Errorf("%s", i18n.T("error.deploy_panel_no_url"))
	}
	client := newPanelClient(creds.PanelURL, creds.AccessKeyID, creds.AccessKeySecret, nil, safelineAuth)
	logging.Debug("safeline DeployCert: siteID=%s keyType=%s", siteID, keyTypeOf(keyPEM))

	// 1) 拉取站点详情（保留完整对象用于回写，避免丢配置）
	siteBody, err := client.doGetRequest(ctx, slSitePath+"/"+siteID, nil)
	if err != nil {
		return nil, fmt.Errorf("%s: %s", i18n.T("error.deploy_panel_cert_deploy"), err.Error())
	}
	inner, err := slUnwrapInner(siteBody)
	if err != nil {
		return nil, fmt.Errorf("%s: %s", i18n.T("error.deploy_panel_cert_deploy"), err.Error())
	}
	var site slSite
	if err := json.Unmarshal(inner, &site); err != nil {
		return nil, fmt.Errorf("%s: %s", i18n.T("error.deploy_panel_cert_deploy"), err.Error())
	}

	// 2) 确定目标证书 id，并在内容变更时就地更新证书内容。
	//    「证书存在是跳过」：站点已绑定证书且内容一致时，复用现有证书、跳过上传/更新。
	certID := site.CertID.Uint()
	targetCertID := certID
	if certID == 0 {
		// 站点尚未绑定任何证书：上传新证书拿到 id，稍后由步骤 3 回写绑定。
		logging.Info("safeline: 站点未绑定证书，上传新证书")
		newID, err := slCreateCert(ctx, client, certPEM, keyPEM)
		if err != nil {
			return nil, fmt.Errorf("%s: %s", i18n.T("error.deploy_panel_cert_deploy"), err.Error())
		}
		targetCertID = newID
	} else {
		same, err := slCertContentSame(ctx, client, certID, certPEM)
		if err != nil {
			return nil, fmt.Errorf("%s: %s", i18n.T("error.deploy_panel_cert_deploy"), err.Error())
		}
		if same {
			logging.Info("safeline: 站点证书(id=%d)内容未变，跳过上传（已是最新）", certID)
		} else {
			logging.Info("safeline: 站点证书(id=%d)内容已变更，就地更新", certID)
			if err := slUpdateCertContent(ctx, client, certID, certPEM, keyPEM); err != nil {
				return nil, fmt.Errorf("%s: %s", i18n.T("error.deploy_panel_cert_deploy"), err.Error())
			}
		}
	}

	// 3) 站点绑定证书（用户强约束：站点绑定不能跳过，每次部署都执行）。
	//    将站点对象的 cert_id 置为目标证书 id，PUT /api/open/site（id 在请求体，须回写完整站点对象）。
	if err := slEnsureSiteBind(ctx, client, &site, targetCertID); err != nil {
		return nil, fmt.Errorf("%s: %s", i18n.T("error.deploy_panel_cert_deploy"), err.Error())
	}

	return &DeployResult{
		// 展示真实的雷池证书 id（目标证书 id），而非站点 id。
		// 字段语义为「云厂商返回的证书 ID」，此前误用 siteID 会导致用户看到误导性的「云证书 ID: 1」。
		CloudCertID: strconv.FormatUint(uint64(targetCertID), 10),
		Message:     i18n.T("deploy.panel_ssl_set"),
	}, nil
}

// GetCurrentCert 查询雷池站点当前生效证书。
// 站点持有 cert_id；优先调用证书详情接口（GET /api/open/cert/{id}）取 manual.crt，
// 解析真实证书得到准确的 SAN/到期时间/签发者；若详情无 PEM（如 acme 自动证书）或调用失败，
// 退回证书列表（GET /api/open/cert → data.nodes）按 id 定位节点，用节点的字段组装（列表不含 PEM）。
func (d SafelineDeployer) GetCurrentCert(ctx context.Context, creds Credentials, _ string, svc map[string]string) (*CurrentCert, error) {
	logging.Debug("safeline GetCurrentCert: siteID=%s", svc["site_id"])
	siteID := svc["site_id"]
	if siteID == "" {
		if name := svc["site_name"]; name != "" {
			if parts := strings.SplitN(name, "||", 2); len(parts) == 2 {
				siteID = strings.TrimSpace(parts[1])
			}
		}
	}
	if siteID == "" {
		return nil, fmt.Errorf("%s: %s", i18n.T("error.deploy_panel_cert_deploy"), "站点 ID 缺失")
	}
	if creds.PanelURL == "" {
		return nil, fmt.Errorf("%s", i18n.T("error.deploy_panel_no_url"))
	}
	client := newPanelClient(creds.PanelURL, creds.AccessKeyID, creds.AccessKeySecret, nil, safelineAuth)
	siteBody, err := client.doGetRequest(ctx, slSitePath+"/"+siteID, nil)
	if err != nil {
		return nil, i18n.Wrap(err, "deploy.error.current_cert_query")
	}
	inner, err := slUnwrapInner(siteBody)
	if err != nil {
		return nil, i18n.Wrap(err, "deploy.error.current_cert_query")
	}
	var site slSite
	if err := json.Unmarshal(inner, &site); err != nil {
		return nil, i18n.Wrap(err, "deploy.error.current_cert_query")
	}
	if site.CertID.Uint() == 0 {
		return nil, i18n.NewError("deploy.error.current_cert_not_configured")
	}
	// 1) 优先用详情接口取 manual.crt，解析真实证书（SAN/NotAfter/Issuer 均来自 PEM）。
	detail, derr := slGetCertDetail(ctx, client, site.CertID.Uint())
	if derr == nil && strings.TrimSpace(detail.Manual.CRT) != "" {
		return parseCertPEM(detail.Manual.CRT)
	}
	// 2) 兜底：详情无 PEM 或失败，用证书列表节点字段组装（valid_before 实测即 RFC3339）。
	nodes, lerr := slListCerts(ctx, client)
	if lerr != nil {
		if derr != nil {
			return nil, i18n.Wrap(derr, "deploy.error.current_cert_query")
		}
		return nil, i18n.Wrap(lerr, "deploy.error.current_cert_query")
	}
	for _, n := range nodes {
		if n.ID.Uint() != site.CertID.Uint() {
			continue
		}
		cn := ""
		if len(n.Domains) > 0 {
			cn = n.Domains[0]
		}
		return &CurrentCert{
			CommonName: cn,
			SANs:       n.Domains,
			Issuer:     n.Issuer,
			NotAfter:   n.ValidBefore, // 实测已是 RFC3339（如 2026-09-29T07:30:28Z）
		}, nil
	}
	return nil, i18n.NewError("deploy.error.current_cert_not_configured")
}
