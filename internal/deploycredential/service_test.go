package deploycredential

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"entgo.io/ent/dialect"

	"cnb.cool/dtapp/certflow/internal/ent"
	"cnb.cool/dtapp/certflow/internal/ent/enttest"
	"cnb.cool/dtapp/certflow/internal/sqlite"
)

// newTestService 建一个独立的临时库 + 迁移 + Service。
// 用文件而不是 :memory:：sqlite 的内存库是「每连接一个」，连接池一换就看不到表。
func newTestService(t *testing.T) *Service {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cred.db")
	client := enttest.Open(t, dialect.SQLite, sqlite.BuildDSN(path))
	t.Cleanup(func() { _ = client.Close() })
	return NewService(client)
}

func sampleInput() CreateDeployCredentialInput {
	return CreateDeployCredentialInput{
		Name:         "阿里云-主账号",
		ProviderType: "aliyun",
		Config:       DeployCredentialConfig{AccessKeyID: "ak", AccessKeySecret: "sk", RegionID: "cn-hangzhou"},
		Comment:      "备注",
	}
}

// parseRFC3339 校验时间字段格式：前端直接显示字符串，格式错了会显示成一串数字。
func parseRFC3339(t *testing.T, field, v string) time.Time {
	t.Helper()
	if v == "" {
		t.Fatalf("%s 不应为空", field)
	}
	tm, err := time.Parse(time.RFC3339, v)
	if err != nil {
		t.Fatalf("%s 不是 RFC3339: %q (%v)", field, v, err)
	}
	return tm
}

func TestService_CRUD(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	created, err := svc.Create(ctx, sampleInput())
	if err != nil {
		t.Fatalf("Create err: %v", err)
	}
	if created.ID == 0 {
		t.Fatal("Create 应返回自增 ID")
	}
	if created.Name != "阿里云-主账号" || created.ProviderType != "aliyun" {
		t.Fatalf("Create 回写字段不符: %+v", created)
	}
	if created.Config.AccessKeyID != "ak" || created.Config.RegionID != "cn-hangzhou" {
		t.Fatalf("Config 未持久化: %+v", created.Config)
	}
	if created.IsActive {
		t.Error("新建凭证默认应为未启用")
	}
	parseRFC3339(t, "created_at", created.CreatedAt)
	parseRFC3339(t, "updated_at", created.UpdatedAt)

	got, err := svc.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID err: %v", err)
	}
	if got.ID != created.ID || got.Comment != "备注" {
		t.Fatalf("GetByID 结果不符: %+v", got)
	}

	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List err: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("List 应有 1 条, 实际 %d", len(list))
	}

	// SetActive
	active, err := svc.SetActive(ctx, created.ID, true)
	if err != nil {
		t.Fatalf("SetActive err: %v", err)
	}
	if !active.IsActive {
		t.Error("SetActive(true) 后应为启用")
	}

	// Update
	updated, err := svc.Update(ctx, created.ID, UpdateDeployCredentialInput{
		Name:         "改名后",
		ProviderType: "tencentcloud",
		Config:       DeployCredentialConfig{SecretID: "id2", SecretKey: "key2", Region: "ap-guangzhou"},
		Comment:      "新备注",
	})
	if err != nil {
		t.Fatalf("Update err: %v", err)
	}
	if updated.Name != "改名后" || updated.ProviderType != "tencentcloud" {
		t.Fatalf("Update 未生效: %+v", updated)
	}
	if updated.Config.SecretID != "id2" || updated.Config.Region != "ap-guangzhou" {
		t.Fatalf("Update 未替换 Config: %+v", updated.Config)
	}
	if !updated.IsActive {
		t.Error("Update 不应重置 is_active")
	}

	// Delete + 再查应报 NotFound
	if err := svc.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete err: %v", err)
	}
	if _, err := svc.GetByID(ctx, created.ID); !ent.IsNotFound(err) {
		t.Fatalf("删除后应 NotFound, 实际 %v", err)
	}
	if _, err := svc.SetActive(ctx, created.ID, true); !ent.IsNotFound(err) {
		t.Fatalf("对已删除记录 SetActive 应 NotFound, 实际 %v", err)
	}
	list, err = svc.List(ctx)
	if err != nil {
		t.Fatalf("List err: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("删除后列表应为空, 实际 %d", len(list))
	}
}

func TestService_ListOrderedByCreatedAt(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	names := []string{"第一条", "第二条", "第三条"}
	for _, n := range names {
		in := sampleInput()
		in.Name = n
		if _, err := svc.Create(ctx, in); err != nil {
			t.Fatalf("Create(%s) err: %v", n, err)
		}
	}

	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List err: %v", err)
	}
	if len(list) != len(names) {
		t.Fatalf("应有 %d 条, 实际 %d", len(names), len(list))
	}
	for i, want := range names {
		if list[i].Name != want {
			t.Fatalf("第 %d 条应为 %q, 实际 %q", i, want, list[i].Name)
		}
	}
}

func TestService_GetByID_NotFound(t *testing.T) {
	svc := newTestService(t)
	if _, err := svc.GetByID(context.Background(), 9999); !ent.IsNotFound(err) {
		t.Fatalf("不存在的 ID 应 NotFound, 实际 %v", err)
	}
}

func TestService_Create_ValidationErrors(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	// name 为空 → schema 的 NotEmpty 约束应拒绝
	in := sampleInput()
	in.Name = ""
	if _, err := svc.Create(ctx, in); err == nil {
		t.Fatal("空名称应被拒绝")
	}

	// provider_type 不在枚举内 → Enum 约束应拒绝
	in = sampleInput()
	in.ProviderType = "not-a-provider"
	if _, err := svc.Create(ctx, in); err == nil {
		t.Fatal("非法 provider_type 应被拒绝")
	}
}

func TestService_Delete_NotFound(t *testing.T) {
	svc := newTestService(t)
	if err := svc.Delete(context.Background(), 9999); !ent.IsNotFound(err) {
		t.Fatalf("删除不存在记录应 NotFound, 实际 %v", err)
	}
}

func TestService_Update_NotFound(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.Update(context.Background(), 9999, UpdateDeployCredentialInput{
		Name:         "x",
		ProviderType: "aliyun",
	})
	if !ent.IsNotFound(err) {
		t.Fatalf("更新不存在记录应 NotFound, 实际 %v", err)
	}
}
