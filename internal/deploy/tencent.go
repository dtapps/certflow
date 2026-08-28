package deploy

import (
	"context"
	"net/http"
	"strings"

	"cnb.cool/dtapp/certflow/internal/httplog"
	"cnb.cool/dtapp/certflow/internal/i18n"
	"cnb.cool/dtapp/certflow/internal/logging"
	cdn "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/cdn/v20180606"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	ecdn "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ecdn/v20191012"
	ssl "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ssl/v20191205"
	teo "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/teo/v20220901"
)

// TencentDeployer 腾讯云部署器：上传证书到 SSL，再部署到 CDN / EdgeOne。
// 统一使用腾讯云官方 SDK（tencentcloud-sdk-go），业务错误由 SDK 直接以 error 返回，
// 无需再手动解析 Response.Error 信封。
type TencentDeployer struct{}

func init() { RegisterDeployer(&TencentDeployer{}) }

func (d *TencentDeployer) Provider() string { return "tencentcloud" }

// tencentCredential 构造腾讯云 SDK 凭证
func tencentCredential(creds Credentials) *common.Credential {
	return common.NewCredential(creds.AccessKeyID, creds.AccessKeySecret)
}

// UploadCert 上传证书到腾讯云 SSL 证书服务，返回证书 ID。
// Repeatable=false：相同指纹的证书不会重复创建，云端直接通过 RepeatCertId 返回已有证书 ID，
// 配合 DeployService 的进程内缓存，保证同一张证书在多个域名/目标部署时只上传一次。
func (d *TencentDeployer) UploadCert(ctx context.Context, creds Credentials, cert CertContent, svcConfig map[string]string) (string, string, error) {
	alias := certName(cert.Domain, svcConfig)
	client, err := ssl.NewClient(tencentCredential(creds), creds.Region, profile.NewClientProfile())
	if err != nil {
		return "", "", i18n.Wrap(err, "deploy.error.tencent_ssl_client_create")
	}
	// 包裹带 HTTP 请求日志的 transport（仅 DEBUG 生效）。
	client.WithHttpTransport(httplog.WrapTransport(&http.Transport{}))
	req := ssl.NewUploadCertificateRequest()
	req.CertificatePublicKey = new(cert.CertPEM)
	req.CertificatePrivateKey = new(cert.KeyPEM)
	req.Alias = new(alias)
	req.Repeatable = new(false)

	logging.Debug(i18n.T("log.deploy.tencent_upload_start", "Domain", cert.Domain, "Alias", alias))
	resp, err := client.UploadCertificateWithContext(ctx, req)
	if err != nil {
		logging.Debug(i18n.T("log.deploy.tencent_upload_failed", "Alias", alias, "Err", err, "Resp", respDump(resp)))
		return "", "", i18n.Wrap(err, "deploy.error.tencent_ssl_upload")
	}
	raw := resp.ToJsonString()
	// 证书已存在时 CertificateId 可能为空，需回退到 RepeatCertId（去重返回的已有证书 ID）
	certID := ""
	if resp.Response != nil {
		if resp.Response.CertificateId != nil {
			certID = *resp.Response.CertificateId
		}
		if certID == "" && resp.Response.RepeatCertId != nil {
			certID = *resp.Response.RepeatCertId
		}
	}
	logging.Debug(i18n.T("log.deploy.tencent_upload_success", "CertID", certID))
	return certID, raw, nil
}

