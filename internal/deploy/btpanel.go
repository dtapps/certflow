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

// BTPanelDeployer 宝塔面板部署器。
// 凭证：api_key（仅需 API Key，无 AppSecret）。
// 部署目标：panel_url 标识面板地址，site 标识待部署站点。
// 鉴权签名见本文件的 btPanelAuth；设置证书到站点（DeployCert）仍待用户提供接口后覆盖实现。
type BTPanelDeployer struct {
	panelDeployerBase
}

func init() { RegisterDeployer(BTPanelDeployer{panelDeployerBase{provider: ProviderBTPanel}}) }

// btPanelSiteItem 站点列表条目（get_list，只解析用到的字段）。
type btPanelSiteItem struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	PS   string `json:"ps"`
}

// btPanelSiteListResponse /mod/proxy/com/get_list 响应（宝塔 mod 系嵌套 data.data）。
type btPanelSiteListResponse struct {
	Data struct {
		Data []btPanelSiteItem `json:"data"`
	} `json:"data"`
}

// btPanelAuth 生成宝塔面板鉴权签名：md5(timestamp + md5(api_key))。
// 请求头 BT-PANEL-APIKEY/TIMESTAMP/SIGNATURE，同时表单附带 request_time/request_token（接口要求两者皆有）。
func btPanelAuth(c *PanelClient, _, _ string, _ url.Values, _ []byte) (http.Header, url.Values, error) {
	h := http.Header{}
	form := url.Values{}
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sign := md5Hex(ts + md5Hex(c.APIKey))
	h.Set("BT-PANEL-APIKEY", c.APIKey)
	h.Set("BT-PANEL-TIMESTAMP", ts)
	h.Set("BT-PANEL-SIGNATURE", sign)
	form.Set("request_time", ts)
	form.Set("request_token", sign)
	return h, form, nil
}

// ListSites 列出宝塔面板站点（网站），用于前端「获取网站」下拉。
// 调用 /mod/proxy/com/get_list（POST 表单参数 p/limit），宝塔 mod 系返回嵌套结构 data.data。
// 站点 ID 用于后续 DeployCert 定位具体站点。
func (d BTPanelDeployer) ListSites(ctx context.Context, creds Credentials, _, _, _ string) ([]string, error) {
	if creds.PanelURL == "" {
		return nil, fmt.Errorf("%s", i18n.T("error.deploy_panel_no_url"))
	}
	client := newPanelClient(creds.PanelURL, creds.AccessKeyID, "", nil, btPanelAuth)
	form := url.Values{}
	form.Set("p", "1")
	form.Set("limit", "100")
	body, err := client.doRequest(ctx, "/mod/proxy/com/get_list", form)
	if err != nil {
		return nil, err
	}
	// 宝塔 /mod 系返回格式：{ "data": { "data": [ {id,name,ps,...}, ... ], "page": {...} } }
	var resp btPanelSiteListResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("%s", i18n.T("error.deploy_panel_list_parse", "Err", err.Error()))
	}
	out := make([]string, 0, len(resp.Data.Data))
	for _, s := range resp.Data.Data {
		name := s.Name
		if name == "" {
			name = s.PS
		}
		out = append(out, name+"||"+strconv.Itoa(s.ID))
	}
	return out, nil
}

