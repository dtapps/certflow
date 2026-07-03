package i18n

import (
	"sort"
	"testing"
)

func TestT_KnownKeyReturnsNonEmpty(t *testing.T) {
	keys := []string{
		"error.ca_name_required",
		"error.password_too_short",
		"error.password_incorrect",
		"log.scheduler_started",
		"app.description",
	}

	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			result := T(key)
			if result == "" {
				t.Errorf("T(%q) returned empty string", key)
			}
			if result == key {
				t.Errorf("T(%q) returned the key itself, expected a translation", key)
			}
		})
	}
}

func TestT_WithTemplateData(t *testing.T) {
	result := T("error.ca_not_found_with_id", "id", 42)
	if result == "" {
		t.Fatal("T with template data returned empty string")
	}
	if result == "error.ca_not_found_with_id" {
		t.Fatal("T with template data returned the key itself")
	}
}

func TestT_FallbackToZhCN(t *testing.T) {
	SetLocale("en-US")
	result := T("nonexistent.key.that.does.not.exist")
	if result != "nonexistent.key.that.does.not.exist" {
		t.Errorf("expected key to be returned for unknown key, got %q", result)
	}
	SetLocale("zh-CN")
}

func TestSetLocale_GetLocale(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"en-US", "en-US"},
		{"zh-CN", "zh-CN"},
		{"invalid", "zh-CN"},
		{"", "zh-CN"},
		{"fr-FR", "zh-CN"},
	}

	for _, tt := range tests {
		t.Run("set_"+tt.input, func(t *testing.T) {
			SetLocale(tt.input)
			if got := GetLocale(); got != tt.expected {
				t.Errorf("GetLocale() after SetLocale(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}

	// 恢复默认语言
	SetLocale("zh-CN")
}

func TestGetCurrentLocale(t *testing.T) {
	SetLocale("en-US")
	if got := GetCurrentLocale(); got != "en-US" {
		t.Errorf("GetCurrentLocale() = %q, want %q", got, "en-US")
	}

	SetLocale("zh-CN")
	if got := GetCurrentLocale(); got != "zh-CN" {
		t.Errorf("GetCurrentLocale() = %q, want %q", got, "zh-CN")
	}
}

func TestSetLocale_UnrecognizedDefaultsToZhCN(t *testing.T) {
	original := GetLocale()
	defer SetLocale(original)

	SetLocale("de-DE")
	if got := GetLocale(); got != "zh-CN" {
		t.Errorf("SetLocale with unrecognized locale should default to zh-CN, got %q", got)
	}
}

func TestSupportedLocales(t *testing.T) {
	locales := SupportedLocales()
	if len(locales) != 2 {
		t.Fatalf("expected 2 supported locales, got %d", len(locales))
	}

	sorted := make([]string, len(locales))
	copy(sorted, locales)
	sort.Strings(sorted)
	expected := []string{"en-US", "zh-CN"}

	for i, v := range sorted {
		if v != expected[i] {
			t.Errorf("SupportedLocales()[%d] = %q, want %q", i, v, expected[i])
		}
	}
}

func TestResolveLocale(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"en-US", "en-US"},
		{"zh-CN", "zh-CN"},
		{"fr-FR", "zh-CN"},
		{"", "zh-CN"},
		{"en", "zh-CN"},
	}

	for _, tt := range tests {
		t.Run("resolve_"+tt.input, func(t *testing.T) {
			if got := ResolveLocale(tt.input); got != tt.expected {
				t.Errorf("ResolveLocale(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestTWithLocale(t *testing.T) {
	// 确保 TWithLocale 使用指定的语言
	resultEn := TWithLocale("en-US", "app.description")
	resultZh := TWithLocale("zh-CN", "app.description")

	// 两者都应返回非空值，但可能不同
	if resultEn == "" {
		t.Error("TWithLocale en-US returned empty")
	}
	if resultZh == "" {
		t.Error("TWithLocale zh-CN returned empty")
	}

	// TWithLocale 调用后语言应恢复为默认值
	if got := GetLocale(); got != "zh-CN" {
		t.Errorf("GetLocale after TWithLocale = %q, expected zh-CN (default)", got)
	}
}
