package certificate

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"cnb.cool/dtapp/certflow/ent/schema"
	"cnb.cool/dtapp/certflow/internal/i18n"
	"cnb.cool/dtapp/certflow/internal/logging"
	"cnb.cool/dtapp/certflow/internal/network"
	legocert "github.com/go-acme/lego/v5/certificate"
	"github.com/go-acme/lego/v5/challenge"
	"github.com/go-acme/lego/v5/challenge/dns01"
	lego "github.com/go-acme/lego/v5/lego"
	"github.com/go-acme/lego/v5/registration"
)

// ManualChallengeInfo 存储手动 DNS 验证信息
type ManualChallengeInfo struct {
	Domain  string
	Records []TXTRecord
}

// TXTRecord 单条 TXT 记录
type TXTRecord struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// ManualDNSProvider 自定义手动 DNS 提供商
// 将 TXT 记录信息存储在内存中，供前端获取，而不是打印到终端
type ManualDNSProvider struct {
	mu        sync.Mutex
	challenge *ManualChallengeInfo
}

// NewManualDNSProvider 创建新的手动 DNS 提供商
func NewManualDNSProvider() *ManualDNSProvider {
	return &ManualDNSProvider{}
}

// Present 存储 TXT 记录信息（不打印到终端，不等待 Enter）
func (p *ManualDNSProvider) Present(ctx context.Context, domain, token, keyAuth string) error {
	// 计算 TXT 记录名称
	info := dns01.GetChallengeInfo(ctx, domain, keyAuth)
	fqdn, value := info.FQDN, info.Value

	p.mu.Lock()
	defer p.mu.Unlock()

	logging.Debug(i18n.T("log.manual_dns_present_call", "Domain", domain, "FQDN", fqdn, "Value", value))

	// 如果已有挑战信息，追加记录（通配符证书会为多个域名分别调用 Present）
	if p.challenge != nil {
		logging.Debug(i18n.T("log.manual_dns_append_record", "Count", len(p.challenge.Records), "NewFQDN", fqdn))
		p.challenge.Records = append(p.challenge.Records, TXTRecord{Name: fqdn, Value: value})
		return nil
	}

	// 新建挑战信息
	logging.Debug(i18n.T("log.manual_dns_new_challenge", "Domain", domain, "FQDN", fqdn))
	p.challenge = &ManualChallengeInfo{
		Domain:  domain,
		Records: []TXTRecord{{Name: fqdn, Value: value}},
	}
	return nil
}

// CleanUp 清理（手动模式无需清理）
func (p *ManualDNSProvider) CleanUp(ctx context.Context, domain, token, keyAuth string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.challenge != nil && p.challenge.Domain == domain {
		p.challenge = nil
	}
	return nil
}

// GetChallenge 获取当前挑战信息
func (p *ManualDNSProvider) GetChallenge() *ManualChallengeInfo {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.challenge == nil {
		return nil
	}
	// 返回副本
	records := make([]TXTRecord, len(p.challenge.Records))
	copy(records, p.challenge.Records)
	return &ManualChallengeInfo{
		Domain:  p.challenge.Domain,
		Records: records,
	}
}

