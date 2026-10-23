package dnsprovider

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"cnb.cool/dtapp/certflow/internal/cloudcred"
)

// TestParse_AllProviderTypes 逐个钉住「厂商标识 → 强类型结构体」的映射。
//
// 这张表是 createDNSProvider（internal/certificate/dns_provider.go）里那串类型断言的唯一来源：
// 一旦某个 case 写错（比如把 "azuredns" 映射成 VultrConfig），类型断言就会落到 default 分支，
// 表现为「配置明明填对了却报 unsupported」，线上排查非常耗时间。
func TestParse_AllProviderTypes(t *testing.T) {
	raw := DNSProviderConfig{
		AccessKeyID:         "ak",
		AccessKeySecret:     "aks",
		SecretAccessKey:     "sak",
		Region:              "r",
		RegionID:            "rid",
		APIToken:            "apitoken",
		APIKey:              "apikey",
		APISecret:           "apisecret",
		SecretID:            "sid",
		SecretKey:           "skey",
		AccessKey:           "ak2",
		PublicKey:           "pk",
		PrivateKey:          "prk",
		Email:               "e@example.com",
		Password:            "pw",
		Username:            "user",
		ClientID:            "cid",
		ClientSecret:        "cs",
		TenantID:            "tid",
		ResourceGroup:       "rg",
		SubscriptionID:      "sub",
		AuthToken:           "at",
		AccessToken:         "atok",
		SecretAPIKey:        "sak2",
		Token:               "tk",
		PersonalAccessToken: "pat",
		ApplicationKey:      "appk",
		ApplicationSecret:   "apps",
		ConsumerKey:         "ck",
		APIUser:             "au",
		ClientIP:            "1.2.3.4",
		AgentID:             "ag",
		AuthUserID:          "auid",
		APIID:               "apid",
		Secret:              "sec",
	}

	cases := []struct {
		providerType string
		wantType     string
		// check 断言至少一个被正确反序列化的字段，证明确实走了 ParseConfig 而不是只造了个零值
		check func(t *testing.T, v any)
	}{
		{"cloudflare", "dnsprovider.CloudflareConfig", func(t *testing.T, v any) {
			if c := v.(CloudflareConfig); c.APIToken != "apitoken" || c.APIKey != "apikey" || c.Email != "e@example.com" {
				t.Fatalf("字段未正确映射: %+v", c)
			}
		}},
		{"aliyun", "dnsprovider.AliyunConfig", func(t *testing.T, v any) {
			if c := v.(AliyunConfig); c.AccessKeyID != "ak" || c.RegionID != "rid" {
				t.Fatalf("字段未正确映射: %+v", c)
			}
		}},
		{"huawei", "dnsprovider.HuaweiConfig", func(t *testing.T, v any) {
			if c := v.(HuaweiConfig); c.SecretAccessKey != "sak" || c.Region != "r" {
				t.Fatalf("字段未正确映射: %+v", c)
			}
		}},
		{"tencentcloud", "dnsprovider.TencentConfig", func(t *testing.T, v any) {
			if c := v.(TencentConfig); c.SecretID != "sid" || c.SecretKey != "skey" {
				t.Fatalf("字段未正确映射: %+v", c)
			}
		}},
		{"aws", "dnsprovider.Route53Config", func(t *testing.T, v any) {
			if c := v.(Route53Config); c.AccessKeyID != "ak" || c.SecretAccessKey != "sak" {
				t.Fatalf("字段未正确映射: %+v", c)
			}
		}},
		{"googlecloud", "dnsprovider.GoogleCloudConfig", func(t *testing.T, v any) {
			if c := v.(GoogleCloudConfig); c.ClientID != "cid" || c.Password != "pw" {
				t.Fatalf("字段未正确映射: %+v", c)
			}
		}},
		{"baiducloud", "dnsprovider.BaiduConfig", func(t *testing.T, v any) {
			if c := v.(BaiduConfig); c.SecretAccessKey != "sak" || c.Region != "r" {
				t.Fatalf("字段未正确映射: %+v", c)
			}
		}},
		{"jdcloud", "dnsprovider.JdcloudConfig", func(t *testing.T, v any) {
			if c := v.(JdcloudConfig); c.RegionID != "rid" {
				t.Fatalf("字段未正确映射: %+v", c)
			}
		}},
		{"volcengine", "dnsprovider.VolcengineConfig", func(t *testing.T, v any) {
			if c := v.(VolcengineConfig); c.AccessKey != "ak2" || c.SecretKey != "skey" {
				t.Fatalf("字段未正确映射: %+v", c)
			}
		}},
		{"edgeone", "dnsprovider.EdgeOneConfig", func(t *testing.T, v any) {
			if c := v.(EdgeOneConfig); c.SecretID != "sid" {
				t.Fatalf("字段未正确映射: %+v", c)
			}
		}},
		{"aliesa", "dnsprovider.AliEsaConfig", func(t *testing.T, v any) {
			if c := v.(AliEsaConfig); c.APIKey != "apikey" || c.RegionID != "rid" {
				t.Fatalf("字段未正确映射: %+v", c)
			}
		}},
		{"ucloud", "dnsprovider.UCloudConfig", func(t *testing.T, v any) {
			if c := v.(UCloudConfig); c.PublicKey != "pk" || c.PrivateKey != "prk" {
				t.Fatalf("字段未正确映射: %+v", c)
			}
		}},
		{"westcn", "dnsprovider.WestCNConfig", func(t *testing.T, v any) {
			if c := v.(WestCNConfig); c.Username != "user" || c.Password != "pw" {
				t.Fatalf("字段未正确映射: %+v", c)
			}
		}},
		{"com35", "dnsprovider.Com35Config", func(t *testing.T, v any) {
			if c := v.(Com35Config); c.Username != "user" {
				t.Fatalf("字段未正确映射: %+v", c)
			}
		}},
		{"rainyun", "dnsprovider.RainYunConfig", func(t *testing.T, v any) {
			if c := v.(RainYunConfig); c.APIKey != "apikey" {
				t.Fatalf("字段未正确映射: %+v", c)
			}
		}},
		{"todaynic", "dnsprovider.TodayNICConfig", func(t *testing.T, v any) {
			if c := v.(TodayNICConfig); c.AuthUserID != "auid" || c.APIKey != "apikey" {
				t.Fatalf("字段未正确映射: %+v", c)
			}
		}},
		{"dnsla", "dnsprovider.DNSLAConfig", func(t *testing.T, v any) {
			if c := v.(DNSLAConfig); c.APIID != "apid" || c.APISecret != "apisecret" {
				t.Fatalf("字段未正确映射: %+v", c)
			}
		}},
		{"dns51", "dnsprovider.DNS51Config", func(t *testing.T, v any) {
			if c := v.(DNS51Config); c.APIKey != "apikey" {
				t.Fatalf("字段未正确映射: %+v", c)
			}
		}},
		{"xinnet", "dnsprovider.XinnetConfig", func(t *testing.T, v any) {
			if c := v.(XinnetConfig); c.Secret != "sec" || c.AgentID != "ag" {
				t.Fatalf("字段未正确映射: %+v", c)
			}
		}},
		{"porkbun", "dnsprovider.PorkbunConfig", func(t *testing.T, v any) {
			if c := v.(PorkbunConfig); c.APIKey != "apikey" || c.SecretAPIKey != "sak2" {
				t.Fatalf("字段未正确映射: %+v", c)
			}
		}},
		{"namecheap", "dnsprovider.NamecheapConfig", func(t *testing.T, v any) {
			if c := v.(NamecheapConfig); c.APIUser != "au" || c.ClientIP != "1.2.3.4" {
				t.Fatalf("字段未正确映射: %+v", c)
			}
		}},
		{"godaddy", "dnsprovider.GoDaddyConfig", func(t *testing.T, v any) {
			if c := v.(GoDaddyConfig); c.APIKey != "apikey" || c.APISecret != "apisecret" {
				t.Fatalf("字段未正确映射: %+v", c)
			}
		}},
		{"gandiv5", "dnsprovider.GandiV5Config", func(t *testing.T, v any) {
			if c := v.(GandiV5Config); c.PersonalAccessToken != "pat" {
				t.Fatalf("字段未正确映射: %+v", c)
			}
		}},
		{"dynadot", "dnsprovider.DynadotConfig", func(t *testing.T, v any) {
			if c := v.(DynadotConfig); c.APISecret != "apisecret" {
				t.Fatalf("字段未正确映射: %+v", c)
			}
		}},
		{"azuredns", "dnsprovider.AzureConfig", func(t *testing.T, v any) {
			if c := v.(AzureConfig); c.SubscriptionID != "sub" || c.ResourceGroup != "rg" || c.TenantID != "tid" {
				t.Fatalf("字段未正确映射: %+v", c)
			}
		}},
		{"digitalocean", "dnsprovider.DigitalOceanConfig", func(t *testing.T, v any) {
			if c := v.(DigitalOceanConfig); c.AuthToken != "at" {
				t.Fatalf("字段未正确映射: %+v", c)
			}
		}},
		{"vultr", "dnsprovider.VultrConfig", func(t *testing.T, v any) {
			if c := v.(VultrConfig); c.APIKey != "apikey" {
				t.Fatalf("字段未正确映射: %+v", c)
			}
		}},
		{"hetzner", "dnsprovider.HetznerConfig", func(t *testing.T, v any) {
			if c := v.(HetznerConfig); c.APIToken != "apitoken" {
				t.Fatalf("字段未正确映射: %+v", c)
			}
		}},
		{"linode", "dnsprovider.LinodeConfig", func(t *testing.T, v any) {
			if c := v.(LinodeConfig); c.Token != "tk" {
				t.Fatalf("字段未正确映射: %+v", c)
			}
		}},
		{"ovh", "dnsprovider.OVHConfig", func(t *testing.T, v any) {
			if c := v.(OVHConfig); c.ApplicationKey != "appk" || c.ConsumerKey != "ck" {
				t.Fatalf("字段未正确映射: %+v", c)
			}
		}},
		{"dnsimple", "dnsprovider.DNSimpleConfig", func(t *testing.T, v any) {
			if c := v.(DNSimpleConfig); c.AccessToken != "atok" {
				t.Fatalf("字段未正确映射: %+v", c)
			}
		}},
		{"ns1", "dnsprovider.NS1Config", func(t *testing.T, v any) {
			if c := v.(NS1Config); c.APIKey != "apikey" {
				t.Fatalf("字段未正确映射: %+v", c)
			}
		}},
	}

	for _, c := range cases {
		t.Run(c.providerType, func(t *testing.T) {
			v, err := Parse(c.providerType, raw)
			if err != nil {
				t.Fatalf("Parse(%s) err: %v", c.providerType, err)
			}
			if got := fmt.Sprintf("%T", v); got != c.wantType {
				t.Fatalf("Parse(%s) 类型应为 %s, 实际 %s", c.providerType, c.wantType, got)
			}
			c.check(t, v)
		})
	}
}

