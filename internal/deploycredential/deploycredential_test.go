package deploycredential

import (
	"testing"

	"cnb.cool/dtapp/certflow/internal/cloudcred"
)

// TestParse 覆盖全部 provider_type 的字段映射。
// 这些映射是「前端存的 JSON 字段名 → 各厂商 SDK 使用的凭证结构」之间唯一的桥，
// 字段名写错不会编译失败、只会在部署时报鉴权错误，所以逐个钉住。
func TestParse(t *testing.T) {
	cases := []struct {
		name string
		typ  string
		raw  DeployCredentialConfig
		want cloudcred.Credentials
	}{
		{
			name: "aliyun 用 region_id",
			typ:  "aliyun",
			raw:  DeployCredentialConfig{AccessKeyID: "ak", AccessKeySecret: "sk", RegionID: "cn-hangzhou"},
			want: cloudcred.Credentials{AccessKeyID: "ak", AccessKeySecret: "sk", Region: "cn-hangzhou"},
		},
		{
			name: "huawei 用 secret_access_key",
			typ:  "huawei",
			raw:  DeployCredentialConfig{AccessKeyID: "ak", SecretAccessKey: "sk", Region: "cn-north-4"},
			want: cloudcred.Credentials{AccessKeyID: "ak", AccessKeySecret: "sk", Region: "cn-north-4"},
		},
		{
			name: "tencentcloud 用 secret_id/secret_key",
			typ:  "tencentcloud",
			raw:  DeployCredentialConfig{SecretID: "id", SecretKey: "key", Region: "ap-guangzhou"},
			want: cloudcred.Credentials{AccessKeyID: "id", AccessKeySecret: "key", Region: "ap-guangzhou"},
		},
		{
			name: "baiducloud 用 access_key_secret",
			typ:  "baiducloud",
			raw:  DeployCredentialConfig{AccessKeyID: "ak", AccessKeySecret: "sk", Region: "bj"},
			want: cloudcred.Credentials{AccessKeyID: "ak", AccessKeySecret: "sk", Region: "bj"},
		},
		{
			name: "ctyun 无 region",
			typ:  "ctyun",
			raw:  DeployCredentialConfig{AccessKeyID: "ak", AccessKeySecret: "sk"},
			want: cloudcred.Credentials{AccessKeyID: "ak", AccessKeySecret: "sk"},
		},
		{
			name: "volcengine 用 access_key_secret + region",
			typ:  "volcengine",
			raw:  DeployCredentialConfig{AccessKeyID: "ak", AccessKeySecret: "sk", Region: "cn-beijing"},
			want: cloudcred.Credentials{AccessKeyID: "ak", AccessKeySecret: "sk", Region: "cn-beijing"},
		},
		{
			name: "btpanel api_key + panel_url",
			typ:  "btpanel",
			raw:  DeployCredentialConfig{APIKey: "k", PanelURL: "https://panel:8888"},
			want: cloudcred.Credentials{AccessKeyID: "k", PanelURL: "https://panel:8888"},
		},
		{
			name: "aapanel 与宝塔同构",
			typ:  "aapanel",
			raw:  DeployCredentialConfig{APIKey: "k", PanelURL: "https://aa:8888"},
			want: cloudcred.Credentials{AccessKeyID: "k", PanelURL: "https://aa:8888"},
		},
		{
			name: "1panel api_key + panel_url",
			typ:  "1panel",
			raw:  DeployCredentialConfig{APIKey: "k", PanelURL: "https://1p:9999"},
			want: cloudcred.Credentials{AccessKeyID: "k", PanelURL: "https://1p:9999"},
		},
		{
			name: "acepanel token_id/token_secret 都要映射",
			typ:  "acepanel",
			raw:  DeployCredentialConfig{TokenID: "tid", TokenSecret: "tsec", PanelURL: "https://ace:8443"},
			want: cloudcred.Credentials{AccessKeyID: "tid", AccessKeySecret: "tsec", PanelURL: "https://ace:8443"},
		},
		{
			name: "aawaf api_key + panel_url",
			typ:  "aawaf",
			raw:  DeployCredentialConfig{APIKey: "k", PanelURL: "https://waf"},
			want: cloudcred.Credentials{AccessKeyID: "k", PanelURL: "https://waf"},
		},
		{
			name: "safeline api_key + panel_url",
			typ:  "safeline",
			raw:  DeployCredentialConfig{APIKey: "k", PanelURL: "https://sl"},
			want: cloudcred.Credentials{AccessKeyID: "k", PanelURL: "https://sl"},
		},
		{
			name: "openrestymanager 优先 jwt_secret",
			typ:  "openrestymanager",
			raw:  DeployCredentialConfig{JWTSecret: "jwt", APIKey: "old", PanelURL: "https://orm"},
			want: cloudcred.Credentials{AccessKeySecret: "jwt", PanelURL: "https://orm"},
		},
		{
			name: "openrestymanager 旧凭证回退 api_key",
			typ:  "openrestymanager",
			raw:  DeployCredentialConfig{APIKey: "old", PanelURL: "https://orm"},
			want: cloudcred.Credentials{AccessKeySecret: "old", PanelURL: "https://orm"},
		},
		{
			name: "空配置解析为零凭证不报错",
			typ:  "aliyun",
			raw:  DeployCredentialConfig{},
			want: cloudcred.Credentials{},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Parse(c.typ, c.raw)
			if err != nil {
				t.Fatalf("Parse(%s) err: %v", c.typ, err)
			}
			if got != c.want {
				t.Fatalf("Parse(%s)\n got  %+v\n want %+v", c.typ, got, c.want)
			}
		})
	}
}

func TestParse_UnsupportedProvider(t *testing.T) {
	if _, err := Parse("nonexistent", DeployCredentialConfig{}); err == nil {
		t.Fatal("未知 provider_type 应返回错误")
	}
}

func TestRegionOf(t *testing.T) {
	if got := regionOf("region", "region_id"); got != "region" {
		t.Fatalf("region 优先, 实际 %q", got)
	}
	if got := regionOf("", "region_id"); got != "region_id" {
		t.Fatalf("region 为空应回退 region_id, 实际 %q", got)
	}
	if got := regionOf("", ""); got != "" {
		t.Fatalf("都为空应为空, 实际 %q", got)
	}
}

// 防止「明面上没 region、实际上被别的字段悄悄带上」：领域类的 Region 只能来自
// region / region_id 两个字段，其余字段不得泄漏进 Region。
func TestParse_RegionNotLeakedFromOtherFields(t *testing.T) {
	got, err := Parse("huawei", DeployCredentialConfig{
		AccessKeyID:     "ak",
		SecretAccessKey: "sk",
		RegionID:        "只应给阿里云用的字段",
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got.Region != "" {
		t.Fatalf("华为云不应读取 region_id, 实际 Region=%q", got.Region)
	}
}
