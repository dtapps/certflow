package deploy

import (
	"context"
	"encoding/json"

	"cnb.cool/dtapp/certflow/internal/cloudcred"
)

// Credentials 云厂商访问凭证。
// 别名指向 cloudcred.Credentials，集中到中性包以避免 deploy 与解析包之间的循环依赖。
type Credentials = cloudcred.Credentials

// CertContent 待部署的证书内容
type CertContent struct {
	Domain  string
	CertPEM string
	KeyPEM  string
}

// DeployResult 部署结果
type DeployResult struct {
	CloudCertID string // 云厂商返回的证书 ID
	Message     string
	RawResponse string // 接口原始响应体（便于排查云端返回）
}

// Deployer 部署器接口，每个云厂商实现一个
type Deployer interface {
	// Provider 返回厂商标识：aliyun / tencentcloud / huawei
	Provider() string
	// UploadCert 将证书上传/导入到云厂商证书服务，返回证书 ID。
	// 该方法会被 DeployService 按「厂商+凭证+证书内容」去重缓存，保证同一张证书只上传一次。
	UploadCert(ctx context.Context, creds Credentials, cert CertContent, svcConfig map[string]string) (certID string, rawResponse string, err error)
	// DeployCert 将已上传的证书部署/绑定到目标服务（如 CDN）。certID 为 UploadCert 返回的证书 ID。
	DeployCert(ctx context.Context, creds Credentials, certID string, svc string, svcConfig map[string]string) (*DeployResult, error)
	// ListDomains 列出该账号在指定服务下的域名（如 CDN 加速域名），用于前端下拉选择。
	// zoneID 仅对部分服务有意义：EdgeOne 传入时返回该站点下的加速域名（hosts），
	// 不传时返回站点列表（供选择 ZoneId）。其余服务忽略该参数。
	ListDomains(ctx context.Context, creds Credentials, svc, region, zoneID string) ([]string, error)
}

// DeployOutcome 单次部署结果（供前端展示）
type DeployOutcome struct {
	TargetID    int    `json:"target_id"`
	TargetName  string `json:"target_name"`
	CloudCertID string `json:"cloud_cert_id"`
	Success     bool   `json:"success"`
	Message     string `json:"message"`
	RawResponse string `json:"raw_response"`
}

// parseConfig 解析 ent 的 []byte 配置为 map
func parseConfig(raw []byte) map[string]string {
	m := map[string]string{}
	if len(raw) == 0 {
		return m
	}
	_ = json.Unmarshal(raw, &m)
	return m
}

// certName 计算云端证书名称（允许用户通过 cert_name 覆盖，默认用域名）
func certName(domain string, cfg map[string]string) string {
	if n, ok := cfg["cert_name"]; ok && n != "" {
		return n
	}
	return domain
}
