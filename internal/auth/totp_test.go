package auth

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"

	"cnb.cool/dtapp/certflow/internal/ent/authmethod"
	"cnb.cool/dtapp/certflow/internal/ent/enttest"
	"cnb.cool/dtapp/certflow/internal/sqlite"

	"entgo.io/ent/dialect"
)

// newTOTPTestService 每个用例使用独立临时库，避免 auth 包共享内存库互相污染。
func newTOTPTestService(t *testing.T) *AuthService {
	t.Helper()
	client := enttest.Open(t, dialect.SQLite, sqlite.BuildDSN(filepath.Join(t.TempDir(), "auth.db")))
	t.Cleanup(func() { _ = client.Close() })
	return NewAuthService(client)
}

// mustSetupAndVerifyTOTP 完成 TOTP 设置与激活。
func mustSetupAndVerifyTOTP(t *testing.T, svc *AuthService) string {
	t.Helper()
	res, err := svc.SetupTOTP()
	if err != nil {
		t.Fatalf("SetupTOTP 失败: %v", err)
	}
	code, err := totp.GenerateCode(res.Secret, time.Now())
	if err != nil {
		t.Fatalf("生成 TOTP 码失败: %v", err)
	}
	if err := svc.VerifyTOTPSetup(code); err != nil {
		t.Fatalf("VerifyTOTPSetup 失败: %v", err)
	}
	return res.Secret
}

// TestSetupTOTP 覆盖首次设置与重复设置。
func TestSetupTOTP(t *testing.T) {
	svc := newTOTPTestService(t)

	res, err := svc.SetupTOTP()
	if err != nil {
		t.Fatalf("首次设置失败: %v", err)
	}
	if res.Secret == "" || res.Issuer != "CertFlow" || res.AccountName != "user" {
		t.Fatalf("设置结果不符: %+v", res)
	}
	if res.URL == "" || len(res.Secret) < 16 {
		t.Fatalf("otpauth URL 或密钥异常: %+v", res)
	}

	// 已存在 TOTP 认证方式（无论是否激活）都不允许重复设置
	if _, err := svc.SetupTOTP(); err == nil {
		t.Fatal("重复设置应报错")
	}
}

// TestVerifyTOTPSetup 覆盖激活成功/失败与未设置场景。
func TestVerifyTOTPSetup(t *testing.T) {
	t.Run("未设置时报错", func(t *testing.T) {
		svc := newTOTPTestService(t)
		if err := svc.VerifyTOTPSetup("123456"); err == nil {
			t.Fatal("未设置时校验应报错")
		}
	})

	t.Run("错误验证码不激活", func(t *testing.T) {
		svc := newTOTPTestService(t)
		if _, err := svc.SetupTOTP(); err != nil {
			t.Fatalf("SetupTOTP 失败: %v", err)
		}
		if err := svc.VerifyTOTPSetup("000000"); err == nil {
			t.Fatal("错误验证码应报错")
		}
		info, err := svc.GetTOTPInfo()
		if err != nil {
			t.Fatalf("GetTOTPInfo 失败: %v", err)
		}
		if info.IsConfigured {
			// 已写入凭据但未激活，IsConfigured 表示记录存在
			t.Log("凭据记录已存在（未激活）")
		}
		method, _ := svc.GetActiveMethod()
		if method == authmethod.MethodTotp.String() {
			t.Fatal("校验失败不应激活 TOTP")
		}
	})

	t.Run("正确验证码激活并停用其它方式", func(t *testing.T) {
		svc := newTOTPTestService(t)
		// 先设置密码（激活 password 方式），验证 TOTP 激活会停用其它方式
		if err := svc.SetPassword("Passw0rd!"); err != nil {
			t.Fatalf("SetPassword 失败: %v", err)
		}
		mustSetupAndVerifyTOTP(t, svc)

		method, err := svc.GetActiveMethod()
		if err != nil {
			t.Fatalf("GetActiveMethod 失败: %v", err)
		}
		if method != authmethod.MethodTotp.String() {
			t.Fatalf("激活方式应为 totp，实际 %q", method)
		}

		// 其它方式必须被停用
		svc.mu.RLock()
		defer svc.mu.RUnlock()
		others, err := svc.db.AuthMethod.Query().
			Where(authmethod.MethodNEQ(authmethod.MethodTotp)).
			All(context.Background())
		if err != nil {
			t.Fatalf("查询认证方式失败: %v", err)
		}
		for _, am := range others {
			if am.IsActive {
				t.Fatalf("认证方式 %s 应被停用", am.Method)
			}
		}
	})
}

