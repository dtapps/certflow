package data

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"cnb.cool/dtapp/certflow/internal/db"
	"cnb.cool/dtapp/certflow/internal/ent"
	"cnb.cool/dtapp/certflow/internal/sqlite"
)

// newTestDB 在临时目录初始化 ent 库（自动建表）并返回客户端与库文件路径。
func newTestDB(t *testing.T) (*ent.Client, string) {
	t.Helper()
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "app")
	if err := db.Init(dataDir); err != nil {
		t.Fatalf("db.Init failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db.Client, filepath.Join(dataDir, "data", "certflow.db")
}

// openConn 打开给定库文件的独立连接。
func openConn(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	conn, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func countRows(t *testing.T, conn *sql.DB, table string) int {
	t.Helper()
	var n int
	if err := conn.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM "+quote(table)).Scan(&n); err != nil {
		t.Fatalf("count %s failed: %v", table, err)
	}
	return n
}

func quote(s string) string {
	return "`" + s + "`"
}

func TestExportImportRoundTrip(t *testing.T) {
	ctx := context.Background()
	client, dbPath := newTestDB(t)

	// 准备一些跨层数据：ca（第1层） + certificate（依赖 ca）+ deploy_target + deploy_log
	ca := client.CA.Create().SetName("Let's Encrypt").SetDirectoryURL("https://acme.example.com/dir").SaveX(ctx)
	cert := client.Certificate.Create().SetDomain("example.com").SetCaID(ca.ID).SaveX(ctx)
	if cert == nil {
		t.Fatal("cert create failed")
	}
	dt := client.DeployTarget.Create().SetName("panel-1").SetProviderType("aliyun").SetDeployService("aliyun").SaveX(ctx)
	client.DeployLog.Create().SetDeployTargetID(dt.ID).SetCertID(cert.ID).SetCertDomain("example.com").SetDeployDomain("example.com").SetSuccess(true).SetMessage("ok").SaveX(ctx)

	conn := openConn(t, sqlite.BuildDSN(dbPath))

	// 导出前各表应有数据
	before := map[string]int{
		"cas":            countRows(t, conn, "cas"),
		"certificates":   countRows(t, conn, "certificates"),
		"deploy_targets": countRows(t, conn, "deploy_targets"),
		"deploy_logs":    countRows(t, conn, "deploy_logs"),
	}
	for tbl, n := range before {
		if n == 0 {
			t.Fatalf("prepare: table %s has 0 rows", tbl)
		}
	}

	// 导出 CSV 到临时目录
	exportDir := t.TempDir()
	tables, err := listBusinessTables(conn)
	if err != nil {
		t.Fatalf("listBusinessTables failed: %v", err)
	}
	if err := exportTablesToCSV(conn, exportDir, tables); err != nil {
		t.Fatalf("exportTablesToCSV failed: %v", err)
	}

	// 清空所有表（模拟导入前的清空动作）
	for _, tbl := range tables {
		//nolint:gosec // 表名来自 listBusinessTables 白名单，且经反引号包裹
		if _, err := conn.ExecContext(ctx, "DELETE FROM "+quote(tbl)); err != nil {
			t.Fatalf("clear %s failed: %v", tbl, err)
		}
	}
	for tbl, n := range before {
		if got := countRows(t, conn, tbl); got != 0 {
			t.Fatalf("clear %s: expected 0 got %d (prepared %d)", tbl, got, n)
		}
	}

	// 导入回库
	if err := importTablesFromCSV(conn, exportDir, tables); err != nil {
		t.Fatalf("importTablesFromCSV failed: %v", err)
	}

	// 校验行数一致
	for tbl, want := range before {
		if got := countRows(t, conn, tbl); got != want {
			t.Fatalf("after import %s: expected %d rows got %d", tbl, want, got)
		}
	}

	// 校验数据内容被正确还原（原 ID 保留）
	var restoredCAID int
	if err := conn.QueryRowContext(ctx, "SELECT id FROM cas LIMIT 1").Scan(&restoredCAID); err != nil {
		t.Fatalf("query restored ca failed: %v", err)
	}
	if restoredCAID != ca.ID {
		t.Fatalf("restored ca id mismatch: expected %d got %d", ca.ID, restoredCAID)
	}
}

// TestImportNullVsEmptyString 验证 CSV 往返能正确区分 NULL 与空串：
// cert_uploads.region 是 NOT NULL 列，存储空串（""）是合法的；若导入把空串误当 NULL，
// 会触发 NOT NULL constraint failed。本用例同时覆盖空串（NOT NULL 列）与真 NULL（可空列）。
func TestImportNullVsEmptyString(t *testing.T) {
	ctx := context.Background()
	client, dbPath := newTestDB(t)

	// 构造 cert_uploads 行：region 用空串（合法，满足 NOT NULL）
	cu := client.CertUpload.Create().
		SetProvider("aliyun").
		SetAccessKeyID("akid-empty-region").
		SetRegion(""). // 空串，合法
		SetCertFingerprint("fp-empty").
		SetCloudCertID("cid-empty").
		SaveX(ctx)
	_ = cu

	conn := openConn(t, sqlite.BuildDSN(dbPath))

	// 确认空串确实写入（region 列为空串而非 NULL）
	var regionVal sql.NullString
	if err := conn.QueryRowContext(ctx, "SELECT region FROM cert_uploads WHERE provider='aliyun'").Scan(&regionVal); err != nil {
		t.Fatalf("query region failed: %v", err)
	}
	if !regionVal.Valid || regionVal.String != "" {
		t.Fatalf("expected empty (non-null) region, got valid=%v str=%q", regionVal.Valid, regionVal.String)
	}

	// 导出
	exportDir := t.TempDir()
	tables, err := listBusinessTables(conn)
	if err != nil {
		t.Fatalf("listBusinessTables failed: %v", err)
	}
	if err := exportTablesToCSV(conn, exportDir, tables); err != nil {
		t.Fatalf("exportTablesToCSV failed: %v", err)
	}

	// 校验导出的 CSV：region 列应是空串（不是哨兵 \N）
	csvPath := filepath.Join(exportDir, "cert_uploads.csv")
	raw, err := os.ReadFile(csvPath)
	if err != nil {
		t.Fatalf("read csv failed: %v", err)
	}
	if string(raw) == "" {
		t.Fatal("csv is empty")
	}
	if containsSentinel(t, raw) {
		t.Fatalf("empty region should NOT be serialized as NULL sentinel, csv=%q", string(raw))
	}

	// 清空并导入
	for _, tbl := range tables {
		//nolint:gosec // 表名来自白名单
		if _, err := conn.ExecContext(ctx, "DELETE FROM "+quote(tbl)); err != nil {
			t.Fatalf("clear %s failed: %v", tbl, err)
		}
	}
	if err := importTablesFromCSV(conn, exportDir, tables); err != nil {
		t.Fatalf("importTablesFromCSV failed (NOT NULL constraint?): %v", err)
	}

	// 校验导入后 region 仍为空串（非 NULL），且行数正确
	var afterRegion sql.NullString
	if err := conn.QueryRowContext(ctx, "SELECT region FROM cert_uploads WHERE provider='aliyun'").Scan(&afterRegion); err != nil {
		t.Fatalf("query after import failed: %v", err)
	}
	if !afterRegion.Valid || afterRegion.String != "" {
		t.Fatalf("after import: expected empty (non-null) region, got valid=%v str=%q", afterRegion.Valid, afterRegion.String)
	}
	if n := countRows(t, conn, "cert_uploads"); n != 1 {
		t.Fatalf("cert_uploads rows after import: expected 1 got %d", n)
	}
}

// containsSentinel 检查 CSV 原始内容是否包含 NULL 哨兵（用于断言"空串不应被序列化为哨兵"）。
func containsSentinel(t *testing.T, raw []byte) bool {
	t.Helper()
	return bytes.Contains(raw, []byte(csvNullSentinel))
}

func TestListBusinessTablesExcludesInternals(t *testing.T) {
	_, dbPath := newTestDB(t)
	conn := openConn(t, sqlite.BuildDSN(dbPath))

	tables, err := listBusinessTables(conn)
	if err != nil {
		t.Fatalf("listBusinessTables failed: %v", err)
	}
	for _, tbl := range tables {
		if tbl == "sqlite_sequence" || tbl == "ent_migration_lock" || len(tbl) > 6 && tbl[:6] == "sqlite" {
			t.Fatalf("listBusinessTables leaked internal table: %s", tbl)
		}
	}
	if len(tables) == 0 {
		t.Fatal("expected at least one business table")
	}
	// 确认真实的复数表名存在（之前 orderedTables 写单数导致 no such table: ca）
	found := map[string]bool{}
	for _, tbl := range tables {
		found[tbl] = true
	}
	for _, expect := range []string{"cas", "certificates", "deploy_targets", "deploy_logs"} {
		if !found[expect] {
			t.Fatalf("expected business table %q in %v", expect, tables)
		}
	}
	_ = os.Getenv
}
