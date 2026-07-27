package deploy

import (
	"context"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"cnb.cool/dtapp/certflow/internal/i18n"
	"cnb.cool/dtapp/certflow/internal/logging"
)

// OnePanelDeployer 1Panel 面板部署器。
// 凭证：仅需 api_key / Token（映射到 AccessKeyID，无 Secret）。
// 部署目标：panel_url 标识面板地址，site 标识待部署站点。
// ListSites / DeployCert 均已覆盖实现。
type OnePanelDeployer struct {
	panelDeployerBase
}

func init() { RegisterDeployer(OnePanelDeployer{panelDeployerBase{provider: ProviderOnePanel}}) }

// onePanelSearchRequest /api/v2/websites/search 请求体（POST JSON）。
type onePanelSearchRequest struct {
	Name           string `json:"name"`
	Page           int    `json:"page"`
	PageSize       int    `json:"pageSize"`
	OrderBy        string `json:"orderBy"`
	Order          string `json:"order"`
	WebsiteGroupID int    `json:"websiteGroupId"`
	Type           string `json:"type"`
}

// onePanelSiteItem 站点列表条目（search 返回）。
type onePanelSiteItem struct {
	ID            int    `json:"id"`
	Alias         string `json:"alias"`
	PrimaryDomain string `json:"primaryDomain"`
}

// onePanelSearchResponse /api/v2/websites/search 响应。
type onePanelSearchResponse struct {
	Code int `json:"code"`
	Data struct {
		Items []onePanelSiteItem `json:"items"`
		Total int                `json:"total"`
	} `json:"data"`
}

// onePanelHTTPSConfig GET /api/v2/websites/{id}/https 返回的配置。
type onePanelHTTPSConfig struct {
	Enable                bool     `json:"enable"`
	HttpConfig            string   `json:"httpConfig"`
	SSLProtocol           []string `json:"SSLProtocol"`
	Algorithm             string   `json:"algorithm"`
	Hsts                  bool     `json:"hsts"`
	HstsIncludeSubDomains bool     `json:"hstsIncludeSubDomains"`
	HttpsPorts            []int    `json:"httpsPorts"`
	HttpsPort             string   `json:"httpsPort"`
	Http3                 bool     `json:"http3"`
}

// onePanelHTTPSConfigResponse GET /api/v2/websites/{id}/https 响应。
type onePanelHTTPSConfigResponse struct {
	Code    int                 `json:"code"`
	Message string              `json:"message"`
	Data    onePanelHTTPSConfig `json:"data"`
}

// onePanelUpdateHTTPSRequest POST /api/v2/websites/{id}/https 请求体（UpdateHTTPSConfig）。
type onePanelUpdateHTTPSRequest struct {
	WebsiteID             int      `json:"websiteId"`
	Type                  string   `json:"type"`
	ImportType            string   `json:"importType"`
	PrivateKey            string   `json:"privateKey"`
	Certificate           string   `json:"certificate"`
	PrivateKeyPath        string   `json:"privateKeyPath"`
	CertificatePath       string   `json:"certificatePath"`
	AcmeAccountID         int      `json:"acmeAccountID"`
	Enable                bool     `json:"enable"`
	SSLProtocol           []string `json:"SSLProtocol"`
	Algorithm             string   `json:"algorithm"`
	Hsts                  bool     `json:"hsts"`
	HstsIncludeSubDomains bool     `json:"hstsIncludeSubDomains"`
	HttpConfig            string   `json:"httpConfig"`
	HttpsPorts            []int    `json:"httpsPorts"`
	HttpsPort             string   `json:"httpsPort"`
	Http3                 bool     `json:"http3"`
}

