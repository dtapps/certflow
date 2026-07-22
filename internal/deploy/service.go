package deploy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"cnb.cool/dtapp/certflow/ent"
	"cnb.cool/dtapp/certflow/ent/certificate"
	"cnb.cool/dtapp/certflow/ent/certupload"
	"cnb.cool/dtapp/certflow/ent/deploylog"
	"cnb.cool/dtapp/certflow/ent/deploytarget"
	"cnb.cool/dtapp/certflow/internal/i18n"
	"cnb.cool/dtapp/certflow/internal/logging"
	"cnb.cool/dtapp/certflow/internal/notification"
	"reflect"
)

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
		creds = credsFromConfig(in.ProviderType, string(deploytarget.CredentialSourceDeployCredential), parseConfig(cred.Config))
	case deploytarget.CredentialSourceDNSProvider:
		if in.DNSProviderID == nil || *in.DNSProviderID == 0 {
			return nil, fmt.Errorf("%s", i18n.T("error.deploy_no_dns_provider"))
		}
		p, err := s.db.DNSProvider.Get(ctx, *in.DNSProviderID)
		if err != nil {
			return nil, fmt.Errorf("%s", i18n.T("error.deploy_no_dns_provider"))
		}
		creds = credsFromConfig(in.ProviderType, string(deploytarget.CredentialSourceDNSProvider), parseConfig(p.Config))
	default:
		return nil, fmt.Errorf("%s", i18n.T("error.deploy_credential_missing"))
	}
	if creds.AccessKeyID == "" || creds.AccessKeySecret == "" {
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
	return d.ListDomains(ctx, creds, in.DeployService, region, zoneID)
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
	return d.ListDomains(ctx, creds, target.DeployService, region, zoneID)
}

// ListByCert 获取关联了某证书的部署目标
func (s *DeployService) ListByCert(ctx context.Context, certID int) ([]*ent.DeployTarget, error) {
	return s.db.DeployTarget.Query().
		Where(deploytarget.HasCertificatesWith(certificate.ID(certID))).
		WithDNSProvider().
		WithDeployCredential().
		All(ctx)
}

// credsFromConfig 根据厂商标识从配置 map 中提取凭证
// credKeySet 各厂商在两种凭证来源下 config 中的 AK/SK 字段名。
// 部署凭证（deploy_credential）沿用项目云厂商约定；DNS 提供商凭证（dns_provider）沿用 lego 各
// provider 的字段名。二者命名可能不同，例如 baiducloud 的 SK：部署凭证为 access_key_secret，
// 而 DNS 凭证为 secret_access_key；volcengine 的 AK/SK：部署凭证为 access_key_id/access_key_secret，
// 而 DNS 凭证为 access_key/secret_key。
// 部署时按 credential_source 选择对应字段名读取，避免复用 DNS 凭证作为部署凭证时读不到密钥
// （这正是之前「百度云 DNS 凭证与部署凭证 key 不一致」导致部署失败的根因）。
var credKeySet = map[string]struct {
	deployAK, deploySK string
	dnsAK, dnsSK       string
}{
	"aliyun":       {"access_key_id", "access_key_secret", "access_key_id", "access_key_secret"},
	"huawei":       {"access_key_id", "secret_access_key", "access_key_id", "secret_access_key"},
	"tencentcloud": {"secret_id", "secret_key", "secret_id", "secret_key"},
	"baiducloud":   {"access_key_id", "access_key_secret", "access_key_id", "secret_access_key"},
	"ctyun":        {"access_key_id", "access_key_secret", "access_key_id", "access_key_secret"},
	"volcengine":   {"access_key_id", "access_key_secret", "access_key", "secret_key"},
}

// credsFromConfig 根据厂商与凭证来源从配置 map 中提取凭证。
// credSource 为 deploytarget.CredentialSource 的字符串值（"deploy_credential" / "dns_provider"），
// 用于决定读取哪一套密钥字段名（见 credKeySet）。
func credsFromConfig(provider, credSource string, cfg map[string]string) Credentials {
	ks, ok := credKeySet[provider]
	if !ok {
		return Credentials{}
	}
	ak, sk := ks.deployAK, ks.deploySK
	if credSource == string(deploytarget.CredentialSourceDNSProvider) {
		ak, sk = ks.dnsAK, ks.dnsSK
	}
	return Credentials{
		AccessKeyID:     cfg[ak],
		AccessKeySecret: cfg[sk],
		Region:          RegionFromConfig(cfg),
	}
}

// credKeys 各厂商凭证在 config 中的字段名（用于自管凭证时剔除），两种来源都列出
var credKeys = map[string][]string{
	"aliyun":       {"access_key_id", "access_key_secret", "region_id"},
	"huawei":       {"access_key_id", "secret_access_key", "region"},
	"tencentcloud": {"secret_id", "secret_key", "region"},
	"baiducloud":   {"access_key_id", "access_key_secret", "secret_access_key"},
	"ctyun":        {"access_key_id", "access_key_secret"},
	"volcengine":   {"access_key_id", "access_key_secret", "access_key", "secret_key"},
}

// stripCreds 从 config 中剔除凭证字段，保留服务配置
func stripCreds(provider string, cfg map[string]string) map[string]string {
	out := map[string]string{}
	drop := map[string]bool{}
	for _, k := range credKeys[provider] {
		drop[k] = true
	}
	for k, v := range cfg {
		if !drop[k] {
			out[k] = v
		}
	}
	return out
}

