package scheduler

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"cnb.cool/dtapp/certflow/ent"
	"cnb.cool/dtapp/certflow/ent/certificate"
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

func TestNewScheduler(t *testing.T) {
	client := setupTestDB(t)
	s := NewScheduler(client, nil, nil, nil, "/tmp/certs")
	if s == nil {
		t.Fatal("NewScheduler returned nil")
	}
	if s.db != client {
		t.Error("db client not set correctly")
	}
	if s.certDir != "/tmp/certs" {
		t.Errorf("certDir = %q, want %q", s.certDir, "/tmp/certs")
	}
}

func TestGetRenewalLogs_Empty(t *testing.T) {
	client := setupTestDB(t)
	s := NewScheduler(client, nil, nil, nil, "")
	ctx := context.Background()

	logs, err := s.GetRenewalLogs(ctx, 1)
	if err != nil {
		t.Fatalf("GetRenewalLogs failed: %v", err)
	}
	if len(logs) != 0 {
		t.Errorf("expected empty logs, got %d", len(logs))
	}
}

func TestGetRecentRenewalLogs_Empty(t *testing.T) {
	client := setupTestDB(t)
	s := NewScheduler(client, nil, nil, nil, "")
	ctx := context.Background()

	logs, err := s.GetRecentRenewalLogs(ctx, 10)
	if err != nil {
		t.Fatalf("GetRecentRenewalLogs failed: %v", err)
	}
	if len(logs) != 0 {
		t.Errorf("expected empty logs, got %d", len(logs))
	}
}

func TestGetRecentRenewalLogs_WithData(t *testing.T) {
	client := setupTestDB(t)
	ctx := context.Background()

	cert, err := client.Certificate.Create().
		SetDomain("example.com").
		SetCertContent("cert-pem").
		SetKeyContent("key-pem").
		SetIssuer("Let's Encrypt").
		SetNotBefore(time.Now()).
		SetNotAfter(time.Now().Add(90 * 24 * time.Hour)).
		SetAutoRenew(true).
		SetRenewalDays(30).
		SetStatus(certificate.StatusActive).
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create test certificate: %v", err)
	}

	client.RenewalLog.Create().
		SetCertificateID(cert.ID).
		SetStatus("success").
		SetAttemptAt(time.Now().Add(-2 * time.Hour)).
		Save(ctx)
	client.RenewalLog.Create().
		SetCertificateID(cert.ID).
		SetStatus("failed").
		SetErrorMessage("test error").
		SetAttemptAt(time.Now().Add(-1 * time.Hour)).
		Save(ctx)

	s := NewScheduler(client, nil, nil, nil, "")

	logs, err := s.GetRecentRenewalLogs(ctx, 10)
	if err != nil {
		t.Fatalf("GetRecentRenewalLogs failed: %v", err)
	}
	if len(logs) != 2 {
		t.Errorf("expected 2 logs, got %d", len(logs))
	}

	if logs[0].Status != "failed" {
		t.Errorf("expected most recent log to be 'failed', got %q", logs[0].Status)
	}
}