// onePanelUpdateHTTPSResponse POST /api/v2/websites/{id}/https 响应。
type onePanelUpdateHTTPSResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// defaultSSLAlgorithm 是 1Panel HTTPS 配置里 algorithm 字段（nginx ssl_ciphers 套件字符串）的兜底值。
// 正常应从 GET /websites/{id}/https 回传原值；仅当站点原本未配置 cipher 套件时回退到该现代安全套件。
const defaultSSLAlgorithm = "ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:" +
	"ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384:" +
	"ECDHE-ECDSA-CHACHA20-POLY1305:ECDHE-RSA-CHACHA20-POLY1305:" +
	"DHE-RSA-AES128-GCM-SHA256:DHE-RSA-AES256-GCM-SHA384"

// onePanelAuth 生成 1Panel 鉴权签名：md5("1panel" + api_key + timestamp)。
// 请求头 1panel-Token / 1panel-Timestamp。
func onePanelAuth(c *PanelClient, _, _ string, _ url.Values, _ []byte) (http.Header, url.Values, error) {
	h := http.Header{}
	form := url.Values{}
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	token := md5Hex("1panel" + c.APIKey + ts)
	h.Set("1panel-Token", token)
	h.Set("1panel-Timestamp", ts)
	return h, form, nil
}

// ListSites 列出 1Panel 网站，用于前端「获取网站」下拉。
// 调用 /api/v2/websites/search（POST JSON 请求体）。
// 1Panel v2 返回格式：{ "code": 200, "data": { "items": [ {id,alias,primaryDomain,...} ], "total": N } }。
func (d OnePanelDeployer) ListSites(ctx context.Context, creds Credentials, _, _, _ string) ([]string, error) {
	if creds.PanelURL == "" {
		return nil, fmt.Errorf("%s", i18n.T("error.deploy_panel_no_url"))
	}
	client := newPanelClient(creds.PanelURL, creds.AccessKeyID, "", nil, onePanelAuth)
	// 1Panel v2 接口要求 JSON 请求体（非表单）
	payload := onePanelSearchRequest{
		Name:           "",
		Page:           1,
		PageSize:       100,
		OrderBy:        "favorite",
		Order:          "descending",
		WebsiteGroupID: 0,
		Type:           "",
	}
	body, err := client.doJSONRequest(ctx, "/api/v2/websites/search", payload)
	if err != nil {
		return nil, err
	}
	var resp onePanelSearchResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("%s", i18n.T("error.deploy_panel_list_parse", "Err", err.Error()))
	}
	out := make([]string, 0, len(resp.Data.Items))
	for _, s := range resp.Data.Items {
		name := s.Alias
		if name == "" {
			name = s.PrimaryDomain
		}
		out = append(out, name+"||"+strconv.Itoa(s.ID))
	}
	return out, nil
}

