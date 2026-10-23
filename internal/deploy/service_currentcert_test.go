package deploy

import (
	"context"
	"testing"

	"cnb.cool/dtapp/certflow/internal/ent/deploytarget"
	"cnb.cool/dtapp/certflow/internal/ent/schema"
)

// TestParseConfigStringSlice 覆盖配置字符串切片的容错解析。
func TestParseConfigStringSlice(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"   ", nil},
		{`["a","b"]`, []string{"a", "b"}},
		{`[]`, []string{}},
		{"a.example.com", []string{"a.example.com"}},
		{"a, b , ,c", []string{"a", "b", "c"}},
	}
	for _, tt := range tests {
		got := parseConfigStringSlice(tt.in)
		if len(got) != len(tt.want) {
			t.Fatalf("parseConfigStringSlice(%q) = %v, want %v", tt.in, got, tt.want)
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Fatalf("parseConfigStringSlice(%q) = %v, want %v", tt.in, got, tt.want)
			}
		}
	}
}

// TestService_GetCurrentCerts_StoredConfig 覆盖已存站点/域名配置下的查询与降级。
func TestService_GetCurrentCerts_StoredConfig(t *testing.T) {
	svc, client := newDeployTestService(t)
	ctx := context.Background()
	svc.SetNotificationService(nil)

	aliyunCred := mustCreateDeployCredential(t, client, "阿里云", "aliyun", aliyunCredConfig())
	btCred := mustCreateDeployCredential(t, client, "宝塔", "btpanel", btPanelCredConfig())

	t.Run("目标不存在", func(t *testing.T) {
		if _, err := svc.GetCurrentCerts(ctx, 9999); err == nil {
			t.Fatal("目标不存在应报错")
		}
	})

	t.Run("凭证不可用", func(t *testing.T) {
		tg := mustCreateTarget(t, client, "无凭证", deploytarget.ProviderTypeAliyun,
			withCredSourceDeployCred(aliyunCred.ID))
		if err := client.DeployTarget.UpdateOneID(tg.ID).ClearDeployCredential().Exec(ctx); err != nil {
			t.Fatalf("清空外键失败: %v", err)
		}
		if _, err := svc.GetCurrentCerts(ctx, tg.ID); err == nil {
			t.Fatal("凭证不可用应报错")
		}
	})

	t.Run("厂商未注册部署器", func(t *testing.T) {
		withRegistry(t, "aliyun", nil)
		tg := mustCreateTarget(t, client, "无部署器", deploytarget.ProviderTypeAliyun,
			withCredSourceDeployCred(aliyunCred.ID),
			withDomainConfig(DeployTargetConfig{Domains: []string{"q.example.com"}}))
		if _, err := svc.GetCurrentCerts(ctx, tg.ID); err == nil {
			t.Fatal("未注册部署器应报错")
		}
	})

	t.Run("部署器未实现查询接口时降级为不支持", func(t *testing.T) {
		fake := &fakeDeployer{provider: "aliyun"}
		withRegistry(t, "aliyun", fake)
		tg := mustCreateTarget(t, client, "不支持", deploytarget.ProviderTypeAliyun,
			withCredSourceDeployCred(aliyunCred.ID),
			withDomainConfig(DeployTargetConfig{Domains: []string{"n1.example.com", "n2.example.com"}}))

		res, err := svc.GetCurrentCerts(ctx, tg.ID)
		if err != nil {
			t.Fatalf("查询失败: %v", err)
		}
		if len(res) != 2 {
			t.Fatalf("应按域名生成 2 个资源，实际 %d", len(res))
		}
		for key, r := range res {
			if r.Supported || r.CurrentCert != nil {
				t.Fatalf("资源 %s 应标记为不支持: %+v", key, r)
			}
		}
		if fake.listCalls != 0 {
			t.Fatal("配置已存域名时不应回落到实时拉取")
		}
	})

	t.Run("云目标按域名查询成功", func(t *testing.T) {
		fake := &fakeDeployerFull{fakeDeployer: &fakeDeployer{provider: "aliyun"}}
		withRegistry(t, "aliyun", fake)
		tg := mustCreateTarget(t, client, "云查询", deploytarget.ProviderTypeAliyun,
			withCredSourceDeployCred(aliyunCred.ID),
			withDomainConfig(DeployTargetConfig{Domains: []string{"c.example.com"}}))

		res, err := svc.GetCurrentCerts(ctx, tg.ID)
		if err != nil {
			t.Fatalf("查询失败: %v", err)
		}
		r, ok := res["c.example.com"]
		if !ok || !r.Supported || r.CurrentCert == nil || r.CurrentCert.CommonName != "current.example.com" {
			t.Fatalf("查询结果不符: %+v", res)
		}
		if fake.currentCalls != 1 || fake.batchCalls != 1 {
			t.Fatalf("应查询 1 次且调用批量前置钩子 1 次，实际 %d/%d", fake.currentCalls, fake.batchCalls)
		}
	})

	t.Run("面板目标按站点ID查询", func(t *testing.T) {
		fake := &fakeDeployerFull{fakeDeployer: &fakeDeployer{provider: "btpanel"}}
		withRegistry(t, "btpanel", fake)
		tg := mustCreateTarget(t, client, "面板查询", deploytarget.ProviderTypeBtpanel,
			withCredSourceDeployCred(btCred.ID),
			withDomainConfig(DeployTargetConfig{SiteName: []string{"站点A", "站点B"}, SiteID: []string{"1"}}))

		res, err := svc.GetCurrentCerts(ctx, tg.ID)
		if err != nil {
			t.Fatalf("查询失败: %v", err)
		}
		if len(res) != 2 {
			t.Fatalf("应按站点生成 2 个资源，实际 %d", len(res))
		}
		// 站点 ID 缺失时 key 回退为站点名
		if _, ok := res["1"]; !ok {
			t.Fatalf("站点A 应以 ID 作为 key: %+v", res)
		}
		if _, ok := res["站点B"]; !ok {
			t.Fatalf("站点B 缺少 ID 时应以名称作为 key: %+v", res)
		}
	})

	t.Run("查询报错写入 Error 字段", func(t *testing.T) {
		fake := &fakeDeployerFull{fakeDeployer: &fakeDeployer{provider: "aliyun"}, currentErr: errBoom}
		withRegistry(t, "aliyun", fake)
		tg := mustCreateTarget(t, client, "查询报错", deploytarget.ProviderTypeAliyun,
			withCredSourceDeployCred(aliyunCred.ID),
			withDomainConfig(DeployTargetConfig{Domains: []string{"e.example.com"}}))

		res, err := svc.GetCurrentCerts(ctx, tg.ID)
		if err != nil {
			t.Fatalf("查询失败: %v", err)
		}
		if r := res["e.example.com"]; r == nil || r.Error == "" || r.CurrentCert != nil {
			t.Fatalf("错误未写入结果: %+v", r)
		}
	})
}

