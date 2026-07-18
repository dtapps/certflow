package certificate

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"cnb.cool/dtapp/certflow/ent"
	"cnb.cool/dtapp/certflow/ent/certificate"
	"cnb.cool/dtapp/certflow/internal/i18n"
	"cnb.cool/dtapp/certflow/internal/logging"
	"cnb.cool/dtapp/certflow/internal/network"
	"cnb.cool/dtapp/certflow/internal/notification"
	"cnb.cool/dtapp/certflow/internal/settings"
	"github.com/go-acme/lego/v5/acme"
	"github.com/go-acme/lego/v5/certcrypto"
	legocert "github.com/go-acme/lego/v5/certificate"
	"github.com/go-acme/lego/v5/challenge/dns01"
	lego "github.com/go-acme/lego/v5/lego"
	"github.com/go-acme/lego/v5/registration"
)

// pendingChallenge 存储待完成的手动 DNS 挑战状态
type pendingChallenge struct {
	client         *lego.Client
	user           *acmeUser
	manualProvider *ManualDNSProvider
	request        legocert.ObtainRequest
	caEntity       *ent.CA
	req            CertificateRequest
	certRecordID   int               // 预创建的数据库记录 ID
	resultChan     chan obtainResult // goroutine Obtain 的结果
}

// obtainResult 后台 Obtain 的结果
type obtainResult struct {
	cert *legocert.Resource
	err  error
}

// CertificateService 提供证书申请和管理功能
type CertificateService struct {
	db                *ent.Client
	certDir           string // 证书存储目录
	notifService      *notification.NotificationService
	settingsProvider  func() settings.Settings
	pendingChallenges sync.Map // domain -> *pendingChallenge
}

// NewCertificateService 创建新的证书服务
func NewCertificateService(client *ent.Client, certDir string) *CertificateService {
	return &CertificateService{
		db:      client,
		certDir: certDir,
	}
}

// SetNotificationService 设置通知服务
func (s *CertificateService) SetNotificationService(ns *notification.NotificationService) {
	s.notifService = ns
}

// SetSettingsProvider 设置设置提供者（用于获取网络配置）
func (s *CertificateService) SetSettingsProvider(fn func() settings.Settings) {
	s.settingsProvider = fn
}

// CertificateRequest 证书申请参数
type CertificateRequest struct {
	Domain        string   `json:"domain"`
	Sans          []string `json:"sans"`
	CAID          int      `json:"ca_id"`
	DNSProviderID *int     `json:"dns_provider_id,omitempty"`
	AutoRenew     bool     `json:"auto_renew"`
	RenewalDays   int      `json:"renewal_days"`
	KeyType       string   `json:"key_type,omitempty"` // RSA2048, RSA4096, EC256, EC384
}

// ApplyCertificateResult 证书申请结果
type ApplyCertificateResult struct {
	Success     bool   `json:"success"`
	CertContent string `json:"cert_content,omitempty"`
	KeyContent  string `json:"key_content,omitempty"`
	Issuer      string `json:"issuer,omitempty"`
	NotBefore   string `json:"not_before,omitempty"`
	NotAfter    string `json:"not_after,omitempty"`
	Error       string `json:"error,omitempty"`
}

