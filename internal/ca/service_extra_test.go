package ca

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cnb.cool/dtapp/certflow/internal/ent/ca"
	"cnb.cool/dtapp/certflow/internal/settings"
)

// TestValidateBuiltinActivation 覆盖内置 CA 启用条件校验。
func TestValidateBuiltinActivation(t *testing.T) {
	le := "https://acme-v02.api.letsencrypt.org/directory"
	zeroSSL := "https://acme.zerossl.com/v2/DV90/directory"

	cases := []struct {
		name             string
		caName, dirURL   string
		email, kid, hmac string
		wantErr          bool
		wantWord         string
	}{
		{"未知 CA 不校验", "自定义", "https://example.com/dir", "", "", "", false, ""},
		{"LE 缺邮箱", "Let's Encrypt", le, "", "", "", true, "邮箱"},
		{"LE 有邮箱", "Let's Encrypt", le, "a@b.com", "", "", false, ""},
		{"ZeroSSL 缺邮箱与 EAB", "ZeroSSL", zeroSSL, "", "", "", true, "EAB"},
		{"ZeroSSL 缺 EAB", "ZeroSSL", zeroSSL, "a@b.com", "", "", true, "EAB"},
		{"ZeroSSL 只有 KID", "ZeroSSL", zeroSSL, "a@b.com", "kid", "", true, "EAB"},
		{"ZeroSSL 完整", "ZeroSSL", zeroSSL, "a@b.com", "kid", "hmac", false, ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateBuiltinActivation(c.caName, c.dirURL, c.email, c.kid, c.hmac)
			if c.wantErr && err == nil {
				t.Fatal("预期报错但通过了")
			}
			if !c.wantErr && err != nil {
				t.Fatalf("不应报错: %v", err)
			}
			if c.wantErr {
				if strings.Contains(err.Error(), "error.") || strings.Contains(err.Error(), "{{") {
					t.Fatalf("错误文案未翻译: %q", err.Error())
				}
				if c.wantWord != "" && !strings.Contains(err.Error(), c.wantWord) {
					t.Fatalf("错误文案应包含 %q，实际 %q", c.wantWord, err.Error())
				}
			}
		})
	}
}

// TestSetActive 覆盖启用/禁用及各校验分支。
func TestSetActive(t *testing.T) {
	client := setupTestDB(t)
	svc := NewCAService(client, t.TempDir())
	ctx := context.Background()

	t.Run("不存在", func(t *testing.T) {
		if _, err := svc.SetActive(ctx, 9999, true); err == nil {
			t.Fatal("不存在的 CA 应报错")
		}
	})

	t.Run("非内置可直接启用", func(t *testing.T) {
		created, err := svc.Create(ctx, CreateCAInput{Name: "自建", DirectoryURL: "https://example.com/dir"})
		if err != nil {
			t.Fatalf("Create 失败: %v", err)
		}
		got, err := svc.SetActive(ctx, created.ID, true)
		if err != nil {
			t.Fatalf("启用失败: %v", err)
		}
		if !got.IsActive {
			t.Fatal("应为启用状态")
		}
		list, err := svc.ListActive(ctx)
		if err != nil {
			t.Fatalf("ListActive 失败: %v", err)
		}
		if len(list) != 1 || list[0].ID != created.ID {
			t.Fatalf("启用列表不符: %+v", list)
		}

		// 禁用
		got, err = svc.SetActive(ctx, created.ID, false)
		if err != nil {
			t.Fatalf("禁用失败: %v", err)
		}
		if got.IsActive {
			t.Fatal("应为禁用状态")
		}
	})

	t.Run("内置缺必填项禁止启用", func(t *testing.T) {
		created, err := svc.Create(ctx, CreateCAInput{
			Name:         "ZeroSSL",
			DirectoryURL: "https://acme.zerossl.com/v2/DV90/directory",
			IsBuiltin:    true,
		})
		if err != nil {
			t.Fatalf("Create 失败: %v", err)
		}
		if _, err := svc.SetActive(ctx, created.ID, true); err == nil {
			t.Fatal("缺邮箱与 EAB 的内置 CA 不应可启用")
		}

		// 补齐后可以启用
		if _, err := svc.Update(ctx, created.ID, UpdateCAInput{
			AccountEmail: "a@b.com",
			EabKid:       new("kid"),
			EabHmac:      new("hmac"),
		}); err != nil {
			t.Fatalf("Update 失败: %v", err)
		}
		if _, err := svc.SetActive(ctx, created.ID, true); err != nil {
			t.Fatalf("补齐后应可启用: %v", err)
		}

		// 禁用不校验
		if _, err := svc.SetActive(ctx, created.ID, false); err != nil {
			t.Fatalf("禁用不应校验: %v", err)
		}
	})
}