// TestService_GetCurrentCerts_LiveResources 覆盖目标未存站点/域名时按凭证实时构造资源。
func TestService_GetCurrentCerts_LiveResources(t *testing.T) {
	svc, client := newDeployTestService(t)
	ctx := context.Background()

	aliyunCred := mustCreateDeployCredential(t, client, "阿里云", "aliyun", aliyunCredConfig())
	btCred := mustCreateDeployCredential(t, client, "宝塔", "btpanel", btPanelCredConfig())

	t.Run("面板实时站点列表", func(t *testing.T) {
		fake := &fakeDeployerFull{fakeDeployer: &fakeDeployer{provider: "btpanel",
			sites: []string{"站点A||1", "站点B||2"}}}
		withRegistry(t, "btpanel", fake)
		tg := mustCreateTarget(t, client, "面板实时", deploytarget.ProviderTypeBtpanel,
			withCredSourceDeployCred(btCred.ID))

		res, err := svc.GetCurrentCerts(ctx, tg.ID)
		if err != nil {
			t.Fatalf("查询失败: %v", err)
		}
		if len(res) != 2 {
			t.Fatalf("应按实时站点生成 2 个资源，实际 %d", len(res))
		}
		if r := res["1"]; r == nil || !r.Supported {
			t.Fatalf("站点资源未生成: %+v", res)
		}
	})

	t.Run("ESA 按站点拉加速域名", func(t *testing.T) {
		fake := &fakeDeployerFull{fakeDeployer: &fakeDeployer{provider: "aliyun",
			sites: []string{"站点A||site-1"}, domains: []string{"esa.example.com"}}}
		withRegistry(t, "aliyun", fake)
		tg := mustCreateTarget(t, client, "ESA实时", deploytarget.ProviderTypeAliyun,
			withCredSourceDeployCred(aliyunCred.ID),
			withDeployServiceParam("esa"))

		res, err := svc.GetCurrentCerts(ctx, tg.ID)
		if err != nil {
			t.Fatalf("查询失败: %v", err)
		}
		if r := res["esa.example.com"]; r == nil || !r.Supported {
			t.Fatalf("ESA 域名资源未生成: %+v", res)
		}
		if fake.lastSvc["site_id"] != "site-1" || fake.lastSvc["domain"] != "esa.example.com" {
			t.Fatalf("ESA 资源 svcConfig 不符: %+v", fake.lastSvc)
		}
	})

	t.Run("EdgeOne 按站点拉加速域名", func(t *testing.T) {
		fake := &fakeDeployerFull{fakeDeployer: &fakeDeployer{provider: "tencentcloud",
			sites: []string{"站点EO||zone-1"}, domains: []string{"eo.example.com"}}}
		withRegistry(t, "tencentcloud", fake)
		tcCred := mustCreateDeployCredential(t, client, "腾讯云", "tencentcloud",
			schema.DeployCredentialConfig{SecretID: "sid", SecretKey: "skey"})
		tg := mustCreateTarget(t, client, "EO实时", deploytarget.ProviderTypeTencentcloud,
			withCredSourceDeployCred(tcCred.ID),
			withDeployServiceParam("edgeone"))

		res, err := svc.GetCurrentCerts(ctx, tg.ID)
		if err != nil {
			t.Fatalf("查询失败: %v", err)
		}
		if r := res["eo.example.com"]; r == nil || !r.Supported {
			t.Fatalf("EdgeOne 域名资源未生成: %+v", res)
		}
		if fake.lastSvc["zone_id"] != "zone-1" {
			t.Fatalf("EdgeOne 资源 svcConfig 不符: %+v", fake.lastSvc)
		}
	})

	t.Run("云厂商实时域名列表", func(t *testing.T) {
		fake := &fakeDeployerFull{fakeDeployer: &fakeDeployer{provider: "aliyun",
			sites: []string{"live.example.com"}}}
		withRegistry(t, "aliyun", fake)
		tg := mustCreateTarget(t, client, "云实时", deploytarget.ProviderTypeAliyun,
			withCredSourceDeployCred(aliyunCred.ID))

		res, err := svc.GetCurrentCerts(ctx, tg.ID)
		if err != nil {
			t.Fatalf("查询失败: %v", err)
		}
		if r := res["live.example.com"]; r == nil || !r.Supported {
			t.Fatalf("实时域名资源未生成: %+v", res)
		}
	})

	t.Run("实时列表为空时返回空结果", func(t *testing.T) {
		fake := &fakeDeployerFull{fakeDeployer: &fakeDeployer{provider: "aliyun"}}
		withRegistry(t, "aliyun", fake)
		tg := mustCreateTarget(t, client, "空实时", deploytarget.ProviderTypeAliyun,
			withCredSourceDeployCred(aliyunCred.ID))

		res, err := svc.GetCurrentCerts(ctx, tg.ID)
		if err != nil {
			t.Fatalf("查询失败: %v", err)
		}
		if len(res) != 0 {
			t.Fatalf("无资源时应返回空结果，实际 %d", len(res))
		}
	})
}
