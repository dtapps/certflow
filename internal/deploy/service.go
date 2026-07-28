package deploy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"strings"
	"sync"
	"time"

	"reflect"

	"cnb.cool/dtapp/certflow/ent"
	"cnb.cool/dtapp/certflow/ent/certificate"
	"cnb.cool/dtapp/certflow/ent/certupload"
	"cnb.cool/dtapp/certflow/ent/deploylog"
	"cnb.cool/dtapp/certflow/ent/deploytarget"
	"cnb.cool/dtapp/certflow/internal/config"
	"cnb.cool/dtapp/certflow/internal/deploycredential"
	"cnb.cool/dtapp/certflow/internal/dnsprovider"
	"cnb.cool/dtapp/certflow/internal/i18n"
	"cnb.cool/dtapp/certflow/internal/logging"
	"cnb.cool/dtapp/certflow/internal/notification"
)

// GetCurrentCerts 查询当前生效证书的限流参数。
const (
	// currentCertMaxConcurrency 并发上限：同时最多这么多个查询请求在途。
	currentCertMaxConcurrency = 5
	// currentCertMaxQPS 发起速率上限（次/秒）：避免并发槽位复用过快导致
	// 每秒请求数超过云厂商 API 频率上限（如腾讯云 20 QPS）。
	currentCertMaxQPS = 5
)

// currentCertRateTicker 全局限速器：所有当前生效证书查询的云 API 调用共享，
// 避免多个部署目标同时刷新时限速各自独立而叠加超限。
var currentCertRateTicker = time.NewTicker(time.Second / currentCertMaxQPS)

// currentCertRateWait 在发起一次云 API 调用前取一个令牌（阻塞至有配额）。
// 注意：限速按「API 调用次数」计，单个资源查询若需多次云 API 调用
// （如腾讯云 EdgeOne、阿里云 GA、百度云均为 2 次），每次调用前都必须取令牌，
// 否则实际 QPS 会翻倍并触发云厂商频率限制。
func currentCertRateWait() {
	<-currentCertRateTicker.C
}

// respDump 将各厂商 SDK 响应安全序列化为单行字符串，便于在调用失败时随错误日志一并打印以便排查。
// 云厂商在业务错误（HTTP 200 + 错误码）时通常仍返回已解析的响应（含错误信封），打印它可暴露云端错误码/消息；
// 在传输错误时响应为 nil 指针，此时返回 "<nil>"。用反射判断指针是否为 nil，规避 nil 指针装箱进 interface{} 后 != nil 的误判。
func respDump(resp any) string {
	if resp == nil {
		return "<nil>"
	}
	rv := reflect.ValueOf(resp)
	if rv.Kind() == reflect.Pointer && rv.IsNil() {
		return "<nil>"
	}
	switch v := resp.(type) {
	case interface{ ToJsonString() string }: // 腾讯云 SDK 响应
		return v.ToJsonString()
	case interface{ String() string }: // 阿里云/华为云 SDK 响应
		return v.String()
	default:
		b, _ := json.Marshal(resp)
		return string(b)
	}
}

// registry 部署器注册表（由各云文件 init 注册）
var registry = map[string]Deployer{}

// RegisterDeployer 注册部署器
func RegisterDeployer(d Deployer) {
	registry[d.Provider()] = d
}

// DeployService 证书部署服务
type DeployService struct {
	db           *ent.Client
	notifService *notification.NotificationService
	// uploadCache 进程内上传缓存（快速层），键为「厂商|AccessKeyId|区域|证书指纹」。
	// 去重的权威来源是数据库 CertUpload 表（跨进程持久化）；本内存缓存只是同进程内的加速层。
	// 两者都由 ensureUploaded 维护，保证同一张证书对同一云账号只真正上传一次。
	uploadCache map[string]string
	uploadMu    sync.Mutex
}

// NewDeployService 创建部署服务
func NewDeployService(db *ent.Client) *DeployService {
	return &DeployService{db: db}
}

// SetNotificationService 注入通知服务（用于部署成功/失败通知）
func (s *DeployService) SetNotificationService(ns *notification.NotificationService) {
	s.notifService = ns
}

// CreateDeployTargetInput 创建部署目标输入
type CreateDeployTargetInput struct {
	Name               string            `json:"name"`
	ProviderType       string            `json:"provider_type"`
	DeployService      string            `json:"deploy_service"`
	Config             map[string]string `json:"config"`
	CredentialSource   string            `json:"credential_source"`
	DNSProviderID      *int              `json:"dns_provider_id,omitempty"`
	DeployCredentialID *int              `json:"deploy_credential_id,omitempty"`
	IsActive           bool              `json:"is_active"`
	Comment            string            `json:"comment"`
}

