package ca

import (
	"context"
	"database/sql"
	"testing"

	"cnb.cool/dtapp/certflow/ent"
	esql "entgo.io/ent/dialect/sql"
	sqlite "modernc.org/sqlite"
)

func init() {
	// 注册 modernc.org/sqlite 驱动为 "sqlite3"，以兼容 ent
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

	drv := esql.OpenDB(dialect, db)
	client := ent.NewClient(ent.Driver(drv))
	t.Cleanup(func() { client.Close() })

	ctx := context.Background()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatal(err)
	}
	return client
}

var dialect = "sqlite3"

// TestNewCAService 测试构造函数
func TestNewCAService(t *testing.T) {
	client := setupTestDB(t)
	svc := NewCAService(client, t.TempDir())
	if svc == nil {
		t.Fatal("NewCAService returned nil")
	}
	if svc.db != client {
		t.Error("db client not set correctly")
	}
}

// TestCreateAndGetByID 测试创建和查询
func TestCreateAndGetByID(t *testing.T) {
	client := setupTestDB(t)
	svc := NewCAService(client, t.TempDir())
	ctx := context.Background()

	input := CreateCAInput{
		Name:         "Let's Encrypt",
		DirectoryURL: "https://acme-v02.api.letsencrypt.org/directory",
		AccountEmail: "test@example.com",
		IsDefault:    true,
		IsActive:     true,
	}

	created, err := svc.Create(ctx, input)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if created.ID == 0 {
		t.Fatal("created CA has zero ID")
	}
	if created.Name != "Let's Encrypt" {
		t.Errorf("Name = %q, want %q", created.Name, "Let's Encrypt")
	}

	got, err := svc.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.DirectoryURL != input.DirectoryURL {
		t.Errorf("DirectoryURL = %q, want %q", got.DirectoryURL, input.DirectoryURL)
	}
}

// TestGetByID_NotFound 测试查询不存在的 CA
func TestGetByID_NotFound(t *testing.T) {
	client := setupTestDB(t)
	svc := NewCAService(client, t.TempDir())
	ctx := context.Background()

	_, err := svc.GetByID(ctx, 9999)
	if err == nil {
		t.Fatal("expected error for non-existent CA")
	}
}

// TestList 测试列出所有 CA
func TestList(t *testing.T) {
	client := setupTestDB(t)
	svc := NewCAService(client, t.TempDir())
	ctx := context.Background()

	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected empty list, got %d", len(list))
	}

	svc.Create(ctx, CreateCAInput{Name: "CA1", DirectoryURL: "url1"})
	svc.Create(ctx, CreateCAInput{Name: "CA2", DirectoryURL: "url2"})

	list, err = svc.List(ctx)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2 CAs, got %d", len(list))
	}
}

// TestUpdate 测试更新字段
func TestUpdate(t *testing.T) {
	client := setupTestDB(t)
	svc := NewCAService(client, t.TempDir())
	ctx := context.Background()

	created, _ := svc.Create(ctx, CreateCAInput{
		Name:         "Old Name",
		DirectoryURL: "https://old.example.com",
	})

	updated, err := svc.Update(ctx, created.ID, UpdateCAInput{
		Name: "New Name",
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if updated.Name != "New Name" {
		t.Errorf("Name = %q, want %q", updated.Name, "New Name")
	}
	if updated.DirectoryURL != "https://old.example.com" {
		t.Error("DirectoryURL should not have changed")
	}
}

// TestUpdate_NotFound 测试更新不存在的 CA
func TestUpdate_NotFound(t *testing.T) {
	client := setupTestDB(t)
	svc := NewCAService(client, t.TempDir())
	ctx := context.Background()

	_, err := svc.Update(ctx, 9999, UpdateCAInput{Name: "x"})
	if err == nil {
		t.Fatal("expected error for non-existent CA")
	}
}

// TestDelete 测试删除 CA
func TestDelete(t *testing.T) {
	client := setupTestDB(t)
	svc := NewCAService(client, t.TempDir())
	ctx := context.Background()

	created, _ := svc.Create(ctx, CreateCAInput{Name: "ToDelete", DirectoryURL: "url"})
	if err := svc.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err := svc.GetByID(ctx, created.ID)
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

// TestDelete_NotFound 测试删除不存在的 CA
func TestDelete_NotFound(t *testing.T) {
	client := setupTestDB(t)
	svc := NewCAService(client, t.TempDir())
	ctx := context.Background()

	err := svc.Delete(ctx, 9999)
	if err == nil {
		t.Fatal("expected error for non-existent CA")
	}
}

// TestSetDefault 测试设置默认 CA 并清除其他默认
func TestSetDefault(t *testing.T) {
	client := setupTestDB(t)
	svc := NewCAService(client, t.TempDir())
	ctx := context.Background()

	ca1, _ := svc.Create(ctx, CreateCAInput{Name: "CA1", DirectoryURL: "url1", IsDefault: true})
	ca2, _ := svc.Create(ctx, CreateCAInput{Name: "CA2", DirectoryURL: "url2"})

	result, err := svc.SetDefault(ctx, ca2.ID)
	if err != nil {
		t.Fatalf("SetDefault failed: %v", err)
	}
	if !result.IsDefault {
		t.Error("expected CA2 to be default")
	}

	ca1Updated, _ := svc.GetByID(ctx, ca1.ID)
	if ca1Updated.IsDefault {
		t.Error("expected CA1 to no longer be default")
	}
}

// TestSetDefault_NotFound 测试设置不存在的 CA 为默认
func TestSetDefault_NotFound(t *testing.T) {
	client := setupTestDB(t)
	svc := NewCAService(client, t.TempDir())
	ctx := context.Background()

	_, err := svc.SetDefault(ctx, 9999)
	if err == nil {
		t.Fatal("expected error for non-existent CA")
	}
}

// TestSeedDefaults 测试初始化默认 CA（插入 4 个）
func TestSeedDefaults(t *testing.T) {
	client := setupTestDB(t)
	tmpDir := t.TempDir()
	svc := NewCAService(client, tmpDir)
	ctx := context.Background()

	if err := svc.SeedDefaults(ctx); err != nil {
		t.Fatalf("SeedDefaults failed: %v", err)
	}

	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != 4 {
		t.Fatalf("expected 4 CAs after SeedDefaults, got %d", len(list))
	}

	// 验证 Let's Encrypt 是默认 CA
	defaultCount := 0
	for _, c := range list {
		if c.IsDefault {
			defaultCount++
		}
	}
	if defaultCount != 1 {
		t.Errorf("expected exactly 1 default CA, got %d", defaultCount)
	}

	// 重复 SeedDefaults 不应添加更多 CA
	if err := svc.SeedDefaults(ctx); err != nil {
		t.Fatalf("SeedDefaults second call failed: %v", err)
	}
	list2, _ := svc.List(ctx)
	if len(list2) != 4 {
		t.Errorf("expected still 4 CAs after second SeedDefaults, got %d", len(list2))
	}
}
