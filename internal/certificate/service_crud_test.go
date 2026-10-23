package certificate

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"entgo.io/ent/dialect"

	"cnb.cool/dtapp/certflow/internal/ent"
	"cnb.cool/dtapp/certflow/internal/ent/certificate"
	"cnb.cool/dtapp/certflow/internal/ent/enttest"
	"cnb.cool/dtapp/certflow/internal/ent/schema"
	"cnb.cool/dtapp/certflow/internal/sqlite"
)

// newTestService 建临时库 + 临时证书目录。
func newTestService(t *testing.T) (*CertificateService, *ent.Client) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "certflow.db")
	client := enttest.Open(t, dialect.SQLite, sqlite.BuildDSN(path))
	t.Cleanup(func() { _ = client.Close() })
	return NewCertificateService(client, filepath.Join(dir, "certs")), client
}

// certOpt 用函数式选项构造各类状态的证书，避免每个用例写一长串 Create 字段。
type certOpt func(*ent.CertificateCreate)

func withNotAfter(d time.Duration) certOpt {
	return func(c *ent.CertificateCreate) {
		c.SetNotBefore(time.Now().Add(-time.Hour)).SetNotAfter(time.Now().Add(d))
	}
}

func withStatus(s certificate.Status) certOpt {
	return func(c *ent.CertificateCreate) { c.SetStatus(s) }
}

func withAutoRenew(v bool) certOpt {
	return func(c *ent.CertificateCreate) { c.SetAutoRenew(v) }
}

func withRenewalDays(d int) certOpt {
	return func(c *ent.CertificateCreate) { c.SetRenewalDays(d) }
}

func createCert(t *testing.T, client *ent.Client, domain string, opts ...certOpt) *ent.Certificate {
	t.Helper()
	b := client.Certificate.Create().
		SetDomain(domain).
		SetSans([]string{domain, "www." + domain}).
		SetIssuer("Test CA").
		SetStatus(certificate.StatusActive).
		SetCertContent("-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----").
		SetKeyContent("-----BEGIN EC PRIVATE KEY-----\nMHcC\n-----END EC PRIVATE KEY-----").
		SetNotBefore(time.Now().Add(-time.Hour)).
		SetNotAfter(time.Now().Add(60 * 24 * time.Hour))
	for _, o := range opts {
		o(b)
	}
	c, err := b.Save(context.Background())
	if err != nil {
		t.Fatalf("create cert %s: %v", domain, err)
	}
	return c
}

func TestService_GetByID(t *testing.T) {
	svc, client := newTestService(t)
	ctx := context.Background()

	created := createCert(t, client, "a.example.com")

	got, err := svc.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID err: %v", err)
	}
	if got.ID != created.ID || got.Domain != "a.example.com" {
		t.Fatalf("结果不符: %+v", got)
	}

	// 不存在 ID：应被翻译成 i18n 文案而不是把 ent 原始错误抛给前端
	if _, err := svc.GetByID(ctx, 999999); err == nil {
		t.Fatal("不存在的 ID 应返回错误")
	}
}

func TestService_List(t *testing.T) {
	svc, client := newTestService(t)
	ctx := context.Background()

	if list, err := svc.List(ctx); err != nil || len(list) != 0 {
		t.Fatalf("空库应返回空列表: %v %v", list, err)
	}

	createCert(t, client, "a.example.com")
	createCert(t, client, "b.example.com")

	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List err: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("应有 2 条, 实际 %d", len(list))
	}
}

// TestService_ListExpiring 只应返回「active 且 未过期 且 在 N 天内到期」的证书。
// 三个条件是调度器发到期提醒的依据，多算会误报、少算会漏报 —— 都是线上事故。
func TestService_ListExpiring(t *testing.T) {
	svc, client := newTestService(t)
	ctx := context.Background()

	in10 := createCert(t, client, "in10.example.com", withNotAfter(10*24*time.Hour))
	createCert(t, client, "in60.example.com", withNotAfter(60*24*time.Hour))  // 超出 30 天窗口
	createCert(t, client, "expired.example.com", withNotAfter(-24*time.Hour)) // 已过期
	createCert(t, client, "pending.example.com", withNotAfter(5*24*time.Hour),
		withStatus(certificate.StatusPending)) // 非 active
	revoked := createCert(t, client, "revoked.example.com", withNotAfter(5*24*time.Hour),
		withStatus(certificate.StatusRevoked)) // 非 active

	list, err := svc.ListExpiring(ctx, 30)
	if err != nil {
		t.Fatalf("ListExpiring err: %v", err)
	}
	if len(list) != 1 || list[0].ID != in10.ID {
		ids := make([]int, 0, len(list))
		for _, c := range list {
			ids = append(ids, c.ID)
		}
		t.Fatalf("应只返回 in10(=%d), 实际 %v（revoked=%d 也不该出现）", in10.ID, ids, revoked.ID)
	}
}

