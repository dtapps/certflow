package deploy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/pem"
	"net/http"
	"strings"

	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/basic"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/global"
	config "github.com/huaweicloud/huaweicloud-sdk-go-v3/core/config"
	cdn "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/cdn/v2"
	cdnmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/cdn/v2/model"
	cdnregion "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/cdn/v2/region"
	scm "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/scm/v3"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/services/scm/v3/model"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/services/scm/v3/region"

	"cnb.cool/dtapp/certflow/internal/httplog"
	"cnb.cool/dtapp/certflow/internal/i18n"
	"cnb.cool/dtapp/certflow/internal/logging"
)

// leafCertDER 从 PEM 文本中提取第一张证书（叶子证书）的 DER 字节。
// 输入可能是单张证书或「证书+链」，我们只取第一块用于指纹比对，规避链/空白差异。
func leafCertDER(pemText string) ([]byte, error) {
	rest := []byte(pemText)
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			return nil, i18n.NewError("deploy.error.huawei_no_pem_block")
		}
		if block.Type == "CERTIFICATE" {
			return block.Bytes, nil
		}
		if len(rest) == 0 {
			return nil, i18n.NewError("deploy.error.huawei_no_certificate_block")
		}
	}
}

// certFingerprint 计算证书 DER 的 SHA256，用于内容比对（忽略格式/空白/链差异）
func certFingerprint(pemText string) ([]byte, error) {
	der, err := leafCertDER(pemText)
	if err != nil {
		return nil, err
	}
	fp := sha256.Sum256(der)
	return fp[:], nil
}

// sameCertPEM 判断两份 PEM 是否为同一张证书（按叶子证书 DER 指纹比对）
func sameCertPEM(a, b string) bool {
	fa, errA := certFingerprint(a)
	fb, errB := certFingerprint(b)
	if errA != nil || errB != nil {
		return false
	}
	return bytes.Equal(fa, fb)
}

// HuaweiDeployer 华为云部署器：导入证书到 SCM，再推送到 CDN / WAF / ELB
type HuaweiDeployer struct{}

func init() { RegisterDeployer(&HuaweiDeployer{}) }

func (d *HuaweiDeployer) Provider() string { return "huawei" }

// newScmClient 构造华为云 SCM 客户端
// 注意：SCM（证书管理服务）与 CDN 等目标服务开放的区域不同（SCM 主区域为 cn-north-4）。
// 证书是账号级资源，导入到 SCM 后可通过 PushCertificate 推送到任意目标区域，
// 因此 SCM 客户端区域独立于「部署目标区域」：优先用用户填写的区域，若不在 SCM 支持列表则回退到主区域 cn-north-4。
func (d *HuaweiDeployer) newScmClient(creds Credentials) (*scm.ScmClient, error) {
	scmRegion := creds.Region
	if scmRegion == "" {
		scmRegion = "cn-north-4"
	}
	reg, err := region.SafeValueOf(scmRegion)
	if err != nil {
		// 用户填写的区域 SCM 不支持时，回退到 SCM 主区域 cn-north-4
		scmRegion = "cn-north-4"
		reg, err = region.SafeValueOf(scmRegion)
		if err != nil {
			return nil, i18n.Wrap(err, "deploy.error.huawei_scm_client_create")
		}
		logging.Debug(i18n.T("log.deploy.huawei_scm_region_fallback", "Region", creds.Region, "ScmRegion", scmRegion))
	}
	credential, err := basic.NewCredentialsBuilder().WithAk(creds.AccessKeyID).WithSk(creds.AccessKeySecret).SafeBuild()
	if err != nil {
		return nil, i18n.Wrap(err, "deploy.error.huawei_credential_create")
	}
	// 仅用 WithRegion：endpoint 由 region 元数据自动推导（如 cn-north-4 → https://scm.cn-north-4.myhuaweicloud.com），
	// 不再手写 WithEndpoint，避免老式写法、自动适配任意区域。
	hc, err := scm.ScmClientBuilder().
		WithCredential(credential).
		WithRegion(reg).
		WithHttpConfig(config.DefaultHttpConfig().WithHttpRoundTripper(httplog.WrapTransport(&http.Transport{}))).
		SafeBuild()
	if err != nil {
		return nil, err
	}
	return scm.NewScmClient(hc), nil
}

