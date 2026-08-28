package deploy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"cnb.cool/dtapp/certflow/internal/httplog"
	"cnb.cool/dtapp/certflow/internal/i18n"
	"cnb.cool/dtapp/certflow/internal/logging"
	cas "github.com/alibabacloud-go/cas-20200407/v3/client"
	cdn "github.com/alibabacloud-go/cdn-20180510/v5/client"
	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	dcdn "github.com/alibabacloud-go/dcdn-20180115/v4/client"
	esa "github.com/alibabacloud-go/esa-20240910/v3/client"
	ga "github.com/alibabacloud-go/ga-20191120/v4/client"
	"github.com/alibabacloud-go/tea/tea"
)

// AliyunDeployer 阿里云部署器：上传证书到 CAS，再部署到 CDN / ESA。
// 统一使用阿里云官方 SDK（alibabacloud-go 系列），业务错误由 SDK 直接以 error 返回。
type AliyunDeployer struct {
	// esaCertsCache 在一次 GetCurrentCerts 批量内缓存各站点的 ListCertificates 结果，
	// 避免同一 site_id 对应多个域名（或并发查询）时重复拉取；每次批量前由 BeforeCurrentCerts 清空。
	// key 为 "credKey@siteID"。
	esaCertsCacheMu sync.Mutex
	esaCertsCache   map[string][]*esa.ListCertificatesResponseBodyResult
}

func init() { RegisterDeployer(&AliyunDeployer{}) }

func (d *AliyunDeployer) Provider() string { return "aliyun" }

// aliyunRegion 返回有效区域，缺省用 cn-hangzhou
func aliyunRegion(region string) string {
	if region == "" {
		return "cn-hangzhou"
	}
	return region
}

// aliyunConfig 构造阿里云 SDK 通用配置。不手写 endpoint，由各产品 SDK 依据 RegionId 自动解析：
//   - CAS / DCDN：SDK 内置 EndpointMap 已把所有真实 region 映射到中心 endpoint（cas.aliyuncs.com / dcdn.aliyuncs.com）；
//   - CDN：EndpointRule=central，推导出 cdn.aliyuncs.com（海外 region 得到更准确的区域 endpoint）；
//   - ESA / GA：按 region 解析区域化 endpoint。
//
// 说明：darabonba-openapi 的 client.Config 与 utils.Config 均为 models.Config 的类型别名，
// 因此同一个配置对象可同时用于 CAS/CDN（老式 client）与 ESA（新式 client）。
func aliyunConfig(creds Credentials, regionID string) *openapi.Config {
	return &openapi.Config{
		AccessKeyId:     new(creds.AccessKeyID),
		AccessKeySecret: new(creds.AccessKeySecret),
		RegionId:        new(regionID),
		// 包裹带 HTTP 请求日志的 client（仅 DEBUG 生效），覆盖所有阿里云产品 SDK 的流量。
		HttpClient: &aliyunLoggingClient{client: &http.Client{}},
	}
}

// aliyunLoggingClient 适配阿里云 darabonba 的 dara.HttpClient 接口
// （Call(request *http.Request, transport *http.Transport)），用带 HTTP 请求日志的
// transport 发送请求，从而记录阿里云 SDK 流量。transport 由 SDK 按 runtime 配置传入。
type aliyunLoggingClient struct {
	mu     sync.Mutex
	client *http.Client
}

func (c *aliyunLoggingClient) Call(request *http.Request, transport *http.Transport) (*http.Response, error) {
	if transport != nil {
		// 仅当 transport 变化时才包裹，避免重复加锁赋值（WrapTransport 在非 DEBUG 下原样返回）。
		c.mu.Lock()
		c.client.Transport = httplog.WrapTransport(transport)
		c.mu.Unlock()
	}
	// 此处 request 由阿里云官方 SDK 构造，URL 为 SDK 依据产品/RegionId 推导的固定 endpoint，
	// 并非用户可控输入，本函数仅适配 dara.HttpClient 接口转发请求，不存在 SSRF 风险。
	return c.client.Do(request) // #nosec G704 -- request 由阿里云 SDK 构造，非用户输入
}

// shortCertHash 计算证书内容短指纹（sha256 前 12 位 hex），用于构造唯一且稳定的 CAS 证书名。
// 同一张证书内容不变 → 指纹不变 → 名称不变；不同证书 → 名称不同，从根本上避免 CAS 名称重复冲突。
func shortCertHash(pem string) string {
	sum := sha256.Sum256([]byte(pem))
	return hex.EncodeToString(sum[:])[:12]
}

