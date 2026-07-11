package scanner

import (
	"context"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"time"

	"cnb.cool/dtapp/certflow/ent"
	"cnb.cool/dtapp/certflow/ent/scanresult"
	"cnb.cool/dtapp/certflow/internal/i18n"
	"cnb.cool/dtapp/certflow/internal/logging"
	"cnb.cool/dtapp/certflow/internal/network"
	"cnb.cool/dtapp/certflow/internal/settings"
)

// ScannerService 证书扫描服务
type ScannerService struct {
	db               *ent.Client
	settingsProvider func() settings.Settings
}

// NewScannerService 创建扫描服务
func NewScannerService(client *ent.Client) *ScannerService {
	return &ScannerService{db: client}
}

// SetSettingsProvider 注入设置提供者（用于 DNS/代理配置）
func (s *ScannerService) SetSettingsProvider(fn func() settings.Settings) {
	s.settingsProvider = fn
}

// ScanInput 扫描请求
type ScanInput struct {
	Domain   string `json:"domain"`    // 域名（必填）
	Port     int    `json:"port"`      // 端口号（默认 443）
	ScanType string `json:"scan_type"` // 扫描类型：https/http
}

// ScanResultItem 扫描结果（前端展示用）
type ScanResultItem struct {
	ID                int      `json:"id"`                   // 扫描结果 ID
	Domain            string   `json:"domain"`               // 扫描的域名
	Port              int      `json:"port"`                 // 端口号
	ScanType          string   `json:"scan_type"`            // 扫描类型：https/http
	ScannedAt         string   `json:"scanned_at"`           // 扫描时间
	ResponseTimeMs    int      `json:"response_time_ms"`     // 响应耗时（毫秒）
	CertIssuer        string   `json:"cert_issuer"`          // 证书颁发者
	CertSubject       string   `json:"cert_subject"`         // 证书主题 (CN)
	CertNotBefore     string   `json:"cert_not_before"`      // 证书生效时间
	CertNotAfter      string   `json:"cert_not_after"`       // 证书过期时间
	CertRemainingDays int      `json:"cert_remaining_days"`  // 证书剩余天数
	CertFingerprint   string   `json:"cert_fingerprint"`     // 证书 SHA256 指纹
	CertSignatureAlgo string   `json:"cert_signature_algo"`  // 签名算法
	CertPublicKeyAlgo string   `json:"cert_public_key_algo"` // 公钥算法
	CertPublicKeyBits int      `json:"cert_public_key_bits"` // 公钥位数
	CertSANs          []string `json:"cert_sans"`            // SAN 列表
	CertSerialNumber  string   `json:"cert_serial_number"`   // 证书序列号
	ErrorMessage      string   `json:"error_message"`        // 扫描错误信息
}

