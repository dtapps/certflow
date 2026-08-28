package deploy

// SiteTargetConfig 是面板/防火墙类部署目标 config 的结构化定义。
// 面板类（btpanel / aapanel / 1panel / acepanel / aawaf）通过 site_name 多选站点，
// 部署时直接使用数组。
// 注意：面板类部署的 panel_url 随「部署凭证」存储（见 deploycredential 包），不在目标配置中。
type SiteTargetConfig struct {
	SiteID   []string `json:"site_id,omitempty"`
	SiteName []string `json:"site_name,omitempty"`
}
