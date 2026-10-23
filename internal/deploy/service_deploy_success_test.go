package deploy

import (
	"context"
	"testing"

	"cnb.cool/dtapp/certflow/internal/ent/deploytarget"
	"cnb.cool/dtapp/certflow/internal/ent/schema"
)

// TestService_DeployCertificate_Success 覆盖云厂商/面板类部署成功路径及 svc 注入。
func TestService_DeployCertificate_Success(t *testing.T) {
	svc, client := newDeployTestService(t)
	ctx := context.Background()

	aliyunCred := mustCreateDeployCredential(t, client, "阿里云", "aliyun", aliyunCredConfig())
	btCred := mustCreateDeployCredential(t, client, "宝塔", "btpanel", btPanelCredConfig())
	cert := mustCreateCert(t, client, "deploy.example.com")

	t.Run("云厂商部署成功", func(t *testing.T) {
		fake := &fakeDeployer{provider: "aliyun", uploadID: "cloud-123"}
		withRegistry(t, "aliyun", fake)
		tg := mustCreateTarget(t, client, "云目标", deploytarget.ProviderTypeAliyun,
			withCredSourceDeployCred(aliyunCred.ID),
			withDomainConfig(DeployTargetConfig{Domains: []string{"a.example.com"}}))

		outcome, err := svc.DeployCertificate(ctx, tg.ID, cert.ID, "a.example.com", "")
		if err != nil {
			t.Fatalf("部署失败: %v", err)
		}
		if !outcome.Success || outcome.CloudCertID != "cloud-123" || outcome.TargetName != "云目标" {
			t.Fatalf("部署结果不符: %+v", outcome)
		}
		// 部署器收到的 svc 配置应包含域名、证书域名与部署服务
		if fake.lastSvc["domain"] != "a.example.com" || fake.lastSvc["cert_domain"] != "deploy.example.com" {
			t.Fatalf("svc 注入不符: %+v", fake.lastSvc)
		}
		if fake.lastSvc["cert_pem"] != "" {
			t.Fatal("云厂商不应注入证书内容")
		}
		// 目标状态与历史记录
		reloaded, _ := client.DeployTarget.Get(ctx, tg.ID)
		if reloaded.LastStatus != "success" || reloaded.LastError != "" || reloaded.LastDeployedAt.IsZero() {
			t.Fatalf("成功状态未落库: %+v", reloaded)
		}
		logs, _ := svc.ListDeployLogs(ctx, tg.ID)
		if len(logs) != 1 || !logs[0].Success || logs[0].CloudCertID != "cloud-123" {
			t.Fatalf("成功日志不符: %+v", logs)
		}
		if logs[0].DeployDomain != "a.example.com" || logs[0].CertDomain != "deploy.example.com" {
			t.Fatalf("日志域名不符: %+v", logs[0])
		}
	})

	t.Run("面板部署成功并注入证书内容", func(t *testing.T) {
		fake := &fakeDeployer{provider: "btpanel"}
		withRegistry(t, "btpanel", fake)
		tg := mustCreateTarget(t, client, "宝塔目标", deploytarget.ProviderTypeBtpanel,
			withCredSourceDeployCred(btCred.ID),
			withDomainConfig(DeployTargetConfig{SiteName: []string{"站点A", "站点B"}, SiteID: []string{"1", "2"}}))

		if _, err := svc.DeployCertificate(ctx, tg.ID, cert.ID, "站点A", "1"); err != nil {
			t.Fatalf("面板部署失败: %v", err)
		}
		if fake.lastSvc["site_name"] != "站点A" || fake.lastSvc["site_id"] != "1" {
			t.Fatalf("站点信息未注入: %+v", fake.lastSvc)
		}
		if fake.lastSvc["cert_pem"] == "" || fake.lastSvc["key_pem"] == "" {
			t.Fatal("面板类应注入证书内容")
		}
	})

	t.Run("面板未传站点ID时按名称配对", func(t *testing.T) {
		fake := &fakeDeployer{provider: "btpanel"}
		withRegistry(t, "btpanel", fake)
		tg := mustCreateTarget(t, client, "宝塔目标2", deploytarget.ProviderTypeBtpanel,
			withCredSourceDeployCred(btCred.ID),
			withDomainConfig(DeployTargetConfig{SiteName: []string{"站点A", "站点B"}, SiteID: []string{"1", "2"}}))

		if _, err := svc.DeployCertificate(ctx, tg.ID, cert.ID, "站点B", ""); err != nil {
			t.Fatalf("面板部署失败: %v", err)
		}
		if fake.lastSvc["site_id"] != "2" {
			t.Fatalf("站点 ID 未按名称配对: %+v", fake.lastSvc)
		}
	})

	t.Run("ESA 站点ID透传到 site_id", func(t *testing.T) {
		fake := &fakeDeployer{provider: "aliyun"}
		withRegistry(t, "aliyun", fake)
		tg := mustCreateTarget(t, client, "ESA目标", deploytarget.ProviderTypeAliyun,
			withCredSourceDeployCred(aliyunCred.ID),
			withDeployServiceParam("esa"))

		if _, err := svc.DeployCertificate(ctx, tg.ID, cert.ID, "a.example.com", "site-esa"); err != nil {
			t.Fatalf("ESA 部署失败: %v", err)
		}
		if fake.lastSvc["site_id"] != "site-esa" {
			t.Fatalf("site_id 未透传: %+v", fake.lastSvc)
		}
	})

	t.Run("EdgeOne 站点ID透传到 zone_id", func(t *testing.T) {
		fake := &fakeDeployer{provider: "tencentcloud"}
		withRegistry(t, "tencentcloud", fake)
		tcCred := mustCreateDeployCredential(t, client, "腾讯云", "tencentcloud", schema.DeployCredentialConfig{
			SecretID: "sid", SecretKey: "skey",
		})
		tg := mustCreateTarget(t, client, "EdgeOne目标", deploytarget.ProviderTypeTencentcloud,
			withCredSourceDeployCred(tcCred.ID),
			withDeployServiceParam("edgeone"))

		if _, err := svc.DeployCertificate(ctx, tg.ID, cert.ID, "a.example.com", "zone-1"); err != nil {
			t.Fatalf("EdgeOne 部署失败: %v", err)
		}
		if fake.lastSvc["zone_id"] != "zone-1" {
			t.Fatalf("zone_id 未透传: %+v", fake.lastSvc)
		}
	})
}