// UpdateDeployTargetInput 更新部署目标输入
type UpdateDeployTargetInput struct {
	Name               string            `json:"name,omitempty"`
	ProviderType       string            `json:"provider_type,omitempty"`
	DeployService      string            `json:"deploy_service,omitempty"`
	Config             map[string]string `json:"config,omitempty"`
	CredentialSource   string            `json:"credential_source,omitempty"`
	DNSProviderID      *int              `json:"dns_provider_id,omitempty"`
	DeployCredentialID *int              `json:"deploy_credential_id,omitempty"`
	IsActive           *bool             `json:"is_active,omitempty"`
	Comment            string            `json:"comment,omitempty"`
}

// Create 创建部署目标
func (s *DeployService) Create(ctx context.Context, in CreateDeployTargetInput) (*ent.DeployTarget, error) {
	cfg, _ := json.Marshal(in.Config)
	b := s.db.DeployTarget.Create().
		SetName(in.Name).
		SetProviderType(deploytarget.ProviderType(in.ProviderType)).
		SetDeployService(in.DeployService).
		SetConfig(cfg).
		SetCredentialSource(deploytarget.CredentialSource(in.CredentialSource)).
		SetIsActive(in.IsActive).
		SetComment(in.Comment)
	if in.DNSProviderID != nil && *in.DNSProviderID > 0 {
		b = b.SetDNSProviderID(*in.DNSProviderID)
	}
	if in.DeployCredentialID != nil && *in.DeployCredentialID > 0 {
		b = b.SetDeployCredentialID(*in.DeployCredentialID)
	}
	return b.Save(ctx)
}

// Update 更新部署目标
func (s *DeployService) Update(ctx context.Context, id int, in UpdateDeployTargetInput) (*ent.DeployTarget, error) {
	b := s.db.DeployTarget.UpdateOneID(id)
	if in.Name != "" {
		b = b.SetName(in.Name)
	}
	if in.ProviderType != "" {
		b = b.SetProviderType(deploytarget.ProviderType(in.ProviderType))
	}
	if in.DeployService != "" {
		b = b.SetDeployService(in.DeployService)
	}
	if in.Config != nil {
		cfg, _ := json.Marshal(in.Config)
		b = b.SetConfig(cfg)
	}
	if in.CredentialSource != "" {
		b = b.SetCredentialSource(deploytarget.CredentialSource(in.CredentialSource))
	}
	if in.DNSProviderID != nil {
		if *in.DNSProviderID > 0 {
			b = b.SetDNSProviderID(*in.DNSProviderID)
		}
	}
	if in.DeployCredentialID != nil {
		if *in.DeployCredentialID > 0 {
			b = b.SetDeployCredentialID(*in.DeployCredentialID)
		}
	}
	if in.IsActive != nil {
		b = b.SetIsActive(*in.IsActive)
	}
	if in.Comment != "" {
		b = b.SetComment(in.Comment)
	}
	return b.Save(ctx)
}

// Delete 删除部署目标
func (s *DeployService) Delete(ctx context.Context, id int) error {
	return s.db.DeployTarget.DeleteOneID(id).Exec(ctx)
}

// Get 获取部署目标
func (s *DeployService) Get(ctx context.Context, id int) (*ent.DeployTarget, error) {
	return s.db.DeployTarget.Query().
		Where(deploytarget.ID(id)).
		WithDNSProvider().
		WithDeployCredential().
		WithCertificates().
		Only(ctx)
}

// List 获取所有部署目标（带凭证来源信息）
func (s *DeployService) List(ctx context.Context) ([]*ent.DeployTarget, error) {
	return s.db.DeployTarget.Query().
		WithDNSProvider().
		WithDeployCredential().
		WithCertificates().
		Order(ent.Desc("created_at")).
		All(ctx)
}

// LinkCert 关联证书到部署目标
func (s *DeployService) LinkCert(ctx context.Context, targetID, certID int) error {
	_, err := s.db.DeployTarget.UpdateOneID(targetID).AddCertificateIDs(certID).Save(ctx)
	return err
}

// UnlinkCert 取消证书与部署目标的关联
func (s *DeployService) UnlinkCert(ctx context.Context, targetID, certID int) error {
	_, err := s.db.DeployTarget.UpdateOneID(targetID).RemoveCertificateIDs(certID).Save(ctx)
	return err
}

// FetchDomainsInput 拉取 CDN 域名输入（新建目标时凭证尚未落库，需内联传入）
type FetchDomainsInput struct {
	ProviderType       string
	DeployService      string
	CredentialSource   string
	DNSProviderID      *int
	DeployCredentialID *int
	Region             string
	Config             map[string]string
}

