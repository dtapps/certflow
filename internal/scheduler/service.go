package scheduler

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"cnb.cool/dtapp/certflow/ent"
	"cnb.cool/dtapp/certflow/ent/renewallog"
	"cnb.cool/dtapp/certflow/internal/certificate"
	"cnb.cool/dtapp/certflow/internal/i18n"
	"cnb.cool/dtapp/certflow/internal/logging"
	"cnb.cool/dtapp/certflow/internal/notification"
	"cnb.cool/dtapp/certflow/internal/settings"
	"entgo.io/ent/dialect/sql"
	"github.com/go-co-op/gocron/v2"
	"github.com/google/uuid"
)

// gocronLogger 适配 gocron Logger 接口
type gocronLogger struct {
	logger *logging.Logger
}

func (l *gocronLogger) Debug(msg string, args ...any) {
	l.logger.Debug(msg, args...)
}

func (l *gocronLogger) Info(msg string, args ...any) {
	l.logger.Info(msg, args...)
}

func (l *gocronLogger) Warn(msg string, args ...any) {
	l.logger.Warn(msg, args...)
}

func (l *gocronLogger) Error(msg string, args ...any) {
	l.logger.Error(msg, args...)
}

// GetJob 根据任务 ID 获取对应的 gocron 任务
func (s *Scheduler) GetJob(jobID string) (gocron.Job, error) {
	id, err := uuid.Parse(jobID)
	if err != nil {
		return nil, fmt.Errorf("invalid job id %s: %w", jobID, err)
	}
	for _, job := range s.scheduler.Jobs() {
		if job.ID() == id {
			return job, nil
		}
	}
	return nil, fmt.Errorf("job not found: %s", jobID)
}

// 任务开始前
func (s *Scheduler) beforePeriodicListener() gocron.EventListener {
	return gocron.BeforeJobRuns(func(jobID uuid.UUID, jobName string) {
		s.logJobEvent("log.job_before_run", jobID, jobName, "info", nil)
	})
}

// 任务结束后
func (s *Scheduler) afterPeriodicListener() gocron.EventListener {
	return gocron.AfterJobRuns(func(jobID uuid.UUID, jobName string) {
		s.logJobEvent("log.job_after_run", jobID, jobName, "info", nil)
	})
}

// 任务异常时
func (s *Scheduler) panicPeriodicListener() gocron.EventListener {
	return gocron.AfterJobRunsWithPanic(func(jobID uuid.UUID, jobName string, recoverData any) {
		s.logJobEvent("log.job_panic", jobID, jobName, "error", recoverData)
	})
}

// logJobEvent 统一通过 gocron 日志器打印任务事件（含上次/下次运行时间）
func (s *Scheduler) logJobEvent(key string, jobID uuid.UUID, jobName string, level string, recoverData any) {
	lastRun, nextRun := "-", "-"
	if job, err := s.GetJob(jobID.String()); err == nil {
		if t, e := job.LastRunStartedAt(); e == nil && !t.IsZero() {
			lastRun = t.Format(time.DateTime)
		}
		if t, e := job.NextRun(); e == nil && !t.IsZero() {
			nextRun = t.Format(time.DateTime)
		}
	}
	args := []any{
		"Job", jobName,
		"ID", jobID.String(),
		"LastRun", lastRun,
		"NextRun", nextRun,
	}
	if recoverData != nil {
		args = append(args, "Error", fmt.Sprintf("%v", recoverData))
	}
	msg := i18n.T(key, args...)
	if s.gocronLog != nil {
		if level == "error" {
			s.gocronLog.Error(msg)
		} else {
			s.gocronLog.Info(msg)
		}
	} else {
		if level == "error" {
			logging.Error(msg)
		} else {
			logging.Info(msg)
		}
	}
}

// Scheduler 提供定时任务调度功能
type Scheduler struct {
	db              *ent.Client
	certService     *certificate.CertificateService
	notifService    *notification.NotificationService
	settingsService *settings.Service
	scheduler       gocron.Scheduler
	gocronLog       *gocronLogger
	certDir         string
	dataDir         string
}

// NewScheduler 创建新的调度器
func NewScheduler(client *ent.Client, certService *certificate.CertificateService, notifService *notification.NotificationService, settingsService *settings.Service, certDir string) *Scheduler {
	return &Scheduler{
		db:              client,
		certService:     certService,
		notifService:    notifService,
		settingsService: settingsService,
		certDir:         certDir,
		dataDir:         certDir,
	}
}

// getLogDir 获取日志目录
func (s *Scheduler) getLogDir() string {
	return filepath.Join(s.dataDir, "logs")
}