// UploadCert 上传证书到阿里云 CAS（证书管理服务），返回证书 ID。
// 注意：阿里云 CAS 并不按证书内容去重，证书「名称(Name)」在账号内必须全局唯一；
// 同一张证书部署到多个目标（CDN/DCDN/ESA）或跨进程重启后重传时，若名称相同会报 NameRepeat。
// 因此这里把证书内容指纹并入名称（同名即同证书），并在遇到 NameRepeat 时按精确名称回查
// 已存在的证书并复用其 CertId，而不是再次上传。
func (d *AliyunDeployer) UploadCert(ctx context.Context, creds Credentials, cert CertContent, svcConfig map[string]string) (string, string, error) {
	name := certName(cert.Domain, svcConfig) + "-" + shortCertHash(cert.CertPEM)
	client, err := cas.NewClient(aliyunConfig(creds, aliyunRegion(creds.Region)))
	if err != nil {
		return "", "", i18n.Wrap(err, "deploy.error.aliyun_cas_client_create")
	}
	req := &cas.UploadUserCertificateRequest{
		Name: new(name),
		Cert: new(cert.CertPEM),
		Key:  new(cert.KeyPEM),
	}
	logging.Debug(i18n.T("log.deploy.aliyun_upload_start", "Domain", cert.Domain, "Name", name))
	resp, err := client.UploadUserCertificate(req)
	if err != nil {
		// 名称已存在（多为同证书重传）：按精确名称回查已有证书并复用，避免重复上传失败。
		if strings.Contains(err.Error(), "NameRepeat") {
			if existID, ferr := findCASCertByName(client, name); ferr == nil && existID != "" {
				logging.Debug(i18n.T("log.deploy.aliyun_upload_reuse", "CertID", existID, "Name", name))
				return existID, "", nil
			}
		}
		logging.Debug(i18n.T("log.deploy.aliyun_upload_failed", "Name", name, "Err", err, "Resp", respDump(resp)))
		return "", "", i18n.Wrap(err, "deploy.error.aliyun_cas_upload")
	}
	certID := ""
	if resp != nil && resp.Body != nil && resp.Body.CertId != nil {
		certID = strconv.FormatInt(*resp.Body.CertId, 10)
	}
	raw := ""
	if resp != nil {
		raw = tea.Prettify(resp.Body)
	}
	logging.Debug(i18n.T("log.deploy.aliyun_upload_success", "CertID", certID))
	return certID, raw, nil
}

// findCASCertByName 在 CAS 中按精确名称查找已存在的用户证书，返回其 CertId。
// ListUserCertificateOrder 仅支持 Keyword 模糊搜索，故拉取后用 Name 精确比对过滤。
func findCASCertByName(client *cas.Client, name string) (string, error) {
	req := &cas.ListUserCertificateOrderRequest{
		Keyword:     new(name),
		CurrentPage: new(int64(1)),
		ShowSize:    new(int64(100)),
	}
	resp, err := client.ListUserCertificateOrder(req)
	if err != nil {
		return "", err
	}
	if resp == nil || resp.Body == nil || resp.Body.CertificateOrderList == nil {
		return "", nil
	}
	for _, item := range resp.Body.CertificateOrderList {
		if item == nil || item.Name == nil || *item.Name != name {
			continue
		}
		if item.CertificateId != nil {
			return strconv.FormatInt(*item.CertificateId, 10), nil
		}
	}
	return "", nil
}

// findESACertByCasID 在指定 ESA 站点下按 CAS 证书 ID 查找已导入的证书，返回其 ESA 证书 ID。
// SetCertificate 报 Certificate.Duplicated 时用于确认「同一 CAS 证书是否已存在于站点」，
// 从而将重复部署识别为幂等成功。ListCertificates 结果里的 CasId 为字符串，与传入的
// certID（CAS 证书 ID 字符串）精确比对。
func findESACertByCasID(client *esa.Client, siteID int64, casID string) (string, error) {
	page := int32(1)
	for {
		req := &esa.ListCertificatesRequest{
			SiteId:     new(siteID),
			PageNumber: new(int64(page)),
			PageSize:   new(int64(100)),
		}
		resp, err := client.ListCertificates(req)
		if err != nil {
			return "", err
		}
		if resp == nil || resp.Body == nil {
			return "", nil
		}
		for _, item := range resp.Body.Result {
			if item == nil || item.CasId == nil || *item.CasId != casID {
				continue
			}
			if item.Id != nil {
				return *item.Id, nil
			}
		}
		total := int64(0)
		if resp.Body.TotalCount != nil {
			total = *resp.Body.TotalCount
		}
		if total == 0 || len(resp.Body.Result) == 0 || int64(page)*100 >= total || page >= 100 {
			break
		}
		page++
	}
	return "", nil
}