// DeployCert 将已上传的证书绑定部署到腾讯云目标服务（CDN / EdgeOne）
func (d *TencentDeployer) DeployCert(ctx context.Context, creds Credentials, certID string, svc string, svcConfig map[string]string) (*DeployResult, error) {
	logging.Debug(i18n.T("log.deploy.tencent_deploy_start", "Svc", svc, "CertID", certID))
	switch svc {
	case "cdn", "ecdn":
		// CDN 与 ECDN 都是加速域名，HTTPS 证书均通过 CDN 的 UpdateDomainConfig 在
		// Https.CertInfo 里引用已上传证书拿到的 CertId 完成绑定（ECDN 加速域名复用 CDN 配置面）。
		domain := svcConfig["domain"]
		if domain == "" {
			return &DeployResult{CloudCertID: certID, Message: i18n.T("deploy.message.tencent_ssl_no_domain")}, nil
		}
		return d.deployTencentCDNDomain(ctx, creds, domain, certID)
	case "edgeone":
		// EdgeOne（边缘安全加速 TEO）：证书已上传到腾讯云 SSL，这里用 ModifyHostsCertificate
		// 把 SSL 证书 ID 绑定到站点下的加速域名。ZoneId 来自部署目标配置，Host 为本次部署的域名。
		zoneID := svcConfig["zone_id"]
		host := svcConfig["domain"]
		if zoneID == "" {
			return &DeployResult{CloudCertID: certID, Message: i18n.T("deploy.message.tencent_ssl_no_zone")}, nil
		}
		if host == "" {
			return &DeployResult{CloudCertID: certID, Message: i18n.T("deploy.message.tencent_ssl_no_edgeone_domain")}, nil
		}
		client, err := teo.NewClient(tencentCredential(creds), creds.Region, profile.NewClientProfile())
		if err != nil {
			return &DeployResult{CloudCertID: certID}, i18n.Wrap(err, "deploy.error.tencent_edgeone_client_create")
		}
		// 包裹带 HTTP 请求日志的 transport（仅 DEBUG 生效）。
		client.WithHttpTransport(httplog.WrapTransport(&http.Transport{}))
		req := teo.NewModifyHostsCertificateRequest()
		req.ZoneId = new(zoneID)
		req.Hosts = common.StringPtrs([]string{host})
		req.Mode = new("sslcert")
		req.ServerCertInfo = []*teo.ServerCertInfo{
			{CertId: new(certID)},
		}
		resp, err := client.ModifyHostsCertificateWithContext(ctx, req)
		if err != nil {
			logging.Debug(i18n.T("log.deploy.tencent_edgeone_deploy_failed", "ZoneID", zoneID, "Host", host, "Err", err, "Resp", respDump(resp)))
			return &DeployResult{CloudCertID: certID}, i18n.Wrap(err, "deploy.error.tencent_edgeone_deploy")
		}
		logging.Debug(i18n.T("log.deploy.tencent_edgeone_deploy_success", "ZoneID", zoneID, "Host", host))
		return &DeployResult{CloudCertID: certID, RawResponse: resp.ToJsonString(), Message: i18n.T("deploy.message.tencent_edgeone_deployed", "Host", host)}, nil
	default:
		return &DeployResult{CloudCertID: certID, Message: i18n.T("deploy.message.tencent_ssl_not_implemented", "Service", svc)}, nil
	}
}

// deployTencentCDNDomain 将已上传的证书绑定到腾讯云 CDN/ECDN 加速域名。
// 腾讯云 CDN 没有独立的 DeployCertificate 动作，标准做法是用 UpdateDomainConfig
// 在 Https.CertInfo 里引用已上传证书拿到的 CertId（ECDN 加速域名复用同一配置面）。
func (d *TencentDeployer) deployTencentCDNDomain(ctx context.Context, creds Credentials, domain, certID string) (*DeployResult, error) {
	client, err := cdn.NewClient(tencentCredential(creds), creds.Region, profile.NewClientProfile())
	if err != nil {
		return &DeployResult{CloudCertID: certID}, i18n.Wrap(err, "deploy.error.tencent_cdn_client_create")
	}
	// 包裹带 HTTP 请求日志的 transport（仅 DEBUG 生效）。
	client.WithHttpTransport(httplog.WrapTransport(&http.Transport{}))
	req := cdn.NewUpdateDomainConfigRequest()
	req.Domain = new(domain)
	req.Https = &cdn.Https{
		Switch: new("on"),
		CertInfo: &cdn.ServerCert{
			CertId: new(certID),
		},
	}
	resp, err := client.UpdateDomainConfigWithContext(ctx, req)
	if err != nil {
		logging.Debug(i18n.T("log.deploy.tencent_cdn_deploy_failed", "Domain", domain, "Err", err, "Resp", respDump(resp)))
		return &DeployResult{CloudCertID: certID}, i18n.Wrap(err, "deploy.error.tencent_cdn_deploy")
	}
	logging.Debug(i18n.T("log.deploy.tencent_cdn_deploy_success", "Domain", domain))
	return &DeployResult{CloudCertID: certID, RawResponse: resp.ToJsonString(), Message: i18n.T("deploy.message.tencent_cdn_deployed", "Domain", domain)}, nil
}