// WaitForChallenge 等待挑战信息就绪（带超时）
func (p *ManualDNSProvider) WaitForChallenge(timeout time.Duration, expectedRecords int) (*ManualChallengeInfo, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if info := p.GetChallenge(); info != nil && len(info.Records) >= expectedRecords {
			logging.Debug(i18n.T("log.manual_dns_challenge_ready", "RecordsCount", len(info.Records), "Records", info.Records))
			return info, nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	// 超时后如果有记录也返回
	if info := p.GetChallenge(); info != nil && len(info.Records) > 0 {
		logging.Debug(i18n.T("log.manual_dns_challenge_ready", "RecordsCount", len(info.Records), "Records", info.Records))
		return info, nil
	}
	return nil, fmt.Errorf(i18n.T("error.wait_dns_challenge_timeout"))
}

// manualDNSProviderAdapter 适配 lego 的 dns01.ChallengeProvider 接口
type manualDNSProviderAdapter struct {
	provider *ManualDNSProvider
}

func (a *manualDNSProviderAdapter) Present(ctx context.Context, domain, token, keyAuth string) error {
	return a.provider.Present(ctx, domain, token, keyAuth)
}

func (a *manualDNSProviderAdapter) CleanUp(ctx context.Context, domain, token, keyAuth string) error {
	return a.provider.CleanUp(ctx, domain, token, keyAuth)
}

// GetDNSProvider 返回适配后的 DNS provider（用于 lego client）
func (p *ManualDNSProvider) GetDNSProvider() *manualDNSProviderAdapter {
	return &manualDNSProviderAdapter{provider: p}
}

// Ensure ManualDNSProvider implements challenge.Provider
var _ challenge.Provider = (*manualDNSProviderAdapter)(nil)

// StartManualDNSChallenge 开始手动 DNS 挑战（第一步）
// 返回 TXT 记录信息，用户需要添加此记录后调用 CompleteManualDNSChallenge
func (s *CertificateService) StartManualDNSChallenge(ctx context.Context, req CertificateRequest) (*ManualChallengeInfo, error) {
	logging.Info(i18n.T("log.manual_dns_challenge_start", "Domain", req.Domain, "CAID", req.CAID))

	// 获取 CA 配置
	caEntity, err := s.db.CA.Get(ctx, req.CAID)
	if err != nil {
		logging.Error(i18n.T("log.get_ca_config_failed", "CAID", req.CAID, "Error", err))
		return nil, fmt.Errorf(i18n.T("error.ca_config_failed", "Error", err))
	}

	if caEntity.AccountEmail == "" {
		logging.Error(i18n.T("log.ca_email_not_configured", "Name", caEntity.Name))
		return nil, fmt.Errorf(i18n.T("error.ca_email_required", "Name", caEntity.Name))
	}

	// 生成两个不同的私钥：一个用于 ACME 账户，一个用于证书
	accountKey, err := generateKeyByType(req.KeyType)
	if err != nil {
		logging.Error(i18n.T("log.generate_key_failed", "Error", err))
		return nil, err
	}
	certKey, err := generateKeyByType(req.KeyType)
	if err != nil {
		logging.Error(i18n.T("log.generate_key_failed", "Error", err))
		return nil, err
	}

	// 创建 lego 配置
	user := &acmeUser{
		Email: caEntity.AccountEmail,
		Key:   accountKey,
	}
	config := lego.NewConfig(user)
	config.CADirURL = caEntity.DirectoryURL

	// 设置自定义 HTTP 客户端（自定义 DNS + 代理）
	if s.settingsProvider != nil {
		config.HTTPClient = network.BuildHTTPClient(s.settingsProvider())
	}

	client, err := lego.NewClient(config)
	if err != nil {
		logging.Error(i18n.T("log.create_acme_client_failed", "Error", err))
		return nil, fmt.Errorf(i18n.T("error.create_acme_client_failed", "Error", err))
	}

	// 注册账户
	reg, err := client.Registration.Register(ctx, registration.RegisterOptions{
		TermsOfServiceAgreed: true,
	})
	if err != nil {
		logging.Warn(i18n.T("log.register_acme_account_warn", "Error", err))
		reg, err = client.Registration.ResolveAccountByKey(ctx)
		if err != nil {
			logging.Error(i18n.T("log.register_acme_account_failed", "Error", err))
			return nil, fmt.Errorf(i18n.T("error.register_acme_account_failed", "Error", err))
		}
	}
	user.Registration = reg

	// 设置自定义手动 DNS 提供商
	manualProvider := NewManualDNSProvider()

	// 配置 DNS 传播检查的 nameserver
	var challengeOpts []dns01.ChallengeOption
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
	// 手动 DNS 模式下使用较长的传播超时（10分钟），给用户充足时间添加 DNS 记录
	challengeOpts = append(challengeOpts, dns01.PropagationWait(10*time.Minute, false))
	client.Challenge.SetDNS01Provider(manualProvider.GetDNSProvider(), challengeOpts...)

	// 创建订单（这会触发 Present 回调）
	domains := append([]string{req.Domain}, req.Sans...)
	request := legocert.ObtainRequest{
		Domains:    domains,
		Bundle:     true,
		PrivateKey: certKey,
		KeyType:    parseKeyType(req.KeyType),
	}

	// 创建结果通道，后台 Obtain 完成后发送结果
	resultChan := make(chan obtainResult, 1)

	// 在后台执行 Obtain，Present 回调会存储 TXT 记录，然后继续验证 DNS
	go func() {
		certRes, err := client.Certificate.Obtain(ctx, request)
		if err != nil {
			logging.Error(i18n.T("log.background_obtain_failed", "Domain", req.Domain, "Error", err))
			resultChan <- obtainResult{err: err}
		} else {
			resultChan <- obtainResult{cert: certRes}
		}
		close(resultChan)
	}()

	// 等待挑战信息就绪（等待所有域名的 Present 调用完成）
	logging.Debug(i18n.T("log.wait_dns_challenge"))
	expectedRecords := len(domains)
	info, err := manualProvider.WaitForChallenge(60*time.Second, expectedRecords)
	if err != nil {
		logging.Error(i18n.T("log.get_dns_challenge_failed", "Error", err))
		return nil, fmt.Errorf(i18n.T("error.get_dns_challenge_failed", "Error", err))
	}
	logging.Debug(i18n.T("log.dns_challenge_ready", "Records", info.Records))

	// 先创建数据库记录，状态为 pending，同时保存 TXT 挑战信息
	challengeRecords := make([]schema.TXTRecord, len(info.Records))
	for i, r := range info.Records {
		challengeRecords[i] = schema.TXTRecord{Name: r.Name, Value: r.Value}
	}
	certRecord, err := s.db.Certificate.Create().
		SetDomain(req.Domain).
		SetSans(req.Sans).
		SetStatus("pending").
		SetAutoRenew(req.AutoRenew).
		SetRenewalDays(req.RenewalDays).
		SetChallengeRecords(challengeRecords).
		SetCa(caEntity).
		Save(ctx)
	if err != nil {
		logging.Error(i18n.T("log.cert_record_create_failed", "Error", err))
		return nil, fmt.Errorf(i18n.T("error.create_cert_record_failed", "Error", err))
	}

	// 保存状态供后续完成
	s.pendingChallenges.Store(req.Domain, &pendingChallenge{
		client:         client,
		user:           user,
		manualProvider: manualProvider,
		request:        request,
		caEntity:       caEntity,
		req:            req,
		certRecordID:   certRecord.ID,
		resultChan:     resultChan,
	})

	logging.Info(i18n.T("log.manual_dns_challenge_created", "Domain", req.Domain, "ID", certRecord.ID, "Records", info.Records))
	return info, nil
}

// GetPendingChallengeInfo 获取待完成的手动 DNS 挑战信息（用于继续申请）
func (s *CertificateService) GetPendingChallengeInfo(ctx context.Context, certID int) (*ManualChallengeInfo, error) {
	cert, err := s.db.Certificate.Get(ctx, certID)
	if err != nil {
		return nil, fmt.Errorf(i18n.T("error.get_cert_failed", "Error", err))
	}

	if cert.Status != "pending" || len(cert.ChallengeRecords) == 0 {
		return nil, fmt.Errorf(i18n.T("error.no_pending_challenge"))
	}

	// 转换数据库中的 challenge_records 为 TXTRecord 列表
	var records []TXTRecord
	for _, r := range cert.ChallengeRecords {
		records = append(records, TXTRecord{Name: r.Name, Value: r.Value})
	}

	return &ManualChallengeInfo{
		Domain:  cert.Domain,
		Records: records,
	}, nil
}

// ResumeManualDNSChallenge 恢复手动 DNS 挑战（重新生成挑战信息，更新 DNS 记录后调用 CompleteManualDNSChallenge）
func (s *CertificateService) ResumeManualDNSChallenge(ctx context.Context, certID int) (*ManualChallengeInfo, error) {
	logging.Info(i18n.T("log.manual_dns_challenge_resume", "ID", certID))

	// 获取原记录
	cert, err := s.db.Certificate.Get(ctx, certID)
	if err != nil {
		logging.Error(i18n.T("log.get_cert_record_failed", "ID", certID, "Error", err))
		return nil, fmt.Errorf(i18n.T("error.get_cert_failed", "Error", err))
	}

	if cert.Status != "pending" {
		logging.Error(i18n.T("log.manual_dns_status_error", "ID", certID, "Status", cert.Status))
		return nil, fmt.Errorf(i18n.T("error.only_pending_can_resume"))
	}

	// 获取 CA 配置
	caEntity, err := cert.QueryCa().Only(ctx)
	if err != nil {
		logging.Error(i18n.T("log.get_ca_config_failed", "CAID", certID, "Error", err))
		return nil, fmt.Errorf(i18n.T("error.ca_config_failed", "Error", err))
	}

	// 构造原始请求
	req := CertificateRequest{
		Domain:      cert.Domain,
		Sans:        cert.Sans,
		CAID:        caEntity.ID,
		AutoRenew:   cert.AutoRenew,
		RenewalDays: cert.RenewalDays,
	}

	// 生成两个不同的私钥：一个用于 ACME 账户，一个用于证书
	accountKey, err := generateKeyByType(req.KeyType)
	if err != nil {
		logging.Error(i18n.T("log.generate_key_failed", "Error", err))
		return nil, err
	}
	certKey, err := generateKeyByType(req.KeyType)
	if err != nil {
		logging.Error(i18n.T("log.generate_key_failed", "Error", err))
		return nil, err
	}

	// 创建 lego 配置
	user := &acmeUser{
		Email: caEntity.AccountEmail,
		Key:   accountKey,
	}
	config := lego.NewConfig(user)
	config.CADirURL = caEntity.DirectoryURL

	// 设置自定义 HTTP 客户端
	if s.settingsProvider != nil {
		config.HTTPClient = network.BuildHTTPClient(s.settingsProvider())
	}

	client, err := lego.NewClient(config)
	if err != nil {
		logging.Error(i18n.T("log.create_acme_client_failed", "Error", err))
		return nil, fmt.Errorf(i18n.T("error.create_acme_client_failed", "Error", err))
	}

	// 注册账户
	reg, err := client.Registration.Register(ctx, registration.RegisterOptions{
		TermsOfServiceAgreed: true,
	})
	if err != nil {
		logging.Warn(i18n.T("log.register_acme_account_warn", "Error", err))
		reg, err = client.Registration.ResolveAccountByKey(ctx)
		if err != nil {
			logging.Error(i18n.T("log.register_acme_account_failed", "Error", err))
			return nil, fmt.Errorf(i18n.T("error.register_acme_account_failed", "Error", err))
		}
	}
	user.Registration = reg

	// 设置手动 DNS 提供商
	manualProvider := NewManualDNSProvider()

	// 配置 DNS 传播检查的 nameserver
	var challengeOpts []dns01.ChallengeOption
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
	// 手动 DNS 模式下使用较长的传播超时（10分钟），给用户充足时间添加 DNS 记录
	challengeOpts = append(challengeOpts, dns01.PropagationWait(10*time.Minute, false))
	client.Challenge.SetDNS01Provider(manualProvider.GetDNSProvider(), challengeOpts...)

	// 创建订单
	domains := append([]string{req.Domain}, req.Sans...)
	request := legocert.ObtainRequest{
		Domains:    domains,
		Bundle:     true,
		PrivateKey: certKey,
	}

	// 创建结果通道，后台 Obtain 完成后发送结果
	resultChan := make(chan obtainResult, 1)

	// 在后台执行 Obtain
	go func() {
		certRes, err := client.Certificate.Obtain(ctx, request)
		if err != nil {
			logging.Error(i18n.T("log.background_obtain_failed", "Domain", req.Domain, "Error", err))
			resultChan <- obtainResult{err: err}
		} else {
			resultChan <- obtainResult{cert: certRes}
		}
		close(resultChan)
	}()

	// 等待挑战信息就绪（等待所有域名的 Present 调用完成）
	logging.Debug(i18n.T("log.wait_dns_challenge"))
	expectedRecords := len(domains)
	info, err := manualProvider.WaitForChallenge(60*time.Second, expectedRecords)
	if err != nil {
		logging.Error(i18n.T("log.get_dns_challenge_failed", "Error", err))
		return nil, fmt.Errorf(i18n.T("error.get_dns_challenge_failed", "Error", err))
	}

	// 更新数据库记录中的挑战信息
	challengeRecords := make([]schema.TXTRecord, len(info.Records))
	for i, r := range info.Records {
		challengeRecords[i] = schema.TXTRecord{Name: r.Name, Value: r.Value}
	}
	_, err = s.db.Certificate.UpdateOneID(certID).
		SetChallengeRecords(challengeRecords).
		Save(ctx)
	if err != nil {
		logging.Error(i18n.T("log.update_challenge_info_failed", "ID", certID, "Error", err))
		return nil, fmt.Errorf(i18n.T("error.update_challenge_info_failed", "Error", err))
	}

	// 保存状态供后续完成
	s.pendingChallenges.Store(req.Domain, &pendingChallenge{
		client:         client,
		user:           user,
		manualProvider: manualProvider,
		request:        request,
		caEntity:       caEntity,
		req:            req,
		certRecordID:   certID,
		resultChan:     resultChan,
	})

	logging.Info(i18n.T("log.manual_dns_challenge_resumed", "Domain", req.Domain, "ID", certID, "Records", info.Records))
	return info, nil
}

// CompleteManualDNSChallenge 完成手动 DNS 挑战（第二步）
// 用户添加 TXT 记录后调用此方法继续验证
func (s *CertificateService) CompleteManualDNSChallenge(ctx context.Context, domain string) (*ApplyCertificateResult, error) {
	logging.Info(i18n.T("log.manual_dns_challenge_complete", "Domain", domain))

	// 获取待完成的挑战
	val, ok := s.pendingChallenges.Load(domain)
	if !ok {
		logging.Error(i18n.T("log.manual_dns_pending_not_found", "Domain", domain))
		return nil, fmt.Errorf(i18n.T("error.pending_challenge_not_found", "Domain", domain))
	}
	pc := val.(*pendingChallenge)

	// 等待后台 Obtain 完成（不要再调 Obtain，会生成新的 challenge token）
	logging.Debug(i18n.T("log.acme_obtain_verify", "Domain", domain))
	result := <-pc.resultChan
	if result.err != nil {
		errMsg := result.err.Error()
		logging.Error(i18n.T("log.manual_dns_challenge_failed", "Domain", domain, "Error", errMsg))
		_, _ = s.db.Certificate.UpdateOneID(pc.certRecordID).
			SetStatus("failed").
			SetLastError(errMsg).
			Save(ctx)
		s.pendingChallenges.Delete(domain)
		if s.notifService != nil {
			_ = s.notifService.SendCertApplyFailed(pc.req.Domain, errMsg)
		}
		return nil, fmt.Errorf(i18n.T("error.apply_cert_failed", "Error", result.err))
	}
	certificates := result.cert
	logging.Debug(i18n.T("log.acme_obtain_success"))

	// 解析证书信息
	x509Cert, err := parseCertificate(certificates.Certificate)
	if err != nil {
		errMsg := err.Error()
		logging.Error(i18n.T("log.cert_parse_failed", "Error", err))
		_, _ = s.db.Certificate.UpdateOneID(pc.certRecordID).
			SetStatus("failed").
			SetLastError(errMsg).
			Save(ctx)
		s.pendingChallenges.Delete(domain)
		if s.notifService != nil {
			_ = s.notifService.SendCertApplyFailed(pc.req.Domain, errMsg)
		}
		return nil, fmt.Errorf(i18n.T("error.parse_cert_failed", "Error", err))
	}

	// 生成证书内容
	certContent, keyContent := generateCertPEM(certificates)
	if err != nil {
		errMsg := err.Error()
		logging.Error(i18n.T("log.cert_save_failed", "Error", err))
		_, _ = s.db.Certificate.UpdateOneID(pc.certRecordID).
			SetStatus("failed").
			SetLastError(errMsg).
			Save(ctx)
		s.pendingChallenges.Delete(domain)
		if s.notifService != nil {
			_ = s.notifService.SendCertApplyFailed(pc.req.Domain, errMsg)
		}
		return nil, fmt.Errorf(i18n.T("error.save_cert_failed", "Error", err))
	}

	// 更新数据库记录为成功
	_, err = s.db.Certificate.UpdateOneID(pc.certRecordID).
		SetCertContent(certContent).
		SetKeyContent(keyContent).
		SetIssuer(x509Cert.Issuer.CommonName).
		SetNotBefore(x509Cert.NotBefore).
		SetNotAfter(x509Cert.NotAfter).
		SetStatus("active").
		SetLastError("").
		Save(ctx)
	if err != nil {
		logging.Error(i18n.T("log.cert_record_update_failed", "ID", pc.certRecordID, "Error", err))
		return nil, fmt.Errorf(i18n.T("error.update_cert_record_failed", "Error", err))
	}

	// 清理挑战状态
	s.pendingChallenges.Delete(domain)

	// 发送成功通知
	if s.notifService != nil {
		_ = s.notifService.SendCertApplied(pc.req.Domain, x509Cert.Issuer.CommonName)
	}

	logging.Info(i18n.T("log.manual_dns_cert_apply_success", "Domain", pc.req.Domain, "Issuer", x509Cert.Issuer.CommonName, "NotAfter", x509Cert.NotAfter))
	return &ApplyCertificateResult{
		Success:     true,
		CertContent: certContent,
		KeyContent:  keyContent,
		Issuer:      x509Cert.Issuer.CommonName,
		NotBefore:   x509Cert.NotBefore.Format(time.RFC3339),
		NotAfter:    x509Cert.NotAfter.Format(time.RFC3339),
	}, nil
}
