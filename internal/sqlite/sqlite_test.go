package sqlite

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

// TestBuildDSN_Shape 钉住「两种驱动各自那套参数语法」。
// 这两个串是双驱动（modernc / mattn）唯一的分叉点，写混了不会编译失败，
// 只会在运行期悄悄丢掉外键约束或 WAL —— 那类 bug 极难在生产里发现。
func TestBuildDSN_Shape(t *testing.T) {
	dsn := BuildDSN("/tmp/x.db")

	if !strings.HasPrefix(dsn, "file:/tmp/x.db?") {
		t.Fatalf("DSN 应以 file:<path>? 开头, 实际 %q", dsn)
	}
	if strings.Contains(dsn, ":memory:") {
		t.Fatalf("不应擅自改写路径, 实际 %q", dsn)
	}

	// 两种驱动语法互斥：只允许出现其中一套
	modernc := strings.Contains(dsn, "_pragma=foreign_keys(1)")
	mattn := strings.Contains(dsn, "_foreign_keys=on")
	switch {
	case modernc && !mattn:
		if !strings.Contains(dsn, "_pragma=journal_mode(WAL)") {
			t.Errorf("modernc 缺 WAL 参数: %q", dsn)
		}
		if !strings.Contains(dsn, "_pragma=busy_timeout=5000") {
			t.Errorf("modernc 缺 busy_timeout 参数: %q", dsn)
		}
	case mattn && !modernc:
		if !strings.Contains(dsn, "_journal_mode=WAL") {
			t.Errorf("mattn 缺 WAL 参数: %q", dsn)
		}
		if !strings.Contains(dsn, "_busy_timeout=5000") {
			t.Errorf("mattn 缺 busy_timeout 参数: %q", dsn)
		}
	default:
		t.Fatalf("DSN 参数语法不属于任一驱动（混用或缺失）: %q", dsn)
	}
}

// TestBuildDSN_Usable 真的拿它开一次库：证明驱动已注册（init 生效）且参数被驱动接受。
// 参数写错时 sql.Open 不一定报错，所以这里强制 Ping + 查一次 pragma。
func TestBuildDSN_Usable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.db")

	db, err := sql.Open("sqlite3", BuildDSN(path))
	if err != nil {
		t.Fatalf("sql.Open err: %v（驱动是否已注册？）", err)
	}
	defer func() { _ = db.Close() }()

	if err := db.Ping(); err != nil {
		t.Fatalf("Ping err: %v", err)
	}

	var fk int
	if err := db.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil {
		t.Fatalf("查询 foreign_keys 失败: %v", err)
	}
	if fk != 1 {
		t.Fatalf("外键约束应为开启(1), 实际 %d", fk)
	}

	var mode string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("查询 journal_mode 失败: %v", err)
	}
	if !strings.EqualFold(mode, "wal") {
		t.Fatalf("日志模式应为 WAL, 实际 %q", mode)
	}
}
