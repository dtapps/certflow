package monitor

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"time"

	"cnb.cool/dtapp/certflow/ent"
	"cnb.cool/dtapp/certflow/ent/monitoreddomain"
	"cnb.cool/dtapp/certflow/internal/i18n"
	"cnb.cool/dtapp/certflow/internal/logging"
	"cnb.cool/dtapp/certflow/internal/network"
	"cnb.cool/dtapp/certflow/internal/notification"
	"cnb.cool/dtapp/certflow/internal/settings"
)

// MonitorService 域名监控服务
type MonitorService struct {
	db               *ent.Client
	stopChan         chan struct{}
	settingsProvider func() settings.Settings
	notifService     interface {
		SendNotification(opts notification.NotificationOption) error
	}
	userAgent string
}

// NewMonitorService 创建监控服务
func NewMonitorService(client *ent.Client) *MonitorService {
	return &MonitorService{
		db:       client,
		stopChan: make(chan struct{}),
	}
}

// SetSettingsProvider 设置设置提供者
func (s *MonitorService) SetSettingsProvider(fn func() settings.Settings) {
	s.settingsProvider = fn
}

// SetNotificationService 设置通知服务
func (s *MonitorService) SetNotificationService(ns interface {
	SendNotification(opts notification.NotificationOption) error
}) {
	s.notifService = ns
}

// SetUserAgent 设置 User-Agent
func (s *MonitorService) SetUserAgent(ua string) {
	s.userAgent = ua
}

// MonitoredDomainItem 监控域名条目（前端展示用）
type MonitoredDomainItem struct {
	ID                int      `json:"id"`                   // 监控 ID
	Domain            string   `json:"domain"`               // 域名
	Port              int      `json:"port"`                 // 端口号
	CheckType         string   `json:"check_type"`           // 检查类型：https/http
	URL               string   `json:"url"`                  // 自定义检查 URL
	CheckInterval     int      `json:"check_interval"`       // 检查间隔（秒）
	Enabled           bool     `json:"enabled"`              // 是否启用
	Status            string   `json:"status"`               // 状态：ok/warning/error/expired/unknown
	CertIssuer        string   `json:"cert_issuer"`          // SSL 证书颁发者
	CertNotBefore     string   `json:"cert_not_before"`      // 证书生效时间
	CertNotAfter      string   `json:"cert_not_after"`       // 证书过期时间
	CertFingerprint   string   `json:"cert_fingerprint"`     // 证书 SHA256 指纹
	CertSubject       string   `json:"cert_subject"`         // 证书主题
	CertSignatureAlgo string   `json:"cert_signature_algo"`  // 签名算法
	CertPublicKeyAlgo string   `json:"cert_public_key_algo"` // 公钥算法
	CertPublicKeyBits int      `json:"cert_public_key_bits"` // 公钥位数
	CertSANs          []string `json:"cert_sans"`            // SAN 列表
	CertRemainingDays int      `json:"cert_remaining_days"`  // 剩余天数
	LastCheckAt       string   `json:"last_check_at"`        // 最后检查时间
	LastCheckError    string   `json:"last_check_error"`     // 最后检查错误
	HTTPStatusCode    int      `json:"http_status_code"`     // HTTP 响应码
	ResponseTimeMs    int      `json:"response_time_ms"`     // 响应时间（毫秒）
	CreatedAt         string   `json:"created_at"`           // 创建时间
	UpdatedAt         string   `json:"updated_at"`           // 更新时间
}

// CreateInput 创建/更新监控域名请求
type CreateInput struct {
	Domain        string `json:"domain"`         // 域名
	Port          int    `json:"port"`           // 端口号
	CheckType     string `json:"check_type"`     // 检查类型：https/http
	URL           string `json:"url"`            // 自定义检查 URL
	CheckInterval int    `json:"check_interval"` // 检查间隔（秒）
	Enabled       bool   `json:"enabled"`        // 是否启用
}

func toItem(m *ent.MonitoredDomain) *MonitoredDomainItem {
	return &MonitoredDomainItem{
		ID:                m.ID,
		Domain:            m.Domain,
		Port:              m.Port,
		CheckType:         m.CheckType.String(),
		URL:               m.URL,
		CheckInterval:     m.CheckInterval,
		Enabled:           m.Enabled,
		Status:            m.Status.String(),
		CertIssuer:        m.CertIssuer,
		CertNotBefore:     formatTime(m.CertNotBefore),
		CertNotAfter:      formatTime(m.CertNotAfter),
		CertFingerprint:   m.CertFingerprint,
		CertSubject:       m.CertSubject,
		CertSignatureAlgo: m.CertSignatureAlgo,
		CertPublicKeyAlgo: m.CertPublicKeyAlgo,
		CertPublicKeyBits: m.CertPublicKeyBits,
		CertSANs:          m.CertSans,
		CertRemainingDays: m.CertRemainingDays,
		LastCheckAt:       formatTime(m.LastCheckAt),
		LastCheckError:    m.LastCheckError,
		HTTPStatusCode:    m.HTTPStatusCode,
		ResponseTimeMs:    m.ResponseTimeMs,
		CreatedAt:         m.CreatedAt.Format(time.DateTime),
		UpdatedAt:         m.UpdatedAt.Format(time.DateTime),
	}
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.DateTime)
}

