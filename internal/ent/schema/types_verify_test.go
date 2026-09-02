package schema

import (
	"encoding/json"
	"testing"
)

// 验证 DeployTargetConfig 现在可直接（反）序列化（迁移后数据已是真数组）。
func TestDeployTargetConfigDirectUnmarshal(t *testing.T) {
	cases := []struct {
		name         string
		in           string
		wantDomains  int
		wantSiteID   int
		wantSiteName int
	}{
		{"真实数组domains", `{"domains":["a.dtapp.net","b.dtapp.net"],"region":"x"}`, 2, 0, 0},
		{"真实数组site", `{"site_id":["9","7"],"site_name":["x","y"]}`, 0, 2, 2},
		{"混合", `{"domains":["a"],"site_id":["1"],"site_name":["s"]}`, 1, 1, 1},
		{"空对象", `{}`, 0, 0, 0},
	}
	for _, c := range cases {
		var cfg DeployTargetConfig
		if err := json.Unmarshal([]byte(c.in), &cfg); err != nil {
			t.Fatalf("%s: unmarshal err: %v", c.name, err)
		}
		if len(cfg.Domains) != c.wantDomains {
			t.Fatalf("%s: domains=%d want %d (%v)", c.name, len(cfg.Domains), c.wantDomains, cfg.Domains)
		}
		if len(cfg.SiteID) != c.wantSiteID {
			t.Fatalf("%s: site_id=%d want %d (%v)", c.name, len(cfg.SiteID), c.wantSiteID, cfg.SiteID)
		}
		if len(cfg.SiteName) != c.wantSiteName {
			t.Fatalf("%s: site_name=%d want %d (%v)", c.name, len(cfg.SiteName), c.wantSiteName, cfg.SiteName)
		}
		// 回写不应破坏数组形态
		if _, err := json.Marshal(&cfg); err != nil {
			t.Fatalf("%s: marshal err: %v", c.name, err)
		}
	}
}
