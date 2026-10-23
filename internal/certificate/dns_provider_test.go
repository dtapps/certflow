package certificate

import (
	"fmt"
	"strings"
	"testing"

	"cnb.cool/dtapp/certflow/internal/ent"
	"cnb.cool/dtapp/certflow/internal/ent/dnsprovider"
	"cnb.cool/dtapp/certflow/internal/ent/schema"
)

// providerEntity 造一个未落库的 ent 实体：createDNSProvider 只用 ProviderType 与 Config 两个字段。
func providerEntity(t string, cfg schema.DNSProviderConfig) *ent.DNSProvider {
	return &ent.DNSProvider{
		ProviderType: dnsprovider.ProviderType(t),
		Config:       cfg,
	}
}

// TestCreateDNSProvider_AllTypes 逐个厂商跑通「配置 → lego provider」。
//
// 覆盖的是 createDNSProvider 里那 32 个 case 的类型断言：cfg 解析出来的具体类型必须和
// case 分支对得上，否则会掉到 default 返回 unsupported —— 这类错误在编译期完全看不出来。
// 顺带验证每个构造器都把字段搬进了 lego 的 Config（lego 会做必填校验，搬漏了这里会报错）。
func TestCreateDNSProvider_AllTypes(t *testing.T) {
	// networkBound：构造器在创建时就会调用云厂商 API（换 token / 查 zone），
	// 没有真实凭证时必然失败，那不算代码缺陷，离线环境只能跳过。
	networkBound := map[string]bool{"huawei": true, "azuredns": true}

	cases := []struct {
		providerType string
		cfg          schema.DNSProviderConfig
	}{
		{"cloudflare", schema.DNSProviderConfig{APIToken: "t"}},
		{"aliyun", schema.DNSProviderConfig{AccessKeyID: "ak", AccessKeySecret: "sk", RegionID: "cn-hangzhou"}},
		{"huawei", schema.DNSProviderConfig{AccessKeyID: "ak", SecretAccessKey: "sk", Region: "cn-north-4"}},
		{"tencentcloud", schema.DNSProviderConfig{SecretID: "id", SecretKey: "key", Region: "ap-guangzhou"}},
		{"aws", schema.DNSProviderConfig{AccessKeyID: "ak", SecretAccessKey: "sk", Region: "us-east-1"}},
		{"baiducloud", schema.DNSProviderConfig{AccessKeyID: "ak", SecretAccessKey: "sk"}},
		{"jdcloud", schema.DNSProviderConfig{AccessKeyID: "ak", AccessKeySecret: "sk"}},
		{"volcengine", schema.DNSProviderConfig{AccessKey: "ak", SecretKey: "sk", Region: "cn-beijing"}},
		{"edgeone", schema.DNSProviderConfig{SecretID: "id", SecretKey: "key"}},
		{"aliesa", schema.DNSProviderConfig{APIKey: "k", SecretKey: "sk"}},
		{"ucloud", schema.DNSProviderConfig{PublicKey: "pk", PrivateKey: "prk"}},
		{"westcn", schema.DNSProviderConfig{Username: "u", Password: "p"}},
		{"com35", schema.DNSProviderConfig{Username: "u", Password: "p"}},
		{"rainyun", schema.DNSProviderConfig{APIKey: "k"}},
		{"todaynic", schema.DNSProviderConfig{AuthUserID: "u", APIKey: "k"}},
		{"dnsla", schema.DNSProviderConfig{APIID: "id", APISecret: "s"}},
		{"dns51", schema.DNSProviderConfig{APIKey: "k", APISecret: "s"}},
		{"xinnet", schema.DNSProviderConfig{Secret: "s", AgentID: "ag"}},
		{"porkbun", schema.DNSProviderConfig{APIKey: "k", SecretAPIKey: "sk"}},
		{"namecheap", schema.DNSProviderConfig{APIUser: "u", APIKey: "k", ClientIP: "1.2.3.4"}},
		{"godaddy", schema.DNSProviderConfig{APIKey: "k", APISecret: "s"}},
		{"gandiv5", schema.DNSProviderConfig{PersonalAccessToken: "pat"}},
		{"dynadot", schema.DNSProviderConfig{APIKey: "k", APISecret: "s"}},
		{"azuredns", schema.DNSProviderConfig{
			SubscriptionID: "sub", ResourceGroup: "rg",
			ClientID: "cid", ClientSecret: "cs", TenantID: "tid",
		}},
		{"digitalocean", schema.DNSProviderConfig{AuthToken: "at"}},
		{"vultr", schema.DNSProviderConfig{APIKey: "k"}},
		{"hetzner", schema.DNSProviderConfig{APIToken: "t"}},
		{"linode", schema.DNSProviderConfig{Token: "t"}},
		{"ovh", schema.DNSProviderConfig{ApplicationKey: "ak", ApplicationSecret: "as", ConsumerKey: "ck"}},
		{"dnsimple", schema.DNSProviderConfig{AccessToken: "at"}},
		{"ns1", schema.DNSProviderConfig{APIKey: "k"}},
	}

	for _, c := range cases {
		t.Run(c.providerType, func(t *testing.T) {
			if networkBound[c.providerType] {
				t.Skip("构造期需联网校验凭证，离线环境跳过")
			}
			p, err := createDNSProvider(providerEntity(c.providerType, c.cfg))
			if err != nil {
				t.Fatalf("createDNSProvider(%s) err: %v", c.providerType, err)
			}
			if p == nil {
				t.Fatalf("createDNSProvider(%s) 返回 nil provider", c.providerType)
			}
			// 构造出来的必须是具体实现（不是某个零值占位）
			if got := fmt.Sprintf("%T", p); got == "" || got == "<nil>" {
				t.Fatalf("createDNSProvider(%s) 返回了空类型", c.providerType)
			}
		})
	}
}