// ListDomains 列出该账号在指定服务下的可绑定目标。
// - cdn：腾讯云 CDN 加速域名（DescribeDomains）
// - edgeone：
//   - 传入 zoneID 时返回该站点下的加速域名（DescribeAccelerationDomains），供选择 hosts；
//   - 不传 zoneID 时返回 EdgeOne 站点列表（DescribeZones），返回 "站点名||ZoneId" 便于前端展示与回填
func (d *TencentDeployer) ListDomains(ctx context.Context, creds Credentials, svc, region, zoneID string) ([]string, error) {
	switch svc {
	case "cdn":
		return d.listTencentCDNDomains(ctx, creds, region)
	case "ecdn":
		return d.listECDNDomains(ctx, creds, region)
	case "edgeone":
		if zoneID != "" {
			return d.listEdgeOneAccelDomains(ctx, creds, region, zoneID)
		}
		return d.listEdgeOneZones(ctx, creds, region)
	default:
		return []string{}, nil
	}
}

// ListSites 云厂商默认回退到 ListDomains。
func (d *TencentDeployer) ListSites(ctx context.Context, creds Credentials, svc, region, zoneID string) ([]string, error) {
	return d.ListDomains(ctx, creds, svc, region, zoneID)
}

// listEdgeOneAccelDomains 列出指定 EdgeOne 站点下的加速域名（hosts），用于绑定证书。
// 返回纯域名列表（DomainName），分页拉取直到取完。
func (d *TencentDeployer) listEdgeOneAccelDomains(ctx context.Context, creds Credentials, region, zoneID string) ([]string, error) {
	client, err := teo.NewClient(tencentCredential(creds), region, profile.NewClientProfile())
	if err != nil {
		return nil, i18n.Wrap(err, "deploy.error.tencent_edgeone_client_create")
	}
	// 包裹带 HTTP 请求日志的 transport（仅 DEBUG 生效）。
	client.WithHttpTransport(httplog.WrapTransport(&http.Transport{}))
	logging.Debug(i18n.T("log.deploy.tencent_list_edgeone_domain_start", "Region", region, "ZoneID", zoneID))
	var domains []string
	offset := int64(0)
	limit := int64(200)
	for {
		req := teo.NewDescribeAccelerationDomainsRequest()
		req.ZoneId = new(zoneID)
		req.Offset = new(offset)
		req.Limit = new(limit)
		resp, err := client.DescribeAccelerationDomainsWithContext(ctx, req)
		if err != nil {
			logging.Debug(i18n.T("log.deploy.tencent_list_edgeone_domain_failed", "Region", region, "ZoneID", zoneID, "Err", err, "Resp", respDump(resp)))
			return nil, i18n.Wrap(err, "deploy.error.tencent_edgeone_list_domains")
		}
		if resp.Response == nil {
			break
		}
		for _, ad := range resp.Response.AccelerationDomains {
			if ad != nil && ad.DomainName != nil && *ad.DomainName != "" {
				domains = append(domains, *ad.DomainName)
			}
		}
		total := int64(0)
		if resp.Response.TotalCount != nil {
			total = *resp.Response.TotalCount
		}
		if total == 0 || len(resp.Response.AccelerationDomains) == 0 || int64(len(domains)) >= total || offset >= 100000 {
			break
		}
		offset += limit
	}
	logging.Debug(i18n.T("log.deploy.tencent_list_edgeone_domain_success", "Region", region, "ZoneID", zoneID, "Count", len(domains)))
	return domains, nil
}

