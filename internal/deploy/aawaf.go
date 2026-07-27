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
)

// AAWafDeployer 堡塔云WAF 防火墙部署器。
// 凭证：仅需 api_key / api_sk（映射到 AccessKeyID，无 Secret）。
// 部署目标：panel_url 标识防火墙地址，site 标识待部署站点。
// ListSites 与 DeployCert 均已实现（/api/wafmastersite/get_site_list、modify_site）。
type AAWafDeployer struct {
	panelDeployerBase
}

func init() { RegisterDeployer(AAWafDeployer{panelDeployerBase{provider: ProviderAAWaf}}) }

// aawafSite 堡塔云WAF 网站条目（get_site_list 列表项，只解析用到的字段）。
type aawafSite struct {
	SiteName string `json:"site_name"`
	SiteID   string `json:"site_id"`
	Server   struct {
		ListenSSLPort []string `json:"listen_ssl_port"`
		SSL           struct {
			IsSSL int `json:"is_ssl"`
		} `json:"ssl"`
	} `json:"server"`
}

// aawafListResponse get_site_list 响应。
type aawafListResponse struct {
	Code int `json:"code"`
	Res  struct {
		List []aawafSite `json:"list"`
	} `json:"res"`
	Msg string `json:"msg"`
}

// aawafGetSiteListRequest get_site_list 请求体。
type aawafGetSiteListRequest struct {
	SiteID   string `json:"site_id"`
	P        int    `json:"p"`
	PSize    int    `json:"p_size"`
	SiteName string `json:"site_name"`
}

// aawafModifySiteRequest modify_site 请求体（types=openCert 只需最小参数）：
// {"types":"openCert","site_id":"...","server":{"listen_ssl_port":["443"],"ssl":{"is_ssl":1,"private_key":"...","full_chain":"..."}}}
type aawafModifySiteRequest struct {
	Types  string            `json:"types"`
	SiteID string            `json:"site_id"`
	Server aawafModifyServer `json:"server"`
}

// aawafModifyServer modify_site 请求中的 server 部分（仅证书相关字段）。
type aawafModifyServer struct {
	ListenSSLPort []string       `json:"listen_ssl_port"`
	SSL           aawafModifySSL `json:"ssl"`
}

// aawafModifySSL modify_site 请求中的 ssl 部分。
type aawafModifySSL struct {
	IsSSL      int    `json:"is_ssl"`
	PrivateKey string `json:"private_key"`
	FullChain  string `json:"full_chain"`
}

// aawafResponse 通用响应（modify_site 等）。
type aawafResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

// aawafAuth 生成堡塔云WAF 鉴权签名：md5(timestamp + md5(api_key))。
// 请求头 waf_request_time / waf_request_token。
func aawafAuth(c *PanelClient, _, _ string, _ url.Values, _ []byte) (http.Header, url.Values, error) {
	h := http.Header{}
	form := url.Values{}
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	token := md5Hex(ts + md5Hex(c.APIKey))
	h.Set("waf_request_time", ts)
	h.Set("waf_request_token", token)
	return h, form, nil
}

// ListSites 列出堡塔云WAF 网站，用于前端「获取网站」下拉。
// 调用 /api/wafmastersite/get_site_list（POST JSON: {p, p_size, site_name}）。
func (d AAWafDeployer) ListSites(ctx context.Context, creds Credentials, _, _, _ string) ([]string, error) {
	if creds.PanelURL == "" {
		return nil, fmt.Errorf("%s", i18n.T("error.deploy_panel_no_url"))
	}
	client := newPanelClient(creds.PanelURL, creds.AccessKeyID, creds.AccessKeySecret, nil, aawafAuth)
	payload := aawafGetSiteListRequest{
		P:        1,
		PSize:    20,
		SiteName: "",
	}
	body, err := client.doJSONRequest(ctx, "/api/wafmastersite/get_site_list", payload)
	if err != nil {
		return nil, err
	}
	var resp aawafListResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("%s", i18n.T("error.deploy_panel_list_parse", "Err", err.Error()))
	}
	out := make([]string, 0, len(resp.Res.List))
	for _, item := range resp.Res.List {
		name := item.SiteName
		if name == "" {
			name = item.SiteID
		}
		out = append(out, name+"||"+item.SiteID)
	}
	return out, nil
}

