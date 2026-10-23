package scheduler

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	entcert "cnb.cool/dtapp/certflow/internal/ent/certificate"
	"cnb.cool/dtapp/certflow/internal/ent/renewallog"
	"cnb.cool/dtapp/certflow/internal/ent/schema"
	"cnb.cool/dtapp/certflow/internal/logging"
	"cnb.cool/dtapp/certflow/internal/settings"
)

// setupLogging 初始化全局日志，避免 gocron 日志适配器拿到 nil logger。
func setupLogging(t *testing.T) {
	t.Helper()
	if err := logging.InitGlobalLogger(filepath.Join(t.TempDir(), "logs"), "error", 10, 3); err != nil {
		t.Fatalf("初始化日志失败: %v", err)
	}
}

// startAndCleanup 启动调度器并注册清理。
func startAndCleanup(t *testing.T, s *Scheduler) {
	t.Helper()
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start 失败: %v", err)
	}
	t.Cleanup(func() { _ = s.Stop() })
}

// TestStartStop_DefaultJobs 覆盖默认配置下的启动与停止。
func TestStartStop_DefaultJobs(t *testing.T) {
	setupLogging(t)
	f := newSchedFixture(t)
	startAndCleanup(t, f.scheduler)

	if f.scheduler.scheduler == nil {
		t.Fatal("调度器未创建")
	}
	if f.scheduler.gocronLog == nil {
		t.Fatal("gocron 日志适配器未创建")
	}

	// 默认 AutoCheckExpiry=true → 应注册两个任务
	jobs := f.scheduler.scheduler.Jobs()
	if len(jobs) != 2 {
		t.Fatalf("默认应注册 2 个任务，实际 %d", len(jobs))
	}
	names := map[string]bool{}
	for _, job := range jobs {
		names[job.Name()] = true
	}
	if !names["auto_renew_certificates"] || !names["check_expiring_certificates"] {
		t.Fatalf("任务名称不符: %v", names)
	}

	// GetJob 能按 ID 找到任务
	for _, job := range jobs {
		if _, err := f.scheduler.GetJob(job.ID().String()); err != nil {
			t.Fatalf("GetJob(%s) 失败: %v", job.Name(), err)
		}
	}

	// Stop 幂等
	if err := f.scheduler.Stop(); err != nil {
		t.Fatalf("Stop 失败: %v", err)
	}
	if err := f.scheduler.Stop(); err != nil {
		t.Fatalf("重复 Stop 不应报错: %v", err)
	}
}

// TestStart_AutoCheckExpiryDisabled 关闭过期检查时只注册续期任务。
func TestStart_AutoCheckExpiryDisabled(t *testing.T) {
	setupLogging(t)
	f := newSchedFixture(t)

	st := f.scheduler.settingsService.Get()
	st.AutoCheckExpiry = false
	if err := f.scheduler.settingsService.Save(st); err != nil {
		t.Fatalf("保存设置失败: %v", err)
	}

	startAndCleanup(t, f.scheduler)

	jobs := f.scheduler.scheduler.Jobs()
	if len(jobs) != 1 || jobs[0].Name() != "auto_renew_certificates" {
		t.Fatalf("关闭过期检查后应只注册续期任务，实际 %d 个: %v", len(jobs), jobs)
	}
}

// TestStart_CustomIntervals 自定义间隔不影响任务数量（仅影响调度周期）。
func TestStart_CustomIntervals(t *testing.T) {
	setupLogging(t)
	f := newSchedFixture(t)

	st := f.scheduler.settingsService.Get()
	st.RenewInterval = 12
	st.CheckInterval = 24
	if err := f.scheduler.settingsService.Save(st); err != nil {
		t.Fatalf("保存设置失败: %v", err)
	}

	startAndCleanup(t, f.scheduler)
	if n := len(f.scheduler.scheduler.Jobs()); n != 2 {
		t.Fatalf("应注册 2 个任务，实际 %d", n)
	}
}

// TestStart_NilSettingsService 无设置服务时使用内置默认间隔。
func TestStart_NilSettingsService(t *testing.T) {
	setupLogging(t)
	f := newSchedFixture(t)
	f.scheduler.settingsService = nil

	startAndCleanup(t, f.scheduler)
	if n := len(f.scheduler.scheduler.Jobs()); n != 2 {
		t.Fatalf("默认应注册 2 个任务，实际 %d", n)
	}
}

// TestStart_ZeroIntervals 设置中的间隔为 0 时回落到默认值（避免 0 间隔）。
func TestStart_ZeroIntervals(t *testing.T) {
	setupLogging(t)
	f := newSchedFixture(t)

	st := f.scheduler.settingsService.Get()
	st.RenewInterval = 0
	st.CheckInterval = 0
	if err := f.scheduler.settingsService.Save(st); err != nil {
		t.Fatalf("保存设置失败: %v", err)
	}

	startAndCleanup(t, f.scheduler)
	if n := len(f.scheduler.scheduler.Jobs()); n != 2 {
		t.Fatalf("应注册 2 个任务，实际 %d", n)
	}
}

