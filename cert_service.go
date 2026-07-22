package main

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"strings"
	"time"

	"cnb.cool/dtapp/certflow/internal/certificate"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// CertificateServiceWrapper 包装 certificate.CertificateService 以适配 Wails v3 服务接口
type CertificateServiceWrapper struct {
	certService *certificate.CertificateService
}

// NewCertificateServiceWrapper 创建新的证书服务包装器
func NewCertificateServiceWrapper(certService *certificate.CertificateService) *CertificateServiceWrapper {
	return &CertificateServiceWrapper{certService: certService}
}

// CertificateListItem 证书列表项（前端展示用）
type CertificateListItem struct {
	ID              int      `json:"id"`                // 证书 ID
	Domain          string   `json:"domain"`            // 主域名
	Sans            []string `json:"sans"`              // 备用域名列表
	CertContent     string   `json:"cert_content"`      // 证书 PEM 内容
	KeyContent      string   `json:"key_content"`       // 私钥 PEM 内容
	Issuer          string   `json:"issuer"`            // 证书颁发者
	NotBefore       string   `json:"not_before"`        // 生效时间
	NotAfter        string   `json:"not_after"`         // 过期时间
	Status          string   `json:"status"`            // 证书状态
	AutoRenew       bool     `json:"auto_renew"`        // 是否自动续期
	RenewalDays     int      `json:"renewal_days"`      // 到期前续期天数
	LastError       string   `json:"last_error"`        // 最后错误信息
	LastRenewedAt   string   `json:"last_renewed_at"`   // 最后续期时间
	KeyType         string   `json:"key_type"`          // 密钥类型
	CAName          string   `json:"ca_name"`           // 关联 CA 名称
	DNSProviderName string   `json:"dns_provider_name"` // 关联 DNS 提供商
	CreatedAt       string   `json:"created_at"`        // 创建时间
	UpdatedAt       string   `json:"updated_at"`        // 更新时间
}

// ApplyCertRequest 申请证书请求
type ApplyCertRequest struct {
	Domain        string   `json:"domain"`                    // 主域名
	Sans          []string `json:"sans"`                      // 备用域名列表
	CAID          int      `json:"ca_id"`                     // CA 配置 ID
	DNSProviderID *int     `json:"dns_provider_id,omitempty"` // DNS 提供商 ID（手动 DNS 为 nil）
	AutoRenew     bool     `json:"auto_renew"`                // 是否自动续期
	RenewalDays   int      `json:"renewal_days"`              // 到期前续期天数
	KeyType       string   `json:"key_type,omitempty"`        // 密钥类型：RSA2048/RSA3072/RSA4096/EC256/EC384
}

// ApplyCertResult 证书申请结果
type ApplyCertResult struct {
	Success     bool   `json:"success"`                // 是否成功
	CertContent string `json:"cert_content,omitempty"` // 证书 PEM 内容
	KeyContent  string `json:"key_content,omitempty"`  // 私钥 PEM 内容
	Issuer      string `json:"issuer,omitempty"`       // 证书颁发者
	NotBefore   string `json:"not_before,omitempty"`   // 生效时间
	NotAfter    string `json:"not_after,omitempty"`    // 过期时间
	Error       string `json:"error,omitempty"`        // 错误信息
}

// ManualDNSChallenge 手动 DNS 挑战信息
type ManualDNSChallenge struct {
	Domain  string          `json:"domain"`  // 域名
	Records []TXTRecordItem `json:"records"` // TXT 记录列表
}

// TXTRecordItem DNS TXT 记录项
type TXTRecordItem struct {
	Name  string `json:"name"`  // TXT 记录名称（FQDN）
	Value string `json:"value"` // TXT 记录值
}

// convertRecords 转换内部记录类型为包装器类型
func convertRecords(records []certificate.TXTRecord) []TXTRecordItem {
	if records == nil {
		return nil
	}
	result := make([]TXTRecordItem, len(records))
	for i, r := range records {
		result[i] = TXTRecordItem{Name: r.Name, Value: r.Value}
	}
	return result
}