// TestService_ListAutoRenew 手动 DNS 证书（无 DNS 提供商）必须被排除：
// 它们需要人工加 TXT 记录，调度器自动续期一定失败，只会刷一堆错误通知。
func TestService_ListAutoRenew(t *testing.T) {
	svc, client := newTestService(t)
	ctx := context.Background()

	provider := client.DNSProvider.Create().
		SetName("CF").
		SetProviderType("cloudflare").
		SetConfig(schema.DNSProviderConfig{APIToken: "t"}).
		SaveX(ctx)

	// 有 DNS 提供商 + 快到期 + 自动续期 → 应续期
	renewable := client.Certificate.Create().
		SetDomain("renew.example.com").
		SetSans([]string{"renew.example.com"}).
		SetNotBefore(time.Now().Add(-time.Hour)).
		SetNotAfter(time.Now().Add(10 * 24 * time.Hour)).
		SetStatus(certificate.StatusActive).
		SetAutoRenew(true).
		SetRenewalDays(30).
		SetDNSProvider(provider).
		SaveX(ctx)

	// 有 DNS 提供商 + 还很久才到期 → 不该续
	client.Certificate.Create().
		SetDomain("fresh.example.com").
		SetSans([]string{"fresh.example.com"}).
		SetNotBefore(time.Now().Add(-time.Hour)).
		SetNotAfter(time.Now().Add(80 * 24 * time.Hour)).
		SetStatus(certificate.StatusActive).
		SetAutoRenew(true).
		SetRenewalDays(30).
		SetDNSProvider(provider).
		SaveX(ctx)

	// 手动 DNS（无 provider）→ 必须排除
	createCert(t, client, "manual.example.com", withNotAfter(5*24*time.Hour),
		withStatus(certificate.StatusActive), withAutoRenew(true), withRenewalDays(30))

	// 有 provider 但关掉自动续期 → 排除
	client.Certificate.Create().
		SetDomain("off.example.com").
		SetSans([]string{"off.example.com"}).
		SetNotBefore(time.Now().Add(-time.Hour)).
		SetNotAfter(time.Now().Add(5 * 24 * time.Hour)).
		SetStatus(certificate.StatusActive).
		SetAutoRenew(false).
		SetRenewalDays(30).
		SetDNSProvider(provider).
		SaveX(ctx)

	list, err := svc.ListAutoRenew(ctx)
	if err != nil {
		t.Fatalf("ListAutoRenew err: %v", err)
	}
	if len(list) != 1 || list[0].ID != renewable.ID {
		domains := make([]string, 0, len(list))
		for _, c := range list {
			domains = append(domains, c.Domain)
		}
		t.Fatalf("应只返回 renew.example.com, 实际 %v", domains)
	}
}

func TestService_UpdateSettings(t *testing.T) {
	svc, client := newTestService(t)
	ctx := context.Background()

	created := createCert(t, client, "a.example.com")

	if err := svc.UpdateSettings(ctx, created.ID, false, 15); err != nil {
		t.Fatalf("UpdateSettings err: %v", err)
	}
	got, err := svc.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID err: %v", err)
	}
	if got.AutoRenew || got.RenewalDays != 15 {
		t.Fatalf("设置未生效: autoRenew=%v renewalDays=%d", got.AutoRenew, got.RenewalDays)
	}

	if err := svc.UpdateSettings(ctx, 999999, true, 30); err == nil {
		t.Fatal("不存在的记录应返回错误")
	}
}

func TestService_Delete(t *testing.T) {
	svc, client := newTestService(t)
	ctx := context.Background()

	created := createCert(t, client, "a.example.com")

	if err := svc.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete err: %v", err)
	}
	if _, err := svc.GetByID(ctx, created.ID); err == nil {
		t.Fatal("删除后不应还能查到")
	}
	if err := svc.Delete(ctx, created.ID); err == nil {
		t.Fatal("重复删除应返回错误")
	}
}

func TestService_GetCertificateContent(t *testing.T) {
	svc, client := newTestService(t)
	ctx := context.Background()

	created := createCert(t, client, "a.example.com")

	certPEM, keyPEM, err := svc.GetCertificateContent(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetCertificateContent err: %v", err)
	}
	if certPEM != created.CertContent || keyPEM != created.KeyContent {
		t.Fatal("返回内容与库中不一致")
	}

	if _, _, err := svc.GetCertificateContent(ctx, 999999); err == nil {
		t.Fatal("不存在的 ID 应返回错误")
	}
}

func TestService_GetCertificateInfoAndExport(t *testing.T) {
	svc, client := newTestService(t)
	ctx := context.Background()

	created := createCert(t, client, "a.example.com")

	info, err := svc.GetCertificateInfo(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetCertificateInfo err: %v", err)
	}
	// 字段名是前端契约，少一个前端就显示空白
	for _, key := range []string{
		"id", "domain", "sans", "cert_content", "key_content", "issuer",
		"not_before", "not_after", "status", "auto_renew", "renewal_days",
		"last_error", "last_renewed_at", "created_at", "updated_at",
	} {
		if _, ok := info[key]; !ok {
			t.Errorf("GetCertificateInfo 缺少字段 %q", key)
		}
	}
	if info["domain"] != "a.example.com" {
		t.Fatalf("domain 不符: %v", info["domain"])
	}

	raw, err := svc.ExportCertificate(ctx, created.ID)
	if err != nil {
		t.Fatalf("ExportCertificate err: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("导出内容不是合法 JSON: %v", err)
	}
	if decoded["domain"] != "a.example.com" {
		t.Fatalf("导出内容 domain 不符: %v", decoded["domain"])
	}

	if _, err := svc.ExportCertificate(ctx, 999999); err == nil {
		t.Fatal("导出不存在的证书应返回错误")
	}
}

// TestService_RevokeCertificate_MissingCA 撤销必须走 CA 配置，没有 CA 时要给出可读错误
// 而不是 panic（这里是「恢复证书」入口的常见失败路径）。
func TestService_RevokeCertificate_MissingCA(t *testing.T) {
	svc, client := newTestService(t)
	ctx := context.Background()

	created := createCert(t, client, "a.example.com")
	if err := svc.RevokeCertificate(ctx, created.ID); err == nil {
		t.Fatal("未关联 CA 时应返回错误")
	}
	if err := svc.RevokeCertificate(ctx, 999999); err == nil {
		t.Fatal("不存在的证书应返回错误")
	}
}
