package deploy

import "cnb.cool/dtapp/certflow/internal/ent/schema"

// DeployTargetConfig 是部署目标配置的统一结构体，引用自 schema 包。
type DeployTargetConfig = schema.DeployTargetConfig

// DomainTargetConfig 是 CDN/云厂商类部署目标 config 的结构化定义。
// CDN 类（aliyun / huawei / tencentcloud / baiducloud / ctyun / volcengine）通过 domains 指定域名，
// 各厂商有独立的 region / cert_name / zone_id 等字段。
type DomainTargetConfig struct {
	Region   string `json:"region,omitempty"`
	RegionID string `json:"region_id,omitempty"`

	// 域名列表
	Domains []string `json:"domains,omitempty"`

	// 云端证书名
	CertName string `json:"cert_name,omitempty"`

	// EdgeOne 站点
	ZoneID   string `json:"zone_id,omitempty"`
	ZoneName string `json:"zone_name,omitempty"`

	// ESA 站点
	SiteID   string `json:"site_id,omitempty"`
	SiteName string `json:"site_name,omitempty"`

	// GA 加速器
	AcceleratorID string `json:"accelerator_id,omitempty"`
	ListenerID    string `json:"listener_id,omitempty"`
}
