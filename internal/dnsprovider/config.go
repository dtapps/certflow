package dnsprovider

import (
	"fmt"

	"cnb.cool/dtapp/certflow/internal/cloudcred"
	"cnb.cool/dtapp/certflow/internal/config"
)

// 以下结构体是 dns_providers.config 按厂商逐一声明的强类型定义。
// 每个厂商的字段名严格对应其 lego provider 所需的配置键（见 internal/certificate/dns_provider.go），
// 解析时使用泛型 config.ParseConfig 反序列化，取代原先散落的 map[string]string 字符串读取。
// 敏感字段以 secret:"true" 标记，便于通过 config.StripSecrets 向前端返回时剔除。

// 以下为支持「作为部署凭证来源」的云厂商（deploy_credential / dns_provider 来源均可复用其密钥字段），
// 它们额外实现 toCredentials 以统一提取 cloudcred.Credentials。

// regionOf 优先返回 region，为空时回退 region_id。
func regionOf(region, regionID string) string {
	if region != "" {
		return region
	}
	return regionID
}

// AliyunConfig 阿里云 DNS
type AliyunConfig struct {
	AccessKeyID     string `json:"access_key_id" secret:"true"`
	AccessKeySecret string `json:"access_key_secret" secret:"true"`
	RegionID        string `json:"region_id"`
}

func (c AliyunConfig) toCredentials() cloudcred.Credentials {
	return cloudcred.Credentials{AccessKeyID: c.AccessKeyID, AccessKeySecret: c.AccessKeySecret, Region: regionOf("", c.RegionID)}
}

// HuaweiConfig 华为云 DNS
type HuaweiConfig struct {
	AccessKeyID     string `json:"access_key_id" secret:"true"`
	SecretAccessKey string `json:"secret_access_key" secret:"true"`
	Region          string `json:"region"`
}

func (c HuaweiConfig) toCredentials() cloudcred.Credentials {
	return cloudcred.Credentials{AccessKeyID: c.AccessKeyID, AccessKeySecret: c.SecretAccessKey, Region: regionOf(c.Region, "")}
}

// TencentConfig 腾讯云 DNS
type TencentConfig struct {
	SecretID  string `json:"secret_id" secret:"true"`
	SecretKey string `json:"secret_key" secret:"true"`
	Region    string `json:"region"`
}

func (c TencentConfig) toCredentials() cloudcred.Credentials {
	return cloudcred.Credentials{AccessKeyID: c.SecretID, AccessKeySecret: c.SecretKey, Region: regionOf(c.Region, "")}
}

// BaiduConfig 百度云 DNS（使用 secret_access_key）
type BaiduConfig struct {
	AccessKeyID     string `json:"access_key_id" secret:"true"`
	SecretAccessKey string `json:"secret_access_key" secret:"true"`
	Region          string `json:"region"`
}

func (c BaiduConfig) toCredentials() cloudcred.Credentials {
	return cloudcred.Credentials{AccessKeyID: c.AccessKeyID, AccessKeySecret: c.SecretAccessKey, Region: regionOf(c.Region, "")}
}

// VolcengineConfig 火山引擎 DNS（使用 access_key / secret_key）
type VolcengineConfig struct {
	AccessKey string `json:"access_key" secret:"true"`
	SecretKey string `json:"secret_key" secret:"true"`
	Region    string `json:"region"`
}

func (c VolcengineConfig) toCredentials() cloudcred.Credentials {
	return cloudcred.Credentials{AccessKeyID: c.AccessKey, AccessKeySecret: c.SecretKey, Region: regionOf(c.Region, "")}
}

// 以下为其余 DNS 厂商配置（仅用于构造 lego provider，不作为部署凭证来源）。

// CloudflareConfig Cloudflare
type CloudflareConfig struct {
	Email    string `json:"email"`
	APIKey   string `json:"api_key" secret:"true"`
	APIToken string `json:"api_token" secret:"true"`
}

// Route53Config AWS Route53
type Route53Config struct {
	AccessKeyID     string `json:"access_key_id" secret:"true"`
	SecretAccessKey string `json:"secret_access_key" secret:"true"`
	Region          string `json:"region"`
}

// GoogleCloudConfig Google Cloud DNS
type GoogleCloudConfig struct {
	ClientID string `json:"client_id" secret:"true"`
	Email    string `json:"email"`
	Password string `json:"password" secret:"true"`
}

