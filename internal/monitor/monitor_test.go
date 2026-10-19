package monitor

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cnb.cool/dtapp/certflow/internal/ent"
	"cnb.cool/dtapp/certflow/internal/ent/monitoreddomain"
	"cnb.cool/dtapp/certflow/internal/events"
	"cnb.cool/dtapp/certflow/internal/i18n"
	"cnb.cool/dtapp/certflow/internal/notification"
	"cnb.cool/dtapp/certflow/internal/settings"
	"cnb.cool/dtapp/certflow/internal/sqlite"
	esql "entgo.io/ent/dialect/sql"
)

func setupTestDB(t *testing.T) *ent.Client {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:ent?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatal(err)
	}
	drv := esql.OpenDB("sqlite3", db)
	client := ent.NewClient(ent.Driver(drv))
	t.Cleanup(func() { client.Close() })
	ctx := context.Background()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatal(err)
	}
	return client
}

func TestNewMonitorService(t *testing.T) {
	client := setupTestDB(t)
	svc := NewMonitorService(client)
	if svc == nil {
		t.Fatal("NewMonitorService returned nil")
	}
	if svc.db != client {
		t.Error("db client not set correctly")
	}
}

func TestSetSettingsProvider(t *testing.T) {
	client := setupTestDB(t)
	svc := NewMonitorService(client)

	called := false
	svc.SetSettingsProvider(func() settings.Settings {
		called = true
		return settings.Settings{}
	})

	if svc.settingsProvider == nil {
		t.Fatal("settingsProvider not set")
	}
	svc.settingsProvider()
	if !called {
		t.Error("settingsProvider function not called")
	}
}

func TestSetNotificationService(t *testing.T) {
	client := setupTestDB(t)
	svc := NewMonitorService(client)

	svc.SetNotificationService(nil)
	if svc.notifService != nil {
		t.Error("expected nil notification service")
	}
}

