package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"cnb.cool/dtapp/certflow/internal/ent/authmethod"
)

// passkeyRegJSON 构造一个合法的注册回传数据。
func passkeyRegJSON(t *testing.T, rawID []byte) string {
	t.Helper()
	payload := map[string]any{
		"id":    base64.RawURLEncoding.EncodeToString(rawID),
		"rawId": base64.RawURLEncoding.EncodeToString(rawID),
		"type":  "public-key",
		"response": map[string]any{
			"attestationObject": "a",
			"clientDataJSON":    "b",
		},
	}
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("构造注册数据失败: %v", err)
	}
	return string(b)
}

// passkeyAuthJSON 构造一个合法的认证回传数据。
func passkeyAuthJSON(t *testing.T, rawID []byte) string {
	t.Helper()
	payload := map[string]any{
		"id":    base64.RawURLEncoding.EncodeToString(rawID),
		"rawId": base64.RawURLEncoding.EncodeToString(rawID),
		"type":  "public-key",
		"response": map[string]any{
			"authenticatorData": "a",
			"clientDataJSON":    "b",
			"signature":         "c",
		},
	}
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("构造认证数据失败: %v", err)
	}
	return string(b)
}

// TestGetWebAuthn 覆盖 WebAuthn 实例构造。
func TestGetWebAuthn(t *testing.T) {
	svc := newTOTPTestService(t)
	wa, err := svc.getWebAuthn()
	if err != nil {
		t.Fatalf("构造 WebAuthn 失败: %v", err)
	}
	if wa == nil {
		t.Fatal("WebAuthn 实例不应为空")
	}
	if wa.Config.RPID != "localhost" || wa.Config.RPDisplayName != "CertFlow" {
		t.Fatalf("WebAuthn 配置不符: %+v", wa.Config)
	}
}

// TestStartPasskeyRegistration 覆盖开始注册：首次成功、重复注册报错。
func TestStartPasskeyRegistration(t *testing.T) {
	svc := newTOTPTestService(t)

	res, err := svc.StartPasskeyRegistration()
	if err != nil {
		t.Fatalf("首次注册失败: %v", err)
	}
	if res == nil || res.CredentialCreationOptions == "" {
		t.Fatalf("注册选项为空: %+v", res)
	}
	// 选项必须是可解析的 JSON，且带 challenge（证明 challenge 来自真实 ceremony）
	var opts map[string]any
	if err := json.Unmarshal([]byte(res.CredentialCreationOptions), &opts); err != nil {
		t.Fatalf("注册选项不是合法 JSON: %v", err)
	}
	if _, ok := opts["challenge"]; !ok {
		t.Fatalf("注册选项缺少 challenge: %v", opts)
	}

	// 已存在 passkey 认证方式时不允许重复注册
	if _, err := svc.StartPasskeyRegistration(); err == nil {
		t.Fatal("重复注册应报错")
	}
}