// JdcloudConfig 京东云 DNS
type JdcloudConfig struct {
	AccessKeyID     string `json:"access_key_id" secret:"true"`
	AccessKeySecret string `json:"access_key_secret" secret:"true"`
	RegionID        string `json:"region_id"`
}

// EdgeOneConfig 腾讯云 EdgeOne
type EdgeOneConfig struct {
	SecretID  string `json:"secret_id" secret:"true"`
	SecretKey string `json:"secret_key" secret:"true"`
	Region    string `json:"region"`
}

// AliEsaConfig 阿里云 ESA
type AliEsaConfig struct {
	APIKey    string `json:"api_key" secret:"true"`
	SecretKey string `json:"secret_key" secret:"true"`
	RegionID  string `json:"region_id"`
}

// UCloudConfig 优刻得
type UCloudConfig struct {
	PublicKey  string `json:"public_key" secret:"true"`
	PrivateKey string `json:"private_key" secret:"true"`
	Region     string `json:"region"`
}

// WestCNConfig 西部数码
type WestCNConfig struct {
	Username string `json:"username" secret:"true"`
	Password string `json:"password" secret:"true"`
}

// Com35Config 35 互联
type Com35Config struct {
	Username string `json:"username" secret:"true"`
	Password string `json:"password" secret:"true"`
}

// RainYunConfig 雨云
type RainYunConfig struct {
	APIKey string `json:"api_key" secret:"true"`
}

// TodayNICConfig 今天互联
type TodayNICConfig struct {
	AuthUserID string `json:"auth_user_id" secret:"true"`
	APIKey     string `json:"api_key" secret:"true"`
}

// DNSLAConfig DNS.LA
type DNSLAConfig struct {
	APIID     string `json:"api_id" secret:"true"`
	APISecret string `json:"api_secret" secret:"true"`
}

// DNS51Config 51DNS
type DNS51Config struct {
	APIKey    string `json:"api_key" secret:"true"`
	APISecret string `json:"api_secret" secret:"true"`
}

// XinnetConfig 新网
type XinnetConfig struct {
	Secret  string `json:"secret" secret:"true"`
	AgentID string `json:"agent_id" secret:"true"`
}

// PorkbunConfig Porkbun
type PorkbunConfig struct {
	APIKey       string `json:"api_key" secret:"true"`
	SecretAPIKey string `json:"secret_api_key" secret:"true"`
}

// NamecheapConfig Namecheap
type NamecheapConfig struct {
	APIUser  string `json:"api_user" secret:"true"`
	APIKey   string `json:"api_key" secret:"true"`
	ClientIP string `json:"client_ip"`
}

// GoDaddyConfig GoDaddy
type GoDaddyConfig struct {
	APIKey    string `json:"api_key" secret:"true"`
	APISecret string `json:"api_secret" secret:"true"`
}

// GandiV5Config Gandi V5
type GandiV5Config struct {
	PersonalAccessToken string `json:"personal_access_token" secret:"true"`
}

// DynadotConfig Dynadot
type DynadotConfig struct {
	APIKey    string `json:"api_key" secret:"true"`
	APISecret string `json:"api_secret" secret:"true"`
}

// AzureConfig Azure DNS
type AzureConfig struct {
	SubscriptionID string `json:"subscription_id" secret:"true"`
	ResourceGroup  string `json:"resource_group"`
	ClientID       string `json:"client_id" secret:"true"`
	ClientSecret   string `json:"client_secret" secret:"true"`
	TenantID       string `json:"tenant_id" secret:"true"`
}

// DigitalOceanConfig DigitalOcean
type DigitalOceanConfig struct {
	AuthToken string `json:"auth_token" secret:"true"`
}

// VultrConfig Vultr
type VultrConfig struct {
	APIKey string `json:"api_key" secret:"true"`
}

// HetznerConfig Hetzner
type HetznerConfig struct {
	APIToken string `json:"api_token" secret:"true"`
}

// LinodeConfig Linode
type LinodeConfig struct {
	Token string `json:"token" secret:"true"`
}

// OVHConfig OVH
type OVHConfig struct {
	ApplicationKey    string `json:"application_key" secret:"true"`
	ApplicationSecret string `json:"application_secret" secret:"true"`
	ConsumerKey       string `json:"consumer_key" secret:"true"`
}

// DNSimpleConfig DNSimple
type DNSimpleConfig struct {
	AccessToken string `json:"access_token" secret:"true"`
}

