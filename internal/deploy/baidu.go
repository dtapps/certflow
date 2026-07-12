package deploy

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"strings"

	"github.com/baidubce/bce-sdk-go/services/cdn"
	"github.com/baidubce/bce-sdk-go/services/cdn/api"
	bcecert "github.com/baidubce/bce-sdk-go/services/cert"

	"cnb.cool/dtapp/certflow/internal/i18n"
	"cnb.cool/dtapp/certflow/internal/logging"
)

// BaiduDeployer 百度云部署器：支持百度云 CDN 与全站加速 DRCDN。
//
// 百度云与阿里云 CAS / 腾讯云 SSL 一样采用「先上传证书到 SSL 证书服务拿 certId，
// 再让域名关联 certId」的两阶段模型（这也是控制台手动部署的方式）：
//   - UploadCert：把证书上传到百度云 SSL 证书服务（cert service），拿到全局 certId。
//   - DeployCert：调用 CDN 的 SetDomainHttps 把域名 HTTPS 关联到该 certId。
//
// CDN 与全站加速（DRCDN）走完全相同的路径，仅域名归属不同。证书为全局服务、不依赖 region，
// 且 certId 与私钥算法无关（RSA / ECDSA 均可），故不再走「按域名 PutCert 塞证书原文」的老路径。
type BaiduDeployer struct{}

func init() { RegisterDeployer(&BaiduDeployer{}) }

func (d *BaiduDeployer) Provider() string { return "baiducloud" }

// certKeyMatch 校验证书与私钥是否配对（公钥一致）。
// 百度云在证书/私钥不匹配时只会返回笼统错误，因此在上传前本地做一次配对校验，
// 可把这类问题转化成明确、可定位的错误。
func certKeyMatch(certPEM, keyPEM string) error {
	cBlock, _ := pem.Decode([]byte(certPEM))
	if cBlock == nil {
		return i18n.NewError("deploy.error.baidu_cert_pem_parse")
	}
	c, err := x509.ParseCertificate(cBlock.Bytes)
	if err != nil {
		return i18n.Wrap(err, "deploy.error.baidu_cert_parse")
	}
	kBlock, _ := pem.Decode([]byte(keyPEM))
	if kBlock == nil {
		return i18n.NewError("deploy.error.baidu_key_pem_parse")
	}
	pub, err := privateKeyPublicKey(kBlock.Bytes)
	if err != nil {
		return err
	}
	if !publicKeysEqual(c.PublicKey, pub) {
		return i18n.NewError("deploy.error.baidu_cert_key_mismatch")
	}
	return nil
}

// privateKeyPublicKey 从多种私钥格式（PKCS#1 / SEC1 / PKCS#8）中提取公钥。
func privateKeyPublicKey(der []byte) (crypto.PublicKey, error) {
	if k, err := x509.ParsePKCS1PrivateKey(der); err == nil {
		return &k.PublicKey, nil
	}
	if k, err := x509.ParseECPrivateKey(der); err == nil {
		return &k.PublicKey, nil
	}
	if k, err := x509.ParsePKCS8PrivateKey(der); err == nil {
		switch kt := k.(type) {
		case *rsa.PrivateKey:
			return &kt.PublicKey, nil
		case *ecdsa.PrivateKey:
			return &kt.PublicKey, nil
		}
	}
	return nil, i18n.NewError("deploy.error.baidu_key_type_unsupported")
}

// publicKeysEqual 比较两个公钥是否一致（仅处理 RSA / ECDSA）。
func publicKeysEqual(a, b crypto.PublicKey) bool {
	switch ka := a.(type) {
	case *rsa.PublicKey:
		if kb, ok := b.(*rsa.PublicKey); ok {
			return ka.Equal(kb)
		}
	case *ecdsa.PublicKey:
		if kb, ok := b.(*ecdsa.PublicKey); ok {
			return ka.Equal(kb)
		}
	}
	return false
}

