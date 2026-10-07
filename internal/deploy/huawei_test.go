package deploy

import "testing"

// TestHuaweiCertName 验证华为云 SCM 证书名净化：仅允许字母、数字、-、_，且通配符/点不导致 SCM.0032。
func TestHuaweiCertName(t *testing.T) {
	cases := []struct {
		name string
		cert CertContent
		cfg  map[string]string
		want string
	}{
		{
			name: "通配符域名净化",
			cert: CertContent{Domain: "*.example.com"},
			cfg:  map[string]string{},
			want: "wildcard-example-com",
		},
		{
			name: "普通域名净化",
			cert: CertContent{Domain: "example.com"},
			cfg:  map[string]string{},
			want: "example-com",
		},
		{
			name: "cert_name 覆盖优先且净化",
			cert: CertContent{Domain: "*.example.com"},
			cfg:  map[string]string{"cert_name": "我的证书.name"},
			want: "name",
		},
		{
			name: "保留合法字符",
			cert: CertContent{Domain: "a-b_c.example.com"},
			cfg:  map[string]string{},
			want: "a-b_c-example-com",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := huaweiCertName(c.cert, c.cfg)
			if got != c.want {
				t.Errorf("huaweiCertName = %q, want %q", got, c.want)
			}
			// 净化结果必须是华为 SCM 合法证书名（仅字母数字 - _）
			for _, r := range got {
				if !isHuaweiNameRune(r) {
					t.Errorf("huaweiCertName %q 含非法字符 %q", got, r)
				}
			}
			if len(got) > 63 {
				t.Errorf("huaweiCertName %q 超长 %d", got, len(got))
			}
		})
	}
}
