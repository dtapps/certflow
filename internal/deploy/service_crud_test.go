package deploy

import (
	"context"
	"strings"
	"testing"

	"cnb.cool/dtapp/certflow/internal/ent/deploytarget"
	"cnb.cool/dtapp/certflow/internal/ent/schema"
)

// TestService_CRUD 覆盖部署目标的增删改查。
func TestService_CRUD(t *testing.T) {
	svc, client := newDeployTestService(t)
	ctx := context.Background()

	dnsProv := mustCreateDNSProvider(t, client, "dnsp", "aliyun", schema.DNSProviderConfig{
		AccessKeyID: "ak", AccessKeySecret: "sk",
	})
	cred := mustCreateDeployCredential(t, client, "cred", "aliyun", aliyunCredConfig())

	created, err := svc.Create(ctx, CreateDeployTargetInput{
		Name:               "目标A",
		ProviderType:       "aliyun",
		DeployService:      "cdn",
		Config:             DeployTargetConfig{RegionID: "cn-hangzhou", Domains: []string{"a.example.com"}},
		CredentialSource:   "dns_provider",
		DNSProviderID:      &dnsProv.ID,
		DeployCredentialID: &cred.ID,
		IsActive:           true,
		Comment:            "备注",
	})
	if err != nil {
		t.Fatalf("Create 失败: %v", err)
	}
	if created.Name != "目标A" || created.Comment != "备注" || !created.IsActive {
		t.Fatalf("Create 字段不符: %+v", created)
	}
	// 外键以边形式存储，需带边查询校验
	reloaded, err := client.DeployTarget.Query().Where(deploytarget.ID(created.ID)).
		WithDNSProvider().WithDeployCredential().Only(ctx)
	if err != nil {
		t.Fatalf("回查目标失败: %v", err)
	}
	if reloaded.Edges.DNSProvider == nil || reloaded.Edges.DNSProvider.ID != dnsProv.ID {
		t.Fatal("DNSProvider 关联未落库")
	}
	if reloaded.Edges.DeployCredential == nil || reloaded.Edges.DeployCredential.ID != cred.ID {
		t.Fatal("DeployCredential 关联未落库")
	}
	if len(created.Config.Domains) != 1 || created.Config.Domains[0] != "a.example.com" {
		t.Fatalf("Config 未落库: %+v", created.Config)
	}

	// 非正数 ID 不应写入外键（视为未选择）
	zero := 0
	noFk, err := svc.Create(ctx, CreateDeployTargetInput{
		Name: "无凭证", ProviderType: "tencentcloud", DeployService: "cdn",
		CredentialSource: "deploy_credential", DNSProviderID: &zero, DeployCredentialID: &zero,
	})
	if err != nil {
		t.Fatalf("Create 无凭证失败: %v", err)
	}
	noFkReloaded, err := client.DeployTarget.Query().Where(deploytarget.ID(noFk.ID)).
		WithDNSProvider().WithDeployCredential().Only(ctx)
	if err != nil {
		t.Fatalf("回查目标失败: %v", err)
	}
	if noFkReloaded.Edges.DNSProvider != nil || noFkReloaded.Edges.DeployCredential != nil {
		t.Fatal("零值外键不应写入")
	}

	// Update：仅非空字段生效，Config 始终覆盖（用于清除历史域名/站点）
	inactive := false
	updated, err := svc.Update(ctx, created.ID, UpdateDeployTargetInput{
		Name:               "目标A-改名",
		ProviderType:       "tencentcloud",
		DeployService:      "edgeone",
		Config:             DeployTargetConfig{},
		CredentialSource:   "deploy_credential",
		DeployCredentialID: &cred.ID,
		IsActive:           &inactive,
		Comment:            "新备注",
	})
	if err != nil {
		t.Fatalf("Update 失败: %v", err)
	}
	if updated.Name != "目标A-改名" || updated.DeployService != "edgeone" || updated.IsActive {
		t.Fatalf("Update 字段不符: %+v", updated)
	}
	if updated.ProviderType != deploytarget.ProviderTypeTencentcloud {
		t.Fatalf("ProviderType 未更新: %v", updated.ProviderType)
	}
	if len(updated.Config.Domains) != 0 {
		t.Fatalf("Config 应被空值覆盖: %+v", updated.Config)
	}

	// Update 空输入：所有可选字段保持原值
	keep, err := svc.Update(ctx, created.ID, UpdateDeployTargetInput{Config: DeployTargetConfig{}})
	if err != nil {
		t.Fatalf("Update 空输入失败: %v", err)
	}
	if keep.Name != "目标A-改名" || keep.Comment != "新备注" || keep.IsActive {
		t.Fatalf("空输入不应修改字段: %+v", keep)
	}

	// Update 不存在的 ID
	if _, err := svc.Update(ctx, 99999, UpdateDeployTargetInput{}); err == nil {
		t.Fatal("更新不存在的目标应报错")
	}

	// Get 带边加载
	got, err := svc.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get 失败: %v", err)
	}
	if got.Edges.DeployCredential == nil {
		t.Fatal("Get 应预加载 DeployCredential")
	}
	if _, err := svc.Get(ctx, 99999); err == nil {
		t.Fatal("查询不存在的目标应报错")
	}

	// List 按创建时间倒序
	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("List 数量不符: %d", len(list))
	}

	// Delete
	if err := svc.Delete(ctx, noFk.ID); err != nil {
		t.Fatalf("Delete 失败: %v", err)
	}
	if _, err := svc.Get(ctx, noFk.ID); err == nil {
		t.Fatal("删除后不应再查到")
	}
}