// TestUpdate_EABAndBuiltinGuard 覆盖 EAB 置空/设置与已启用内置 CA 的再校验。
func TestUpdate_EABAndBuiltinGuard(t *testing.T) {
	client := setupTestDB(t)
	svc := NewCAService(client, t.TempDir())
	ctx := context.Background()

	created, err := svc.Create(ctx, CreateCAInput{
		Name:         "ZeroSSL",
		DirectoryURL: "https://acme.zerossl.com/v2/DV90/directory",
		AccountEmail: "a@b.com",
		EabKid:       "kid",
		EabHmac:      "hmac",
		IsBuiltin:    true,
	})
	if err != nil {
		t.Fatalf("Create 失败: %v", err)
	}
	if created.EabKid != "kid" || created.EabHmac != "hmac" {
		t.Fatalf("EAB 未落库: %+v", created)
	}

	// 清空 EAB：未启用时允许
	updated, err := svc.Update(ctx, created.ID, UpdateCAInput{EabKid: new(""), EabHmac: new("")})
	if err != nil {
		t.Fatalf("未启用时应允许清空 EAB: %v", err)
	}
	if updated.EabKid != "" || updated.EabHmac != "" {
		t.Fatalf("EAB 未清空: %+v", updated)
	}

	// 重新设置并启用
	if _, err := svc.Update(ctx, created.ID, UpdateCAInput{EabKid: new("kid2"), EabHmac: new("hmac2")}); err != nil {
		t.Fatalf("Update 失败: %v", err)
	}
	if _, err := svc.SetActive(ctx, created.ID, true); err != nil {
		t.Fatalf("启用失败: %v", err)
	}

	// 已启用状态下清空 EAB 应被拦截
	if _, err := svc.Update(ctx, created.ID, UpdateCAInput{EabKid: new("")}); err == nil {
		t.Fatal("已启用内置 CA 清空 EAB 应报错")
	}
	// 已启用状态下清空邮箱同样应被拦截
	if _, err := svc.Update(ctx, created.ID, UpdateCAInput{AccountEmail: ""}); err != nil {
		// AccountEmail 为空字符串时不会写入（等价于不改），因此不应报错
		t.Fatalf("空邮箱表示不修改，不应报错: %v", err)
	}
	// 其它字段更新不受影响
	if _, err := svc.Update(ctx, created.ID, UpdateCAInput{Name: "ZeroSSL 改名"}); err != nil {
		t.Fatalf("改名失败: %v", err)
	}
}

// TestDelete_BuiltinForbidden 内置 CA 不可删除。
func TestDelete_BuiltinForbidden(t *testing.T) {
	client := setupTestDB(t)
	svc := NewCAService(client, t.TempDir())
	ctx := context.Background()

	created, err := svc.Create(ctx, CreateCAInput{
		Name:         "Let's Encrypt",
		DirectoryURL: "https://acme-v02.api.letsencrypt.org/directory",
		IsBuiltin:    true,
	})
	if err != nil {
		t.Fatalf("Create 失败: %v", err)
	}
	err = svc.Delete(ctx, created.ID)
	if err == nil {
		t.Fatal("内置 CA 不应可删除")
	}
	if !strings.Contains(err.Error(), "内置") {
		t.Fatalf("错误文案不符: %q", err.Error())
	}
	// 仍存在
	if _, err := svc.GetByID(ctx, created.ID); err != nil {
		t.Fatalf("内置 CA 应仍存在: %v", err)
	}
}

// TestSeedDefaults_PreservesExisting 覆盖重复预置时保留用户已配置的内置 CA。
func TestSeedDefaults_PreservesExisting(t *testing.T) {
	client := setupTestDB(t)
	svc := NewCAService(client, t.TempDir())
	ctx := context.Background()

	// 预先存在一个已填 EAB 的 ZeroSSL（用户配置过）
	pre, err := svc.Create(ctx, CreateCAInput{
		Name:         "ZeroSSL",
		DirectoryURL: "https://acme.zerossl.com/v2/DV90/directory",
		AccountEmail: "user@example.com",
		EabKid:       "user-kid",
		EabHmac:      "user-hmac",
		IsBuiltin:    true,
	})
	if err != nil {
		t.Fatalf("Create 失败: %v", err)
	}

	if err := svc.SeedDefaults(ctx); err != nil {
		t.Fatalf("SeedDefaults 失败: %v", err)
	}
	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	// 5 个默认 + 1 个预先存在（ZeroSSL 因 URL 相同被跳过）
	if len(list) != 5 {
		t.Fatalf("应按 directory_url 去重后共 5 个，实际 %d", len(list))
	}
	got, err := svc.GetByID(ctx, pre.ID)
	if err != nil {
		t.Fatalf("GetByID 失败: %v", err)
	}
	if got.EabKid != "user-kid" || got.AccountEmail != "user@example.com" {
		t.Fatalf("用户配置被覆盖: %+v", got)
	}

	// seeded 标记生效后再次调用直接返回
	if err := svc.SeedDefaults(ctx); err != nil {
		t.Fatalf("第二次 SeedDefaults 失败: %v", err)
	}
}