// parseAAWafSite 从 get_site_list 响应中按 site_id 取出站点配置。
func parseAAWafSite(body []byte, siteID string) (aawafSite, error) {
	var resp aawafListResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return aawafSite{}, fmt.Errorf("%s", i18n.T("error.deploy_panel_list_parse", "Err", err.Error()))
	}
	if resp.Code != 0 {
		return aawafSite{}, fmt.Errorf("%s", resp.Msg)
	}
	for _, item := range resp.Res.List {
		if item.SiteID == siteID {
			return item, nil
		}
	}
	if len(resp.Res.List) > 0 {
		return resp.Res.List[0], nil
	}
	return aawafSite{}, fmt.Errorf("未找到站点 %s", siteID)
}

// DeployCert 将证书部署到堡塔云 WAF 站点（开启 HTTPS）。
// 流程：POST /api/wafmastersite/get_site_list 拉取站点现有 listen_ssl_port（不写死端口）
// → POST /api/wafmastersite/modify_site（types=openCert），server 只带
//
//	listen_ssl_port + ssl{is_ssl, private_key, full_chain} 最小参数，与面板前端请求一致。
func (d AAWafDeployer) DeployCert(ctx context.Context, creds Credentials, _ string, _ string, svc map[string]string) (*DeployResult, error) {
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

	client := newPanelClient(creds.PanelURL, creds.AccessKeyID, creds.AccessKeySecret, nil, aawafAuth)
	logging.Debug("aawaf DeployCert: siteID=%s keyType=%s", siteID, keyTypeOf(keyPEM))

	// 1) 拉取站点现有 listen_ssl_port 与 is_ssl（均不写死）
	detailBody, err := client.doJSONRequest(ctx, "/api/wafmastersite/get_site_list", aawafGetSiteListRequest{
		SiteID:   siteID,
		P:        1,
		PSize:    10,
		SiteName: "",
	})
	if err != nil {
		return nil, fmt.Errorf("%s: %s", i18n.T("error.deploy_panel_cert_deploy"), err.Error())
	}
	site, err := parseAAWafSite(detailBody, siteID)
	if err != nil {
		return nil, fmt.Errorf("%s: %s", i18n.T("error.deploy_panel_cert_deploy"), err.Error())
	}
	sslPorts := site.Server.ListenSSLPort

	// 2) 按面板前端的最小参数结构发起 modify_site（types=openCert）
	payload := aawafModifySiteRequest{
		Types:  "openCert",
		SiteID: siteID,
		Server: aawafModifyServer{
			ListenSSLPort: sslPorts,
			SSL: aawafModifySSL{
				IsSSL:      site.Server.SSL.IsSSL,
				PrivateKey: keyPEM,
				FullChain:  certPEM,
			},
		},
	}
	body, err := client.doJSONRequest(ctx, "/api/wafmastersite/modify_site", payload)
	if err != nil {
		return nil, fmt.Errorf("%s: %s", i18n.T("error.deploy_panel_cert_deploy"), err.Error())
	}
	var resp aawafResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("%s: %s", i18n.T("error.deploy_panel_cert_deploy"), err.Error())
	}
	if resp.Code != 0 {
		return nil, fmt.Errorf("%s: %s", i18n.T("error.deploy_panel_cert_deploy"), resp.Msg)
	}
	return &DeployResult{
		CloudCertID: siteID,
		Message:     i18n.T("deploy.panel_ssl_set"),
	}, nil
}