// TestService_LinkCert 覆盖证书关联/解除关联与按证书反查目标。
func TestService_LinkCert(t *testing.T) {
	svc, client := newDeployTestService(t)
	ctx := context.Background()

	cert := mustCreateCert(t, client, "link.example.com")
	target := mustCreateTarget(t, client, "目标", deploytarget.ProviderTypeAliyun)

	if err := svc.LinkCert(ctx, target.ID, cert.ID); err != nil {
		t.Fatalf("LinkCert 失败: %v", err)
	}
	targets, err := svc.ListByCert(ctx, cert.ID)
	if err != nil {
		t.Fatalf("ListByCert 失败: %v", err)
	}
	if len(targets) != 1 || targets[0].ID != target.ID {
		t.Fatalf("关联后应反查到 1 个目标，实际 %d", len(targets))
	}

	if err := svc.UnlinkCert(ctx, target.ID, cert.ID); err != nil {
		t.Fatalf("UnlinkCert 失败: %v", err)
	}
	if targets, _ := svc.ListByCert(ctx, cert.ID); len(targets) != 0 {
		t.Fatalf("解除关联后应查不到目标，实际 %d", len(targets))
	}
}

// assertI18nRendered 断言错误文案已渲染（i18n 缺参会渲染出 <no value>）。
func assertI18nRendered(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("期望报错，实际为 nil")
	}
	if msg := err.Error(); msg == "" || strings.Contains(msg, "<no value>") {
		t.Fatalf("错误文案异常: %q", msg)
	}
}

