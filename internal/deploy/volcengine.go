package deploy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"cnb.cool/dtapp/certflow/internal/httplog"
	"cnb.cool/dtapp/certflow/internal/i18n"
	"cnb.cool/dtapp/certflow/internal/logging"
	"github.com/volcengine/volc-sdk-golang/base"
	"github.com/volcengine/volc-sdk-golang/service/cdn"
)

// VolcengineDeployer 火山引擎部署器：支持火山引擎 CDN 与全站加速（DCDN）的证书部署。
//   - CDN：使用 volc-sdk-golang 内置的 AddCertificate / BatchDeployCert / ListCdnDomains。
//   - DCDN（全站加速）：SDK 未提供证书相关方法，改用 base.Client 直接调用 DCDN OpenAPI
//     （证书中心与 CDN 统一，上传复用 CDN AddCertificate；绑定走 CreateCertBind）。
type VolcengineDeployer struct{}

func init() { RegisterDeployer(&VolcengineDeployer{}) }

func (d *VolcengineDeployer) Provider() string { return "volcengine" }

// newVolcCDNClient 构造火山引擎 CDN 客户端并完成签名鉴权。
// 火山引擎 CDN 证书接口为全球统一服务（无需指定 region），仅需 AccessKey/SecretKey。
func newVolcCDNClient(creds Credentials) *cdn.CDN {
	c := cdn.NewInstance()
	// 包裹带 HTTP 请求日志的 transport（仅 DEBUG 生效），记录火山引擎 CDN 请求。
	c.Client.Client.Transport = httplog.WrapTransport(c.Client.Client.Transport)
	c.Client.SetAccessKey(creds.AccessKeyID)
	c.Client.SetSecretKey(creds.AccessKeySecret)
	return c
}

// newVolcDCDNClient 构造火山引擎全站加速（DCDN）OpenAPI 客户端。
// DCDN 证书绑定走 OpenAPI（CreateCertBind / ListDomainConfig），复用 SDK 的 base.Client
// 完成火山标准签名，无需自行实现 HMAC。Host 固定为 open.volcengineapi.com，
// Service 为 dcdn，Region 默认 cn-north-1（DCDN 为区域服务）。
func newVolcDCDNClient(creds Credentials) *base.Client {
	region := creds.Region
	if region == "" {
		region = base.RegionCnNorth1
	}
	info := &base.ServiceInfo{
		Host:   "open.volcengineapi.com",
		Scheme: "https",
		Credentials: base.Credentials{
			AccessKeyID:     creds.AccessKeyID,
			SecretAccessKey: creds.AccessKeySecret,
			Service:         "dcdn",
			Region:          region,
		},
		Header: http.Header{"Accept": []string{"application/json"}},
	}
	apiInfoList := map[string]*base.ApiInfo{
		"CreateCertBind": {
			Method: "POST",
			Path:   "/",
			Query:  url.Values{"Action": {"CreateCertBind"}, "Version": {"2021-04-01"}},
		},
		"ListDomainConfig": {
			Method: "POST",
			Path:   "/",
			Query:  url.Values{"Action": {"ListDomainConfig"}, "Version": {"2021-04-01"}},
		},
	}
	c := base.NewClient(info, apiInfoList)
	c.Client.Transport = httplog.WrapTransport(c.Client.Transport)
	return c
}

// volcErrText 提取火山引擎响应的业务错误信息（ResponseMetadata.Error 含 Code/Message）。
// SDK 在 HTTP 200 + 错误码时仍返回已解析的响应（含错误信封），需据此判断业务失败。
func volcErrText(meta *cdn.ResponseMetadata) string {
	if meta == nil || meta.Error == nil {
		return ""
	}
	e := meta.Error
	if e.Code != "" {
		return fmt.Sprintf("[%s] %s", e.Code, e.Message)
	}
	return e.Message
}