// newClient 构造百度云 CDN 客户端（endpoint 默认 cdn.baidubce.com）。
// CDN 与全站加速（DRCDN）的域名 HTTPS 配置（SetDomainHttps/GetDomainHttps）均通过该客户端调用；
// 百度云证书为全局服务，不依赖 region，故忽略 creds.Region。
func (d *BaiduDeployer) newClient(creds Credentials) (*cdn.Client, error) {
	client, err := cdn.NewClient(creds.AccessKeyID, creds.AccessKeySecret, "")
	if err != nil {
		return nil, i18n.Wrap(err, "deploy.error.baidu_cdn_client_create")
	}
	return client, nil
}

// newCertClient 构造百度云 SSL 证书服务客户端（endpoint certificate.baidubce.com）。
// 证书须先注册到该服务、拿到 certId，再在域名 HTTPS 配置中引用，因此 UploadCert 通过该客户端上传。
func (d *BaiduDeployer) newCertClient(creds Credentials) (*bcecert.Client, error) {
	client, err := bcecert.NewClient(creds.AccessKeyID, creds.AccessKeySecret, "")
	if err != nil {
		return nil, i18n.Wrap(err, "deploy.error.baidu_cdn_client_create")
	}
	return client, nil
}

// baiduCertName 生成上传到百度云 SSL 证书服务时的证书名：
// 优先用配置里的 cert_name；否则以域名净化（去掉通配符 * 等非法字符）后追加内容指纹短后缀，
// 保证同一张证书幂等命名、不同证书（如续期后）不会重名冲突。
func baiduCertName(cert CertContent, cfg map[string]string) string {
	if n := cfg["cert_name"]; n != "" {
		return n
	}
	base := strings.NewReplacer("*", "wildcard", ".", "-", " ", "-").Replace(strings.TrimSpace(cert.Domain))
	sum := sha256.Sum256([]byte(cert.CertPEM + cert.KeyPEM))
	return fmt.Sprintf("certflow-%s-%s", base, hex.EncodeToString(sum[:])[:8])
}

// splitCertChain 把完整证书链 PEM 拆成叶子证书（第一块）与中间证书链（其余块）。
// 百度云证书服务要求 certServerData 放叶子证书、certLinkData 放中间证书链，故上传前需拆分。
func splitCertChain(certPEM string) (leaf, chain string) {
	var leafBuilder, chainBuilder strings.Builder
	rest := []byte(certPEM)
	first := true
	for {
		b, next := pem.Decode(rest)
		if b == nil {
			break
		}
		block := string(pem.EncodeToMemory(b))
		if first {
			leafBuilder.WriteString(block)
			first = false
		} else {
			chainBuilder.WriteString(block)
		}
		rest = next
	}
	return leafBuilder.String(), chainBuilder.String()
}

// UploadCert 把证书上传到百度云 SSL 证书服务并返回全局 certId（CDN 与 DRCDN 通用）。
// 上传前本地校验证书/私钥配对，把百度笼统的错误转成明确、可定位的报错。
func (d *BaiduDeployer) UploadCert(ctx context.Context, creds Credentials, cert CertContent, svcConfig map[string]string) (string, string, error) {
	if cert.CertPEM == "" || cert.KeyPEM == "" {
		return "", "", i18n.NewError("deploy.error.baidu_cert_empty")
	}
	if err := certKeyMatch(cert.CertPEM, cert.KeyPEM); err != nil {
		return "", "", err
	}
	cc, err := d.newCertClient(creds)
	if err != nil {
		return "", "", err
	}
	// 叶子证书放 certServerData、中间证书链放 certLinkData（官方证书参数语义）。
	leaf, chain := splitCertChain(cert.CertPEM)
	name := baiduCertName(cert, svcConfig)
	resp, err := cc.CreateCert(&bcecert.CreateCertArgs{
		CertName:        name,
		CertServerData:  leaf,
		CertPrivateData: cert.KeyPEM,
		CertLinkData:    chain,
	})
	if err != nil {
		logging.Debug(i18n.T("log.deploy.baidu_upload_failed", "Name", name, "Err", err))
		return "", "", i18n.Wrap(err, "deploy.error.baidu_upload_cert", "Name", name)
	}
	logging.Debug(i18n.T("log.deploy.baidu_upload", "Name", name, "CertID", resp.CertId))
	return resp.CertId, "", nil
}