// TestFinishPasskeyRegistration 覆盖完成注册的各条分支。
func TestFinishPasskeyRegistration(t *testing.T) {
	t.Run("非法JSON", func(t *testing.T) {
		svc := newTOTPTestService(t)
		if err := svc.FinishPasskeyRegistration("not json"); err == nil {
			t.Fatal("非法 JSON 应报错")
		}
	})

	t.Run("非法凭证ID", func(t *testing.T) {
		svc := newTOTPTestService(t)
		// 用一个含非法 base64url 字符的 rawId
		if err := svc.FinishPasskeyRegistration(`{"rawId":"!!!","response":{}}`); err == nil {
			t.Fatal("非法 rawId 应报错")
		}
	})

	t.Run("未开始注册", func(t *testing.T) {
		svc := newTOTPTestService(t)
		if err := svc.FinishPasskeyRegistration(passkeyRegJSON(t, []byte("cred-x"))); err == nil {
			t.Fatal("无待完成注册时应报错")
		}
	})

	t.Run("成功注册并激活", func(t *testing.T) {
		svc := newTOTPTestService(t)
		// 先配置密码，验证 passkey 注册会停用其它方式
		if err := svc.SetPassword("Passw0rd!"); err != nil {
			t.Fatalf("SetPassword 失败: %v", err)
		}
		if _, err := svc.StartPasskeyRegistration(); err != nil {
			t.Fatalf("开始注册失败: %v", err)
		}
		rawID := []byte("cred-1")
		if err := svc.FinishPasskeyRegistration(passkeyRegJSON(t, rawID)); err != nil {
			t.Fatalf("完成注册失败: %v", err)
		}

		info, err := svc.GetPasskeyInfo()
		if err != nil {
			t.Fatalf("GetPasskeyInfo 失败: %v", err)
		}
		if !info.IsConfigured || info.CredentialCount != 1 || info.CreatedAt == "" {
			t.Fatalf("Passkey 信息不符: %+v", info)
		}

		method, err := svc.GetActiveMethod()
		if err != nil {
			t.Fatalf("GetActiveMethod 失败: %v", err)
		}
		if method != authmethod.MethodPasskey.String() {
			t.Fatalf("注册后应激活 passkey，实际 %q", method)
		}

		// 其它方式被停用
		ctx := context.Background()
		others, err := svc.db.AuthMethod.Query().
			Where(authmethod.MethodNEQ(authmethod.MethodPasskey)).
			All(ctx)
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

// TestStartPasskeyLogin 覆盖开始登录：无凭据报错、有凭据返回选项。
func TestStartPasskeyLogin(t *testing.T) {
	svc := newTOTPTestService(t)

	if _, err := svc.StartPasskeyLogin(); err == nil {
		t.Fatal("无凭据时应报错")
	}

	// 注册一个凭据
	if _, err := svc.StartPasskeyRegistration(); err != nil {
		t.Fatalf("开始注册失败: %v", err)
	}
	if err := svc.FinishPasskeyRegistration(passkeyRegJSON(t, []byte("cred-login"))); err != nil {
		t.Fatalf("完成注册失败: %v", err)
	}

	res, err := svc.StartPasskeyLogin()
	if err != nil {
		t.Fatalf("开始登录失败: %v", err)
	}
	var opts map[string]any
	if err := json.Unmarshal([]byte(res.CredentialRequestOptions), &opts); err != nil {
		t.Fatalf("登录选项不是合法 JSON: %v", err)
	}
	if _, ok := opts["challenge"]; !ok {
		t.Fatalf("登录选项缺少 challenge: %v", opts)
	}
	allowed, ok := opts["allowCredentials"].([]any)
	if !ok || len(allowed) != 1 {
		t.Fatalf("允许凭据列表不符: %v", opts["allowCredentials"])
	}
}

// TestFinishPasskeyLogin 覆盖完成登录：非法数据、未知凭据、已知凭据。
func TestFinishPasskeyLogin(t *testing.T) {
	svc := newTOTPTestService(t)

	if _, err := svc.FinishPasskeyLogin("not json"); err == nil {
		t.Fatal("非法 JSON 应报错")
	}
	if _, err := svc.FinishPasskeyLogin(`{"rawId":"!!!"}`); err == nil {
		t.Fatal("非法 rawId 应报错")
	}

	// 凭据不存在：返回 false 且不报错
	ok, err := svc.FinishPasskeyLogin(passkeyAuthJSON(t, []byte("unknown")))
	if err != nil {
		t.Fatalf("未知凭据不应报错: %v", err)
	}
	if ok {
		t.Fatal("未知凭据不应通过")
	}

	// 注册后登录成功
	rawID := []byte("cred-login-2")
	if _, err := svc.StartPasskeyRegistration(); err != nil {
		t.Fatalf("开始注册失败: %v", err)
	}
	if err := svc.FinishPasskeyRegistration(passkeyRegJSON(t, rawID)); err != nil {
		t.Fatalf("完成注册失败: %v", err)
	}
	ok, err = svc.FinishPasskeyLogin(passkeyAuthJSON(t, rawID))
	if err != nil {
		t.Fatalf("登录失败: %v", err)
	}
	if !ok {
		t.Fatal("已注册凭据应通过")
	}
}

// TestClearPasskey 覆盖清除 Passkey 并回落其它方式。
func TestClearPasskey(t *testing.T) {
	svc := newTOTPTestService(t)

	// 空库清除不报错
	if err := svc.ClearPasskey(); err != nil {
		t.Fatalf("空库清除不应报错: %v", err)
	}

	if err := svc.SetPassword("Passw0rd!"); err != nil {
		t.Fatalf("SetPassword 失败: %v", err)
	}
	if _, err := svc.StartPasskeyRegistration(); err != nil {
		t.Fatalf("开始注册失败: %v", err)
	}
	if err := svc.FinishPasskeyRegistration(passkeyRegJSON(t, []byte("cred-clear"))); err != nil {
		t.Fatalf("完成注册失败: %v", err)
	}

	if err := svc.ClearPasskey(); err != nil {
		t.Fatalf("清除 Passkey 失败: %v", err)
	}

	info, err := svc.GetPasskeyInfo()
	if err != nil {
		t.Fatalf("GetPasskeyInfo 失败: %v", err)
	}
	if info.IsConfigured || info.CredentialCount != 0 {
		t.Fatalf("清除后不应再有 passkey: %+v", info)
	}

	ctx := context.Background()
	if n, err := svc.db.PasskeyCredential.Query().Count(ctx); err != nil || n != 0 {
		t.Fatalf("凭据未清理: n=%d err=%v", n, err)
	}

	// 其它方式应被重新激活
	method, err := svc.GetActiveMethod()
	if err != nil {
		t.Fatalf("GetActiveMethod 失败: %v", err)
	}
	if method != authmethod.MethodPassword.String() {
		t.Fatalf("应回落到密码方式，实际 %q", method)
	}
}

// TestGetPasskeyInfo_NotConfigured 覆盖未配置时的信息读取。
func TestGetPasskeyInfo_NotConfigured(t *testing.T) {
	svc := newTOTPTestService(t)
	info, err := svc.GetPasskeyInfo()
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if info.IsConfigured {
		t.Fatal("未配置时 IsConfigured 应为 false")
	}
}

// TestPasskeyI18nMessages 确保 passkey 相关错误文案已翻译且无占位符残留。
func TestPasskeyI18nMessages(t *testing.T) {
	svc := newTOTPTestService(t)
	_, err := svc.StartPasskeyLogin()
	if err == nil {
		t.Fatal("预期错误")
	}
	msg := err.Error()
	if strings.Contains(msg, "error.") || strings.Contains(msg, "{{") {
		t.Fatalf("错误文案未翻译: %q", msg)
	}
}