// TestCreateDNSProvider_EmptyConfig 空配置必须在进入 Parse 之前就被拦住，
// 否则各厂商构造器会抛出五花八门的 500，前端不知道该填什么。
func TestCreateDNSProvider_EmptyConfig(t *testing.T) {
	_, err := createDNSProvider(providerEntity("cloudflare", schema.DNSProviderConfig{}))
	if err == nil {
		t.Fatal("空配置应返回错误")
	}
	if strings.Contains(err.Error(), "<no value>") {
		t.Fatalf("错误信息残留模板占位符（i18n 文案参数缺失）: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "配置") && !strings.Contains(err.Error(), "config") {
		t.Fatalf("错误信息应提示配置为空, 实际 %q", err.Error())
	}
}

// TestCreateDNSProvider_UnknownType 未知厂商：Parse 报 unsupported，且错误被包一层人话。
func TestCreateDNSProvider_UnknownType(t *testing.T) {
	_, err := createDNSProvider(providerEntity("not-a-provider", schema.DNSProviderConfig{APIKey: "k"}))
	if err == nil {
		t.Fatal("未知厂商应返回错误")
	}
	if strings.Contains(err.Error(), "<no value>") {
		t.Fatalf("错误信息残留模板占位符（i18n 文案参数缺失）: %q", err.Error())
	}
	// 厂商类型必须出现在错误里，否则用户不知道该改哪一条配置
	if !strings.Contains(err.Error(), "not-a-provider") {
		t.Fatalf("错误信息应带出厂商类型, 实际 %q", err.Error())
	}
}

// TestCreateDNSProvider_MissingRequiredFields 关键凭证缺失时，错误必须来自构造器而不是静默返回
// 一个「能建但不能用」的 provider —— 后者会让失败推迟到真正调 API 的那一刻，排查成本高得多。
func TestCreateDNSProvider_MissingRequiredFields(t *testing.T) {
	cases := []struct {
		name         string
		providerType string
		cfg          schema.DNSProviderConfig
	}{
		{"cloudflare 无 token 无 email/key", "cloudflare", schema.DNSProviderConfig{Region: "只有无关字段"}},
		{"aliyun 无 SK", "aliyun", schema.DNSProviderConfig{AccessKeyID: "ak"}},
		{"azuredns 缺 tenant", "azuredns", schema.DNSProviderConfig{SubscriptionID: "s", ResourceGroup: "rg", ClientID: "c", ClientSecret: "cs"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, err := createDNSProvider(providerEntity(c.providerType, c.cfg))
			if err == nil {
				t.Fatalf("应返回错误, 实际拿到 %T", p)
			}
		})
	}
}
