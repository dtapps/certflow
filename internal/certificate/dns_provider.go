package certificate

import (
	"encoding/json"
	"fmt"
	"time"

	"cnb.cool/dtapp/certflow/ent"
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

// createDNSProvider 根据提供商类型和配置创建 lego DNS provider
func createDNSProvider(provider *ent.DNSProvider) (challenge.Provider, error) {
	if len(provider.Config) == 0 {
		return nil, fmt.Errorf("%s", i18n.T("error.dns_provider_config_empty"))
	}

	var configMap map[string]string
	if err := json.Unmarshal(provider.Config, &configMap); err != nil {
		return nil, fmt.Errorf("%s", i18n.T("error.dns_provider_config_parse_failed", "Error", err))
	}

	switch provider.ProviderType {
	case "cloudflare":
		return createCloudflareProvider(configMap)
	case "aliyun":
		return createAliyunProvider(configMap)
	case "huawei":
		return createHuaweiProvider(configMap)
	case "tencentcloud":
		return createTencentCloudProvider(configMap)
	case "aws":
		return createRoute53Provider(configMap)
	case "googlecloud":
		return createGoogleCloudProvider(configMap)
	case "baiducloud":
		return createBaiduCloudProvider(configMap)
	case "jdcloud":
		return createJDCloudProvider(configMap)
	case "volcengine":
		return createVolcengineProvider(configMap)
	case "edgeone":
		return createEdgeOneProvider(configMap)
	case "aliesa":
		return createAliesaProvider(configMap)
	case "ucloud":
		return createUCloudProvider(configMap)
	case "westcn":
		return createWestCNProvider(configMap)
	case "com35":
		return createCom35Provider(configMap)
	case "rainyun":
		return createRainYunProvider(configMap)
	case "todaynic":
		return createTodayNICProvider(configMap)
	case "dnsla":
		return createDNSLAProvider(configMap)
	case "dns51":
		return createDNS51Provider(configMap)
	case "xinnet":
		return createXinnetProvider(configMap)
	case "porkbun":
		return createPorkbunProvider(configMap)
	case "namecheap":
		return createNamecheapProvider(configMap)
	case "godaddy":
		return createGoDaddyProvider(configMap)
	case "gandiv5":
		return createGandiV5Provider(configMap)
	case "dynadot":
		return createDynadotProvider(configMap)
	case "azuredns":
		return createAzureDNSProvider(configMap)
	case "digitalocean":
		return createDigitalOceanProvider(configMap)
	case "vultr":
		return createVultrProvider(configMap)
	case "hetzner":
		return createHetznerProvider(configMap)
	case "linode":
		return createLinodeProvider(configMap)
	case "ovh":
		return createOVHProvider(configMap)
	case "dnsimple":
		return createDNSimpleProvider(configMap)
	case "ns1":
		return createNS1Provider(configMap)
	default:
		return nil, fmt.Errorf("%s", i18n.T("error.dns_provider_unsupported", "Type", provider.ProviderType))
	}
}

// createCloudflareProvider 创建 Cloudflare DNS provider，用于 ACME DNS 验证
func createCloudflareProvider(configMap map[string]string) (challenge.Provider, error) {
	cfg := cloudflare.NewDefaultConfig()
	cfg.HTTPClient = httplog.WrapClient(cfg.HTTPClient)
	cfg.AuthEmail = configMap["email"]
	cfg.AuthKey = configMap["api_key"]
	cfg.AuthToken = configMap["api_token"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return cloudflare.NewDNSProviderConfig(cfg)
}

// createAliyunProvider 创建阿里云 DNS provider，用于 ACME DNS 验证
func createAliyunProvider(configMap map[string]string) (challenge.Provider, error) {
	cfg := alidns.NewDefaultConfig()
	cfg.APIKey = configMap["access_key_id"]
	cfg.SecretKey = configMap["access_key_secret"]
	cfg.RegionID = configMap["region_id"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return alidns.NewDNSProviderConfig(cfg)
}

// createHuaweiProvider 创建华为云 DNS provider，用于 ACME DNS 验证
func createHuaweiProvider(configMap map[string]string) (challenge.Provider, error) {
	cfg := huaweicloud.NewDefaultConfig()
	cfg.AccessKeyID = configMap["access_key_id"]
	cfg.SecretAccessKey = configMap["secret_access_key"]
	cfg.Region = configMap["region"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return huaweicloud.NewDNSProviderConfig(cfg)
}

// createTencentCloudProvider 创建腾讯云 DNS provider，用于 ACME DNS 验证
func createTencentCloudProvider(configMap map[string]string) (challenge.Provider, error) {
	cfg := tencentcloud.NewDefaultConfig()
	cfg.SecretID = configMap["secret_id"]
	cfg.SecretKey = configMap["secret_key"]
	cfg.Region = configMap["region"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return tencentcloud.NewDNSProviderConfig(cfg)
}

// createRoute53Provider 创建 AWS Route53 DNS provider，用于 ACME DNS 验证
func createRoute53Provider(configMap map[string]string) (challenge.Provider, error) {
	cfg := route53.NewDefaultConfig()
	cfg.AccessKeyID = configMap["access_key_id"]
	cfg.SecretAccessKey = configMap["secret_access_key"]
	cfg.Region = configMap["region"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return route53.NewDNSProviderConfig(cfg)
}

// createGoogleCloudProvider 创建 Google Cloud DNS provider，用于 ACME DNS 验证
func createGoogleCloudProvider(configMap map[string]string) (challenge.Provider, error) {
	cfg := clouddns.NewDefaultConfig()
	cfg.HTTPClient = httplog.WrapClient(cfg.HTTPClient)
	cfg.ClientID = configMap["client_id"]
	cfg.Email = configMap["email"]
	cfg.Password = configMap["password"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return clouddns.NewDNSProviderConfig(cfg)
}

// createBaiduCloudProvider 创建百度智能云 DNS provider，用于 ACME DNS 验证
func createBaiduCloudProvider(configMap map[string]string) (challenge.Provider, error) {
	cfg := baiducloud.NewDefaultConfig()
	cfg.AccessKeyID = configMap["access_key_id"]
	cfg.SecretAccessKey = configMap["secret_access_key"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return baiducloud.NewDNSProviderConfig(cfg)
}

// createJDCloudProvider 创建京东云 DNS provider，用于 ACME DNS 验证
func createJDCloudProvider(configMap map[string]string) (challenge.Provider, error) {
	cfg := jdcloud.NewDefaultConfig()
	cfg.AccessKeyID = configMap["access_key_id"]
	cfg.AccessKeySecret = configMap["access_key_secret"]
	cfg.RegionID = configMap["region_id"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return jdcloud.NewDNSProviderConfig(cfg)
}

// createVolcengineProvider 创建火山引擎 DNS provider，用于 ACME DNS 验证
func createVolcengineProvider(configMap map[string]string) (challenge.Provider, error) {
	cfg := volcengine.NewDefaultConfig()
	cfg.AccessKey = configMap["access_key"]
	cfg.SecretKey = configMap["secret_key"]
	cfg.Region = configMap["region"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return volcengine.NewDNSProviderConfig(cfg)
}

// createEdgeOneProvider 创建腾讯云 EdgeOne DNS provider，用于 ACME DNS 验证
func createEdgeOneProvider(configMap map[string]string) (challenge.Provider, error) {
	cfg := edgeone.NewDefaultConfig()
	cfg.SecretID = configMap["secret_id"]
	cfg.SecretKey = configMap["secret_key"]
	cfg.Region = configMap["region"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return edgeone.NewDNSProviderConfig(cfg)
}

// createAliesaProvider 创建阿里云 ESA(边缘安全加速) DNS provider，用于 ACME DNS 验证
func createAliesaProvider(configMap map[string]string) (challenge.Provider, error) {
	cfg := aliesa.NewDefaultConfig()
	cfg.APIKey = configMap["api_key"]
	cfg.SecretKey = configMap["secret_key"]
	cfg.RegionID = configMap["region_id"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return aliesa.NewDNSProviderConfig(cfg)
}

// createUCloudProvider 创建 UCloud DNS provider，用于 ACME DNS 验证
func createUCloudProvider(configMap map[string]string) (challenge.Provider, error) {
	cfg := ucloud.NewDefaultConfig()
	cfg.PublicKey = configMap["public_key"]
	cfg.PrivateKey = configMap["private_key"]
	cfg.Region = configMap["region"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return ucloud.NewDNSProviderConfig(cfg)
}

// createWestCNProvider 创建西部数码 DNS provider，用于 ACME DNS 验证
func createWestCNProvider(configMap map[string]string) (challenge.Provider, error) {
	cfg := westcn.NewDefaultConfig()
	cfg.Username = configMap["username"]
	cfg.Password = configMap["password"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return westcn.NewDNSProviderConfig(cfg)
}

// createCom35Provider 创建 35 互联 DNS provider，用于 ACME DNS 验证
func createCom35Provider(configMap map[string]string) (challenge.Provider, error) {
	cfg := com35.NewDefaultConfig()
	cfg.Username = configMap["username"]
	cfg.Password = configMap["password"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return com35.NewDNSProviderConfig(cfg)
}

// createRainYunProvider 创建雨云 DNS provider，用于 ACME DNS 验证
func createRainYunProvider(configMap map[string]string) (challenge.Provider, error) {
	cfg := rainyun.NewDefaultConfig()
	cfg.HTTPClient = httplog.WrapClient(cfg.HTTPClient)
	cfg.APIKey = configMap["api_key"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return rainyun.NewDNSProviderConfig(cfg)
}

// createTodayNICProvider 创建 TodayDNS(时代互联) DNS provider，用于 ACME DNS 验证
func createTodayNICProvider(configMap map[string]string) (challenge.Provider, error) {
	cfg := todaynic.NewDefaultConfig()
	cfg.HTTPClient = httplog.WrapClient(cfg.HTTPClient)
	cfg.AuthUserID = configMap["auth_user_id"]
	cfg.APIKey = configMap["api_key"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return todaynic.NewDNSProviderConfig(cfg)
}

// createDNSLAProvider 创建 DNSLA provider，用于 ACME DNS 验证
func createDNSLAProvider(configMap map[string]string) (challenge.Provider, error) {
	cfg := dnsla.NewDefaultConfig()
	cfg.HTTPClient = httplog.WrapClient(cfg.HTTPClient)
	cfg.APIID = configMap["api_id"]
	cfg.APISecret = configMap["api_secret"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return dnsla.NewDNSProviderConfig(cfg)
}

// createDNS51Provider 创建 51DNS provider，用于 ACME DNS 验证
func createDNS51Provider(configMap map[string]string) (challenge.Provider, error) {
	cfg := dns51.NewDefaultConfig()
	cfg.HTTPClient = httplog.WrapClient(cfg.HTTPClient)
	cfg.APIKey = configMap["api_key"]
	cfg.APISecret = configMap["api_secret"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return dns51.NewDNSProviderConfig(cfg)
}

// createXinnetProvider 创建新网 DNS provider，用于 ACME DNS 验证
func createXinnetProvider(configMap map[string]string) (challenge.Provider, error) {
	cfg := xinnet.NewDefaultConfig()
	cfg.HTTPClient = httplog.WrapClient(cfg.HTTPClient)
	cfg.Secret = configMap["secret"]
	cfg.AgentID = configMap["agent_id"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return xinnet.NewDNSProviderConfig(cfg)
}

// createPorkbunProvider 创建 Porkbun DNS provider，用于 ACME DNS 验证
func createPorkbunProvider(configMap map[string]string) (challenge.Provider, error) {
	cfg := porkbun.NewDefaultConfig()
	cfg.HTTPClient = httplog.WrapClient(cfg.HTTPClient)
	cfg.APIKey = configMap["api_key"]
	cfg.SecretAPIKey = configMap["secret_api_key"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return porkbun.NewDNSProviderConfig(cfg)
}

// createNamecheapProvider 创建 Namecheap DNS provider，用于 ACME DNS 验证
func createNamecheapProvider(configMap map[string]string) (challenge.Provider, error) {
	cfg := namecheap.NewDefaultConfig()
	cfg.HTTPClient = httplog.WrapClient(cfg.HTTPClient)
	cfg.APIUser = configMap["api_user"]
	cfg.APIKey = configMap["api_key"]
	cfg.ClientIP = configMap["client_ip"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return namecheap.NewDNSProviderConfig(cfg)
}

// createGoDaddyProvider 创建 GoDaddy DNS provider，用于 ACME DNS 验证
func createGoDaddyProvider(configMap map[string]string) (challenge.Provider, error) {
	cfg := godaddy.NewDefaultConfig()
	cfg.HTTPClient = httplog.WrapClient(cfg.HTTPClient)
	cfg.APIKey = configMap["api_key"]
	cfg.APISecret = configMap["api_secret"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return godaddy.NewDNSProviderConfig(cfg)
}

// createGandiV5Provider 创建 Gandi v5 DNS provider，用于 ACME DNS 验证
func createGandiV5Provider(configMap map[string]string) (challenge.Provider, error) {
	cfg := gandiv5.NewDefaultConfig()
	cfg.HTTPClient = httplog.WrapClient(cfg.HTTPClient)
	cfg.PersonalAccessToken = configMap["personal_access_token"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return gandiv5.NewDNSProviderConfig(cfg)
}

// createDynadotProvider 创建 Dynadot DNS provider，用于 ACME DNS 验证
func createDynadotProvider(configMap map[string]string) (challenge.Provider, error) {
	cfg := dynadot.NewDefaultConfig()
	cfg.HTTPClient = httplog.WrapClient(cfg.HTTPClient)
	cfg.APIKey = configMap["api_key"]
	cfg.APISecret = configMap["api_secret"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return dynadot.NewDNSProviderConfig(cfg)
}

// createAzureDNSProvider 创建 Azure DNS provider，用于 ACME DNS 验证
func createAzureDNSProvider(configMap map[string]string) (challenge.Provider, error) {
	cfg := azuredns.NewDefaultConfig()
	cfg.HTTPClient = httplog.WrapClient(cfg.HTTPClient)
	cfg.SubscriptionID = configMap["subscription_id"]
	cfg.ResourceGroup = configMap["resource_group"]
	cfg.ClientID = configMap["client_id"]
	cfg.ClientSecret = configMap["client_secret"]
	cfg.TenantID = configMap["tenant_id"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return azuredns.NewDNSProviderConfig(cfg)
}

// createDigitalOceanProvider 创建 DigitalOcean DNS provider，用于 ACME DNS 验证
func createDigitalOceanProvider(configMap map[string]string) (challenge.Provider, error) {
	cfg := digitalocean.NewDefaultConfig()
	cfg.HTTPClient = httplog.WrapClient(cfg.HTTPClient)
	cfg.AuthToken = configMap["auth_token"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return digitalocean.NewDNSProviderConfig(cfg)
}

// createVultrProvider 创建 Vultr DNS provider，用于 ACME DNS 验证
func createVultrProvider(configMap map[string]string) (challenge.Provider, error) {
	cfg := vultr.NewDefaultConfig()
	cfg.HTTPClient = httplog.WrapClient(cfg.HTTPClient)
	cfg.APIKey = configMap["api_key"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return vultr.NewDNSProviderConfig(cfg)
}

// createHetznerProvider 创建 Hetzner DNS provider，用于 ACME DNS 验证
func createHetznerProvider(configMap map[string]string) (challenge.Provider, error) {
	cfg := hetzner.NewDefaultConfig()
	cfg.HTTPClient = httplog.WrapClient(cfg.HTTPClient)
	cfg.APIToken = configMap["api_token"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return hetzner.NewDNSProviderConfig(cfg)
}

// createLinodeProvider 创建 Linode( Akamai ) DNS provider，用于 ACME DNS 验证
func createLinodeProvider(configMap map[string]string) (challenge.Provider, error) {
	cfg := linode.NewDefaultConfig()
	cfg.Token = configMap["token"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return linode.NewDNSProviderConfig(cfg)
}

// createOVHProvider 创建 OVH DNS provider，用于 ACME DNS 验证
func createOVHProvider(configMap map[string]string) (challenge.Provider, error) {
	cfg := ovh.NewDefaultConfig()
	cfg.HTTPClient = httplog.WrapClient(cfg.HTTPClient)
	cfg.ApplicationKey = configMap["application_key"]
	cfg.ApplicationSecret = configMap["application_secret"]
	cfg.ConsumerKey = configMap["consumer_key"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return ovh.NewDNSProviderConfig(cfg)
}

// createDNSimpleProvider 创建 DNSimple DNS provider，用于 ACME DNS 验证
func createDNSimpleProvider(configMap map[string]string) (challenge.Provider, error) {
	cfg := dnsimple.NewDefaultConfig()
	cfg.AccessToken = configMap["access_token"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return dnsimple.NewDNSProviderConfig(cfg)
}

// createNS1Provider 创建 NS1 DNS provider，用于 ACME DNS 验证
func createNS1Provider(configMap map[string]string) (challenge.Provider, error) {
	cfg := ns1.NewDefaultConfig()
	cfg.HTTPClient = httplog.WrapClient(cfg.HTTPClient)
	cfg.APIKey = configMap["api_key"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return ns1.NewDNSProviderConfig(cfg)
}