// FetchCDNDomains 根据内联传入的凭证拉取 CDN 域名列表（用于新建目标时选择）
func (s *DeployService) FetchCDNDomains(ctx context.Context, in FetchDomainsInput) ([]string, error) {
	var creds Credentials
	switch deploytarget.CredentialSource(in.CredentialSource) {
	case deploytarget.CredentialSourceDeployCredential:
		if in.DeployCredentialID == nil || *in.DeployCredentialID == 0 {
			return nil, fmt.Errorf("%s", i18n.T("error.deploy_no_credential"))
		}
		cred, err := s.db.DeployCredential.Get(ctx, *in.DeployCredentialID)
		if err != nil {
			return nil, fmt.Errorf("%s", i18n.T("error.deploy_no_credential"))
		}
		creds, err = deploycredential.Parse(in.ProviderType, cred.Config)
		if err != nil {
			return nil, err
		}
	case deploytarget.CredentialSourceDNSProvider:
		if in.DNSProviderID == nil || *in.DNSProviderID == 0 {
			return nil, fmt.Errorf("%s", i18n.T("error.deploy_no_dns_provider"))
		}
		p, err := s.db.DNSProvider.Get(ctx, *in.DNSProviderID)
		if err != nil {
			return nil, fmt.Errorf("%s", i18n.T("error.deploy_no_dns_provider"))
		}
		creds, err = dnsprovider.ParseCredential(in.ProviderType, p.Config)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("%s", i18n.T("error.deploy_credential_missing"))
	}
	if creds.AccessKeyID == "" || (creds.AccessKeySecret == "" && !isPanelProvider(in.ProviderType)) {
		return nil, fmt.Errorf("%s", i18n.T("error.deploy_credential_missing"))
	}
	region := in.Region
	if region == "" {
		region = creds.Region
	}
	logging.Debug(i18n.T("log.deploy.fetch_domains_start", "Provider", in.ProviderType, "Service", in.DeployService, "Region", region))
	d := registry[in.ProviderType]
	if d == nil {
		return nil, fmt.Errorf("%s", i18n.T("error.deploy_unsupported_provider", "Provider", in.ProviderType))
	}
	// 站点级服务在已选站点时按站点拉域名：EdgeOne 用 zone_id，ESA 用 site_id（统一走 zoneID 参数）。
	zoneID := in.Config["zone_id"]
	if zoneID == "" {
		zoneID = in.Config["site_id"]
	}
	return callListSites(ctx, d, creds, in.DeployService, region, zoneID)
}

// ListCDNDomains 拉取指定部署目标已配置凭证下的 CDN 域名列表
func (s *DeployService) ListCDNDomains(ctx context.Context, targetID int) ([]string, error) {
	logging.Debug(i18n.T("log.deploy.fetch_domains_target_start", "TargetID", targetID))
	target, err := s.db.DeployTarget.Query().
		Where(deploytarget.ID(targetID)).
		WithDNSProvider().
		WithDeployCredential().
		Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s", i18n.T("error.deploy_target_not_found"))
	}
	creds, _, err := s.loadCredsAndConfig(target)
	if err != nil {
		return nil, err
	}
	region := creds.Region
	d := registry[target.ProviderType.String()]
	if d == nil {
		return nil, fmt.Errorf("%s", i18n.T("error.deploy_unsupported_provider", "Provider", target.ProviderType.String()))
	}
	// 站点级服务按已配置站点拉取域名：EdgeOne 用 zone_id，ESA 用 site_id；其余服务忽略。
	cfg := parseConfig(target.Config)
	zoneID := cfg["zone_id"]
	if zoneID == "" {
		zoneID = cfg["site_id"]
	}
	return callListSites(ctx, d, creds, target.DeployService, region, zoneID)
}

// siteLister 是可选接口：面板类部署器实现 ListSites 以返回网站列表。
type siteLister interface {
	ListSites(ctx context.Context, creds Credentials, svc, region, zoneID string) ([]string, error)
}

// callListSites 优先调用部署器的 ListSites（面板类），否则回退到 ListDomains（云厂商）。
func callListSites(ctx context.Context, d Deployer, creds Credentials, svc, region, zoneID string) ([]string, error) {
	if sl, ok := d.(siteLister); ok {
		return sl.ListSites(ctx, creds, svc, region, zoneID)
	}
	return d.ListDomains(ctx, creds, svc, region, zoneID)
}

// ListByCert 获取关联了某证书的部署目标
func (s *DeployService) ListByCert(ctx context.Context, certID int) ([]*ent.DeployTarget, error) {
	return s.db.DeployTarget.Query().
		Where(deploytarget.HasCertificatesWith(certificate.ID(certID))).
		WithDNSProvider().
		WithDeployCredential().
		All(ctx)
}

