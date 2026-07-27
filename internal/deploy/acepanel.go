package deploy

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"cnb.cool/dtapp/certflow/internal/i18n"
	"cnb.cool/dtapp/certflow/internal/logging"
)

// AcePanelDeployer AcePanel 面板部署器。
// 凭证：token_id（令牌 ID）+ token_secret（令牌密钥），HMAC-SHA256 签名为 HMAC(token_secret, stringToSign)。
// 部署目标：panel_url 标识面板地址，site 标识待部署站点。
// ListSites / DeployCert 均已覆盖实现。
type AcePanelDeployer struct {
	panelDeployerBase
}

func init() { RegisterDeployer(AcePanelDeployer{panelDeployerBase{provider: ProviderAcePanel}}) }

// acePanelSite AcePanel 网站条目。
type acePanelSite struct {
	ID      int      `json:"id"`
	Name    string   `json:"name"`
	Domains []string `json:"domains"`
}

// displayName 返回用于展示的网站名称：优先取 domains 第一个元素（真实域名），回退到 name（站点标识），最后回退 ID。
func (s acePanelSite) displayName() string {
	if len(s.Domains) > 0 && s.Domains[0] != "" {
		return s.Domains[0]
	}
	if s.Name != "" {
		return s.Name
	}
	return strconv.Itoa(s.ID)
}

// acePanelListResponse AcePanel 网站列表响应。
type acePanelListResponse struct {
	Msg  string `json:"msg"`
	Data struct {
		Items []acePanelSite `json:"items"`
		Total int            `json:"total"`
	} `json:"data"`
}

// acePanelUploadCertRequest 上传证书请求体（POST /api/cert/cert/upload）。
type acePanelUploadCertRequest struct {
	Cert string `json:"cert"`
	Key  string `json:"key"`
}