// List 获取所有监控域名
func (s *MonitorService) List(ctx context.Context) ([]*MonitoredDomainItem, error) {
	domains, err := s.db.MonitoredDomain.Query().Order(ent.Asc("id")).All(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]*MonitoredDomainItem, len(domains))
	for i, m := range domains {
		items[i] = toItem(m)
	}
	return items, nil
}

// Create 创建监控域名
func (s *MonitorService) Create(ctx context.Context, input CreateInput) (*MonitoredDomainItem, error) {
	logging.Info(i18n.T("log.monitor_create_start", "Domain", input.Domain, "Type", input.CheckType))
	if input.Port == 0 {
		input.Port = 443
	}
	if input.CheckInterval == 0 {
		input.CheckInterval = 3600
	}

	m, err := s.db.MonitoredDomain.Create().
		SetDomain(input.Domain).
		SetPort(input.Port).
		SetCheckType(monitoreddomain.CheckType(input.CheckType)).
		SetURL(input.URL).
		SetCheckInterval(input.CheckInterval).
		SetEnabled(input.Enabled).
		SetStatus(monitoreddomain.StatusUnknown).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s", i18n.T("error.create_monitored_domain_failed", "Error", err))
	}
	return toItem(m), nil
}

// Update 更新监控域名
func (s *MonitorService) Update(ctx context.Context, id int, input CreateInput) (*MonitoredDomainItem, error) {
	m, err := s.db.MonitoredDomain.UpdateOneID(id).
		SetDomain(input.Domain).
		SetPort(input.Port).
		SetCheckType(monitoreddomain.CheckType(input.CheckType)).
		SetURL(input.URL).
		SetCheckInterval(input.CheckInterval).
		SetEnabled(input.Enabled).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s", i18n.T("error.update_monitored_domain_failed", "Error", err))
	}
	return toItem(m), nil
}

// ToggleEnabled 切换监控域名的启用状态
func (s *MonitorService) ToggleEnabled(ctx context.Context, id int) (*MonitoredDomainItem, error) {
	m, err := s.db.MonitoredDomain.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	updated, err := s.db.MonitoredDomain.UpdateOneID(id).
		SetEnabled(!m.Enabled).
		Save(ctx)
	if err != nil {
		return nil, err
	}
	return toItem(updated), nil
}

// Delete 删除监控域名
func (s *MonitorService) Delete(ctx context.Context, id int) error {
	logging.Info(i18n.T("log.monitor_delete", "ID", id))
	return s.db.MonitoredDomain.DeleteOneID(id).Exec(ctx)
}

// CheckNow 立即执行一次检查
func (s *MonitorService) CheckNow(ctx context.Context, id int) (*MonitoredDomainItem, error) {
	m, err := s.db.MonitoredDomain.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	result := s.checkDomain(m)
	logging.Info(i18n.T("log.monitor_check_result", "Domain", m.Domain, "Status", result.status, "RemainingDays", result.certRemainingDays))

	_, err = s.db.MonitoredDomain.UpdateOneID(m.ID).
		SetStatus(monitoreddomain.Status(result.status)).
		SetCertIssuer(result.certIssuer).
		SetCertNotBefore(result.certNotBefore).
		SetCertNotAfter(result.certNotAfter).
		SetCertFingerprint(result.certFingerprint).
		SetCertSubject(result.certSubject).
		SetCertSignatureAlgo(result.certSignatureAlgo).
		SetCertPublicKeyAlgo(result.certPublicKeyAlgo).
		SetCertPublicKeyBits(result.certPublicKeyBits).
		SetCertSans(result.certSANs).
		SetCertRemainingDays(result.certRemainingDays).
		SetLastCheckAt(time.Now()).
		SetLastCheckError(result.checkError).
		SetHTTPStatusCode(result.httpStatusCode).
		SetResponseTimeMs(result.responseTimeMs).
		Save(ctx)
	if err != nil {
		return nil, err
	}

	updated, err := s.db.MonitoredDomain.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	s.notifyIfProblem(m.Domain, result)
	return toItem(updated), nil
}