func TestParse_Unsupported(t *testing.T) {
	v, err := Parse("no-such-provider", DNSProviderConfig{APIKey: "x"})
	if err == nil {
		t.Fatal("未知厂商应返回错误")
	}
	if v != nil {
		t.Fatalf("出错时应返回 nil, 实际 %T", v)
	}
	if got := err.Error(); got != "unsupported dns provider: no-such-provider" {
		t.Fatalf("错误信息不符: %q", got)
	}
}

// TestParseCredential 只支持 5 个 AK/SK 类厂商，其余必须报错（不能静默返回空凭证，
// 否则部署时会拿着空 AK 去调 API，报一个完全无关的鉴权错误）。
func TestParseCredential(t *testing.T) {
	cases := []struct {
		providerType string
		raw          DNSProviderConfig
		want         cloudcred.Credentials
	}{
		{"aliyun", DNSProviderConfig{AccessKeyID: "ak", AccessKeySecret: "sk", RegionID: "cn-hangzhou"},
			cloudcred.Credentials{AccessKeyID: "ak", AccessKeySecret: "sk", Region: "cn-hangzhou"}},
		{"huawei", DNSProviderConfig{AccessKeyID: "ak", SecretAccessKey: "sk", Region: "cn-north-4"},
			cloudcred.Credentials{AccessKeyID: "ak", AccessKeySecret: "sk", Region: "cn-north-4"}},
		{"tencentcloud", DNSProviderConfig{SecretID: "id", SecretKey: "key", Region: "ap-guangzhou"},
			cloudcred.Credentials{AccessKeyID: "id", AccessKeySecret: "key", Region: "ap-guangzhou"}},
		{"baiducloud", DNSProviderConfig{AccessKeyID: "ak", SecretAccessKey: "sk"},
			cloudcred.Credentials{AccessKeyID: "ak", AccessKeySecret: "sk"}},
		{"volcengine", DNSProviderConfig{AccessKey: "ak", SecretKey: "sk", Region: "cn-beijing"},
			cloudcred.Credentials{AccessKeyID: "ak", AccessKeySecret: "sk", Region: "cn-beijing"}},
	}

	for _, c := range cases {
		t.Run(c.providerType, func(t *testing.T) {
			got, err := ParseCredential(c.providerType, c.raw)
			if err != nil {
				t.Fatalf("ParseCredential(%s) err: %v", c.providerType, err)
			}
			if got != c.want {
				t.Fatalf("ParseCredential(%s)\n got  %+v\n want %+v", c.providerType, got, c.want)
			}
		})
	}

	// 非 AK/SK 厂商：必须显式报错
	for _, typ := range []string{"cloudflare", "aws", "azuredns", "godaddy", "ns1"} {
		if _, err := ParseCredential(typ, DNSProviderConfig{APIKey: "x"}); err == nil {
			t.Fatalf("%s 不应能作为部署凭证来源", typ)
		}
	}
}

