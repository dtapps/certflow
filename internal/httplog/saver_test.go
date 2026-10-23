package httplog

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cnb.cool/dtapp/certflow/internal/logging"
	"cnb.cool/dtapp/certflow/internal/sqlite"
	"go.dtapp.net/library/contrib/http_log"
)

// TestHandleLog_WritesRow 校验 DEBUG 级别下日志正确落库（含 unicode 解码与 Header 序列化）。
func TestHandleLog_WritesRow(t *testing.T) {
	setLevel(t, logging.DEBUG)

	const url = "https://api.example.com/v1/certificates"
	data := &http_log.LogData{
		Hostname:          "api.example.com",
		Method:            http.MethodPost,
		URL:               url,
		StatusCode:        http.StatusCreated,
		ElapseTime:        42,
		ProcessElapseTime: 7,
		IsError:           true,
		GoVersion:         "go1.27",
		PluginVersion:     "v1.0.13",
		RequestHeaders:    map[string][]string{"Content-Type": {"application/json"}},
		ResponseHeaders:   map[string][]string{"Server": {"nginx"}},
		RequestBody:       json.RawMessage(`{"name":"\u8bc1\u4e66"}`),
		ResponseBody:      json.RawMessage(`"ok"`),
	}

	if err := (&entLogSaver{}).HandleLog(context.Background(), data); err != nil {
		t.Fatalf("HandleLog 不应返回错误: %v", err)
	}
	if n := countByURL(t, url); n != 1 {
		t.Fatalf("应落库 1 条，实际 %d 条", n)
	}

	var (
		host, method, status    string
		reqHeaders, reqBody     string
		elapse, processElapse   int64
		isError                 bool
		goVersion, pluginVer    string
		responseHeaders, respBD string
		createdAt               time.Time
	)
	err := currentConn(t).QueryRow(`
SELECT hostname, method, CAST(status_code AS TEXT), request_headers, request_body,
       elapse_time, process_elapse_time, is_error, go_version, plugin_version,
       response_headers, response_body, created_at
FROM http_log WHERE url = ?`, url).
		Scan(&host, &method, &status, &reqHeaders, &reqBody, &elapse, &processElapse,
			&isError, &goVersion, &pluginVer, &responseHeaders, &respBD, &createdAt)
	if err != nil {
		t.Fatalf("查询落库记录失败: %v", err)
	}

	if host != "api.example.com" || method != "POST" || status != "201" {
		t.Fatalf("基础字段不符: host=%q method=%q status=%q", host, method, status)
	}
	if elapse != 42 || processElapse != 7 {
		t.Fatalf("耗时字段不符: elapse=%d process=%d", elapse, processElapse)
	}
	if !isError {
		t.Fatal("IsError 未落库")
	}
	if goVersion != "go1.27" || pluginVer != "v1.0.13" {
		t.Fatalf("版本字段不符: go=%q plugin=%q", goVersion, pluginVer)
	}
	if !strings.Contains(reqHeaders, "application/json") {
		t.Fatalf("请求头 JSON 未落库: %q", reqHeaders)
	}
	if !strings.Contains(responseHeaders, "nginx") {
		t.Fatalf("响应头 JSON 未落库: %q", responseHeaders)
	}
	// \u8bc1\u4e66 应解码为「证书」写入
	if !strings.Contains(reqBody, "证书") {
		t.Fatalf("请求体 unicode 未解码: %q", reqBody)
	}
	if respBD != `"ok"` {
		t.Fatalf("响应体落库不符: %q", respBD)
	}
	if createdAt.IsZero() {
		t.Fatal("created_at 未写入")
	}
}

// TestHandleLog_SkipBranches 覆盖空值/非 DEBUG/连接缺失三条早退分支。
func TestHandleLog_SkipBranches(t *testing.T) {
	saver := &entLogSaver{}
	ctx := context.Background()
	markURL := "https://skip.example.com/never-saved"
	data := &http_log.LogData{Method: http.MethodGet, URL: markURL, StatusCode: 200}

	t.Run("非 DEBUG 不落库", func(t *testing.T) {
		setLevel(t, logging.INFO)
		if err := saver.HandleLog(ctx, data); err != nil {
			t.Fatalf("不应返回错误: %v", err)
		}
		if n := countByURL(t, markURL); n != 0 {
			t.Fatalf("非 DEBUG 级别不应落库，实际 %d 条", n)
		}
	})

	t.Run("nil 数据不落库", func(t *testing.T) {
		setLevel(t, logging.DEBUG)
		if err := saver.HandleLog(ctx, nil); err != nil {
			t.Fatalf("不应返回错误: %v", err)
		}
		if n := countByURL(t, markURL); n != 0 {
			t.Fatalf("nil 数据不应落库，实际 %d 条", n)
		}
	})

	t.Run("常驻连接为 nil 不落库", func(t *testing.T) {
		setLevel(t, logging.DEBUG)
		mu.Lock()
		saved := conn
		conn = nil
		mu.Unlock()
		defer func() {
			mu.Lock()
			conn = saved
			mu.Unlock()
		}()

		if err := saver.HandleLog(ctx, data); err != nil {
			t.Fatalf("不应返回错误: %v", err)
		}
	})

	t.Run("写入失败仅记录不报错", func(t *testing.T) {
		setLevel(t, logging.DEBUG)
		closed, err := sql.Open("sqlite3", sqlite.BuildDSN(filepath.Join(t.TempDir(), "closed.db")))
		if err != nil {
			t.Fatalf("打开临时库失败: %v", err)
		}
		_ = closed.Close()

		mu.Lock()
		saved := conn
		conn = closed
		mu.Unlock()
		defer func() {
			mu.Lock()
			conn = saved
			mu.Unlock()
		}()

		// 落库失败时对外仍返回 nil（避免阻塞业务请求链路）
		if err := saver.HandleLog(ctx, data); err != nil {
			t.Fatalf("写入失败不应向上返回错误: %v", err)
		}
	})
}