// TestTestConnection 覆盖连接测试。
func TestTestConnection(t *testing.T) {
	client := setupTestDB(t)
	svc := NewCAService(client, t.TempDir())
	ctx := context.Background()

	created, err := svc.Create(ctx, CreateCAInput{Name: "LE", DirectoryURL: "https://acme-v02.api.letsencrypt.org/directory"})
	if err != nil {
		t.Fatalf("Create 失败: %v", err)
	}

	msg, err := svc.TestConnection(ctx, created.ID)
	if err != nil {
		t.Fatalf("TestConnection 失败: %v", err)
	}
	if !strings.Contains(msg, "LE") && !strings.Contains(msg, "acme-v02") {
		t.Fatalf("提示信息应包含 CA 名称或 URL，实际 %q", msg)
	}
	if strings.Contains(msg, "%!") || strings.Contains(msg, "error.") {
		t.Fatalf("提示信息格式化异常: %q", msg)
	}

	if _, err := svc.TestConnection(ctx, 9999); err == nil {
		t.Fatal("不存在的 CA 应报错")
	}
}

// TestCheckDirectoryURL 覆盖 ACME 目录地址校验（全部走 httptest，不访问外网）。
func TestCheckDirectoryURL(t *testing.T) {
	client := setupTestDB(t)
	svc := NewCAService(client, t.TempDir())
	ctx := context.Background()

	t.Run("空地址", func(t *testing.T) {
		if _, err := svc.CheckDirectoryURL(ctx, ""); err == nil {
			t.Fatal("空地址应报错")
		}
	})

	t.Run("非法地址", func(t *testing.T) {
		if _, err := svc.CheckDirectoryURL(ctx, "://bad-url"); err == nil {
			t.Fatal("非法地址应报错")
		}
	})

	t.Run("有效目录", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("User-Agent") == "" {
				t.Error("应携带 User-Agent")
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"newNonce":"https://x/nonce","newAccount":"https://x/account"}`))
		}))
		defer srv.Close()

		msg, err := svc.CheckDirectoryURL(ctx, srv.URL)
		if err != nil {
			t.Fatalf("有效目录不应报错: %v", err)
		}
		if msg == "" || strings.Contains(msg, "error.") {
			t.Fatalf("提示信息不符: %q", msg)
		}
	})

	t.Run("非 2xx", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		if _, err := svc.CheckDirectoryURL(ctx, srv.URL); err == nil {
			t.Fatal("非 2xx 应报错")
		}
	})

	t.Run("非JSON", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("not json"))
		}))
		defer srv.Close()

		if _, err := svc.CheckDirectoryURL(ctx, srv.URL); err == nil {
			t.Fatal("非 JSON 应报错")
		}
	})

	t.Run("缺少newNonce", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"newAccount":"https://x/account"}`))
		}))
		defer srv.Close()

		if _, err := svc.CheckDirectoryURL(ctx, srv.URL); err == nil {
			t.Fatal("缺少 newNonce 应报错")
		}
	})

	t.Run("注入设置后使用统一客户端", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"newNonce":"https://x/nonce"}`))
		}))
		defer srv.Close()

		var got settings.Settings
		svc.SetSettingsProvider(func() settings.Settings { return got })
		if _, err := svc.CheckDirectoryURL(ctx, srv.URL); err != nil {
			t.Fatalf("注入设置后校验失败: %v", err)
		}
	})
}

// TestListActive_Empty 覆盖无启用 CA 的场景。
func TestListActive_Empty(t *testing.T) {
	client := setupTestDB(t)
	svc := NewCAService(client, t.TempDir())
	ctx := context.Background()

	list, err := svc.ListActive(ctx)
	if err != nil {
		t.Fatalf("ListActive 失败: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("应为空列表，实际 %d", len(list))
	}

	// 非启用状态的 CA 不应出现
	if _, err := svc.Create(ctx, CreateCAInput{Name: "未启用", DirectoryURL: "https://e.com/d", IsBuiltin: false}); err != nil {
		t.Fatalf("Create 失败: %v", err)
	}
	count, err := client.CA.Query().Where(ca.IsActiveEQ(true)).Count(ctx)
	if err != nil {
		t.Fatalf("统计失败: %v", err)
	}
	if count != 0 {
		t.Fatalf("新建 CA 默认不应启用，实际 %d", count)
	}
}
