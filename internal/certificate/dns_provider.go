package certificate

import (
	"fmt"
	"time"

	"cnb.cool/dtapp/certflow/ent"
	"cnb.cool/dtapp/certflow/internal/dnsprovider"
	"cnb.cool/dtapp/certflow/internal/httplog"
	"cnb.cool/dtapp/certflow/internal/i18n"
	"github.com/go-acme/lego/v5/challenge"
	"github.com/go-acme/lego/v5/providers/dns/alidns"
	"github.com/go-acme/lego/v5/providers/dns/aliesa"
	"github.com/go-acme/lego/v5/providers/dns/azuredns"
	"github.com/go-acme/lego/v5/providers/dns/baiducloud"
	"github.com/go-acme/lego/v5/providers/dns/clouddns"
	"github.com/go-acme/lego/v5/providers/dns/cloudflare"
	"github.com/go-acme/lego/v5/providers/dns/com35"
	"github.com/go-acme/lego/v5/providers/dns/digitalocean"
	"github.com/go-acme/lego/v5/providers/dns/dns51"
	"github.com/go-acme/lego/v5/providers/dns/dnsimple"
	"github.com/go-acme/lego/v5/providers/dns/dnsla"
	"github.com/go-acme/lego/v5/providers/dns/dynadot"
	"github.com/go-acme/lego/v5/providers/dns/edgeone"
	"github.com/go-acme/lego/v5/providers/dns/gandiv5"
	"github.com/go-acme/lego/v5/providers/dns/godaddy"
	"github.com/go-acme/lego/v5/providers/dns/hetzner"
	"github.com/go-acme/lego/v5/providers/dns/huaweicloud"
	"github.com/go-acme/lego/v5/providers/dns/jdcloud"
	"github.com/go-acme/lego/v5/providers/dns/linode"
	"github.com/go-acme/lego/v5/providers/dns/namecheap"
	"github.com/go-acme/lego/v5/providers/dns/ns1"
	"github.com/go-acme/lego/v5/providers/dns/ovh"
	"github.com/go-acme/lego/v5/providers/dns/porkbun"
	"github.com/go-acme/lego/v5/providers/dns/rainyun"
	"github.com/go-acme/lego/v5/providers/dns/route53"
	"github.com/go-acme/lego/v5/providers/dns/tencentcloud"
	"github.com/go-acme/lego/v5/providers/dns/todaynic"
	"github.com/go-acme/lego/v5/providers/dns/ucloud"
	"github.com/go-acme/lego/v5/providers/dns/volcengine"
	"github.com/go-acme/lego/v5/providers/dns/vultr"
	"github.com/go-acme/lego/v5/providers/dns/westcn"
	"github.com/go-acme/lego/v5/providers/dns/xinnet"
)

