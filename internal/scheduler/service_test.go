package scheduler

import (
	"path/filepath"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/google/uuid"

	"cnb.cool/dtapp/certflow/internal/certificate"
	"cnb.cool/dtapp/certflow/internal/ent"
	entcert "cnb.cool/dtapp/certflow/internal/ent/certificate"
	"cnb.cool/dtapp/certflow/internal/ent/enttest"
	"cnb.cool/dtapp/certflow/internal/ent/renewallog"
	"cnb.cool/dtapp/certflow/internal/logging"
	"cnb.cool/dtapp/certflow/internal/settings"
	"cnb.cool/dtapp/certflow/internal/sqlite"
)

type schedFixture struct {
	scheduler *Scheduler
	client    *ent.Client
	certSvc   *certificate.CertificateService
	dataDir   string
}

// newSchedFixture 建临时库 + 临时目录 + 真实的 certificate/settings 服务。
//
// 这两个后台任务之所以能离线测：库里没有待续期/待过期证书时它们只做查询就走完；
// 一旦有证书就会真的去连 ACME 服务器，所以只覆盖「无事可做」与「总闸关闭」两条路。
func newSchedFixture(t *testing.T) *schedFixture {
	t.Helper()
	dir := t.TempDir()
	client := enttest.Open(t, dialect.SQLite, sqlite.BuildDSN(filepath.Join(dir, "sched.db")))
	t.Cleanup(func() { _ = client.Close() })

	certDir := filepath.Join(dir, "certs")
	settingsSvc, err := settings.NewService(dir)
	if err != nil {
		t.Fatalf("settings.NewService: %v", err)
	}

	certSvc := certificate.NewCertificateService(client, certDir)
	s := NewScheduler(client, certSvc, nil, settingsSvc, certDir)

	return &schedFixture{scheduler: s, client: client, certSvc: certSvc, dataDir: dir}
}

func (f *schedFixture) seedCert(t *testing.T, domain string, autoRenew bool, notAfter time.Duration) *ent.Certificate {
	t.Helper()
	return f.client.Certificate.Create().
		SetDomain(domain).
		SetSans([]string{domain}).
		SetNotBefore(time.Now().Add(-time.Hour)).
		SetNotAfter(time.Now().Add(notAfter)).
		SetStatus(entcert.StatusActive).
		SetAutoRenew(autoRenew).
		SetRenewalDays(30).
		SaveX(t.Context())
}

func TestGetLogDir(t *testing.T) {
	f := newSchedFixture(t)
	// dataDir 目前与 certDir 取同一个值，日志落在 <certDir>/logs
	if got, want := f.scheduler.getLogDir(), filepath.Join(f.scheduler.certDir, "logs"); got != want {
		t.Fatalf("日志目录 = %q, want %q", got, want)
	}
}

// TestGetJob 未启动（或已停止）时没有任何 job：
// 非法 ID 与「合法但不存在」的 ID 都必须返回错误 —— 上层接口靠这个判断 job 是否还活着。
func TestGetJob(t *testing.T) {
	f := newSchedFixture(t)

	if _, err := f.scheduler.GetJob("not-a-uuid"); err == nil {
		t.Fatal("非法 job ID 应返回错误")
	}
	// 未 Start 时必须是错误而不是 panic
	if _, err := f.scheduler.GetJob(uuid.New().String()); err == nil {
		t.Fatal("不存在的 job 应返回错误")
	}
}

// TestRunAutoRenew_NoCerts 覆盖三个提前返回点：总闸关闭、无证书、settingsService 为 nil。
func TestRunAutoRenew_NoCerts(t *testing.T) {
	t.Run("总闸关闭时直接返回", func(t *testing.T) {
		f := newSchedFixture(t)
		cfg := f.scheduler.settingsService.Get()
		cfg.AutoRenewEnabled = false
		if err := f.scheduler.settingsService.Save(cfg); err != nil {
			t.Fatalf("保存设置失败: %v", err)
		}

		done := make(chan struct{})
		go func() { f.scheduler.runAutoRenew(false); close(done) }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("总闸关闭时 runAutoRenew 应立即返回")
		}
	})

	t.Run("无待续期证书时正常结束", func(t *testing.T) {
		f := newSchedFixture(t)
		f.scheduler.runAutoRenew(true) // force 绕过总闸
	})

	t.Run("settingsService 为 nil 时不 panic", func(t *testing.T) {
		f := newSchedFixture(t)
		f.scheduler.settingsService = nil
		f.scheduler.runAutoRenew(false)
	})
}