// volcCertName 仅用于日志/备注展示：火山证书以 CertId 关联（无备注名概念）。
// 命名风格对齐 ctyun/aliyun（域名 + 证书指纹前 8 位），保证续期生成的证书名唯一、不撞车。
func volcCertName(cert CertContent) string {
	base := strings.NewReplacer("*", "wildcard", ".", "-", " ", "-").Replace(strings.TrimSpace(cert.Domain))
	sum := sha256.Sum256([]byte(cert.CertPEM + cert.KeyPEM))
	return fmt.Sprintf("certflow-%s-%s", base, hex.EncodeToString(sum[:])[:8])
}

// UploadCert 上传证书到火山引擎证书中心（AddCertificate），返回云端证书 CertId。
// 火山证书中心为 CDN 与全站加速（DCDN）共用，因此 CDN / DCDN 的上传逻辑一致。
func (d *VolcengineDeployer) UploadCert(ctx context.Context, creds Credentials, cert CertContent, svcConfig map[string]string) (string, string, error) {
	if cert.CertPEM == "" || cert.KeyPEM == "" {
		return "", "", i18n.NewError("deploy.error.volcengine_cert_empty")
	}

	client := newVolcCDNClient(creds)
	req := &cdn.AddCertificateRequest{
		Certificate: cert.CertPEM,
		PrivateKey:  cert.KeyPEM,
	}
	resp, err := client.AddCertificate(req)
	if err != nil {
		return "", "", i18n.Wrap(err, "deploy.error.volcengine_upload_cert")
	}
	if msg := volcErrText(resp.ResponseMetadata); msg != "" {
		return "", "", fmt.Errorf("%s: %s", i18n.T("deploy.error.volcengine_upload_cert"), msg)
	}

	certID := resp.Result.CertId
	if certID == "" {
		return "", "", fmt.Errorf("%s: %s", i18n.T("deploy.error.volcengine_upload_cert"), "empty cert id")
	}
	raw, _ := json.Marshal(resp)
	logging.Debug(i18n.T("log.deploy.volcengine_upload", "CertID", certID, "CertName", volcCertName(cert)))
	return certID, string(raw), nil
}

// DeployCert 将已上传的证书部署/绑定到火山引擎目标服务（CDN 或全站加速 DCDN）。
// certID 为 UploadCert 返回的证书中心 CertId；svc 区分 cdn / dcdn；domain 来自 svcConfig["domain"]。
func (d *VolcengineDeployer) DeployCert(ctx context.Context, creds Credentials, certID string, svc string, svcConfig map[string]string) (*DeployResult, error) {
	if svc == "dcdn" {
		return d.deployVolcDCDN(ctx, creds, certID, svcConfig)
	}
	return d.deployVolcCDN(ctx, creds, certID, svcConfig)
}

// deployVolcCDN 将证书绑定到火山引擎 CDN 加速域名（BatchDeployCert）。
func (d *VolcengineDeployer) deployVolcCDN(ctx context.Context, creds Credentials, certID string, svcConfig map[string]string) (*DeployResult, error) {
	domain := svcConfig["domain"]
	if domain == "" {
		return &DeployResult{CloudCertID: certID, Message: i18n.T("deploy.message.volcengine_no_domain")}, nil
	}

	client := newVolcCDNClient(creds)
	req := &cdn.BatchDeployCertRequest{
		CertId: certID,
		Domain: domain,
	}
	logging.Debug(i18n.T("log.deploy.volcengine_deploy_start", "Domain", domain, "CertID", certID))

	resp, err := client.BatchDeployCert(req)
	if err != nil {
		logging.Debug(i18n.T("log.deploy.volcengine_deploy_failed", "Domain", domain, "Err", err))
		return &DeployResult{CloudCertID: certID}, i18n.Wrap(err, "deploy.error.volcengine_bind_cert")
	}
	if msg := volcErrText(resp.ResponseMetadata); msg != "" {
		logging.Debug(i18n.T("log.deploy.volcengine_deploy_failed", "Domain", domain, "Err", msg))
		return &DeployResult{CloudCertID: certID}, fmt.Errorf("%s: %s", i18n.T("deploy.error.volcengine_bind_cert"), msg)
	}

	// BatchDeployCert 逐域名返回部署状态，需检查目标域名是否真正成功（Status=success）。
	for _, st := range resp.Result.DeployResult {
		if st.Domain == domain && st.Status != "success" {
			logging.Debug(i18n.T("log.deploy.volcengine_deploy_failed", "Domain", domain, "Err", st.ErrorMsg))
			return &DeployResult{CloudCertID: certID}, fmt.Errorf("%s: %s", i18n.T("deploy.error.volcengine_bind_cert"), st.ErrorMsg)
		}
	}

	raw, _ := json.Marshal(resp)
	logging.Debug(i18n.T("log.deploy.volcengine_deploy_success", "Domain", domain))
	return &DeployResult{
		CloudCertID: certID,
		Message:     i18n.T("deploy.message.volcengine_deployed", "Domain", domain),
		RawResponse: string(raw),
	}, nil
}