// TestService_loadCredsAndConfig 覆盖三条凭证解析路径的成功与失败分支。
func TestService_loadCredsAndConfig(t *testing.T) {
	_, client := newDeployTestService(t)
	ctx := context.Background()

	aliyunCred := mustCreateDeployCredential(t, client, "阿里云", "aliyun", aliyunCredConfig())
	emptyCred := mustCreateDeployCredential(t, client, "空凭证", "aliyun", schema.DeployCredentialConfig{})
	halfCred := mustCreateDeployCredential(t, client, "半凭证", "aliyun", schema.DeployCredentialConfig{AccessKeyID: "ak"})
	btCred := mustCreateDeployCredential(t, client, "宝塔", "btpanel", btPanelCredConfig())
	emptyBtCred := mustCreateDeployCredential(t, client, "空宝塔", "btpanel", schema.DeployCredentialConfig{})
	dnsProv := mustCreateDNSProvider(t, client, "DNS", "aliyun", schema.DNSProviderConfig{
		AccessKeyID: "ak-dns", AccessKeySecret: "sk-dns",
	})
	halfDnsProv := mustCreateDNSProvider(t, client, "半DNS", "aliyun", schema.DNSProviderConfig{AccessKeyID: "ak"})

	load := func(id int) (Credentials, map[string]string, error) {
		t.Helper()
		tg, err := client.DeployTarget.Query().Where(deploytarget.ID(id)).
			WithDNSProvider().WithDeployCredential().Only(ctx)
		if err != nil {
			t.Fatalf("加载目标失败: %v", err)
		}
		return NewDeployService(client).loadCredsAndConfig(tg)
	}

	// 路径一：DNS 提供商 + 云厂商域名配置
	dnsTarget := mustCreateTarget(t, client, "DNS目标", deploytarget.ProviderTypeAliyun,
		withCredSourceDNSProvider(dnsProv.ID),
		withDomainConfig(DeployTargetConfig{RegionID: "cn-shanghai", Domains: []string{"d.example.com"}}))
	creds, svc, err := load(dnsTarget.ID)
	if err != nil {
		t.Fatalf("DNS 路径解析失败: %v", err)
	}
	if creds.AccessKeyID != "ak-dns" || creds.AccessKeySecret != "sk-dns" {
		t.Fatalf("DNS 路径凭证不符: %+v", creds)
	}
	if !strings.Contains(svc["domains"], "d.example.com") {
		t.Fatalf("DNS 路径配置未转 map: %+v", svc)
	}

	// DNS 路径：缺失 DNS 提供商边
	noDnsTarget := mustCreateTarget(t, client, "无DNS", deploytarget.ProviderTypeAliyun,
		withCredSourceDNSProvider(dnsProv.ID))
	// 直接清空边（模拟外键被置空）
	if err := client.DeployTarget.UpdateOneID(noDnsTarget.ID).ClearDNSProvider().Exec(ctx); err != nil {
		t.Fatalf("清空 DNS 外键失败: %v", err)
	}
	_, _, err = load(noDnsTarget.ID)
	assertI18nRendered(t, err)

	// DNS 路径：凭证不完整
	halfDnsTarget := mustCreateTarget(t, client, "半DNS目标", deploytarget.ProviderTypeAliyun,
		withCredSourceDNSProvider(halfDnsProv.ID))
	_, _, err = load(halfDnsTarget.ID)
	assertI18nRendered(t, err)

	// 路径二：面板部署凭证（site）
	btTarget := mustCreateTarget(t, client, "宝塔目标", deploytarget.ProviderTypeBtpanel,
		withCredSourceDeployCred(btCred.ID),
		withDomainConfig(DeployTargetConfig{SiteName: []string{"site-a"}, SiteID: []string{"1"}}))
	creds, svc, err = load(btTarget.ID)
	if err != nil {
		t.Fatalf("面板路径解析失败: %v", err)
	}
	if creds.AccessKeyID != "bt-key" {
		t.Fatalf("面板路径凭证不符: %+v", creds)
	}
	if svc["panel_url"] != "https://panel.example.com:8888" {
		t.Fatalf("panel_url 未注入 svc: %+v", svc)
	}

	// 面板路径：无部署凭证边
	noCredTarget := mustCreateTarget(t, client, "无凭证宝塔", deploytarget.ProviderTypeBtpanel,
		withCredSourceDeployCred(btCred.ID))
	if err := client.DeployTarget.UpdateOneID(noCredTarget.ID).ClearDeployCredential().Exec(ctx); err != nil {
		t.Fatalf("清空部署凭证外键失败: %v", err)
	}
	_, _, err = load(noCredTarget.ID)
	assertI18nRendered(t, err)

	// 面板路径：凭证为空（API Key 与 JWT 均为空）
	emptyBtTarget := mustCreateTarget(t, client, "空宝塔目标", deploytarget.ProviderTypeBtpanel,
		withCredSourceDeployCred(emptyBtCred.ID))
	_, _, err = load(emptyBtTarget.ID)
	assertI18nRendered(t, err)

	// 路径三：云厂商部署凭证（domain）
	domainTarget := mustCreateTarget(t, client, "云目标", deploytarget.ProviderTypeAliyun,
		withCredSourceDeployCred(aliyunCred.ID),
		withDomainConfig(DeployTargetConfig{Domains: []string{"x.example.com"}}))
	creds, svc, err = load(domainTarget.ID)
	if err != nil {
		t.Fatalf("云厂商路径解析失败: %v", err)
	}
	if creds.Region != "cn-hangzhou" {
		t.Fatalf("region 未从凭证解析: %+v", creds)
	}
	if !strings.Contains(svc["domains"], "x.example.com") {
		t.Fatalf("云厂商路径配置未转 map: %+v", svc)
	}

	// 云厂商路径：AK 齐全但缺 SK
	halfTarget := mustCreateTarget(t, client, "半云目标", deploytarget.ProviderTypeAliyun,
		withCredSourceDeployCred(halfCred.ID))
	_, _, err = load(halfTarget.ID)
	assertI18nRendered(t, err)

	// 云厂商路径：凭证为空
	emptyTarget := mustCreateTarget(t, client, "空凭证目标", deploytarget.ProviderTypeAliyun,
		withCredSourceDeployCred(emptyCred.ID))
	_, _, err = load(emptyTarget.ID)
	assertI18nRendered(t, err)

	// DNS 凭证来源但边被清空：走 loadDNSProviderPath 的空边分支
	unknownSrc := mustCreateTarget(t, client, "未知来源", deploytarget.ProviderTypeAliyun)
	if err := client.DeployTarget.UpdateOneID(unknownSrc.ID).
		SetCredentialSource(deploytarget.CredentialSourceDNSProvider).
		ClearDNSProvider().Exec(ctx); err != nil {
		t.Fatalf("设置凭证来源失败: %v", err)
	}
	_, _, err = load(unknownSrc.ID)
	assertI18nRendered(t, err)
}

