package deploy

import (
	"testing"
)

// FuzzParseConfig 模糊测试配置解析：对任意输入不得 panic 且始终返回非 nil map
func FuzzParseConfig(f *testing.F) {
	f.Add([]byte(`{"region":"cn-hangzhou","domain":"x.com"}`))
	f.Add([]byte(``))
	f.Add([]byte(`{}`))
	f.Add([]byte(`not-json`))
	f.Add([]byte(`{"nested": {"a": 1}, "arr": [1,2,3]}`))
	f.Add([]byte(`{"unclosed": `))
	f.Add([]byte{0x00, 0x01, 0x02})
	f.Add([]byte(`{"k": "` + "\x00" + `"}`))

	f.Fuzz(func(t *testing.T, raw []byte) {
		got := parseConfig(raw)
		if got == nil {
			t.Errorf("parseConfig(%q) returned nil map", raw)
		}
	})
}

// FuzzRegionFromConfig 模糊测试区域提取：region 字段存在时必原样返回
func FuzzRegionFromConfig(f *testing.F) {
	f.Add("cn-hangzhou")
	f.Add("")
	f.Add("ap-guangzhou-1")
	f.Add("cn-north-4")
	f.Add(string([]byte{0}))

	f.Fuzz(func(t *testing.T, region string) {
		if got := RegionFromConfig(map[string]string{"region": region}); got != region {
			t.Errorf("RegionFromConfig(region=%q) = %q", region, got)
		}
	})
}

// FuzzCertName 模糊测试证书名：cert_name 覆盖优先，否则回退域名
func FuzzCertName(f *testing.F) {
	f.Add("example.com")
	f.Add("")
	f.Add("*.example.com")
	f.Add("a.b.c.d.example.com")

	f.Fuzz(func(t *testing.T, domain string) {
		// 覆盖场景：始终返回 cert_name
		if got := certName(domain, map[string]string{"cert_name": "fixed"}); got != "fixed" {
			t.Errorf("certName override = %q, want fixed", got)
		}
		// 默认场景：回退域名本身
		if got := certName(domain, map[string]string{}); got != domain {
			t.Errorf("certName default = %q, want %q", got, domain)
		}
	})
}