// acePanelUploadCertResponse 上传证书响应：
// {"msg":"...","data":{"id":N}|N}，data 形态不定（对象或裸数字），用 RawMessage 兼容。
type acePanelUploadCertResponse struct {
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

// acePanelCertIDData 上传证书响应 data 对象（id/cert_id 可能为数字或字符串）。
type acePanelCertIDData struct {
	ID     any `json:"id"`
	CertID any `json:"cert_id"`
}

// acePanelDeployCertRequest 部署证书请求体（POST /api/cert/cert/{id}/deploy）。
type acePanelDeployCertRequest struct {
	ID          int  `json:"id"`
	WebsiteID   int  `json:"website_id"`
	EnableHTTPS bool `json:"enable_https"`
}

// acePanelDeployCertResponse 部署证书响应。
type acePanelDeployCertResponse struct {
	Msg string `json:"msg"`
}

// acePanelCertListResponse GET /api/cert/cert 证书库列表响应。
type acePanelCertListResponse struct {
	Msg  string `json:"msg"`
	Data struct {
		Items []acePanelCertItem `json:"items"`
		Total int                `json:"total"`
	} `json:"data"`
}

// acePanelAuth 生成 AcePanel 鉴权签名：HMAC-SHA256(token_secret, stringToSign)。
// 请求头 X-Timestamp / Authorization: HMAC-SHA256 Credential=<token_id>, Signature=<sig>（逗号+空格）。
// TODO(确认)：Credential 取值与 Signature 明文是否参与，待用户给官方算法。
// acePanelAuth 实现 AcePanel 鉴权签名（https://acepanel.net/advanced/api）。
// 签名算法：
//  1. 规范化请求 = METHOD + "\n" + 规范化路径 + "\n" + 查询串 + "\n" + SHA256(请求体)
//  2. 待签名字符串 = "HMAC-SHA256" + "\n" + 时间戳 + "\n" + SHA256(规范化请求)
//  3. 签名 = HMAC-SHA256(待签名字符串, token)
//
// 其中 规范化路径 需剥离 /api 之前的前缀（如 /entrance），查询串需 URL 编码。
func acePanelAuth(c *PanelClient, method, path string, query url.Values, body []byte) (http.Header, url.Values, error) {
	h := http.Header{}
	form := url.Values{}

	// 规范化路径：确保以 /api 开头
	canonicalPath := path
	if !strings.HasPrefix(canonicalPath, "/api") {
		if idx := strings.Index(canonicalPath, "/api"); idx != -1 {
			canonicalPath = canonicalPath[idx:]
		}
	}

	// 查询串 URL 编码（url.Values.Encode() 自动按 key 排序）
	queryStr := ""
	if query != nil {
		queryStr = query.Encode()
	}

	// 请求体哈希（GET 为空体，SHA256 空串固定值）
	bodyHash := sha256Hex(string(body))

	// 步骤 1：构造规范化请求
	canonicalRequest := fmt.Sprintf("%s\n%s\n%s\n%s", method, canonicalPath, queryStr, bodyHash)

	// 步骤 2：构造待签名字符串
	ts := time.Now().Unix()
	tsStr := strconv.FormatInt(ts, 10)
	stringToSign := fmt.Sprintf("HMAC-SHA256\n%d\n%s", ts, sha256Hex(canonicalRequest))

	// 步骤 3：计算签名（token = APISecret）
	signature := hmacSHA256Hex(c.APISecret, stringToSign)

	// 步骤 4：设置请求头
	h.Set("X-Timestamp", tsStr)
	h.Set("Authorization", fmt.Sprintf("HMAC-SHA256 Credential=%s, Signature=%s", c.APIKey, signature))
	return h, form, nil
}

// ListSites 列出 AcePanel 网站，用于前端「获取网站」下拉。
// 调用 /api/website?type=all&page=1&limit=100（GET 查询参数）。
// AcePanel 返回格式：{ "msg": "success", "data": { "items": [ {id,domain,...} ], "total": N } }。
func (d AcePanelDeployer) ListSites(ctx context.Context, creds Credentials, _, _, _ string) ([]string, error) {
	if creds.PanelURL == "" {
		return nil, fmt.Errorf("%s", i18n.T("error.deploy_panel_no_url"))
	}
	// 提取安全入口路径（如 /u5GJ9L），AcePanel API 必须带此路径才能正确路由
	pathPrefix := ""
	if u, err := url.Parse(creds.PanelURL); err == nil && u.Path != "" && u.Path != "/" {
		pathPrefix = strings.TrimRight(u.Path, "/")
	}
	client := newPanelClientWithPath(creds.PanelURL, pathPrefix, creds.AccessKeyID, creds.AccessKeySecret, nil, acePanelAuth)
	query := url.Values{}
	query.Set("type", "all")
	query.Set("page", "1")
	query.Set("limit", "100")
	body, err := client.doGetRequest(ctx, "/api/website", query)
	if err != nil {
		return nil, err
	}
	var resp acePanelListResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("%s", i18n.T("error.deploy_panel_list_parse", "Err", err.Error()))
	}
	out := make([]string, 0, len(resp.Data.Items))
	for _, item := range resp.Data.Items {
		name := item.displayName()
		out = append(out, name+"||"+strconv.Itoa(item.ID))
	}
	return out, nil
}

