package data

import (
	"archive/zip"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cnb.cool/dtapp/certflow/internal/db"
	"cnb.cool/dtapp/certflow/internal/ent/schema"
	"cnb.cool/dtapp/certflow/internal/sqlite"
)

// TestService_Lifecycle 覆盖服务构造、目录推导与未注入 app 时的守卫。
func TestService_Lifecycle(t *testing.T) {
	dataDir := t.TempDir()
	svc := NewService(dataDir)

	if got, want := svc.dbDir(), filepath.Join(dataDir, "data"); got != want {
		t.Fatalf("dbDir() = %q, want %q", got, want)
	}

	// 未注入 app：导入导出都必须返回可读错误而不是 panic
	if _, err := svc.ExportData(); err == nil {
		t.Fatal("未注入 app 时导出口应报错")
	} else if strings.Contains(err.Error(), "error.") || strings.Contains(err.Error(), "%!") {
		t.Fatalf("导出错误文案未翻译: %q", err)
	}
	if err := svc.ImportData(); err == nil {
		t.Fatal("未注入 app 时导入应报错")
	} else if strings.Contains(err.Error(), "error.") || strings.Contains(err.Error(), "%!") {
		t.Fatalf("导入错误文案未翻译: %q", err)
	}

	// SetApp(nil) 后行为不变（仍是守卫分支）
	svc.SetApp(nil)
	if _, err := svc.ExportData(); err == nil {
		t.Fatal("app 为 nil 时仍应报错")
	}
}

// TestImportStatus 覆盖导入进度的读写与并发保护。
func TestImportStatus(t *testing.T) {
	svc := NewService(t.TempDir())

	st := svc.GetImportStatus()
	if st.Running || st.Stage != "" || st.FinishedAt != 0 {
		t.Fatalf("初始状态应为空: %+v", st)
	}

	svc.setImportStatus("正在导入表 1/3", 1, 3)
	st = svc.GetImportStatus()
	if !st.Running || st.Current != 1 || st.Total != 3 || st.Stage != "正在导入表 1/3" {
		t.Fatalf("进度状态不符: %+v", st)
	}

	svc.finishImportStatus(false, "")
	st = svc.GetImportStatus()
	if st.Running {
		t.Fatal("完成后 Running 应为 false")
	}
	if st.Error != "" {
		t.Fatalf("成功时不应有错误: %q", st.Error)
	}
	if st.FinishedAt == 0 {
		t.Fatal("完成时间戳应非零")
	}

	// 失败场景
	svc2 := NewService(t.TempDir())
	svc2.finishImportStatus(true, "导入失败")
	st = svc2.GetImportStatus()
	if st.Running || st.Error != "导入失败" || st.FinishedAt == 0 {
		t.Fatalf("失败状态不符: %+v", st)
	}
}