// createDNSProvider 根据提供商类型和配置创建 lego DNS provider。
// 配置按厂商经 dnsprovider.Parse 解析为强类型结构体（泛型反序列化），
// 再通过类型断言分派到对应的构造器，彻底消除原先散落的 map[string]string 字符串读取。
func createDNSProvider(provider *ent.DNSProvider) (challenge.Provider, error) {
	if len(provider.Config) == 0 {
		return nil, fmt.Errorf("%s", i18n.T("error.dns_provider_config_empty"))
	}
	cfg, err := dnsprovider.Parse(provider.ProviderType.String(), provider.Config)
	if err != nil {
		return nil, fmt.Errorf("%s", i18n.T("error.dns_provider_config_parse_failed", "Error", err))
	}
	switch c := cfg.(type) {
	case dnsprovider.CloudflareConfig:
		return createCloudflareProvider(c)
	case dnsprovider.AliyunConfig:
		return createAliyunProvider(c)
	case dnsprovider.HuaweiConfig:
		return createHuaweiProvider(c)
	case dnsprovider.TencentConfig:
		return createTencentCloudProvider(c)
	case dnsprovider.Route53Config:
		return createRoute53Provider(c)
	case dnsprovider.GoogleCloudConfig:
		return createGoogleCloudProvider(c)
	case dnsprovider.BaiduConfig:
		return createBaiduCloudProvider(c)
	case dnsprovider.JdcloudConfig:
		return createJDCloudProvider(c)
	case dnsprovider.VolcengineConfig:
		return createVolcengineProvider(c)
	case dnsprovider.EdgeOneConfig:
		return createEdgeOneProvider(c)
	case dnsprovider.AliEsaConfig:
		return createAliesaProvider(c)
	case dnsprovider.UCloudConfig:
		return createUCloudProvider(c)
	case dnsprovider.WestCNConfig:
		return createWestCNProvider(c)
	case dnsprovider.Com35Config:
		return createCom35Provider(c)
	case dnsprovider.RainYunConfig:
		return createRainYunProvider(c)
	case dnsprovider.TodayNICConfig:
		return createTodayNICProvider(c)
	case dnsprovider.DNSLAConfig:
		return createDNSLAProvider(c)
	case dnsprovider.DNS51Config:
		return createDNS51Provider(c)
	case dnsprovider.XinnetConfig:
		return createXinnetProvider(c)
	case dnsprovider.PorkbunConfig:
		return createPorkbunProvider(c)
	case dnsprovider.NamecheapConfig:
		return createNamecheapProvider(c)
	case dnsprovider.GoDaddyConfig:
		return createGoDaddyProvider(c)
	case dnsprovider.GandiV5Config:
		return createGandiV5Provider(c)
	case dnsprovider.DynadotConfig:
		return createDynadotProvider(c)
	case dnsprovider.AzureConfig:
		return createAzureDNSProvider(c)
	case dnsprovider.DigitalOceanConfig:
		return createDigitalOceanProvider(c)
	case dnsprovider.VultrConfig:
		return createVultrProvider(c)
	case dnsprovider.HetznerConfig:
		return createHetznerProvider(c)
	case dnsprovider.LinodeConfig:
		return createLinodeProvider(c)
	case dnsprovider.OVHConfig:
		return createOVHProvider(c)
	case dnsprovider.DNSimpleConfig:
		return createDNSimpleProvider(c)
	case dnsprovider.NS1Config:
		return createNS1Provider(c)
	default:
		return nil, fmt.Errorf("%s", i18n.T("error.dns_provider_unsupported", "Type", provider.ProviderType))
	}
}

// createCloudflareProvider 创建 Cloudflare DNS provider，用于 ACME DNS 验证
func createCloudflareProvider(c dnsprovider.CloudflareConfig) (challenge.Provider, error) {
	cfg := cloudflare.NewDefaultConfig()
	cfg.HTTPClient = httplog.WrapClient(cfg.HTTPClient)
	cfg.AuthEmail = c.Email
	cfg.AuthKey = c.APIKey
	cfg.AuthToken = c.APIToken
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return cloudflare.NewDNSProviderConfig(cfg)
}

// createAliyunProvider 创建阿里云 DNS provider，用于 ACME DNS 验证
func createAliyunProvider(c dnsprovider.AliyunConfig) (challenge.Provider, error) {
	cfg := alidns.NewDefaultConfig()
	cfg.APIKey = c.AccessKeyID
	cfg.SecretKey = c.AccessKeySecret
	cfg.RegionID = c.RegionID
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return alidns.NewDNSProviderConfig(cfg)
}

// createHuaweiProvider 创建华为云 DNS provider，用于 ACME DNS 验证
func createHuaweiProvider(c dnsprovider.HuaweiConfig) (challenge.Provider, error) {
	cfg := huaweicloud.NewDefaultConfig()
	cfg.AccessKeyID = c.AccessKeyID
	cfg.SecretAccessKey = c.SecretAccessKey
	cfg.Region = c.Region
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return huaweicloud.NewDNSProviderConfig(cfg)
}

// createTencentCloudProvider 创建腾讯云 DNS provider，用于 ACME DNS 验证
func createTencentCloudProvider(c dnsprovider.TencentConfig) (challenge.Provider, error) {
	cfg := tencentcloud.NewDefaultConfig()
	cfg.SecretID = c.SecretID
	cfg.SecretKey = c.SecretKey
	cfg.Region = c.Region
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return tencentcloud.NewDNSProviderConfig(cfg)
}

// createRoute53Provider 创建 AWS Route53 DNS provider，用于 ACME DNS 验证
func createRoute53Provider(c dnsprovider.Route53Config) (challenge.Provider, error) {
	cfg := route53.NewDefaultConfig()
	cfg.AccessKeyID = c.AccessKeyID
	cfg.SecretAccessKey = c.SecretAccessKey
	cfg.Region = c.Region
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return route53.NewDNSProviderConfig(cfg)
}