// TestService_FetchCDNDomains 覆盖内联凭证拉取域名列表的各分支。
func TestService_FetchCDNDomains(t *testing.T) {
	svc, client := newDeployTestService(t)
	ctx := context.Background()

	cred := mustCreateDeployCredential(t, client, "阿里云", "aliyun", aliyunCredConfig())
	emptyCred := mustCreateDeployCredential(t, client, "空", "aliyun", schema.DeployCredentialConfig{})
	dnsProv := mustCreateDNSProvider(t, client, "DNS", "aliyun", schema.DNSProviderConfig{
		AccessKeyID: "ak", AccessKeySecret: "sk",
	})
	btCred := mustCreateDeployCredential(t, client, "宝塔", "btpanel", btPanelCredConfig())

	t.Run("未知凭证来源", func(t *testing.T) {
		_, err := svc.FetchCDNDomains(ctx, FetchDomainsInput{ProviderType: "aliyun"})
		assertI18nRendered(t, err)
	})

	t.Run("部署凭证 ID 缺失", func(t *testing.T) {
		_, err := svc.FetchCDNDomains(ctx, FetchDomainsInput{
			ProviderType: "aliyun", CredentialSource: "deploy_credential",
		})
		assertI18nRendered(t, err)
	})

	t.Run("部署凭证不存在", func(t *testing.T) {
		id := 9999
		_, err := svc.FetchCDNDomains(ctx, FetchDomainsInput{
			ProviderType: "aliyun", CredentialSource: "deploy_credential", DeployCredentialID: &id,
		})
		assertI18nRendered(t, err)
	})

	t.Run("DNS 提供商 ID 缺失", func(t *testing.T) {
		_, err := svc.FetchCDNDomains(ctx, FetchDomainsInput{
			ProviderType: "aliyun", CredentialSource: "dns_provider",
		})
		assertI18nRendered(t, err)
	})

	t.Run("DNS 提供商不存在", func(t *testing.T) {
		id := 9999
		_, err := svc.FetchCDNDomains(ctx, FetchDomainsInput{
			ProviderType: "aliyun", CredentialSource: "dns_provider", DNSProviderID: &id,
		})
		assertI18nRendered(t, err)
	})

	t.Run("凭证字段为空", func(t *testing.T) {
		_, err := svc.FetchCDNDomains(ctx, FetchDomainsInput{
			ProviderType: "aliyun", CredentialSource: "deploy_credential", DeployCredentialID: &emptyCred.ID,
		})
		assertI18nRendered(t, err)
	})

	t.Run("厂商未注册部署器", func(t *testing.T) {
		withRegistry(t, "aliyun", nil)
		_, err := svc.FetchCDNDomains(ctx, FetchDomainsInput{
			ProviderType: "aliyun", CredentialSource: "deploy_credential", DeployCredentialID: &cred.ID,
		})
		assertI18nRendered(t, err)
	})

	t.Run("部署凭证拉取成功", func(t *testing.T) {
		fake := &fakeDeployer{provider: "aliyun", sites: []string{"a.example.com", "b.example.com"}}
		withRegistry(t, "aliyun", fake)

		items, err := svc.FetchCDNDomains(ctx, FetchDomainsInput{
			ProviderType: "aliyun", DeployService: "cdn", CredentialSource: "deploy_credential",
			DeployCredentialID: &cred.ID, Region: "cn-beijing",
			Config: DeployTargetConfig{SiteID: []string{"site-1"}},
		})
		if err != nil {
			t.Fatalf("拉取域名失败: %v", err)
		}
		if len(items) != 2 {
			t.Fatalf("域名数量不符: %v", items)
		}
		if fake.siteCalls != 1 {
			t.Fatalf("应调用 ListSites 1 次，实际 %d", fake.siteCalls)
		}
	})

	t.Run("DNS 凭证拉取成功", func(t *testing.T) {
		fake := &fakeDeployer{provider: "aliyun", sites: []string{"c.example.com"}}
		withRegistry(t, "aliyun", fake)

		items, err := svc.FetchCDNDomains(ctx, FetchDomainsInput{
			ProviderType: "aliyun", DeployService: "cdn", CredentialSource: "dns_provider",
			DNSProviderID: &dnsProv.ID,
		})
		if err != nil {
			t.Fatalf("拉取域名失败: %v", err)
		}
		if len(items) != 1 || items[0] != "c.example.com" {
			t.Fatalf("域名不符: %v", items)
		}
	})

	t.Run("面板凭证拉取站点", func(t *testing.T) {
		fake := &fakeDeployer{provider: "btpanel", sites: []string{"站点A||1"}}
		withRegistry(t, "btpanel", fake)

		items, err := svc.FetchCDNDomains(ctx, FetchDomainsInput{
			ProviderType: "btpanel", DeployService: "btpanel", CredentialSource: "deploy_credential",
			DeployCredentialID: &btCred.ID,
		})
		if err != nil {
			t.Fatalf("拉取站点失败: %v", err)
		}
		if len(items) != 1 {
			t.Fatalf("站点数量不符: %v", items)
		}
	})
}