// TestHandleLog_ThroughTransport 端到端校验：DEBUG 下 WrapClient 的请求会被自动记录。
func TestHandleLog_ThroughTransport(t *testing.T) {
	setLevel(t, logging.DEBUG)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"pong":true}`))
	}))
	defer srv.Close()

	// 显式使用独立 transport，避免包裹 http.DefaultTransport 造成重复落库
	client := WrapClient(&http.Client{Transport: &http.Transport{}})
	resp, err := client.Get(srv.URL + "/ping")
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	resp.Body.Close()

	markURL := srv.URL + "/ping"
	// 落库在独立 goroutine 中完成，此处轮询等待
	deadline := time.Now().Add(3 * time.Second)
	for countByURL(t, markURL) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("等待异步落库超时")
		}
		time.Sleep(20 * time.Millisecond)
	}

	var method, status string
	if err := currentConn(t).QueryRow(
		`SELECT method, CAST(status_code AS TEXT) FROM http_log WHERE url = ? ORDER BY id DESC LIMIT 1`, markURL).
		Scan(&method, &status); err != nil {
		t.Fatalf("查询落库记录失败: %v", err)
	}
	if method != http.MethodGet || status != "200" {
		t.Fatalf("落库内容不符: method=%q status=%q", method, status)
	}
}

// TestCleanup 校验按保留天数删除旧记录。
func TestCleanup(t *testing.T) {
	const oldURL = "https://old.example.com/retention"
	const newURL = "https://new.example.com/retention"

	c := currentConn(t)
	now := time.Now()
	if _, err := c.Exec(`INSERT INTO http_log (hostname, url, created_at) VALUES (?, ?, ?)`,
		"old.example.com", oldURL, now.AddDate(0, 0, -60)); err != nil {
		t.Fatalf("插入旧记录失败: %v", err)
	}
	if _, err := c.Exec(`INSERT INTO http_log (hostname, url, created_at) VALUES (?, ?, ?)`,
		"new.example.com", newURL, now); err != nil {
		t.Fatalf("插入新记录失败: %v", err)
	}

	n, err := Cleanup(30)
	if err != nil {
		t.Fatalf("清理失败: %v", err)
	}
	if n < 1 {
		t.Fatalf("应至少删除 1 条旧记录，实际 %d", n)
	}
	if got := countByURL(t, oldURL); got != 0 {
		t.Fatalf("旧记录未删除，剩余 %d 条", got)
	}
	if got := countByURL(t, newURL); got != 1 {
		t.Fatalf("新记录不应被删除，剩余 %d 条", got)
	}

	// retentionDays <= 0 表示不清理
	if n, err := Cleanup(0); err != nil || n != 0 {
		t.Fatalf("retentionDays=0 应直接返回，实际 n=%d err=%v", n, err)
	}

	// DSN 为空（未初始化）时直接返回
	mu.Lock()
	savedDSN := connDSN
	connDSN = ""
	mu.Unlock()
	n, err = Cleanup(30)
	mu.Lock()
	connDSN = savedDSN
	mu.Unlock()
	if err != nil || n != 0 {
		t.Fatalf("DSN 为空应直接返回，实际 n=%d err=%v", n, err)
	}

	// 连接失败路径需上报错误
	mu.Lock()
	connDSN = sqlite.BuildDSN(filepath.Join(t.TempDir(), "no-such-dir", "sub", "x.db"))
	mu.Unlock()
	if _, err := Cleanup(30); err == nil {
		t.Fatal("无法打开临时连接时应返回错误")
	}
	mu.Lock()
	connDSN = savedDSN
	mu.Unlock()
}

// TestClose 校验 Close 关闭连接且可重复调用。
func TestClose(t *testing.T) {
	mu.Lock()
	saved := conn
	other, err := sql.Open("sqlite3", sqlite.BuildDSN(filepath.Join(t.TempDir(), "close.db")))
	conn = other
	mu.Unlock()
	if err != nil {
		t.Fatalf("打开临时库失败: %v", err)
	}
	t.Cleanup(func() {
		mu.Lock()
		conn = saved
		mu.Unlock()
	})

	if err := Close(); err != nil {
		t.Fatalf("首次 Close 不应报错: %v", err)
	}
	// 第二次：conn 已为 nil，直接返回 nil
	if err := Close(); err != nil {
		t.Fatalf("重复 Close 不应报错: %v", err)
	}
}
