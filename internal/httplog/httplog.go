package httplog

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
	"unicode/utf16"

	"cnb.cool/dtapp/certflow/ent_log"
	"cnb.cool/dtapp/certflow/ent_log/httplog"
	"cnb.cool/dtapp/certflow/internal/i18n"
	"cnb.cool/dtapp/certflow/internal/logging"
	"cnb.cool/dtapp/certflow/internal/sqlite"
	"cnb.cool/dtapp/certflow/internal/useragent"
	"entgo.io/ent/dialect"
	"go.dtapp.net/library/contrib/http_log"
)

// decodeUnicodeEscapes 检测字节序列中是否包含 \uXXXX 转义序列，如有则解码为正常文本。
// 支持普通 BMP 字符（如 \u8bc1\u4e66 → 证书）和代理对（如 \uD83D\uDE00 → 😀）。
// 若不含转义序列则原样返回，避免无谓的替换开销。
func decodeUnicodeEscapes(b []byte) []byte {
	if !bytes.Contains(b, []byte(`\u`)) {
		return b
	}

	result := make([]byte, 0, len(b))
	remaining := b

	for len(remaining) > 0 {
		idx := bytes.Index(remaining, []byte(`\u`))
		if idx < 0 {
			result = append(result, remaining...)
			break
		}

		// \u 之前的内容直接保留
		result = append(result, remaining[:idx]...)

		// 检查是否有完整的 \uXXXX（至少 6 字节）
		if idx+6 > len(remaining) {
			result = append(result, remaining[idx:]...)
			break
		}

		// 解析第一个 \uXXXX
		code, err := strconv.ParseUint(string(remaining[idx+2:idx+6]), 16, 16)
		if err != nil {
			result = append(result, remaining[idx:idx+6]...)
			remaining = remaining[idx+6:]
			continue
		}

		// 高代理 (0xD800-0xDBFF)：检查是否为代理对
		if code >= 0xD800 && code <= 0xDBFF && idx+12 <= len(remaining) &&
			bytes.Equal(remaining[idx+6:idx+8], []byte(`\u`)) {
			code2, err2 := strconv.ParseUint(string(remaining[idx+8:idx+12]), 16, 16)
			if err2 == nil && code2 >= 0xDC00 && code2 <= 0xDFFF {
				// 合法的代理对，解码为一个 rune
				r := utf16.DecodeRune(rune(code), rune(code2))
				result = append(result, []byte(string(r))...)
				remaining = remaining[idx+12:]
				continue
			}
		}

		// 普通 \uXXXX
		result = append(result, []byte(string(rune(code)))...)
		remaining = remaining[idx+6:]
	}

	return result
}

// dbFileName 独立的 HTTP 请求日志数据库文件名（存放在 dataDir/data 下，与主库 certflow.db 分离）
const dbFileName = "httplog.db"

var (
	client *ent_log.Client
	mu     sync.RWMutex
	once   sync.Once
)

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
	dsn := sqlite.BuildDSN(filepath.Join(dbDir, dbFileName))

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

// setupTransport 将全局默认 HTTP 传输包裹为「UA 注入 +（DEBUG 时）日志记录」。
// UA 注入不受日志级别限制，始终生效；日志记录仅 DEBUG 级别生效。
func setupTransport() {
	// http.DefaultTransport 默认即为 *http.Transport，兜底处理 nil 情况。
	base := http.DefaultTransport
	if base == nil {
		base = &http.Transport{}
	}

	var rt = base
	if logging.Global() != nil && logging.Global().GetLevel() == logging.DEBUG {
		// 仅使用 LogHandler 接口（不传回调）；库内部 OnLog 优先、Handler 其次，二选一避免重复落库。
		rt = http_log.NewLoggingRoundTripper(rt, &entLogSaver{}, nil)
		logging.Info("%s", i18n.T("log.httplog.enabled"))
	} else {
		logging.Info("%s", i18n.T("log.httplog.skip_not_debug"))
	}

	// UA 注入放最外层：日志层记录到的请求头即为实际发出的（含 UA）。
	rt = useragent.Wrap(rt)

	http.DefaultTransport = rt
	http.DefaultClient = &http.Client{Transport: rt}
}

// WrapTransport 将任意 RoundTripper 包裹为「UA 注入 +（DEBUG 时）HTTP 请求日志」的 RoundTripper。
// UA 注入始终生效（请求未显式设置 User-Agent 时补全局 UA）；日志仅在 DEBUG 级别包裹。
// 业务侧自建 *http.Client / *http.Transport（从而绕开 http.DefaultTransport）时，
// 必须调用本函数包裹其 transport，否则这些请求不会被 httplog 记录。
// 注意：base 若已是被包裹的 transport（如 http.DefaultTransport），请勿重复包裹以免重复落库。
func WrapTransport(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	if logging.Global() != nil && logging.Global().GetLevel() == logging.DEBUG {
		base = http_log.NewLoggingRoundTripper(base, &entLogSaver{}, nil)
	}
	// UA 注入放最外层：日志层记录到的请求头即为实际发出的（含 UA）。
	// useragent.Wrap 对已包裹的 *useragent.Transport 幂等，且重复包裹无副作用（不覆盖已有 UA）。
	return useragent.Wrap(base)
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
		builder = builder.SetRequestBody(decodeUnicodeEscapes(data.RequestBody))
	}
	if data.ResponseHeaders != nil {
		builder = builder.SetResponseHeaders(data.ResponseHeaders)
	}
	if len(data.ResponseBody) > 0 {
		builder = builder.SetResponseBody(decodeUnicodeEscapes(data.ResponseBody))
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

// Cleanup 删除早於 retentionDays 天前的 HTTP 请求日志（基于 created_at）。
// retentionDays <= 0 时表示不清理，直接返回。
// 返回被删除的记录数。
func Cleanup(retentionDays int) (int, error) {
	if retentionDays <= 0 {
		return 0, nil
	}
	mu.RLock()
	db := client
	mu.RUnlock()
	if db == nil {
		return 0, nil
	}
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	n, err := db.HttpLog.Delete().Where(httplog.CreatedAtLT(cutoff)).Exec(context.Background())
	if err != nil {
		return 0, fmt.Errorf("清理 HTTP 请求日志失败: %w", err)
	}
	return n, nil
}