// Start 启动调度器
func (s *Scheduler) Start(ctx context.Context) error {
	// 创建 gocron 独立日志记录器（使用全局日志配置）
	gocronLogDir := s.getLogDir()
	var logger *logging.Logger
	if logging.Global() != nil {
		logger, _ = logging.NewLoggerWithFilename(gocronLogDir, "gocron.log",
			logging.Global().GetLevel(),
			logging.Global().GetMaxSize(),
			logging.Global().GetMaxBackups())
	}
	if logger == nil {
		logger = logging.Global()
	}

	// 创建 gocron 日志适配器
	gocronLog := &gocronLogger{
		logger: logger,
	}
	s.gocronLog = gocronLog

	// 创建 gocron 调度器
	scheduler, err := gocron.NewScheduler(
		gocron.WithLogger(gocronLog),
		gocron.WithLocation(time.Local),
	)
	if err != nil {
		return fmt.Errorf("%s", i18n.T("error.create_scheduler_failed", "Error", err))
	}
	s.scheduler = scheduler

	// 从设置读取任务间隔与开关（带兜底默认值，避免设置异常导致间隔为 0）
	renewInterval := 1
	checkInterval := 6
	autoCheckExpiry := true
	if s.settingsService != nil {
		st := s.settingsService.Get()
		if st.RenewInterval > 0 {
			renewInterval = st.RenewInterval
		}
		if st.CheckInterval > 0 {
			checkInterval = st.CheckInterval
		}
		autoCheckExpiry = st.AutoCheckExpiry
	}

	// 添加自动续期任务：按设置间隔检查
	_, err = s.scheduler.NewJob(
		gocron.DurationJob(time.Duration(renewInterval)*time.Hour),
		gocron.NewTask(s.autoRenewTask),
		gocron.WithName("auto_renew_certificates"),
		gocron.WithIdentifier(func() uuid.UUID { id, _ := uuid.NewV7(); return id }()),
		gocron.WithEventListeners(
			s.panicPeriodicListener(),
			s.beforePeriodicListener(),
			s.afterPeriodicListener(),
		),
	)
	if err != nil {
		return fmt.Errorf("%s", i18n.T("error.add_renew_job_failed", "Error", err))
	}

	// 仅当开启自动检查过期时，注册证书过期检查任务（开关关闭则不注册，修改需重启生效）
	if autoCheckExpiry {
		_, err = s.scheduler.NewJob(
			gocron.DurationJob(time.Duration(checkInterval)*time.Hour),
			gocron.NewTask(s.expiryCheckTask),
			gocron.WithName("check_expiring_certificates"),
			gocron.WithIdentifier(func() uuid.UUID { id, _ := uuid.NewV7(); return id }()),
			gocron.WithEventListeners(
				s.panicPeriodicListener(),
				s.beforePeriodicListener(),
				s.afterPeriodicListener(),
			),
		)
		if err != nil {
			return fmt.Errorf("%s", i18n.T("error.add_expiry_check_job_failed", "Error", err))
		}
	}

	// 启动调度器
	s.scheduler.Start()

	logging.Info(i18n.T("log.scheduler_started"))
	return nil
}

// Stop 停止调度器
func (s *Scheduler) Stop() error {
	if s.scheduler != nil {
		if err := s.scheduler.Shutdown(); err != nil {
			return fmt.Errorf("%s", i18n.T("error.stop_scheduler_failed", "Error", err))
		}
	}
	logging.Info(i18n.T("log.scheduler_stopped"))
	return nil
}