// TestRunRenewalNow_NoCerts 覆盖手动续期在无待续期证书时的行为。
func TestRunRenewalNow_NoCerts(t *testing.T) {
	setupLogging(t)
	f := newSchedFixture(t)
	startAndCleanup(t, f.scheduler)

	// 手动触发忽略全局开关：即使关闭自动续期也会执行
	st := f.scheduler.settingsService.Get()
	st.AutoRenewEnabled = false
	if err := f.scheduler.settingsService.Save(st); err != nil {
		t.Fatalf("保存设置失败: %v", err)
	}

	f.scheduler.RunRenewalNow()

	count, err := f.client.RenewalLog.Query().Count(context.Background())
	if err != nil {
		t.Fatalf("统计续期日志失败: %v", err)
	}
	if count != 0 {
		t.Fatalf("无待续期证书不应产生日志，实际 %d", count)
	}
}

// TestRunRenewalNow_RenewFailure 覆盖续期执行失败时日志落库为 failed。
//
// 这里刻意让证书没有关联 CA：RenewCertificate 会在查询 CA 时立即失败返回，
// 不触碰 ACME 网络，因此可离线稳定复现「续期失败」分支。
func TestRunRenewalNow_RenewFailure(t *testing.T) {
	setupLogging(t)
	f := newSchedFixture(t)
	startAndCleanup(t, f.scheduler)

	cert := f.seedCert(t, "renew-fail.example.com", true, 5*24*time.Hour)
	// ListAutoRenew 只处理「已关联 DNS 提供商」的证书（手动 DNS 证书无法自动续期）
	prov := f.client.DNSProvider.Create().
		SetName("测试 DNS").
		SetProviderType("aliyun").
		SetConfig(schema.DNSProviderConfig{AccessKeyID: "ak", AccessKeySecret: "sk"}).
		SaveX(context.Background())
	if err := f.client.Certificate.UpdateOneID(cert.ID).SetDNSProvider(prov).Exec(context.Background()); err != nil {
		t.Fatalf("关联 DNS 提供商失败: %v", err)
	}

	f.scheduler.RunRenewalNow()

	logs, err := f.client.RenewalLog.Query().All(context.Background())
	if err != nil {
		t.Fatalf("查询续期日志失败: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("应产生 1 条续期日志，实际 %d", len(logs))
	}
	if logs[0].Status != renewallog.StatusFailed {
		t.Fatalf("续期应失败，实际状态 %s", logs[0].Status)
	}
	if logs[0].ErrorMessage == "" {
		t.Fatal("失败日志应记录错误信息")
	}

	// 证书状态应被标记为续期失败（若实现如此），至少不能是 pending
	reloaded, err := f.client.Certificate.Get(context.Background(), cert.ID)
	if err != nil {
		t.Fatalf("查询证书失败: %v", err)
	}
	if reloaded.Status == entcert.StatusPending {
		t.Fatal("失败后证书不应停留在 pending 状态")
	}
}

// TestGocronLogger_Methods 覆盖 gocron 日志适配器的各等级转发。
func TestGocronLogger_Methods(t *testing.T) {
	setupLogging(t)
	logger, err := logging.NewLoggerWithFilename(t.TempDir(), "gocron_test.log", logging.ERROR, 10, 3)
	if err != nil {
		t.Fatalf("创建日志器失败: %v", err)
	}
	l := &gocronLogger{logger: logger}

	// 各等级调用都不应 panic
	l.Debug("debug %s", "msg")
	l.Info("info %s", "msg")
	l.Warn("warn %s", "msg")
	l.Error("error %s", "msg")
}

// TestAutoRenewTask_Disabled 覆盖定时任务入口在总闸关闭时直接返回。
func TestAutoRenewTask_Disabled(t *testing.T) {
	setupLogging(t)
	f := newSchedFixture(t)

	st := f.scheduler.settingsService.Get()
	st.AutoRenewEnabled = false
	if err := f.scheduler.settingsService.Save(st); err != nil {
		t.Fatalf("保存设置失败: %v", err)
	}

	// 有 auto_renew 证书也不应处理（关联 DNS 提供商，确保它本应被 ListAutoRenew 选中）
	cert := f.seedCert(t, "auto-off.example.com", true, 5*24*time.Hour)
	prov := f.client.DNSProvider.Create().
		SetName("测试 DNS").
		SetProviderType("aliyun").
		SetConfig(schema.DNSProviderConfig{AccessKeyID: "ak", AccessKeySecret: "sk"}).
		SaveX(context.Background())
	if err := f.client.Certificate.UpdateOneID(cert.ID).SetDNSProvider(prov).Exec(context.Background()); err != nil {
		t.Fatalf("关联 DNS 提供商失败: %v", err)
	}
	f.scheduler.autoRenewTask()

	count, err := f.client.RenewalLog.Query().Count(context.Background())
	if err != nil {
		t.Fatalf("统计续期日志失败: %v", err)
	}
	if count != 0 {
		t.Fatalf("总闸关闭时不应续期，实际产生 %d 条日志", count)
	}
}

// TestSettingsDefaults 确认默认设置与调度器假设一致。
func TestSettingsDefaults(t *testing.T) {
	dir := t.TempDir()
	svc, err := settings.NewService(dir)
	if err != nil {
		t.Fatalf("settings.NewService 失败: %v", err)
	}
	st := svc.Get()
	if !st.AutoCheckExpiry {
		t.Fatal("默认应开启过期检查")
	}
	if st.RenewInterval <= 0 || st.CheckInterval <= 0 {
		t.Fatalf("默认间隔应为正数: renew=%d check=%d", st.RenewInterval, st.CheckInterval)
	}
}