// NS1Config NS1
type NS1Config struct {
	APIKey string `json:"api_key" secret:"true"`
}

// credentialer 可由结构化配置提取统一凭证的接口（方法为包内可见）。
type credentialer interface {
	toCredentials() cloudcred.Credentials
}

// parseCred 泛型解析为具体厂商凭证结构体并提取统一凭证，错误上抛。
func parseCred[T credentialer](raw []byte) (cloudcred.Credentials, error) {
	v, err := config.ParseConfig[T](raw)
	if err != nil {
		return cloudcred.Credentials{}, err
	}
	return v.toCredentials(), nil
}

// parseAny 泛型解析为具体厂商结构体并以 any 返回，由调用方类型断言。
func parseAny[T any](raw []byte) (any, error) {
	v, err := config.ParseConfig[T](raw)
	if err != nil {
		return nil, err
	}
	return v, nil
}

// Parse 按厂商标识将存储的配置字节解析为对应厂商的结构体（以 any 返回，由调用方类型断言）。
// 使用泛型 config.ParseConfig 完成反序列化。
func Parse(providerType string, raw []byte) (any, error) {
	switch providerType {
	case "cloudflare":
		return parseAny[CloudflareConfig](raw)
	case "aliyun":
		return parseAny[AliyunConfig](raw)
	case "huawei":
		return parseAny[HuaweiConfig](raw)
	case "tencentcloud":
		return parseAny[TencentConfig](raw)
	case "aws":
		return parseAny[Route53Config](raw)
	case "googlecloud":
		return parseAny[GoogleCloudConfig](raw)
	case "baiducloud":
		return parseAny[BaiduConfig](raw)
	case "jdcloud":
		return parseAny[JdcloudConfig](raw)
	case "volcengine":
		return parseAny[VolcengineConfig](raw)
	case "edgeone":
		return parseAny[EdgeOneConfig](raw)
	case "aliesa":
		return parseAny[AliEsaConfig](raw)
	case "ucloud":
		return parseAny[UCloudConfig](raw)
	case "westcn":
		return parseAny[WestCNConfig](raw)
	case "com35":
		return parseAny[Com35Config](raw)
	case "rainyun":
		return parseAny[RainYunConfig](raw)
	case "todaynic":
		return parseAny[TodayNICConfig](raw)
	case "dnsla":
		return parseAny[DNSLAConfig](raw)
	case "dns51":
		return parseAny[DNS51Config](raw)
	case "xinnet":
		return parseAny[XinnetConfig](raw)
	case "porkbun":
		return parseAny[PorkbunConfig](raw)
	case "namecheap":
		return parseAny[NamecheapConfig](raw)
	case "godaddy":
		return parseAny[GoDaddyConfig](raw)
	case "gandiv5":
		return parseAny[GandiV5Config](raw)
	case "dynadot":
		return parseAny[DynadotConfig](raw)
	case "azuredns":
		return parseAny[AzureConfig](raw)
	case "digitalocean":
		return parseAny[DigitalOceanConfig](raw)
	case "vultr":
		return parseAny[VultrConfig](raw)
	case "hetzner":
		return parseAny[HetznerConfig](raw)
	case "linode":
		return parseAny[LinodeConfig](raw)
	case "ovh":
		return parseAny[OVHConfig](raw)
	case "dnsimple":
		return parseAny[DNSimpleConfig](raw)
	case "ns1":
		return parseAny[NS1Config](raw)
	default:
		return nil, fmt.Errorf("unsupported dns provider: %s", providerType)
	}
}

// ParseCredential 当部署目标以 dns_provider 作为凭证来源时，提取其云厂商密钥为 cloudcred.Credentials。
// 仅支持具备 AK/SK 的云厂商（与 deploy_credential 来源一致）。
func ParseCredential(providerType string, raw []byte) (cloudcred.Credentials, error) {
	switch providerType {
	case "aliyun":
		return parseCred[AliyunConfig](raw)
	case "huawei":
		return parseCred[HuaweiConfig](raw)
	case "tencentcloud":
		return parseCred[TencentConfig](raw)
	case "baiducloud":
		return parseCred[BaiduConfig](raw)
	case "volcengine":
		return parseCred[VolcengineConfig](raw)
	default:
		return cloudcred.Credentials{}, fmt.Errorf("dns provider %s cannot be used as deploy credential", providerType)
	}
}
