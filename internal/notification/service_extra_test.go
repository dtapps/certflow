package notification

import (
	"context"
	"strings"
	"testing"

	"entgo.io/ent/dialect"

	"cnb.cool/dtapp/certflow/internal/ent"
	"cnb.cool/dtapp/certflow/internal/ent/enttest"
	entnotification "cnb.cool/dtapp/certflow/internal/ent/notification"
	"cnb.cool/dtapp/certflow/internal/i18n"
	"cnb.cool/dtapp/certflow/internal/sqlite"
)

func newTestNotif(t *testing.T) (*NotificationService, *ent.Client) {
	t.Helper()
	client := enttest.Open(t, dialect.SQLite, sqlite.BuildDSN(t.TempDir()+"/notif.db"))
	t.Cleanup(func() { _ = client.Close() })

	svc := NewNotificationService()
	svc.SetDB(client)
	return svc, client
}

func seedNotification(t *testing.T, client *ent.Client, title string, read bool) *ent.Notification {
	t.Helper()
	return client.Notification.Create().
		SetTitle(title).
		SetBody("body").
		SetCategory(entnotification.CategoryCert).
		SetLevel(entnotification.LevelInfo).
		SetRead(read).
		SaveX(context.Background())
}

// TestCRUD_NoDB 数据库未初始化时每个入口都必须给出可读错误，
// 不能返回 nil（前端会以为操作成功）。
func TestCRUD_NoDB(t *testing.T) {
	svc := NewNotificationService() // 不 SetDB
	ctx := context.Background()

	if _, err := svc.ListNotifications(ctx, 10, 0); err == nil {
		t.Error("ListNotifications 应报错")
	}
	if _, err := svc.CountUnread(ctx); err == nil {
		t.Error("CountUnread 应报错")
	}
	if err := svc.MarkAsRead(ctx, 1); err == nil {
		t.Error("MarkAsRead 应报错")
	}
	if err := svc.MarkAllAsRead(ctx); err == nil {
		t.Error("MarkAllAsRead 应报错")
	}
	if err := svc.DeleteNotification(ctx, 1); err == nil {
		t.Error("DeleteNotification 应报错")
	}
	if err := svc.ClearAllNotifications(ctx); err == nil {
		t.Error("ClearAllNotifications 应报错")
	}
}

func TestMarkAllAsRead(t *testing.T) {
	svc, client := newTestNotif(t)
	ctx := context.Background()

	seedNotification(t, client, "a", false)
	seedNotification(t, client, "b", false)
	seedNotification(t, client, "c", true)

	if err := svc.MarkAllAsRead(ctx); err != nil {
		t.Fatalf("MarkAllAsRead err: %v", err)
	}
	n, err := svc.CountUnread(ctx)
	if err != nil {
		t.Fatalf("CountUnread err: %v", err)
	}
	if n != 0 {
		t.Fatalf("全部标记已读后未读数应为 0, 实际 %d", n)
	}
	if err := svc.MarkAllAsRead(ctx); err != nil {
		t.Fatalf("空表重复调用应无害: %v", err)
	}
}

func TestDeleteNotification(t *testing.T) {
	svc, client := newTestNotif(t)
	ctx := context.Background()

	n := seedNotification(t, client, "a", true)

	if err := svc.DeleteNotification(ctx, n.ID); err != nil {
		t.Fatalf("DeleteNotification err: %v", err)
	}
	if left, _ := client.Notification.Query().Count(ctx); left != 0 {
		t.Fatalf("删除后应剩 0 条, 实际 %d", left)
	}
	if err := svc.DeleteNotification(ctx, n.ID); err == nil {
		t.Fatal("重复删除应报错")
	}
}

// TestListNotifications_Pagination 分页游标：limit/offset 传错会让「加载更多」重复或漏条。
func TestListNotifications_Pagination(t *testing.T) {
	svc, client := newTestNotif(t)
	ctx := context.Background()

	for _, title := range []string{"1", "2", "3", "4", "5"} {
		seedNotification(t, client, title, true)
	}

	page1, err := svc.ListNotifications(ctx, 2, 0)
	if err != nil {
		t.Fatalf("ListNotifications err: %v", err)
	}
	page2, err := svc.ListNotifications(ctx, 2, 2)
	if err != nil {
		t.Fatalf("ListNotifications err: %v", err)
	}
	if len(page1) != 2 || len(page2) != 2 {
		t.Fatalf("每页应 2 条: %d / %d", len(page1), len(page2))
	}
	if page1[0].ID == page2[0].ID {
		t.Fatal("第二页与第一页重复")
	}

	all, err := svc.ListNotifications(ctx, 0, 0)
	if err != nil {
		t.Fatalf("ListNotifications err: %v", err)
	}
	if len(all) != 5 {
		t.Fatalf("limit=0 应返回全部 5 条, 实际 %d", len(all))
	}
}

// TestNotificationTemplates_Render 全部通知文案都必须能正常渲染。
//
// 这些字符串直接进系统通知和通知中心，i18n key 写错或模板占位符与传参不一致时，
// 用户看到的是 "notification.xxx.body" 原文或 "域名 <no value> 已过期"。
// 不依赖 Wails 运行时，只校验模板本身，所以能进单元测试。
func TestNotificationTemplates_Render(t *testing.T) {
	cases := []struct {
		key    string
		params []any
	}{
		{"notification.cert_applied.title", nil},
		{"notification.cert_applied.body", []any{"Domain", "a.example.com", "Issuer", "Test CA"}},
		{"notification.cert_renewed.title", nil},
		{"notification.cert_renewed.body", []any{"Domain", "a.example.com", "Issuer", "Test CA", "ValidUntil", "2027-01-01"}},
		{"notification.cert_revoked.title", nil},
		{"notification.cert_revoked.body", []any{"Domain", "a.example.com"}},
		{"notification.cert_expiring.title", nil},
		{"notification.cert_expiring.subtitle", nil},
		{"notification.cert_expiring.body", []any{"Domain", "a.example.com", "Days", 3}},
		{"notification.cert_apply_failed.title", nil},
		{"notification.cert_apply_failed.body", []any{"Domain", "a.example.com", "Error", "boom"}},
		{"notification.cert_renew_failed.title", nil},
		{"notification.cert_renew_failed.body", []any{"Domain", "a.example.com", "Error", "boom"}},
		{"notification.deploy_success.title", nil},
		{"notification.deploy_success.body", []any{"Domain", "a.example.com", "Target", "aliyun"}},
		{"notification.deploy_success_site.body", []any{"Site", "site.example.com", "Target", "btpanel"}},
		{"notification.deploy_failed.title", nil},
		{"notification.deploy_failed.body", []any{"Domain", "a.example.com", "Target", "aliyun", "Error", "boom"}},
		{"notification.deploy_failed_site.body", []any{"Site", "site.example.com", "Target", "btpanel", "Error", "boom"}},
	}

	for _, loc := range []string{"zh-CN", "en-US"} {
		for _, c := range cases {
			got := i18n.TWithLocale(loc, c.key, c.params...)

			if got == "" {
				t.Errorf("[%s] %s 渲染为空", loc, c.key)
				continue
			}
			// key 原样返回 = 词条缺失
			if got == c.key {
				t.Errorf("[%s] %s 词条缺失（原样返回 key）", loc, c.key)
				continue
			}
			// 占位符没被替换 = 传参名字对不上
			if strings.Contains(got, "{{") || strings.Contains(got, "<no value>") {
				t.Errorf("[%s] %s 渲染残留占位符: %q", loc, c.key, got)
			}
		}
	}
}
