package useragent

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// recordingTransport 记录交给 base 的请求，用于断言注入结果。
type recordingTransport struct {
	got *http.Request
}

func (r *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r.got = req
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("ok")),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

// resetUA 隔离全局状态：Set 是包级变量，测试之间必须复原。
func resetUA(t *testing.T) {
	t.Helper()
	Set("")
	t.Cleanup(func() { Set("") })
}

func TestSetGet(t *testing.T) {
	resetUA(t)

	if got := Get(); got != "" {
		t.Fatalf("初始应为空, 实际 %q", got)
	}

	Set("CertFlow-Test/1.0")
	if got := Get(); got != "CertFlow-Test/1.0" {
		t.Fatalf("Get 未返回设定值: %q", got)
	}

	// 空串是合法值：表示「不注入」，回到 Go 默认 UA
	Set("")
	if got := Get(); got != "" {
		t.Fatalf("清空后应为空, 实际 %q", got)
	}
}

func TestRoundTrip_InjectsGlobalUA(t *testing.T) {
	resetUA(t)
	Set("CertFlow-Test/1.0")

	base := &recordingTransport{}
	tr := &Transport{Base: base}

	req := httptest.NewRequest(http.MethodGet, "http://example.com/x", nil)
	req.Header.Del("User-Agent") // httptest 默认会带 Go-http-client，先清掉

	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip err: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if got := base.got.Header.Get("User-Agent"); got != "CertFlow-Test/1.0" {
		t.Fatalf("应注入全局 UA, 实际 %q", got)
	}
	// 约定：不修改原请求，注入时 Clone
	if got := req.Header.Get("User-Agent"); got != "" {
		t.Fatalf("原请求不应被修改, 实际 %q", got)
	}
}

func TestRoundTrip_KeepsExplicitUA(t *testing.T) {
	resetUA(t)
	Set("CertFlow-Test/1.0")

	base := &recordingTransport{}
	tr := &Transport{Base: base}

	req := httptest.NewRequest(http.MethodGet, "http://example.com/x", nil)
	req.Header.Set("User-Agent", "alibabacloud-sdk/1.0")

	if _, err := tr.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip err: %v", err)
	}
	if got := base.got.Header.Get("User-Agent"); got != "alibabacloud-sdk/1.0" {
		t.Fatalf("显式 UA 不应被覆盖, 实际 %q", got)
	}
}

func TestRoundTrip_NoGlobalUA(t *testing.T) {
	resetUA(t) // 全局 UA 为空 → 不注入

	base := &recordingTransport{}
	tr := &Transport{Base: base}

	req := httptest.NewRequest(http.MethodGet, "http://example.com/x", nil)
	req.Header.Del("User-Agent")

	if _, err := tr.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip err: %v", err)
	}
	if got := base.got.Header.Get("User-Agent"); got != "" {
		t.Fatalf("全局 UA 为空时不应注入, 实际 %q", got)
	}
}

func TestRoundTrip_NilBaseFallsBackToDefault(t *testing.T) {
	resetUA(t)
	Set("CertFlow-Test/1.0")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, r.Header.Get("User-Agent"))
	}))
	defer srv.Close()

	tr := &Transport{} // Base 为 nil → 走 http.DefaultTransport
	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest err: %v", err)
	}
	req.Header.Del("User-Agent")

	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip err: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "CertFlow-Test/1.0" {
		t.Fatalf("服务端收到的 UA 应为注入值, 实际 %q", body)
	}
}

func TestWrap(t *testing.T) {
	base := &recordingTransport{}

	wrapped := Wrap(base)
	tr, ok := wrapped.(*Transport)
	if !ok {
		t.Fatalf("Wrap 应返回 *Transport, 实际 %T", wrapped)
	}
	if tr.Base != base {
		t.Fatal("Wrap 应原样保留 base")
	}

	// 已包裹过的不再重复包裹（避免 Transport 套 Transport）
	if again := Wrap(tr); again != tr {
		t.Fatalf("重复 Wrap 应返回同一实例, 实际 %T", again)
	}
}
