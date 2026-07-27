package deploy

// SiteTargetConfig 是面板/防火墙类部署目标 config 的结构化定义。
// 面板类（btpanel / aapanel / 1panel / acepanel / aawaf）通过 site_name 多选站点，
// 前端存的是 JSON 数组字符串（如 "["site1","site2"]"），由 StringSlice 兼容解析。
// 注意：面板类部署的 panel_url 随「部署凭证」存储（见 deploycredential 包），不在目标配置中。
type SiteTargetConfig struct {
	SiteID        StringSlice `json:"site_id,omitempty"`
	SiteName      StringSlice `json:"site_name,omitempty"`
	DeployService string      `json:"deploy_service,omitempty"`
}
