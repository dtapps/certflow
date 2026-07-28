package deploy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"cnb.cool/dtapp/certflow/internal/i18n"
	"cnb.cool/dtapp/certflow/internal/logging"
)

// AAPanelDeployer 宝塔国际版（aaPanel）部署器。
// 凭证：仅需 API Key（无 AppSecret）。
// 部署目标：panel_url 标识面板地址，site 标识待部署站点。
// aaPanel 使用 v2 接口（/v2/...），鉴权为查询参数 request_time(毫秒)/request_token（md5(时间+md5(key))），见 aaPanelAuth。
type AAPanelDeployer struct {
	panelDeployerBase
}

func init() { RegisterDeployer(AAPanelDeployer{panelDeployerBase{provider: ProviderAAPanel}}) }

// aaPanelSiteItem 站点列表条目（get_list，只解析用到的字段）。
type aaPanelSiteItem struct {
	ID   int            `json:"id"`
	Name string         `json:"name"`
	PS   string         `json:"ps"`
	SSL  aaPanelSiteSSL `json:"ssl"`
}

// aaPanelSiteSSL get_list 返回的站点 ssl 字段（当前生效证书概要，非完整 PEM）。
type aaPanelSiteSSL struct {
	Issuer    string   `json:"issuer"`
	IssuerO   string   `json:"issuer_O"`
	NotAfter  string   `json:"notAfter"`
	NotBefore string   `json:"notBefore"`
	DNS       []string `json:"dns"`
	Subject   string   `json:"subject"`
	Endtime   int      `json:"endtime"`
}

// aaPanelSiteListResponse /v2/mod/proxy/com/get_list 响应。
type aaPanelSiteListResponse struct {
	Message struct {
		Data struct {
			Data []aaPanelSiteItem `json:"data"`
		} `json:"data"`
	} `json:"message"`
}

// aaPanelUploadCertResponse upload_cert 响应：
// {"status":0,"timestamp":...,"message":{"hash":"...",...}}，hash 在 message.hash 中。
type aaPanelUploadCertResponse struct {
	Status  int    `json:"status"`
	Msg     string `json:"msg"`
	Message struct {
		Hash string `json:"hash"`
	} `json:"message"`
}

// aaPanelDeployResponse cert_deploy_sites 响应（message 可能是字符串或对象，用 RawMessage 兼容）。
type aaPanelDeployResponse struct {
	Status  int             `json:"status"`
	Msg     string          `json:"msg"`
	Message json.RawMessage `json:"message"`
}

// aaPanelAuth 生成 aaPanel v2 接口鉴权：
// request_token = md5(request_time + md5(api_key))，request_time 为毫秒时间戳；
// 两者作为查询参数附加到请求 URL（aaPanel v2 不依赖 BT-PANEL-APIKEY 请求头）。
func aaPanelAuth(c *PanelClient, _, _ string, _ url.Values, _ []byte) (http.Header, url.Values, error) {
	h := http.Header{}
	form := url.Values{}
	ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
	sign := md5Hex(ts + md5Hex(c.APIKey))
	form.Set("request_time", ts)
	form.Set("request_token", sign)
	return h, form, nil
}

// ListSites 列出 aaPanel 站点（网站），用于前端「获取网站」下拉。
// 调用 v2 接口 /v2/mod/proxy/com/get_list（POST 表单参数 p/limit）。
func (d AAPanelDeployer) ListSites(ctx context.Context, creds Credentials, _, _, _ string) ([]string, error) {
	if creds.PanelURL == "" {
		return nil, fmt.Errorf("%s", i18n.T("error.deploy_panel_no_url"))
	}
	client := newPanelClient(creds.PanelURL, creds.AccessKeyID, "", nil, aaPanelAuth)
	form := url.Values{}
	form.Set("p", "1")
	form.Set("limit", "100")
	body, err := client.doV2Request(ctx, "/v2/mod/proxy/com/get_list", nil, form)
	if err != nil {
		return nil, err
	}
	var resp aaPanelSiteListResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("%s", i18n.T("error.deploy_panel_list_parse", "Err", err.Error()))
	}
	out := make([]string, 0, len(resp.Message.Data.Data))
	for _, s := range resp.Message.Data.Data {
		name := s.Name
		if name == "" {
			name = s.PS
		}
		out = append(out, name+"||"+strconv.Itoa(s.ID))
	}
	return out, nil
}

