package monitor

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/x509"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"sync"
	"time"

	"cnb.cool/dtapp/certflow/internal/ent"
	"cnb.cool/dtapp/certflow/internal/ent/monitorchecklog"
	"cnb.cool/dtapp/certflow/internal/ent/monitoreddomain"
	entnotification "cnb.cool/dtapp/certflow/internal/ent/notification"
	"cnb.cool/dtapp/certflow/internal/events"
	"cnb.cool/dtapp/certflow/internal/i18n"
	"cnb.cool/dtapp/certflow/internal/logging"
	"cnb.cool/dtapp/certflow/internal/network"
	"cnb.cool/dtapp/certflow/internal/notification"
	"cnb.cool/dtapp/certflow/internal/settings"
)

// maxConcurrentChecks 单轮域名检查的最大并发数。
// 单个检查最坏耗时等于 HTTP 客户端超时（默认 10s），串行执行时单轮耗时随域名数线性放大：
// 域名较多时一个 tick 做不完，检查间隔失真、后续轮次顺延。有界并发将最坏单轮耗时
// 控制在「域名数 / 并发数」个超时以内，同时避免瞬间打过量的并发请求。
const maxConcurrentChecks = 5

// checkJitterMax 单个检查启动前的最大随机抖动。
// 同一 tick 内到期的域名（尤其是检查间隔相同的域名）会同时发起请求，
// 抖动用于错开请求时刻，避免对本地网络与被监控服务造成瞬时拥塞。
const checkJitterMax = 2 * time.Second

// EventEmitter 监控更新事件发射器。
// 由 main.go 以 Wails app.Event 适配注入，避免 monitor 包直接依赖 Wails application 包（便于单测）。
type EventEmitter interface {
	EmitMonitorUpdated(payload events.MonitorUpdatedPayload)
}

// MonitorService 域名监控服务
type MonitorService struct {
	db               *ent.Client
	stopChan         chan struct{}
	stopOnce         sync.Once
	settingsProvider func() settings.Settings
	// clientCache 缓存按设置构建的 HTTP 客户端：避免每次检查都新建 Transport
	// （无连接池复用、每次完整 TCP+TLS 握手），设置变化时自动重建
	clientCache  *network.ClientCache
	notifService interface {
		SendNotification(opts notification.NotificationOption) error
	}
	// notifMu 保护 lastNotified 并发读写（后台循环与 CheckNow 可能并发）
	notifMu sync.RWMutex
	// lastNotified 记录各域名上次已通知的状态，作为告警去重基线：
	// 只有状态发生变化（如 ok→error、error→ok）才发送通知，避免同一异常每轮检查重复告警
	lastNotified map[int]string
	// notifySendMu 串行化通知发送：后台检查并发执行时，保证通知服务的副作用
	// （系统通知/事件发射/落库）有序进行，避免多 goroutine 并发调用通知服务
	notifySendMu sync.Mutex
	// eventEmitter 监控更新事件发射器（可选），用于向前端推送实时刷新事件
	eventEmitter EventEmitter
}

// NewMonitorService 创建监控服务
func NewMonitorService(client *ent.Client) *MonitorService {
	return &MonitorService{
		db:           client,
		stopChan:     make(chan struct{}),
		lastNotified: make(map[int]string),
	}
}

// SetSettingsProvider 设置设置提供者
func (s *MonitorService) SetSettingsProvider(fn func() settings.Settings) {
	s.settingsProvider = fn
	// 客户端缓存与设置提供者同生命周期：provider 变更（代理/DNS）时按指纹重建客户端
	s.clientCache = network.NewClientCache(fn)
}

// SetNotificationService 设置通知服务
func (s *MonitorService) SetNotificationService(ns interface {
	SendNotification(opts notification.NotificationOption) error
}) {
	s.notifService = ns
}

// SetEventEmitter 设置监控更新事件发射器（由 main.go 注入 Wails app.Event 适配器）
func (s *MonitorService) SetEventEmitter(em EventEmitter) {
	s.eventEmitter = em
}