// ApplyCertificate 申请证书
func (s *CertificateService) ApplyCertificate(ctx context.Context, req CertificateRequest) (*ApplyCertificateResult, error) {
	logging.Info(i18n.T("log.cert_apply_start", "Domain", req.Domain, "CAID", req.CAID, "Sans", req.Sans))

	// 获取 CA 配置
	caEntity, err := s.db.CA.Get(ctx, req.CAID)
	if err != nil {
		logging.Error(i18n.T("log.get_ca_config_failed", "CAID", req.CAID, "Error", err))
		return nil, fmt.Errorf("%s", i18n.T("error.ca_config_failed", "Error", err))
	}
	logging.Debug(i18n.T("log.ca_config_debug", "Name", caEntity.Name, "Dir", caEntity.DirectoryURL, "Email", caEntity.AccountEmail))

	// 校验 CA 邮箱已配置
	if caEntity.AccountEmail == "" {
		logging.Error(i18n.T("log.ca_email_not_configured", "Name", caEntity.Name))
		return nil, fmt.Errorf("%s", i18n.T("error.ca_email_required", "Name", caEntity.Name))
	}

	// 生成两个不同的私钥：一个用于 ACME 账户，一个用于证书
	accountKey, err := generateKeyByType(req.KeyType)
	if err != nil {
		logging.Error(i18n.T("log.generate_key_failed", "Error", err))
		return nil, fmt.Errorf("%s", i18n.T("error.generate_key_failed", "Error", err))
	}
	certKey, err := generateKeyByType(req.KeyType)
	if err != nil {
		logging.Error(i18n.T("log.generate_key_failed", "Error", err))
		return nil, fmt.Errorf("%s", i18n.T("error.generate_key_failed", "Error", err))
	}

	// 创建 ACME 用户
	user := &acmeUser{
		Email: caEntity.AccountEmail,
		Key:   accountKey,
	}

	// 创建 lego 配置
	config := lego.NewConfig(user)
	config.CADirURL = caEntity.DirectoryURL

	// 设置自定义 HTTP 客户端（自定义 DNS + 代理）
	if s.settingsProvider != nil {
		config.HTTPClient = network.BuildHTTPClient(s.settingsProvider())
		logging.Debug(i18n.T("log.custom_http_client_set"))
	}

	// 创建 lego 客户端
	client, err := lego.NewClient(config)
	if err != nil {
		logging.Error(i18n.T("log.create_acme_client_failed", "Error", err))
		return nil, fmt.Errorf("%s", i18n.T("error.create_acme_client_failed", "Error", err))
	}

	// 配置 DNS 传播检查的 nameserver
	if s.settingsProvider != nil {
		cfg := s.settingsProvider()
		var nameservers []string
		for _, dc := range cfg.DNSConfigs {
			if dc.Enabled && len(dc.Servers) > 0 {
				for _, server := range dc.Servers {
					nameservers = append(nameservers, net.JoinHostPort(server, "53"))
				}
			}
		}
		if len(nameservers) > 0 {
			dns01.SetDefaultClient(dns01.NewClient(&dns01.Options{RecursiveNameservers: nameservers}))
			logging.Debug(i18n.T("log.dns_nameservers_configured", "Servers", nameservers))
		}
	}

	// 注册账户（如果尚未注册）
	var reg *acme.ExtendedAccount
	var regErr error
	if caEntity.EabKid != "" && caEntity.EabHmac != "" {
		// 配置了 EAB（如 LiteSSL / ZeroSSL 等强制要求的 CA）
		logging.Debug(i18n.T("log.acme_register_eab", "Kid", caEntity.EabKid))
		reg, regErr = client.Registration.RegisterWithExternalAccountBinding(ctx, registration.RegisterEABOptions{
			TermsOfServiceAgreed: true,
			Kid:                  caEntity.EabKid,
			HmacEncoded:          caEntity.EabHmac,
		})
	} else {
		reg, regErr = client.Registration.Register(ctx, registration.RegisterOptions{
			TermsOfServiceAgreed: true,
		})
	}
	if regErr != nil {
		logging.Warn(i18n.T("log.register_acme_account_warn", "Error", regErr))
		// 如果注册失败，尝试通过 key 解析账户
		reg, regErr = client.Registration.ResolveAccountByKey(ctx)
		if regErr != nil {
			logging.Error(i18n.T("log.register_acme_account_failed", "Error", regErr))
			return nil, fmt.Errorf("%s", i18n.T("error.register_acme_account_failed", "Error", regErr))
		}
	}
	logging.Debug(i18n.T("log.acme_account_registered"))
	user.Registration = reg

	// 设置 DNS 提供商
	var dnsProviderEntity *ent.DNSProvider
	if req.DNSProviderID != nil && *req.DNSProviderID > 0 {
		// 自动 DNS 模式 - 从数据库获取提供商配置
		dnsProvider, err := s.db.DNSProvider.Get(ctx, *req.DNSProviderID)
		if err != nil {
			logging.Error(i18n.T("log.dns_provider_not_found", "ID", *req.DNSProviderID, "Error", err))
			return nil, fmt.Errorf("%s", i18n.T("error.get_dns_failed", "Error", err))
		}
		logging.Debug(i18n.T("log.dns_provider_using", "Name", dnsProvider.Name))
		dnsProviderEntity = dnsProvider

		// 创建 lego DNS provider
		legoDNSProvider, err := createDNSProvider(dnsProvider)
		if err != nil {
			logging.Error(i18n.T("log.dns_provider_create_failed", "Error", err))
			return nil, fmt.Errorf("%s", i18n.T("error.dns_provider_create_failed", "Error", err))
		}
		client.Challenge.SetDNS01Provider(legoDNSProvider)
	} else {
		// 手动 DNS 模式 - 第一步：获取 TXT 记录信息
		logging.Info(i18n.T("log.manual_dns_mode"))
		challengeInfo, err := s.StartManualDNSChallenge(ctx, req)
		if err != nil {
			logging.Error(i18n.T("log.start_manual_dns_failed", "Error", err))
			return nil, fmt.Errorf("%s", i18n.T("error.start_manual_dns_failed", "Error", err))
		}
		// 返回 TXT 记录信息给前端，等待用户添加 DNS 记录
		logging.Info(i18n.T("log.manual_dns_challenge_created_simple", "Records", formatRecords(challengeInfo.Records)))
		return &ApplyCertificateResult{
			Success: false,
			Error:   i18n.T("error.add_dns_txt_records", "Records", formatRecords(challengeInfo.Records)),
		}, nil
	}

	// 申请证书（自动 DNS 模式）
	domains := append([]string{req.Domain}, req.Sans...)
	request := legocert.ObtainRequest{
		Domains:    domains,
		Bundle:     true,
		PrivateKey: certKey,
		KeyType:    parseKeyType(req.KeyType),
	}

	// 先创建数据库记录，状态为 pending
	createBuilder := s.db.Certificate.Create().
		SetDomain(req.Domain).
		SetSans(req.Sans).
		SetStatus("pending").
		SetAutoRenew(req.AutoRenew).
		SetRenewalDays(req.RenewalDays).
		SetKeyType(certificate.KeyType(req.KeyType)).
		SetCa(caEntity)
	if dnsProviderEntity != nil {
		createBuilder = createBuilder.SetDNSProvider(dnsProviderEntity)
	}
	certRecord, err := createBuilder.Save(ctx)
	if err != nil {
		logging.Error(i18n.T("log.cert_record_create_failed", "Error", err))
		return nil, fmt.Errorf("%s", i18n.T("error.create_cert_record_failed", "Error", err))
	}
	logging.Debug(i18n.T("log.cert_record_created", "ID", certRecord.ID, "Domain", req.Domain))

	// 执行证书申请
	logging.Info(i18n.T("log.start_acme_apply", "Domains", domains))
	certificates, err := client.Certificate.Obtain(ctx, request)
	if err != nil {
		errMsg := err.Error()
		logging.Error(i18n.T("log.cert_apply_obtain_failed", "Domain", req.Domain, "Error", err))
		// 更新数据库记录为失败
		_, _ = s.db.Certificate.UpdateOneID(certRecord.ID).
			SetStatus("failed").
			SetLastError(errMsg).
			Save(ctx)
		// 发送失败通知
		if s.notifService != nil {
			_ = s.notifService.SendCertApplyFailed(req.Domain, errMsg)
		}
		return nil, fmt.Errorf("%s", i18n.T("error.apply_cert_failed", "Error", err))
	}
	logging.Debug(i18n.T("log.acme_obtain_success"))

	// 解析证书信息
	x509Cert, err := parseCertificate(certificates.Certificate)
	if err != nil {
		errMsg := err.Error()
		logging.Error(i18n.T("log.cert_parse_failed", "Error", err))
		_, _ = s.db.Certificate.UpdateOneID(certRecord.ID).
			SetStatus("failed").
			SetLastError(errMsg).
			Save(ctx)
		if s.notifService != nil {
			_ = s.notifService.SendCertApplyFailed(req.Domain, errMsg)
		}
		return nil, fmt.Errorf("%s", i18n.T("error.parse_cert_failed", "Error", err))
	}
	logging.Debug(i18n.T("log.cert_parse_success", "Issuer", x509Cert.Issuer.CommonName, "NotAfter", x509Cert.NotAfter))

	// 生成证书内容
	certContent, keyContent := generateCertPEM(certificates)
	logging.Debug(i18n.T("log.cert_saved", "Domain", req.Domain))

	// 更新数据库记录为成功
	_, err = s.db.Certificate.UpdateOneID(certRecord.ID).
		SetCertContent(certContent).
		SetKeyContent(keyContent).
		SetIssuer(x509Cert.Issuer.CommonName).
		SetNotBefore(x509Cert.NotBefore).
		SetNotAfter(x509Cert.NotAfter).
		SetStatus("active").
		SetLastError("").
		Save(ctx)
	if err != nil {
		logging.Error(i18n.T("log.cert_record_update_failed_simple", "Error", err))
		return nil, fmt.Errorf("%s", i18n.T("error.update_cert_record_failed", "Error", err))
	}

	// 发送成功通知
	if s.notifService != nil {
		_ = s.notifService.SendCertApplied(req.Domain, x509Cert.Issuer.CommonName)
	}

	logging.Info(i18n.T("log.cert_apply_success_full", "Domain", req.Domain, "Issuer", x509Cert.Issuer.CommonName, "NotAfter", x509Cert.NotAfter))
	return &ApplyCertificateResult{
		Success:     true,
		CertContent: certContent,
		KeyContent:  keyContent,
		Issuer:      x509Cert.Issuer.CommonName,
		NotBefore:   x509Cert.NotBefore.Format(time.RFC3339),
		NotAfter:    x509Cert.NotAfter.Format(time.RFC3339),
	}, nil
}