// DeployCert 将证书部署到 1Panel 网站。
// 接口说明（用户提供的官方 API）：设置站点证书走 POST /api/v2/websites/{id}/https
// （UpdateHTTPSConfig），而非 /websites/update（该接口无证书字段）。
// 该接口所有字段均为值类型，若只传 privateKey+certificate 会清空既有 HTTPS 配置
// （HSTS、SSL 协议、端口等），因此先 GET /api/v2/websites/{id}/https 拉取当前配置，
// 再原样回传、仅替换证书相关字段（type=manual、privateKey、certificate、enable=true）。
// 站点 ID 来自 svc["site_id"]（与站点名分开放置，不再用「名称||ID」拼接/解析）。
func (d OnePanelDeployer) DeployCert(ctx context.Context, creds Credentials, _ string, _ string, svc map[string]string) (*DeployResult, error) {
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
	// 1Panel 部分版本仅接受传统格式私钥（RSA PKCS#1 / EC SEC1），
	// 对 PKCS#8（BEGIN PRIVATE KEY）会报「私钥文件校验失败」。先归一化为传统格式。
	keyPEM = normalizePrivateKeyPEM(keyPEM)
	// 本地预校验：私钥必须与证书（叶子）匹配，提前暴露证书数据问题
	// （手动导入/续期存错会导致 key 与 cert 不匹配，1Panel 会返回晦涩的「私钥文件校验失败」）。
	if err := verifyKeyCertMatch(keyPEM, certPEM); err != nil {
		return nil, fmt.Errorf("%s: %s (%s)", i18n.T("error.deploy_panel_cert_verify"), err.Error(), keyCertMismatchDetail(keyPEM, certPEM))
	}

	client := newPanelClient(creds.PanelURL, creds.AccessKeyID, "", nil, onePanelAuth)

	// 1) 拉取当前 HTTPS 配置（GET /api/v2/websites/{id}/https）
	getBody, err := client.doGetRequest(ctx, fmt.Sprintf("/api/v2/websites/%d/https", siteID), url.Values{})
	if err != nil {
		return nil, fmt.Errorf("%s: %s", i18n.T("error.deploy_panel_cert_deploy"), err.Error())
	}
	var getResp onePanelHTTPSConfigResponse
	if err := json.Unmarshal(getBody, &getResp); err != nil {
		return nil, fmt.Errorf("%s: %s", i18n.T("error.deploy_panel_cert_deploy"), err.Error())
	}
	if getResp.Code != 200 {
		return nil, fmt.Errorf("%s: %s", i18n.T("error.deploy_panel_cert_deploy"), getResp.Message)
	}

	// 2) 回传：保留既有 HTTPS 配置，仅替换证书相关字段。
	// 关键：必须带 importType:"paste"，告诉 1Panel 使用内联的 privateKey/certificate；
	// 漏掉该字段时 1Panel 会去读 privateKeyPath/certificatePath（为空），导致「私钥文件校验失败」。
	// algorithm 是 nginx 的 cipher 套件字符串（对应 ssl_ciphers），从 GET 回传原值即可，
	// 并非密钥类型（RSA/ECC）；SSLProtocol 是 TLS 协议版本数组。二者都直接复用 GET 配置。
	algorithm := getResp.Data.Algorithm
	if algorithm == "" {
		algorithm = defaultSSLAlgorithm
	}
	httpConfig := getResp.Data.HttpConfig
	if httpConfig == "" {
		httpConfig = "HTTPAlso"
	}
	httpsPort := getResp.Data.HttpsPort
	if httpsPort == "" {
		httpsPort = "443"
	}
	logging.Debug("1panel UpdateHTTPS: siteID=%d keyType=%s algorithm=%s",
		siteID, keyTypeOf(keyPEM), algorithm)
	payload := onePanelUpdateHTTPSRequest{
		WebsiteID:             siteID,
		Type:                  "manual",
		ImportType:            "paste",
		PrivateKey:            keyPEM,
		Certificate:           certPEM,
		PrivateKeyPath:        "",
		CertificatePath:       "",
		AcmeAccountID:         0,
		Enable:                true,
		SSLProtocol:           getResp.Data.SSLProtocol,
		Algorithm:             algorithm,
		Hsts:                  getResp.Data.Hsts,
		HstsIncludeSubDomains: getResp.Data.HstsIncludeSubDomains,
		HttpConfig:            httpConfig,
		HttpsPorts:            getResp.Data.HttpsPorts,
		HttpsPort:             httpsPort,
		Http3:                 getResp.Data.Http3,
	}
	postBody, err := client.doJSONRequest(ctx, fmt.Sprintf("/api/v2/websites/%d/https", siteID), payload)
	if err != nil {
		return nil, fmt.Errorf("%s: %s", i18n.T("error.deploy_panel_cert_deploy"), err.Error())
	}
	var postResp onePanelUpdateHTTPSResponse
	if err := json.Unmarshal(postBody, &postResp); err != nil {
		return nil, fmt.Errorf("%s: %s", i18n.T("error.deploy_panel_cert_deploy"), err.Error())
	}
	logging.Debug("1panel UpdateHTTPS resp: code=%d message=%s", postResp.Code, postResp.Message)
	if postResp.Code != 200 {
		return nil, fmt.Errorf("%s: %s", i18n.T("error.deploy_panel_cert_deploy"), postResp.Message)
	}

	return &DeployResult{
		CloudCertID: strconv.Itoa(siteID),
		Message:     i18n.T("deploy.panel_ssl_set"),
	}, nil
}

