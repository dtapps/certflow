package dnsprovider

import (
	"context"
	"strings"
	"testing"

	"cnb.cool/dtapp/certflow/internal/ent"
	"cnb.cool/dtapp/certflow/internal/ent/dnsprovider"
)

// mkProvider 创建一条 DNS 提供商记录。
func mkProvider(t *testing.T, svc *DNSProviderService, name, providerType string, cfg DNSProviderConfig, comment string, active bool) int {
	t.Helper()
	p, err := svc.Create(context.Background(), CreateDNSProviderInput{
		Name:         name,
		ProviderType: providerType,
		Config:       cfg,
		Comment:      comment,
	})
	if err != nil {
		t.Fatalf("创建 DNS 提供商失败: %v", err)
	}
	// Create 不写入 IsActive，这里显式设置目标状态
	if _, err := svc.SetActive(context.Background(), p.ID, active); err != nil {
		t.Fatalf("设置启用状态失败: %v", err)
	}
	return p.ID
}

// hasProvider 判断列表中是否包含指定 ID。
func hasProvider(list []*ent.DNSProvider, id int) bool {
	for _, p := range list {
		if p.ID == id {
			return true
		}
	}
	return false
}

// TestSetActive 覆盖启用/停用与不存在 ID 的错误分支。
func TestSetActive(t *testing.T) {
	svc := NewDNSProviderService(setupTestDB(t))
	ctx := context.Background()
	id := mkProvider(t, svc, "开关测试", "aliyun", DNSProviderConfig{AccessKeyID: "ak"}, "", true)

	p, err := svc.SetActive(ctx, id, false)
	if err != nil {
		t.Fatalf("停用失败: %v", err)
	}
	if p.IsActive {
		t.Fatal("停用后 IsActive 应为 false")
	}

	p, err = svc.SetActive(ctx, id, true)
	if err != nil {
		t.Fatalf("启用失败: %v", err)
	}
	if !p.IsActive {
		t.Fatal("启用后 IsActive 应为 true")
	}

	// 不存在 → 翻译过的 not found 错误
	_, err = svc.SetActive(ctx, 999999, true)
	if err == nil {
		t.Fatal("不存在的 ID 应报错")
	}
	msg := err.Error()
	if strings.Contains(msg, "error.") || strings.Contains(msg, "%!") {
		t.Fatalf("错误文案未翻译: %q", msg)
	}
}

// TestListActive 覆盖只返回启用记录。
func TestListActive(t *testing.T) {
	svc := NewDNSProviderService(setupTestDB(t))
	ctx := context.Background()

	activeID := mkProvider(t, svc, "启用中", "aliyun", DNSProviderConfig{AccessKeyID: "ak"}, "", true)
	inactiveID := mkProvider(t, svc, "已停用", "aliyun", DNSProviderConfig{AccessKeyID: "ak"}, "", false)

	list, err := svc.ListActive(ctx)
	if err != nil {
		t.Fatalf("ListActive 失败: %v", err)
	}
	if !hasProvider(list, activeID) {
		t.Fatal("启用记录应出现在结果中")
	}
	if hasProvider(list, inactiveID) {
		t.Fatal("停用记录不应出现在结果中")
	}
	for _, p := range list {
		if !p.IsActive {
			t.Fatalf("ListActive 返回了停用记录: %d", p.ID)
		}
	}
}

// TestListByType 覆盖按厂商类型过滤。
func TestListByType(t *testing.T) {
	svc := NewDNSProviderService(setupTestDB(t))
	ctx := context.Background()

	cloudflareID := mkProvider(t, svc, "CF", "cloudflare", DNSProviderConfig{APIToken: "t"}, "", true)
	aliyunID := mkProvider(t, svc, "AL", "aliyun", DNSProviderConfig{AccessKeyID: "ak", AccessKeySecret: "sk"}, "", true)

	list, err := svc.ListByType(ctx, "cloudflare")
	if err != nil {
		t.Fatalf("ListByType 失败: %v", err)
	}
	if !hasProvider(list, cloudflareID) {
		t.Fatal("同类型记录应被返回")
	}
	if hasProvider(list, aliyunID) {
		t.Fatal("其它类型记录不应被返回")
	}
	for _, p := range list {
		if p.ProviderType != dnsprovider.ProviderTypeCloudflare {
			t.Fatalf("返回了错误类型: %s", p.ProviderType)
		}
	}

	// 未知类型 → 空结果且不报错
	list, err = svc.ListByType(ctx, "no-such-provider-type")
	if err != nil {
		t.Fatalf("未知类型不应报错: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("未知类型应返回空，实际 %d", len(list))
	}
}

// TestUpdate 覆盖字段更新、空值不覆盖、不存在 ID 的错误分支。
func TestUpdate(t *testing.T) {
	svc := NewDNSProviderService(setupTestDB(t))
	ctx := context.Background()
	id := mkProvider(t, svc, "原名", "aliyun", DNSProviderConfig{AccessKeyID: "old"}, "原备注", true)

	// 只改名字和配置，空字段不覆盖
	p, err := svc.Update(ctx, id, UpdateDNSProviderInput{
		Name:   "新名字",
		Config: DNSProviderConfig{AccessKeyID: "new"},
	})
	if err != nil {
		t.Fatalf("更新失败: %v", err)
	}
	if p.Name != "新名字" {
		t.Fatalf("Name = %q", p.Name)
	}
	if p.Config.AccessKeyID != "new" {
		t.Fatalf("Config 未更新: %+v", p.Config)
	}
	if p.Comment != "原备注" {
		t.Fatalf("空 Comment 不应覆盖原值: %q", p.Comment)
	}
	if p.ProviderType != dnsprovider.ProviderTypeAliyun {
		t.Fatalf("空 ProviderType 不应覆盖原值: %s", p.ProviderType)
	}

	// 改类型与备注
	p, err = svc.Update(ctx, id, UpdateDNSProviderInput{ProviderType: "cloudflare", Comment: "新备注"})
	if err != nil {
		t.Fatalf("更新失败: %v", err)
	}
	if p.ProviderType != dnsprovider.ProviderTypeCloudflare {
		t.Fatalf("ProviderType = %s", p.ProviderType)
	}
	if p.Comment != "新备注" {
		t.Fatalf("Comment = %q", p.Comment)
	}

	// 空配置不覆盖（isConfigEmpty 生效）
	p, err = svc.Update(ctx, id, UpdateDNSProviderInput{Config: DNSProviderConfig{}})
	if err != nil {
		t.Fatalf("更新失败: %v", err)
	}
	if p.Config.AccessKeyID != "new" {
		t.Fatalf("空配置不应覆盖原值: %+v", p.Config)
	}

	// 不存在 → 翻译过的 not found 错误
	_, err = svc.Update(ctx, 999999, UpdateDNSProviderInput{Name: "x"})
	if err == nil {
		t.Fatal("不存在的 ID 应报错")
	}
	if msg := err.Error(); strings.Contains(msg, "error.") || strings.Contains(msg, "%!") {
		t.Fatalf("错误文案未翻译: %q", msg)
	}
}