// Scan 执行一次证书扫描并保存结果
func (s *ScannerService) Scan(ctx context.Context, input ScanInput) (*ScanResultItem, error) {
	domain := input.Domain
	if domain == "" {
		return nil, fmt.Errorf("%s", i18n.T("error.scanner_empty_domain"))
	}

	port := input.Port
	if port <= 0 {
		if input.ScanType == "http" {
			port = 80
		} else {
			port = 443
		}
	}

	scanType := input.ScanType
	if scanType == "" {
		scanType = "https"
	}

	logging.Info(i18n.T("log.scanner_start", "Domain", domain))

	start := time.Now()
	result := &ScanResultItem{
		Domain:    domain,
		Port:      port,
		ScanType:  scanType,
		ScannedAt: time.Now().Format(time.DateTime),
	}

	// 使用带 DNS/代理支持的 HTTP 客户端
	var client *http.Client
	if s.settingsProvider != nil {
		client = network.BuildHTTPClient(s.settingsProvider())
	} else {
		client = &http.Client{Timeout: 15 * time.Second}
	}

	// 构建 URL
	scheme := "https"
	if scanType == "http" {
		scheme = "http"
	}
	url := fmt.Sprintf("%s://%s", scheme, domain)
	if (scanType == "https" && port != 443) || (scanType == "http" && port != 80) {
		url = fmt.Sprintf("%s://%s:%d", scheme, domain, port)
	}

	// 发送请求
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		result.ErrorMessage = err.Error()
		logging.Error(i18n.T("log.scanner_req_failed", "Error", err))
	} else {
		req.Header.Set("User-Agent", "CertFlow/1.0")
		resp, err := client.Do(req)
		if err != nil {
			result.ErrorMessage = err.Error()
			logging.Error(i18n.T("log.scanner_connect_failed", "Domain", domain, "Error", err))
		} else {
			defer resp.Body.Close()
			io.Copy(io.Discard, resp.Body)
		}
	}

	result.ResponseTimeMs = int(time.Since(start).Milliseconds())

	// HTTPS 模式下提取证书信息
	if result.ErrorMessage == "" && scanType == "https" {
		s.extractCertInfo(domain, port, client, result)
	}

	// 保存到数据库
	item, err := s.saveResult(ctx, result)
	if err != nil {
		logging.Error(i18n.T("log.scanner_save_failed", "Domain", domain, "Error", err))
		return nil, fmt.Errorf("%s", i18n.T("error.scanner_save_failed", "Error", err))
	}

	if result.ErrorMessage != "" {
		logging.Warn(i18n.T("log.scanner_done_error", "Domain", domain, "Error", result.ErrorMessage))
	} else {
		logging.Info(i18n.T("log.scanner_done", "Domain", domain, "Issuer", result.CertIssuer, "Days", result.CertRemainingDays))
	}

	return item, nil
}

// extractCertInfo 通过 TLS 连接提取证书信息
func (s *ScannerService) extractCertInfo(domain string, port int, client *http.Client, result *ScanResultItem) {
	url := fmt.Sprintf("https://%s", domain)
	if port != 443 {
		url = fmt.Sprintf("https://%s:%d", domain, port)
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		result.ErrorMessage = err.Error()
		return
	}
	req.Header.Set("User-Agent", "CertFlow/1.0")

	resp, err := client.Do(req)
	if err != nil {
		result.ErrorMessage = err.Error()
		return
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
		leaf := resp.TLS.PeerCertificates[0]
		fp := sha256.Sum256(leaf.Raw)

		result.CertIssuer = leaf.Issuer.CommonName
		result.CertSubject = leaf.Subject.CommonName
		result.CertNotBefore = leaf.NotBefore.Format(time.DateTime)
		result.CertNotAfter = leaf.NotAfter.Format(time.DateTime)
		result.CertRemainingDays = int(time.Until(leaf.NotAfter).Hours() / 24)
		result.CertFingerprint = fmt.Sprintf("%x", fp)
		result.CertSignatureAlgo = leaf.SignatureAlgorithm.String()
		result.CertPublicKeyAlgo = leaf.PublicKeyAlgorithm.String()
		result.CertPublicKeyBits = getPubKeyBits(leaf)
		result.CertSANs = leaf.DNSNames
		result.CertSerialNumber = fmt.Sprintf("%x", leaf.SerialNumber)
		result.ErrorMessage = ""
	}
}