func TestCreate(t *testing.T) {
	client := setupTestDB(t)
	svc := NewMonitorService(client)
	ctx := context.Background()

	item, err := svc.Create(ctx, CreateInput{
		Domain:        "example.com",
		Port:          443,
		CheckType:     "https",
		CheckInterval: 3600,
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if item.ID == 0 {
		t.Fatal("created item has zero ID")
	}
	if item.Domain != "example.com" {
		t.Errorf("Domain = %q, want %q", item.Domain, "example.com")
	}
	if item.Status != "unknown" {
		t.Errorf("Status = %q, want %q", item.Status, "unknown")
	}
}

func TestCreate_DefaultPortAndInterval(t *testing.T) {
	client := setupTestDB(t)
	svc := NewMonitorService(client)
	ctx := context.Background()

	item, err := svc.Create(ctx, CreateInput{
		Domain:    "example.com",
		CheckType: "https",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if item.Port != 443 {
		t.Errorf("default Port = %d, want 443", item.Port)
	}
	if item.CheckInterval != 3600 {
		t.Errorf("default CheckInterval = %d, want 3600", item.CheckInterval)
	}
}

func TestList(t *testing.T) {
	client := setupTestDB(t)
	svc := NewMonitorService(client)
	ctx := context.Background()

	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected empty list, got %d", len(list))
	}

	svc.Create(ctx, CreateInput{Domain: "a.com", CheckType: "https"})
	svc.Create(ctx, CreateInput{Domain: "b.com", CheckType: "http"})

	list, err = svc.List(ctx)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2 items, got %d", len(list))
	}
}

func TestUpdate(t *testing.T) {
	client := setupTestDB(t)
	svc := NewMonitorService(client)
	ctx := context.Background()

	created, _ := svc.Create(ctx, CreateInput{
		Domain:    "old.com",
		CheckType: "https",
	})

	updated, err := svc.Update(ctx, created.ID, UpdateInput{
		Domain:        "new.com",
		Port:          8080,
		CheckType:     "http",
		CheckInterval: 7200,
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if updated.Domain != "new.com" {
		t.Errorf("Domain = %q, want %q", updated.Domain, "new.com")
	}
	if updated.Port != 8080 {
		t.Errorf("Port = %d, want 8080", updated.Port)
	}
}

func TestDelete(t *testing.T) {
	client := setupTestDB(t)
	svc := NewMonitorService(client)
	ctx := context.Background()

	created, _ := svc.Create(ctx, CreateInput{
		Domain:    "example.com",
		CheckType: "https",
	})

	if err := svc.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	list, _ := svc.List(ctx)
	if len(list) != 0 {
		t.Error("expected empty list after delete")
	}
}

// mockNotifService 记录通知调用，用于验证告警去重与恢复通知
type mockNotifService struct {
	calls []notification.NotificationOption
}

func (m *mockNotifService) SendNotification(opts notification.NotificationOption) error {
	m.calls = append(m.calls, opts)
	return nil
}

// TestNotifyIfProblem_DedupAndRecovery 验证告警风暴修复：
// 同一异常状态连续检查只通知一次，恢复时发送恢复通知，持续正常不再通知。
func TestNotifyIfProblem_DedupAndRecovery(t *testing.T) {
	client := setupTestDB(t)
	svc := NewMonitorService(client)
	mock := &mockNotifService{}
	svc.SetNotificationService(mock)

	ctx := context.Background()
	m, err := client.MonitoredDomain.Create().
		SetDomain("example.com").
		SetCheckType(monitoreddomain.CheckTypeHTTPS).
		Save(ctx)
	if err != nil {
		t.Fatalf("create domain failed: %v", err)
	}

	// 首次异常：通知一次
	svc.notifyIfProblem(m, checkResult{status: "error", checkError: "connection refused"})
	if len(mock.calls) != 1 {
		t.Fatalf("expected 1 notification on first error, got %d", len(mock.calls))
	}

	// 同一异常状态重复检查：抑制重复告警
	svc.notifyIfProblem(m, checkResult{status: "error", checkError: "connection refused"})
	svc.notifyIfProblem(m, checkResult{status: "error", checkError: "connection refused"})
	if len(mock.calls) != 1 {
		t.Fatalf("expected still 1 notification after duplicate errors, got %d", len(mock.calls))
	}

	// 恢复正常：发送恢复通知（success 级别）
	svc.notifyIfProblem(m, checkResult{status: "ok"})
	if len(mock.calls) != 2 {
		t.Fatalf("expected 2 notifications after recovery, got %d", len(mock.calls))
	}
	if want := i18n.T("notification.monitor_recovered.title"); mock.calls[1].Title != want {
		t.Errorf("recovery title = %q, want %q", mock.calls[1].Title, want)
	}
	if mock.calls[1].Level != "success" {
		t.Errorf("recovery level = %q, want %q", mock.calls[1].Level, "success")
	}

	// 持续正常：不再通知
	svc.notifyIfProblem(m, checkResult{status: "ok"})
	if len(mock.calls) != 2 {
		t.Fatalf("expected still 2 notifications while staying ok, got %d", len(mock.calls))
	}

	// 再次异常：状态再次跳变，应重新通知
	svc.notifyIfProblem(m, checkResult{status: "error", checkError: "timeout"})
	if len(mock.calls) != 3 {
		t.Fatalf("expected 3 notifications after re-error, got %d", len(mock.calls))
	}
}

// mockEventEmitter 记录监控更新事件，用于验证事件发射
type mockEventEmitter struct {
	payloads []events.MonitorUpdatedPayload
}

func (m *mockEventEmitter) EmitMonitorUpdated(p events.MonitorUpdatedPayload) {
	m.payloads = append(m.payloads, p)
}

// TestEmitUpdated 验证监控更新事件发射：未设置发射器不 panic，设置后正确携带负载。
func TestEmitUpdated(t *testing.T) {
	client := setupTestDB(t)
	svc := NewMonitorService(client)

	// 未设置发射器时调用不应 panic
	svc.emitUpdated(1, "example.com", "ok")

	mock := &mockEventEmitter{}
	svc.SetEventEmitter(mock)
	svc.emitUpdated(2, "example.com", "error")
	if len(mock.payloads) != 1 {
		t.Fatalf("expected 1 event, got %d", len(mock.payloads))
	}
	p := mock.payloads[0]
	if p.DomainID != 2 {
		t.Errorf("DomainID = %d, want 2", p.DomainID)
	}
	if p.Domain != "example.com" {
		t.Errorf("Domain = %q, want %q", p.Domain, "example.com")
	}
	if p.Status != "error" {
		t.Errorf("Status = %q, want %q", p.Status, "error")
	}
	if p.CheckedAt == "" {
		t.Error("expected non-empty CheckedAt")
	}

	// 批量更新（DomainID=0）
	svc.emitUpdated(0, "", "")
	if len(mock.payloads) != 2 {
		t.Fatalf("expected 2 events, got %d", len(mock.payloads))
	}
	if mock.payloads[1].DomainID != 0 {
		t.Errorf("batch DomainID = %d, want 0", mock.payloads[1].DomainID)
	}
}

// setupConcurrentTestDB 创建基于临时文件的测试库（WAL + busy_timeout，与生产 DSN 一致）。
// 不复用 shared-cache 内存库：内存 shared-cache 在并发写下会直接返回 SQLITE_LOCKED
// （不触发 busy 等待），并发测试会偶发失败。
func setupConcurrentTestDB(t *testing.T) *ent.Client {
	t.Helper()
	path := filepath.Join(t.TempDir(), "monitor_concurrent.db")
	db, err := sql.Open("sqlite3", sqlite.BuildDSN(path))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatal(err)
	}
	drv := esql.OpenDB("sqlite3", db)
	client := ent.NewClient(ent.Driver(drv))
	t.Cleanup(func() { client.Close() })
	if err := client.Schema.Create(context.Background()); err != nil {
		t.Fatal(err)
	}
	return client
}

// TestRunChecks_Concurrent 验证到期域名以有界并发方式检查：
// 屏障服务器让每个检查阻塞直到 n 个检查全部并发到达——若退化为串行，
// 第 n 个检查永远无法发起，测试将在超时后失败。
func TestRunChecks_Concurrent(t *testing.T) {
	client := setupConcurrentTestDB(t)
	svc := NewMonitorService(client)
	ctx := context.Background()

	// 并发数固定为 5（即 maxConcurrentChecks 声明的并发级别）：
	// 若退化为串行或并发度不足 5，第 5 个检查无法发起，屏障永不关闭，测试超时失败。
	const n = 5
	var arrived atomic.Int32
	release := make(chan struct{})
	allArrived := make(chan struct{})
	var releaseOnce sync.Once
	releaseAll := func() { releaseOnce.Do(func() { close(release) }) }
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if arrived.Add(1) == int32(n) {
			close(allArrived)
		}
		<-release // 阻塞直到测试方放行，确保请求在服务端并发驻留
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	// 失败路径兜底：确保退出时屏障必然释放。必须后于 srv.Close 注册（LIFO 先执行），
	// 否则处理器永久阻塞会导致 srv.Close() 永久挂起。
	defer releaseAll()

	for i := range n {
		if _, err := client.MonitoredDomain.Create().
			SetDomain(fmt.Sprintf("concurrent-%d.test", i)).
			SetCheckType(monitoreddomain.CheckTypeHTTP).
			SetURL(srv.URL).
			SetCheckInterval(60).
			SetEnabled(true).
			Save(ctx); err != nil {
			t.Fatalf("create domain %d failed: %v", i, err)
		}
	}

	done := make(chan struct{})
	go func() {
		svc.runChecks()
		close(done)
	}()

	select {
	case <-allArrived:
		// n 个检查并发到达：通过
	case <-time.After(15 * time.Second):
		t.Fatalf("expected %d concurrent checks, only %d arrived (serial execution?)", n, arrived.Load())
	}
	releaseAll()
	<-done

	// 全部域名应被更新为 ok，且每域名写入一条检查历史
	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != n {
		t.Fatalf("expected %d domains, got %d", n, len(list))
	}
	for _, item := range list {
		if item.Status != "ok" {
			t.Errorf("domain %s status = %q, want ok", item.Domain, item.Status)
		}
	}
	logCount, err := client.MonitorCheckLog.Query().Count(ctx)
	if err != nil {
		t.Fatalf("count check logs failed: %v", err)
	}
	if logCount != n {
		t.Errorf("check log count = %d, want %d", logCount, n)
	}
}

// TestBuildMonitorNotification 验证各状态对应的通知内容选择。
func TestBuildMonitorNotification(t *testing.T) {
	tests := []struct {
		name      string
		result    checkResult
		prev      string
		loaded    bool
		wantTitle string // 期望的 i18n key；空串表示不应通知
	}{
		{
			name:      "error 异常",
			result:    checkResult{status: "error", checkError: "refused"},
			wantTitle: "notification.monitor_error.title",
		},
		{
			name:      "expired 证书过期",
			result:    checkResult{status: "expired", certIssuer: "Let's Encrypt"},
			wantTitle: "notification.monitor_expired.title",
		},
		{
			name:      "warning 证书临期",
			result:    checkResult{status: "warning", certRemainingDays: 10},
			wantTitle: "notification.monitor_expiring.title",
		},
		{
			name:      "warning HTTP 4xx/5xx",
			result:    checkResult{status: "warning", httpStatusCode: 503},
			wantTitle: "notification.monitor_http_error.title",
		},
		{
			name:      "warning 无具体原因不通知",
			result:    checkResult{status: "warning"},
			wantTitle: "",
		},
		{
			name:      "ok 从异常恢复才通知",
			result:    checkResult{status: "ok"},
			prev:      "error",
			loaded:    true,
			wantTitle: "notification.monitor_recovered.title",
		},
		{
			name:      "ok 首次检查即正常不通知",
			result:    checkResult{status: "ok"},
			wantTitle: "",
		},
		{
			name:      "unknown 不通知",
			result:    checkResult{status: "unknown"},
			wantTitle: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			title, body, notify := buildMonitorNotification("example.com", tt.result, tt.prev, tt.loaded)
			if tt.wantTitle == "" {
				if notify {
					t.Errorf("expected no notification, got title=%q body=%q", title, body)
				}
				return
			}
			if !notify {
				t.Fatalf("expected notification with title key %q, got none", tt.wantTitle)
			}
			if want := i18n.T(tt.wantTitle); title != want {
				t.Errorf("title = %q, want %q", title, want)
			}
			if body == "" {
				t.Error("expected non-empty body")
			}
		})
	}
}
