package db

import (
	"strings"
	"testing"
)

func TestNormalizeDeployTargetConfig(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		changed bool
	}{
		{
			name:    "双重编码domains",
			in:      `{"domains":"[\"a.dtapp.net\",\"b.dtapp.net\"]","region":"x"}`,
			changed: true,
		},
		{
			name:    "双重编码site_id/site_name",
			in:      `{"site_id":"[\"9\",\"7\"]","site_name":"[\"x\",\"y\"]"}`,
			changed: true,
		},
		{
			name:    "单值site_id(cdn)",
			in:      `{"site_id":"1037605694327280","site_name":"dtapp.net"}`,
			changed: true,
		},
		{
			name:    "已是数组，不变更",
			in:      `{"domains":["a","b"],"site_id":["1"]}`,
			changed: false,
		},
		{
			name:    "空串字段规整为[]",
			in:      `{"domains":""}`,
			changed: true,
		},
	}
	for _, c := range cases {
		out, changed := normalizeDeployTargetConfig(c.in)
		if changed != c.changed {
			t.Fatalf("%s: changed=%v want %v (out=%s)", c.name, changed, c.changed, out)
		}
		if !strings.Contains(out, "[") {
			t.Fatalf("%s: 结果缺少数组形态: %s", c.name, out)
		}
	}
}
