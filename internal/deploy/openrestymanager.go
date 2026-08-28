package deploy

import (
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
	"github.com/golang-jwt/jwt/v5"
)

// OpenRestyManagerDeployer OpenResty Manager（Safe3/openresty-manager 面板）部署器。
// API 前缀 /api/v1，管理接口 /api/v1/admin/*。
// 鉴权：面板使用 echojwt 中间件（HS256 + JWT 密钥），仅校验签名与过期时间，无自定义 ClaimsValidator；
// 因登录接口需「用户名+OTP」（用户已开启 OTP），故采用客户端用 JWT 密钥本地签发 JWT 的方式，
// 由 ormAuth 在每次请求时生成 Authorization: Bearer <JWT>，免去登录与 OTP。
// 凭证：panel_url 标识面板地址，jwt_secret 即面板的「JWT 密钥」（映射到 AccessKeySecret）。
// 部署目标：site 标识待部署站点（站点 id）。
//
// 证书部署流程（面板无内嵌证书字段，站点仅持有 cert_id）：
//  1. GET /api/v1/admin/sites 拉取完整站点对象（保留全部字段以便回写不丢配置）；
//  2. POST /api/v1/admin/certs（type=1 手动上传）上传证书，得到证书名；
//  3. GET /api/v1/admin/certs 按名查得证书 id；
//  4. 将站点 cert_id 指向新证书，PUT /api/v1/admin/sites 回写完整站点对象。
type OpenRestyManagerDeployer struct {
	panelDeployerBase
}

func init() {
	RegisterDeployer(OpenRestyManagerDeployer{panelDeployerBase{provider: ProviderOpenRestyManager}})
}

// ormAuth 生成 OpenResty Manager 鉴权头：Authorization: Bearer <JWT>。
// 该面板使用 echojwt 中间件，仅校验 HS256 签名与过期时间（无自定义 ClaimsValidator），
// 故客户端用面板的「JWT 密钥」本地签发 JWT 即可通过鉴权，免去「用户名+OTP」登录流程。
func ormAuth(c *PanelClient, _, _ string, _ url.Values, _ []byte) (http.Header, url.Values, error) {
	h := http.Header{}
	if c.APISecret != "" {
		token, err := signORMJWT(c.APISecret)
		if err != nil {
			return nil, nil, err
		}
		h.Set("Authorization", "Bearer "+token)
	}
	return h, nil, nil
}

// ormJWTClaims 与 openresty-manager 自定义 JWT claims 对齐（uid/username/role + 标准注册声明）。
// 因中间件未配置 ClaimsValidator，这些业务字段仅作兼容填充，不参与鉴权决策。
type ormJWTClaims struct {
	Uid      uint   `json:"uid"`
	Username string `json:"username"`
	Role     int    `json:"role"`
	jwt.RegisteredClaims
}

// signORMJWT 用面板 JWT 密钥（HS256）本地签发一个有效期 24 小时的 JWT，
// 等价于面板登录接口下发的 token，从而无需用户名/OTP 登录。
func signORMJWT(jwtKey string) (string, error) {
	now := time.Now()
	claims := ormJWTClaims{
		Uid:       1,
		Username:  "admin",
		Role:      0, // 0 代表管理员
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(24 * time.Hour)),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(jwtKey))
}

// ormUint 兼容 OpenResty Manager 接口中以数字或字符串形式下发的无符号整数
// （如 id / cert_id 可能为 2 或 "2"）。
type ormUint uint

func (u *ormUint) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}
	if n, err := strconv.ParseUint(s, 10, 64); err == nil {
		*u = ormUint(n)
		return nil
	}
	var f float64
	if err := json.Unmarshal([]byte(s), &f); err == nil {
		*u = ormUint(f)
	}
	return nil
}

func (u ormUint) Uint() uint { return uint(u) }

func (u ormUint) String() string { return strconv.FormatUint(uint64(u), 10) }

// ormSite OpenResty Manager 站点对象（GET /api/v1/admin/sites 中单个站点）。
// 站点结构复杂、含完整 nginx 配置；为 PUT 回写时不丢失面板配置，
// 通过 Raw 保留原始 JSON，仅覆盖 cert_id 后再整体回写（见 ormSiteWithCertID）。
// id 键兼容 "id" / "ID" 两种大小写，值兼容数字与字符串（见 ormUint）。
type ormSite struct {
	ID     ormUint         `json:"id"`
	IDBig  ormUint         `json:"ID"`
	Name   string          `json:"name"`
	CertID ormUint         `json:"cert_id"`
	Raw    json.RawMessage `json:"-"`
}