// loadCredsAndConfig 按 credential_source + 厂商类型分发到三条独立解析路径（不再写在一起）：
//   - loadDNSProviderPath：DNS 提供商凭证 + 云/CDN 域名配置（DomainTargetConfig）
//   - loadDeployCredSitePath：部署凭证 + 面板/防火墙站点配置（SiteTargetConfig）
//   - loadDeployCredDomainPath：部署凭证 + 云厂商 CDN 域名配置（DomainTargetConfig）
func (s *DeployService) loadCredsAndConfig(target *ent.DeployTarget) (Credentials, map[string]string, error) {
	switch target.CredentialSource {
	case deploytarget.CredentialSourceDNSProvider:
		return s.loadDNSProviderPath(target)
	case deploytarget.CredentialSourceDeployCredential:
		if isPanelProvider(target.ProviderType.String()) {
			return s.loadDeployCredSitePath(target)
		}
		return s.loadDeployCredDomainPath(target)
	default:
		return Credentials{}, nil, fmt.Errorf("%s", i18n.T("error.deploy_credential_missing"))
	}
}

// 路径一：DNS 提供商凭证（复用 DNS 厂商凭证）+ 云/CDN 域名配置。
func (s *DeployService) loadDNSProviderPath(target *ent.DeployTarget) (Credentials, map[string]string, error) {
	if target.Edges.DNSProvider == nil {
		return Credentials{}, nil, fmt.Errorf("%s", i18n.T("error.deploy_no_dns_provider"))
	}
	creds, err := dnsprovider.ParseCredential(target.ProviderType.String(), target.Edges.DNSProvider.Config)
	if err != nil {
		return Credentials{}, nil, err
	}
	if creds.AccessKeyID == "" || creds.AccessKeySecret == "" {
		return Credentials{}, nil, fmt.Errorf("%s", i18n.T("error.deploy_credential_missing"))
	}
	dtc := config.MustParseConfig[DomainTargetConfig](target.Config)
	svc := config.AsMap(dtc)
	return creds, svc, nil
}

// 路径二：部署凭证 + 面板/防火墙站点配置（SiteTargetConfig）。
// 面板/防火墙类仅需 AccessKeyID（如 API Key），无 Secret 亦可。
func (s *DeployService) loadDeployCredSitePath(target *ent.DeployTarget) (Credentials, map[string]string, error) {
	if target.Edges.DeployCredential == nil {
		return Credentials{}, nil, fmt.Errorf("%s", i18n.T("error.deploy_no_credential"))
	}
	creds, err := deploycredential.Parse(target.ProviderType.String(), target.Edges.DeployCredential.Config)
	if err != nil {
		return Credentials{}, nil, err
	}
	if creds.AccessKeyID == "" {
		return Credentials{}, nil, fmt.Errorf("%s", i18n.T("error.deploy_credential_missing"))
	}
	dtc := config.MustParseConfig[SiteTargetConfig](target.Config)
	svc := config.AsMap(dtc)
	// 面板/防火墙类：panel_url 随凭证存储，注入到 svc 供部署器消费
	if creds.PanelURL != "" {
		svc["panel_url"] = creds.PanelURL
	}
	return creds, svc, nil
}

// 路径三：部署凭证 + 云厂商 CDN 域名配置（DomainTargetConfig）。
func (s *DeployService) loadDeployCredDomainPath(target *ent.DeployTarget) (Credentials, map[string]string, error) {
	if target.Edges.DeployCredential == nil {
		return Credentials{}, nil, fmt.Errorf("%s", i18n.T("error.deploy_no_credential"))
	}
	creds, err := deploycredential.Parse(target.ProviderType.String(), target.Edges.DeployCredential.Config)
	if err != nil {
		return Credentials{}, nil, err
	}
	if creds.AccessKeyID == "" || creds.AccessKeySecret == "" {
		return Credentials{}, nil, fmt.Errorf("%s", i18n.T("error.deploy_credential_missing"))
	}
	dtc := config.MustParseConfig[DomainTargetConfig](target.Config)
	svc := config.AsMap(dtc)
	return creds, svc, nil
}

// matchSiteID 兜底配对：未显式传入 siteID 时，按站点名从配置的 site_name/site_id 两组
// 按索引对齐的数组中查出对应的站点 ID。站点名与 ID 在配置里本就是分开存储的两份数组，
// 这里仅在「前端未单独传 ID」的回退场景下使用，主路径由 DeployCertificate 直接落盘 siteID。
func matchSiteID(namesRaw, idsRaw, name string) string {
	if name == "" {
		return ""
	}
	var names, ids []string
	_ = json.Unmarshal([]byte(namesRaw), &names)
	_ = json.Unmarshal([]byte(idsRaw), &ids)
	for i, n := range names {
		if n == name && i < len(ids) {
			return ids[i]
		}
	}
	return ""
}