// DeployCert 将已上传的证书绑定部署到阿里云目标服务（CDN / ESA）
func (d *AliyunDeployer) DeployCert(ctx context.Context, creds Credentials, certID string, svc string, svcConfig map[string]string) (*DeployResult, error) {
	name := certName(svcConfig["cert_domain"], svcConfig)
	logging.Debug(i18n.T("log.deploy.aliyun_deploy_start", "Svc", svc, "CertID", certID))
	switch svc {
	case "cdn":
		domain := svcConfig["domain"]
		if domain == "" {
			return &DeployResult{CloudCertID: certID, Message: i18n.T("deploy.message.aliyun_cas_no_cdn_domain")}, nil
		}
		certIDInt, perr := strconv.ParseInt(certID, 10, 64)
		if perr != nil {
			return &DeployResult{CloudCertID: certID}, i18n.Wrap(perr, "deploy.error.aliyun_cdn_invalid_cert_id", "CertID", certID)
		}
		client, err := cdn.NewClient(aliyunConfig(creds, aliyunRegion(creds.Region)))
		if err != nil {
			return &DeployResult{CloudCertID: certID}, i18n.Wrap(err, "deploy.error.aliyun_cdn_client_create")
		}
		// CertType=cas + CertId 表示引用已上传到 CAS 的证书
		req := &cdn.SetCdnDomainSSLCertificateRequest{
			DomainName:  new(domain),
			CertName:    new(name),
			CertId:      new(certIDInt),
			CertType:    new("cas"),
			SSLProtocol: new("on"),
		}
		resp, err := client.SetCdnDomainSSLCertificate(req)
		if err != nil {
			logging.Debug(i18n.T("log.deploy.aliyun_cdn_deploy_failed", "Domain", domain, "Err", err, "Resp", respDump(resp)))
			return &DeployResult{CloudCertID: certID}, i18n.Wrap(err, "deploy.error.aliyun_cdn_deploy")
		}
		logging.Debug(i18n.T("log.deploy.aliyun_cdn_deploy_success", "Domain", domain))
		return &DeployResult{CloudCertID: certID, RawResponse: tea.Prettify(resp.Body), Message: i18n.T("deploy.message.aliyun_cdn_deployed", "Domain", domain)}, nil
	case "dcdn":
		// DCDN（全站加速）证书部署与 CDN 一致：CertType=cas + CertId 引用已上传到 CAS 的证书。
		// CAS 证书统一走 cas.aliyuncs.com（cn-hangzhou），故 CertRegion 用默认（不显式指定）。
		domain := svcConfig["domain"]
		if domain == "" {
			return &DeployResult{CloudCertID: certID, Message: i18n.T("deploy.message.aliyun_cas_no_dcdn_domain")}, nil
		}
		certIDInt, perr := strconv.ParseInt(certID, 10, 64)
		if perr != nil {
			return &DeployResult{CloudCertID: certID}, i18n.Wrap(perr, "deploy.error.aliyun_dcdn_invalid_cert_id", "CertID", certID)
		}
		client, err := dcdn.NewClient(aliyunConfig(creds, aliyunRegion(creds.Region)))
		if err != nil {
			return &DeployResult{CloudCertID: certID}, i18n.Wrap(err, "deploy.error.aliyun_dcdn_client_create")
		}
		req := &dcdn.SetDcdnDomainSSLCertificateRequest{
			DomainName:  new(domain),
			CertId:      new(certIDInt),
			CertType:    new("cas"),
			SSLProtocol: new("on"),
		}
		resp, err := client.SetDcdnDomainSSLCertificate(req)
		if err != nil {
			logging.Debug(i18n.T("log.deploy.aliyun_dcdn_deploy_failed", "Domain", domain, "Err", err, "Resp", respDump(resp)))
			return &DeployResult{CloudCertID: certID}, i18n.Wrap(err, "deploy.error.aliyun_dcdn_deploy")
		}
		logging.Debug(i18n.T("log.deploy.aliyun_dcdn_deploy_success", "Domain", domain))
		return &DeployResult{CloudCertID: certID, RawResponse: tea.Prettify(resp.Body), Message: i18n.T("deploy.message.aliyun_dcdn_deployed", "Domain", domain)}, nil
	case "esa":
		// ESA（边缘安全加速）：证书已上传到 CAS，这里用 SetCertificate 以 cas 类型
		// 引用 CAS 证书 ID（CasId）绑定到站点（SiteId）。SiteId 来自部署目标配置。
		siteID := svcConfig["site_id"]
		if siteID == "" {
			return &DeployResult{CloudCertID: certID, Message: i18n.T("deploy.message.aliyun_cas_no_esa_site")}, nil
		}
		region := aliyunRegion(creds.Region)
		siteIDInt, serr := strconv.ParseInt(siteID, 10, 64)
		if serr != nil {
			return &DeployResult{CloudCertID: certID}, i18n.Wrap(serr, "deploy.error.aliyun_esa_invalid_site_id", "SiteID", siteID)
		}
		casIDInt, cerr := strconv.ParseInt(certID, 10, 64)
		if cerr != nil {
			return &DeployResult{CloudCertID: certID}, i18n.Wrap(cerr, "deploy.error.aliyun_esa_invalid_cert_id", "CertID", certID)
		}
		// ESA 使用区域化 endpoint，endpoint 留空由 SDK 按 RegionId 推导
		client, err := esa.NewClient(aliyunConfig(creds, region))
		if err != nil {
			return &DeployResult{CloudCertID: certID}, i18n.Wrap(err, "deploy.error.aliyun_esa_client_create")
		}
		req := &esa.SetCertificateRequest{
			SiteId: new(siteIDInt),
			Type:   new("cas"),
			CasId:  new(casIDInt),
			Region: new(region),
		}
		resp, err := client.SetCertificate(req)
		if err != nil {
			// 证书名称已存在（Certificate.Duplicated）：多为同一张 CAS 证书重复部署到该站点
			// （手动重跑或续期后二次部署）。ESA 导入 CAS 证书时按内容派生名称，同名即同证书，
			// 已在站点内存在。此时按 CasId 回查站点已有证书确认存在，则视为「已部署」幂等成功，
			// 而非报错，避免用户看到 Certificate.Duplicated。
			if strings.Contains(err.Error(), "Certificate.Duplicated") {
				if existID, ferr := findESACertByCasID(client, siteIDInt, certID); ferr == nil && existID != "" {
					logging.Debug(i18n.T("log.deploy.aliyun_esa_deploy_exists", "SiteID", siteID, "EsaCertID", existID))
					return &DeployResult{CloudCertID: certID, Message: i18n.T("deploy.message.aliyun_esa_already_deployed", "SiteID", siteID)}, nil
				}
			}
			logging.Debug(i18n.T("log.deploy.aliyun_esa_deploy_failed", "SiteID", siteID, "Err", err, "Resp", respDump(resp)))
			return &DeployResult{CloudCertID: certID}, i18n.Wrap(err, "deploy.error.aliyun_esa_deploy")
		}
		logging.Debug(i18n.T("log.deploy.aliyun_esa_deploy_success", "SiteID", siteID))
		return &DeployResult{CloudCertID: certID, RawResponse: tea.Prettify(resp.Body), Message: i18n.T("deploy.message.aliyun_esa_deployed", "SiteID", siteID)}, nil
	case "ga":
		// 全球加速 GA：证书已上传到 CAS，这里用 AssociateAdditionalCertificatesWithListener
		// 把 CAS 证书 ID 关联到指定 HTTPS 监听器（按域名生效）。accelerator_id / listener_id
		// 来自部署目标配置，domain 为本次部署的加速域名。
		acceleratorID := svcConfig["accelerator_id"]
		listenerID := svcConfig["listener_id"]
		domain := svcConfig["domain"]
		if acceleratorID == "" || listenerID == "" {
			return &DeployResult{CloudCertID: certID, Message: i18n.T("deploy.message.aliyun_cas_no_ga_ids")}, nil
		}
		if domain == "" {
			return &DeployResult{CloudCertID: certID, Message: i18n.T("deploy.message.aliyun_cas_no_ga_domain")}, nil
		}
		// GA 为全局加速服务，地域固定 cn-hangzhou，endpoint 由 SDK 按 RegionId 推导
		client, err := ga.NewClient(aliyunConfig(creds, "cn-hangzhou"))
		if err != nil {
			return &DeployResult{CloudCertID: certID}, i18n.Wrap(err, "deploy.error.aliyun_ga_client_create")
		}
		req := &ga.AssociateAdditionalCertificatesWithListenerRequest{
			AcceleratorId: new(acceleratorID),
			ListenerId:    new(listenerID),
			RegionId:      new("cn-hangzhou"),
			Certificates: []*ga.AssociateAdditionalCertificatesWithListenerRequestCertificates{
				{Id: new(certID), Domain: new(domain)},
			},
		}
		resp, err := client.AssociateAdditionalCertificatesWithListener(req)
		if err != nil {
			logging.Debug(i18n.T("log.deploy.aliyun_ga_deploy_failed", "Acc", acceleratorID, "Listener", listenerID, "Err", err, "Resp", respDump(resp)))
			return &DeployResult{CloudCertID: certID}, i18n.Wrap(err, "deploy.error.aliyun_ga_deploy")
		}
		logging.Debug(i18n.T("log.deploy.aliyun_ga_deploy_success", "Acc", acceleratorID, "Listener", listenerID, "Domain", domain))
		return &DeployResult{CloudCertID: certID, RawResponse: tea.Prettify(resp.Body), Message: i18n.T("deploy.message.aliyun_ga_deployed", "ListenerID", listenerID, "Domain", domain)}, nil
	default:
		return &DeployResult{CloudCertID: certID, Message: i18n.T("deploy.message.aliyun_cas_not_implemented", "Service", svc)}, nil
	}
}