// DeployCert 将证书部署到 AcePanel 网站。
// AcePanel 提供独立证书库，标准流程分两步（均为 POST + JSON）：
//  1. POST /api/cert/cert/upload 上传证书（body: {"cert":..., "key":...}），返回证书 id；
//  2. POST /api/cert/cert/{id}/deploy 部署到站点（body: {"id":证书id,"website_id":站点id,"enable_https":true}）。
//
// 该方式比「全量 PUT 替换站点对象」更稳健，也规避了隧道入口在 PUT 复用连接上写出非法响应头的 bug。
func (d AcePanelDeployer) DeployCert(ctx context.Context, creds Credentials, _ string, _ string, svc map[string]string) (*DeployResult, error) {
	siteIDStr := svc["site_id"]
	if siteIDStr == "" {
		// 兜底：从历史「名称||ID」形式或纯站点名解析（兼容旧配置/手动调用）
		if name := svc["site_name"]; name != "" {
			if parts := strings.SplitN(name, "||", 2); len(parts) == 2 {
				siteIDStr = strings.TrimSpace(parts[1])
			}
		}
	}
	if siteIDStr == "" {
		return nil, fmt.Errorf("%s: %s", i18n.T("error.deploy_panel_cert_deploy"), "站点 ID 缺失")
	}
	siteID, err := strconv.Atoi(strings.TrimSpace(siteIDStr))
	if err != nil {
		return nil, fmt.Errorf("%s: %s", i18n.T("error.deploy_panel_cert_deploy"), "站点 id 解析失败: "+siteIDStr)
	}
	certPEM := svc["cert_pem"]
	keyPEM := svc["key_pem"]
	if certPEM == "" || keyPEM == "" {
		return nil, fmt.Errorf("%s: %s", i18n.T("error.deploy_panel_cert_deploy"), "证书或私钥为空")
	}
	// AcePanel 接受传统格式私钥；对 PKCS#8 归一化，并做本地 key/cert 匹配预校验，提前暴露证书数据问题。
	keyPEM = normalizePrivateKeyPEM(keyPEM)
	if err := verifyKeyCertMatch(keyPEM, certPEM); err != nil {
		return nil, fmt.Errorf("%s: %s (%s)", i18n.T("error.deploy_panel_cert_verify"), err.Error(), keyCertMismatchDetail(keyPEM, certPEM))
	}

	// 提取安全入口路径（如 /u5GJ9L），AcePanel API 必须带此路径才能正确路由
	pathPrefix := ""
	if u, err := url.Parse(creds.PanelURL); err == nil && u.Path != "" && u.Path != "/" {
		pathPrefix = strings.TrimRight(u.Path, "/")
	}
	client := newPanelClientWithPath(creds.PanelURL, pathPrefix, creds.AccessKeyID, creds.AccessKeySecret, nil, acePanelAuth)

	logging.Debug("acepanel DeployCert: siteID=%d keyType=%s", siteID, keyTypeOf(keyPEM))

	// 1) 先查证书列表，若面板证书库里已有同一张证书则直接复用其 id，避免重复上传
	certID, found := findExistingAcePanelCert(ctx, client, certPEM)
	if found {
		logging.Debug("acepanel reuse existing cert id=%d", certID)
	} else {
		// 2) 未命中才上传证书，拿到证书 id（请求体含私钥，不打印原文）
		uploadBody := acePanelUploadCertRequest{
			Cert: certPEM,
			Key:  keyPEM,
		}
		uploadResp, err := client.doJSONRequest(ctx, "/api/cert/cert/upload", uploadBody)
		if err != nil {
			return nil, fmt.Errorf("%s: %s", i18n.T("error.deploy_panel_cert_deploy"), err.Error())
		}
		certID, err = parseAcePanelCertID(uploadResp)
		if err != nil {
			return nil, fmt.Errorf("%s: %s", i18n.T("error.deploy_panel_cert_deploy"), err.Error())
		}
		logging.Debug("acepanel upload cert id=%d", certID)
	}

	// 3) 部署证书到站点（开启 HTTPS）
	deployBody := acePanelDeployCertRequest{
		ID:          certID,
		WebsiteID:   siteID,
		EnableHTTPS: true,
	}
	deployResp, err := client.doJSONRequest(ctx, fmt.Sprintf("/api/cert/cert/%d/deploy", certID), deployBody)
	if err != nil {
		return nil, fmt.Errorf("%s: %s", i18n.T("error.deploy_panel_cert_deploy"), err.Error())
	}
	var deployResult acePanelDeployCertResponse
	if err := json.Unmarshal(deployResp, &deployResult); err != nil {
		return nil, fmt.Errorf("%s: %s", i18n.T("error.deploy_panel_cert_deploy"), err.Error())
	}
	if deployResult.Msg != "" && deployResult.Msg != "success" {
		return nil, fmt.Errorf("%s: %s", i18n.T("error.deploy_panel_cert_deploy"), deployResult.Msg)
	}

	return &DeployResult{
		CloudCertID: strconv.Itoa(certID),
		Message:     i18n.T("deploy.panel_ssl_set"),
	}, nil
}

// findExistingAcePanelCert 在 AcePanel 证书库（GET /api/cert/cert?page=N&limit=100）中
// 查找与 certPEM 为同一张证书的条目，命中则返回其 id。
// 匹配策略（确定性优先，宁可漏配不可错配）：
//  1. 列表项含证书 PEM 内容（cert/certificate/pem 字段）时，按叶子证书 DER 的 SHA256 指纹比对；
//  2. 否则按序列号（serial_number/serial 字段，十进制或十六进制字符串）比对。
//
// acePanelCertItem AcePanel 证书库列表项（GET /api/cert/cert 返回），
// 仅声明匹配复用所需的字段：证书 id、PEM 内容、序列号。
type acePanelCertItem struct {
	ID           int    `json:"id"`
	SerialNumber string `json:"serial_number"`
	Serial       string `json:"serial"`
	Cert         string `json:"cert"`
}

