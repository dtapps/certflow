package deploy

import (
	"context"
	"testing"

	"cnb.cool/dtapp/certflow/internal/ent/deploytarget"
)

// TestService_EnsureUploaded_Dedup 覆盖上传去重的三层逻辑：内存缓存 / 数据库映射 / 云端校验后重传。
func TestService_EnsureUploaded_Dedup(t *testing.T) {
	svc, client := newDeployTestService(t)
	ctx := context.Background()

	cred := mustCreateDeployCredential(t, client, "阿里云", "aliyun", aliyunCredConfig())
	cert := mustCreateCert(t, client, "dedup.example.com")
	fake := &fakeDeployer{provider: "aliyun", uploadID: "cloud-dedup"}
	withRegistry(t, "aliyun", fake)

	tg := mustCreateTarget(t, client, "去重目标", deploytarget.ProviderTypeAliyun,
		withCredSourceDeployCred(cred.ID),
		withDomainConfig(DeployTargetConfig{Domains: []string{"d.example.com"}}))

	// 第一次部署：真正上传
	if _, err := svc.DeployCertificate(ctx, tg.ID, cert.ID, "d.example.com", ""); err != nil {
		t.Fatalf("首次部署失败: %v", err)
	}
	// 第二次部署：命中内存缓存
	if _, err := svc.DeployCertificate(ctx, tg.ID, cert.ID, "d.example.com", ""); err != nil {
		t.Fatalf("二次部署失败: %v", err)
	}
	if upload, deploy := fake.counters(); upload != 1 || deploy != 2 {
		t.Fatalf("应只上传 1 次、部署 2 次，实际上传 %d 部署 %d", upload, deploy)
	}

	// 清空内存缓存（模拟进程重启）：命中数据库映射，仍不重复上传
	svc.uploadCache = nil
	if _, err := svc.DeployCertificate(ctx, tg.ID, cert.ID, "d.example.com", ""); err != nil {
		t.Fatalf("重启后部署失败: %v", err)
	}
	if upload, _ := fake.counters(); upload != 1 {
		t.Fatalf("数据库复用时不应重复上传，实际上传 %d 次", upload)
	}

	// 云端校验失败（证书已被删除）：应重新上传
	full := &fakeDeployerFull{fakeDeployer: fake, validateOK: false}
	withRegistry(t, "aliyun", full)
	svc.uploadCache = nil
	if _, err := svc.DeployCertificate(ctx, tg.ID, cert.ID, "d.example.com", ""); err != nil {
		t.Fatalf("校验重传部署失败: %v", err)
	}
	if upload, _ := fake.counters(); upload != 2 {
		t.Fatalf("云端证书不存在时应重新上传，实际上传 %d 次", upload)
	}
	if full.validateCalls == 0 {
		t.Fatal("应调用 ValidateCert 校验云端证书")
	}

	// 校验接口报错（无法确认）：沿用数据库记录，不重复上传
	full.validateErr = errBoom
	svc.uploadCache = nil
	if _, err := svc.DeployCertificate(ctx, tg.ID, cert.ID, "d.example.com", ""); err != nil {
		t.Fatalf("校验异常部署失败: %v", err)
	}
	if upload, _ := fake.counters(); upload != 2 {
		t.Fatalf("校验异常时应沿用记录，实际上传 %d 次", upload)
	}
}