// listECDNDomains 列出腾讯云 ECDN（全站加速）加速域名，用于绑定证书。
// 返回纯域名列表（DomainBriefInfo.Domain），分页拉取直到取完。
func (d *TencentDeployer) listECDNDomains(ctx context.Context, creds Credentials, region string) ([]string, error) {
	client, err := ecdn.NewClient(tencentCredential(creds), region, profile.NewClientProfile())
	if err != nil {
		return nil, i18n.Wrap(err, "deploy.error.tencent_ecdn_client_create")
	}
	// 包裹带 HTTP 请求日志的 transport（仅 DEBUG 生效）。
	client.WithHttpTransport(httplog.WrapTransport(&http.Transport{}))
	logging.Debug(i18n.T("log.deploy.tencent_list_ecdn_start", "Region", region))
	var domains []string
	offset := int64(0)
	for {
		req := ecdn.NewDescribeDomainsRequest()
		req.Offset = new(offset)
		req.Limit = common.Int64Ptr(100)
		resp, err := client.DescribeDomainsWithContext(ctx, req)
		if err != nil {
			logging.Debug(i18n.T("log.deploy.tencent_list_ecdn_failed", "Region", region, "Err", err, "Resp", respDump(resp)))
			return nil, i18n.Wrap(err, "deploy.error.tencent_ecdn_list_domains")
		}
		if resp.Response == nil {
			break
		}
		for _, dm := range resp.Response.Domains {
			if dm != nil && dm.Domain != nil && *dm.Domain != "" {
				domains = append(domains, *dm.Domain)
			}
		}
		total := int64(0)
		if resp.Response.TotalCount != nil {
			total = *resp.Response.TotalCount
		}
		if total == 0 || len(resp.Response.Domains) == 0 || int64(len(domains)) >= total || offset >= 100000 {
			break
		}
		offset += 100
	}
	logging.Debug(i18n.T("log.deploy.tencent_list_ecdn_success", "Region", region, "Count", len(domains)))
	return domains, nil
}

// listTencentCDNDomains 列出腾讯云 CDN 加速域名
func (d *TencentDeployer) listTencentCDNDomains(ctx context.Context, creds Credentials, region string) ([]string, error) {
	client, err := cdn.NewClient(tencentCredential(creds), region, profile.NewClientProfile())
	if err != nil {
		return nil, i18n.Wrap(err, "deploy.error.tencent_cdn_client_create")
	}
	// 包裹带 HTTP 请求日志的 transport（仅 DEBUG 生效）。
	client.WithHttpTransport(httplog.WrapTransport(&http.Transport{}))
	logging.Debug(i18n.T("log.deploy.tencent_list_cdn_start", "Region", region))
	var domains []string
	offset := int64(0)
	for {
		req := cdn.NewDescribeDomainsRequest()
		req.Offset = new(offset)
		req.Limit = common.Int64Ptr(500)
		resp, err := client.DescribeDomainsWithContext(ctx, req)
		if err != nil {
			logging.Debug(i18n.T("log.deploy.tencent_list_cdn_failed", "Region", region, "Err", err, "Resp", respDump(resp)))
			return nil, i18n.Wrap(err, "deploy.error.tencent_cdn_list_domains")
		}
		if resp.Response == nil {
			break
		}
		for _, dm := range resp.Response.Domains {
			if dm != nil && dm.Domain != nil && *dm.Domain != "" {
				domains = append(domains, *dm.Domain)
			}
		}
		total := int64(0)
		if resp.Response.TotalNumber != nil {
			total = *resp.Response.TotalNumber
		}
		if total == 0 || len(resp.Response.Domains) == 0 || int64(len(domains)) >= total || offset >= 100000 {
			break
		}
		offset += 500
	}
	logging.Debug(i18n.T("log.deploy.tencent_list_cdn_success", "Region", region, "Count", len(domains)))
	return domains, nil
}

// listEdgeOneZones 列出腾讯云 EdgeOne 站点，返回 "站点名||ZoneId" 形式，
// 前端按 "||" 拆分得到展示名与 ZoneId（ZoneId 即 ModifyHostsCertificate 绑定所需的 Id）。
func (d *TencentDeployer) listEdgeOneZones(ctx context.Context, creds Credentials, region string) ([]string, error) {
	client, err := teo.NewClient(tencentCredential(creds), region, profile.NewClientProfile())
	if err != nil {
		return nil, i18n.Wrap(err, "deploy.error.tencent_edgeone_client_create")
	}
	// 包裹带 HTTP 请求日志的 transport（仅 DEBUG 生效）。
	client.WithHttpTransport(httplog.WrapTransport(&http.Transport{}))
	logging.Debug(i18n.T("log.deploy.tencent_list_edgeone_zone_start", "Region", region))
	var zones []string
	offset := int64(0)
	limit := int64(100)
	for {
		req := teo.NewDescribeZonesRequest()
		req.Offset = new(offset)
		req.Limit = new(limit)
		resp, err := client.DescribeZonesWithContext(ctx, req)
		if err != nil {
			logging.Debug(i18n.T("log.deploy.tencent_list_edgeone_zone_failed", "Region", region, "Err", err, "Resp", respDump(resp)))
			return nil, i18n.Wrap(err, "deploy.error.tencent_edgeone_list_zones")
		}
		if resp.Response == nil {
			break
		}
		for _, z := range resp.Response.Zones {
			if z != nil && z.ZoneId != nil && *z.ZoneId != "" {
				name := ""
				if z.ZoneName != nil {
					name = *z.ZoneName
				}
				zones = append(zones, name+"||"+*z.ZoneId)
			}
		}
		total := int64(0)
		if resp.Response.TotalCount != nil {
			total = *resp.Response.TotalCount
		}
		if total == 0 || len(resp.Response.Zones) == 0 || offset >= total {
			break
		}
		offset += limit
	}
	logging.Debug(i18n.T("log.deploy.tencent_list_edgeone_zone_success", "Region", region, "Count", len(zones)))
	return zones, nil
}

