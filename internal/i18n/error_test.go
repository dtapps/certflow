package i18n

import (
	"errors"
	"strings"
	"testing"
)

// TestNewError_Params 覆盖命名参数配对与非法入参的容错。
func TestNewError_Params(t *testing.T) {
	e := NewError("error.dns_provider_config_empty", "Provider", "aliyun", "Extra")
	if got := e.Params["Provider"]; got != "aliyun" {
		t.Fatalf("Params[Provider] = %v, want aliyun", got)
	}
	if len(e.Params) != 1 {
		t.Fatalf("落单的参数应被丢弃，实际有 %d 个", len(e.Params))
	}
	// key 不是字符串时跳过（不 panic）
	e2 := NewError("k", 1, "v")
	if len(e2.Params) != 0 {
		t.Fatalf("非字符串 key 应被跳过，实际 %+v", e2.Params)
	}
	// 无参数
	if e3 := NewError("k"); len(e3.Params) != 0 {
		t.Fatalf("无参数时 Params 应为空，实际 %+v", e3.Params)
	}
}

// TestWrap_ErrorChain 覆盖错误链：Unwrap / errors.Is / errors.As。
func TestWrap_ErrorChain(t *testing.T) {
	base := errors.New("底层失败")
	e := Wrap(base, "error.dns_provider_config_empty", "Provider", "aliyun")

	if !errors.Is(e, base) {
		t.Fatal("errors.Is 应能匹配底层错误")
	}
	var target *Error
	if !errors.As(e, &target) {
		t.Fatal("errors.As 应能取出 *Error")
	}
	if target.Key != "error.dns_provider_config_empty" {
		t.Fatalf("Key = %q", target.Key)
	}

	// Error() 含已翻译文本 + 信封（两个分隔符）
	msg := e.Error()
	if n := strings.Count(msg, sep); n != 2 {
		t.Fatalf("信封应含 2 个分隔符，实际 %d: %q", n, msg)
	}
	if !strings.Contains(msg, "底层失败") {
		t.Fatalf("Error() 应包含底层错误信息: %q", msg)
	}
}

// TestTranslateError 覆盖日志/落库用的纯翻译文本提取。
func TestTranslateError(t *testing.T) {
	if got := TranslateError(nil); got != "" {
		t.Fatalf("nil 应返回空串，实际 %q", got)
	}

	// 普通错误原样返回
	plain := errors.New("普通错误")
	if got := TranslateError(plain); got != "普通错误" {
		t.Fatalf("普通错误应原样返回，实际 %q", got)
	}

	// 带信封的错误应剥离信封
	e := Wrap(errors.New("boom"), "error.dns_provider_config_empty", "Provider", "aliyun")
	got := TranslateError(e)
	if strings.Contains(got, sep) {
		t.Fatalf("不应包含信封分隔符: %q", got)
	}
	if !strings.Contains(got, "boom") {
		t.Fatalf("应包含底层错误信息: %q", got)
	}

	// 包装在 fmt.Errorf 之类的链式错误里也能取到
	wrapped := errors.Join(errors.New("other"), e)
	if got := TranslateError(wrapped); strings.Contains(got, sep) {
		t.Fatalf("链式错误也应剥离信封: %q", got)
	}
}

// TestParseError 覆盖前端解析：有效信封、无信封、残缺信封。
func TestParseError(t *testing.T) {
	// 有效信封：往返一致
	orig := Wrap(errors.New("boom"), "error.dns_provider_config_empty", "Provider", "aliyun")
	key, params, translated, ok := ParseError(orig.Error())
	if !ok {
		t.Fatal("有效信封应解析成功")
	}
	if key != "error.dns_provider_config_empty" {
		t.Fatalf("key = %q", key)
	}
	if params["Provider"] != "aliyun" {
		t.Fatalf("params[Provider] = %v", params["Provider"])
	}
	if translated == "" {
		t.Fatal("应返回信封内的已翻译文本")
	}
	if strings.Contains(translated, sep) {
		t.Fatalf("translated 不应含分隔符: %q", translated)
	}
	if translated != TranslateError(orig) {
		t.Fatalf("translated 应与 TranslateError 一致: %q vs %q", translated, TranslateError(orig))
	}

	// 无信封：ok=false，原样返回
	key, params, translated, ok = ParseError("plain text")
	if ok || key != "" || params != nil || translated != "plain text" {
		t.Fatalf("无信封应原样返回: key=%q params=%v translated=%q ok=%v", key, params, translated, ok)
	}

	// 只有一个分隔符：残缺，按无信封处理
	malformed := "msg" + sep + "key"
	if _, _, translated, ok = ParseError(malformed); ok || translated != malformed {
		t.Fatalf("残缺信封应按原文返回: %q ok=%v", translated, ok)
	}

	// 参数 JSON 非法时不报错，params 为空 map
	if _, params, _, ok := ParseError("msg" + sep + "key" + sep + "{not-json"); !ok || len(params) != 0 {
		t.Fatalf("非法 JSON 应得到空参数并仍然 ok: %v %v", params, ok)
	}
}
