package dnsprovider

import (
	"context"
	"database/sql"
	"testing"

	"cnb.cool/dtapp/certflow/internal/ent"
	_ "cnb.cool/dtapp/certflow/internal/sqlite"
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

func TestNewDNSProviderService(t *testing.T) {
	client := setupTestDB(t)
	svc := NewDNSProviderService(client)
	if svc == nil {
		t.Fatal("NewDNSProviderService returned nil")
	}
}

func TestGetProviderTypes(t *testing.T) {
	client := setupTestDB(t)
	svc := NewDNSProviderService(client)
	ctx := context.Background()

	types := svc.GetProviderTypes(ctx)
	if len(types) == 0 {
		t.Fatal("expected non-empty provider types")
	}

	seen := make(map[string]bool)
	for _, pt := range types {
		seen[pt] = true
	}
	for _, want := range []string{"cloudflare", "aliyun", "aws", "godaddy"} {
		if !seen[want] {
			t.Errorf("expected provider type %q not found", want)
		}
	}
}

func TestCreateAndGetByID(t *testing.T) {
	client := setupTestDB(t)
	svc := NewDNSProviderService(client)
	ctx := context.Background()

	input := CreateDNSProviderInput{
		Name:         "My Cloudflare",
		ProviderType: "cloudflare",
		Config:       map[string]string{"api_token": "test-token"},
		Comment:      "test provider",
	}

	created, err := svc.Create(ctx, input)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if created.ID == 0 {
		t.Fatal("created DNS provider has zero ID")
	}
	if created.Name != "My Cloudflare" {
		t.Errorf("Name = %q, want %q", created.Name, "My Cloudflare")
	}

	got, err := svc.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.ProviderType != "cloudflare" {
		t.Errorf("ProviderType = %q, want %q", got.ProviderType, "cloudflare")
	}
}

func TestGetByID_NotFound(t *testing.T) {
	client := setupTestDB(t)
	svc := NewDNSProviderService(client)
	ctx := context.Background()

	_, err := svc.GetByID(ctx, 9999)
	if err == nil {
		t.Fatal("expected error for non-existent DNS provider")
	}
}

func TestList(t *testing.T) {
	client := setupTestDB(t)
	svc := NewDNSProviderService(client)
	ctx := context.Background()

	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected empty list, got %d", len(list))
	}

	svc.Create(ctx, CreateDNSProviderInput{Name: "P1", ProviderType: "cloudflare"})
	svc.Create(ctx, CreateDNSProviderInput{Name: "P2", ProviderType: "aliyun"})

	list, err = svc.List(ctx)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2 providers, got %d", len(list))
	}
}

func TestDelete(t *testing.T) {
	client := setupTestDB(t)
	svc := NewDNSProviderService(client)
	ctx := context.Background()

	created, _ := svc.Create(ctx, CreateDNSProviderInput{
		Name:         "ToDelete",
		ProviderType: "cloudflare",
	})

	if err := svc.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err := svc.GetByID(ctx, created.ID)
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestDelete_NotFound(t *testing.T) {
	client := setupTestDB(t)
	svc := NewDNSProviderService(client)
	ctx := context.Background()

	err := svc.Delete(ctx, 9999)
	if err == nil {
		t.Fatal("expected error for non-existent DNS provider")
	}
}