// TestService_DeployAllForCert 覆盖按证书批量部署到所有启用目标。
func TestService_DeployAllForCert(t *testing.T) {
	svc, client := newDeployTestService(t)
	ctx := context.Background()

	aliyunCred := mustCreateDeployCredential(t, client, "阿里云", "aliyun", aliyunCredConfig())
	btCred := mustCreateDeployCredential(t, client, "宝塔", "btpanel", btPanelCredConfig())
	cert := mustCreateCert(t, client, "all.example.com")

	withRegistry(t, "aliyun", &fakeDeployer{provider: "aliyun"})
	withRegistry(t, "btpanel", &fakeDeployer{provider: "btpanel"})

	t.Run("未关联任何目标", func(t *testing.T) {
		outcomes, err := svc.DeployAllForCert(ctx, cert.ID)
		if err != nil {
			t.Fatalf("批量部署失败: %v", err)
		}
		if len(outcomes) != 0 {
			t.Fatalf("未关联目标时应无结果，实际 %d", len(outcomes))
		}
	})

	t.Run("跳过停用目标", func(t *testing.T) {
		disabledCert := mustCreateCert(t, client, "disabled.example.com")
		mustCreateTarget(t, client, "停用目标", deploytarget.ProviderTypeAliyun,
			withCredSourceDeployCred(aliyunCred.ID), withActive(false), withLinkedCerts(disabledCert.ID))

		outcomes, err := svc.DeployAllForCert(ctx, disabledCert.ID)
		if err != nil {
			t.Fatalf("批量部署失败: %v", err)
		}
		if len(outcomes) != 0 {
			t.Fatalf("停用目标应被跳过，实际 %d 个结果", len(outcomes))
		}
	})

	t.Run("云目标单次部署", func(t *testing.T) {
		cloudCert := mustCreateCert(t, client, "cloud.example.com")
		tg := mustCreateTarget(t, client, "云目标", deploytarget.ProviderTypeAliyun,
			withCredSourceDeployCred(aliyunCred.ID),
			withDomainConfig(DeployTargetConfig{Domains: []string{"cloud.example.com"}}),
			withLinkedCerts(cloudCert.ID))

		outcomes, err := svc.DeployAllForCert(ctx, cloudCert.ID)
		if err != nil {
			t.Fatalf("批量部署失败: %v", err)
		}
		if len(outcomes) != 1 || !outcomes[0].Success || outcomes[0].TargetID != tg.ID {
			t.Fatalf("云目标应部署 1 次且成功: %+v", outcomes)
		}
	})

	t.Run("面板目标按站点循环", func(t *testing.T) {
		panelCert := mustCreateCert(t, client, "panel.example.com")
		tg := mustCreateTarget(t, client, "面板目标", deploytarget.ProviderTypeBtpanel,
			withCredSourceDeployCred(btCred.ID),
			withDomainConfig(DeployTargetConfig{SiteName: []string{"站点A", "站点B"}, SiteID: []string{"1", "2"}}),
			withLinkedCerts(panelCert.ID))

		outcomes, err := svc.DeployAllForCert(ctx, panelCert.ID)
		if err != nil {
			t.Fatalf("批量部署失败: %v", err)
		}
		if len(outcomes) != 2 {
			t.Fatalf("面板目标应按 2 个站点部署，实际 %d", len(outcomes))
		}
		for _, o := range outcomes {
			if !o.Success || o.TargetID != tg.ID {
				t.Fatalf("面板站点部署结果不符: %+v", o)
			}
		}
	})

	t.Run("面板目标未配置站点", func(t *testing.T) {
		emptyCert := mustCreateCert(t, client, "empty.example.com")
		mustCreateTarget(t, client, "面板目标2", deploytarget.ProviderTypeBtpanel,
			withCredSourceDeployCred(btCred.ID), withLinkedCerts(emptyCert.ID))

		outcomes, err := svc.DeployAllForCert(ctx, emptyCert.ID)
		if err != nil {
			t.Fatalf("批量部署失败: %v", err)
		}
		if len(outcomes) != 1 || !outcomes[0].Success {
			t.Fatalf("未配置站点时应部署 1 次: %+v", outcomes)
		}
	})
}

// TestService_LinkCert_SingleTargetLimit 记录当前「一个证书只能关联一个部署目标」的缺陷。
//
// TODO(schema): DeployTarget.certificates 的注释声明为多对多，但 Certificate 侧缺少
// 反向边（edge.From("deploy_targets", DeployTarget.Type).Ref("certificates")），
// ent 因此生成了一对多（外键列 certificates.deploy_target_certificates），
// 导致同一证书关联第二个目标时报 constraint failed，DeployAllForCert 也永远只能
// 找到 1 个目标。补反向边并重新生成 ent 代码后，本用例应改为断言「两个目标都能关联」。
func TestService_LinkCert_SingleTargetLimit(t *testing.T) {
	svc, client := newDeployTestService(t)
	ctx := context.Background()

	cert := mustCreateCert(t, client, "multi.example.com")
	first := mustCreateTarget(t, client, "目标1", deploytarget.ProviderTypeAliyun)
	second := mustCreateTarget(t, client, "目标2", deploytarget.ProviderTypeAliyun)

	if err := svc.LinkCert(ctx, first.ID, cert.ID); err != nil {
		t.Fatalf("首次关联失败: %v", err)
	}
	if err := svc.LinkCert(ctx, second.ID, cert.ID); err == nil {
		t.Fatal("当前实现下二次关联应失败（说明已修复为多对多，请更新本用例）")
	}
	targets, err := svc.ListByCert(ctx, cert.ID)
	if err != nil {
		t.Fatalf("ListByCert 失败: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("当前实现下一个证书只关联 1 个目标，实际 %d", len(targets))
	}
}