// emitUpdated 发送监控数据更新事件给前端（触发界面实时刷新）。
// id 为发生更新的域名 ID，传 0 表示批量更新（前端刷新全部列表）。
func (s *MonitorService) emitUpdated(id int, domain, status string) {
	if s.eventEmitter == nil {
		return
	}
	s.eventEmitter.EmitMonitorUpdated(events.MonitorUpdatedPayload{
		DomainID:  id,
		Domain:    domain,
		Status:    status,
		CheckedAt: time.Now().Format(time.RFC3339),
	})
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

// MonitorCheckLogItem 检查历史记录条目（前端趋势图用）
type MonitorCheckLogItem struct {
	ID                int    `json:"id"`                  // 历史记录 ID
	CheckedAt         string `json:"checked_at"`          // 检查时间
	Status            string `json:"status"`              // 状态：ok/warning/error/expired/unknown
	CertRemainingDays *int   `json:"cert_remaining_days"` // 证书剩余天数（无证书时为 null）
	ResponseTimeMs    *int   `json:"response_time_ms"`    // 响应时间（毫秒，无数据时为 null）
	HTTPStatusCode    *int   `json:"http_status_code"`    // HTTP 响应码（无数据时为 null）
	LastCheckError    string `json:"last_check_error"`    // 检查错误信息
}

// CreateInput 创建监控域名请求
type CreateInput struct {
	Domain        string `json:"domain"`         // 域名
	Port          int    `json:"port"`           // 端口号
	CheckType     string `json:"check_type"`     // 检查类型：https/http
	URL           string `json:"url"`            // 自定义检查 URL
	CheckInterval int    `json:"check_interval"` // 检查间隔（秒）
}

// UpdateInput 更新监控域名请求
type UpdateInput struct {
	Domain        string `json:"domain"`         // 域名
	Port          int    `json:"port"`           // 端口号
	CheckType     string `json:"check_type"`     // 检查类型：https/http
	URL           string `json:"url"`            // 自定义检查 URL
	CheckInterval int    `json:"check_interval"` // 检查间隔（秒）
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
		CreatedAt:         m.CreatedAt.Format(time.RFC3339),
		UpdatedAt:         m.UpdatedAt.Format(time.RFC3339),
	}
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

// createCheckLog 将一次检查结果写入历史表（趋势图数据源）
func (s *MonitorService) createCheckLog(ctx context.Context, m *ent.MonitoredDomain, result checkResult) {
	create := s.db.MonitorCheckLog.Create().
		SetDomain(m).
		SetStatus(monitorchecklog.Status(result.status)).
		SetCheckedAt(time.Now()).
		SetLastCheckError(result.checkError).
		SetResponseTimeMs(result.responseTimeMs).
		SetHTTPStatusCode(result.httpStatusCode)
	if result.certRemainingDays > 0 {
		create.SetCertRemainingDays(result.certRemainingDays)
	}
	if _, err := create.Save(ctx); err != nil {
		logging.Error(i18n.T("log.monitor_checklog_failed", "Domain", m.Domain, "Error", err))
	}
}

// ListHistory 获取监控域名的检查历史（用于趋势图）
func (s *MonitorService) ListHistory(ctx context.Context, id, days int) ([]*MonitorCheckLogItem, error) {
	if days <= 0 {
		days = 30
	}
	since := time.Now().AddDate(0, 0, -days)
	logs, err := s.db.MonitorCheckLog.Query().
		Where(
			monitorchecklog.HasDomainWith(monitoreddomain.ID(id)),
			monitorchecklog.CheckedAtGTE(since),
		).
		Order(ent.Asc("checked_at")).
		All(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]*MonitorCheckLogItem, len(logs))
	for i, l := range logs {
		items[i] = &MonitorCheckLogItem{
			ID:                l.ID,
			CheckedAt:         formatTime(l.CheckedAt),
			Status:            l.Status.String(),
			CertRemainingDays: l.CertRemainingDays,
			ResponseTimeMs:    l.ResponseTimeMs,
			HTTPStatusCode:    l.HTTPStatusCode,
			LastCheckError:    l.LastCheckError,
		}
	}
	return items, nil
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
		SetStatus(monitoreddomain.StatusUnknown).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s", i18n.T("error.create_monitored_domain_failed", "Error", err))
	}
	return toItem(m), nil
}

// Update 更新监控域名
func (s *MonitorService) Update(ctx context.Context, id int, input UpdateInput) (*MonitoredDomainItem, error) {
	m, err := s.db.MonitoredDomain.UpdateOneID(id).
		SetDomain(input.Domain).
		SetPort(input.Port).
		SetCheckType(monitoreddomain.CheckType(input.CheckType)).
		SetURL(input.URL).
		SetCheckInterval(input.CheckInterval).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s", i18n.T("error.update_monitored_domain_failed", "Error", err))
	}
	return toItem(m), nil
}