// deployVolcDCDN 将证书绑定到火山引擎全站加速（DCDN）加速域名（CreateCertBind）。
// 证书中心与 CDN 共用，故 CertId 可直接复用；DomainNames 为数组，这里按单个部署域名绑定。
func (d *VolcengineDeployer) deployVolcDCDN(ctx context.Context, creds Credentials, certID string, svcConfig map[string]string) (*DeployResult, error) {
	domain := svcConfig["domain"]
	if domain == "" {
		return &DeployResult{CloudCertID: certID, Message: i18n.T("deploy.message.volcengine_no_domain")}, nil
	}

	client := newVolcDCDNClient(creds)
	body := map[string]any{
		"CertSource":  "volc",
		"CertId":      certID,
		"DomainNames": []string{domain},
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return &DeployResult{CloudCertID: certID}, i18n.Wrap(err, "deploy.error.volcengine_bind_cert")
	}

	logging.Debug(i18n.T("log.deploy.volcengine_deploy_start", "Domain", domain, "CertID", certID))
	raw, _, err := client.Json("CreateCertBind", url.Values{}, string(bodyBytes))
	if err != nil {
		logging.Debug(i18n.T("log.deploy.volcengine_deploy_failed", "Domain", domain, "Err", err))
		return &DeployResult{CloudCertID: certID}, i18n.Wrap(err, "deploy.error.volcengine_bind_cert")
	}

	var resp volcDCDNBindResp
	if err := json.Unmarshal(raw, &resp); err != nil {
		return &DeployResult{CloudCertID: certID}, fmt.Errorf("%s: %s", i18n.T("deploy.error.volcengine_bind_cert"), "invalid response")
	}
	if msg := volcErrText(resp.ResponseMetadata); msg != "" {
		logging.Debug(i18n.T("log.deploy.volcengine_deploy_failed", "Domain", domain, "Err", msg))
		return &DeployResult{CloudCertID: certID}, fmt.Errorf("%s: %s", i18n.T("deploy.error.volcengine_bind_cert"), msg)
	}

	logging.Debug(i18n.T("log.deploy.volcengine_deploy_success", "Domain", domain))
	return &DeployResult{
		CloudCertID: certID,
		Message:     i18n.T("deploy.message.volcengine_deployed", "Domain", domain),
		RawResponse: string(raw),
	}, nil
}

// ListDomains 列出火山引擎目标服务下的加速域名。
// svc 区分 cdn（ListCdnDomains）/ dcdn（ListDomainConfig）。
func (d *VolcengineDeployer) ListDomains(ctx context.Context, creds Credentials, svc, region, zoneID string) ([]string, error) {
	if svc == "dcdn" {
		return d.listVolcDCDNDomains(ctx, creds)
	}
	return d.listVolcCDNDomains(ctx, creds)
}