// ListDomains 列出该账号在指定服务下的可绑定目标。
// - cdn：阿里云 CDN 加速域名（DescribeUserDomains）
// - esa：
//   - 不传 zoneID 时返回 ESA 站点列表（ListSites），形如 "站点名||SiteId" 便于前端展示与回填；
//   - 传入 zoneID（即 SiteId）时返回该站点下的记录域名（ListRecords），供展示/选择。
func (d *AliyunDeployer) ListDomains(ctx context.Context, creds Credentials, svc, region, zoneID string) ([]string, error) {
	switch svc {
	case "cdn":
		return d.listAliyunCDNDomains(ctx, creds, region)
	case "dcdn":
		return d.listAliyunDCDNDomains(ctx, creds, region)
	case "esa":
		if zoneID != "" {
			return d.listESARecords(ctx, creds, region, zoneID)
		}
		return d.listESASites(ctx, creds, region)
	default:
		return []string{}, nil
	}
}

// ListSites 云厂商默认回退到 ListDomains。
func (d *AliyunDeployer) ListSites(ctx context.Context, creds Credentials, svc, region, zoneID string) ([]string, error) {
	return d.ListDomains(ctx, creds, svc, region, zoneID)
}

// listAliyunCDNDomains 列出阿里云 CDN 加速域名
func (d *AliyunDeployer) listAliyunCDNDomains(ctx context.Context, creds Credentials, region string) ([]string, error) {
	client, err := cdn.NewClient(aliyunConfig(creds, aliyunRegion(region)))
	if err != nil {
		return nil, i18n.Wrap(err, "deploy.error.aliyun_cdn_client_create")
	}
	logging.Debug(i18n.T("log.deploy.aliyun_list_cdn_start", "Region", region))
	var domains []string
	page := int32(1)
	for {
		req := &cdn.DescribeUserDomainsRequest{
			PageSize:   tea.Int32(500),
			PageNumber: new(page),
		}
		resp, err := client.DescribeUserDomains(req)
		if err != nil {
			logging.Debug(i18n.T("log.deploy.aliyun_list_cdn_failed", "Region", region, "Err", err, "Resp", respDump(resp)))
			return nil, i18n.Wrap(err, "deploy.error.aliyun_cdn_list_domains")
		}
		if resp.Body == nil || resp.Body.Domains == nil {
			break
		}
		pageData := resp.Body.Domains.PageData
		for _, dm := range pageData {
			if dm != nil && dm.DomainName != nil && *dm.DomainName != "" {
				domains = append(domains, *dm.DomainName)
			}
		}
		total := int64(0)
		if resp.Body.TotalCount != nil {
			total = *resp.Body.TotalCount
		}
		if total == 0 || len(pageData) == 0 || int64(len(domains)) >= total || page >= 100 {
			break
		}
		page++
	}
	logging.Debug(i18n.T("log.deploy.aliyun_list_cdn_success", "Region", region, "Count", len(domains)))
	return domains, nil
}