// RenewCertificate 续期证书
func (s *CertificateService) RenewCertificate(ctx context.Context, certID int) (*ApplyCertificateResult, error) {
	logging.Info(i18n.T("log.cert_renew_start", "ID", certID))

	// 获取现有证书
	certEntity, err := s.db.Certificate.Get(ctx, certID)
	if err != nil {
		logging.Error(i18n.T("log.get_cert_record_failed", "ID", certID, "Error", err))
		return nil, fmt.Errorf("%s", i18n.T("error.get_cert_failed", "Error", err))
	}
	logging.Debug(i18n.T("log.cert_entity_info", "Domain", certEntity.Domain, "Sans", certEntity.Sans, "Status", certEntity.Status))

	// 获取关联的 CA（通过 edge 查询）
	caEntity, err := certEntity.QueryCa().Only(ctx)
	if err != nil {
		logging.Error(i18n.T("log.get_ca_config_failed", "CAID", certID, "Error", err))
		return nil, fmt.Errorf("%s", i18n.T("error.ca_config_failed", "Error", err))
	}

	// 生成两个不同的私钥：一个用于 ACME 账户，一个用于证书
	accountKey, err := generateKeyByType(certEntity.KeyType.String())
	if err != nil {
		logging.Error(i18n.T("log.generate_key_failed", "Error", err))
		return nil, fmt.Errorf("%s", i18n.T("error.generate_key_failed", "Error", err))
	}
	certKey, err := generateKeyByType(certEntity.KeyType.String())
	if err != nil {
		logging.Error(i18n.T("log.generate_key_failed", "Error", err))
		return nil, fmt.Errorf("%s", i18n.T("error.generate_key_failed", "Error", err))
	}

	// 创建 lego 配置
	config := lego.NewConfig(&acmeUser{
		Email: caEntity.AccountEmail,
		Key:   accountKey,
	})
	config.CADirURL = caEntity.DirectoryURL

	// 设置自定义 HTTP 客户端（自定义 DNS + 代理）
	if s.settingsProvider != nil {
		config.HTTPClient = network.BuildHTTPClient(s.settingsProvider())
	}

	client, err := lego.NewClient(config)
	if err != nil {
		logging.Error(i18n.T("log.create_acme_client_failed", "Error", err))
		return nil, fmt.Errorf("%s", i18n.T("error.create_acme_client_failed", "Error", err))
	}

	// 配置 DNS 传播检查的 nameserver
	if s.settingsProvider != nil {
		cfg := s.settingsProvider()
		var nameservers []string
		for _, dc := range cfg.DNSConfigs {
			if dc.Enabled && len(dc.Servers) > 0 {
				for _, server := range dc.Servers {
					nameservers = append(nameservers, net.JoinHostPort(server, "53"))
				}
			}
		}
		if len(nameservers) > 0 {
			dns01.SetDefaultClient(dns01.NewClient(&dns01.Options{RecursiveNameservers: nameservers}))
			logging.Debug(i18n.T("log.dns_nameservers_configured", "Servers", nameservers))
		}
	}

	// 注册/获取账户
	_, err = client.Registration.ResolveAccountByKey(ctx)
	if err != nil {
		logging.Warn(i18n.T("log.register_acme_account_warn", "Error", err))
		_, err = client.Registration.Register(ctx, registration.RegisterOptions{
			TermsOfServiceAgreed: true,
		})
		if err != nil {
			logging.Error(i18n.T("log.register_acme_account_error_simple", "Error", err))
			return nil, fmt.Errorf("%s", i18n.T("error.register_acme_account_failed", "Error", err))
		}
	}

	// 使用新私钥重新申请
	domains := append([]string{certEntity.Domain}, certEntity.Sans...)
	request := legocert.ObtainRequest{
		Domains:    domains,
		Bundle:     true,
		PrivateKey: certKey,
		KeyType:    parseKeyType(certEntity.KeyType.String()),
	}

	logging.Info(i18n.T("log.start_acme_renew", "Domains", domains))
	certificates, err := client.Certificate.Obtain(ctx, request)
	if err != nil {
		logging.Error(i18n.T("log.cert_renew_obtain_failed", "Domain", certEntity.Domain, "Error", err))
		return nil, fmt.Errorf("%s", i18n.T("error.renew_cert_failed", "Error", err))
	}

	// 生成证书内容
	certContent, keyContent := generateCertPEM(certificates)
	if err != nil {
		logging.Error(i18n.T("log.cert_save_failed_domain", "Domain", certEntity.Domain, "Error", err))
		return nil, fmt.Errorf("%s", i18n.T("error.save_cert_failed", "Error", err))
	}

	// 解析证书
	x509Cert, err := parseCertificate(certificates.Certificate)
	if err != nil {
		logging.Error(i18n.T("log.cert_parse_failed", "Error", err))
		return nil, fmt.Errorf("%s", i18n.T("error.parse_cert_failed", "Error", err))
	}

	// 更新数据库
	_, err = s.db.Certificate.UpdateOneID(certID).
		SetCertContent(certContent).
		SetKeyContent(keyContent).
		SetIssuer(x509Cert.Issuer.CommonName).
		SetNotBefore(x509Cert.NotBefore).
		SetNotAfter(x509Cert.NotAfter).
		SetStatus("active").
		SetLastError("").
		SetLastRenewedAt(time.Now()).
		Save(ctx)
	if err != nil {
		logging.Error(i18n.T("log.cert_record_update_failed_simple_id", "ID", certID, "Error", err))
		return nil, fmt.Errorf("%s", i18n.T("error.update_cert_record_failed", "Error", err))
	}

	logging.Info(i18n.T("log.cert_renew_success_full", "Domain", certEntity.Domain, "Issuer", x509Cert.Issuer.CommonName, "NotAfter", x509Cert.NotAfter))
	return &ApplyCertificateResult{
		Success:     true,
		CertContent: certContent,
		KeyContent:  keyContent,
		Issuer:      x509Cert.Issuer.CommonName,
		NotBefore:   x509Cert.NotBefore.Format(time.RFC3339),
		NotAfter:    x509Cert.NotAfter.Format(time.RFC3339),
	}, nil
}