// ListSites 云厂商默认回退到 ListDomains。
func (d *VolcengineDeployer) ListSites(ctx context.Context, creds Credentials, svc, region, zoneID string) ([]string, error) {
	return d.ListDomains(ctx, creds, svc, region, zoneID)
}

// listVolcCDNDomains 列出火山引擎 CDN 加速域名（ListCdnDomains，分页拉取全部）。
func (d *VolcengineDeployer) listVolcCDNDomains(ctx context.Context, creds Credentials) ([]string, error) {
	client := newVolcCDNClient(creds)
	var domains []string
	pageNum := int64(1)
	pageSize := int64(100)
	for {
		req := &cdn.ListCdnDomainsRequest{
			PageNum:  &pageNum,
			PageSize: &pageSize,
		}
		resp, err := client.ListCdnDomains(req)
		if err != nil {
			return nil, i18n.Wrap(err, "deploy.error.volcengine_list_domains")
		}
		if msg := volcErrText(resp.ResponseMetadata); msg != "" {
			return nil, fmt.Errorf("%s: %s", i18n.T("deploy.error.volcengine_list_domains"), msg)
		}
		for _, item := range resp.Result.Data {
			if item.Domain != "" {
				domains = append(domains, item.Domain)
			}
		}
		// 已拉完所有分页（当前页末条 >= 总数）则停止。
		if resp.Result.Total <= pageNum*pageSize {
			break
		}
		pageNum++
	}
	logging.Debug(i18n.T("log.deploy.volcengine_list_success", "Count", len(domains)))
	return domains, nil
}

// listVolcDCDNDomains 列出火山引擎全站加速（DCDN）加速域名（ListDomainConfig，分页拉取全部）。
func (d *VolcengineDeployer) listVolcDCDNDomains(ctx context.Context, creds Credentials) ([]string, error) {
	client := newVolcDCDNClient(creds)
	var domains []string
	pageNum := 1
	pageSize := 100
	for {
		body := map[string]any{
			"PageNumber": pageNum,
			"PageSize":   pageSize,
		}
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return nil, i18n.Wrap(err, "deploy.error.volcengine_list_domains")
		}
		raw, _, err := client.Json("ListDomainConfig", url.Values{}, string(bodyBytes))
		if err != nil {
			return nil, i18n.Wrap(err, "deploy.error.volcengine_list_domains")
		}
		var resp volcDCDNListResp
		if err := json.Unmarshal(raw, &resp); err != nil {
			return nil, fmt.Errorf("%s: %s", i18n.T("deploy.error.volcengine_list_domains"), "invalid response")
		}
		if msg := volcErrText(resp.ResponseMetadata); msg != "" {
			return nil, fmt.Errorf("%s: %s", i18n.T("deploy.error.volcengine_list_domains"), msg)
		}
		for _, item := range resp.Result.DomainConfigs {
			if item.Domain != "" {
				domains = append(domains, item.Domain)
			}
		}
		if resp.Result.Total <= pageNum*pageSize {
			break
		}
		pageNum++
	}
	logging.Debug(i18n.T("log.deploy.volcengine_list_success", "Count", len(domains)))
	return domains, nil
}

// volcDCDNBindResp DCDN 批量绑定证书（CreateCertBind）的响应结构。
type volcDCDNBindResp struct {
	ResponseMetadata *cdn.ResponseMetadata `json:"ResponseMetadata"`
	Result           json.RawMessage       `json:"Result"`
}

// volcDCDNListResp DCDN 查询域名配置列表（ListDomainConfig）的响应结构。
type volcDCDNListResp struct {
	ResponseMetadata *cdn.ResponseMetadata `json:"ResponseMetadata"`
	Result           struct {
		DomainConfigs []struct {
			Domain string `json:"Domain"`
		} `json:"DomainConfigs"`
		Total int `json:"Total"`
	} `json:"Result"`
}
