package schema

// DeployCredentialConfig 是部署凭证配置的统一结构体。
// 包含所有厂商可能的字段（面板类和云厂商类），按 provider_type 不同使用不同字段。
// 敏感字段以 secret:"true" 标记，便于通过 config.StripSecrets 向前端返回时剔除。
type DeployCredentialConfig struct {
	// 云厂商 AK/SK 类
	AccessKeyID     string `json:"access_key_id,omitempty" secret:"true"`
	AccessKeySecret string `json:"access_key_secret,omitempty" secret:"true"`
	SecretAccessKey string `json:"secret_access_key,omitempty" secret:"true"`
	SecretID        string `json:"secret_id,omitempty" secret:"true"`
	SecretKey       string `json:"secret_key,omitempty" secret:"true"`

	// 区域
	Region   string `json:"region,omitempty"`
	RegionID string `json:"region_id,omitempty"`

	// 面板类
	APIKey      string `json:"api_key,omitempty" secret:"true"`
	PanelURL    string `json:"panel_url,omitempty"`
	TokenID     string `json:"token_id,omitempty" secret:"true"`
	TokenSecret string `json:"token_secret,omitempty" secret:"true"`
	JWTSecret   string `json:"jwt_secret,omitempty" secret:"true"`
}

// DeployProviderTypes 部署相关的云厂商/提供商类型常量
// deploy_credential 和 deploy_target 共享此定义
var DeployProviderTypes = []string{
	// 云厂商
	"aliyun", "tencentcloud", "huawei", "baiducloud", "ctyun", "volcengine",
	// 面板
	"btpanel", "1panel", "acepanel", "aapanel", "openrestymanager", "uuwaf",
	// 防火墙
	"aawaf", "safeline",
}

// DNSProviderConfig 是 DNS 提供商配置的统一结构体。
// 包含所有厂商可能的字段，按 provider_type 不同使用不同字段。
// 敏感字段以 secret:"true" 标记，便于通过 config.StripSecrets 向前端返回时剔除。
type DNSProviderConfig struct {
	// 云厂商 AK/SK 类
	AccessKeyID     string `json:"access_key_id,omitempty" secret:"true"`
	AccessKeySecret string `json:"access_key_secret,omitempty" secret:"true"`
	SecretAccessKey string `json:"secret_access_key,omitempty" secret:"true"`
	AccessKey       string `json:"access_key,omitempty" secret:"true"`
	SecretKey       string `json:"secret_key,omitempty" secret:"true"`
	SecretID        string `json:"secret_id,omitempty" secret:"true"`

	// 区域
	Region   string `json:"region,omitempty"`
	RegionID string `json:"region_id,omitempty"`

	// API Token / Key 类
	APIToken            string `json:"api_token,omitempty" secret:"true"`
	APIKey              string `json:"api_key,omitempty" secret:"true"`
	APISecret           string `json:"api_secret,omitempty" secret:"true"`
	ApplicationKey      string `json:"application_key,omitempty" secret:"true"`
	ApplicationSecret   string `json:"application_secret,omitempty" secret:"true"`
	ConsumerKey         string `json:"consumer_key,omitempty" secret:"true"`
	AuthToken           string `json:"auth_token,omitempty" secret:"true"`
	PersonalAccessToken string `json:"personal_access_token,omitempty" secret:"true"`
	Token               string `json:"token,omitempty" secret:"true"`
	AccessToken         string `json:"access_token,omitempty" secret:"true"`

	// 邮箱
	Email string `json:"email,omitempty"`

	// 密码类
	Password     string `json:"password,omitempty" secret:"true"`
	ClientSecret string `json:"client_secret,omitempty" secret:"true"`

	// Azure 特有
	SubscriptionID string `json:"subscription_id,omitempty" secret:"true"`
	ResourceGroup  string `json:"resource_group,omitempty"`
	ClientID       string `json:"client_id,omitempty" secret:"true"`
	TenantID       string `json:"tenant_id,omitempty" secret:"true"`

	// 面板类
	PublicKey  string `json:"public_key,omitempty" secret:"true"`
	PrivateKey string `json:"private_key,omitempty" secret:"true"`
	Username   string `json:"username,omitempty" secret:"true"`

	// 其他
	AgentID    string `json:"agent_id,omitempty" secret:"true"`
	AuthUserID string `json:"auth_user_id,omitempty" secret:"true"`
	APIID      string `json:"api_id,omitempty" secret:"true"`
	ClientIP   string `json:"client_ip,omitempty"`
}

var DnsProviderTypes = []string{
	"cloudflare", "aliyun", "tencentcloud", "huawei", "aws", "googlecloud",
	"baiducloud", "jdcloud", "volcengine", "edgeone", "aliesa",
	"ucloud", "westcn", "com35", "rainyun", "todaynic",
	"dnsla", "dns51", "xinnet",
}

// DeployTargetConfig 是部署目标配置的统一结构体。
// 包含所有部署目标可能的字段（CDN/云厂商类和面板类），按 provider_type/deploy_service 不同使用不同字段。
type DeployTargetConfig struct {
	// 区域（aliyun 用 region_id，其余用 region）
	Region   string `json:"region,omitempty"`
	RegionID string `json:"region_id,omitempty"`

	// 域名列表
	Domains []string `json:"domains,omitempty"`

	// 云端证书名
	CertName string `json:"cert_name,omitempty"`

	// EdgeOne 站点
	ZoneID   string `json:"zone_id,omitempty"`
	ZoneName string `json:"zone_name,omitempty"`

	// ESA/面板站点
	SiteID   []string `json:"site_id,omitempty"`
	SiteName []string `json:"site_name,omitempty"`

	// GA 加速器
	AcceleratorID string `json:"accelerator_id,omitempty"`
	ListenerID    string `json:"listener_id,omitempty"`
}
