package auth

import (
	"os"
	"path/filepath"
	"testing"
)

func newTestService(t *testing.T) (*AuthService, string) {
	t.Helper()
	dir := t.TempDir()
	svc, err := NewAuthService(dir)
	if err != nil {
		t.Fatalf("NewAuthService: %v", err)
	}
	return svc, dir
}

func TestNewAuthService(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewAuthService(dir)
	if err != nil {
		t.Fatalf("NewAuthService failed: %v", err)
	}
	if svc == nil {
		t.Fatal("expected non-nil AuthService")
	}
	if svc.IsPasswordSet() {
		t.Fatal("expected no password set initially")
	}

	// 数据目录应该存在
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Fatal("expected data directory to be created")
	}

	// auth.json 此时不应存在
	authFile := filepath.Join(dir, "auth.json")
	if _, err := os.Stat(authFile); !os.IsNotExist(err) {
		t.Fatal("expected auth.json to not exist before setting password")
	}
}

func TestNewAuthService_LoadsExistingPassword(t *testing.T) {
	dir := t.TempDir()

	// 创建第一个服务并设置密码
	svc1, err := NewAuthService(dir)
	if err != nil {
		t.Fatalf("NewAuthService: %v", err)
	}
	if err := svc1.SetPassword("secret123"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}

	// 从同一目录创建第二个服务 — 应加载已保存的哈希
	svc2, err := NewAuthService(dir)
	if err != nil {
		t.Fatalf("NewAuthService (load): %v", err)
	}
	if !svc2.IsPasswordSet() {
		t.Fatal("expected password to be loaded from file")
	}
	if !svc2.VerifyPassword("secret123") {
		t.Fatal("expected loaded password to verify")
	}
}

func TestSetPassword(t *testing.T) {
	svc, dir := newTestService(t)

	if err := svc.SetPassword("password123"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	if !svc.IsPasswordSet() {
		t.Fatal("expected IsPasswordSet to return true after SetPassword")
	}

	// auth.json 应该已创建且包含哈希值
	authFile := filepath.Join(dir, "auth.json")
	data, err := os.ReadFile(authFile)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected auth.json to have content")
	}
}

func TestSetPassword_TooShort(t *testing.T) {
	svc, _ := newTestService(t)

	tests := []struct {
		name     string
		password string
	}{
		{"empty", ""},
		{"1 char", "a"},
		{"5 chars", "12345"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.SetPassword(tt.password)
			if err == nil {
				t.Fatalf("expected error for password %q, got nil", tt.password)
			}
		})
	}
}

func TestVerifyPassword(t *testing.T) {
	svc, _ := newTestService(t)

	// 未设置密码 — 应返回 true（无密码时验证通过）
	if !svc.VerifyPassword("anything") {
		t.Fatal("expected VerifyPassword to return true when no password is set")
	}

	// 设置密码
	if err := svc.SetPassword("mypassword"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}

	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"correct", "mypassword", true},
		{"wrong", "wrongpassword", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := svc.VerifyPassword(tt.input); got != tt.expected {
				t.Errorf("VerifyPassword(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestChangePassword(t *testing.T) {
	svc, _ := newTestService(t)

	// 未设置旧密码时修改密码（旧密码验证在未设置时通过）
	if err := svc.ChangePassword("", "newpass123"); err != nil {
		t.Fatalf("ChangePassword (no existing): %v", err)
	}
	if !svc.VerifyPassword("newpass123") {
		t.Fatal("expected new password to verify")
	}
	if svc.VerifyPassword("oldpass") {
		t.Fatal("expected old password to not verify")
	}

	// 已有密码时修改密码
	if err := svc.ChangePassword("newpass123", "newpass456"); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}
	if !svc.VerifyPassword("newpass456") {
		t.Fatal("expected updated password to verify")
	}
}

func TestChangePassword_WrongOldPassword(t *testing.T) {
	svc, _ := newTestService(t)

	if err := svc.SetPassword("correct123"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}

	err := svc.ChangePassword("wrongpass", "newpass123")
	if err == nil {
		t.Fatal("expected error when old password is wrong")
	}
}

func TestChangePassword_NewTooShort(t *testing.T) {
	svc, _ := newTestService(t)

	if err := svc.SetPassword("correct123"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}

	err := svc.ChangePassword("correct123", "short")
	if err == nil {
		t.Fatal("expected error when new password is too short")
	}
}

func TestClearPassword(t *testing.T) {
	svc, _ := newTestService(t)

	if err := svc.SetPassword("mypassword"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	if !svc.IsPasswordSet() {
		t.Fatal("expected password to be set")
	}

	if err := svc.ClearPassword(); err != nil {
		t.Fatalf("ClearPassword: %v", err)
	}
	if svc.IsPasswordSet() {
		t.Fatal("expected IsPasswordSet to return false after ClearPassword")
	}

	// 清除密码后，任何密码都应该验证通过
	if !svc.VerifyPassword("anything") {
		t.Fatal("expected VerifyPassword to return true after clearing password")
	}
}

func TestIsPasswordSet_Persistence(t *testing.T) {
	dir := t.TempDir()

	svc, err := NewAuthService(dir)
	if err != nil {
		t.Fatalf("NewAuthService: %v", err)
	}

	if svc.IsPasswordSet() {
		t.Fatal("expected false before setting")
	}

	if err := svc.SetPassword("testpass"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	if !svc.IsPasswordSet() {
		t.Fatal("expected true after setting")
	}

	if err := svc.ClearPassword(); err != nil {
		t.Fatalf("ClearPassword: %v", err)
	}
	if svc.IsPasswordSet() {
		t.Fatal("expected false after clearing")
	}
}