// DeployCert 将证书部署到 aaPanel 站点。
// 先调用 /v2/ssl_domain?action=upload_cert 上传证书拿到 hash，
// 再调用 /v2/ssl_domain?action=cert_deploy_sites 把证书应用到指定站点。
// svc["site_name"] 为站点名称（单站字符串或多站 JSON 数组），svc["cert_pem"]/svc["key_pem"] 为证书内容。
func (d AAPanelDeployer) DeployCert(ctx context.Context, creds Credentials, _ string, _ string, svc map[string]string) (*DeployResult, error) {
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
	if certPEM == "" || keyPEM == "" {
		return nil, fmt.Errorf("%s", i18n.T("error.deploy_panel_no_cert"))
	}

	// site_name 可能是 JSON 数组（多站部署）或单站字符串
	var sites []string
	if err := json.Unmarshal([]byte(siteName), &sites); err != nil || len(sites) == 0 {
		sites = []string{siteName}
	}
	if len(sites) == 0 {
		return nil, fmt.Errorf("%s", i18n.T("error.deploy_panel_no_site"))
	}

	client := newPanelClient(panelURL, creds.AccessKeyID, "", nil, aaPanelAuth)

	// 1) 上传证书，拿到证书 hash
	upForm := url.Values{}
	upForm.Set("key", keyPEM)
	upForm.Set("cert", certPEM)
	upBody, err := client.doV2Request(ctx, "/v2/ssl_domain", url.Values{"action": {"upload_cert"}}, upForm)
	if err != nil {
		return nil, err
	}
	var upResp aaPanelUploadCertResponse
	if err := json.Unmarshal(upBody, &upResp); err != nil {
		return nil, fmt.Errorf("%s: %s", i18n.T("error.deploy_panel_cert_upload"), err.Error())
	}
	certHash := upResp.Message.Hash
	// aaPanel 约定 status=0 表示成功
	if upResp.Status != 0 || certHash == "" {
		return nil, fmt.Errorf("%s: %s", i18n.T("error.deploy_panel_cert_upload"), upResp.Msg)
	}

	// 2) 把证书部署到站点
	domainsJSON, err := json.Marshal(sites)
	if err != nil {
		return nil, err
	}
	depForm := url.Values{}
	depForm.Set("hash", certHash)
	depForm.Set("append", "1")
	depForm.Set("domains", string(domainsJSON))
	depBody, err := client.doV2Request(ctx, "/v2/ssl_domain", url.Values{"action": {"cert_deploy_sites"}}, depForm)
	if err != nil {
		return nil, err
	}
	var depResp aaPanelDeployResponse
	if err := json.Unmarshal(depBody, &depResp); err != nil {
		return nil, fmt.Errorf("%s: %s", i18n.T("error.deploy_panel_cert_deploy"), err.Error())
	}
	// aaPanel 约定 status=0 表示成功
	if depResp.Status != 0 {
		msg := depResp.Msg
		if msg == "" {
			var s string
			if json.Unmarshal(depResp.Message, &s) == nil {
				msg = s
			}
		}
		return nil, fmt.Errorf("%s: %s", i18n.T("error.deploy_panel_cert_deploy"), msg)
	}
	return &DeployResult{CloudCertID: certHash, Message: i18n.T("deploy.panel_ssl_set"), RawResponse: string(depBody)}, nil
}

// GetCurrentCert 查询 aaPanel 站点当前生效证书。
// aaPanel 站点列表 /v2/mod/proxy/com/get_list 的每条站点 ssl 字段已含当前生效证书概要
// （issuer/notAfter/notBefore/dns/subject），无需额外解析 PEM，直接组装 CurrentCert。
func (d AAPanelDeployer) GetCurrentCert(ctx context.Context, creds Credentials, _ string, svc map[string]string) (*CurrentCert, error) {
	logging.Debug(i18n.T("log.deploy.aapanel_get_current_cert",
		"SiteID", svc["site_id"],
		"SiteName", svc["site_name"]))
	panelURL := creds.PanelURL
	if panelURL == "" {
		if v, ok := svc["panel_url"]; ok && v != "" {
			panelURL = v
		} else {
			return nil, fmt.Errorf("%s", i18n.T("error.deploy_panel_no_url"))
		}
	}
	siteID := svc["site_id"]
	siteName := svc["site_name"]
	if siteID == "" && siteName == "" {
		return nil, fmt.Errorf("%s", i18n.T("error.deploy_panel_no_site"))
	}
	client := newPanelClient(panelURL, creds.AccessKeyID, "", nil, aaPanelAuth)
	form := url.Values{}
	form.Set("p", "1")
	form.Set("limit", "100")
	body, err := client.doV2Request(ctx, "/v2/mod/proxy/com/get_list", nil, form)
	if err != nil {
		return nil, i18n.Wrap(err, "deploy.error.current_cert_query")
	}
	var resp aaPanelSiteListResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, i18n.Wrap(err, "deploy.error.current_cert_query")
	}
	// 定位目标站点：优先按 site_id 精确匹配，否则按站点名匹配
	var target *aaPanelSiteItem
	for i := range resp.Message.Data.Data {
		s := &resp.Message.Data.Data[i]
		if siteID != "" {
			if strconv.Itoa(s.ID) == siteID {
				target = s
				break
			}
			continue
		}
		name := s.Name
		if name == "" {
			name = s.PS
		}
		if name == siteName {
			target = s
			break
		}
	}
	if target == nil {
		return nil, fmt.Errorf("%s: %s", i18n.T("error.deploy_panel_no_site"), siteName)
	}
	// ssl 字段缺失或 issuer/notAfter 均为空，视为未配置 SSL 证书
	if target.SSL.Issuer == "" && target.SSL.NotAfter == "" {
		return nil, i18n.NewError("deploy.error.current_cert_not_configured")
	}
	issuer := target.SSL.IssuerO
	if issuer == "" {
		issuer = target.SSL.Issuer
	}
	return &CurrentCert{
		CommonName:   target.SSL.Subject,
		SANs:         target.SSL.DNS,
		Issuer:       issuer,
		NotBefore:    target.SSL.NotBefore,
		NotAfter:     target.SSL.NotAfter,
		SerialNumber: "",
	}, nil
}