// createGoogleCloudProvider 创建 Google Cloud DNS provider，用于 ACME DNS 验证
func createGoogleCloudProvider(c dnsprovider.GoogleCloudConfig) (challenge.Provider, error) {
	cfg := clouddns.NewDefaultConfig()
	cfg.HTTPClient = httplog.WrapClient(cfg.HTTPClient)
	cfg.ClientID = c.ClientID
	cfg.Email = c.Email
	cfg.Password = c.Password
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return clouddns.NewDNSProviderConfig(cfg)
}

// createBaiduCloudProvider 创建百度智能云 DNS provider，用于 ACME DNS 验证
func createBaiduCloudProvider(c dnsprovider.BaiduConfig) (challenge.Provider, error) {
	cfg := baiducloud.NewDefaultConfig()
	cfg.AccessKeyID = c.AccessKeyID
	cfg.SecretAccessKey = c.SecretAccessKey
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return baiducloud.NewDNSProviderConfig(cfg)
}

// createJDCloudProvider 创建京东云 DNS provider，用于 ACME DNS 验证
func createJDCloudProvider(c dnsprovider.JdcloudConfig) (challenge.Provider, error) {
	cfg := jdcloud.NewDefaultConfig()
	cfg.AccessKeyID = c.AccessKeyID
	cfg.AccessKeySecret = c.AccessKeySecret
	cfg.RegionID = c.RegionID
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return jdcloud.NewDNSProviderConfig(cfg)
}

// createVolcengineProvider 创建火山引擎 DNS provider，用于 ACME DNS 验证
func createVolcengineProvider(c dnsprovider.VolcengineConfig) (challenge.Provider, error) {
	cfg := volcengine.NewDefaultConfig()
	cfg.AccessKey = c.AccessKey
	cfg.SecretKey = c.SecretKey
	cfg.Region = c.Region
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return volcengine.NewDNSProviderConfig(cfg)
}

// createEdgeOneProvider 创建腾讯云 EdgeOne DNS provider，用于 ACME DNS 验证
func createEdgeOneProvider(c dnsprovider.EdgeOneConfig) (challenge.Provider, error) {
	cfg := edgeone.NewDefaultConfig()
	cfg.SecretID = c.SecretID
	cfg.SecretKey = c.SecretKey
	cfg.Region = c.Region
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return edgeone.NewDNSProviderConfig(cfg)
}

// createAliesaProvider 创建阿里云 ESA(边缘安全加速) DNS provider，用于 ACME DNS 验证
func createAliesaProvider(c dnsprovider.AliEsaConfig) (challenge.Provider, error) {
	cfg := aliesa.NewDefaultConfig()
	cfg.APIKey = c.APIKey
	cfg.SecretKey = c.SecretKey
	cfg.RegionID = c.RegionID
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return aliesa.NewDNSProviderConfig(cfg)
}

// createUCloudProvider 创建 UCloud DNS provider，用于 ACME DNS 验证
func createUCloudProvider(c dnsprovider.UCloudConfig) (challenge.Provider, error) {
	cfg := ucloud.NewDefaultConfig()
	cfg.PublicKey = c.PublicKey
	cfg.PrivateKey = c.PrivateKey
	cfg.Region = c.Region
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return ucloud.NewDNSProviderConfig(cfg)
}

// createWestCNProvider 创建西部数码 DNS provider，用于 ACME DNS 验证
func createWestCNProvider(c dnsprovider.WestCNConfig) (challenge.Provider, error) {
	cfg := westcn.NewDefaultConfig()
	cfg.Username = c.Username
	cfg.Password = c.Password
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return westcn.NewDNSProviderConfig(cfg)
}

// createCom35Provider 创建 35 互联 DNS provider，用于 ACME DNS 验证
func createCom35Provider(c dnsprovider.Com35Config) (challenge.Provider, error) {
	cfg := com35.NewDefaultConfig()
	cfg.Username = c.Username
	cfg.Password = c.Password
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return com35.NewDNSProviderConfig(cfg)
}