// UploadCert 导入证书到华为云 SCM，返回证书 ID。
// 华为云 ImportCertificate 不保证按内容去重，跨进程/跨重启可能重复建证书。
// 因此上传前先按证书名关键字列出候选，再逐个导出比对 DER 指纹，命中则直接复用已有证书 ID，
// 保证「同一张证书在华为云始终唯一」。
func (d *HuaweiDeployer) UploadCert(ctx context.Context, creds Credentials, cert CertContent, svcConfig map[string]string) (string, string, error) {
	client, err := d.newScmClient(creds)
	if err != nil {
		return "", "", i18n.Wrap(err, "deploy.error.huawei_scm_client_create")
	}
	name := certName(cert.Domain, svcConfig)
	logging.Debug(i18n.T("log.deploy.huawei_upload_start", "Domain", cert.Domain, "Name", name))

	// 1) 先查云端是否已有相同证书（按名关键字过滤 + DER 指纹精确比对）
	if existingID, ok := d.findExistingCert(client, name, cert.CertPEM); ok {
		logging.Debug(i18n.T("log.deploy.huawei_upload_reuse", "CertID", existingID))
		return existingID, "", nil
	}

	// 2) 未命中再导入
	importReq := &model.ImportCertificateRequest{
		Body: &model.ImportCertificateRequestBody{
			Name:        name,
			Certificate: cert.CertPEM,
			PrivateKey:  cert.KeyPEM,
		},
	}
	importResp, err := client.ImportCertificate(importReq)
	if err != nil {
		logging.Debug(i18n.T("log.deploy.huawei_upload_failed", "Name", name, "Err", err, "Resp", respDump(importResp)))
		return "", "", i18n.Wrap(err, "deploy.error.huawei_scm_import")
	}
	if importResp.CertificateId == nil {
		return "", "", i18n.NewError("deploy.error.huawei_scm_no_cert_id")
	}
	logging.Debug(i18n.T("log.deploy.huawei_import_success", "CertID", *importResp.CertificateId))
	return *importResp.CertificateId, "", nil
}

// findExistingCert 在华为云 SCM 中查找与给定名称和证书内容一致的已存在证书，返回其 ID。
// 通过 ListCertificates（按名关键字过滤）分页遍历，对每个候选 ExportCertificate 比对 DER 指纹。
func (d *HuaweiDeployer) findExistingCert(client *scm.ScmClient, name, certPEM string) (string, bool) {
	limit := int32(50)
	offset := int32(0)
	for {
		req := &model.ListCertificatesRequest{
			Limit:   &limit,
			Offset:  &offset,
			Content: &name,
		}
		resp, err := client.ListCertificates(req)
		if err != nil {
			// 查询失败不致命，回退到直接导入；导入若报重复由上层处理
			logging.Debug(i18n.T("log.deploy.huawei_query_existing_failed", "Name", name, "Err", err, "Resp", respDump(resp)))
			return "", false
		}
		if resp.Certificates == nil || len(*resp.Certificates) == 0 {
			return "", false
		}
		for _, c := range *resp.Certificates {
			if c.Id == "" {
				continue
			}
			exp, e2 := client.ExportCertificate(&model.ExportCertificateRequest{CertificateId: c.Id})
			if e2 != nil || exp.Certificate == nil {
				logging.Debug(i18n.T("log.deploy.huawei_export_failed", "CertID", c.Id, "Err", e2, "Resp", respDump(exp)))
				continue
			}
			if sameCertPEM(*exp.Certificate, certPEM) {
				logging.Debug(i18n.T("log.deploy.huawei_hit_existing", "CertID", c.Id, "Name", name))
				return c.Id, true
			}
		}
		total := int32(0)
		if resp.TotalCount != nil {
			total = *resp.TotalCount
		}
		offset += limit
		if total <= offset {
			return "", false
		}
	}
}