// TestVerifyTOTP 覆盖登录时的验证码校验。
func TestVerifyTOTP(t *testing.T) {
	svc := newTOTPTestService(t)

	// 未配置时返回 false
	if svc.VerifyTOTP("123456") {
		t.Fatal("未配置时应返回 false")
	}

	secret := mustSetupAndVerifyTOTP(t, svc)
	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("生成验证码失败: %v", err)
	}
	if !svc.VerifyTOTP(code) {
		t.Fatal("正确验证码应通过")
	}
	if svc.VerifyTOTP("000000") {
		t.Fatal("错误验证码不应通过")
	}
}

// TestClearTOTP 覆盖清除 TOTP 后可回落到其它方式。
func TestClearTOTP(t *testing.T) {
	svc := newTOTPTestService(t)
	if err := svc.SetPassword("Passw0rd!"); err != nil {
		t.Fatalf("SetPassword 失败: %v", err)
	}
	mustSetupAndVerifyTOTP(t, svc)

	if err := svc.ClearTOTP(); err != nil {
		t.Fatalf("ClearTOTP 失败: %v", err)
	}
	info, err := svc.GetTOTPInfo()
	if err != nil {
		t.Fatalf("GetTOTPInfo 失败: %v", err)
	}
	if info.IsConfigured {
		t.Fatal("清除后不应再显示已配置")
	}
	// 清除后应回落到仍存在的密码方式
	method, err := svc.GetActiveMethod()
	if err != nil {
		t.Fatalf("GetActiveMethod 失败: %v", err)
	}
	if method != authmethod.MethodPassword.String() {
		t.Fatalf("应回落到密码方式，实际 %q", method)
	}

	// 无任何方式时清除也不报错
	empty := newTOTPTestService(t)
	if err := empty.ClearTOTP(); err != nil {
		t.Fatalf("空库清除不应报错: %v", err)
	}
}

// TestCancelTOTP 覆盖取消未确认的 TOTP 设置。
func TestCancelTOTP(t *testing.T) {
	svc := newTOTPTestService(t)
	if _, err := svc.SetupTOTP(); err != nil {
		t.Fatalf("SetupTOTP 失败: %v", err)
	}
	if err := svc.CancelTOTP(); err != nil {
		t.Fatalf("CancelTOTP 失败: %v", err)
	}
	info, err := svc.GetTOTPInfo()
	if err != nil {
		t.Fatalf("GetTOTPInfo 失败: %v", err)
	}
	if info.IsConfigured {
		t.Fatal("取消后不应再显示已配置")
	}
	// 重复取消（无数据）也应成功
	if err := svc.CancelTOTP(); err != nil {
		t.Fatalf("重复取消不应报错: %v", err)
	}
}

// TestGetTOTPInfo 覆盖 TOTP 信息读取。
func TestGetTOTPInfo(t *testing.T) {
	svc := newTOTPTestService(t)

	info, err := svc.GetTOTPInfo()
	if err != nil {
		t.Fatalf("未配置时读取失败: %v", err)
	}
	if info.IsConfigured {
		t.Fatal("未配置时 IsConfigured 应为 false")
	}

	mustSetupAndVerifyTOTP(t, svc)
	info, err = svc.GetTOTPInfo()
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if !info.IsConfigured || info.Issuer != "CertFlow" || info.AccountName != "user" {
		t.Fatalf("信息不符: %+v", info)
	}
	if info.CreatedAt == "" {
		t.Fatal("创建时间不应为空")
	}
}

// TestSetActiveMethod 覆盖认证方式切换的各种校验。
func TestSetActiveMethod(t *testing.T) {
	svc := newTOTPTestService(t)

	if err := svc.SetActiveMethod("unknown"); err == nil {
		t.Fatal("非法方式应报错")
	}
	if err := svc.SetActiveMethod(authmethod.MethodTotp.String()); err == nil {
		t.Fatal("未配置的方式不应可激活")
	}

	if err := svc.SetPassword("Passw0rd!"); err != nil {
		t.Fatalf("SetPassword 失败: %v", err)
	}
	mustSetupAndVerifyTOTP(t, svc)

	// 切到密码方式
	if err := svc.SetActiveMethod(authmethod.MethodPassword.String()); err != nil {
		t.Fatalf("切换密码方式失败: %v", err)
	}
	if method, _ := svc.GetActiveMethod(); method != authmethod.MethodPassword.String() {
		t.Fatalf("当前方式应为 password，实际 %q", method)
	}

	// 切回 TOTP
	if err := svc.SetActiveMethod(authmethod.MethodTotp.String()); err != nil {
		t.Fatalf("切换 TOTP 方式失败: %v", err)
	}
	if method, _ := svc.GetActiveMethod(); method != authmethod.MethodTotp.String() {
		t.Fatalf("当前方式应为 totp，实际 %q", method)
	}
}