// createRainYunProvider 创建雨云 DNS provider，用于 ACME DNS 验证
func createRainYunProvider(c dnsprovider.RainYunConfig) (challenge.Provider, error) {
	cfg := rainyun.NewDefaultConfig()
	cfg.HTTPClient = httplog.WrapClient(cfg.HTTPClient)
	cfg.APIKey = c.APIKey
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return rainyun.NewDNSProviderConfig(cfg)
}

// createTodayNICProvider 创建 TodayDNS(时代互联) DNS provider，用于 ACME DNS 验证
func createTodayNICProvider(c dnsprovider.TodayNICConfig) (challenge.Provider, error) {
	cfg := todaynic.NewDefaultConfig()
	cfg.HTTPClient = httplog.WrapClient(cfg.HTTPClient)
	cfg.AuthUserID = c.AuthUserID
	cfg.APIKey = c.APIKey
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return todaynic.NewDNSProviderConfig(cfg)
}

// createDNSLAProvider 创建 DNSLA provider，用于 ACME DNS 验证
func createDNSLAProvider(c dnsprovider.DNSLAConfig) (challenge.Provider, error) {
	cfg := dnsla.NewDefaultConfig()
	cfg.HTTPClient = httplog.WrapClient(cfg.HTTPClient)
	cfg.APIID = c.APIID
	cfg.APISecret = c.APISecret
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return dnsla.NewDNSProviderConfig(cfg)
}

// createDNS51Provider 创建 51DNS provider，用于 ACME DNS 验证
func createDNS51Provider(c dnsprovider.DNS51Config) (challenge.Provider, error) {
	cfg := dns51.NewDefaultConfig()
	cfg.HTTPClient = httplog.WrapClient(cfg.HTTPClient)
	cfg.APIKey = c.APIKey
	cfg.APISecret = c.APISecret
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return dns51.NewDNSProviderConfig(cfg)
}

// createXinnetProvider 创建新网 DNS provider，用于 ACME DNS 验证
func createXinnetProvider(c dnsprovider.XinnetConfig) (challenge.Provider, error) {
	cfg := xinnet.NewDefaultConfig()
	cfg.HTTPClient = httplog.WrapClient(cfg.HTTPClient)
	cfg.Secret = c.Secret
	cfg.AgentID = c.AgentID
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return xinnet.NewDNSProviderConfig(cfg)
}

// createPorkbunProvider 创建 Porkbun DNS provider，用于 ACME DNS 验证
func createPorkbunProvider(c dnsprovider.PorkbunConfig) (challenge.Provider, error) {
	cfg := porkbun.NewDefaultConfig()
	cfg.HTTPClient = httplog.WrapClient(cfg.HTTPClient)
	cfg.APIKey = c.APIKey
	cfg.SecretAPIKey = c.SecretAPIKey
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return porkbun.NewDNSProviderConfig(cfg)
}

// createNamecheapProvider 创建 Namecheap DNS provider，用于 ACME DNS 验证
func createNamecheapProvider(c dnsprovider.NamecheapConfig) (challenge.Provider, error) {
	cfg := namecheap.NewDefaultConfig()
	cfg.HTTPClient = httplog.WrapClient(cfg.HTTPClient)
	cfg.APIUser = c.APIUser
	cfg.APIKey = c.APIKey
	cfg.ClientIP = c.ClientIP
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return namecheap.NewDNSProviderConfig(cfg)
}

// createGoDaddyProvider 创建 GoDaddy DNS provider，用于 ACME DNS 验证
func createGoDaddyProvider(c dnsprovider.GoDaddyConfig) (challenge.Provider, error) {
	cfg := godaddy.NewDefaultConfig()
	cfg.HTTPClient = httplog.WrapClient(cfg.HTTPClient)
	cfg.APIKey = c.APIKey
	cfg.APISecret = c.APISecret
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return godaddy.NewDNSProviderConfig(cfg)
}

// createGandiV5Provider 创建 Gandi v5 DNS provider，用于 ACME DNS 验证
func createGandiV5Provider(c dnsprovider.GandiV5Config) (challenge.Provider, error) {
	cfg := gandiv5.NewDefaultConfig()
	cfg.HTTPClient = httplog.WrapClient(cfg.HTTPClient)
	cfg.PersonalAccessToken = c.PersonalAccessToken
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return gandiv5.NewDNSProviderConfig(cfg)
}

