package httplog

import (
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cnb.cool/dtapp/certflow/internal/logging"
	"cnb.cool/dtapp/certflow/internal/sqlite"
	"cnb.cool/dtapp/certflow/internal/useragent"
)

// testDataDir 测试用数据目录（由 TestMain 创建）。
// Init 内部使用 sync.Once，全进程只真正初始化一次，故所有用例共享同一常驻连接。
var testDataDir string

// TestMain 完成一次真实的 Init：建库、建表、应用迁移、包裹默认传输层。
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "certflow-httplog-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "创建临时目录失败: %v\n", err)
		os.Exit(1)
	}
	testDataDir = dir

	// 全局日志级别设为 DEBUG：Init 才会走「包裹 LoggingRoundTripper」分支。
	if err := logging.InitGlobalLogger(filepath.Join(dir, "logs"), "DEBUG", 10, 3); err != nil {
		fmt.Fprintf(os.Stderr, "初始化日志器失败: %v\n", err)
		os.Exit(1)
	}

	origTransport, origClient := http.DefaultTransport, http.DefaultClient
	if err := Init(dir); err != nil {
		fmt.Fprintf(os.Stderr, "初始化 httplog 失败: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()

	http.DefaultTransport, http.DefaultClient = origTransport, origClient
	_ = Close()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// setLevel 临时切换全局日志级别（默认恢复为 DEBUG）。
func setLevel(t *testing.T, lv logging.Level) {
	t.Helper()
	logging.Global().SetLevel(lv)
	t.Cleanup(func() { logging.Global().SetLevel(logging.DEBUG) })
}

// stubTransport 测试用 RoundTripper：记录最后一次请求并返回固定响应（不发真实网络请求）。
type stubTransport struct {
	lastReq *http.Request
	body    string
}

func (s *stubTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	s.lastReq = req
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(s.body)),
		Request:    req,
	}, nil
}

// currentConn 取常驻连接（带读锁）。
func currentConn(t *testing.T) *sql.DB {
	t.Helper()
	mu.RLock()
	c := conn
	mu.RUnlock()
	if c == nil {
		t.Fatal("常驻连接未初始化")
	}
	return c
}

// countByURL 统计某个 url 的日志条数。
// 用 url 作标记而非全表计数，避免其它用例的异步落库干扰断言。
func countByURL(t *testing.T, url string) int {
	t.Helper()
	var n int
	if err := currentConn(t).QueryRow(`SELECT COUNT(*) FROM http_log WHERE url = ?`, url).Scan(&n); err != nil {
		t.Fatalf("统计日志条数失败: %v", err)
	}
	return n
}

// TestInit 校验 Init 的副作用：建库文件、建表、包裹默认传输层、二次调用幂等。
func TestInit(t *testing.T) {
	dbFile := filepath.Join(testDataDir, "data", dbFileName)
	if _, err := os.Stat(dbFile); err != nil {
		t.Fatalf("日志库文件未创建: %v", err)
	}

	mu.RLock()
	c, dsn := conn, connDSN
	mu.RUnlock()
	if c == nil {
		t.Fatal("Init 后常驻连接不应为 nil")
	}
	if !strings.Contains(dsn, dbFileName) {
		t.Fatalf("connDSN 未包含日志库文件名: %q", dsn)
	}

	// 表已建立
	var name string
	if err := c.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='http_log'`).Scan(&name); err != nil {
		t.Fatalf("http_log 表不存在: %v", err)
	}

	// 默认传输层已包裹（UA 注入在最外层）
	if _, ok := http.DefaultTransport.(*useragent.Transport); !ok {
		t.Fatalf("http.DefaultTransport 未被包裹为 *useragent.Transport，实际 %T", http.DefaultTransport)
	}
	if http.DefaultClient == nil || http.DefaultClient.Transport != http.DefaultTransport {
		t.Fatal("http.DefaultClient 未同步包裹")
	}

	// once 语义：再次调用不报错，且不会重复初始化
	if err := Init(testDataDir); err != nil {
		t.Fatalf("二次 Init 不应报错: %v", err)
	}
}

// TestDecodeUnicodeEscapes 覆盖 unicode 转义解码的各分支。
func TestDecodeUnicodeEscapes(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"无转义原样返回", `{"msg":"ok"}`, `{"msg":"ok"}`},
		{"BMP 字符", `"\u8bc1\u4e66"`, `"证书"`},
		{"代理对（emoji）", `"\ud83d\ude00"`, `"😀"`},
		{"混合普通与转义", `a\u4e2db`, `a中b`},
		{"高位代理未跟随低位", `\ud83dabc`, "\ufffdabc"},
		{"非法十六进制原样保留", `\uZZZZx`, `\uZZZZx`},
		{"截断的转义序列", `tail\u8b`, `tail\u8b`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := string(decodeUnicodeEscapes([]byte(tt.in))); got != tt.want {
				t.Fatalf("decodeUnicodeEscapes(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}

	// 无 \u 时返回同一底层数组（零拷贝快路径）
	in := []byte("plain")
	if out := decodeUnicodeEscapes(in); &out[0] != &in[0] {
		t.Fatal("不含转义时不应复制切片")
	}
}

// TestNullableHelpers 覆盖空值映射为 NULL 的辅助函数。
func TestNullableHelpers(t *testing.T) {
	if nullableStr("") != nil {
		t.Fatal("空字符串应映射为 nil")
	}
	if v := nullableStr("x"); v == nil || *v != "x" {
		t.Fatal("非空字符串应映射为指针")
	}
	if nullableInt64(0) != nil {
		t.Fatal("零值应映射为 nil")
	}
	if v := nullableInt64(-5); v == nil || *v != -5 {
		t.Fatal("非零值应映射为指针")
	}
}

// TestMigrate 校验迁移幂等性：已存在列（duplicate column）被忽略，新列正常追加。
func TestMigrate(t *testing.T) {
	d, err := sql.Open("sqlite3", sqlite.BuildDSN(filepath.Join(t.TempDir(), "migrate.db")))
	if err != nil {
		t.Fatalf("打开测试库失败: %v", err)
	}
	if _, err := d.Exec(schemaSQL); err != nil {
		t.Fatalf("建表失败: %v", err)
	}

	// 覆盖内置迁移脚本：第一条是已存在列（模拟历史库），第二条是新列。
	orig := migrationSQL
	migrationSQL = `ALTER TABLE http_log ADD COLUMN hostname TEXT;
ALTER TABLE http_log ADD COLUMN migrated_flag TEXT DEFAULT '';`
	defer func() { migrationSQL = orig }()

	if err := Migrate(d); err != nil {
		t.Fatalf("首次迁移失败: %v", err)
	}
	// 二次执行：两条 ALTER 都会报 duplicate column，应全部忽略
	if err := Migrate(d); err != nil {
		t.Fatalf("二次迁移应幂等，实际报错: %v", err)
	}

	var n int
	if err := d.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('http_log') WHERE name = 'migrated_flag'`).Scan(&n); err != nil {
		t.Fatalf("查询列信息失败: %v", err)
	}
	if n != 1 {
		t.Fatal("新列 migrated_flag 未追加成功")
	}

	// 非幂等错误（连接已关闭）必须上报，不能被吞掉
	_ = d.Close()
	if err := Migrate(d); err == nil {
		t.Fatal("连接已关闭时 Migrate 应返回错误")
	}
}