// DeployCertificate 将指定证书部署到指定目标。
// domain 为云厂商的 CDN 域名或面板/防火墙类的站点名；siteID 为面板/防火墙类的站点 ID（云厂商忽略）。
func (s *DeployService) DeployCertificate(ctx context.Context, targetID, certID int, domain, siteID string) (*DeployOutcome, error) {
	target, err := s.db.DeployTarget.Query().
		Where(deploytarget.ID(targetID)).
		WithDNSProvider().
		WithDeployCredential().
		Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s", i18n.T("error.deploy_target_not_found"))
	}
	// 面板/防火墙类按「站点/网站」部署，云厂商按「域名」部署，日志与通知需区分语义。
	isSite := isPanelProvider(target.ProviderType.String())
	if isSite {
		logging.Debug(i18n.T("log.deploy.cert_deploy_start_site", "TargetID", targetID, "CertID", certID, "Site", domain, "Provider", target.ProviderType.String(), "Service", target.DeployService))
	} else {
		logging.Debug(i18n.T("log.deploy.cert_deploy_start", "TargetID", targetID, "CertID", certID, "Domain", domain, "Provider", target.ProviderType.String(), "Service", target.DeployService))
	}
	cert, err := s.db.Certificate.Get(ctx, certID)
	if err != nil {
		return nil, fmt.Errorf("%s", i18n.T("error.cert_not_found"))
	}
	creds, svc, err := s.loadCredsAndConfig(target)
	if err != nil {
		return s.recordFailure(ctx, target, cert, domain, err, "")
	}
	// 面板/防火墙类按「站点」部署，云厂商按「域名」部署。
	// 站点名（site_name）与站点 ID（site_id）在配置里是两份独立数组，部署时各自使用：
	//   - 前端下拉选项值为「名称||ID」，部署调用会把站点名（domain）与站点 ID（siteID）分开传入；
	//   - 1Panel 部署器直接用 svc["site_id"] 定位站点；aaPanel/宝塔直接用 svc["site_name"] 做站点名。
	// 因此此处只做字段落盘，不再拼接/解析「名称||ID」。
	if isSite {
		if siteID != "" {
			svc["site_name"] = domain
			svc["site_id"] = siteID
		} else {
			// 兜底：未传入 siteID（如回退到配置里的站点名）时，按站点名从配置的 site_id 数组配对。
			if id := matchSiteID(svc["site_name"], svc["site_id"], domain); id != "" {
				svc["site_name"] = domain
				svc["site_id"] = id
			}
		}
	} else if domain != "" {
		svc["domain"] = domain
	}
	// 实际部署的资源名称：面板/防火墙类为站点名，云厂商为 CDN 域名。
	// 未单独选中时，站点类取配置里的 site_name，域名类取证书的域名。
	deployedName := domain
	if deployedName == "" {
		if isSite {
			deployedName = svc["site_name"]
		} else {
			deployedName = cert.Domain
		}
	}
	// 供部署器在绑定阶段计算证书名称使用（与上传阶段保持一致）
	svc["cert_domain"] = cert.Domain
	// 记录部署服务类型，供部署器区分 CDN / 全站加速（DRCDN）。
	svc["deploy_service"] = target.DeployService
	// 面板/防火墙类无独立证书库（UploadCert 为空操作），将证书内容注入 svc 供 DeployCert 直接写站点。
	if isPanelProvider(target.ProviderType.String()) {
		svc["cert_pem"] = cert.CertContent
		svc["key_pem"] = cert.KeyContent
	}
	d := registry[target.ProviderType.String()]
	if d == nil {
		return s.recordFailure(ctx, target, cert, deployedName,
			i18n.NewError("error.deploy_unsupported_provider", "Provider", target.ProviderType.String()), "")
	}

	// 上传去重：相同「厂商+账号+区域+证书内容」只真正上传一次，之后直接复用云端证书 ID。
	// 去重映射持久化到数据库（CertUpload），跨进程重启仍然生效，不依赖任何云厂商的原生去重能力。
	cloudCertID, uploadRaw, uerr := s.ensureUploaded(ctx, d, target.ProviderType.String(), creds, cert, svc)
	if uerr != nil {
		return s.recordFailure(ctx, target, cert, deployedName, uerr, uploadRaw)
	}

	res, err := d.DeployCert(ctx, creds, cloudCertID, target.DeployService, svc)
	if err != nil {
		rawResp := ""
		if res != nil {
			rawResp = res.RawResponse
		}
		return s.recordFailure(ctx, target, cert, deployedName, err, rawResp)
	}
	// 部署成功
	_, _ = s.db.DeployTarget.UpdateOneID(targetID).
		SetLastStatus("success").
		SetLastDeployedAt(time.Now()).
		SetLastError("").
		Save(ctx)
	_ = s.recordLog(ctx, target, cert, deployedName, target.ProviderType.String(), target.DeployService, true, res.Message, res.CloudCertID, res.RawResponse)
	if s.notifService != nil {
		if isSite {
			_ = s.notifService.SendDeploySuccessSite(deployedName, target.Name)
		} else {
			_ = s.notifService.SendDeploySuccess(deployedName, target.Name)
		}
	}
	if isSite {
		logging.Info(i18n.T("log.deploy_success_site", "Site", deployedName, "Target", target.Name))
	} else {
		logging.Info(i18n.T("log.deploy_success", "Domain", deployedName, "Target", target.Name))
	}
	return &DeployOutcome{TargetID: targetID, TargetName: target.Name, CloudCertID: res.CloudCertID, Success: true, Message: res.Message, RawResponse: res.RawResponse}, nil
}