// 查询失败或无法确定性匹配时返回 false，由调用方回退到上传流程（不阻断部署）。
func findExistingAcePanelCert(ctx context.Context, client *PanelClient, certPEM string) (int, bool) {
	wantFP, fpErr := certFingerprint(certPEM)
	var wantSerial string
	if der, err := leafCertDER(certPEM); err == nil {
		if leaf, err := x509.ParseCertificate(der); err == nil {
			wantSerial = leaf.SerialNumber.String()
		}
	}

	const limit = 100
	for page := 1; page <= 10; page++ { // 最多翻 10 页（1000 张），防御异常 total
		query := url.Values{}
		query.Set("page", strconv.Itoa(page))
		query.Set("limit", strconv.Itoa(limit))
		body, err := client.doGetRequest(ctx, "/api/cert/cert", query)
		if err != nil {
			logging.Debug("acepanel list certs failed: %v", err)
			return 0, false
		}
		var resp acePanelCertListResponse
		if err := json.Unmarshal(body, &resp); err != nil || (resp.Msg != "" && resp.Msg != "success") {
			logging.Debug("acepanel list certs parse failed: err=%v msg=%s", err, resp.Msg)
			return 0, false
		}
		for _, item := range resp.Data.Items {
			if item.ID == 0 {
				continue
			}
			// 策略 1：按证书内容指纹比对
			if fpErr == nil && strings.Contains(item.Cert, "BEGIN CERTIFICATE") {
				if fp, err := certFingerprint(item.Cert); err == nil && bytes.Equal(fp, wantFP) {
					return item.ID, true
				}
			}
			// 策略 2：按序列号比对（兼容十进制/十六进制表示）
			if wantSerial != "" && (serialEqual(item.SerialNumber, wantSerial) || serialEqual(item.Serial, wantSerial)) {
				return item.ID, true
			}
		}
		if page*limit >= resp.Data.Total || len(resp.Data.Items) == 0 {
			break
		}
	}
	return 0, false
}

// serialEqual 比对证书序列号是否一致：a 可能是十进制或十六进制（可含冒号/空格分隔）字符串，
// wantDec 为本地证书序列号的十进制表示。
func serialEqual(a, wantDec string) bool {
	a = strings.TrimSpace(a)
	if a == wantDec {
		return true
	}
	// 尝试按十六进制解析（去掉常见分隔符与 0x 前缀）
	hexStr := strings.NewReplacer(":", "", " ", "", "-", "").Replace(a)
	hexStr = strings.TrimPrefix(strings.ToLower(hexStr), "0x")
	n := new(big.Int)
	if _, ok := n.SetString(hexStr, 16); ok {
		return n.String() == wantDec
	}
	return false
}

// parseAcePanelCertID 从「上传证书」响应的 data 中提取证书 id。
// AcePanel 返回格式未完全公开，这里兼容 data 为 {"id":N} / {"cert_id":N} / 裸数字 N 几种形态
// （id 可为数字或字符串）。
func parseAcePanelCertID(body []byte) (int, error) {
	var resp acePanelUploadCertResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return 0, fmt.Errorf("解析上传证书响应失败: %w", err)
	}
	if resp.Msg != "" && resp.Msg != "success" {
		return 0, fmt.Errorf("上传证书失败: %s", resp.Msg)
	}
	// data 为对象：{id|cert_id: N|"N"}
	var obj acePanelCertIDData
	if json.Unmarshal(resp.Data, &obj) == nil {
		for _, v := range []any{obj.ID, obj.CertID} {
			if id, ok := anyToInt(v); ok && id != 0 {
				return id, nil
			}
		}
	}
	// data 为裸数字
	var num int
	if json.Unmarshal(resp.Data, &num) == nil && num != 0 {
		return num, nil
	}
	return 0, fmt.Errorf("上传证书响应中未找到证书 id: %s", string(body))
}

// anyToInt 把 JSON 数字（float64）或字符串形式的整数转为 int。
func anyToInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	case string:
		if i, err := strconv.Atoi(strings.TrimSpace(n)); err == nil {
			return i, true
		}
	}
	return 0, false
}