// listAliyunDCDNDomains 列出阿里云 DCDN（全站加速）加速域名
func (d *AliyunDeployer) listAliyunDCDNDomains(ctx context.Context, creds Credentials, region string) ([]string, error) {
	client, err := dcdn.NewClient(aliyunConfig(creds, aliyunRegion(region)))
	if err != nil {
		return nil, i18n.Wrap(err, "deploy.error.aliyun_dcdn_client_create")
	}
	logging.Debug(i18n.T("log.deploy.aliyun_list_dcdn_start", "Region", region))
	var domains []string
	page := int32(1)
	for {
		req := &dcdn.DescribeDcdnUserDomainsRequest{
			PageSize:   tea.Int32(500),
			PageNumber: new(page),
		}
		resp, err := client.DescribeDcdnUserDomains(req)
		if err != nil {
			logging.Debug(i18n.T("log.deploy.aliyun_list_dcdn_failed", "Region", region, "Err", err, "Resp", respDump(resp)))
			return nil, i18n.Wrap(err, "deploy.error.aliyun_dcdn_list_domains")
		}
		if resp.Body == nil || resp.Body.Domains == nil {
			break
		}
		pageData := resp.Body.Domains.PageData
		for _, dm := range pageData {
			if dm != nil && dm.DomainName != nil && *dm.DomainName != "" {
				domains = append(domains, *dm.DomainName)
			}
		}
		total := int64(0)
		if resp.Body.TotalCount != nil {
			total = *resp.Body.TotalCount
		}
		if total == 0 || len(pageData) == 0 || int64(len(domains)) >= total || page >= 100 {
			break
		}
		page++
	}
	logging.Debug(i18n.T("log.deploy.aliyun_list_dcdn_success", "Region", region, "Count", len(domains)))
	return domains, nil
}

// listESASites 列出阿里云 ESA 站点，返回 "站点名||SiteId" 形式，
// 前端按 "||" 拆分得到展示名与 SiteId（SiteId 即 SetCertificate 绑定所需的 Id）。
func (d *AliyunDeployer) listESASites(ctx context.Context, creds Credentials, region string) ([]string, error) {
	// region 与 Deploy 分支保持一致：空 region 经 aliyunRegion 兜底为 cn-hangzhou（ESA 大陆主站），
	// 否则 esa.NewClient 会因 "RegionId is empty" 直接失败。ESA 为区域化服务，endpoint 由 SDK 按
	// RegionId 推导（EndpointMap 仅含 cn-hangzhou / ap-southeast-1）；只有「显式设置了非 ESA 区域」
	// 才会把错误 region 透传给对端（被 reset），从而提示用户改回正确区域。
	client, err := esa.NewClient(aliyunConfig(creds, aliyunRegion(region)))
	if err != nil {
		return nil, i18n.Wrap(err, "deploy.error.aliyun_esa_client_create")
	}
	logging.Debug(i18n.T("log.deploy.aliyun_list_esa_start", "Region", region))
	var sites []string
	page := int32(1)
	for {
		req := &esa.ListSitesRequest{
			PageNumber: new(page),
			PageSize:   tea.Int32(500),
		}
		resp, err := client.ListSites(req)
		if err != nil {
			logging.Debug(i18n.T("log.deploy.aliyun_list_esa_failed", "Region", region, "Err", err, "Resp", respDump(resp)))
			return nil, i18n.Wrap(err, "deploy.error.aliyun_esa_list_sites")
		}
		if resp.Body == nil {
			break
		}
		for _, st := range resp.Body.Sites {
			if st != nil && st.SiteId != nil {
				name := ""
				if st.SiteName != nil {
					name = *st.SiteName
				}
				sites = append(sites, name+"||"+strconv.FormatInt(*st.SiteId, 10))
			}
		}
		total := 0
		if resp.Body.TotalCount != nil {
			total = int(*resp.Body.TotalCount)
		}
		if total == 0 || len(resp.Body.Sites) == 0 || len(sites) >= total || page >= 100 {
			break
		}
		page++
	}
	logging.Debug(i18n.T("log.deploy.aliyun_list_esa_success", "Region", region, "Count", len(sites)))
	return sites, nil
}