// tcStr 安全解引用 *string，nil 返回空串（腾讯云 common 包低版本未导出 StringValue）。
func tcStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// tencentCertPEMByID 经腾讯云 SSL 证书服务按证书 ID 反查证书公钥 PEM。
// DescribeDomainsConfig / DescribeHostsSetting 等配置查询接口出于安全不回传证书内容，
// 只返回 CertId 元数据，需经此函数二次查询取得 PEM。
func tencentCertPEMByID(ctx context.Context, creds Credentials, certID string) (string, error) {
	sslClient, err := ssl.NewClient(tencentCredential(creds), creds.Region, profile.NewClientProfile())
	if err != nil {
		return "", i18n.Wrap(err, "deploy.error.current_cert_query")
	}
	detailReq := ssl.NewDescribeCertificateDetailRequest()
	detailReq.CertificateId = new(certID)
	// 单资源第二次云 API 调用，需再取一个限速令牌，避免实际 QPS 翻倍触发频率限制。
	currentCertRateWait()
	detailResp, err := sslClient.DescribeCertificateDetailWithContext(ctx, detailReq)
	if err != nil {
		return "", i18n.Wrap(err, "deploy.error.current_cert_query")
	}
	if detailResp.Response == nil {
		return "", i18n.NewError("deploy.error.current_cert_not_configured")
	}
	pem := strings.TrimSpace(tcStr(detailResp.Response.CertificatePublicKey))
	if pem == "" {
		return "", i18n.NewError("deploy.error.current_cert_not_configured")
	}
	return pem, nil
}

