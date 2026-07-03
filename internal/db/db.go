package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"cnb.cool/dtapp/certflow/ent"
	"cnb.cool/dtapp/certflow/internal/i18n"
	"entgo.io/ent/dialect"
	"modernc.org/sqlite"
)

func init() {
	// modernc.org/sqlite 默认注册驱动名为 "sqlite"，但 ent 的 dialect.SQLite 是 "sqlite3"
	// 将 modernc 驱动注册为 "sqlite3" 以兼容 ent
	sql.Register(dialect.SQLite, &sqlite.Driver{})
}

// DB 全局数据库客户端
var Client *ent.Client

// Init 初始化数据库连接并创建 schema
func Init(dataDir string) error {
	dbDir := filepath.Join(dataDir, "data")
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return fmt.Errorf(i18n.T("error.create_db_dir_failed", "Error", err))
	}
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout=5000", filepath.Join(dbDir, "certflow.db"))

	client, err := ent.Open(dialect.SQLite, dsn)
	if err != nil {
		return fmt.Errorf(i18n.T("error.open_db_failed", "Error", err))
	}

	// 运行自动迁移
	ctx := context.Background()
	if err := client.Schema.Create(ctx); err != nil {
		client.Close()
		return fmt.Errorf(i18n.T("error.create_schema_failed", "Error", err))
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
