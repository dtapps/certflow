package deploy

import (
	"context"
	"strings"
	"testing"

	"cnb.cool/dtapp/certflow/internal/ent/deploytarget"
)

// TestService_DeployCertificate_Failures 覆盖部署前的各类失败分支及其落库副作用。
func TestService_DeployCertificate_Failures(t *testing.T) {
	svc, client := newDeployTestService(t)
	ctx := context.Background()

	cred := mustCreateDeployCredential(t, client, "阿里云", "aliyun", aliyunCredConfig())
	cert := mustCreateCert(t, client, "deploy.example.com")

	t.Run("目标不存在", func(t *testing.T) {
		_, err := svc.DeployCertificate(ctx, 9999, cert.ID, "a.example.com", "")
		assertI18nRendered(t, err)
	})

	t.Run("证书不存在", func(t *testing.T) {
		tg := mustCreateTarget(t, client, "目标", deploytarget.ProviderTypeAliyun, withCredSourceDeployCred(cred.ID))
		_, err := svc.DeployCertificate(ctx, tg.ID, 9999, "a.example.com", "")
		assertI18nRendered(t, err)
	})

	t.Run("凭证不可用", func(t *testing.T) {
		fake := &fakeDeployer{provider: "aliyun"}
		withRegistry(t, "aliyun", fake)
		tg := mustCreateTarget(t, client, "凭证缺失目标", deploytarget.ProviderTypeAliyun,
			withCredSourceDeployCred(cred.ID))
		if err := client.DeployTarget.UpdateOneID(tg.ID).ClearDeployCredential().Exec(ctx); err != nil {
			t.Fatalf("清空外键失败: %v", err)
		}

		outcome, err := svc.DeployCertificate(ctx, tg.ID, cert.ID, "a.example.com", "")
		if err == nil || outcome == nil || outcome.Success {
			t.Fatalf("凭证不可用应返回失败结果，实际 outcome=%+v err=%v", outcome, err)
		}
		// 失败信息需落库到目标与历史记录
		reloaded, qerr := client.DeployTarget.Get(ctx, tg.ID)
		if qerr != nil {
			t.Fatalf("回查目标失败: %v", qerr)
		}
		if reloaded.LastStatus != "failed" || reloaded.LastError == "" || reloaded.LastDeployedAt.IsZero() {
			t.Fatalf("失败状态未落库: %+v", reloaded)
		}
		logs, _ := svc.ListDeployLogs(ctx, tg.ID)
		if len(logs) != 1 || logs[0].Success || logs[0].Message == "" {
			t.Fatalf("失败日志未落库: %+v", logs)
		}
	})

	t.Run("厂商未注册部署器", func(t *testing.T) {
		withRegistry(t, "aliyun", nil)
		tg := mustCreateTarget(t, client, "无部署器目标", deploytarget.ProviderTypeAliyun,
			withCredSourceDeployCred(cred.ID))

		outcome, err := svc.DeployCertificate(ctx, tg.ID, cert.ID, "a.example.com", "")
		if err == nil || outcome == nil || outcome.Success {
			t.Fatalf("未注册部署器应失败: %+v %v", outcome, err)
		}
		if !strings.Contains(outcome.Message, "aliyun") {
			t.Fatalf("错误信息应包含厂商名: %q", outcome.Message)
		}
	})

	t.Run("上传失败", func(t *testing.T) {
		fake := &fakeDeployer{provider: "aliyun", uploadErr: errBoom, uploadRaw: `{"err":"upload"}`}
		withRegistry(t, "aliyun", fake)
		tg := mustCreateTarget(t, client, "上传失败目标", deploytarget.ProviderTypeAliyun,
			withCredSourceDeployCred(cred.ID))

		outcome, err := svc.DeployCertificate(ctx, tg.ID, cert.ID, "a.example.com", "")
		if err == nil || outcome.Success {
			t.Fatalf("上传失败应返回失败: %+v %v", outcome, err)
		}
		logs, _ := svc.ListDeployLogs(ctx, tg.ID)
		if len(logs) != 1 || logs[0].Response != `{"err":"upload"}` {
			t.Fatalf("上传失败的原始响应未落库: %+v", logs)
		}
	})

	t.Run("部署失败（带原始响应）", func(t *testing.T) {
		fake := &fakeDeployer{
			provider:  "aliyun",
			deployErr: errBoom,
			deployRes: &DeployResult{RawResponse: `{"err":"deploy"}`},
		}
		withRegistry(t, "aliyun", fake)
		tg := mustCreateTarget(t, client, "部署失败目标", deploytarget.ProviderTypeAliyun,
			withCredSourceDeployCred(cred.ID))

		_, err := svc.DeployCertificate(ctx, tg.ID, cert.ID, "a.example.com", "")
		if err == nil {
			t.Fatal("部署失败应返回错误")
		}
		logs, _ := svc.ListDeployLogs(ctx, tg.ID)
		if len(logs) != 1 || logs[0].Response != `{"err":"deploy"}` {
			t.Fatalf("部署失败的原始响应未落库: %+v", logs)
		}
	})

	t.Run("部署失败（无响应体）", func(t *testing.T) {
		fake := &fakeDeployer{provider: "aliyun", deployErr: errBoom}
		withRegistry(t, "aliyun", fake)
		tg := mustCreateTarget(t, client, "无响应失败目标", deploytarget.ProviderTypeAliyun,
			withCredSourceDeployCred(cred.ID))

		_, err := svc.DeployCertificate(ctx, tg.ID, cert.ID, "", "")
		if err == nil {
			t.Fatal("部署失败应返回错误")
		}
		logs, _ := svc.ListDeployLogs(ctx, tg.ID)
		if len(logs) != 1 || logs[0].DeployDomain != "deploy.example.com" {
			t.Fatalf("未传域名时应回退证书域名: %+v", logs)
		}
	})
}
