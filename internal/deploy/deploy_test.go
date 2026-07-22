package deploy

import (
	"strings"
	"testing"
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
			// 未知厂商返回空凭证
			provider: "unknown", credSource: deploy,
			cfg:    map[string]string{"access_key_id": "ak", "access_key_secret": "sk"},
			wantID: "", wantSec: "", wantRgn: "",
		},
	}
	for _, c := range cases {
		t.Run(c.provider+"/"+c.credSource, func(t *testing.T) {
			creds := credsFromConfig(c.provider, c.credSource, c.cfg)
			if creds.AccessKeyID != c.wantID || creds.AccessKeySecret != c.wantSec || creds.Region != c.wantRgn {
				t.Errorf("credsFromConfig(%q,%q) = %+v, want id=%q sec=%q region=%q",
					c.provider, c.credSource, creds, c.wantID, c.wantSec, c.wantRgn)
			}
		})
	}
}

func TestStripCreds(t *testing.T) {
	cases := []struct {
		provider string
		cfg      map[string]string
		keep     []string // 期望保留的 key
		drop     []string // 期望被剔除的 key
	}{
		{
			provider: "aliyun",
			cfg:      map[string]string{"access_key_id": "ak", "access_key_secret": "sk", "region_id": "cn", "domain": "x.com", "cert_name": "y"},
			keep:     []string{"domain", "cert_name"},
			drop:     []string{"access_key_id", "access_key_secret", "region_id"},
		},
		{
			provider: "huawei",
			cfg:      map[string]string{"access_key_id": "ak", "secret_access_key": "sk", "region": "cn", "domain": "x.com"},
			keep:     []string{"domain"},
			drop:     []string{"access_key_id", "secret_access_key", "region"},
		},
		{
			provider: "tencentcloud",
			cfg:      map[string]string{"secret_id": "ak", "secret_key": "sk", "region": "ap", "domain": "x.com"},
			keep:     []string{"domain"},
			drop:     []string{"secret_id", "secret_key", "region"},
		},
		{
			provider: "baiducloud",
			cfg:      map[string]string{"access_key_id": "ak", "access_key_secret": "sk", "domain": "x.com"},
			keep:     []string{"domain"},
			drop:     []string{"access_key_id", "access_key_secret"},
		},
	}
	for _, c := range cases {
		t.Run(c.provider, func(t *testing.T) {
			got := stripCreds(c.provider, c.cfg)
			for _, k := range c.keep {
				if _, ok := got[k]; !ok {
					t.Errorf("stripCreds(%q) missing expected key %q, got %v", c.provider, k, got)
				}
			}
			for _, k := range c.drop {
				if _, ok := got[k]; ok {
					t.Errorf("stripCreds(%q) should have dropped key %q, got %v", c.provider, k, got)
				}
			}
		})
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
