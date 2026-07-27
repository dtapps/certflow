package deploy

// DomainTargetConfig 是 CDN/云厂商类部署目标 config 的结构化定义。
// CDN 类（aliyun / huawei / tencentcloud / baiducloud / ctyun / volcengine）通过 domain 指定域名，
// 各厂商有独立的 region / cert_name / zone_id 等字段。
type DomainTargetConfig struct {
	Region        string `json:"region,omitempty"`
	RegionID      string `json:"region_id,omitempty"`
	CertName      string `json:"cert_name,omitempty"`
	Domain        string `json:"domain,omitempty"`
	CertDomain    string `json:"cert_domain,omitempty"`
	ZoneID        string `json:"zone_id,omitempty"`
	ZoneName      string `json:"zone_name,omitempty"`
	AcceleratorID string `json:"accelerator_id,omitempty"`
	ListenerID    string `json:"listener_id,omitempty"`
	DeployService string `json:"deploy_service,omitempty"`
}