// loadCredsAndConfig 解析部署目标对应的凭证与服务配置
func (s *DeployService) loadCredsAndConfig(target *ent.DeployTarget) (Credentials, map[string]string, error) {
	var creds Credentials
	switch target.CredentialSource {
	case deploytarget.CredentialSourceDeployCredential:
		// 从部署凭证表读取
		if target.Edges.DeployCredential == nil {
			return creds, nil, fmt.Errorf("%s", i18n.T("error.deploy_no_credential"))
		}
		credCfg := parseConfig(target.Edges.DeployCredential.Config)
		creds = credsFromConfig(target.ProviderType.String(), target.CredentialSource.String(), credCfg)
		svc := parseConfig(target.Config)
		if creds.AccessKeyID == "" || creds.AccessKeySecret == "" {
			return creds, nil, fmt.Errorf("%s", i18n.T("error.deploy_credential_missing"))
		}
		return creds, svc, nil
	case deploytarget.CredentialSourceDNSProvider:
		// 复用 DNS 提供商凭证
		if target.Edges.DNSProvider == nil {
			return creds, nil, fmt.Errorf("%s", i18n.T("error.deploy_no_dns_provider"))
		}
		dnsCfg := parseConfig(target.Edges.DNSProvider.Config)
		creds = credsFromConfig(target.ProviderType.String(), target.CredentialSource.String(), dnsCfg)
		svc := parseConfig(target.Config)
		if creds.AccessKeyID == "" || creds.AccessKeySecret == "" {
			return creds, nil, fmt.Errorf("%s", i18n.T("error.deploy_credential_missing"))
		}
		return creds, svc, nil
	default:
		return creds, nil, fmt.Errorf("%s", i18n.T("error.deploy_credential_missing"))
	}
}

// DeployCertificate 将指定证书部署到指定目标（domain 非空时覆盖配置中的 CDN 域名）
func (s *DeployService) DeployCertificate(ctx context.Context, targetID, certID int, domain string) (*DeployOutcome, error) {
	target, err := s.db.DeployTarget.Query().
		Where(deploytarget.ID(targetID)).
		WithDNSProvider().
		WithDeployCredential().
		Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s", i18n.T("error.deploy_target_not_found"))
	}
	logging.Debug(i18n.T("log.deploy.cert_deploy_start", "TargetID", targetID, "CertID", certID, "Domain", domain, "Provider", target.ProviderType.String(), "Service", target.DeployService))
	cert, err := s.db.Certificate.Get(ctx, certID)
	if err != nil {
		return nil, fmt.Errorf("%s", i18n.T("error.cert_not_found"))
	}
	creds, svc, err := s.loadCredsAndConfig(target)
	if err != nil {
		return s.recordFailure(ctx, target, cert, domain, err, "")
	}
	if domain != "" {
		svc["domain"] = domain
	}
	// 供部署器在绑定阶段计算证书名称使用（与上传阶段保持一致）
	svc["cert_domain"] = cert.Domain
	// 记录部署服务类型，供部署器区分 CDN / 全站加速（DRCDN）。
	svc["deploy_service"] = target.DeployService
	d := registry[target.ProviderType.String()]
	if d == nil {
		return s.recordFailure(ctx, target, cert, domain,
			i18n.NewError("error.deploy_unsupported_provider", "Provider", target.ProviderType.String()), "")
	}

	// 上传去重：相同「厂商+账号+区域+证书内容」只真正上传一次，之后直接复用云端证书 ID。
	// 去重映射持久化到数据库（CertUpload），跨进程重启仍然生效，不依赖任何云厂商的原生去重能力。
	cloudCertID, uploadRaw, uerr := s.ensureUploaded(ctx, d, target.ProviderType.String(), creds, cert, svc)
	if uerr != nil {
		return s.recordFailure(ctx, target, cert, domain, uerr, uploadRaw)
	}

	res, err := d.DeployCert(ctx, creds, cloudCertID, target.DeployService, svc)
	if err != nil {
		rawResp := ""
		if res != nil {
			rawResp = res.RawResponse
		}
		return s.recordFailure(ctx, target, cert, domain, err, rawResp)
	}
	// 部署成功
	_, _ = s.db.DeployTarget.UpdateOneID(targetID).
		SetLastStatus("success").
		SetLastDeployedAt(time.Now()).
		SetLastError("").
		Save(ctx)
	_ = s.recordLog(ctx, target, cert, domain, target.ProviderType.String(), target.DeployService, true, res.Message, res.CloudCertID, res.RawResponse)
	if s.notifService != nil {
		_ = s.notifService.SendDeploySuccess(cert.Domain, target.Name)
	}
	logging.Info(i18n.T("log.deploy_success", "Domain", cert.Domain, "Target", target.Name))
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
		logging.Error(i18n.T("log.deploy.history_write_failed", "Err", err.Error()))
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
	if s.notifService != nil {
		_ = s.notifService.SendDeployFailed(cert.Domain, target.Name, msg)
	}
	logging.Error(i18n.T("log.deploy_failed", "Target", target.Name, "Error", msg))
	return &DeployOutcome{TargetID: target.ID, TargetName: target.Name, Success: false, Message: msg}, deployErr
}

// ListDeployLogs 获取某部署目标的历史记录（按时间倒序）
func (s *DeployService) ListDeployLogs(ctx context.Context, targetID int) ([]*ent.DeployLog, error) {
	return s.db.DeployLog.Query().
		Where(deploylog.HasDeployTargetWith(deploytarget.ID(targetID))).
		Order(ent.Desc("created_at")).
		All(ctx)
}

// DeployAllForCert 将该证书部署到所有关联且启用的目标
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
		outcome, _ := s.DeployCertificate(ctx, t.ID, certID, "")
		if outcome != nil {
			outcomes = append(outcomes, outcome)
		}
	}
	return outcomes, nil
}