// TestToCloudCredentials_FallbackChain 钉住「同义字段名」的优先级：
// 不同厂商/不同版本存的字段名不一样（access_key_id / access_key / secret_id），
// 回退顺序错了就会拿到空 AK 或空 SK。
func TestToCloudCredentials_FallbackChain(t *testing.T) {
	cases := []struct {
		name string
		in   DNSProviderConfig
		want cloudcred.Credentials
	}{
		{
			name: "region 优先于 region_id",
			in:   DNSProviderConfig{Region: "r1", RegionID: "r2"},
			want: cloudcred.Credentials{Region: "r1"},
		},
		{
			name: "region 空则用 region_id",
			in:   DNSProviderConfig{RegionID: "r2"},
			want: cloudcred.Credentials{Region: "r2"},
		},
		{
			name: "access_key_id 优先",
			in:   DNSProviderConfig{AccessKeyID: "a1", AccessKey: "a2", SecretID: "a3"},
			want: cloudcred.Credentials{AccessKeyID: "a1"},
		},
		{
			name: "无 access_key_id 时用 access_key",
			in:   DNSProviderConfig{AccessKey: "a2", SecretID: "a3"},
			want: cloudcred.Credentials{AccessKeyID: "a2"},
		},
		{
			name: "只剩 secret_id 时用它",
			in:   DNSProviderConfig{SecretID: "a3"},
			want: cloudcred.Credentials{AccessKeyID: "a3"},
		},
		{
			name: "access_key_secret 优先于 secret_access_key",
			in:   DNSProviderConfig{AccessKeySecret: "s1", SecretAccessKey: "s2", SecretKey: "s3"},
			want: cloudcred.Credentials{AccessKeySecret: "s1"},
		},
		{
			name: "无 access_key_secret 时用 secret_access_key",
			in:   DNSProviderConfig{SecretAccessKey: "s2", SecretKey: "s3"},
			want: cloudcred.Credentials{AccessKeySecret: "s2"},
		},
		{
			name: "只剩 secret_key 时用它",
			in:   DNSProviderConfig{SecretKey: "s3"},
			want: cloudcred.Credentials{AccessKeySecret: "s3"},
		},
		{
			name: "全空返回零值",
			in:   DNSProviderConfig{},
			want: cloudcred.Credentials{},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ToCloudCredentials(c.in); got != c.want {
				t.Fatalf("got %+v, want %+v", got, c.want)
			}
		})
	}
}

