package deploy

// DeployTargetConfig 是 deploy_targets.config 的结构化定义。
// 不同厂商的服务配置字段名不同，统一声明于此；解析时使用泛型 config.ParseConfig[DeployTargetConfig]，
// 解析后再通过 config.AsMap 转回 map 供各部署器消费（json tag 与存储键名保持一致）。
type DeployTargetConfig struct {
	Region        string `json:"region,omitempty"`
	RegionID      string `json:"region_id,omitempty"`
	CertName      string `json:"cert_name,omitempty"`
	Domain        string `json:"domain,omitempty"`
	CertDomain    string `json:"cert_domain,omitempty"`
	ZoneID        string `json:"zone_id,omitempty"`
	SiteID        string `json:"site_id,omitempty"`
	AcceleratorID string `json:"accelerator_id,omitempty"`
	ListenerID    string `json:"listener_id,omitempty"`
	DeployService string `json:"deploy_service,omitempty"`
}
