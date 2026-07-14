package monitor

import (
	"context"
	"database/sql"
	"testing"

	"cnb.cool/dtapp/certflow/ent"
	"cnb.cool/dtapp/certflow/internal/settings"
	esql "entgo.io/ent/dialect/sql"
	sqlite "modernc.org/sqlite"
)

func init() {
	// 生产依赖包（db/httplog）可能已注册同名驱动，重复注册会 panic，故用 recover 兜底。
	defer func() { _ = recover() }()
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
		Enabled:       true,
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
		Enabled:   true,
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

	svc.Create(ctx, CreateInput{Domain: "a.com", CheckType: "https", Enabled: true})
	svc.Create(ctx, CreateInput{Domain: "b.com", CheckType: "http", Enabled: false})

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
		Enabled:   true,
	})

	updated, err := svc.Update(ctx, created.ID, CreateInput{
		Domain:        "new.com",
		Port:          8080,
		CheckType:     "http",
		CheckInterval: 7200,
		Enabled:       false,
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
		Enabled:   true,
	})

	if err := svc.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	list, _ := svc.List(ctx)
	if len(list) != 0 {
		t.Error("expected empty list after delete")
	}
}