// certValidator 可选的证书存在性校验接口：供 ensureUploaded 在复用数据库记录前确认云端证书仍存在，
// 避免复用旧版本遗留的脏数据（非真实 certId）或已被手动删除的证书。未实现该接口的厂商默认信任
// 数据库记录（其 certId 由云端返回、本就真实）。
type certValidator interface {
	ValidateCert(creds Credentials, certID string) (bool, error)
}

// ensureUploaded 确保证书已上传到云端：相同「厂商+账号+区域+证书内容」只真正上传一次，
// 之后直接复用云端证书 ID。去重映射同时保存在内存（快速）与数据库（跨进程持久化）两层，
// 因此不管是否同一进程、是否重启，都不会重复上传，也不依赖各云厂商的原生去重行为。
func (s *DeployService) ensureUploaded(ctx context.Context, d Deployer, provider string, creds Credentials, cert *ent.Certificate, svc map[string]string) (string, string, error) {
	content := cert.CertContent + "\n" + cert.KeyContent
	fp := sha256.Sum256([]byte(content))
	fpHex := hex.EncodeToString(fp[:])
	// 加入部署服务类型，使 CDN 与全站加速（DRCDN）的上传去重相互独立：
	// 两者证书模型不同（CDN 暂存指纹、DRCDN 注册真实 certId），共享去重会导致 certId 错配。
	svcType := svc["deploy_service"]
	cacheKey := provider + "|" + creds.AccessKeyID + "|" + creds.Region + "|" + svcType + "|" + fpHex

	// 1) 内存缓存（同进程内最快）
	if id, ok := s.cacheGet(cacheKey); ok {
		return id, "", nil
	}

	// 2) 数据库已持久化的映射（跨进程重启仍生效）
	if s.db != nil {
		if existing, err := s.db.CertUpload.Query().
			Where(
				certupload.Provider(provider),
				certupload.AccessKeyID(creds.AccessKeyID),
				certupload.Region(creds.Region),
				certupload.CertFingerprint(fpHex),
			).
			Only(ctx); err == nil && existing != nil && existing.CloudCertID != "" {
			// 复用前校验云端证书是否仍存在（自愈旧版本脏数据 / 被手动删除的证书）：
			// 仅实现 certValidator 的厂商会校验；未实现的厂商沿用原记录。
			if v, ok := d.(certValidator); ok {
				if valid, verr := v.ValidateCert(creds, existing.CloudCertID); verr == nil && !valid {
					logging.Debug(i18n.T("log.deploy.cert_reupload", "CertID", existing.CloudCertID))
					existing = nil
				}
			}
			if existing != nil {
				s.cachePut(cacheKey, existing.CloudCertID)
				logging.Debug(i18n.T("log.deploy.reuse_upload_record", "CertID", existing.CloudCertID, "Key", cacheKey))
				return existing.CloudCertID, "", nil
			}
		}
	}

	// 3) 真正上传
	cloudCertID, raw, uerr := d.UploadCert(ctx, creds,
		CertContent{Domain: cert.Domain, CertPEM: cert.CertContent, KeyPEM: cert.KeyContent}, svc)
	if uerr != nil {
		return "", raw, uerr
	}

	// 4) 持久化映射（唯一键冲突时更新云端 ID，兼容并发/历史脏数据）
	if s.db != nil {
		if derr := s.db.CertUpload.Create().
			SetProvider(provider).
			SetAccessKeyID(creds.AccessKeyID).
			SetRegion(creds.Region).
			SetCertFingerprint(fpHex).
			SetCloudCertID(cloudCertID).
			OnConflictColumns("provider", "access_key_id", "region", "cert_fingerprint").
			UpdateNewValues().
			Exec(ctx); derr != nil {
			logging.Warn(i18n.T("log.deploy.persist_upload_map_failed", "Err", derr.Error()))
		}
	}
	s.cachePut(cacheKey, cloudCertID)
	return cloudCertID, raw, nil
}

func (s *DeployService) cacheGet(key string) (string, bool) {
	s.uploadMu.Lock()
	defer s.uploadMu.Unlock()
	if s.uploadCache == nil {
		return "", false
	}
	id, ok := s.uploadCache[key]
	return id, ok
}

