package i18n

import "testing"

// FuzzT 模糊测试翻译函数
func FuzzT(f *testing.F) {
	f.Add("error.ca_name_required")
	f.Add("error.password_too_short")
	f.Add("nonexistent.key")
	f.Add("")
	f.Add("<script>alert(1)</script>")
	f.Add(string([]byte{0, 0, 0}))

	f.Fuzz(func(t *testing.T, key string) {
		result := T(key)
		_ = result
	})
}

// FuzzTranslateWithTemplate 模糊测试带模板参数的翻译
func FuzzTranslateWithTemplate(f *testing.F) {
	f.Add("error.ca_not_found_with_id", "id")
	f.Add("notification.cert_applied.body", "domain")

	f.Fuzz(func(t *testing.T, key string, tplKey string) {
		_ = T(key, tplKey, "test-value")
	})
}

// FuzzResolveLocale 模糊测试语言环境解析
func FuzzResolveLocale(f *testing.F) {
	f.Add("en-US")
	f.Add("zh-CN")
	f.Add("en")
	f.Add("zh")
	f.Add("")
	f.Add("fr-FR")
	f.Add("ja-JP")
	f.Add("en-US\nINJECTION")
	f.Add(string([]byte{0}))

	f.Fuzz(func(t *testing.T, loc string) {
		result := ResolveLocale(loc)
		// 只应返回 zh-CN 或 en-US
		if result != "zh-CN" && result != "en-US" {
			t.Errorf("ResolveLocale(%q) = %q, want zh-CN or en-US", loc, result)
		}
	})
}
