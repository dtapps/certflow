package deploy

// RegionFromConfig 从部署目标 config 中提取区域代码。
// 阿里云将区域存为 region_id，其余厂商存为 region；统一返回其一（优先 region）。
func RegionFromConfig(cfg map[string]string) string {
	if cfg == nil {
		return ""
	}
	if r := cfg["region"]; r != "" {
		return r
	}
	return cfg["region_id"]
}
