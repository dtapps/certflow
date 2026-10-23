package config

import (
	"encoding/json"
	"reflect"
	"testing"
)

// testCred 覆盖三类字段：敏感字符串、普通字符串、非字符串（切片），用于验证 AsMap / StripSecrets。
type testCred struct {
	AccessKeyID     string   `json:"access_key_id,omitempty" secret:"true"`
	AccessKeySecret string   `json:"access_key_secret,omitempty" secret:"true"`
	Region          string   `json:"region,omitempty"`
	Domains         []string `json:"domains,omitempty"`
}

// secretEdge 覆盖 secretFieldNames 的两个跳过分支：无 json tag、json tag 为 "-"。
type secretEdge struct {
	NoJSON   string `secret:"true"`
	DashJSON string `json:"-" secret:"true"`
	Normal   string `json:"normal"`
}

func TestParseConfig(t *testing.T) {
	t.Run("空输入返回零值且不报错", func(t *testing.T) {
		for _, raw := range [][]byte{nil, {}} {
			v, err := ParseConfig[testCred](raw)
			if err != nil {
				t.Fatalf("空输入不应报错: %v", err)
			}
			if !reflect.DeepEqual(v, testCred{}) {
				t.Fatalf("空输入应返回零值, 实际 %+v", v)
			}
		}
	})

	t.Run("正常反序列化", func(t *testing.T) {
		raw := []byte(`{"access_key_id":"ak","region":"cn-hangzhou","domains":["a.com","b.com"]}`)
		v, err := ParseConfig[testCred](raw)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if v.AccessKeyID != "ak" || v.Region != "cn-hangzhou" {
			t.Fatalf("字段解析错误: %+v", v)
		}
		if len(v.Domains) != 2 || v.Domains[0] != "a.com" {
			t.Fatalf("切片字段解析错误: %+v", v.Domains)
		}
	})

	t.Run("非法 JSON 返回错误", func(t *testing.T) {
		if _, err := ParseConfig[testCred]([]byte(`{not json`)); err == nil {
			t.Fatal("非法 JSON 应返回错误")
		}
	})

	t.Run("类型不匹配返回错误", func(t *testing.T) {
		if _, err := ParseConfig[testCred]([]byte(`{"region":123}`)); err == nil {
			t.Fatal("类型不匹配应返回错误")
		}
	})
}

func TestMustParseConfig(t *testing.T) {
	// 非法输入降级为零值，不 panic —— 调用方依赖这个语义做「配置缺失可安全继续」。
	if v := MustParseConfig[testCred]([]byte(`{bad`)); !reflect.DeepEqual(v, testCred{}) {
		t.Fatalf("非法输入应降级为零值, 实际 %+v", v)
	}
	v := MustParseConfig[testCred]([]byte(`{"region":"cn-beijing"}`))
	if v.Region != "cn-beijing" {
		t.Fatalf("合法输入应正常解析, 实际 %+v", v)
	}
}

func TestMarshalRoundTrip(t *testing.T) {
	in := testCred{AccessKeyID: "ak", AccessKeySecret: "sk", Region: "cn-shanghai", Domains: []string{"x.com"}}
	raw, err := Marshal(in)
	if err != nil {
		t.Fatalf("Marshal err: %v", err)
	}
	out, err := ParseConfig[testCred](raw)
	if err != nil {
		t.Fatalf("ParseConfig err: %v", err)
	}
	if out.AccessKeyID != in.AccessKeyID || out.AccessKeySecret != in.AccessKeySecret ||
		out.Region != in.Region || len(out.Domains) != 1 || out.Domains[0] != "x.com" {
		t.Fatalf("往返不一致: in=%+v out=%+v", in, out)
	}
}

func TestAsMap(t *testing.T) {
	t.Run("字符串字段原样、非字符串字段重新序列化", func(t *testing.T) {
		m := AsMap(testCred{AccessKeyID: "ak", Region: "cn-hangzhou", Domains: []string{"a.com", "b.com"}})
		if m["access_key_id"] != "ak" {
			t.Fatalf("字符串字段应原样: %q", m["access_key_id"])
		}
		if m["region"] != "cn-hangzhou" {
			t.Fatalf("region 应原样: %q", m["region"])
		}
		// 非字符串字段转成 JSON 字符串，保证下游按 key 读取时不丢信息
		if m["domains"] != `["a.com","b.com"]` {
			t.Fatalf("切片字段应为 JSON 字符串, 实际 %q", m["domains"])
		}
	})

	t.Run("omitempty 空字段被丢弃", func(t *testing.T) {
		m := AsMap(testCred{Region: "r"})
		if _, ok := m["access_key_id"]; ok {
			t.Fatalf("空字段不应出现在 map: %v", m)
		}
		if _, ok := m["domains"]; ok {
			t.Fatalf("空切片不应出现在 map: %v", m)
		}
		if len(m) != 1 {
			t.Fatalf("应只剩 1 个键, 实际 %v", m)
		}
	})
}

func TestStripSecrets(t *testing.T) {
	t.Run("剔除 secret:true 字段并保留其余", func(t *testing.T) {
		raw := []byte(`{"access_key_id":"ak","access_key_secret":"sk","region":"cn-hangzhou"}`)
		out, err := StripSecrets[testCred](raw)
		if err != nil {
			t.Fatalf("StripSecrets err: %v", err)
		}
		var m map[string]any
		if err := json.Unmarshal(out, &m); err != nil {
			t.Fatalf("结果不是合法 JSON: %v", err)
		}
		if _, ok := m["access_key_id"]; ok {
			t.Errorf("access_key_id 应被剔除: %v", m)
		}
		if _, ok := m["access_key_secret"]; ok {
			t.Errorf("access_key_secret 应被剔除: %v", m)
		}
		if m["region"] != "cn-hangzhou" {
			t.Errorf("region 应保留: %v", m)
		}
	})

	t.Run("非法 JSON 原样返回并带错误", func(t *testing.T) {
		raw := []byte(`not json`)
		out, err := StripSecrets[testCred](raw)
		if err == nil {
			t.Fatal("非法 JSON 应返回错误")
		}
		if string(out) != string(raw) {
			t.Fatalf("出错时应原样返回入参, 实际 %q", out)
		}
	})

	t.Run("无 secret 字段的结构体不做任何删除", func(t *testing.T) {
		out, err := StripSecrets[secretEdge]([]byte(`{"NoJSON":"x","normal":"n"}`))
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		var m map[string]any
		_ = json.Unmarshal(out, &m)
		// 无 json tag / json:"-" 的字段不进入剔除名单（保守行为：只删明确标注且可序列化的）
		if m["NoJSON"] != "x" || m["normal"] != "n" {
			t.Fatalf("不应删除未标注 json 名的字段: %v", m)
		}
	})
}

func TestSecretFieldNames(t *testing.T) {
	names := secretFieldNames[secretEdge]()
	if len(names) != 0 {
		t.Fatalf("无可用 json tag 的字段不应进入名单: %v", names)
	}

	names = secretFieldNames[testCred]()
	if !names["access_key_id"] || !names["access_key_secret"] {
		t.Fatalf("secret 字段应被收集: %v", names)
	}
	if names["region"] || names["domains"] {
		t.Fatalf("非 secret 字段不应进入名单: %v", names)
	}
}
