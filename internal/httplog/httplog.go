package httplog

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"cnb.cool/dtapp/certflow/ent_log"
	"cnb.cool/dtapp/certflow/internal/i18n"
	"cnb.cool/dtapp/certflow/internal/logging"
	"entgo.io/ent/dialect"
	"go.dtapp.net/library/contrib/http_log"
	"modernc.org/sqlite"
)

// dbFileName 独立的 HTTP 请求日志数据库文件名（存放在 dataDir/data 下，与主库 certflow.db 分离）
const dbFileName = "httplog.db"

var (
	client *ent_log.Client
	mu     sync.RWMutex
	once   sync.Once
)

func init() {
	// modernc.org/sqlite 注册为 "sqlite3" 以兼容 ent 的 dialect.SQLite。
	// db 包已注册过，重复注册会 panic，故用 recover 兜底。
	defer func() { _ = recover() }()
	sql.Register(dialect.SQLite, &sqlite.Driver{})
}

// Init 初始化 HTTP 请求日志：打开独立的日志库，并在日志级别为 DEBUG 时
// 将 http.DefaultTransport / http.DefaultClient 包裹为 LoggingRoundTripper，
// 从而记录所有经过默认客户端的 HTTP 请求，便于排查请求是否正常。
// 非 DEBUG 级别不记录（也不包裹），避免性能损耗。
func Init(dataDir string) error {
	var initErr error
	once.Do(func() {
		initErr = initClient(dataDir)
	})
	if initErr != nil {
		return initErr
	}
	setupTransport()
	return nil
}

func initClient(dataDir string) error {
	dbDir := filepath.Join(dataDir, "data")
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return fmt.Errorf("%s", i18n.T("error.create_db_dir_failed", "Error", err))
	}
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout=5000",
		filepath.Join(dbDir, dbFileName))

	c, err := ent_log.Open(dialect.SQLite, dsn)
	if err != nil {
		return fmt.Errorf("%s", i18n.T("error.open_httplog_db_failed", "Error", err))
	}

	ctx := context.Background()
	// ent_log 包仅包含 HttpLog 一张表，这里只会创建 http_log 表，不影响主库。
	if err := c.Schema.Create(ctx); err != nil {
		c.Close()
		return fmt.Errorf("%s", i18n.T("error.create_httplog_schema_failed", "Error", err))
	}

	mu.Lock()
	client = c
	mu.Unlock()
	return nil
}

// setupTransport 将全局默认 HTTP 传输包裹为日志记录器（仅 DEBUG 级别生效）。
func setupTransport() {
	if logging.Global() == nil || logging.Global().GetLevel() != logging.DEBUG {
		logging.Info("%s", i18n.T("log.httplog.skip_not_debug"))
		return
	}

	// http.DefaultTransport 默认即为 *http.Transport，兜底处理 nil 情况。
	base := http.DefaultTransport
	if base == nil {
		base = &http.Transport{}
	}

	// 仅使用 LogHandler 接口（不传回调）；库内部 OnLog 优先、Handler 其次，二选一避免重复落库。
	rt := http_log.NewLoggingRoundTripper(base, &entLogSaver{}, nil)

	http.DefaultTransport = rt
	http.DefaultClient = &http.Client{Transport: rt}

	logging.Info("%s", i18n.T("log.httplog.enabled"))
}

// WrapTransport 将任意 RoundTripper 包裹为带 HTTP 请求日志的 RoundTripper。
// 仅在日志级别为 DEBUG 时包裹；非 DEBUG 直接返回原 transport（零开销）。
// 业务侧自建 *http.Client / *http.Transport（从而绕开 http.DefaultTransport）时，
// 必须调用本函数包裹其 transport，否则这些请求不会被 httplog 记录。
// 注意：base 若已是被包裹的 transport（如 http.DefaultTransport），请勿重复包裹以免重复落库。
func WrapTransport(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	if logging.Global() == nil || logging.Global().GetLevel() != logging.DEBUG {
		return base
	}
	return http_log.NewLoggingRoundTripper(base, &entLogSaver{}, nil)
}

// WrapClient 用 WrapTransport 包裹 client 的 Transport，返回新的 *http.Client（其余字段原样透传）。
// 用于业务侧直接 new http.Client 的场景，保证其请求也能被记录。
func WrapClient(client *http.Client) *http.Client {
	if client == nil {
		client = &http.Client{}
	}
	base := client.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	return &http.Client{
		Transport:     WrapTransport(base),
		CheckRedirect: client.CheckRedirect,
		Jar:           client.Jar,
		Timeout:       client.Timeout,
	}
}

// entLogSaver 实现 http_log.LogHandler 接口，将 HTTP 请求日志写入独立的日志数据库。
// 库的 emit 已在独立 goroutine 中调用本方法，故这里直接同步写入即可。
type entLogSaver struct{}

func (s *entLogSaver) HandleLog(ctx context.Context, data *http_log.LogData) error {
	if logging.Global() == nil || logging.Global().GetLevel() != logging.DEBUG {
		return nil
	}
	mu.RLock()
	db := client
	mu.RUnlock()
	if db == nil || data == nil {
		return nil
	}

	builder := db.HttpLog.Create().
		SetHostname(data.Hostname).
		SetMethod(data.Method).
		SetURL(data.URL).
		SetStatusCode(data.StatusCode).
		SetElapseTime(data.ElapseTime).
		SetProcessElapseTime(data.ProcessElapseTime).
		SetIsError(data.IsError).
		SetGoVersion(data.GoVersion).
		SetPluginVersion(data.PluginVersion)

	// 请求/响应体仅在非空时写入，避免向日志库写入无意义的 NULL。
	if data.RequestHeaders != nil {
		builder = builder.SetRequestHeaders(data.RequestHeaders)
	}
	if len(data.RequestBody) > 0 {
		builder = builder.SetRequestBody((data.RequestBody))
	}
	if data.ResponseHeaders != nil {
		builder = builder.SetResponseHeaders(data.ResponseHeaders)
	}
	if len(data.ResponseBody) > 0 {
		builder = builder.SetResponseBody(data.ResponseBody)
	}

	if err := builder.Exec(ctx); err != nil {
		logging.Error("%s: %v", i18n.T("log.httplog.save_failed"), err)
	}
	return nil
}

// Close 关闭日志数据库。
func Close() error {
	mu.Lock()
	defer mu.Unlock()
	if client != nil {
		err := client.Close()
		client = nil
		return err
	}
	return nil
}