// GetByID 根据 ID 获取证书
func (s *CertificateService) GetByID(ctx context.Context, id int) (*ent.Certificate, error) {
	result, err := s.db.Certificate.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%s", i18n.T("error.cert_not_found"))
		}
		return nil, fmt.Errorf("%s", i18n.T("error.get_cert_failed", "Error", err))
	}
	return result, nil
}

// List 获取所有证书
func (s *CertificateService) List(ctx context.Context) ([]*ent.Certificate, error) {
	results, err := s.db.Certificate.Query().All(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s", i18n.T("error.list_certs_failed", "Error", err))
	}
	return results, nil
}

// ListExpiring 获取即将过期的证书
func (s *CertificateService) ListExpiring(ctx context.Context, days int) ([]*ent.Certificate, error) {
	threshold := time.Now().AddDate(0, 0, days)
	results, err := s.db.Certificate.Query().
		Where(certificate.NotAfterLTE(threshold)).
		Where(certificate.StatusEQ("active")).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s", i18n.T("error.list_expiring_failed", "Error", err))
	}
	return results, nil
}

// ListAutoRenew 获取需要自动续期的证书
func (s *CertificateService) ListAutoRenew(ctx context.Context) ([]*ent.Certificate, error) {
	now := time.Now()
	results, err := s.db.Certificate.Query().
		Where(certificate.AutoRenewEQ(true)).
		Where(certificate.StatusEQ("active")).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s", i18n.T("error.list_auto_renew_failed", "Error", err))
	}

	// 筛选需要续期的证书
	var needRenewal []*ent.Certificate
	for _, cert := range results {
		if cert.NotAfter.Sub(now) <= time.Duration(cert.RenewalDays)*24*time.Hour {
			needRenewal = append(needRenewal, cert)
		}
	}
	return needRenewal, nil
}