// autoRenewTask 自动续期任务
func (s *Scheduler) autoRenewTask() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	logging.Info(i18n.T("log.renewal_task_start"))

	// 获取需要自动续期的证书
	certs, err := s.certService.ListAutoRenew(ctx)
	if err != nil {
		logging.Error(i18n.T("log.get_auto_renew_certs_failed", "Error", err))
		return
	}

	if len(certs) == 0 {
		logging.Info(i18n.T("log.renewal_no_certs"))
		return
	}

	logging.Info(i18n.T("log.renewal_found_certs", "Count", len(certs)))

	for _, cert := range certs {
		// 创建续期日志
		logEntry, err := s.db.RenewalLog.Create().
			SetCertificateID(cert.ID).
			SetStatus("in_progress").
			SetAttemptAt(time.Now()).
			Save(ctx)
		if err != nil {
			logging.Error(i18n.T("log.create_renewal_log_failed", "ID", cert.ID, "Error", err))
			continue
		}

		// 执行续期
		result, err := s.certService.RenewCertificate(ctx, cert.ID)
		if err != nil {
			logging.Error(i18n.T("log.renewal_failed", "ID", cert.ID, "Error", err))
			// 更新续期日志为失败
			_, _ = s.db.RenewalLog.UpdateOneID(logEntry.ID).
				SetStatus("failed").
				SetErrorMessage(err.Error()).
				SetCompletedAt(time.Now()).
				Save(ctx)
			// 发送失败通知
			if s.notifService != nil && s.notifService.CheckPermission() {
				_ = s.notifService.SendCertRenewFailed(cert.Domain, err.Error())
			}
			continue
		}

		// 更新续期日志为成功
		_, err = s.db.RenewalLog.UpdateOneID(logEntry.ID).
			SetStatus("success").
			SetCertContent(result.CertContent).
			SetKeyContent(result.KeyContent).
			SetCompletedAt(time.Now()).
			Save(ctx)
		if err != nil {
			logging.Error(i18n.T("log.update_renewal_log_failed", "ID", cert.ID, "Error", err))
		}

		logging.Info(i18n.T("log.renewal_success", "ID", cert.ID, "Domain", cert.Domain))

		// 发送续期成功通知
		if s.notifService != nil && s.notifService.CheckPermission() {
			_ = s.notifService.SendCertRenewed(cert.Domain, result.Issuer, result.NotAfter)
		}
	}

	logging.Info(i18n.T("log.renewal_task_complete"))
}

// expiryCheckTask 证书过期检查任务
func (s *Scheduler) expiryCheckTask() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	logging.Info(i18n.T("log.expiry_check_start"))

	// 检查通知权限
	if s.notifService != nil && !s.notifService.CheckPermission() {
		logging.Info(i18n.T("log.notification_disabled"))
		return
	}

	now := time.Now()

	// 获取7天内过期的证书
	certs, err := s.certService.ListExpiring(ctx, 7)
	if err != nil {
		logging.Error(i18n.T("log.get_expiring_certs_failed", "Error", err))
		return
	}

	if len(certs) > 0 {
		logging.Warn(i18n.T("log.expiry_found_certs", "Count", len(certs)))
		for _, cert := range certs {
			daysLeft := int(cert.NotAfter.Sub(now).Hours() / 24)
			logging.Warn(i18n.T("log.cert_expiring_detail",
				"ID", cert.ID, "Domain", cert.Domain,
				"Expiry", cert.NotAfter.Format(time.DateOnly), "Days", daysLeft))
			if s.notifService != nil {
				if err := s.notifService.SendCertExpiring(cert.Domain, daysLeft); err != nil {
					logging.Error(i18n.T("log.expiring_notification_failed", "ID", cert.ID, "Error", err))
				}
			}
		}
	}

	// 获取30天内过期的证书（仅发送一次通知，不重复发送7天内的）
	certs30, err := s.certService.ListExpiring(ctx, 30)
	if err != nil {
		logging.Error(i18n.T("log.get_30day_expiry_failed", "Error", err))
		return
	}

	if len(certs30) > len(certs) {
		logging.Info(i18n.T("log.total_30day_expiring", "Count", len(certs30)))
	}

	logging.Info(i18n.T("log.expiry_check_complete"))
}

// GetRenewalLogs 获取续期日志
func (s *Scheduler) GetRenewalLogs(ctx context.Context, certID int) ([]*ent.RenewalLog, error) {
	logging.Debug(i18n.T("log.renewal_logs_query", "CertID", certID))
	// 通过 certificate edge 查询
	results, err := s.db.RenewalLog.Query().
		Where(func(s *sql.Selector) {
			s.Where(sql.EQ(renewallog.CertificateColumn, certID))
		}).
		WithCertificate().
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s", i18n.T("error.get_renewal_logs_failed", "Error", err))
	}
	return results, nil
}

// GetRecentRenewalLogs 获取最近的续期日志
func (s *Scheduler) GetRecentRenewalLogs(ctx context.Context, limit int) ([]*ent.RenewalLog, error) {
	results, err := s.db.RenewalLog.Query().
		Order(ent.Desc(renewallog.FieldAttemptAt)).
		Limit(limit).
		WithCertificate().
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s", i18n.T("error.get_renewal_logs_failed", "Error", err))
	}
	return results, nil
}

// RunRenewalNow 立即执行一次续期检查（手动触发）
func (s *Scheduler) RunRenewalNow() {
	logging.Info(i18n.T("log.manual_renewal_triggered"))
	s.autoRenewTask()
}

// RunExpiryCheckNow 立即执行一次过期检查（手动触发）
func (s *Scheduler) RunExpiryCheckNow() {
	logging.Info(i18n.T("log.manual_expiry_triggered"))
	s.expiryCheckTask()
}
