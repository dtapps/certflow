package httplog

import (
	"bytes"
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf16"

	"cnb.cool/dtapp/certflow/internal/httplog/db"
	"cnb.cool/dtapp/certflow/internal/i18n"
	"cnb.cool/dtapp/certflow/internal/logging"
	"cnb.cool/dtapp/certflow/internal/sqlite"
	"cnb.cool/dtapp/certflow/internal/useragent"
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

// schemaSQL 为建表 DDL，直接嵌入 schema.sql（单一事实源，与 sqlc 类型推导共用同一文件）。
// 该库仅 http_log 一张表，仅 CREATE TABLE / INSERT / 定时 DELETE，
// 常驻连接为 append-only（只 INSERT），删除由 Cleanup 临时连接执行。
//
//go:embed schema.sql
var schemaSQL string

// migrationSQL 为迁移脚本，嵌入 migration.sql（历史库兼容：追加新列/索引）。
// 与 schema.sql 分离，遵循「建表归建表、迁移归迁移」的单一职责。
//
//go:embed migration.sql
var migrationSQL string

var (
	// conn 常驻连接，仅用于 append-only 写入（INSERT），不执行任何 DELETE。
	conn *sql.DB
	// connDSN 保存已打开连接的 DSN，供 Cleanup 临时打开独立连接删除使用。
	connDSN string
	mu      sync.RWMutex
	once    sync.Once
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

	c, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return fmt.Errorf("%s", i18n.T("error.open_httplog_db_failed", "Error", err))
	}

	ctx := context.Background()
	if _, err := c.ExecContext(ctx, schemaSQL); err != nil {
		c.Close()
		return fmt.Errorf("%s", i18n.T("error.create_httplog_schema_failed", "Error", err))
	}

	// 应用迁移脚本（历史库兼容）：逐条执行 migration.sql，忽略幂等错误。
	if err := Migrate(c); err != nil {
		c.Close()
		return fmt.Errorf("%s", i18n.T("error.create_httplog_schema_failed", "Error", err))
	}

	mu.Lock()
	conn = c
	connDSN = dsn
	mu.Unlock()
	return nil
}

// Migrate 执行 migration.sql 中的迁移语句（按 ';' 拆分，每条独立执行）。
// SQLite 不支持 ADD COLUMN IF NOT EXISTS，对已存在列的 ALTER 会报
// "duplicate column" 错误；此处忽略该错误以保证幂等——
// 二次启动 / 已迁移过的库直接跳过，不会报错。其它错误仍上报。
func Migrate(db *sql.DB) error {
	statements := strings.SplitSeq(migrationSQL, ";")
	for stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := db.Exec(stmt); err != nil {
			// 忽略列已存在的错误（兼容旧数据库）
			if !strings.Contains(err.Error(), "duplicate column") {
				return err
			}
		}
	}
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
// 写入仅通过常驻 append-only 连接（conn）执行 INSERT，绝不在此处删除。
type entLogSaver struct{}

func (s *entLogSaver) HandleLog(ctx context.Context, data *http_log.LogData) error {
	if logging.Global() == nil || logging.Global().GetLevel() != logging.DEBUG {
		return nil
	}
	mu.RLock()
	c := conn
	mu.RUnlock()
	if c == nil || data == nil {
		return nil
	}

	params := db.InsertHttpLogParams{
		Hostname:          nullableStr(data.Hostname),
		Method:            nullableStr(data.Method),
		Url:               nullableStr(data.URL),
		StatusCode:        nullableInt64(int64(data.StatusCode)),
		ElapseTime:        nullableInt64(data.ElapseTime),
		ProcessElapseTime: nullableInt64(data.ProcessElapseTime),
		IsError:           data.IsError,
		CreatedAt:         sql.NullTime{Time: time.Now(), Valid: true},
		GoVersion:         nullableStr(data.GoVersion),
		PluginVersion:     nullableStr(data.PluginVersion),
	}

	// 请求/响应头序列化为 JSON 文本（对应 ent 的 TypeJSON map[string][]string）。
	if data.RequestHeaders != nil {
		if b, err := json.Marshal(data.RequestHeaders); err == nil {
			params.RequestHeaders = sql.NullString{String: string(b), Valid: true}
		}
	}
	if data.ResponseHeaders != nil {
		if b, err := json.Marshal(data.ResponseHeaders); err == nil {
			params.ResponseHeaders = sql.NullString{String: string(b), Valid: true}
		}
	}
	// 请求/响应体仅在非空时写入，避免向日志库写入无意义的 NULL。
	// []byte 字段以 nil 表达 NULL，非空时赋解码后的切片。
	if len(data.RequestBody) > 0 {
		params.RequestBody = decodeUnicodeEscapes(data.RequestBody)
	}
	if len(data.ResponseBody) > 0 {
		params.ResponseBody = decodeUnicodeEscapes(data.ResponseBody)
	}

	if err := db.New(c).InsertHttpLog(ctx, params); err != nil {
		logging.Error("%s: %v", i18n.T("log.httplog.save_failed"), err)
	}
	return nil
}

// nullableStr 将 Go 字符串转为 *string（空字符串视为 NULL）。
func nullableStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// nullableInt64 将 int64 转为 *int64（零值视为 NULL）。
func nullableInt64(v int64) *int64 {
	if v == 0 {
		return nil
	}
	return &v
}

// Close 关闭常驻日志数据库连接。
func Close() error {
	mu.Lock()
	defer mu.Unlock()
	if conn != nil {
		err := conn.Close()
		conn = nil
		return err
	}
	return nil
}

// Cleanup 删除早于 retentionDays 天前的 HTTP 请求日志（基于 created_at）。
// retentionDays <= 0 时表示不清理，直接返回。
// 删除为定时任务，使用临时建立的独立连接执行，不影响常驻 append-only 连接。
// 返回被删除的记录数。
func Cleanup(retentionDays int) (int, error) {
	if retentionDays <= 0 {
		return 0, nil
	}

	mu.RLock()
	dsn := connDSN
	mu.RUnlock()
	if dsn == "" {
		return 0, nil
	}

	// 临时连接：单独打开同一 httplog.db 文件执行 DELETE，用完即关。
	tmp, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return 0, fmt.Errorf("打开临时清理连接失败: %w", err)
	}
	defer tmp.Close()

	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	n, err := db.New(tmp).DeleteOldHttpLog(context.Background(), sql.NullTime{Time: cutoff, Valid: true})
	if err != nil {
		return 0, fmt.Errorf("清理 HTTP 请求日志失败: %w", err)
	}
	return int(n), nil
}