// RealID 返回站点 id 的字符串形式（兼容 "id" / "ID" 两种键名）。
func (s ormSite) RealID() string {
	if s.ID != 0 {
		return s.ID.String()
	}
	return s.IDBig.String()
}

// UnmarshalJSON 在解析站点字段的同时，原样保留原始 JSON 用于回写。
func (s *ormSite) UnmarshalJSON(b []byte) error {
	type alias ormSite
	var a alias
	if err := json.Unmarshal(b, &a); err != nil {
		return err
	}
	*s = ormSite(a)
	s.Raw = make(json.RawMessage, len(b))
	copy(s.Raw, b)
	return nil
}

// ormParseSites 解析 GET /api/v1/admin/sites 响应 {"sites":[...]}。
func ormParseSites(body []byte) ([]ormSite, error) {
	var resp struct {
		Sites []ormSite `json:"sites"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("%s", i18n.T("error.deploy_panel_list_parse", "Err", err.Error()))
	}
	return resp.Sites, nil
}

// ormFindSite 从站点列表中按站点 id 取出完整站点对象。
func ormFindSite(sites []ormSite, siteID string) (ormSite, error) {
	for _, s := range sites {
		if s.RealID() == siteID {
			return s, nil
		}
	}
	if len(sites) > 0 {
		return sites[0], nil
	}
	return ormSite{}, fmt.Errorf("未找到站点 %s", siteID)
}

// ormSiteWithCertID 基于站点原始 JSON 覆盖 cert_id 后返回回写体（map[string]json.RawMessage），
// 其余字段保持原样透传（doPutJSONRequest 会将其原样序列化，不解析/猜测站点内部结构）。
func ormSiteWithCertID(raw json.RawMessage, certID uint) (map[string]json.RawMessage, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("%s", i18n.T("error.deploy_panel_list_parse", "Err", err.Error()))
	}
	m["cert_id"] = json.RawMessage(strconv.FormatUint(uint64(certID), 10))
	return m, nil
}

// ormCertDomains 由证书 PEM 提取域名，拼接为 orm certs 接口要求的 JSON 数组字符串。
func ormCertDomains(certPEM string) string {
	domains := []string{}
	if cc, err := parseCertPEM(certPEM); err == nil {
		domains = append(domains, cc.SANs...)
		if len(domains) == 0 && cc.CommonName != "" {
			domains = append(domains, cc.CommonName)
		}
	}
	if len(domains) == 0 {
		domains = []string{"certflow.local"}
	}
	b, _ := json.Marshal(domains)
	return string(b)
}

// ormCert OpenResty Manager 证书对象（GET /api/v1/admin/certs 中单个证书）。
// 实测该接口直接返回裸数组 [...]（非 {"certs":[...]}），ormParseCerts 统一兼容两种外层。
// crt / key 为完整 PEM 文本（含换行），domains 为 JSON 数组字符串（如 "[\"a\",\"b\"]"）。
type ormCert struct {
	ID            uint   `json:"id"`
	IDBig         uint   `json:"ID"`
	Name          string `json:"name"`
	UID           uint   `json:"uid"`
	Type          int    `json:"type"`
	DNSChallenge  bool   `json:"dns_challenge"`
	DNSProvider   string `json:"dns_provider"`
	DNSCredential string `json:"dns_credential"`
	Domains       string `json:"domains"`
	Email         string `json:"email"`
	CRT           string `json:"crt"`
	Key           string `json:"key"`
	Expires       string `json:"expires"`
}

// RealID 返回证书 id（兼容 "id" / "ID" 两种键名）。
func (c ormCert) RealID() uint {
	if c.ID != 0 {
		return c.ID
	}
	return c.IDBig
}

// ormParseCerts 解析 GET /api/v1/admin/certs 响应（兼容裸数组与 {"certs":[...]}）。
func ormParseCerts(body []byte) ([]ormCert, error) {
	var arr []ormCert
	if err := json.Unmarshal(body, &arr); err == nil {
		return arr, nil
	}
	var obj struct {
		Certs []ormCert `json:"certs"`
	}
	if err := json.Unmarshal(body, &obj); err != nil {
		return nil, fmt.Errorf("%s", i18n.T("error.deploy_panel_list_parse", "Err", err.Error()))
	}
	return obj.Certs, nil
}

// ormFindCertIDByName 从证书列表中按证书名查找 id。
// addCert 响应仅返回 "OK" 不含 id，故上传后需按唯一证书名匹配。
func ormFindCertIDByName(certs []ormCert, name string) (uint, error) {
	for _, c := range certs {
		if c.Name == name {
			return c.RealID(), nil
		}
	}
	return 0, fmt.Errorf("未找到证书 %s", name)
}

// ormFindCertByID 从证书列表中按 id 取出证书对象。
func ormFindCertByID(certs []ormCert, certID uint) (ormCert, error) {
	for _, c := range certs {
		if c.RealID() == certID {
			return c, nil
		}
	}
	return ormCert{}, fmt.Errorf("未找到证书 id=%d", certID)
}

// ormSameCertContent 比较面板已有证书 crt 与本次待部署证书内容是否一致
// （按序列号 + SAN 比对，避免 PEM 文本格式/换行差异误判为新证书）。
// 内容一致时复用已有证书，避免每次部署都新建证书、产生大量孤儿证书。
func ormSameCertContent(existingCRT, newCertPEM string) bool {
	exist, err := parseCertPEM(existingCRT)
	if err != nil {
		return false
	}
	got, err := parseCertPEM(newCertPEM)
	if err != nil {
		return false
	}
	if exist.SerialNumber != got.SerialNumber {
		return false
	}
	if len(exist.SANs) != len(got.SANs) {
		return false
	}
	set := make(map[string]struct{}, len(got.SANs))
	for _, s := range got.SANs {
		set[s] = struct{}{}
	}
	for _, s := range exist.SANs {
		if _, ok := set[s]; !ok {
			return false
		}
	}
	return true
}

// ListSites 列出 OpenResty Manager 站点，用于前端「获取网站」下拉。
// 调用 GET /api/v1/admin/sites，响应结构 {"sites":[...]}。
func (d OpenRestyManagerDeployer) ListSites(ctx context.Context, creds Credentials, _, _, _ string) ([]string, error) {
	if creds.PanelURL == "" {
		return nil, fmt.Errorf("%s", i18n.T("error.deploy_panel_no_url"))
	}
	client := newPanelClient(creds.PanelURL, creds.AccessKeyID, creds.AccessKeySecret, nil, ormAuth)
	body, err := client.doGetRequest(ctx, "/api/v1/admin/sites", nil)
	if err != nil {
		return nil, err
	}
	sites, err := ormParseSites(body)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(sites))
	for _, s := range sites {
		name := s.Name
		id := s.RealID()
		if name == "" {
			name = id
		}
		out = append(out, name+"||"+id)
	}
	return out, nil
}

// DeployCert 将证书部署到 OpenResty Manager 站点（绑定证书）。
// 流程：拉取完整站点 → 上传证书（type=1）→ 按名查证书 id → 写回站点 cert_id → PUT 站点。
func (d OpenRestyManagerDeployer) DeployCert(ctx context.Context, creds Credentials, _ string, _ string, svc map[string]string) (*DeployResult, error) {
	siteID := svc["site_id"]
	if siteID == "" {
		// 兜底：从历史「名称||ID」形式解析（兼容旧配置/手动调用）
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
	// 归一化私钥（PKCS#8 → RSA/EC）并做本地 key/cert 匹配预校验，提前暴露证书数据问题。
	keyPEM = normalizePrivateKeyPEM(keyPEM)
	if err := verifyKeyCertMatch(keyPEM, certPEM); err != nil {
		return nil, fmt.Errorf("%s: %s (%s)", i18n.T("error.deploy_panel_cert_verify"), err.Error(), keyCertMismatchDetail(keyPEM, certPEM))
	}
	if creds.PanelURL == "" {
		return nil, fmt.Errorf("%s", i18n.T("error.deploy_panel_no_url"))
	}
	client := newPanelClient(creds.PanelURL, creds.AccessKeyID, creds.AccessKeySecret, nil, ormAuth)
	logging.Debug("openrestymanager DeployCert: siteID=%s keyType=%s", siteID, keyTypeOf(keyPEM))

	// 1) 拉取站点列表，定位目标站点（保留完整对象用于回写，避免丢配置）
	siteBody, err := client.doGetRequest(ctx, "/api/v1/admin/sites", nil)
	if err != nil {
		return nil, fmt.Errorf("%s: %s", i18n.T("error.deploy_panel_cert_deploy"), err.Error())
	}
	sites, err := ormParseSites(siteBody)
	if err != nil {
		return nil, fmt.Errorf("%s: %s", i18n.T("error.deploy_panel_cert_deploy"), err.Error())
	}
	site, err := ormFindSite(sites, siteID)
	if err != nil {
		return nil, fmt.Errorf("%s: %s", i18n.T("error.deploy_panel_cert_deploy"), err.Error())
	}

	// 2) 拉取证书列表，按内容去重：相同证书（序列号 + SAN 一致）直接复用，
	// 不再每次都新建证书，避免面板积累大量孤儿证书、证书 id 不断变化。
	certBody, err := client.doGetRequest(ctx, "/api/v1/admin/certs", nil)
	if err != nil {
		return nil, fmt.Errorf("%s: %s", i18n.T("error.deploy_panel_cert_deploy"), err.Error())
	}
	certs, err := ormParseCerts(certBody)
	if err != nil {
		return nil, fmt.Errorf("%s: %s", i18n.T("error.deploy_panel_cert_deploy"), err.Error())
	}
	certID := uint(0)
	for _, c := range certs {
		if c.CRT == "" {
			continue
		}
		if ormSameCertContent(c.CRT, certPEM) {
			certID = c.RealID()
			break
		}
	}

	// 3) 未找到相同证书才上传新证书（type=1 手动上传）；addCert 响应仅返回 OK，无 id，需按名查回
	if certID == 0 {
		certName := fmt.Sprintf("certflow-%s-%d", siteID, time.Now().Unix())
		certReq := map[string]any{
			"name":    certName,
			"type":    1,
			"crt":     certPEM,
			"key":     keyPEM,
			"domains": ormCertDomains(certPEM),
		}
		if _, err := client.doJSONRequest(ctx, "/api/v1/admin/certs", certReq); err != nil {
			return nil, fmt.Errorf("%s: %s", i18n.T("error.deploy_panel_cert_deploy"), err.Error())
		}
		certBody2, err := client.doGetRequest(ctx, "/api/v1/admin/certs", nil)
		if err != nil {
			return nil, fmt.Errorf("%s: %s", i18n.T("error.deploy_panel_cert_deploy"), err.Error())
		}
		certs2, err := ormParseCerts(certBody2)
		if err != nil {
			return nil, fmt.Errorf("%s: %s", i18n.T("error.deploy_panel_cert_deploy"), err.Error())
		}
		certID, err = ormFindCertIDByName(certs2, certName)
		if err != nil {
			return nil, fmt.Errorf("%s: %s", i18n.T("error.deploy_panel_cert_deploy"), err.Error())
		}
	}

	// 4) 将站点 cert_id 指向新证书并回写完整站点对象（PUT 替换，保留其余字段）
	putBody, err := ormSiteWithCertID(site.Raw, certID)
	if err != nil {
		return nil, fmt.Errorf("%s: %s", i18n.T("error.deploy_panel_cert_deploy"), err.Error())
	}
	if _, err := client.doPutJSONRequest(ctx, "/api/v1/admin/sites", putBody); err != nil {
		return nil, fmt.Errorf("%s: %s", i18n.T("error.deploy_panel_cert_deploy"), err.Error())
	}

	return &DeployResult{
		CloudCertID: siteID,
		Message:     i18n.T("deploy.panel_ssl_set"),
	}, nil
}

// GetCurrentCert 查询 OpenResty Manager 站点当前生效证书。
// 站点持有 cert_id，按 id 查证书列表取出 crt，解析其叶子证书组装 CurrentCert。
func (d OpenRestyManagerDeployer) GetCurrentCert(ctx context.Context, creds Credentials, _ string, svc map[string]string) (*CurrentCert, error) {
	logging.Debug("openrestymanager GetCurrentCert: siteID=%s", svc["site_id"])
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
	client := newPanelClient(creds.PanelURL, creds.AccessKeyID, creds.AccessKeySecret, nil, ormAuth)
	siteBody, err := client.doGetRequest(ctx, "/api/v1/admin/sites", nil)
	if err != nil {
		return nil, i18n.Wrap(err, "deploy.error.current_cert_query")
	}
	sites, err := ormParseSites(siteBody)
	if err != nil {
		return nil, i18n.Wrap(err, "deploy.error.current_cert_query")
	}
	site, err := ormFindSite(sites, siteID)
	if err != nil {
		return nil, i18n.Wrap(err, "deploy.error.current_cert_query")
	}
	certID := site.CertID.Uint()
	if certID == 0 {
		return nil, i18n.NewError("deploy.error.current_cert_not_configured")
	}
	certBody, err := client.doGetRequest(ctx, "/api/v1/admin/certs", nil)
	if err != nil {
		return nil, i18n.Wrap(err, "deploy.error.current_cert_query")
	}
	certs, err := ormParseCerts(certBody)
	if err != nil {
		return nil, i18n.Wrap(err, "deploy.error.current_cert_query")
	}
	cert, err := ormFindCertByID(certs, certID)
	if err != nil {
		return nil, i18n.Wrap(err, "deploy.error.current_cert_query")
	}
	crt := strings.TrimSpace(cert.CRT)
	if crt == "" {
		return nil, i18n.NewError("deploy.error.current_cert_no_pem")
	}
	return parseCertPEM(crt)
}