// GetCurrentCert 查询腾讯云资源当前生效的 SSL 证书。
//   - CDN / ECDN：DescribeDomainsConfig 取域名 HTTPS 配置得 CertId（接口不回传证书内容），再 ssl.DescribeCertificateDetail 取公钥 PEM 解析；
//   - EdgeOne：DescribeHostsSetting 按 ZoneId+域名取 Https.CertInfo（仅含 CertId 元数据），再 ssl.DescribeCertificateDetail 取公钥 PEM 解析。
func (d *TencentDeployer) GetCurrentCert(ctx context.Context, creds Credentials, svc string, svcConfig map[string]string) (*CurrentCert, error) {
	logging.Debug(i18n.T("log.deploy.tencent_get_current_cert",
		"Svc", svc,
		"Domain", svcConfig["domain"]))
	domain := svcConfig["domain"]
	if strings.TrimSpace(domain) == "" {
		return nil, i18n.NewError("deploy.error.current_cert_domain_empty")
	}

	switch svc {
	case "cdn":
		client, err := cdn.NewClient(tencentCredential(creds), creds.Region, profile.NewClientProfile())
		if err != nil {
			return nil, i18n.Wrap(err, "deploy.error.current_cert_query")
		}
		req := cdn.NewDescribeDomainsConfigRequest()
		req.Filters = []*cdn.DomainFilter{
			{
				Name:  new("domain"),
				Value: common.StringPtrs([]string{domain}),
				Fuzzy: new(false),
			},
		}
		resp, err := client.DescribeDomainsConfigWithContext(ctx, req)
		if err != nil {
			return nil, i18n.Wrap(err, "deploy.error.current_cert_query")
		}
		if resp.Response == nil || len(resp.Response.Domains) == 0 {
			return nil, i18n.NewError("deploy.error.current_cert_not_configured")
		}
		var dd *cdn.DetailDomain
		for _, d := range resp.Response.Domains {
			if d != nil && d.Domain != nil && *d.Domain == domain {
				dd = d
				break
			}
		}
		if dd == nil || dd.Https == nil || dd.Https.CertInfo == nil {
			return nil, i18n.NewError("deploy.error.current_cert_not_configured")
		}
		// DescribeDomainsConfig 不回传证书内容（Certificate 为空），仅返回 CertId，
		// 需经 SSL 证书服务反查 PEM。
		pem := strings.TrimSpace(tcStr(dd.Https.CertInfo.Certificate))
		if pem == "" {
			certID := tcStr(dd.Https.CertInfo.CertId)
			if certID == "" {
				return nil, i18n.NewError("deploy.error.current_cert_not_configured")
			}
			var perr error
			pem, perr = tencentCertPEMByID(ctx, creds, certID)
			if perr != nil {
				return nil, perr
			}
		}
		return parseCertPEM(pem)

	case "ecdn":
		client, err := ecdn.NewClient(tencentCredential(creds), creds.Region, profile.NewClientProfile())
		if err != nil {
			return nil, i18n.Wrap(err, "deploy.error.current_cert_query")
		}
		req := ecdn.NewDescribeDomainsConfigRequest()
		req.Filters = []*ecdn.DomainFilter{
			{
				Name:  new("domain"),
				Value: common.StringPtrs([]string{domain}),
				Fuzzy: new(false),
			},
		}
		resp, err := client.DescribeDomainsConfigWithContext(ctx, req)
		if err != nil {
			return nil, i18n.Wrap(err, "deploy.error.current_cert_query")
		}
		if resp.Response == nil || len(resp.Response.Domains) == 0 {
			return nil, i18n.NewError("deploy.error.current_cert_not_configured")
		}
		var di *ecdn.DomainDetailInfo
		for _, d := range resp.Response.Domains {
			if d != nil && d.Domain != nil && *d.Domain == domain {
				di = d
				break
			}
		}
		if di == nil || di.Https == nil || di.Https.CertInfo == nil {
			return nil, i18n.NewError("deploy.error.current_cert_not_configured")
		}
		// 同 CDN：配置查询接口不回传证书内容，PEM 为空时按 CertId 反查。
		pem := strings.TrimSpace(tcStr(di.Https.CertInfo.Certificate))
		if pem == "" {
			certID := tcStr(di.Https.CertInfo.CertId)
			if certID == "" {
				return nil, i18n.NewError("deploy.error.current_cert_not_configured")
			}
			var perr error
			pem, perr = tencentCertPEMByID(ctx, creds, certID)
			if perr != nil {
				return nil, perr
			}
		}
		return parseCertPEM(pem)

	case "edgeone":
		// EdgeOne 以站点（ZoneId）+ 加速域名（host）维度管理证书，需配置中提供 zone_id。
		zoneID := svcConfig["zone_id"]
		if strings.TrimSpace(zoneID) == "" {
			return nil, i18n.NewError("deploy.error.tencent_ssl_no_zone")
		}
		client, err := teo.NewClient(tencentCredential(creds), creds.Region, profile.NewClientProfile())
		if err != nil {
			return nil, i18n.Wrap(err, "deploy.error.current_cert_query")
		}
		req := teo.NewDescribeHostsSettingRequest()
		req.ZoneId = new(zoneID)
		req.Filters = []*teo.Filter{
			{
				Name:   new("host"),
				Values: common.StringPtrs([]string{domain}),
			},
		}
		resp, err := client.DescribeHostsSettingWithContext(ctx, req)
		if err != nil {
			return nil, i18n.Wrap(err, "deploy.error.current_cert_query")
		}
		if resp.Response == nil || len(resp.Response.DetailHosts) == 0 {
			return nil, i18n.NewError("deploy.error.current_cert_not_configured")
		}
		var host *teo.DetailHost
		for _, h := range resp.Response.DetailHosts {
			if h != nil && h.Host != nil && *h.Host == domain {
				host = h
				break
			}
		}
		if host == nil || host.Https == nil || len(host.Https.CertInfo) == 0 {
			return nil, i18n.NewError("deploy.error.current_cert_not_configured")
		}
		certID := tcStr(host.Https.CertInfo[0].CertId)
		if certID == "" {
			return nil, i18n.NewError("deploy.error.current_cert_not_configured")
		}
		pem, perr := tencentCertPEMByID(ctx, creds, certID)
		if perr != nil {
			return nil, perr
		}
		return parseCertPEM(pem)

	default:
		return nil, i18n.NewError("deploy.error.current_cert_cloud_not_supported")
	}
}
