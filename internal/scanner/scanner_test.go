package scanner

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"cnb.cool/dtapp/certflow/ent"
	"cnb.cool/dtapp/certflow/internal/settings"
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

func TestNewScannerService(t *testing.T) {
	client := setupTestDB(t)
	svc := NewScannerService(client)
	if svc == nil {
		t.Fatal("NewScannerService returned nil")
	}
	if svc.db != client {
		t.Error("db client not set correctly")
	}
}

func TestSetSettingsProvider(t *testing.T) {
	client := setupTestDB(t)
	svc := NewScannerService(client)

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

func TestScan_EmptyDomain(t *testing.T) {
	client := setupTestDB(t)
	svc := NewScannerService(client)
	ctx := context.Background()

	_, err := svc.Scan(ctx, ScanInput{Domain: ""})
	if err == nil {
		t.Fatal("expected error for empty domain")
	}
}

func TestListHistory_Empty(t *testing.T) {
	client := setupTestDB(t)
	svc := NewScannerService(client)
	ctx := context.Background()

	results, err := svc.ListHistory(ctx)
	if err != nil {
		t.Fatalf("ListHistory failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected empty history, got %d", len(results))
	}
}

func TestSaveAndListHistory(t *testing.T) {
	client := setupTestDB(t)
	svc := NewScannerService(client)
	_ = svc // 用于后续扩展
	ctx := context.Background()

	// 手动插入扫描结果
	item, err := client.ScanResult.Create().
		SetDomain("example.com").
		SetPort(443).
		SetScanType("https").
		SetScannedAt(time.Now()).
		SetResponseTimeMs(120).
		SetCertIssuer("Let's Encrypt").
		SetCertSubject("example.com").
		SetCertNotBefore(time.Now().AddDate(-1, 0, 0)).
		SetCertNotAfter(time.Now().AddDate(0, 6, 0)).
		SetCertRemainingDays(180).
		SetCertFingerprint("abc123").
		SetCertSignatureAlgo("SHA256-RSA").
		SetCertPublicKeyAlgo("RSA").
		SetCertPublicKeyBits(2048).
		SetCertSans([]string{"example.com", "www.example.com"}).
		SetCertSerialNumber("1234567890").
		Save(ctx)
	if err != nil {
		t.Fatalf("Create scan result failed: %v", err)
	}
	if item.ID == 0 {
		t.Fatal("created item has zero ID")
	}

	// 测试 ListHistory
	svc2 := NewScannerService(client)
	results, err := svc2.ListHistory(ctx)
	if err != nil {
		t.Fatalf("ListHistory failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Domain != "example.com" {
		t.Errorf("Domain = %q, want %q", results[0].Domain, "example.com")
	}
	if results[0].CertIssuer != "Let's Encrypt" {
		t.Errorf("CertIssuer = %q, want %q", results[0].CertIssuer, "Let's Encrypt")
	}
	if results[0].ResponseTimeMs != 120 {
		t.Errorf("ResponseTimeMs = %d, want 120", results[0].ResponseTimeMs)
	}
	if len(results[0].CertSANs) != 2 {
		t.Errorf("CertSANs len = %d, want 2", len(results[0].CertSANs))
	}
}

func TestListHistory_Order(t *testing.T) {
	client := setupTestDB(t)
	ctx := context.Background()

	// 插入两条不同时间的记录
	now := time.Now()
	client.ScanResult.Create().
		SetDomain("old.example.com").
		SetPort(443).
		SetScanType("https").
		SetScannedAt(now.Add(-1 * time.Hour)).
		SetResponseTimeMs(100).
		Save(ctx)

	client.ScanResult.Create().
		SetDomain("new.example.com").
		SetPort(443).
		SetScanType("https").
		SetScannedAt(now).
		SetResponseTimeMs(200).
		Save(ctx)

	svc := NewScannerService(client)
	results, err := svc.ListHistory(ctx)
	if err != nil {
		t.Fatalf("ListHistory failed: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	// 按时间倒序，新的在前
	if results[0].Domain != "new.example.com" {
		t.Errorf("first result Domain = %q, want %q", results[0].Domain, "new.example.com")
	}
	if results[1].Domain != "old.example.com" {
		t.Errorf("second result Domain = %q, want %q", results[1].Domain, "old.example.com")
	}
}

func TestDeleteResult(t *testing.T) {
	client := setupTestDB(t)
	ctx := context.Background()

	item, _ := client.ScanResult.Create().
		SetDomain("example.com").
		SetPort(443).
		SetScanType("https").
		SetScannedAt(time.Now()).
		SetResponseTimeMs(100).
		Save(ctx)

	svc := NewScannerService(client)
	err := svc.DeleteResult(ctx, item.ID)
	if err != nil {
		t.Fatalf("DeleteResult failed: %v", err)
	}

	// 确认已删除
	results, _ := svc.ListHistory(ctx)
	if len(results) != 0 {
		t.Errorf("expected 0 results after delete, got %d", len(results))
	}
}

func TestClearHistory(t *testing.T) {
	client := setupTestDB(t)
	ctx := context.Background()

	// 插入多条记录
	for range 5 {
		client.ScanResult.Create().
			SetDomain("example.com").
			SetPort(443).
			SetScanType("https").
			SetScannedAt(time.Now()).
			SetResponseTimeMs(100).
			Save(ctx)
	}

	svc := NewScannerService(client)
	err := svc.ClearHistory(ctx)
	if err != nil {
		t.Fatalf("ClearHistory failed: %v", err)
	}

	results, _ := svc.ListHistory(ctx)
	if len(results) != 0 {
		t.Errorf("expected 0 results after clear, got %d", len(results))
	}
}

func TestScanResultFields(t *testing.T) {
	// 验证 ScanResultItem 所有字段正确映射
	client := setupTestDB(t)
	ctx := context.Background()

	sans := []string{"a.com", "b.com"}
	item, err := client.ScanResult.Create().
		SetDomain("test.com").
		SetPort(8443).
		SetScanType("https").
		SetScannedAt(time.Now()).
		SetResponseTimeMs(250).
		SetCertIssuer("Test Issuer").
		SetCertSubject("Test Subject").
		SetCertNotBefore(time.Now().AddDate(-1, 0, 0)).
		SetCertNotAfter(time.Now().AddDate(0, 1, 0)).
		SetCertRemainingDays(30).
		SetCertFingerprint("fp123").
		SetCertSignatureAlgo("SHA256-RSA").
		SetCertPublicKeyAlgo("ECDSA").
		SetCertPublicKeyBits(256).
		SetCertSans(sans).
		SetCertSerialNumber("serial123").
		SetErrorMessage("test error").
		Save(ctx)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	svc := NewScannerService(client)
	results, err := svc.ListHistory(ctx)
	if err != nil {
		t.Fatalf("ListHistory failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	r := results[0]
	if r.ID != item.ID {
		t.Errorf("ID = %d, want %d", r.ID, item.ID)
	}
	if r.Domain != "test.com" {
		t.Errorf("Domain = %q, want %q", r.Domain, "test.com")
	}
	if r.Port != 8443 {
		t.Errorf("Port = %d, want 8443", r.Port)
	}
	if r.ScanType != "https" {
		t.Errorf("ScanType = %q, want %q", r.ScanType, "https")
	}
	if r.ResponseTimeMs != 250 {
		t.Errorf("ResponseTimeMs = %d, want 250", r.ResponseTimeMs)
	}
	if r.CertIssuer != "Test Issuer" {
		t.Errorf("CertIssuer = %q, want %q", r.CertIssuer, "Test Issuer")
	}
	if r.CertSubject != "Test Subject" {
		t.Errorf("CertSubject = %q, want %q", r.CertSubject, "Test Subject")
	}
	if r.CertRemainingDays != 30 {
		t.Errorf("CertRemainingDays = %d, want 30", r.CertRemainingDays)
	}
	if r.CertFingerprint != "fp123" {
		t.Errorf("CertFingerprint = %q, want %q", r.CertFingerprint, "fp123")
	}
	if r.CertSignatureAlgo != "SHA256-RSA" {
		t.Errorf("CertSignatureAlgo = %q, want %q", r.CertSignatureAlgo, "SHA256-RSA")
	}
	if r.CertPublicKeyAlgo != "ECDSA" {
		t.Errorf("CertPublicKeyAlgo = %q, want %q", r.CertPublicKeyAlgo, "ECDSA")
	}
	if r.CertPublicKeyBits != 256 {
		t.Errorf("CertPublicKeyBits = %d, want 256", r.CertPublicKeyBits)
	}
	if len(r.CertSANs) != 2 {
		t.Errorf("CertSANs len = %d, want 2", len(r.CertSANs))
	}
	if r.CertSerialNumber != "serial123" {
		t.Errorf("CertSerialNumber = %q, want %q", r.CertSerialNumber, "serial123")
	}
	if r.ErrorMessage != "test error" {
		t.Errorf("ErrorMessage = %q, want %q", r.ErrorMessage, "test error")
	}
}