// DeployCert 把已上传的证书（certId）关联到百度云目标域名的 HTTPS 配置（CDN 与 DRCDN 通用）。
// 先读取现有 HTTPS 配置，仅覆盖 certId 与 enabled，避免清空其他 HTTPS 设置。
func (d *BaiduDeployer) DeployCert(ctx context.Context, creds Credentials, certID string, svc string, svcConfig map[string]string) (*DeployResult, error) {
	domain := svcConfig["domain"]
	if domain == "" {
		return &DeployResult{CloudCertID: certID, Message: i18n.T("deploy.message.baidu_cdn_no_domain")}, nil
	}
	client, err := d.newClient(creds)
	if err != nil {
		return &DeployResult{CloudCertID: certID}, err
	}
	httpsCfg, herr := client.GetDomainHttps(domain)
	if herr != nil {
		httpsCfg = &api.HTTPSConfig{}
	}
	httpsCfg.Enabled = true
	httpsCfg.CertId = certID
	logging.Debug(i18n.T("log.deploy.baidu_deploy_start", "Svc", svc, "Domain", domain, "CertID", certID))
	if err := client.SetDomainHttps(domain, httpsCfg); err != nil {
		logging.Debug(i18n.T("log.deploy.baidu_deploy_failed", "Domain", domain, "Err", err))
		return &DeployResult{CloudCertID: certID}, i18n.Wrap(err, "deploy.error.baidu_set_https", "Service", svc, "Domain", domain)
	}
	logging.Debug(i18n.T("log.deploy.baidu_deploy_success", "Domain", domain))
	return &DeployResult{
		CloudCertID: certID,
		Message:     i18n.T("deploy.message.baidu_deployed", "Service", svc, "Domain", domain),
	}, nil
}

// ValidateCert 校验指定 certId 是否仍真实存在于百度云证书服务。
// ensureUploaded 复用数据库记录前会调用本方法：若证书已被手动删除，或该记录是旧版本（PutCert 时代）
// 遗留的脏数据（非真实 certId，如把内容指纹当 certId 存），返回 false 触发重新上传，
// 避免把无效 certId 关联到域名导致 ResourceNotFound。仅 ResourceNotFound 类错误视为失效；
// 网络/鉴权等其他错误保守返回 true（沿用原记录，不阻断部署）。
func (d *BaiduDeployer) ValidateCert(creds Credentials, certID string) (bool, error) {
	cc, err := d.newCertClient(creds)
	if err != nil {
		return true, nil //nolint:nilerr // 网络/鉴权错误时保守返回 true，不阻断部署
	}
	if _, err := cc.GetCertMeta(certID); err != nil {
		if strings.Contains(err.Error(), "ResourceNotFound") || strings.Contains(err.Error(), "No certificate found") {
			return false, nil
		}
		return true, nil
	}
	return true, nil
}

// ListDomains 列出百度云加速域名（分页），用于前端下拉选择。
// 直接使用 SDK 的 client.ListDomains(marker) 分页拉取。百度云 /v2/domain 接口不返回可靠的
// 域名类型，因此无法在此区分 CDN 与全站加速（DRCDN）——两类域名都返回，由用户在部署目标里
// 选择服务类型。两种服务的部署路径完全相同（上传证书拿 certId → SetDomainHttps 关联）。
func (d *BaiduDeployer) ListDomains(ctx context.Context, creds Credentials, svc, region, zoneID string) ([]string, error) {
	client, err := d.newClient(creds)
	if err != nil {
		return nil, err
	}
	logging.Debug(i18n.T("log.deploy.baidu_list_start", "Service", svc))
	var domains []string
	marker := ""
	for {
		page, nextMarker, err := client.ListDomains(marker)
		if err != nil {
			logging.Debug(i18n.T("log.deploy.baidu_list_failed", "Service", svc, "Err", err))
			return nil, i18n.Wrap(err, "deploy.error.baidu_cdn_list_domains")
		}
		for _, name := range page {
			if name == "" {
				continue
			}
			domains = append(domains, name)
		}
		if nextMarker == "" {
			break
		}
		marker = nextMarker
	}
	logging.Debug(i18n.T("log.deploy.baidu_list_success", "Service", svc, "Count", len(domains)))
	return domains, nil
}