// listESARecords 列出指定 ESA 站点（SiteId）下的记录域名，用于展示/选择（按名称去重）。
func (d *AliyunDeployer) listESARecords(ctx context.Context, creds Credentials, region, siteID string) ([]string, error) {
	siteIDInt, err := strconv.ParseInt(siteID, 10, 64)
	if err != nil {
		return nil, i18n.Wrap(err, "deploy.error.aliyun_esa_invalid_site_id_param", "SiteID", siteID)
	}
	// 同 listESASites：空 region 兜底为 cn-hangzhou，仅「显式设置了非 ESA 区域」才透传错误 region。
	client, err := esa.NewClient(aliyunConfig(creds, aliyunRegion(region)))
	if err != nil {
		return nil, i18n.Wrap(err, "deploy.error.aliyun_esa_client_create")
	}
	logging.Debug(i18n.T("log.deploy.aliyun_list_esa_domain_start", "Region", region, "SiteID", siteID))
	var domains []string
	seen := map[string]struct{}{}
	page := int32(1)
	for {
		req := &esa.ListRecordsRequest{
			SiteId:     new(siteIDInt),
			PageNumber: new(page),
			PageSize:   tea.Int32(500),
		}
		resp, err := client.ListRecords(req)
		if err != nil {
			logging.Debug(i18n.T("log.deploy.aliyun_list_esa_domain_failed", "Region", region, "SiteID", siteID, "Err", err, "Resp", respDump(resp)))
			return nil, i18n.Wrap(err, "deploy.error.aliyun_esa_list_domains")
		}
		if resp.Body == nil {
			break
		}
		for _, rc := range resp.Body.Records {
			if rc == nil || rc.RecordName == nil || *rc.RecordName == "" {
				continue
			}
			if _, ok := seen[*rc.RecordName]; ok {
				continue
			}
			seen[*rc.RecordName] = struct{}{}
			domains = append(domains, *rc.RecordName)
		}
		total := int32(0)
		if resp.Body.TotalCount != nil {
			total = *resp.Body.TotalCount
		}
		if total == 0 || len(resp.Body.Records) == 0 || page >= 100 {
			break
		}
		page++
	}
	logging.Debug(i18n.T("log.deploy.aliyun_list_esa_domain_success", "Region", region, "SiteID", siteID, "Count", len(domains)))
	return domains, nil
}