// SetActive 设置监控域名的启用状态
func (s *MonitorService) SetActive(ctx context.Context, id int, active bool) (*MonitoredDomainItem, error) {
	if _, err := s.db.MonitoredDomain.Get(ctx, id); err != nil {
		return nil, err
	}
	updated, err := s.db.MonitoredDomain.UpdateOneID(id).
		SetEnabled(active).
		Save(ctx)
	if err != nil {
		return nil, err
	}
	// 重新启用时清除该域名的告警基线，使下次检查若异常可重新通知（重新布防）
	if active {
		s.notifMu.Lock()
		delete(s.lastNotified, id)
		s.notifMu.Unlock()
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
	s.createCheckLog(ctx, m, result)

	updated, err := s.db.MonitoredDomain.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	s.notifyIfProblem(m, result)
	// 手动检查完成：发射单域名更新事件，前端实时刷新该域名数据与趋势图
	s.emitUpdated(m.ID, m.Domain, result.status)
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
	logging.Debug(i18n.T("log.monitor_check_start", "Domain", m.Domain, "Type", m.CheckType))
	var result checkResult
	switch m.CheckType {
	case "http":
		result = s.checkHTTP(m)
	default:
		result = s.checkHTTPS(m)
	}
	if result.status == "ok" {
		logging.Debug(i18n.T("log.monitor_check_ok", "Domain", m.Domain, "Status", result.status, "Time", result.responseTimeMs, "ms"))
	} else {
		logging.Warn(i18n.T("log.monitor_check_warn", "Domain", m.Domain, "Status", result.status, "Error", result.checkError))
	}
	return result
}

// getHTTPClient 获取配置了 DNS 和代理的 HTTP 客户端（按设置缓存，复用连接池）
func (s *MonitorService) getHTTPClient() *http.Client {
	if s.clientCache != nil {
		return s.clientCache.Get()
	}
	return &http.Client{Timeout: 10 * time.Second}
}

// checkHTTPS 通过 HTTPS 检查接口健康 + SSL 证书信息
func (s *MonitorService) checkHTTPS(m *ent.MonitoredDomain) checkResult {
	result := checkResult{status: "error"}

	url := m.URL
	if url == "" {
		// 未指定 URL 时按域名+端口拼接（端口为默认 443 时省略，保持 URL 简洁）
		if m.Port > 0 && m.Port != 443 {
			url = fmt.Sprintf("https://%s:%d", m.Domain, m.Port)
		} else {
			url = fmt.Sprintf("https://%s", m.Domain)
		}
	}

	client := s.getHTTPClient()

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		result.checkError = err.Error()
		return result
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
		result.certIssuer = certIssuerName(leaf)
		result.certNotBefore = leaf.NotBefore
		result.certNotAfter = leaf.NotAfter
		result.certFingerprint = fmt.Sprintf("%x", fp)
		result.certSubject = certSubjectName(leaf)
		result.certSignatureAlgo = leaf.SignatureAlgorithm.String()
		result.certPublicKeyAlgo = leaf.PublicKeyAlgorithm.String()
		result.certPublicKeyBits = getPubKeyBits(leaf)
		result.certSANs = leaf.DNSNames
		result.certRemainingDays = int(time.Until(leaf.NotAfter).Hours() / 24)

		// SSL 证书过期降级为 expired 状态（启用 expired 枚举，与通知分支对应）
		if result.certRemainingDays <= 0 {
			result.status = "expired"
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
		// 未指定 URL 时按域名+端口拼接。
		// 80 为 HTTP 默认端口省略；443 为旧版遗留默认值（旧版端口不可配、统一存 443），
		// 同样按默认端口处理，避免旧 HTTP 监控被误拼成 :443 导致检查失败。
		if m.Port > 0 && m.Port != 80 && m.Port != 443 {
			url = fmt.Sprintf("http://%s:%d", m.Domain, m.Port)
		} else {
			url = fmt.Sprintf("http://%s", m.Domain)
		}
	}

	client := s.getHTTPClient()

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		result.checkError = err.Error()
		return result
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
		result.certIssuer = certIssuerName(leaf)
		result.certNotBefore = leaf.NotBefore
		result.certNotAfter = leaf.NotAfter
		result.certFingerprint = fmt.Sprintf("%x", fp)
		result.certSubject = certSubjectName(leaf)
		result.certSANs = leaf.DNSNames
		result.certRemainingDays = int(time.Until(leaf.NotAfter).Hours() / 24)

		// 与 HTTPS 检查保持一致：重定向到 HTTPS 场景下证书过期/临期同样降级状态
		if result.certRemainingDays <= 0 {
			result.status = "expired"
		} else if result.certRemainingDays <= 30 && result.status == "ok" {
			result.status = "warning"
		}
	}

	return result
}

// Start 启动后台监控循环
func (s *MonitorService) Start() {
	logging.Info(i18n.T("log.monitor_started"))
	s.loadNotifyBaseline()
	go s.monitorLoop()
}