// saveResult 保存扫描结果到数据库
func (s *ScannerService) saveResult(ctx context.Context, result *ScanResultItem) (*ScanResultItem, error) {
	scannedAt, _ := time.Parse(time.DateTime, result.ScannedAt)
	var notBefore, notAfter time.Time
	if result.CertNotBefore != "" {
		notBefore, _ = time.Parse(time.DateTime, result.CertNotBefore)
	}
	if result.CertNotAfter != "" {
		notAfter, _ = time.Parse(time.DateTime, result.CertNotAfter)
	}

	create := s.db.ScanResult.Create().
		SetDomain(result.Domain).
		SetPort(result.Port).
		SetScanType(scanresult.ScanType(result.ScanType)).
		SetScannedAt(scannedAt).
		SetResponseTimeMs(result.ResponseTimeMs)

	if result.CertIssuer != "" {
		create = create.SetCertIssuer(result.CertIssuer)
	}
	if result.CertSubject != "" {
		create = create.SetCertSubject(result.CertSubject)
	}
	if !notBefore.IsZero() {
		create = create.SetCertNotBefore(notBefore)
	}
	if !notAfter.IsZero() {
		create = create.SetCertNotAfter(notAfter)
	}
	create = create.SetCertRemainingDays(result.CertRemainingDays)
	if result.CertFingerprint != "" {
		create = create.SetCertFingerprint(result.CertFingerprint)
	}
	if result.CertSignatureAlgo != "" {
		create = create.SetCertSignatureAlgo(result.CertSignatureAlgo)
	}
	if result.CertPublicKeyAlgo != "" {
		create = create.SetCertPublicKeyAlgo(result.CertPublicKeyAlgo)
	}
	create = create.SetCertPublicKeyBits(result.CertPublicKeyBits)
	if result.CertSANs != nil {
		create = create.SetCertSans(result.CertSANs)
	}
	if result.CertSerialNumber != "" {
		create = create.SetCertSerialNumber(result.CertSerialNumber)
	}
	if result.ErrorMessage != "" {
		create = create.SetErrorMessage(result.ErrorMessage)
	}

	saved, err := create.Save(ctx)
	if err != nil {
		return nil, err
	}

	result.ID = saved.ID
	return result, nil
}

// ListHistory 获取扫描历史（按时间倒序）
func (s *ScannerService) ListHistory(ctx context.Context) ([]*ScanResultItem, error) {
	items, err := s.db.ScanResult.Query().
		Order(ent.Desc("scanned_at")).
		All(ctx)
	if err != nil {
		return nil, err
	}

	results := make([]*ScanResultItem, 0, len(items))
	for _, item := range items {
		results = append(results, &ScanResultItem{
			ID:                item.ID,
			Domain:            item.Domain,
			Port:              item.Port,
			ScanType:          string(item.ScanType),
			ScannedAt:         item.ScannedAt.Format(time.DateTime),
			ResponseTimeMs:    item.ResponseTimeMs,
			CertIssuer:        item.CertIssuer,
			CertSubject:       item.CertSubject,
			CertNotBefore:     item.CertNotBefore.Format(time.DateTime),
			CertNotAfter:      item.CertNotAfter.Format(time.DateTime),
			CertRemainingDays: item.CertRemainingDays,
			CertFingerprint:   item.CertFingerprint,
			CertSignatureAlgo: item.CertSignatureAlgo,
			CertPublicKeyAlgo: item.CertPublicKeyAlgo,
			CertPublicKeyBits: item.CertPublicKeyBits,
			CertSANs:          item.CertSans,
			CertSerialNumber:  item.CertSerialNumber,
			ErrorMessage:      item.ErrorMessage,
		})
	}
	return results, nil
}

// DeleteResult 删除扫描结果
func (s *ScannerService) DeleteResult(ctx context.Context, id int) error {
	if err := s.db.ScanResult.DeleteOneID(id).Exec(ctx); err != nil {
		logging.Error(i18n.T("log.scanner_delete_failed", "ID", id, "Error", err))
		return err
	}
	logging.Info(i18n.T("log.scanner_delete_ok", "ID", id))
	return nil
}

// ClearHistory 清空扫描历史
func (s *ScannerService) ClearHistory(ctx context.Context) error {
	n, err := s.db.ScanResult.Delete().Exec(ctx)
	if err != nil {
		logging.Error(i18n.T("log.scanner_clear_failed", "Error", err))
		return err
	}
	logging.Info(i18n.T("log.scanner_clear_ok", "Count", n))
	return nil
}

// getPubKeyBits 获取公钥位数
func getPubKeyBits(cert *x509.Certificate) int {
	switch k := cert.PublicKey.(type) {
	case *rsa.PublicKey:
		return k.N.BitLen()
	case *ecdsa.PublicKey:
		return k.Curve.Params().BitSize
	default:
		return 256
	}
}