// GetCurrentCert 查询阿里云资源当前生效的 SSL 证书。
//   - CDN：DescribeDomainCertificateInfo，证书链（含叶子）在 ServerCertificate 字段，
//     ServerCertificateStatus=off 表示 HTTPS 未开启。
//   - DCDN：DescribeDcdnDomainCertificateInfo，证书公钥在 SSLPub 字段，SSLProtocol=off 表示未开启。
//   - ESA：ListCertificates 按站点查询，按域名匹配覆盖该域名的当前生效证书（仅返回元数据）。
func (d *AliyunDeployer) GetCurrentCert(ctx context.Context, creds Credentials, svc string, svcConfig map[string]string) (*CurrentCert, error) {
	logging.Debug(i18n.T("log.deploy.aliyun_get_current_cert",
		"Svc", svc,
		"Domain", svcConfig["domain"]))
	domain := svcConfig["domain"]
	if strings.TrimSpace(domain) == "" {
		return nil, i18n.NewError("deploy.error.current_cert_domain_empty")
	}

	switch svc {
	case "cdn":
		client, err := cdn.NewClient(aliyunConfig(creds, aliyunRegion(creds.Region)))
		if err != nil {
			return nil, i18n.Wrap(err, "deploy.error.current_cert_query")
		}
		resp, err := client.DescribeDomainCertificateInfo(&cdn.DescribeDomainCertificateInfoRequest{
			DomainName: new(domain),
		})
		if err != nil {
			return nil, i18n.Wrap(err, "deploy.error.current_cert_query")
		}
		if resp.Body == nil || resp.Body.CertInfos == nil || len(resp.Body.CertInfos.CertInfo) == 0 {
			return nil, i18n.NewError("deploy.error.current_cert_not_configured")
		}
		info := resp.Body.CertInfos.CertInfo[0]
		// ServerCertificateStatus: on/off，off 表示 HTTPS 未开启，无生效证书。
		if tea.StringValue(info.ServerCertificateStatus) == "off" {
			return nil, i18n.NewError("deploy.error.current_cert_not_configured")
		}
		pem := strings.TrimSpace(tea.StringValue(info.ServerCertificate))
		if pem == "" {
			return nil, i18n.NewError("deploy.error.current_cert_not_configured")
		}
		return parseCertPEM(pem)

	case "dcdn":
		client, err := dcdn.NewClient(aliyunConfig(creds, aliyunRegion(creds.Region)))
		if err != nil {
			return nil, i18n.Wrap(err, "deploy.error.current_cert_query")
		}
		resp, err := client.DescribeDcdnDomainCertificateInfo(&dcdn.DescribeDcdnDomainCertificateInfoRequest{
			DomainName: new(domain),
		})
		if err != nil {
			return nil, i18n.Wrap(err, "deploy.error.current_cert_query")
		}
		if resp.Body == nil || resp.Body.CertInfos == nil || len(resp.Body.CertInfos.CertInfo) == 0 {
			return nil, i18n.NewError("deploy.error.current_cert_not_configured")
		}
		info := resp.Body.CertInfos.CertInfo[0]
		// SSLProtocol: on/off，off 表示未开启 HTTPS，无生效证书。
		if tea.StringValue(info.SSLProtocol) == "off" {
			return nil, i18n.NewError("deploy.error.current_cert_not_configured")
		}
		pem := strings.TrimSpace(tea.StringValue(info.SSLPub))
		if pem == "" {
			return nil, i18n.NewError("deploy.error.current_cert_not_configured")
		}
		return parseCertPEM(pem)

	case "esa":
		// ESA 证书按站点（site_id）维度管理，ListCertificates 仅返回证书元数据（不含 PEM），
		// 故基于元数据直接构造 CurrentCert，并按域名匹配站点下当前覆盖该域名的证书。
		siteIDStr := svcConfig["site_id"]
		if siteIDStr == "" {
			return nil, i18n.NewError("deploy.error.site_id_required")
		}
		siteID, err := strconv.ParseInt(siteIDStr, 10, 64)
		if err != nil {
			return nil, i18n.NewError("deploy.error.site_id_required")
		}
		client, err := esa.NewClient(aliyunConfig(creds, aliyunRegion(creds.Region)))
		if err != nil {
			return nil, i18n.Wrap(err, "deploy.error.current_cert_query")
		}
		logging.Debug(i18n.T("log.deploy.aliyun_esa_get_current_cert",
			"Svc", svc, "Domain", domain, "SiteID", siteIDStr))
		certs, err := d.esaCertsForSite(client, siteID, creds.AccessKeyID+"@"+creds.Region)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", i18n.T("deploy.error.current_cert_query_failed"), err)
		}
		info := matchESACertByDomain(certs, domain)
		if info == nil {
			logging.Debug(i18n.T("log.deploy.aliyun_esa_get_current_cert_none",
				"SiteID", siteIDStr, "Domain", domain))
			return nil, i18n.NewError("deploy.error.current_cert_not_configured")
		}
		cc := &CurrentCert{
			CommonName:   tea.StringValue(info.CommonName),
			SANs:         splitCommaList(tea.StringValue(info.SAN)),
			Issuer:       tea.StringValue(info.Issuer),
			NotBefore:    parseESACertDate(tea.StringValue(info.NotBefore)),
			NotAfter:     parseESACertDate(tea.StringValue(info.NotAfter)),
			SerialNumber: tea.StringValue(info.SerialNumber),
		}
		logging.Debug(i18n.T("log.deploy.aliyun_esa_get_current_cert_result",
			"Domain", domain, "CN", cc.CommonName, "NotAfter", cc.NotAfter))
		return cc, nil

	case "ga":
		// 全球加速 GA：证书绑定在 HTTPS 监听器上，DescribeListener 仅在 Certificates 中返回
		// 证书 ID（Id），不含公钥/PEM。故先按 listener_id 取监听器的证书 ID，再用 CAS 的
		// GetUserCertificateDetail 反查证书内容（PEM），最后复用 parseCertPEM 构造 CurrentCert。
		// 注：GA 为「监听器/实例」维度而非域名维度，监听器 ID 来自部署目标配置。
		listenerID := svcConfig["listener_id"]
		if strings.TrimSpace(listenerID) == "" {
			return nil, i18n.NewError("deploy.error.aliyun_ga_listener_id_required")
		}
		gaClient, err := ga.NewClient(aliyunConfig(creds, "cn-hangzhou"))
		if err != nil {
			return nil, i18n.Wrap(err, "deploy.error.current_cert_query")
		}
		gaResp, err := gaClient.DescribeListener(&ga.DescribeListenerRequest{
			ListenerId: new(listenerID),
			RegionId:   new("cn-hangzhou"),
		})
		if err != nil {
			return nil, i18n.Wrap(err, "deploy.error.current_cert_query")
		}
		if gaResp.Body == nil {
			return nil, i18n.NewError("deploy.error.current_cert_not_configured")
		}
		if tea.StringValue(gaResp.Body.Protocol) != "https" {
			return nil, i18n.NewError("deploy.error.current_cert_not_configured")
		}
		certs := gaResp.Body.Certificates
		if len(certs) == 0 {
			return nil, i18n.NewError("deploy.error.current_cert_not_configured")
		}
		var certIDStr string
		for _, c := range certs {
			if c == nil || tea.StringValue(c.Id) == "" {
				continue
			}
			if tea.StringValue(c.Type) != "Default" {
				certIDStr = tea.StringValue(c.Id)
				break
			}
			if certIDStr == "" {
				certIDStr = tea.StringValue(c.Id)
			}
		}
		if certIDStr == "" {
			return nil, i18n.NewError("deploy.error.current_cert_not_configured")
		}
		certIDInt, perr := strconv.ParseInt(certIDStr, 10, 64)
		if perr != nil {
			return nil, i18n.Wrap(perr, "deploy.error.current_cert_query")
		}
		casClient, err := cas.NewClient(aliyunConfig(creds, "cn-hangzhou"))
		if err != nil {
			return nil, i18n.Wrap(err, "deploy.error.current_cert_query")
		}
		// 单资源第二次云 API 调用，需再取一个限速令牌，避免实际 QPS 翻倍触发频率限制。
		currentCertRateWait()
		casResp, err := casClient.GetUserCertificateDetail(&cas.GetUserCertificateDetailRequest{
			CertId:     new(certIDInt),
			CertFilter: new(false),
		})
		if err != nil {
			return nil, i18n.Wrap(err, "deploy.error.current_cert_query")
		}
		if casResp.Body == nil {
			return nil, i18n.NewError("deploy.error.current_cert_not_configured")
		}
		pem := strings.TrimSpace(tea.StringValue(casResp.Body.Cert))
		if pem == "" {
			return nil, i18n.NewError("deploy.error.current_cert_not_configured")
		}
		return parseCertPEM(pem)

	default:
		return nil, i18n.NewError("deploy.error.current_cert_cloud_not_supported")
	}
}