// createDynadotProvider 创建 Dynadot DNS provider，用于 ACME DNS 验证
func createDynadotProvider(c dnsprovider.DynadotConfig) (challenge.Provider, error) {
	cfg := dynadot.NewDefaultConfig()
	cfg.HTTPClient = httplog.WrapClient(cfg.HTTPClient)
	cfg.APIKey = c.APIKey
	cfg.APISecret = c.APISecret
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return dynadot.NewDNSProviderConfig(cfg)
}

// createAzureDNSProvider 创建 Azure DNS provider，用于 ACME DNS 验证
func createAzureDNSProvider(c dnsprovider.AzureConfig) (challenge.Provider, error) {
	cfg := azuredns.NewDefaultConfig()
	cfg.HTTPClient = httplog.WrapClient(cfg.HTTPClient)
	cfg.SubscriptionID = c.SubscriptionID
	cfg.ResourceGroup = c.ResourceGroup
	cfg.ClientID = c.ClientID
	cfg.ClientSecret = c.ClientSecret
	cfg.TenantID = c.TenantID
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return azuredns.NewDNSProviderConfig(cfg)
}

// createDigitalOceanProvider 创建 DigitalOcean DNS provider，用于 ACME DNS 验证
func createDigitalOceanProvider(c dnsprovider.DigitalOceanConfig) (challenge.Provider, error) {
	cfg := digitalocean.NewDefaultConfig()
	cfg.HTTPClient = httplog.WrapClient(cfg.HTTPClient)
	cfg.AuthToken = c.AuthToken
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return digitalocean.NewDNSProviderConfig(cfg)
}

// createVultrProvider 创建 Vultr DNS provider，用于 ACME DNS 验证
func createVultrProvider(c dnsprovider.VultrConfig) (challenge.Provider, error) {
	cfg := vultr.NewDefaultConfig()
	cfg.HTTPClient = httplog.WrapClient(cfg.HTTPClient)
	cfg.APIKey = c.APIKey
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return vultr.NewDNSProviderConfig(cfg)
}

// createHetznerProvider 创建 Hetzner DNS provider，用于 ACME DNS 验证
func createHetznerProvider(c dnsprovider.HetznerConfig) (challenge.Provider, error) {
	cfg := hetzner.NewDefaultConfig()
	cfg.HTTPClient = httplog.WrapClient(cfg.HTTPClient)
	cfg.APIToken = c.APIToken
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return hetzner.NewDNSProviderConfig(cfg)
}

// createLinodeProvider 创建 Linode( Akamai ) DNS provider，用于 ACME DNS 验证
func createLinodeProvider(c dnsprovider.LinodeConfig) (challenge.Provider, error) {
	cfg := linode.NewDefaultConfig()
	cfg.Token = c.Token
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return linode.NewDNSProviderConfig(cfg)
}

// createOVHProvider 创建 OVH DNS provider，用于 ACME DNS 验证
func createOVHProvider(c dnsprovider.OVHConfig) (challenge.Provider, error) {
	cfg := ovh.NewDefaultConfig()
	cfg.HTTPClient = httplog.WrapClient(cfg.HTTPClient)
	cfg.ApplicationKey = c.ApplicationKey
	cfg.ApplicationSecret = c.ApplicationSecret
	cfg.ConsumerKey = c.ConsumerKey
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return ovh.NewDNSProviderConfig(cfg)
}

// createDNSimpleProvider 创建 DNSimple DNS provider，用于 ACME DNS 验证
func createDNSimpleProvider(c dnsprovider.DNSimpleConfig) (challenge.Provider, error) {
	cfg := dnsimple.NewDefaultConfig()
	cfg.AccessToken = c.AccessToken
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return dnsimple.NewDNSProviderConfig(cfg)
}

// createNS1Provider 创建 NS1 DNS provider，用于 ACME DNS 验证
func createNS1Provider(c dnsprovider.NS1Config) (challenge.Provider, error) {
	cfg := ns1.NewDefaultConfig()
	cfg.HTTPClient = httplog.WrapClient(cfg.HTTPClient)
	cfg.APIKey = c.APIKey
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return ns1.NewDNSProviderConfig(cfg)
}