type checkResult struct {
	status            string
	certIssuer        string
	certNotBefore     time.Time
	certNotAfter      time.Time
	certFingerprint   string
	certSubject       string
	certSignatureAlgo string
	certPublicKeyAlgo string
	certPublicKeyBits int
	certSANs          []string
	certRemainingDays int
	checkError        string
	httpStatusCode    int
	responseTimeMs    int
}

func (s *MonitorService) checkDomain(m *ent.MonitoredDomain) checkResult {
	logging.Info(i18n.T("log.monitor_check_start", "Domain", m.Domain, "Type", m.CheckType))
	var result checkResult
	switch m.CheckType {
	case "http":
		result = s.checkHTTP(m)
	default:
		result = s.checkHTTPS(m)
	}
	if result.status == "ok" {
		logging.Info(i18n.T("log.monitor_check_ok", "Domain", m.Domain, "Status", result.status, "Time", result.responseTimeMs, "ms"))
	} else {
		logging.Warn(i18n.T("log.monitor_check_warn", "Domain", m.Domain, "Status", result.status, "Error", result.checkError))
	}
	return result
}

// getHTTPClient 获取配置了 DNS 和代理的 HTTP 客户端
func (s *MonitorService) getHTTPClient() *http.Client {
	if s.settingsProvider != nil {
		return network.BuildHTTPClient(s.settingsProvider())
	}
	return &http.Client{Timeout: 10 * time.Second}
}

// checkHTTPS 通过 HTTPS 检查接口健康 + SSL 证书信息
func (s *MonitorService) checkHTTPS(m *ent.MonitoredDomain) checkResult {
	result := checkResult{status: "error"}

	url := m.URL
	if url == "" {
		url = fmt.Sprintf("https://%s", m.Domain)
	}

	client := s.getHTTPClient()

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		result.checkError = err.Error()
		return result
	}
	if s.userAgent != "" {
		req.Header.Set("User-Agent", s.userAgent)
	}

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		result.checkError = err.Error()
		return result
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	result.responseTimeMs = int(time.Since(start).Milliseconds())
	result.httpStatusCode = resp.StatusCode

	// HTTP 状态判断
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		result.status = "ok"
	} else if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		result.status = "warning"
	} else {
		result.status = "error"
	}

	// 提取 SSL 证书信息
	if resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
		leaf := resp.TLS.PeerCertificates[0]
		fp := sha256.Sum256(leaf.Raw)
		result.certIssuer = leaf.Issuer.CommonName
		result.certNotBefore = leaf.NotBefore
		result.certNotAfter = leaf.NotAfter
		result.certFingerprint = fmt.Sprintf("%x", fp)
		result.certSubject = leaf.Subject.CommonName
		result.certSignatureAlgo = leaf.SignatureAlgorithm.String()
		result.certPublicKeyAlgo = leaf.PublicKeyAlgorithm.String()
		result.certPublicKeyBits = getPubKeyBits(leaf)
		result.certSANs = leaf.DNSNames
		result.certRemainingDays = int(time.Until(leaf.NotAfter).Hours() / 24)

		// SSL 证书过期也会降级状态
		if result.certRemainingDays <= 0 {
			result.status = "error"
		} else if result.certRemainingDays <= 30 && result.status == "ok" {
			result.status = "warning"
		}
	}

	logging.Debug(i18n.T("log.monitor_ssl_info", "Domain", m.Domain,
		"Issuer", result.certIssuer,
		"NotAfter", formatTime(result.certNotAfter),
		"RemainingDays", result.certRemainingDays,
		"StatusCode", result.httpStatusCode,
		"ResponseTime", result.responseTimeMs, "ms"))

	return result
}

// checkHTTP 通过 HTTP 请求检查接口健康（不检查 SSL）
func (s *MonitorService) checkHTTP(m *ent.MonitoredDomain) checkResult {
	result := checkResult{status: "error"}

	url := m.URL
	if url == "" {
		url = fmt.Sprintf("http://%s", m.Domain)
	}

	client := s.getHTTPClient()

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		result.checkError = err.Error()
		return result
	}
	if s.userAgent != "" {
		req.Header.Set("User-Agent", s.userAgent)
	}

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		result.checkError = err.Error()
		return result
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	result.responseTimeMs = int(time.Since(start).Milliseconds())
	result.httpStatusCode = resp.StatusCode

	logging.Debug(i18n.T("log.monitor_http_info", "Domain", m.Domain,
		"URL", url,
		"StatusCode", resp.StatusCode,
		"ResponseTime", result.responseTimeMs, "ms"))

	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		result.status = "ok"
	} else if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		result.status = "warning"
	} else {
		result.status = "error"
	}

	if resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
		leaf := resp.TLS.PeerCertificates[0]
		fp := sha256.Sum256(leaf.Raw)
		result.certIssuer = leaf.Issuer.CommonName
		result.certNotBefore = leaf.NotBefore
		result.certNotAfter = leaf.NotAfter
		result.certFingerprint = fmt.Sprintf("%x", fp)
		result.certSubject = leaf.Subject.CommonName
		result.certSANs = leaf.DNSNames
		result.certRemainingDays = int(time.Until(leaf.NotAfter).Hours() / 24)
	}

	return result
}

