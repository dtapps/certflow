package auth

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"cnb.cool/dtapp/certflow/ent"
	_ "cnb.cool/dtapp/certflow/internal/sqlite"
	esql "entgo.io/ent/dialect/sql"
)

func newFuzzClient(t *testing.T) *ent.Client {
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

// FuzzSetPassword 模糊测试密码设置
func FuzzSetPassword(f *testing.F) {
	f.Add("password123")
	f.Add("short")
	f.Add("")
	f.Add(strings.Repeat("a", 10000))
	f.Add("中文密码123")
	f.Add("<script>alert(1)</script>")
	f.Add("' OR 1=1 --")

	f.Fuzz(func(t *testing.T, password string) {
		client := newFuzzClient(t)
		svc := NewAuthService(client)

		err := svc.SetPassword(password)
		if err != nil {
			// 密码太短或太长（bcrypt 限制 72 字节）应报错
			if len(password) < 6 || len([]byte(password)) > 72 {
				return
			}
			t.Errorf("SetPassword(%q) unexpected error: %v", password, err)
		}

		// 设置成功后应能验证
		if err == nil && len(password) >= 6 && len([]byte(password)) <= 72 {
			if !svc.IsPasswordSet() {
				t.Error("IsPasswordSet() should return true after SetPassword")
			}
			if !svc.VerifyPassword(password) {
				t.Errorf("VerifyPassword(%q) should return true", password)
			}
		}
	})
}

// FuzzVerifyPassword 模糊测试密码验证
func FuzzVerifyPassword(f *testing.F) {
	f.Add("mypassword", "mypassword", true)
	f.Add("mypassword", "wrongpass", false)
	f.Add("mypassword", "", false)
	f.Add("mypassword", strings.Repeat("x", 10000), false)

	f.Fuzz(func(t *testing.T, setPw, tryPw string, _ bool) {
		client := newFuzzClient(t)
		svc := NewAuthService(client)

		// 先设置密码
		if len(setPw) < 6 {
			t.Skip("setPw too short")
		}
		if err := svc.SetPassword(setPw); err != nil {
			t.Skip("SetPassword failed")
		}

		// 模糊验证不应 panic
		svc.VerifyPassword(tryPw)
	})
}

// FuzzAuthenticate 模糊测试统一认证接口
func FuzzAuthenticate(f *testing.F) {
	f.Add("password", "mypassword", "mypassword")
	f.Add("password", "mypassword", "wrongpass")
	f.Add("totp", "123456", "")
	f.Add("passkey", "data", "")
	f.Add("unsupported", "any", "")

	f.Fuzz(func(t *testing.T, method, setPw, tryPw string) {
		client := newFuzzClient(t)
		svc := NewAuthService(client)

		if len(setPw) >= 6 {
			_ = svc.SetPassword(setPw)
		}

		// Authenticate 不应 panic，只应返回 error 或 false
		svc.Authenticate(method, tryPw)
	})
}