// TestService_ListCDNDomains 覆盖按已存目标拉取域名列表。
func TestService_ListCDNDomains(t *testing.T) {
	svc, client := newDeployTestService(t)
	ctx := context.Background()

	cred := mustCreateDeployCredential(t, client, "阿里云", "aliyun", aliyunCredConfig())

	t.Run("目标不存在", func(t *testing.T) {
		_, err := svc.ListCDNDomains(ctx, 12345)
		assertI18nRendered(t, err)
	})

	t.Run("凭证不可用", func(t *testing.T) {
		tg := mustCreateTarget(t, client, "无凭证目标", deploytarget.ProviderTypeAliyun,
			withCredSourceDeployCred(cred.ID))
		if err := client.DeployTarget.UpdateOneID(tg.ID).ClearDeployCredential().Exec(ctx); err != nil {
			t.Fatalf("清空外键失败: %v", err)
		}
		_, err := svc.ListCDNDomains(ctx, tg.ID)
		assertI18nRendered(t, err)
	})

	t.Run("厂商未注册部署器", func(t *testing.T) {
		withRegistry(t, "aliyun", nil)
		tg := mustCreateTarget(t, client, "目标", deploytarget.ProviderTypeAliyun,
			withCredSourceDeployCred(cred.ID))
		_, err := svc.ListCDNDomains(ctx, tg.ID)
		assertI18nRendered(t, err)
	})

	t.Run("拉取成功", func(t *testing.T) {
		fake := &fakeDeployer{provider: "aliyun", sites: []string{"d.example.com"}}
		withRegistry(t, "aliyun", fake)
		tg := mustCreateTarget(t, client, "目标", deploytarget.ProviderTypeAliyun,
			withCredSourceDeployCred(cred.ID),
			withDomainConfig(DeployTargetConfig{SiteID: []string{"site-9"}}))

		items, err := svc.ListCDNDomains(ctx, tg.ID)
		if err != nil {
			t.Fatalf("拉取失败: %v", err)
		}
		if len(items) != 1 || items[0] != "d.example.com" {
			t.Fatalf("域名不符: %v", items)
		}
	})
}