// TestExpiryCheck_NoCerts 过期检查在「通知不可用」和「没有临期证书」时都不能出错。
func TestExpiryCheck_NoCerts(t *testing.T) {
	// 注：不能传真实的 NotificationService —— CheckPermission() 会调用 Wails 的通知服务，
	// 在测试进程里（无 app runtime）直接 panic。这条分支只能在集成环境验证。

	t.Run("notifService 为 nil", func(t *testing.T) {
		f := newSchedFixture(t)
		f.scheduler.notifService = nil
		f.scheduler.expiryCheckTask()
	})

	t.Run("手动触发入口", func(t *testing.T) {
		f := newSchedFixture(t)
		f.scheduler.RunExpiryCheckNow()
	})
}

// TestRenewalLogs_EdgesPreloaded 旧用例只查了条数，这里补上「证书关联必须预加载」：
// 前端「续期记录」列表直接读 log.certificate.domain，没预加载就整列空白。
func TestRenewalLogs_EdgesPreloaded(t *testing.T) {
	f := newSchedFixture(t)
	ctx := t.Context()

	cert := f.seedCert(t, "a.example.com", true, 10*24*time.Hour)
	for i, st := range []renewallog.Status{renewallog.StatusFailed, renewallog.StatusSuccess} {
		f.client.RenewalLog.Create().
			SetCertificateID(cert.ID).
			SetStatus(st).
			SetAttemptAt(time.Now().Add(time.Duration(i) * time.Minute)).
			SaveX(ctx)
	}

	logs, err := f.scheduler.GetRenewalLogs(ctx, cert.ID)
	if err != nil {
		t.Fatalf("GetRenewalLogs err: %v", err)
	}
	for _, l := range logs {
		if l.Edges.Certificate == nil || l.Edges.Certificate.Domain != "a.example.com" {
			t.Fatalf("GetRenewalLogs 未预加载证书关联: %+v", l.Edges.Certificate)
		}
	}

	recent, err := f.scheduler.GetRecentRenewalLogs(ctx, 2)
	if err != nil {
		t.Fatalf("GetRecentRenewalLogs err: %v", err)
	}
	if len(recent) != 2 {
		t.Fatalf("limit=2 应返回 2 条, 实际 %d", len(recent))
	}
	if !recent[0].AttemptAt.After(recent[1].AttemptAt) {
		t.Fatalf("应按 attempt_at 倒序: %v / %v", recent[0].AttemptAt, recent[1].AttemptAt)
	}
	for _, l := range recent {
		if l.Edges.Certificate == nil {
			t.Fatal("GetRecentRenewalLogs 未预加载证书关联")
		}
	}
}

// TestLogJobEvent 两种日志后端：gocron 专属 logger（Start 之后）与全局 logger（Start 之前）。
func TestLogJobEvent(t *testing.T) {
	f := newSchedFixture(t)
	jobID := uuid.New()

	// Start 之前 gocronLog 为 nil → 走全局 logging，不能 panic
	f.scheduler.gocronLog = nil
	f.scheduler.logJobEvent("log.manual_renewal_triggered", jobID, "auto-renew", "info", nil)
	f.scheduler.logJobEvent("log.manual_renewal_triggered", jobID, "auto-renew", "error", nil)
	f.scheduler.logJobEvent("log.manual_renewal_triggered", jobID, "auto-renew", "error", "boom")

	// 模拟 Start 之后的 logger
	lg, err := logging.NewLoggerWithFilename(f.scheduler.getLogDir(), "gocron.log", logging.INFO, 10, 3)
	if err != nil {
		t.Fatalf("创建 logger 失败: %v", err)
	}
	f.scheduler.gocronLog = &gocronLogger{logger: lg}
	f.scheduler.logJobEvent("log.manual_renewal_triggered", jobID, "auto-renew", "info", nil)
	f.scheduler.logJobEvent("log.manual_renewal_triggered", jobID, "auto-renew", "error", nil)
}
