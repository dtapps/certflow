package auth

import (
	"context"
	"database/sql"
	"testing"

	"cnb.cool/dtapp/certflow/ent"
	esql "entgo.io/ent/dialect/sql"
	sqlite "modernc.org/sqlite"
)

func init() {
	sql.Register("sqlite3", &sqlite.Driver{})
}

func newTestService(t *testing.T) (*AuthService, string) {
	t.Helper()
	dir := t.TempDir()

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

	// 运行迁移
	if err := client.Schema.Create(context.Background()); err != nil {
		t.Fatalf("Schema.Create: %v", err)
	}

	svc := NewAuthService(client)
	return svc, dir
}

func newTestClient(t *testing.T) *ent.Client {
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

	if err := client.Schema.Create(context.Background()); err != nil {
		t.Fatalf("Schema.Create: %v", err)
	}

	return client
}

func TestNewAuthService(t *testing.T) {
	client := newTestClient(t)
	svc := NewAuthService(client)
	if svc == nil {
		t.Fatal("expected non-nil AuthService")
	}
	if svc.IsPasswordSet() {
		t.Fatal("expected no password set initially")
	}
}

func TestSetPassword(t *testing.T) {
	svc, _ := newTestService(t)

	if err := svc.SetPassword("password123"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	if !svc.IsPasswordSet() {
		t.Fatal("expected IsPasswordSet to return true after SetPassword")
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
	client := newTestClient(t)
	svc := NewAuthService(client)

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

func TestGetActiveMethod(t *testing.T) {
	svc, _ := newTestService(t)

	// 初始状态应该没有激活的方法
	method, err := svc.GetActiveMethod()
	if err != nil {
		t.Fatalf("GetActiveMethod: %v", err)
	}
	if method != "" {
		t.Fatalf("expected empty method, got %s", method)
	}

	// 设置密码后，密码应该是激活的方法
	if err := svc.SetPassword("testpass"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}

	method, err = svc.GetActiveMethod()
	if err != nil {
		t.Fatalf("GetActiveMethod: %v", err)
	}
	if method != "password" {
		t.Fatalf("expected 'password', got %s", method)
	}
}

func TestGetAvailableMethods(t *testing.T) {
	svc, _ := newTestService(t)

	// 初始状态应该没有可用的方法
	methods, err := svc.GetAvailableMethods()
	if err != nil {
		t.Fatalf("GetAvailableMethods: %v", err)
	}
	if len(methods) != 0 {
		t.Fatalf("expected 0 methods, got %d", len(methods))
	}

	// 设置密码后，应该有一个可用的方法
	if err := svc.SetPassword("testpass"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}

	methods, err = svc.GetAvailableMethods()
	if err != nil {
		t.Fatalf("GetAvailableMethods: %v", err)
	}
	if len(methods) != 1 {
		t.Fatalf("expected 1 method, got %d", len(methods))
	}
	if methods[0] != "password" {
		t.Fatalf("expected 'password', got %s", methods[0])
	}
}

func TestAuthenticate(t *testing.T) {
	svc, _ := newTestService(t)

	// 未设置密码时，任何密码都应该验证通过
	result, err := svc.Authenticate("password", "anything")
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if !result {
		t.Fatal("expected true when no password is set")
	}

	// 设置密码后
	if err := svc.SetPassword("testpass"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}

	// 正确密码
	result, err = svc.Authenticate("password", "testpass")
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if !result {
		t.Fatal("expected true for correct password")
	}

	// 错误密码
	result, err = svc.Authenticate("password", "wrongpass")
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if result {
		t.Fatal("expected false for wrong password")
	}

	// 不支持的方法
	_, err = svc.Authenticate("unsupported", "test")
	if err == nil {
		t.Fatal("expected error for unsupported method")
	}
}
