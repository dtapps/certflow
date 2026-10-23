package httplog

import (
	"net/http"
	"net/http/cookiejar"
	"testing"
	"time"

	"cnb.cool/dtapp/certflow/internal/logging"
	"cnb.cool/dtapp/certflow/internal/useragent"
	"go.dtapp.net/library/contrib/http_log"
)

// TestWrapTransport 校验 UA 注入始终生效、日志层仅在 DEBUG 包裹。
func TestWrapTransport(t *testing.T) {
	t.Run("非 DEBUG 仅注入 UA", func(t *testing.T) {
		setLevel(t, logging.INFO)
		useragent.Set("CertFlow-Test/1.0")
		t.Cleanup(func() { useragent.Set("") })

		base := &stubTransport{body: "ok"}
		rt := WrapTransport(base)
		ut, ok := rt.(*useragent.Transport)
		if !ok {
			t.Fatalf("应返回 *useragent.Transport，实际 %T", rt)
		}
		if _, ok := ut.Base.(*http_log.LoggingRoundTripper); ok {
			t.Fatal("非 DEBUG 级别不应包裹日志层")
		}

		req, err := http.NewRequest(http.MethodGet, "http://example.com/x", nil)
		if err != nil {
			t.Fatalf("构造请求失败: %v", err)
		}
		resp, err := rt.RoundTrip(req)
		if err != nil {
			t.Fatalf("RoundTrip 失败: %v", err)
		}
		resp.Body.Close()
		if got := base.lastReq.Header.Get("User-Agent"); got != "CertFlow-Test/1.0" {
			t.Fatalf("未注入全局 UA，实际 %q", got)
		}
	})

	t.Run("DEBUG 追加日志层", func(t *testing.T) {
		setLevel(t, logging.DEBUG)
		rt := WrapTransport(&stubTransport{})
		ut, ok := rt.(*useragent.Transport)
		if !ok {
			t.Fatalf("应返回 *useragent.Transport，实际 %T", rt)
		}
		lrt, ok := ut.Base.(*http_log.LoggingRoundTripper)
		if !ok {
			t.Fatalf("DEBUG 级别应包裹日志层，实际 %T", ut.Base)
		}
		if _, ok := lrt.Transport.(*stubTransport); !ok {
			t.Fatal("日志层应包裹原始 transport")
		}
	})

	t.Run("nil 回退到 DefaultTransport", func(t *testing.T) {
		rt := WrapTransport(nil)
		if _, ok := rt.(*useragent.Transport); !ok {
			t.Fatalf("应返回 *useragent.Transport，实际 %T", rt)
		}
	})
}

// TestWrapClient 校验客户端包裹：其余字段原样透传。
func TestWrapClient(t *testing.T) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("创建 cookie jar 失败: %v", err)
	}
	redirected := false
	orig := &http.Client{
		Transport: &stubTransport{},
		Timeout:   5 * time.Second,
		Jar:       jar,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			redirected = true
			return nil
		},
	}

	wrapped := WrapClient(orig)
	if wrapped.Timeout != orig.Timeout {
		t.Fatalf("Timeout 未透传: %v", wrapped.Timeout)
	}
	if wrapped.Jar != jar {
		t.Fatal("Jar 未透传")
	}
	if wrapped.CheckRedirect == nil {
		t.Fatal("CheckRedirect 未透传")
	}
	_ = wrapped.CheckRedirect(nil, nil)
	if !redirected {
		t.Fatal("CheckRedirect 未指向原函数")
	}
	if _, ok := wrapped.Transport.(*useragent.Transport); !ok {
		t.Fatalf("Transport 未被包裹，实际 %T", wrapped.Transport)
	}

	// nil Transport / nil client 都应安全兜底
	if c := WrapClient(&http.Client{}); c == nil || c.Transport == nil {
		t.Fatal("nil Transport 客户端应回退到 DefaultTransport")
	}
	if c := WrapClient(nil); c == nil || c.Transport == nil {
		t.Fatal("nil 客户端应返回可用客户端")
	}
}

// TestSetupTransport 覆盖 setupTransport 的 DEBUG / 非 DEBUG / nil 兜底三条路径。
func TestSetupTransport(t *testing.T) {
	origT, origC := http.DefaultTransport, http.DefaultClient
	t.Cleanup(func() {
		http.DefaultTransport, http.DefaultClient = origT, origC
	})

	t.Run("非 DEBUG 不加日志层", func(t *testing.T) {
		setLevel(t, logging.INFO)
		base := &stubTransport{}
		http.DefaultTransport = base
		setupTransport()

		ut, ok := http.DefaultTransport.(*useragent.Transport)
		if !ok {
			t.Fatalf("应包裹为 *useragent.Transport，实际 %T", http.DefaultTransport)
		}
		if ut.Base != http.RoundTripper(base) {
			t.Fatal("非 DEBUG 时底层应保持原 transport")
		}
		if http.DefaultClient.Transport != http.DefaultTransport {
			t.Fatal("DefaultClient 未同步")
		}
	})

	t.Run("DEBUG 加日志层", func(t *testing.T) {
		setLevel(t, logging.DEBUG)
		base := &stubTransport{}
		http.DefaultTransport = base
		setupTransport()

		ut, ok := http.DefaultTransport.(*useragent.Transport)
		if !ok {
			t.Fatalf("应包裹为 *useragent.Transport，实际 %T", http.DefaultTransport)
		}
		if _, ok := ut.Base.(*http_log.LoggingRoundTripper); !ok {
			t.Fatalf("DEBUG 时应包裹日志层，实际 %T", ut.Base)
		}
	})

	t.Run("DefaultTransport 为 nil 时兜底", func(t *testing.T) {
		setLevel(t, logging.INFO)
		http.DefaultTransport = nil
		setupTransport()

		ut, ok := http.DefaultTransport.(*useragent.Transport)
		if !ok {
			t.Fatalf("应包裹为 *useragent.Transport，实际 %T", http.DefaultTransport)
		}
		if _, ok := ut.Base.(*http.Transport); !ok {
			t.Fatalf("nil 时应回退到空 *http.Transport，实际 %T", ut.Base)
		}
	})
}