// DeployCert 将证书部署到宝塔面板站点。
// 调用 /mod/proxy/com/set_ssl（POST 表单：site_name/key/cert）。
// svc["site_name"] 为站点名称（单站字符串或多站 JSON 数组），svc["cert_pem"]/svc["key_pem"] 为证书内容。
func (d BTPanelDeployer) DeployCert(ctx context.Context, creds Credentials, _ string, _ string, svc map[string]string) (*DeployResult, error) {
	panelURL := creds.PanelURL
	if panelURL == "" {
		if v, ok := svc["panel_url"]; ok && v != "" {
			panelURL = v
		} else {
			return nil, fmt.Errorf("%s", i18n.T("error.deploy_panel_no_url"))
		}
	}
	siteName := svc["site_name"]
	if siteName == "" {
		return nil, fmt.Errorf("%s", i18n.T("error.deploy_panel_no_site"))
	}
	certPEM := svc["cert_pem"]
	keyPEM := svc["key_pem"]

	// site_name 可能是 JSON 数组（多站部署）或单站字符串
	var sites []string
	if err := json.Unmarshal([]byte(siteName), &sites); err != nil {
		sites = []string{siteName}
	}
	if len(sites) == 0 {
		return nil, fmt.Errorf("%s", i18n.T("error.deploy_panel_no_site"))
	}

	client := newPanelClient(panelURL, creds.AccessKeyID, "", nil, btPanelAuth)

	// 先验证证书链
	verifyForm := url.Values{}
	verifyForm.Set("key", keyPEM)
	verifyForm.Set("cert", certPEM)
	if _, err := client.doRequest(ctx, "/ssl/cert/verify_certificate_chain", verifyForm); err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("error.deploy_panel_cert_verify"), err)
	}

	// 逐站设置 SSL
	var lastRaw string
	for _, name := range sites {
		form := url.Values{}
		form.Set("site_name", name)
		form.Set("key", keyPEM)
		form.Set("csr", certPEM)
		body, err := client.doRequest(ctx, "/mod/proxy/com/set_ssl", form)
		if err != nil {
			return nil, err
		}
		lastRaw = string(body)
	}
	return &DeployResult{CloudCertID: "", Message: i18n.T("deploy.panel_ssl_set"), RawResponse: lastRaw}, nil
}

// btPanelCertData GetSSL 响应中的 cert_data 摘要（仅声明用到的字段）。
type btPanelCertData struct {
	IssuerO  string   `json:"issuer_O"`
	Issuer   string   `json:"issuer"`
	NotAfter string   `json:"notAfter"`
	Subject  string   `json:"subject"`
	DNS      []string `json:"dns"`
}

// btPanelGetSSLResponse POST /site?action=GetSSL 响应（返回站点当前生效证书）。
type btPanelGetSSLResponse struct {
	Status   bool            `json:"status"`
	CSR      string          `json:"csr"`  // 完整证书链 PEM（含叶子+中间+根）
	Cert     string          `json:"cert"` // 兜底字段
	CertData btPanelCertData `json:"cert_data"`
}

// GetCurrentCert 查询宝塔面板站点当前生效证书。
// 调用 POST /site?action=GetSSL（siteName=站点名）获取当前生效证书链（csr 字段为完整 PEM），
// 解析其叶子证书组装 CurrentCert，统一输出 RFC3339，与本地证书对比逻辑对齐。
func (d BTPanelDeployer) GetCurrentCert(ctx context.Context, creds Credentials, _ string, svc map[string]string) (*CurrentCert, error) {
	logging.Debug(i18n.T("log.deploy.btpanel_get_current_cert",
		"SiteName", svc["site_name"]))
	panelURL := creds.PanelURL
	if panelURL == "" {
		if v, ok := svc["panel_url"]; ok && v != "" {
			panelURL = v
		} else {
			return nil, fmt.Errorf("%s", i18n.T("error.deploy_panel_no_url"))
		}
	}
	siteName := svc["site_name"]
	if siteName == "" {
		return nil, fmt.Errorf("%s", i18n.T("error.deploy_panel_no_site"))
	}
	// svc["site_name"] 可能形如 "name||id"（来自站点列表），仅取站点名部分作为 GetSSL 入参。
	if parts := strings.SplitN(siteName, "||", 2); len(parts) == 2 {
		siteName = strings.TrimSpace(parts[0])
	}
	client := newPanelClient(panelURL, creds.AccessKeyID, "", nil, btPanelAuth)
	form := url.Values{}
	form.Set("siteName", siteName)
	body, err := client.doRequest(ctx, "/site?action=GetSSL", form)
	if err != nil {
		return nil, i18n.Wrap(err, "deploy.error.current_cert_query")
	}
	var resp btPanelGetSSLResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, i18n.Wrap(err, "deploy.error.current_cert_query")
	}
	pem := strings.TrimSpace(resp.CSR)
	if pem == "" {
		pem = strings.TrimSpace(resp.Cert)
	}
	if pem == "" {
		return nil, i18n.NewError("deploy.error.current_cert_not_configured")
	}
	return parseCertPEM(pem)
}