// RevokeCertificate 撤销证书
func (s *CertificateService) RevokeCertificate(ctx context.Context, certID int) error {
	certEntity, err := s.db.Certificate.Get(ctx, certID)
	if err != nil {
		return fmt.Errorf("%s", i18n.T("error.get_cert_failed", "Error", err))
	}

	// 获取 CA（通过 edge 查询）
	caEntity, err := certEntity.QueryCa().Only(ctx)
	if err != nil {
		return fmt.Errorf("%s", i18n.T("error.ca_config_failed", "Error", err))
	}

	// 从数据库读取证书内容
	certPEM := []byte(certEntity.CertContent)
	if len(certPEM) == 0 {
		return fmt.Errorf("%s", i18n.T("error.cert_content_empty"))
	}

	// 创建 lego 客户端
	privateKey, err := loadPrivateKeyFromContent([]byte(certEntity.KeyContent))
	if err != nil {
		return fmt.Errorf("%s", i18n.T("error.load_private_key_failed", "Error", err))
	}

	config := lego.NewConfig(&acmeUser{
		Email: caEntity.AccountEmail,
		Key:   privateKey,
	})
	config.CADirURL = caEntity.DirectoryURL

	client, err := lego.NewClient(config)
	if err != nil {
		return fmt.Errorf("%s", i18n.T("error.create_acme_client_failed", "Error", err))
	}

	// 撤销证书
	err = client.Certificate.Revoke(ctx, certPEM)
	if err != nil {
		return fmt.Errorf("%s", i18n.T("error.revoke_cert_failed", "Error", err))
	}

	// 更新数据库状态
	_, err = s.db.Certificate.UpdateOneID(certID).
		SetStatus("revoked").
		Save(ctx)
	if err != nil {
		return fmt.Errorf("%s", i18n.T("error.update_cert_record_failed", "Error", err))
	}

	return nil
}