// TestConfigContract_AllProviderFieldsCarried 契约测试。
//
// 判定链：前端按各厂商的 key 写 JSON → 存进 ent 的 DNSProviderConfig 结构体 →
// mustMarshal 重新序列化 → ParseConfig 反序列化成厂商私有结构体。
// 这条链上**任何一环少一个字段都不会报错**，只会在运行期拿到空值（典型症状：
// 面板里明明填了 API User，lego 报 "API user is missing"）。
// 本项目已真实踩过：api_user / secret_api_key / secret 三个 key 长期缺失，
// 导致 Namecheap / Porkbun / 新网 三家永远鉴权失败。
func TestConfigContract_AllProviderFieldsCarried(t *testing.T) {
	providers := []any{
		CloudflareConfig{}, AliyunConfig{}, HuaweiConfig{}, TencentConfig{},
		Route53Config{}, GoogleCloudConfig{}, BaiduConfig{}, JdcloudConfig{},
		VolcengineConfig{}, EdgeOneConfig{}, AliEsaConfig{}, UCloudConfig{},
		WestCNConfig{}, Com35Config{}, RainYunConfig{}, TodayNICConfig{},
		DNSLAConfig{}, DNS51Config{}, XinnetConfig{}, PorkbunConfig{},
		NamecheapConfig{}, GoDaddyConfig{}, GandiV5Config{}, DynadotConfig{},
		AzureConfig{}, DigitalOceanConfig{}, VultrConfig{}, HetznerConfig{},
		LinodeConfig{}, OVHConfig{}, DNSimpleConfig{}, NS1Config{},
	}

	unified := map[string]bool{}
	ut := reflect.TypeFor[DNSProviderConfig]()
	for field := range ut.Fields() {
		tag, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		if tag != "" && tag != "-" {
			unified[tag] = true
		}
	}

	for _, p := range providers {
		pt := reflect.TypeOf(p)
		for f := range pt.Fields() {
			tag, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			if tag == "" || tag == "-" {
				continue
			}
			if !unified[tag] {
				t.Errorf("%s.%s 的 json key %q 在 DNSProviderConfig 里不存在 —— 该厂商此字段永远读到零值", pt.Name(), f.Name, tag)
			}
		}
	}
}

// TestIsConfigEmpty_RecognizesAllFields 反向契约：只要有任一字段有值，就不算空配置。
// 漏判会让「只填了新字段」的配置被当成空配置直接拒绝。
func TestIsConfigEmpty_RecognizesAllFields(t *testing.T) {
	if !isConfigEmpty(DNSProviderConfig{}) {
		t.Fatal("零值应判定为空")
	}

	ut := reflect.TypeFor[DNSProviderConfig]()
	for i := 0; i < ut.NumField(); i++ {
		f := ut.Field(i)
		if f.Type.Kind() != reflect.String {
			continue
		}
		var v DNSProviderConfig
		reflect.ValueOf(&v).Elem().Field(i).SetString("x")
		if isConfigEmpty(v) {
			t.Errorf("只填了 %s 时不应判定为空（isConfigEmpty 漏了这个字段）", f.Name)
		}
	}
}