// loadNotifyBaseline 从数据库加载各域名当前状态作为告警去重基线，
// 避免应用重启后对重启前已通知过的异常状态重复告警。
func (s *MonitorService) loadNotifyBaseline() {
	domains, err := s.db.MonitoredDomain.Query().All(context.Background())
	if err != nil {
		logging.Error("%s", i18n.T("log.monitor_query_failed", "Error", err))
		return
	}
	s.notifMu.Lock()
	defer s.notifMu.Unlock()
	for _, m := range domains {
		s.lastNotified[m.ID] = m.Status.String()
	}
}

// Stop 停止后台监控。幂等：重复调用安全（如 updater 提前清理与 ServiceShutdown 各调一次）。
func (s *MonitorService) Stop() {
	logging.Info(i18n.T("log.monitor_stopped"))
	s.stopOnce.Do(func() {
		close(s.stopChan)
	})
}

func (s *MonitorService) monitorLoop() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	// 每天清理一次过期的检查历史
	cleanupTicker := time.NewTicker(24 * time.Hour)
	defer cleanupTicker.Stop()

	// 启动时先执行一次清理
	s.cleanupHistory()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.runChecks()
		case <-cleanupTicker.C:
			s.cleanupHistory()
		}
	}
}

// historyRetentionDays 返回检查历史保留天数（来自设置，默认 90 天）
func (s *MonitorService) historyRetentionDays() int {
	days := 90
	if s.settingsProvider != nil {
		if d := s.settingsProvider().MonitorHistoryDays; d > 0 {
			days = d
		}
	}
	return days
}

// cleanupHistory 清理超过保留期限的检查历史记录
func (s *MonitorService) cleanupHistory() {
	days := s.historyRetentionDays()
	before := time.Now().AddDate(0, 0, -days)
	n, err := s.db.MonitorCheckLog.Delete().
		Where(monitorchecklog.CheckedAtLT(before)).
		Exec(context.Background())
	if err != nil {
		logging.Error(i18n.T("log.monitor_checklog_cleanup_failed", "Error", err.Error()))
		return
	}
	if n > 0 {
		logging.Info(i18n.T("log.monitor_checklog_cleanup", "Count", n, "Days", days))
	}
}

func (s *MonitorService) runChecks() {
	ctx := context.Background()
	domains, err := s.db.MonitoredDomain.Query().
		Where(monitoreddomain.Enabled(true)).
		All(ctx)
	if err != nil {
		logging.Error("%s", i18n.T("log.monitor_query_failed", "Error", err))
		return
	}

	// 先筛选出本轮到期待检查的域名，再交给并发执行，避免逐个串行等待 HTTP 超时
	now := time.Now()
	due := make([]*ent.MonitoredDomain, 0, len(domains))
	for _, m := range domains {
		if m.CheckInterval <= 0 {
			continue
		}
		nextCheck := m.LastCheckAt.Add(time.Duration(m.CheckInterval) * time.Second)
		if now.Before(nextCheck) {
			continue
		}
		due = append(due, m)
	}
	if len(due) == 0 {
		return
	}

	s.checkDomains(due, now)
	logging.Debug(i18n.T("log.monitor_check_done", "Count", len(due)))
	// 发射批量更新事件，通知前端实时刷新
	s.emitUpdated(0, "", "")
}

// checkDomains 以有界并发执行一批域名的检查（信号量 worker pool + 启动抖动）。
// ent Client 可安全并发使用；通知发送在 notifyIfProblem 内经 notifySendMu 串行化。
func (s *MonitorService) checkDomains(domains []*ent.MonitoredDomain, now time.Time) {
	ctx := context.Background()
	var wg sync.WaitGroup
	sem := make(chan struct{}, maxConcurrentChecks)
	for _, m := range domains {
		wg.Add(1)
		go func(m *ent.MonitoredDomain) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			// 启动抖动：错开同一 tick 内各检查的发起时刻。
			// 选型 math/rand/v2：非安全随机的现代 API，用户态 PRNG 无系统调用，
			// 纳秒级、并发安全；抖动不涉及令牌/密钥，无需 crypto/rand 的不可预测性。
			//nolint:gosec // G404：请求抖动无安全敏感性；生产代码非安全随机需显式声明理由
			if jitter := time.Duration(rand.Int64N(int64(checkJitterMax))); jitter > 0 {
				time.Sleep(jitter)
			}

			result := s.checkDomain(m)

			if _, err := s.db.MonitoredDomain.UpdateOneID(m.ID).
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
				Save(ctx); err != nil {
				logging.Error("%s", i18n.T("log.monitor_update_failed", "Domain", m.Domain, "Error", err))
			}
			s.createCheckLog(ctx, m, result)

			s.notifyIfProblem(m, result)
		}(m)
	}
	wg.Wait()
}

