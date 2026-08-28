package deploy

import (
	"encoding/json"
	"strings"
	"testing"

	"cnb.cool/dtapp/certflow/internal/config"
	"cnb.cool/dtapp/certflow/internal/deploycredential"
	"cnb.cool/dtapp/certflow/internal/dnsprovider"
)

func TestRegionFromConfig(t *testing.T) {
	cases := []struct {
		name string
		cfg  map[string]string
		want string
	}{
		{"nil map", nil, ""},
		{"empty map", map[string]string{}, ""},
		{"region only", map[string]string{"region": "cn-hangzhou"}, "cn-hangzhou"},
		{"region_id only", map[string]string{"region_id": "cn-hangzhou"}, "cn-hangzhou"},
		{"region priority over region_id", map[string]string{"region": "cn-beijing", "region_id": "cn-hangzhou"}, "cn-beijing"},
		{"empty region falls back to region_id", map[string]string{"region": "", "region_id": "cn-hangzhou"}, "cn-hangzhou"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := RegionFromConfig(c.cfg); got != c.want {
				t.Errorf("RegionFromConfig(%v) = %q, want %q", c.cfg, got, c.want)
			}
		})
	}
}

func TestParseConfig(t *testing.T) {
	cases := []struct {
		name string
		raw  []byte
		want map[string]string
	}{
		{"nil", nil, map[string]string{}},
		{"empty", []byte{}, map[string]string{}},
		{"invalid json returns empty", []byte("not-json"), map[string]string{}},
		{"valid", []byte(`{"region":"cn-hangzhou","domain":"x.com"}`), map[string]string{"region": "cn-hangzhou", "domain": "x.com"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseConfig(c.raw)
			if len(got) != len(c.want) {
				t.Fatalf("parseConfig(%q) = %v, want %v", c.raw, got, c.want)
			}
			for k, v := range c.want {
				if got[k] != v {
					t.Errorf("parseConfig(%q)[%q] = %q, want %q", c.raw, k, got[k], v)
				}
			}
		})
	}
}

func TestCertName(t *testing.T) {
	cases := []struct {
		name   string
		domain string
		cfg    map[string]string
		want   string
	}{
		{"default uses domain", "example.com", map[string]string{}, "example.com"},
		{"override via cert_name", "example.com", map[string]string{"cert_name": "my-cert"}, "my-cert"},
		{"empty cert_name falls back to domain", "example.com", map[string]string{"cert_name": ""}, "example.com"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := certName(c.domain, c.cfg); got != c.want {
				t.Errorf("certName(%q, %v) = %q, want %q", c.domain, c.cfg, got, c.want)
			}
		})
	}
}