// UpdateSettings 更新证书设置（自动续期、续期天数）
func (s *CertificateService) UpdateSettings(ctx context.Context, certID int, autoRenew bool, renewalDays int) error {
	_, err := s.db.Certificate.UpdateOneID(certID).
		SetAutoRenew(autoRenew).
		SetRenewalDays(renewalDays).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("%s", i18n.T("error.cert_not_found"))
		}
		return fmt.Errorf("%s", i18n.T("error.update_cert_record_failed", "Error", err))
	}
	return nil
}

// Delete 删除证书记录
func (s *CertificateService) Delete(ctx context.Context, certID int) error {
	_, err := s.db.Certificate.Get(ctx, certID)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("%s", i18n.T("error.cert_not_found"))
		}
		return fmt.Errorf("%s", i18n.T("error.get_cert_failed", "Error", err))
	}

	// 删除数据库记录
	err = s.db.Certificate.DeleteOneID(certID).Exec(ctx)
	if err != nil {
		return fmt.Errorf("%s", i18n.T("error.delete_cert_failed", "Error", err))
	}

	return nil
}

// generateCertPEM 生成证书 PEM 内容（含完整证书链）
func generateCertPEM(certs *legocert.Resource) (string, string) {
	// lego 的 Certificate 和 PrivateKey 已经是 PEM 编码的，直接使用
	// Bundle: true 时 Certificate 已包含完整证书链（叶子 + 中间证书）
	return strings.TrimSpace(string(certs.Certificate)), strings.TrimSpace(string(certs.PrivateKey))
}