// notifyIfProblem 在状态发生变化时发送通知（异常/过期/警告 + 恢复通知）。
// 通过 lastNotified 基线去重：同一异常状态每轮检查只通知一次，
// 从异常恢复为正常时发送恢复通知，避免告警风暴。
func (s *MonitorService) notifyIfProblem(m *ent.MonitoredDomain, result checkResult) {
	if s.notifService == nil {
		return
	}

	s.notifMu.RLock()
	prev, loaded := s.lastNotified[m.ID]
	s.notifMu.RUnlock()

	// 状态未变化：抑制重复告警（告警风暴修复的核心判断）
	if loaded && prev == result.status {
		return
	}

	title, body, notify := buildMonitorNotification(m.Domain, result, prev, loaded)

	// 无论本次是否发送通知，都更新基线为当前状态
	s.notifMu.Lock()
	s.lastNotified[m.ID] = result.status
	s.notifMu.Unlock()

	if !notify {
		return
	}

	level := entnotification.LevelInfo.String()
	switch result.status {
	case "error", "expired":
		level = entnotification.LevelError.String()
	case "warning":
		level = entnotification.LevelWarning.String()
	case "ok":
		level = entnotification.LevelSuccess.String()
	}

	// 串行化发送：后台检查并发执行时，避免多 goroutine 并发调用通知服务
	s.notifySendMu.Lock()
	err := s.notifService.SendNotification(notification.NotificationOption{
		Title:    title,
		Body:     body,
		Category: "monitor",
		Level:    level,
	})
	s.notifySendMu.Unlock()
	_ = err
}

// buildMonitorNotification 根据检查结果与上次通知状态构造通知内容。
// 返回值 notify 为 false 表示本次不需要发送通知（如首次检查即正常、未知状态）。
// prev/loaded 为上次已通知的状态及是否存在基线。
func buildMonitorNotification(domain string, result checkResult, prev string, loaded bool) (title, body string, notify bool) {
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
		return title, body, true
	case "expired":
		title = i18n.T("notification.monitor_expired.title")
		body = i18n.T("notification.monitor_expired.body", "Domain", domain)
		if result.certIssuer != "" {
			body += fmt.Sprintf(" (%s)", result.certIssuer)
		}
		return title, body, true
	case "warning":
		if result.certRemainingDays > 0 && result.certRemainingDays <= 30 {
			title = i18n.T("notification.monitor_expiring.title")
			body = i18n.T("notification.monitor_expiring.body", "Domain", domain, "Days", result.certRemainingDays)
			if result.certIssuer != "" {
				body += fmt.Sprintf(" (%s)", result.certIssuer)
			}
			return title, body, true
		}
		if result.httpStatusCode >= 400 {
			title = i18n.T("notification.monitor_http_error.title")
			body = i18n.T("notification.monitor_http_error.body", "Domain", domain, "Code", result.httpStatusCode)
			return title, body, true
		}
		return "", "", false
	case "ok":
		// 恢复通知：此前处于异常状态，本次恢复正常才通知；首次检查即正常不打扰
		if loaded && (prev == "error" || prev == "warning" || prev == "expired") {
			title = i18n.T("notification.monitor_recovered.title")
			body = i18n.T("notification.monitor_recovered.body", "Domain", domain)
			return title, body, true
		}
		return "", "", false
	default:
		return "", "", false
	}
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

// certIssuerName 提取证书颁发者展示名：CN → Organization → 空串。
// 现代 CA（如 Let's Encrypt）签发的证书普遍不再写 CommonName，
// 只取 CN 会让监控列表的「颁发者」整列空白，因此需要按字段依次兜底。
func certIssuerName(cert *x509.Certificate) string {
	if cert.Issuer.CommonName != "" {
		return cert.Issuer.CommonName
	}
	for _, o := range cert.Issuer.Organization {
		if o != "" {
			return o
		}
	}
	return ""
}

// certSubjectName 提取证书主体展示名：CN → 首个 SAN → Organization → 空串。
// 对证书主体而言，域名（SAN）比组织名更有辨识度，故 SAN 优先于 Organization。
func certSubjectName(cert *x509.Certificate) string {
	if cert.Subject.CommonName != "" {
		return cert.Subject.CommonName
	}
	for _, d := range cert.DNSNames {
		if d != "" {
			return d
		}
	}
	for _, o := range cert.Subject.Organization {
		if o != "" {
			return o
		}
	}
	return ""
}