func TestCredsFromConfig(t *testing.T) {
	const (
		deploy = "deploy_credential"
		dns    = "dns_provider"
	)
	cases := []struct {
		provider   string
		credSource string
		cfg        map[string]string
		wantID     string
		wantSec    string
		wantRgn    string
		wantErr    bool
	}{
		{
			provider: "aliyun", credSource: deploy,
			cfg:    map[string]string{"access_key_id": "ak", "access_key_secret": "sk", "region_id": "cn-hangzhou"},
			wantID: "ak", wantSec: "sk", wantRgn: "cn-hangzhou",
		},
		{
			provider: "huawei", credSource: deploy,
			cfg:    map[string]string{"access_key_id": "ak", "secret_access_key": "sk", "region": "cn-north-4"},
			wantID: "ak", wantSec: "sk", wantRgn: "cn-north-4",
		},
		{
			provider: "tencentcloud", credSource: deploy,
			cfg:    map[string]string{"secret_id": "ak", "secret_key": "sk", "region": "ap-guangzhou"},
			wantID: "ak", wantSec: "sk", wantRgn: "ap-guangzhou",
		},
		{
			// 部署凭证来源：百度云 SK 读 access_key_secret
			provider: "baiducloud", credSource: deploy,
			cfg:    map[string]string{"access_key_id": "ak", "access_key_secret": "sk"},
			wantID: "ak", wantSec: "sk", wantRgn: "",
		},
		{
			// DNS 凭证来源：百度云 SK 读 secret_access_key（与前一项同值不同 key）
			provider: "baiducloud", credSource: dns,
			cfg:    map[string]string{"access_key_id": "ak", "secret_access_key": "sk"},
			wantID: "ak", wantSec: "sk", wantRgn: "",
		},
		{
			// DNS 凭证来源：火山引擎 AK/SK 为 access_key/secret_key
			provider: "volcengine", credSource: dns,
			cfg:    map[string]string{"access_key": "ak", "secret_key": "sk"},
			wantID: "ak", wantSec: "sk", wantRgn: "",
		},
		{
			provider: "ctyun", credSource: deploy,
			cfg:    map[string]string{"access_key_id": "ak", "access_key_secret": "sk"},
			wantID: "ak", wantSec: "sk", wantRgn: "",
		},
		{
			// 未知厂商返回错误
			provider: "unknown", credSource: deploy,
			cfg:     map[string]string{"access_key_id": "ak", "access_key_secret": "sk"},
			wantErr: true,
		},
	}
	for _, c := range cases {
		t.Run(c.provider+"/"+c.credSource, func(t *testing.T) {
			var creds Credentials
			var err error
			if c.credSource == deploy {
				creds, err = deploycredential.Parse(c.provider, deploycredential.DeployCredentialConfig{
					AccessKeyID:     c.cfg["access_key_id"],
					AccessKeySecret: c.cfg["access_key_secret"],
					SecretAccessKey: c.cfg["secret_access_key"],
					SecretID:        c.cfg["secret_id"],
					SecretKey:       c.cfg["secret_key"],
					Region:          c.cfg["region"],
					RegionID:        c.cfg["region_id"],
					APIKey:          c.cfg["api_key"],
					PanelURL:        c.cfg["panel_url"],
					TokenID:         c.cfg["token_id"],
					TokenSecret:     c.cfg["token_secret"],
					JWTSecret:       c.cfg["jwt_secret"],
				})
			} else {
				creds, err = dnsprovider.ParseCredential(c.provider, dnsprovider.DNSProviderConfig{
					AccessKeyID:     c.cfg["access_key_id"],
					AccessKeySecret: c.cfg["access_key_secret"],
					SecretAccessKey: c.cfg["secret_access_key"],
					AccessKey:       c.cfg["access_key"],
					SecretKey:       c.cfg["secret_key"],
					SecretID:        c.cfg["secret_id"],
					Region:          c.cfg["region"],
					RegionID:        c.cfg["region_id"],
				})
			}
			if c.wantErr {
				if err == nil {
					t.Errorf("expected error for unknown provider, got nil; creds=%+v", creds)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if creds.AccessKeyID != c.wantID || creds.AccessKeySecret != c.wantSec || creds.Region != c.wantRgn {
				t.Errorf("got %+v, want id=%q sec=%q region=%q", creds, c.wantID, c.wantSec, c.wantRgn)
			}
		})
	}
}

func TestStripCreds(t *testing.T) {
	check := func(t *testing.T, out []byte, keep, drop []string, cfg map[string]string) {
		var m map[string]string
		_ = json.Unmarshal(out, &m)
		for _, k := range keep {
			if m[k] != cfg[k] {
				t.Errorf("should keep %q=%q, got %v", k, cfg[k], m)
			}
		}
		for _, k := range drop {
			if _, ok := m[k]; ok {
				t.Errorf("should have dropped %q, got %v", k, m)
			}
		}
	}
	aliyun := []byte(`{"access_key_id":"ak","access_key_secret":"sk","region_id":"cn","domain":"x.com","cert_name":"y"}`)
	out, err := config.StripSecrets[deploycredential.AliyunDeployCred](aliyun)
	if err == nil {
		check(t, out, []string{"region_id", "domain", "cert_name"}, []string{"access_key_id", "access_key_secret"}, map[string]string{"region_id": "cn", "domain": "x.com", "cert_name": "y"})
	}
	huawei := []byte(`{"access_key_id":"ak","secret_access_key":"sk","region":"cn","domain":"x.com"}`)
	out, err = config.StripSecrets[deploycredential.HuaweiDeployCred](huawei)
	if err == nil {
		check(t, out, []string{"region", "domain"}, []string{"access_key_id", "secret_access_key"}, map[string]string{"region": "cn", "domain": "x.com"})
	}
	tencent := []byte(`{"secret_id":"ak","secret_key":"sk","region":"ap","domain":"x.com"}`)
	out, err = config.StripSecrets[deploycredential.TencentDeployCred](tencent)
	if err == nil {
		check(t, out, []string{"region", "domain"}, []string{"secret_id", "secret_key"}, map[string]string{"region": "ap", "domain": "x.com"})
	}
	baidu := []byte(`{"access_key_id":"ak","access_key_secret":"sk","domain":"x.com"}`)
	out, err = config.StripSecrets[deploycredential.BaiduDeployCred](baidu)
	if err == nil {
		check(t, out, []string{"domain"}, []string{"access_key_id", "access_key_secret"}, map[string]string{"domain": "x.com"})
	}
}

// respJson 实现 ToJsonString() 接口（腾讯云 SDK 风格）
type respJson struct{ s string }

func (r respJson) ToJsonString() string { return r.s }

// respStr 实现 String() 接口（阿里云/华为云 SDK 风格）
type respStr struct{ s string }

func (r respStr) String() string { return r.s }

func TestRespDump(t *testing.T) {
	cases := []struct {
		name string
		resp any
		want string
	}{
		{"nil interface", nil, "<nil>"},
		{"nil pointer", func() any { var p *int; return p }(), "<nil>"},
		{"ToJsonString type", respJson{s: `{"a":1}`}, `{"a":1}`},
		{"String type", respStr{s: "raw-string"}, "raw-string"},
		{"default json marshal", struct{ A int }{A: 5}, `{"A":5}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := respDump(c.resp); got != c.want {
				t.Errorf("respDump(%v) = %q, want %q", c.resp, got, c.want)
			}
		})
	}
}

func TestRespDump_PointerTypeNotNil(t *testing.T) {
	// 非 nil 指针不应被当作 <nil>（验证 reflect 判空逻辑规避装箱误判）
	v := 42
	if got := respDump(&v); !strings.Contains(got, "42") {
		t.Errorf("respDump(&42) = %q, want it to contain 42", got)
	}
}
