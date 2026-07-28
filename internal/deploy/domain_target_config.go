package deploy

// DomainTargetConfig 是 CDN/云厂商类部署目标 config 的结构化定义。
// CDN 类（aliyun / huawei / tencentcloud / baiducloud / ctyun / volcengine）通过 domain 指定域名，
// 各厂商有独立的 region / cert_name / zone_id 等字段。
type DomainTargetConfig struct {
	Region        string `json:"region,omitempty"`
	RegionID      string `json:"region_id,omitempty"`
	CertName      string `json:"cert_name,omitempty"`
	Domain        string `json:"domain,omitempty"`
	Domains       string `json:"domains,omitempty"` // 关联域名列表（JSON 数组字符串），与前端 deployRows 来源一致
	CertDomain    string `json:"cert_domain,omitempty"`
	ZoneID        string `json:"zone_id,omitempty"`
	ZoneName      string `json:"zone_name,omitempty"`
	AcceleratorID string `json:"accelerator_id,omitempty"`
	ListenerID    string `json:"listener_id,omitempty"`
	SiteID        string `json:"site_id,omitempty"`   // ESA 站点 ID（阿里云 ESA）
	SiteName      string `json:"site_name,omitempty"` // ESA 站点名称
	DeployService string `json:"deploy_service,omitempty"`
}