// DeployCert 将已导入的证书部署到华为云目标服务（如 CDN / WAF / ELB）。
// 注意：SCM 的 PushCertificate 只是把证书「推送」到目标服务（使其进入托管证书列表），
// 并不代表证书已绑定到具体域名。CDN 必须再调用 UpdateDomainMultiCertificates 按域名绑定，
// 域名才会真正用上该证书（这是与阿里云/腾讯云一致的「按域名绑定」语义）。
func (d *HuaweiDeployer) DeployCert(ctx context.Context, creds Credentials, certID string, svc string, svcConfig map[string]string) (*DeployResult, error) {
	rg := creds.Region
	if rg == "" {
		rg = "cn-north-1"
	}
	client, err := d.newScmClient(creds)
	if err != nil {
		return nil, i18n.Wrap(err, "deploy.error.huawei_scm_client_create")
	}
	logging.Debug(i18n.T("log.deploy.huawei_deploy_start", "Svc", svc, "CertID", certID, "Region", rg))
	switch svc {
	case "cdn":
		// CDN 需把证书真正绑定到具体加速域名：
		// 1) Push 证书到 CDN 服务，使其进入 CDN 托管证书列表（后续按证书名引用）；
		//    重复推送报 SCM.0211「cloudCertificate is exist」表示已推送过，继续后续绑定即可。
		// 2) 取证书名称，调用 UpdateDomainMultiCertificates 按域名绑定（这才是「部署到域名」）。
		domain := svcConfig["domain"]
		if domain == "" {
			return &DeployResult{CloudCertID: certID, Message: i18n.T("deploy.message.huawei_cdn_pushed_no_domain")}, nil
		}
		pushResp, err := client.PushCertificate(&model.PushCertificateRequest{
			CertificateId: certID,
			Body: &model.PushCertificateRequestBody{
				TargetProject: rg,
				TargetService: "CDN",
			},
		})
		if err != nil {
			// SCM.0211「cloudCertificate is exist」：证书已推送过（同一证书重复推送），
			// 仍需继续完成域名绑定，不当作成功返回（否则会误报「已部署」而实际未绑定）。
			if strings.Contains(err.Error(), "SCM.0211") && strings.Contains(err.Error(), "is exist") {
				logging.Debug(i18n.T("log.deploy.huawei_cdn_push_exists", "CertID", certID, "Err", err))
			} else {
				logging.Debug(i18n.T("log.deploy.huawei_cdn_push_failed", "CertID", certID, "Err", err, "Resp", respDump(pushResp)))
				return nil, i18n.Wrap(err, "deploy.error.huawei_cdn_push")
			}
		} else {
			logging.Debug(i18n.T("log.deploy.huawei_cdn_push_success", "CertID", certID, "Resp", respDump(pushResp)))
		}
		// 取证书名称（Push 后在 CDN 托管列表中以该名称引用）
		certName, err := d.certNameByID(client, certID, svcConfig)
		if err != nil {
			return nil, err
		}
		// 绑定到具体加速域名（SCM 托管证书模式：按证书名/SCM 证书 ID 引用，无需重传证书内容与私钥）。
		// 注意 v2 的 CertificateType 取值：2=华为云 SCM 证书（v1 的 1=托管证书的语义在 v2 下需改为 2）。
		cdnClient, err := d.newCdnClient(creds)
		if err != nil {
			return nil, err
		}
		certType := int32(2) // 2=SCM 证书
		updResp, err := cdnClient.UpdateDomainMultiCertificates(&cdnmodel.UpdateDomainMultiCertificatesRequest{
			Body: &cdnmodel.UpdateDomainMultiCertificatesRequestBody{
				Https: &cdnmodel.UpdateDomainMultiCertificatesRequestBodyContent{
					DomainName:       domain,
					HttpsSwitch:      int32(1),
					CertificateType:  &certType,
					CertName:         &certName,
					ScmCertificateId: &certID,
				},
			},
		})
		if err != nil {
			logging.Debug(i18n.T("log.deploy.huawei_cdn_bind_failed", "Domain", domain, "CertName", certName, "Err", err, "Resp", respDump(updResp)))
			return nil, i18n.Wrap(err, "deploy.error.huawei_cdn_bind", "Domain", domain)
		}
		logging.Debug(i18n.T("log.deploy.huawei_cdn_bind_success", "Domain", domain, "CertName", certName))
		return &DeployResult{CloudCertID: certID, RawResponse: respDump(updResp), Message: i18n.T("deploy.message.huawei_cdn_deployed", "Domain", domain)}, nil
	case "waf", "elb":
		// WAF / ELB 当前以 Push 作为部署动作（按实例引用证书，无现成的按域名绑定 API）。
		// 重复推送报 SCM.0211「cloudCertificate is exist」表示已部署，按幂等成功处理。
		pushResp, err := client.PushCertificate(&model.PushCertificateRequest{
			CertificateId: certID,
			Body: &model.PushCertificateRequestBody{
				TargetProject: rg,
				TargetService: upper(svc),
			},
		})
		if err != nil {
			if strings.Contains(err.Error(), "SCM.0211") && strings.Contains(err.Error(), "is exist") {
				logging.Debug(i18n.T("log.deploy.huawei_push_idempotent", "Svc", upper(svc), "CertID", certID, "Err", err))
				return &DeployResult{CloudCertID: certID, Message: i18n.T("deploy.message.huawei_svc_idempotent", "Service", upper(svc))}, nil
			}
			logging.Debug(i18n.T("log.deploy.huawei_push_failed", "Svc", upper(svc), "CertID", certID, "Err", err, "Resp", respDump(pushResp)))
			return nil, i18n.Wrap(err, "deploy.error.huawei_push_svc", "Service", upper(svc))
		}
		logging.Debug(i18n.T("log.deploy.huawei_push_success", "Svc", upper(svc), "CertID", certID))
		return &DeployResult{CloudCertID: certID, Message: i18n.T("deploy.message.huawei_svc_pushed", "Service", upper(svc))}, nil
	default:
		return &DeployResult{CloudCertID: certID, Message: i18n.T("deploy.message.huawei_scm_not_implemented", "Service", svc)}, nil
	}
}