// BeforeCurrentCerts 实现 currentCertBatch：批量查询前清空 ESA 证书缓存，保证刷新能拿到最新数据。
func (a *AliyunDeployer) BeforeCurrentCerts(ctx context.Context) {
	a.esaCertsCacheMu.Lock()
	defer a.esaCertsCacheMu.Unlock()
	a.esaCertsCache = nil
}

// esaCertsForSite 返回指定站点的全部证书；同一次批量内相同 site_id+凭证只真正拉取一次 ListCertificates。
// 并发安全：以 "credKey@siteID" 为键缓存，不同站点互不覆盖。
func (a *AliyunDeployer) esaCertsForSite(client *esa.Client, siteID int64, credKey string) ([]*esa.ListCertificatesResponseBodyResult, error) {
	cacheKey := credKey + "@" + strconv.FormatInt(siteID, 10)
	a.esaCertsCacheMu.Lock()
	if a.esaCertsCache == nil {
		a.esaCertsCache = make(map[string][]*esa.ListCertificatesResponseBodyResult)
	}
	if c, ok := a.esaCertsCache[cacheKey]; ok {
		a.esaCertsCacheMu.Unlock()
		return c, nil
	}
	a.esaCertsCacheMu.Unlock()

	certs, err := listAllESACerts(client, siteID)
	if err != nil {
		return nil, err
	}
	a.esaCertsCacheMu.Lock()
	a.esaCertsCache[cacheKey] = certs
	a.esaCertsCacheMu.Unlock()
	return certs, nil
}

// listAllESACerts 分页拉取 ESA 站点下的全部证书（ListCertificates 仅返回元数据，不含 PEM）。
func listAllESACerts(client *esa.Client, siteID int64) ([]*esa.ListCertificatesResponseBodyResult, error) {
	var all []*esa.ListCertificatesResponseBodyResult
	page := int64(1)
	pageSize := int64(50)
	for {
		req := &esa.ListCertificatesRequest{}
		req.SetSiteId(siteID)
		req.SetPageNumber(page)
		req.SetPageSize(pageSize)
		resp, err := client.ListCertificates(req)
		if err != nil {
			return nil, err
		}
		if resp == nil || resp.Body == nil {
			break
		}
		all = append(all, resp.Body.Result...)
		if tea.Int64Value(resp.Body.TotalCount) <= int64(len(all)) || len(resp.Body.Result) == 0 {
			break
		}
		page++
	}
	return all, nil
}

// matchESACertByDomain 从站点证书列表中挑出覆盖目标域名的当前生效证书。
// 优先匹配 CommonName/SAN（支持通配符），在命中项里偏向未过期（Status 非 Expired）且 NotAfter 最新者。
func matchESACertByDomain(certs []*esa.ListCertificatesResponseBodyResult, domain string) *esa.ListCertificatesResponseBodyResult {
	domain = strings.ToLower(strings.TrimSpace(domain))
	var active, expired []*esa.ListCertificatesResponseBodyResult
	for _, c := range certs {
		if c == nil {
			continue
		}
		if !certCoversDomain(c, domain) {
			continue
		}
		if strings.EqualFold(tea.StringValue(c.Status), "Expired") {
			expired = append(expired, c)
		} else {
			active = append(active, c)
		}
	}
	if len(active) > 0 {
		return latestByNotAfter(active)
	}
	if len(expired) > 0 {
		return latestByNotAfter(expired)
	}
	return nil
}

// certCoversDomain 判断证书是否覆盖给定域名（CN 或任一 SAN，支持 *.example.com 通配）。
func certCoversDomain(c *esa.ListCertificatesResponseBodyResult, domain string) bool {
	if hostMatchesPattern(domain, tea.StringValue(c.CommonName)) {
		return true
	}
	for _, s := range splitCommaList(tea.StringValue(c.SAN)) {
		if hostMatchesPattern(domain, s) {
			return true
		}
	}
	return false
}

// hostMatchesPattern 域名与证书名称匹配，支持通配符 *.example.com。
func hostMatchesPattern(domain, pattern string) bool {
	domain = strings.ToLower(strings.TrimSpace(domain))
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	if domain == "" || pattern == "" {
		return false
	}
	if domain == pattern {
		return true
	}
	if strings.HasPrefix(pattern, "*.") {
		base := pattern[1:] // ".example.com"
		return domain == pattern[2:] || strings.HasSuffix("."+domain, base)
	}
	return false
}

// latestByNotAfter 取 NotAfter 最新的证书。
func latestByNotAfter(certs []*esa.ListCertificatesResponseBodyResult) *esa.ListCertificatesResponseBodyResult {
	var best *esa.ListCertificatesResponseBodyResult
	var bestT time.Time
	for _, c := range certs {
		t, err := time.Parse("2006-01-02 15:04:05", tea.StringValue(c.NotAfter))
		if err != nil {
			t = time.Time{}
		}
		if best == nil || t.After(bestT) {
			best = c
			bestT = t
		}
	}
	return best
}

// splitCommaList 按逗号拆分证书 SAN 字符串。
func splitCommaList(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// parseESACertDate 将 ESA 证书日期（YYYY-MM-DD HH:MM:SS，UTC）转为 RFC3339；解析失败原样返回。
func parseESACertDate(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if t, err := time.Parse("2006-01-02 15:04:05", s); err == nil {
		return t.UTC().Format(time.RFC3339)
	}
	return s
}