func (s *DeployService) cachePut(key, id string) {
	s.uploadMu.Lock()
	defer s.uploadMu.Unlock()
	if s.uploadCache == nil {
		s.uploadCache = make(map[string]string)
	}
	s.uploadCache[key] = id
}

// recordLog 写入一条部署历史记录（失败也不阻断主流程）
func (s *DeployService) recordLog(ctx context.Context, target *ent.DeployTarget, cert *ent.Certificate, domain, providerType, deployService string, success bool, message, cloudCertID, rawResponse string) error {
	if domain == "" {
		domain = cert.Domain
	}
	if _, err := s.db.DeployLog.Create().
		SetDeployTarget(target).
		SetCertID(cert.ID).
		SetCertDomain(cert.Domain).
		SetDeployDomain(domain).
		SetTargetName(target.Name).
		SetProviderType(providerType).
		SetDeployService(deployService).
		SetSuccess(success).
		SetMessage(message).
		SetResponse(rawResponse).
		SetCloudCertID(cloudCertID).
		Save(ctx); err != nil {
		logging.Error("%s", i18n.T("log.deploy.history_write_failed", "Err", err.Error()))
		return err
	}
	return nil
}

// recordFailure 记录部署失败并发送失败通知。
// deployErr 为原始错误（可能是携带 i18n key 的 *i18n.Error）；落库/通知使用其不含信封的纯翻译文本，
// 返回给前端的仍是原错误（含信封），便于前端按自身语言重新翻译，实现前后端解耦。
func (s *DeployService) recordFailure(ctx context.Context, target *ent.DeployTarget, cert *ent.Certificate, domain string, deployErr error, rawResponse string) (*DeployOutcome, error) {
	msg := i18n.TranslateError(deployErr)
	_, _ = s.db.DeployTarget.UpdateOneID(target.ID).
		SetLastStatus("failed").
		SetLastDeployedAt(time.Now()).
		SetLastError(msg).
		Save(ctx)
	_ = s.recordLog(ctx, target, cert, domain, target.ProviderType.String(), target.DeployService, false, msg, "", rawResponse)
	// 通知按站点/域名区分语义；资源名为空时回退到证书域名。
	notifyName := domain
	if notifyName == "" {
		notifyName = cert.Domain
	}
	if s.notifService != nil {
		if isPanelProvider(target.ProviderType.String()) {
			_ = s.notifService.SendDeployFailedSite(notifyName, target.Name, msg)
		} else {
			_ = s.notifService.SendDeployFailed(notifyName, target.Name, msg)
		}
	}
	logging.Error("%s", i18n.T("log.deploy_failed", "Target", target.Name, "Error", msg))
	return &DeployOutcome{TargetID: target.ID, TargetName: target.Name, Success: false, Message: msg}, deployErr
}

// ListDeployLogs 获取某部署目标的历史记录（按时间倒序）
func (s *DeployService) ListDeployLogs(ctx context.Context, targetID int) ([]*ent.DeployLog, error) {
	return s.db.DeployLog.Query().
		Where(deploylog.HasDeployTargetWith(deploytarget.ID(targetID))).
		Order(ent.Desc("created_at")).
		All(ctx)
}

// DeployAllForCert 将该证书部署到所有关联且启用的目标。
// 面板/防火墙类目标按配置里的每个站点循环部署（各自带 site_id），云厂商目标按配置域名部署。
func (s *DeployService) DeployAllForCert(ctx context.Context, certID int) ([]*DeployOutcome, error) {
	targets, err := s.ListByCert(ctx, certID)
	if err != nil {
		return nil, err
	}
	var outcomes []*DeployOutcome
	for _, t := range targets {
		if !t.IsActive {
			continue
		}
		if isPanelProvider(t.ProviderType.String()) {
			cfg := parseConfig(t.Config)
			names := parseConfigStringSlice(cfg["site_name"])
			ids := parseConfigStringSlice(cfg["site_id"])
			if len(names) == 0 {
				outcome, _ := s.DeployCertificate(ctx, t.ID, certID, "", "")
				if outcome != nil {
					outcomes = append(outcomes, outcome)
				}
				continue
			}
			for i, name := range names {
				id := ""
				if i < len(ids) {
					id = ids[i]
				}
				outcome, _ := s.DeployCertificate(ctx, t.ID, certID, name, id)
				if outcome != nil {
					outcomes = append(outcomes, outcome)
				}
			}
		} else {
			outcome, _ := s.DeployCertificate(ctx, t.ID, certID, "", "")
			if outcome != nil {
				outcomes = append(outcomes, outcome)
			}
		}
	}
	return outcomes, nil
}

// currentCertGetter 已在 types.go 定义；以下为资源列表构造与批量查询。

