package db

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"cnb.cool/dtapp/certflow/ent"
	"cnb.cool/dtapp/certflow/internal/i18n"
	"cnb.cool/dtapp/certflow/internal/logging"
	"cnb.cool/dtapp/certflow/internal/sqlite"
	"entgo.io/ent/dialect"
)

// DB 全局数据库客户端
var Client *ent.Client

// Init 初始化数据库连接并创建 schema
func Init(dataDir string) error {
	dbDir := filepath.Join(dataDir, "data")
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return fmt.Errorf("%s", i18n.T("error.create_db_dir_failed", "Error", err))
	}
	dsn := sqlite.BuildDSN(filepath.Join(dbDir, "certflow.db"))

	// 配置 ent 客户端选项
	opts := []ent.Option{}

	// 创建 ent SQL 日志记录器，复用全局 logging 模块（与 certflow.log / gocron.log 统一管理，含轮转），
	// 日志级别跟随全局级别（logging.Global().GetLevel()），不脱离统一日志体系。
	// 注意：ent 的自定义日志函数（ent.Log）只有在 debug 模式（ent.Debug）下才会被应用到 driver。
	// 因此只有「成功创建 entLogger 且全局级别为 DEBUG」时，才同时添加 ent.Log 与 ent.Debug：
	// 若 entLogger 创建失败，则两者都不加，避免 ent.Debug 回落到默认 log.Println 把 SQL 打到控制台（stderr）。
	logDir := filepath.Join(dataDir, "logs")
	var entLogger *logging.Logger
	if logging.Global() != nil {
		entLogger, _ = logging.NewLoggerWithFilename(logDir, "ent.log",
			logging.Global().GetLevel(),
			logging.Global().GetMaxSize(),
			logging.Global().GetMaxBackups())
	}
	if entLogger != nil && logging.Global() != nil && logging.Global().GetLevel() == logging.DEBUG {
		opts = append(opts, ent.Log(func(args ...any) {
			if len(args) > 0 {
				msg := fmt.Sprint(args...)
				msg = strings.ReplaceAll(msg, "\"", "'")
				entLogger.Debug("%s", msg)
			}
		}))
		// 调试模式下开启 ent SQL 日志（触发 driver 包裹，使上面的回调生效）。
		opts = append(opts, ent.Debug())
	} else if logging.Global() != nil && logging.Global().GetLevel() == logging.DEBUG {
		// 全局为 DEBUG 但 ent 日志文件创建失败：不开启 ent.Debug，避免 SQL 泄漏到控制台。
		logging.Warn("%s", i18n.T("log.deploy.ent_log_file_failed", "LogDir", logDir))
	}

	client, err := ent.Open(dialect.SQLite, dsn, opts...)
	if err != nil {
		return fmt.Errorf("%s", i18n.T("error.open_db_failed", "Error", err))
	}

	// 运行自动迁移
	ctx := context.Background()
	if err := client.Schema.Create(ctx); err != nil {
		client.Close()
		return fmt.Errorf("%s", i18n.T("error.create_schema_failed", "Error", err))
	}

	Client = client
	return nil
}

// Close 关闭数据库连接
func Close() error {
	if Client != nil {
		return Client.Close()
	}
	return nil
}