// TestRunImport_Success 覆盖后台导入的成功路径（真实改写 SQLite 文件）。
func TestRunImport_Success(t *testing.T) {
	ctx := context.Background()
	dataDir := filepath.Join(t.TempDir(), "app")
	if err := db.Init(dataDir); err != nil {
		t.Fatalf("db.Init 失败: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Client.DNSProvider.Create().
		SetName("导入验证").
		SetProviderType("aliyun").
		SetConfig(schema.DNSProviderConfig{AccessKeyID: "ak", AccessKeySecret: "sk"}).
		Save(ctx); err != nil {
		t.Fatalf("造数据失败: %v", err)
	}

	// 先把当前库导出为 CSV
	csvDir := filepath.Join(t.TempDir(), "csv")
	if err := os.MkdirAll(csvDir, 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	conn, err := sql.Open("sqlite3", sqlite.BuildDSN(filepath.Join(dataDir, "data", certflowDB)))
	if err != nil {
		t.Fatalf("打开连接失败: %v", err)
	}
	tables, err := exportTablesToCSV(conn, csvDir)
	if err != nil {
		conn.Close()
		t.Fatalf("导出 CSV 失败: %v", err)
	}
	conn.Close()
	if len(tables) == 0 {
		t.Fatal("应导出至少一张表")
	}

	svc := NewService(dataDir)
	// runImport 内部会关闭 ent 连接，随后用独立连接改写库文件
	svc.runImport(csvDir, tables)

	st := svc.GetImportStatus()
	if st.Running {
		t.Fatalf("导入结束后 Running 应为 false: %+v", st)
	}
	if st.Error != "" {
		t.Fatalf("导入不应失败: %s", st.Error)
	}
	if st.Current != len(tables) || st.Total != len(tables) {
		t.Fatalf("进度计数不符: %+v（共 %d 张表）", st, len(tables))
	}
	if st.Stage == "" || strings.Contains(st.Stage, "settings.data") {
		t.Fatalf("阶段文案未翻译: %q", st.Stage)
	}
	if _, err := os.Stat(csvDir); !os.IsNotExist(err) {
		t.Fatal("导入后临时目录应被清理")
	}

	// 重新打开库校验数据仍在（导入保留原 ID）
	if err := db.Init(dataDir); err != nil {
		t.Fatalf("重新初始化失败: %v", err)
	}
	n, err := db.Client.DNSProvider.Query().Count(ctx)
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if n != 1 {
		t.Fatalf("导入后应保留 1 条数据，实际 %d", n)
	}
	only, err := db.Client.DNSProvider.Query().Only(ctx)
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if only.ID != 1 || only.Name != "导入验证" {
		t.Fatalf("数据与原值不一致: %+v", only)
	}
}

// TestRunImport_InvalidTable 覆盖备份内含非本库表名时被白名单拒绝。
func TestRunImport_InvalidTable(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "app")
	if err := db.Init(dataDir); err != nil {
		t.Fatalf("db.Init 失败: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	csvDir := filepath.Join(t.TempDir(), "csv")
	if err := os.MkdirAll(csvDir, 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	// 伪造一个不在本库中的表（模拟恶意/损坏备份）
	if err := os.WriteFile(filepath.Join(csvDir, `evil" (id) VALUES (1); DROP TABLE dns_providers;--.csv`),
		[]byte("id\n1\n"), 0o600); err != nil {
		t.Fatalf("写入伪造文件失败: %v", err)
	}

	svc := NewService(dataDir)
	svc.runImport(csvDir, []string{`evil" (id) VALUES (1); DROP TABLE dns_providers;--`})

	st := svc.GetImportStatus()
	if st.Running {
		t.Fatal("失败后 Running 应为 false")
	}
	if st.Error == "" {
		t.Fatal("非法表名应导致导入失败")
	}
	if strings.Contains(st.Error, "error.") || strings.Contains(st.Error, "%!") {
		t.Fatalf("错误文案未翻译: %q", st.Error)
	}

	// 表仍存在（未被执行注入语句）
	if err := db.Init(dataDir); err != nil {
		t.Fatalf("重新初始化失败: %v", err)
	}
	if _, err := db.Client.DNSProvider.Query().Count(context.Background()); err != nil {
		t.Fatalf("dns_providers 表应仍可用: %v", err)
	}
}

// TestZipUnzipRoundTrip 覆盖打包/解压往返，并确认只打包白名单内的 CSV。
func TestZipUnzipRoundTrip(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.csv", "b.csv", "extra.csv"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name+"\n1\n"), 0o600); err != nil {
			t.Fatalf("写入失败: %v", err)
		}
	}

	zipPath := filepath.Join(t.TempDir(), "backup.zip")
	if err := zipDirTables(zipPath, dir, []string{"a", "b"}); err != nil {
		t.Fatalf("打包失败: %v", err)
	}

	destDir := filepath.Join(t.TempDir(), "out")
	if err := unzip(zipPath, destDir); err != nil {
		t.Fatalf("解压失败: %v", err)
	}
	for _, name := range []string{"a.csv", "b.csv"} {
		if _, err := os.Stat(filepath.Join(destDir, name)); err != nil {
			t.Fatalf("应解出 %s: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(destDir, "extra.csv")); !os.IsNotExist(err) {
		t.Fatal("白名单外的 CSV 不应被写入备份包")
	}

	// 解压目录自动创建（多级）
	nested := filepath.Join(t.TempDir(), "x", "y", "z")
	if err := unzip(zipPath, nested); err != nil {
		t.Fatalf("多级目录解压失败: %v", err)
	}

	// 非 zip 文件
	bad := filepath.Join(t.TempDir(), "bad.zip")
	if err := os.WriteFile(bad, []byte("not a zip"), 0o600); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	if err := unzip(bad, t.TempDir()); err == nil {
		t.Fatal("非 zip 文件应报错")
	}

	// 打包不存在的表 → 报错
	if err := zipDirTables(filepath.Join(t.TempDir(), "z.zip"), dir, []string{"missing"}); err == nil {
		t.Fatal("打包不存在的表应报错")
	}
}

// TestUnzipRejectsPathTraversal 覆盖 zip 内相对路径穿越被拒绝。
func TestUnzipRejectsPathTraversal(t *testing.T) {
	zipPath := filepath.Join(t.TempDir(), "evil.zip")
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.Create("../evil.csv")
	if err != nil {
		t.Fatalf("创建条目失败: %v", err)
	}
	if _, err := w.Write([]byte("x")); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("关闭失败: %v", err)
	}
	f.Close()

	destDir := filepath.Join(t.TempDir(), "out")
	err = unzip(zipPath, destDir)
	if err == nil {
		t.Fatal("路径穿越应被拒绝")
	}
	if !strings.Contains(err.Error(), "备份") && !strings.Contains(err.Error(), "文件") {
		t.Fatalf("错误文案不符: %q", err)
	}
	if _, statErr := os.Stat(filepath.Join(filepath.Dir(destDir), "evil.csv")); !os.IsNotExist(statErr) {
		t.Fatal("不应写出目标目录之外的文件")
	}
}

// TestCopyFile 覆盖文件复制。
func TestCopyFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	if err := os.WriteFile(src, []byte("hello"), 0o600); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	dest := filepath.Join(dir, "dest.txt")
	if err := copyFile(src, dest); err != nil {
		t.Fatalf("复制失败: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if string(got) != "hello" {
		t.Fatalf("内容不符: %q", got)
	}

	if err := copyFile(filepath.Join(dir, "missing"), dest); err == nil {
		t.Fatal("源文件不存在应报错")
	}
}
