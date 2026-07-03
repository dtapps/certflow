package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewService(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewService(dir)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if svc == nil {
		t.Fatal("expected non-nil Service")
	}

	// 数据目录和 settings.json 应该已创建
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Fatal("expected data directory to be created")
	}
	settingsFile := filepath.Join(dir, "settings.json")
	if _, err := os.Stat(settingsFile); os.IsNotExist(err) {
		t.Fatal("expected settings.json to be created")
	}
}

func TestNewService_LoadsExisting(t *testing.T) {
	dir := t.TempDir()
	svc1, err := NewService(dir)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	s := svc1.Get()
	s.Language = "en-US"
	if err := svc1.Save(s); err != nil {
		t.Fatalf("Save: %v", err)
	}

	svc2, err := NewService(dir)
	if err != nil {
		t.Fatalf("NewService (load): %v", err)
	}
	got := svc2.Get()
	if got.Language != "en-US" {
		t.Errorf("Language = %q, want %q", got.Language, "en-US")
	}
}

func TestDefaultSettings(t *testing.T) {
	s := DefaultSettings()
	if s.DefaultRenewalDays == 0 {
		t.Error("expected non-zero DefaultRenewalDays")
	}
	if s.CheckInterval == 0 {
		t.Error("expected non-zero CheckInterval")
	}
	if s.Theme == "" {
		t.Error("expected non-empty Theme")
	}
	if len(s.DNSConfigs) == 0 {
		t.Error("expected non-empty DNSConfigs")
	}
	if s.Proxy.Port == 0 {
		t.Error("expected non-zero Proxy.Port")
	}
	if s.Log.Level == "" {
		t.Error("expected non-empty Log.Level")
	}
}

func TestGetSaveRoundtrip(t *testing.T) {
	svc, err := NewService(t.TempDir())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	s := svc.Get()
	s.DataDir = "/tmp/test-data"
	s.DefaultRenewalDays = 60
	s.Language = "en-US"
	s.Theme = "light"

	if err := svc.Save(s); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got := svc.Get()
	if got.DataDir != "/tmp/test-data" {
		t.Errorf("DataDir = %q, want %q", got.DataDir, "/tmp/test-data")
	}
	if got.DefaultRenewalDays != 60 {
		t.Errorf("DefaultRenewalDays = %d, want 60", got.DefaultRenewalDays)
	}
	if got.Language != "en-US" {
		t.Errorf("Language = %q, want %q", got.Language, "en-US")
	}
	if got.Theme != "light" {
		t.Errorf("Theme = %q, want %q", got.Theme, "light")
	}
}

func TestIsSeeded_MarkSeeded(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewService(dir)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	if svc.IsSeeded() {
		t.Fatal("expected IsSeeded to return false initially")
	}

	if err := svc.MarkSeeded(); err != nil {
		t.Fatalf("MarkSeeded: %v", err)
	}

	if !svc.IsSeeded() {
		t.Fatal("expected IsSeeded to return true after MarkSeeded")
	}

	// 验证新实例能读取已保存的数据
	svc2, err := NewService(dir)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if !svc2.IsSeeded() {
		t.Fatal("expected IsSeeded to persist across service instances")
	}
}

func TestUpdateNotificationEnabled(t *testing.T) {
	svc, err := NewService(t.TempDir())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	// 默认值应为 true
	s := svc.Get()
	if !s.NotificationEnabled {
		t.Fatal("expected default NotificationEnabled to be true")
	}

	if err := svc.UpdateNotificationEnabled(false); err != nil {
		t.Fatalf("UpdateNotificationEnabled: %v", err)
	}
	if svc.Get().NotificationEnabled {
		t.Error("expected NotificationEnabled to be false after update")
	}

	if err := svc.UpdateNotificationEnabled(true); err != nil {
		t.Fatalf("UpdateNotificationEnabled: %v", err)
	}
	if !svc.Get().NotificationEnabled {
		t.Error("expected NotificationEnabled to be true after re-enable")
	}
}
