package schema

// DeployProviderTypes 部署相关的云厂商/提供商类型常量
// deploy_credential 和 deploy_target 共享此定义
var DeployProviderTypes = []string{
	// 云厂商
	"aliyun", "tencentcloud", "huawei", "baiducloud", "ctyun", "volcengine",
	// 面板
	"btpanel", "1panel", "acepanel", "aapanel",
	// 防火墙
	"aawaf",
}