// Start 启动后台监控循环
func (s *MonitorService) Start() {
	logging.Info(i18n.T("log.monitor_started"))
	go s.monitorLoop()
}

// Stop 停止后台监控
func (s *MonitorService) Stop() {
	logging.Info(i18n.T("log.monitor_stopped"))
	close(s.stopChan)
}

func (s *MonitorService) monitorLoop() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.runChecks()
		}
	}
}

func (s *MonitorService) runChecks() {
	ctx := context.Background()
	domains, err := s.db.MonitoredDomain.Query().
		Where(monitoreddomain.Enabled(true)).
		All(ctx)
	if err != nil {
		logging.Error(i18n.T("log.monitor_query_failed", "Error", err))
		return
	}

	now := time.Now()
	for _, m := range domains {
		if m.CheckInterval <= 0 {
			continue
		}
		nextCheck := m.LastCheckAt.Add(time.Duration(m.CheckInterval) * time.Second)
		if now.Before(nextCheck) {
			continue
		}

		result := s.checkDomain(m)

		_, err := s.db.MonitoredDomain.UpdateOneID(m.ID).
			SetStatus(monitoreddomain.Status(result.status)).
			SetCertIssuer(result.certIssuer).
			SetCertNotBefore(result.certNotBefore).
			SetCertNotAfter(result.certNotAfter).
			SetCertFingerprint(result.certFingerprint).
			SetCertSubject(result.certSubject).
			SetCertSignatureAlgo(result.certSignatureAlgo).
			SetCertPublicKeyAlgo(result.certPublicKeyAlgo).
			SetCertPublicKeyBits(result.certPublicKeyBits).
			SetCertSans(result.certSANs).
			SetCertRemainingDays(result.certRemainingDays).
			SetLastCheckAt(now).
			SetLastCheckError(result.checkError).
			SetHTTPStatusCode(result.httpStatusCode).
			SetResponseTimeMs(result.responseTimeMs).
			Save(ctx)
		if err != nil {
			logging.Error(i18n.T("log.monitor_update_failed", "Domain", m.Domain, "Error", err))
		}

		s.notifyIfProblem(m.Domain, result)
	}
	if len(domains) > 0 {
		logging.Debug(i18n.T("log.monitor_check_done", "Count", len(domains)))
	}
}

// notifyIfProblem 检查结果异常时发送通知
func (s *MonitorService) notifyIfProblem(domain string, result checkResult) {
	if s.notifService == nil {
		return
	}

	var title string
	var body string

	switch result.status {
	case "error":
		title = i18n.T("notification.monitor_error.title")
		body = i18n.T("notification.monitor_error.body", "Domain", domain)
		if result.checkError != "" {
			body += ": " + result.checkError
		}
		if result.httpStatusCode > 0 {
			body += fmt.Sprintf(" (HTTP %d)", result.httpStatusCode)
		}
	case "expired":
		title = i18n.T("notification.monitor_expired.title")
		body = i18n.T("notification.monitor_expired.body", "Domain", domain)
		if result.certIssuer != "" {
			body += fmt.Sprintf(" (%s)", result.certIssuer)
		}
	case "warning":
		if result.certRemainingDays > 0 && result.certRemainingDays <= 30 {
			title = i18n.T("notification.monitor_expiring.title")
			body = i18n.T("notification.monitor_expiring.body", "Domain", domain, "Days", result.certRemainingDays)
			if result.certIssuer != "" {
				body += fmt.Sprintf(" (%s)", result.certIssuer)
			}
		} else if result.httpStatusCode >= 400 {
			title = i18n.T("notification.monitor_http_error.title")
			body = i18n.T("notification.monitor_http_error.body", "Domain", domain, "Code", result.httpStatusCode)
		}
	default:
		return
	}

	_ = s.notifService.SendNotification(notification.NotificationOption{
		Title:    title,
		Body:     body,
		Category: "monitor",
		Level:    "info",
	})
}

// getPubKeyBits 获取公钥位数
func getPubKeyBits(cert *x509.Certificate) int {
	switch pk := cert.PublicKey.(type) {
	case *ecdsa.PublicKey:
		switch pk.Curve {
		case elliptic.P256():
			return 256
		case elliptic.P384():
			return 384
		case elliptic.P521():
			return 521
		}
		return pk.Curve.Params().BitSize
	case interface{ Size() int }:
		return pk.Size() * 8
	}
	return 0
}
