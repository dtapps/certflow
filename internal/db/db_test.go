package db

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInit(t *testing.T) {
	dir := t.TempDir()

	if err := Init(dir); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer Close()

	if Client == nil {
		t.Fatal("Client is nil after Init")
	}

	// 验证数据库文件已创建
	dbPath := filepath.Join(dir, "data", "certflow.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Errorf("database file not created at %s", dbPath)
	}
}

func TestClose_NoClient(t *testing.T) {
	// 将 Client 重置为 nil
	original := Client
	Client = nil
	defer func() { Client = original }()

	// 不应 panic
	if err := Close(); err != nil {
		t.Fatalf("Close with nil client should not error, got: %v", err)
	}
}

func TestClose_WithClient(t *testing.T) {
	dir := t.TempDir()

	if err := Init(dir); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	if err := Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	Client = nil
}