// newCdnClient 构造华为云 CDN 客户端（CDN 为全局服务，使用 global 凭据）。
func (d *HuaweiDeployer) newCdnClient(creds Credentials) (*cdn.CdnClient, error) {
	rg := creds.Region
	if rg == "" {
		rg = "cn-north-1"
	}
	credential, err := global.NewCredentialsBuilder().WithAk(creds.AccessKeyID).WithSk(creds.AccessKeySecret).SafeBuild()
	if err != nil {
		return nil, i18n.Wrap(err, "deploy.error.huawei_credential_create")
	}
	reg, err := cdnregion.SafeValueOf(rg)
	if err != nil {
		// 用户填写的区域不在 CDN 支持列表时，回退到默认区域 cn-north-1（endpoint 自动为 https://cdn.myhuaweicloud.com）
		reg, err = cdnregion.SafeValueOf("cn-north-1")
		if err != nil {
			return nil, i18n.NewError("deploy.error.huawei_region_invalid", "Region", rg)
		}
	}
	// 仅用 WithRegion：endpoint 由 region 元数据自动推导，避免老式 WithEndpoint 写法。
	hc, err := cdn.CdnClientBuilder().
		WithCredential(credential).
		WithRegion(reg).
		WithHttpConfig(config.DefaultHttpConfig().WithHttpRoundTripper(httplog.WrapTransport(&http.Transport{}))).
		SafeBuild()
	if err != nil {
		return nil, i18n.Wrap(err, "deploy.error.huawei_cdn_client_create")
	}
	return cdn.NewCdnClient(hc), nil
}

// certNameByID 通过证书 ID 查询 SCM 中的证书名称。Push 到 CDN 后该名称即为 CDN 托管证书名，
// 绑定域名时需按此名称引用。SCM 未返回名称时回退到与上传一致的命名规则构造（默认域名）。
func (d *HuaweiDeployer) certNameByID(client *scm.ScmClient, certID string, svcConfig map[string]string) (string, error) {
	resp, err := client.ShowCertificate(&model.ShowCertificateRequest{CertificateId: certID})
	if err != nil {
		return "", i18n.Wrap(err, "deploy.error.huawei_cert_name_query")
	}
	if resp.Name != nil && *resp.Name != "" {
		return *resp.Name, nil
	}
	return certName(svcConfig["cert_domain"], svcConfig), nil
}

// ListDomains 列出华为云 CDN 加速域名
// ListSites 云厂商默认回退到 ListDomains。
func (d *HuaweiDeployer) ListSites(ctx context.Context, creds Credentials, svc, region, zoneID string) ([]string, error) {
	return d.ListDomains(ctx, creds, svc, region, zoneID)
}

func (d *HuaweiDeployer) ListDomains(ctx context.Context, creds Credentials, svc, region, zoneID string) ([]string, error) {
	rg := creds.Region
	client, err := d.newCdnClient(creds)
	if err != nil {
		logging.Debug(i18n.T("log.deploy.huawei_list_create_client_failed", "Err", err))
		return nil, err
	}
	logging.Debug(i18n.T("log.deploy.huawei_list_start", "Svc", svc))
	resp, err := client.ListDomains(&cdnmodel.ListDomainsRequest{})
	if err != nil {
		logging.Debug(i18n.T("log.deploy.huawei_list_failed", "Region", rg, "Err", err, "Resp", respDump(resp)))
		return nil, i18n.Wrap(err, "deploy.error.huawei_cdn_list_domains")
	}
	if resp.Domains == nil {
		return []string{}, nil
	}
	var domains []string
	for _, dm := range *resp.Domains {
		if dm.DomainName != nil && *dm.DomainName != "" {
			domains = append(domains, *dm.DomainName)
		}
	}
	logging.Debug(i18n.T("log.deploy.huawei_list_success", "Region", rg, "Count", len(domains)))
	return domains, nil
}

func upper(s string) string {
	if s == "" {
		return s
	}
	b := []byte(s)
	b[0] = toUpper(b[0])
	return string(b)
}

func toUpper(c byte) byte {
	if c >= 'a' && c <= 'z' {
		return c - ('a' - 'A')
	}
	return c
}
