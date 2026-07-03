package notification

import (
	"context"
	"database/sql"
	"testing"

	"cnb.cool/dtapp/certflow/ent"
	"cnb.cool/dtapp/certflow/ent/notification"
	esql "entgo.io/ent/dialect/sql"
	sqlite "modernc.org/sqlite"
)

func init() {
	sql.Register("sqlite3", &sqlite.Driver{})
}

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

func setupServiceWithDB(t *testing.T) *NotificationService {
	t.Helper()
	client := setupTestDB(t)
	return &NotificationService{db: client}
}

func TestNewNotificationService(t *testing.T) {
	svc := NewNotificationService()
	if svc == nil {
		t.Fatal("NewNotificationService returned nil")
	}
	if svc.notifService == nil {
		t.Error("notifService not initialized")
	}
}

func TestListNotifications_Empty(t *testing.T) {
	svc := setupServiceWithDB(t)
	ctx := context.Background()

	list, err := svc.ListNotifications(ctx, 10, 0)
	if err != nil {
		t.Fatalf("ListNotifications failed: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected empty list, got %d", len(list))
	}
}

func TestListNotifications_WithEntries(t *testing.T) {
	client := setupTestDB(t)
	ctx := context.Background()

	client.Notification.Create().SetTitle("Title1").SetBody("Body1").SetCategory(notification.CategorySystem).Save(ctx)
	client.Notification.Create().SetTitle("Title2").SetBody("Body2").SetCategory(notification.CategoryCert).Save(ctx)
	client.Notification.Create().SetTitle("Title3").SetBody("Body3").SetCategory(notification.CategoryMonitor).Save(ctx)

	svc := &NotificationService{db: client}

	list, err := svc.ListNotifications(ctx, 2, 0)
	if err != nil {
		t.Fatalf("ListNotifications failed: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2 entries with limit 2, got %d", len(list))
	}

	list, err = svc.ListNotifications(ctx, 10, 2)
	if err != nil {
		t.Fatalf("ListNotifications with offset failed: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 entry with offset 2, got %d", len(list))
	}
}

func TestCountUnread(t *testing.T) {
	client := setupTestDB(t)
	ctx := context.Background()

	client.Notification.Create().SetTitle("Unread1").SetRead(false).Save(ctx)
	client.Notification.Create().SetTitle("Unread2").SetRead(false).Save(ctx)
	client.Notification.Create().SetTitle("Read1").SetRead(true).Save(ctx)

	svc := &NotificationService{db: client}

	count, err := svc.CountUnread(ctx)
	if err != nil {
		t.Fatalf("CountUnread failed: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 unread, got %d", count)
	}
}

func TestMarkAsRead(t *testing.T) {
	client := setupTestDB(t)
	ctx := context.Background()

	n, _ := client.Notification.Create().SetTitle("Unread").SetRead(false).Save(ctx)

	svc := &NotificationService{db: client}

	if err := svc.MarkAsRead(ctx, n.ID); err != nil {
		t.Fatalf("MarkAsRead failed: %v", err)
	}

	updated, _ := client.Notification.Get(ctx, n.ID)
	if !updated.Read {
		t.Error("expected notification to be marked as read")
	}
}

func TestClearAllNotifications(t *testing.T) {
	client := setupTestDB(t)
	ctx := context.Background()

	client.Notification.Create().SetTitle("N1").Save(ctx)
	client.Notification.Create().SetTitle("N2").Save(ctx)
	client.Notification.Create().SetTitle("N3").Save(ctx)

	svc := &NotificationService{db: client}

	if err := svc.ClearAllNotifications(ctx); err != nil {
		t.Fatalf("ClearAllNotifications failed: %v", err)
	}

	count, _ := client.Notification.Query().Count(ctx)
	if count != 0 {
		t.Errorf("expected 0 notifications after clear, got %d", count)
	}
}