// verifyKeyCertMatch 复现面板/服务端的私钥-证书匹配校验：用 tls.X509KeyPair
// 解析（certificate 支持完整证书链，取首个叶子证书与私钥比对）。不匹配返回具体错误，
// 便于把「私钥文件校验失败」这类面板晦涩报错转译为明确的证书数据问题提示。
func verifyKeyCertMatch(keyPEM, certPEM string) error {
	if _, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM)); err != nil {
		return err
	}
	return nil
}

// normalizePrivateKeyPEM 将 PKCS#8 私钥（BEGIN PRIVATE KEY）归一化为 1Panel 兼容性更好的
// 传统格式：RSA -> PKCS#1（BEGIN RSA PRIVATE KEY），EC -> SEC1（BEGIN EC PRIVATE KEY）。
// 已是传统格式或无法解析时原样返回，避免破坏可用数据。
func normalizePrivateKeyPEM(keyPEM string) string {
	block, _ := pem.Decode([]byte(keyPEM))
	if block == nil {
		return keyPEM
	}
	if block.Type == "RSA PRIVATE KEY" || block.Type == "EC PRIVATE KEY" {
		return keyPEM
	}
	if k, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		switch key := k.(type) {
		case *rsa.PrivateKey:
			out := x509.MarshalPKCS1PrivateKey(key)
			return string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: out}))
		case *ecdsa.PrivateKey:
			out, e := x509.MarshalECPrivateKey(key)
			if e == nil {
				return string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: out}))
			}
		}
	}
	return keyPEM
}

// keyCertMismatchDetail 解析私钥类型与证书覆盖的域名，用于在私钥-证书不匹配时给出可定位的排查信息。
func keyCertMismatchDetail(keyPEM, certPEM string) string {
	keyType := "未知"
	if b, _ := pem.Decode([]byte(keyPEM)); b != nil {
		switch b.Type {
		case "RSA PRIVATE KEY":
			keyType = "RSA"
		case "EC PRIVATE KEY":
			keyType = "EC"
		case "PRIVATE KEY":
			if k, e := x509.ParsePKCS8PrivateKey(b.Bytes); e == nil {
				switch k.(type) {
				case *rsa.PrivateKey:
					keyType = "RSA(PKCS#8)"
				case *ecdsa.PrivateKey:
					keyType = "EC(PKCS#8)"
				}
			}
		}
	}
	domains := []string{}
	if b, _ := pem.Decode([]byte(certPEM)); b != nil {
		if c, e := x509.ParseCertificate(b.Bytes); e == nil {
			domains = append(domains, c.DNSNames...)
			if c.Subject.CommonName != "" {
				domains = append(domains, c.Subject.CommonName)
			}
		}
	}
	return fmt.Sprintf("私钥类型=%s, 证书域名=%v", keyType, domains)
}

// keyTypeOf 返回私钥的简短类型描述（RSA / EC / RSA(PKCS#8) / EC(PKCS#8) / 未知），仅用于日志。
func keyTypeOf(keyPEM string) string {
	if b, _ := pem.Decode([]byte(keyPEM)); b != nil {
		switch b.Type {
		case "RSA PRIVATE KEY":
			return "RSA"
		case "EC PRIVATE KEY":
			return "EC"
		case "PRIVATE KEY":
			if k, e := x509.ParsePKCS8PrivateKey(b.Bytes); e == nil {
				switch k.(type) {
				case *rsa.PrivateKey:
					return "RSA(PKCS#8)"
				case *ecdsa.PrivateKey:
					return "EC(PKCS#8)"
				}
			}
			return "PKCS#8"
		}
	}
	return "未知"
}