// deployResource 单个可查询当前证书的资源（站点或域名）。
type deployResource struct {
	key       string // 与前端 deployRows 的行 key 对齐（面板：id||name；云：domain）
	name      string
	siteID    string
	svcConfig map[string]string
}

// buildCurrentCertResources 根据目标配置构造「需要查询当前证书」的资源列表，
// 每个资源的 svcConfig 与 DeployCertificate 部署时一致（面板带 site_name/site_id，云带 domain）。
// 资源 key 必须与前端 deployRows 行 key 完全一致，否则无法回显到对应行。
func (s *DeployService) buildCurrentCertResources(target *ent.DeployTarget, svc map[string]string) []deployResource {
	if isPanelProvider(target.ProviderType.String()) {
		names := parseConfigStringSlice(svc["site_name"])
		ids := parseConfigStringSlice(svc["site_id"])
		var res []deployResource
		for i, name := range names {
			id := ""
			if i < len(ids) {
				id = ids[i]
			}
			key := id
			if key == "" {
				key = name
			}
			rc := cloneStringMap(svc)
			rc["site_name"] = name
			rc["site_id"] = id
			res = append(res, deployResource{key: key, name: name, siteID: id, svcConfig: rc})
		}
		return res
	}
	domains := parseConfigStringSlice(svc["domains"])
	if len(domains) == 0 {
		domains = parseConfigStringSlice(svc["domain"])
	}
	var res []deployResource
	for _, d := range domains {
		rc := cloneStringMap(svc)
		rc["domain"] = d
		res = append(res, deployResource{key: d, name: d, svcConfig: rc})
	}
	return res
}

func cloneStringMap(m map[string]string) map[string]string {
	cp := make(map[string]string, len(m))
	maps.Copy(cp, m)
	return cp
}

// GetCurrentCerts 批量查询某部署目标下所有资源当前生效证书。
// 返回以资源 key 索引的结果；未实现 currentCertGetter 的部署器，资源标记 Supported=false。
func (s *DeployService) GetCurrentCerts(ctx context.Context, targetID int) (map[string]*CurrentCertResult, error) {
	target, err := s.db.DeployTarget.Query().
		Where(deploytarget.ID(targetID)).
		WithDNSProvider().
		WithDeployCredential().
		Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s", i18n.T("error.deploy_target_not_found"))
	}
	creds, svc, err := s.loadCredsAndConfig(target)
	if err != nil {
		return nil, err
	}
	d := registry[target.ProviderType.String()]
	if d == nil {
		return nil, fmt.Errorf("%s", i18n.T("error.deploy_unsupported_provider", "Provider", target.ProviderType.String()))
	}
	getter, ok := d.(currentCertGetter)
	resources := s.buildCurrentCertResources(target, svc)
	if batcher, ok := getter.(currentCertBatch); ok {
		batcher.BeforeCurrentCerts(ctx)
	}
	logging.Debug(i18n.T("log.deploy.get_current_certs_entry",
		"Provider", target.ProviderType.String(),
		"DeployService", target.DeployService,
		"Count", len(resources),
		"GetterOK", ok))
	results := make(map[string]*CurrentCertResult, len(resources))
	if !ok {
		for _, r := range resources {
			results[r.key] = &CurrentCertResult{Supported: false}
		}
		return results, nil
	}
	// 并发查询各资源的当前生效证书：按索引写入结果切片（避免 map 并发写竞争）。
	// 双重限流（参数见包级 const）：信号量限制并发度 + 全局限速器限制发起速率。
	sem := make(chan struct{}, currentCertMaxConcurrency)
	resSlice := make([]*CurrentCertResult, len(resources))
	var wg sync.WaitGroup
	for i := range resources {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			currentCertRateWait()
			r := resources[i]
			logging.Debug(i18n.T("log.deploy.get_current_certs_query",
				"Key", r.key,
				"Svc", target.DeployService))
			cc, cerr := getter.GetCurrentCert(ctx, creds, target.DeployService, r.svcConfig)
			res := &CurrentCertResult{Supported: true, CurrentCert: cc}
			if cerr != nil {
				res.Error = i18n.TranslateError(cerr)
			}
			resSlice[i] = res
			logging.Debug(i18n.T("log.deploy.get_current_certs_result",
				"Key", r.key,
				"Supported", res.Supported,
				"Err", res.Error))
		}(i)
	}
	wg.Wait()
	for i, r := range resources {
		results[r.key] = resSlice[i]
	}
	return results, nil
}

// parseConfigStringSlice 解析 JSON 数组字符串为字符串切片（容错非数组/空值）。
func parseConfigStringSlice(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var arr []string
	if err := json.Unmarshal([]byte(s), &arr); err == nil {
		return arr
	}
	// 兼容非 JSON 形式：单值或逗号分隔（如 domain 单数场景）
	parts := strings.Split(s, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
