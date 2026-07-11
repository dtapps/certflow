package certificate

import (
	"encoding/json"
	"fmt"
	"time"

	"cnb.cool/dtapp/certflow/ent"
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

func createCloudflareProvider(configMap map[string]string) (challenge.Provider, error) {
	cfg := cloudflare.NewDefaultConfig()
	cfg.AuthEmail = configMap["email"]
	cfg.AuthKey = configMap["api_key"]
	cfg.AuthToken = configMap["api_token"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return cloudflare.NewDNSProviderConfig(cfg)
}

func createAliyunProvider(configMap map[string]string) (challenge.Provider, error) {
	cfg := alidns.NewDefaultConfig()
	cfg.APIKey = configMap["access_key_id"]
	cfg.SecretKey = configMap["access_key_secret"]
	cfg.RegionID = configMap["region_id"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return alidns.NewDNSProviderConfig(cfg)
}

func createHuaweiProvider(configMap map[string]string) (challenge.Provider, error) {
	cfg := huaweicloud.NewDefaultConfig()
	cfg.AccessKeyID = configMap["access_key_id"]
	cfg.SecretAccessKey = configMap["secret_access_key"]
	cfg.Region = configMap["region"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return huaweicloud.NewDNSProviderConfig(cfg)
}

func createTencentCloudProvider(configMap map[string]string) (challenge.Provider, error) {
	cfg := tencentcloud.NewDefaultConfig()
	cfg.SecretID = configMap["secret_id"]
	cfg.SecretKey = configMap["secret_key"]
	cfg.Region = configMap["region"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return tencentcloud.NewDNSProviderConfig(cfg)
}

func createRoute53Provider(configMap map[string]string) (challenge.Provider, error) {
	cfg := route53.NewDefaultConfig()
	cfg.AccessKeyID = configMap["access_key_id"]
	cfg.SecretAccessKey = configMap["secret_access_key"]
	cfg.Region = configMap["region"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return route53.NewDNSProviderConfig(cfg)
}

func createGoogleCloudProvider(configMap map[string]string) (challenge.Provider, error) {
	cfg := clouddns.NewDefaultConfig()
	cfg.ClientID = configMap["client_id"]
	cfg.Email = configMap["email"]
	cfg.Password = configMap["password"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return clouddns.NewDNSProviderConfig(cfg)
}

func createBaiduCloudProvider(configMap map[string]string) (challenge.Provider, error) {
	cfg := baiducloud.NewDefaultConfig()
	cfg.AccessKeyID = configMap["access_key_id"]
	cfg.SecretAccessKey = configMap["secret_access_key"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return baiducloud.NewDNSProviderConfig(cfg)
}

func createJDCloudProvider(configMap map[string]string) (challenge.Provider, error) {
	cfg := jdcloud.NewDefaultConfig()
	cfg.AccessKeyID = configMap["access_key_id"]
	cfg.AccessKeySecret = configMap["access_key_secret"]
	cfg.RegionID = configMap["region_id"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return jdcloud.NewDNSProviderConfig(cfg)
}

func createVolcengineProvider(configMap map[string]string) (challenge.Provider, error) {
	cfg := volcengine.NewDefaultConfig()
	cfg.AccessKey = configMap["access_key"]
	cfg.SecretKey = configMap["secret_key"]
	cfg.Region = configMap["region"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return volcengine.NewDNSProviderConfig(cfg)
}

func createEdgeOneProvider(configMap map[string]string) (challenge.Provider, error) {
	cfg := edgeone.NewDefaultConfig()
	cfg.SecretID = configMap["secret_id"]
	cfg.SecretKey = configMap["secret_key"]
	cfg.Region = configMap["region"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return edgeone.NewDNSProviderConfig(cfg)
}

func createAliesaProvider(configMap map[string]string) (challenge.Provider, error) {
	cfg := aliesa.NewDefaultConfig()
	cfg.APIKey = configMap["api_key"]
	cfg.SecretKey = configMap["secret_key"]
	cfg.RegionID = configMap["region_id"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return aliesa.NewDNSProviderConfig(cfg)
}

func createUCloudProvider(configMap map[string]string) (challenge.Provider, error) {
	cfg := ucloud.NewDefaultConfig()
	cfg.PublicKey = configMap["public_key"]
	cfg.PrivateKey = configMap["private_key"]
	cfg.Region = configMap["region"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return ucloud.NewDNSProviderConfig(cfg)
}

func createWestCNProvider(configMap map[string]string) (challenge.Provider, error) {
	cfg := westcn.NewDefaultConfig()
	cfg.Username = configMap["username"]
	cfg.Password = configMap["password"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return westcn.NewDNSProviderConfig(cfg)
}

func createCom35Provider(configMap map[string]string) (challenge.Provider, error) {
	cfg := com35.NewDefaultConfig()
	cfg.Username = configMap["username"]
	cfg.Password = configMap["password"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return com35.NewDNSProviderConfig(cfg)
}

func createRainYunProvider(configMap map[string]string) (challenge.Provider, error) {
	cfg := rainyun.NewDefaultConfig()
	cfg.APIKey = configMap["api_key"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return rainyun.NewDNSProviderConfig(cfg)
}

func createTodayNICProvider(configMap map[string]string) (challenge.Provider, error) {
	cfg := todaynic.NewDefaultConfig()
	cfg.AuthUserID = configMap["auth_user_id"]
	cfg.APIKey = configMap["api_key"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return todaynic.NewDNSProviderConfig(cfg)
}

func createDNSLAProvider(configMap map[string]string) (challenge.Provider, error) {
	cfg := dnsla.NewDefaultConfig()
	cfg.APIID = configMap["api_id"]
	cfg.APISecret = configMap["api_secret"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return dnsla.NewDNSProviderConfig(cfg)
}

func createDNS51Provider(configMap map[string]string) (challenge.Provider, error) {
	cfg := dns51.NewDefaultConfig()
	cfg.APIKey = configMap["api_key"]
	cfg.APISecret = configMap["api_secret"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return dns51.NewDNSProviderConfig(cfg)
}

func createXinnetProvider(configMap map[string]string) (challenge.Provider, error) {
	cfg := xinnet.NewDefaultConfig()
	cfg.Secret = configMap["secret"]
	cfg.AgentID = configMap["agent_id"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return xinnet.NewDNSProviderConfig(cfg)
}

func createPorkbunProvider(configMap map[string]string) (challenge.Provider, error) {
	cfg := porkbun.NewDefaultConfig()
	cfg.APIKey = configMap["api_key"]
	cfg.SecretAPIKey = configMap["secret_api_key"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return porkbun.NewDNSProviderConfig(cfg)
}

func createNamecheapProvider(configMap map[string]string) (challenge.Provider, error) {
	cfg := namecheap.NewDefaultConfig()
	cfg.APIUser = configMap["api_user"]
	cfg.APIKey = configMap["api_key"]
	cfg.ClientIP = configMap["client_ip"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return namecheap.NewDNSProviderConfig(cfg)
}

func createGoDaddyProvider(configMap map[string]string) (challenge.Provider, error) {
	cfg := godaddy.NewDefaultConfig()
	cfg.APIKey = configMap["api_key"]
	cfg.APISecret = configMap["api_secret"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return godaddy.NewDNSProviderConfig(cfg)
}

func createGandiV5Provider(configMap map[string]string) (challenge.Provider, error) {
	cfg := gandiv5.NewDefaultConfig()
	cfg.PersonalAccessToken = configMap["personal_access_token"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return gandiv5.NewDNSProviderConfig(cfg)
}

func createDynadotProvider(configMap map[string]string) (challenge.Provider, error) {
	cfg := dynadot.NewDefaultConfig()
	cfg.APIKey = configMap["api_key"]
	cfg.APISecret = configMap["api_secret"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return dynadot.NewDNSProviderConfig(cfg)
}

func createAzureDNSProvider(configMap map[string]string) (challenge.Provider, error) {
	cfg := azuredns.NewDefaultConfig()
	cfg.SubscriptionID = configMap["subscription_id"]
	cfg.ResourceGroup = configMap["resource_group"]
	cfg.ClientID = configMap["client_id"]
	cfg.ClientSecret = configMap["client_secret"]
	cfg.TenantID = configMap["tenant_id"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return azuredns.NewDNSProviderConfig(cfg)
}

func createDigitalOceanProvider(configMap map[string]string) (challenge.Provider, error) {
	cfg := digitalocean.NewDefaultConfig()
	cfg.AuthToken = configMap["auth_token"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return digitalocean.NewDNSProviderConfig(cfg)
}

func createVultrProvider(configMap map[string]string) (challenge.Provider, error) {
	cfg := vultr.NewDefaultConfig()
	cfg.APIKey = configMap["api_key"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return vultr.NewDNSProviderConfig(cfg)
}

func createHetznerProvider(configMap map[string]string) (challenge.Provider, error) {
	cfg := hetzner.NewDefaultConfig()
	cfg.APIToken = configMap["api_token"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return hetzner.NewDNSProviderConfig(cfg)
}

func createLinodeProvider(configMap map[string]string) (challenge.Provider, error) {
	cfg := linode.NewDefaultConfig()
	cfg.Token = configMap["token"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return linode.NewDNSProviderConfig(cfg)
}

func createOVHProvider(configMap map[string]string) (challenge.Provider, error) {
	cfg := ovh.NewDefaultConfig()
	cfg.ApplicationKey = configMap["application_key"]
	cfg.ApplicationSecret = configMap["application_secret"]
	cfg.ConsumerKey = configMap["consumer_key"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return ovh.NewDNSProviderConfig(cfg)
}

func createDNSimpleProvider(configMap map[string]string) (challenge.Provider, error) {
	cfg := dnsimple.NewDefaultConfig()
	cfg.AccessToken = configMap["access_token"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return dnsimple.NewDNSProviderConfig(cfg)
}

func createNS1Provider(configMap map[string]string) (challenge.Provider, error) {
	cfg := ns1.NewDefaultConfig()
	cfg.APIKey = configMap["api_key"]
	cfg.PropagationTimeout = 120 * time.Second
	cfg.PollingInterval = 2 * time.Second
	return ns1.NewDNSProviderConfig(cfg)
}