// 解析证书
func parseCertificate(certBytes []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(certBytes)
	if block == nil {
		return nil, fmt.Errorf("%s", i18n.T("error.parse_pem_cert_failed"))
	}
	return x509.ParseCertificate(block.Bytes)
}

// 从内容加载私钥
func loadPrivateKeyFromContent(keyBytes []byte) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode(keyBytes)
	if block == nil {
		return nil, fmt.Errorf("%s", i18n.T("error.parse_pem_key_failed"))
	}
	return x509.ParseECPrivateKey(block.Bytes)
}

// acmeUser 实现 lego 的 registration.User 接口
type acmeUser struct {
	Email        string
	Key          crypto.Signer
	Registration *acme.ExtendedAccount
}

func (u *acmeUser) GetEmail() string {
	return u.Email
}

func (u *acmeUser) GetRegistration() *acme.ExtendedAccount {
	return u.Registration
}

func (u *acmeUser) GetPrivateKey() crypto.Signer {
	return u.Key
}

// formatRecords 格式化 TXT 记录为可读字符串
func formatRecords(records []TXTRecord) string {
	var parts []string
	for _, r := range records {
		parts = append(parts, fmt.Sprintf("%s = %s", r.Name, r.Value))
	}
	return strings.Join(parts, "\n")
}

// GetCertificateContent 获取证书内容
func (s *CertificateService) GetCertificateContent(ctx context.Context, certID int) (string, string, error) {
	cert, err := s.db.Certificate.Get(ctx, certID)
	if err != nil {
		return "", "", fmt.Errorf("%s", i18n.T("error.get_cert_failed", "Error", err))
	}
	return cert.CertContent, cert.KeyContent, nil
}

// GetCertificateInfo 获取证书详细信息
func (s *CertificateService) GetCertificateInfo(ctx context.Context, certID int) (map[string]any, error) {
	cert, err := s.db.Certificate.Get(ctx, certID)
	if err != nil {
		return nil, fmt.Errorf("%s", i18n.T("error.get_cert_failed", "Error", err))
	}

	return map[string]any{
		"id":              cert.ID,
		"domain":          cert.Domain,
		"sans":            cert.Sans,
		"cert_content":    cert.CertContent,
		"key_content":     cert.KeyContent,
		"issuer":          cert.Issuer,
		"not_before":      cert.NotBefore,
		"not_after":       cert.NotAfter,
		"status":          cert.Status,
		"auto_renew":      cert.AutoRenew,
		"renewal_days":    cert.RenewalDays,
		"last_error":      cert.LastError,
		"last_renewed_at": cert.LastRenewedAt,
		"created_at":      cert.CreatedAt,
		"updated_at":      cert.UpdatedAt,
	}, nil
}

// ExportCertificate 导出证书为 JSON
func (s *CertificateService) ExportCertificate(ctx context.Context, certID int) ([]byte, error) {
	logging.Info(i18n.T("log.export_cert_start", "ID", certID))
	info, err := s.GetCertificateInfo(ctx, certID)
	if err != nil {
		logging.Error(i18n.T("log.export_cert_failed", "ID", certID, "Error", err))
		return nil, err
	}
	logging.Debug(i18n.T("log.export_cert_success", "ID", certID))
	return json.MarshalIndent(info, "", "  ")
}

// parseKeyType 将字符串 KeyType 转换为 certcrypto.KeyType，默认 EC256
func parseKeyType(keyType string) certcrypto.KeyType {
	if keyType == "" {
		return certcrypto.EC256
	}
	kt, err := certcrypto.ToKeyType(keyType)
	if err != nil {
		return certcrypto.EC256
	}
	return kt
}

// generateKeyByType 根据 KeyType 字符串生成私钥，默认 EC256
func generateKeyByType(keyType string) (crypto.Signer, error) {
	return certcrypto.GeneratePrivateKey(parseKeyType(keyType))
}