// ListCertificates 获取所有证书
func (s *CertificateServiceWrapper) ListCertificates() ([]CertificateListItem, error) {
	ctx := context.Background()
	certs, err := s.certService.List(ctx)
	if err != nil {
		return nil, err
	}

	items := make([]CertificateListItem, len(certs))
	for i, c := range certs {
		lastRenewed := ""
		if !c.LastRenewedAt.IsZero() {
			lastRenewed = c.LastRenewedAt.Format(time.DateTime)
		}
		notBefore := ""
		if !c.NotBefore.IsZero() {
			notBefore = c.NotBefore.Format(time.DateTime)
		}
		notAfter := ""
		if !c.NotAfter.IsZero() {
			notAfter = c.NotAfter.Format(time.DateTime)
		}
		// 查询关联的 CA 与 DNS 提供商名称（与 GetCertificateInfo 保持一致）
		caName := ""
		if ca, err := c.QueryCa().Only(ctx); err == nil {
			caName = ca.Name
		}
		dnsProviderName := ""
		if dp, err := c.QueryDNSProvider().Only(ctx); err == nil {
			dnsProviderName = dp.Name
		}
		items[i] = CertificateListItem{
			ID:              c.ID,
			Domain:          c.Domain,
			Sans:            c.Sans,
			CertContent:     c.CertContent,
			KeyContent:      c.KeyContent,
			Issuer:          c.Issuer,
			NotBefore:       notBefore,
			NotAfter:        notAfter,
			Status:          c.Status.String(),
			AutoRenew:       c.AutoRenew,
			RenewalDays:     c.RenewalDays,
			LastError:       c.LastError,
			LastRenewedAt:   lastRenewed,
			KeyType:         c.KeyType.String(),
			CAName:          caName,
			DNSProviderName: dnsProviderName,
			CreatedAt:       c.CreatedAt.Format(time.DateTime),
			UpdatedAt:       c.UpdatedAt.Format(time.DateTime),
		}
	}
	return items, nil
}

// ApplyCertificate 申请证书
func (s *CertificateServiceWrapper) ApplyCertificate(input ApplyCertRequest) (*ApplyCertResult, error) {
	ctx := context.Background()
	result, err := s.certService.ApplyCertificate(ctx, certificate.CertificateRequest{
		Domain:        input.Domain,
		Sans:          input.Sans,
		CAID:          input.CAID,
		DNSProviderID: input.DNSProviderID,
		AutoRenew:     input.AutoRenew,
		RenewalDays:   input.RenewalDays,
		KeyType:       input.KeyType,
	})
	if err != nil {
		return &ApplyCertResult{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	return &ApplyCertResult{
		Success:     result.Success,
		CertContent: result.CertContent,
		KeyContent:  result.KeyContent,
		Issuer:      result.Issuer,
		NotBefore:   result.NotBefore,
		NotAfter:    result.NotAfter,
		Error:       result.Error,
	}, nil
}

// RenewCertificate 续期证书
func (s *CertificateServiceWrapper) RenewCertificate(id int) (*ApplyCertResult, error) {
	ctx := context.Background()
	result, err := s.certService.RenewCertificate(ctx, id)
	if err != nil {
		return &ApplyCertResult{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	return &ApplyCertResult{
		Success:     result.Success,
		CertContent: result.CertContent,
		KeyContent:  result.KeyContent,
		Issuer:      result.Issuer,
		NotBefore:   result.NotBefore,
		NotAfter:    result.NotAfter,
	}, nil
}

// RevokeCertificate 撤销证书
func (s *CertificateServiceWrapper) RevokeCertificate(id int) error {
	ctx := context.Background()
	return s.certService.RevokeCertificate(ctx, id)
}

// UpdateCertificateSettings 更新证书设置（自动续期、续期天数）
func (s *CertificateServiceWrapper) UpdateCertificateSettings(id int, autoRenew bool, renewalDays int) error {
	ctx := context.Background()
	return s.certService.UpdateSettings(ctx, id, autoRenew, renewalDays)
}

// DeleteCertificate 删除证书
func (s *CertificateServiceWrapper) DeleteCertificate(id int) error {
	ctx := context.Background()
	return s.certService.Delete(ctx, id)
}

// GetCertificateInfo 获取证书详情
func (s *CertificateServiceWrapper) GetCertificateInfo(id int) (*CertificateListItem, error) {
	ctx := context.Background()
	cert, err := s.certService.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	lastRenewed := ""
	if !cert.LastRenewedAt.IsZero() {
		lastRenewed = cert.LastRenewedAt.Format(time.DateTime)
	}
	notBefore := ""
	if !cert.NotBefore.IsZero() {
		notBefore = cert.NotBefore.Format(time.DateTime)
	}
	notAfter := ""
	if !cert.NotAfter.IsZero() {
		notAfter = cert.NotAfter.Format(time.DateTime)
	}

	// 查询关联的 CA 和 DNS 提供商名称
	caName := ""
	if ca, err := cert.QueryCa().Only(ctx); err == nil {
		caName = ca.Name
	}
	dnsProviderName := ""
	if dp, err := cert.QueryDNSProvider().Only(ctx); err == nil {
		dnsProviderName = dp.Name
	}

	return &CertificateListItem{
		ID:              cert.ID,
		Domain:          cert.Domain,
		Sans:            cert.Sans,
		CertContent:     cert.CertContent,
		KeyContent:      cert.KeyContent,
		Issuer:          cert.Issuer,
		NotBefore:       notBefore,
		NotAfter:        notAfter,
		Status:          cert.Status.String(),
		AutoRenew:       cert.AutoRenew,
		RenewalDays:     cert.RenewalDays,
		LastError:       cert.LastError,
		LastRenewedAt:   lastRenewed,
		KeyType:         cert.KeyType.String(),
		CAName:          caName,
		DNSProviderName: dnsProviderName,
		CreatedAt:       cert.CreatedAt.Format(time.DateTime),
		UpdatedAt:       cert.UpdatedAt.Format(time.DateTime),
	}, nil
}

// StartManualDNSChallenge 开始手动 DNS 挑战（第一步）
func (s *CertificateServiceWrapper) StartManualDNSChallenge(input ApplyCertRequest) (*ManualDNSChallenge, error) {
	ctx := context.Background()
	info, err := s.certService.StartManualDNSChallenge(ctx, certificate.CertificateRequest{
		Domain:        input.Domain,
		Sans:          input.Sans,
		CAID:          input.CAID,
		DNSProviderID: input.DNSProviderID,
		AutoRenew:     input.AutoRenew,
		RenewalDays:   input.RenewalDays,
		KeyType:       input.KeyType,
	})
	if err != nil {
		return nil, err
	}
	return &ManualDNSChallenge{
		Domain:  info.Domain,
		Records: convertRecords(info.Records),
	}, nil
}

// CompleteManualDNSChallenge 完成手动 DNS 挑战（第二步）
func (s *CertificateServiceWrapper) CompleteManualDNSChallenge(domain string) (*ApplyCertResult, error) {
	ctx := context.Background()
	result, err := s.certService.CompleteManualDNSChallenge(ctx, domain)
	if err != nil {
		return &ApplyCertResult{
			Success: false,
			Error:   err.Error(),
		}, nil
	}
	return &ApplyCertResult{
		Success:     result.Success,
		CertContent: result.CertContent,
		KeyContent:  result.KeyContent,
		Issuer:      result.Issuer,
		NotBefore:   result.NotBefore,
		NotAfter:    result.NotAfter,
		Error:       result.Error,
	}, nil
}

// GetPendingChallengeInfo 获取待完成的手动 DNS 挑战信息（用于继续申请）
func (s *CertificateServiceWrapper) GetPendingChallengeInfo(certID int) (*ManualDNSChallenge, error) {
	ctx := context.Background()
	info, err := s.certService.GetPendingChallengeInfo(ctx, certID)
	if err != nil {
		return nil, err
	}
	return &ManualDNSChallenge{
		Domain:  info.Domain,
		Records: convertRecords(info.Records),
	}, nil
}

// ResumeManualDNSChallenge 恢复手动 DNS 挑战（重新生成挑战信息）
func (s *CertificateServiceWrapper) ResumeManualDNSChallenge(certID int) (*ManualDNSChallenge, error) {
	ctx := context.Background()
	info, err := s.certService.ResumeManualDNSChallenge(ctx, certID)
	if err != nil {
		return nil, err
	}
	return &ManualDNSChallenge{
		Domain:  info.Domain,
		Records: convertRecords(info.Records),
	}, nil
}

// GetExpiringCertificates 获取即将过期的证书
func (s *CertificateServiceWrapper) GetExpiringCertificates(days int) ([]CertificateListItem, error) {
	ctx := context.Background()
	certs, err := s.certService.ListExpiring(ctx, days)
	if err != nil {
		return nil, err
	}

	items := make([]CertificateListItem, len(certs))
	for i, c := range certs {
		notBefore := ""
		if !c.NotBefore.IsZero() {
			notBefore = c.NotBefore.Format(time.DateTime)
		}
		notAfter := ""
		if !c.NotAfter.IsZero() {
			notAfter = c.NotAfter.Format(time.DateTime)
		}
		items[i] = CertificateListItem{
			ID:        c.ID,
			Domain:    c.Domain,
			Sans:      c.Sans,
			Issuer:    c.Issuer,
			NotBefore: notBefore,
			NotAfter:  notAfter,
			Status:    c.Status.String(),
			AutoRenew: c.AutoRenew,
			KeyType:   c.KeyType.String(),
		}
	}
	return items, nil
}

// CertificateDetails 证书详细信息（x509 解析结果）
type CertificateDetails struct {
	SerialNumber       string   `json:"serial_number"`        // 序列号
	SignatureAlgorithm string   `json:"signature_algorithm"`  // 签名算法
	PublicKeyAlgorithm string   `json:"public_key_algorithm"` // 公钥算法
	PublicKeySize      int      `json:"public_key_size"`      // 公钥位数
	KeyUsage           string   `json:"key_usage"`            // 密钥用途
	ExtKeyUsage        string   `json:"ext_key_usage"`        // 扩展密钥用途
	IsCA               bool     `json:"is_ca"`                // 是否 CA 证书
	Version            int      `json:"version"`              // 证书版本
	FingerprintSHA256  string   `json:"fingerprint_sha256"`   // SHA256 指纹
	DNSNames           []string `json:"dns_names"`            // DNS 名称（SAN）
	IPAddresses        []string `json:"ip_addresses"`         // IP 地址（SAN）
	EmailAddresses     []string `json:"email_addresses"`      // 邮箱地址（SAN）
}

// ParseCertificateDetails 解析证书 PEM 内容返回详细信息
func (s *CertificateServiceWrapper) ParseCertificateDetails(id int) (*CertificateDetails, error) {
	ctx := context.Background()
	cert, err := s.certService.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if cert.CertContent == "" {
		return nil, fmt.Errorf("certificate content is empty")
	}

	// 解析 PEM
	block, _ := pem.Decode([]byte(cert.CertContent))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM")
	}
	x509Cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}

	// 计算 SHA-256 指纹
	fingerprint := sha256.Sum256(block.Bytes)

	// IP 地址转字符串
	ipAddrs := make([]string, len(x509Cert.IPAddresses))
	for i, ip := range x509Cert.IPAddresses {
		ipAddrs[i] = ip.String()
	}

	return &CertificateDetails{
		SerialNumber:       x509Cert.SerialNumber.String(),
		SignatureAlgorithm: x509Cert.SignatureAlgorithm.String(),
		PublicKeyAlgorithm: x509Cert.PublicKeyAlgorithm.String(),
		PublicKeySize:      getPublicKeySize(x509Cert),
		KeyUsage:           formatKeyUsage(x509Cert.KeyUsage),
		ExtKeyUsage:        formatExtKeyUsage(x509Cert.ExtKeyUsage),
		IsCA:               x509Cert.IsCA,
		Version:            x509Cert.Version,
		FingerprintSHA256:  hex.EncodeToString(fingerprint[:]),
		DNSNames:           x509Cert.DNSNames,
		IPAddresses:        ipAddrs,
		EmailAddresses:     x509Cert.EmailAddresses,
	}, nil
}

// formatKeyUsage 格式化密钥用途
func formatKeyUsage(ku x509.KeyUsage) string {
	var usages []string
	flags := []struct {
		flag x509.KeyUsage
		name string
	}{
		{x509.KeyUsageDigitalSignature, "Digital Signature"},
		{x509.KeyUsageContentCommitment, "Content Commitment"},
		{x509.KeyUsageKeyEncipherment, "Key Encipherment"},
		{x509.KeyUsageDataEncipherment, "Data Encipherment"},
		{x509.KeyUsageKeyAgreement, "Key Agreement"},
		{x509.KeyUsageCertSign, "Certificate Sign"},
		{x509.KeyUsageCRLSign, "CRL Sign"},
		{x509.KeyUsageEncipherOnly, "Encipher Only"},
		{x509.KeyUsageDecipherOnly, "Decipher Only"},
	}
	for _, f := range flags {
		if ku&f.flag != 0 {
			usages = append(usages, f.name)
		}
	}
	return strings.Join(usages, ", ")
}

// formatExtKeyUsage 格式化扩展密钥用途
func formatExtKeyUsage(eku []x509.ExtKeyUsage) string {
	var usages []string
	for _, u := range eku {
		switch u {
		case x509.ExtKeyUsageServerAuth:
			usages = append(usages, "Server Auth")
		case x509.ExtKeyUsageClientAuth:
			usages = append(usages, "Client Auth")
		case x509.ExtKeyUsageCodeSigning:
			usages = append(usages, "Code Signing")
		case x509.ExtKeyUsageEmailProtection:
			usages = append(usages, "Email Protection")
		case x509.ExtKeyUsageIPSECEndSystem:
			usages = append(usages, "IPSEC End System")
		case x509.ExtKeyUsageIPSECTunnel:
			usages = append(usages, "IPSEC Tunnel")
		case x509.ExtKeyUsageIPSECUser:
			usages = append(usages, "IPSEC User")
		case x509.ExtKeyUsageTimeStamping:
			usages = append(usages, "Timestamping")
		case x509.ExtKeyUsageOCSPSigning:
			usages = append(usages, "OCSP Signing")
		case x509.ExtKeyUsageMicrosoftServerGatedCrypto:
			usages = append(usages, "Microsoft Server Gated Crypto")
		case x509.ExtKeyUsageMicrosoftCommercialCodeSigning:
			usages = append(usages, "Microsoft Commercial Code Signing")
		default:
			usages = append(usages, fmt.Sprintf("Unknown(%d)", u))
		}
	}
	return strings.Join(usages, ", ")
}

// getPublicKeySize 获取公钥大小
func getPublicKeySize(cert *x509.Certificate) int {
	switch pub := cert.PublicKey.(type) {
	case interface{ Size() int }:
		return pub.Size() * 8
	default:
		return 0
	}
}

// ServiceStartup 实现 Wails 服务接口
func (s *CertificateServiceWrapper) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	return nil
}

// ServiceShutdown 实现 Wails 服务接口
func (s *CertificateServiceWrapper) ServiceShutdown() error {
	return nil
}

// ServiceName 实现 Wails 服务接口
func (s *CertificateServiceWrapper) ServiceName() string {
	return "CertificateService"
}
