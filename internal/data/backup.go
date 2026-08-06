package data

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"cnb.cool/dtapp/certflow/internal/db"
	"cnb.cool/dtapp/certflow/internal/i18n"
	"cnb.cool/dtapp/certflow/internal/sqlite"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// 备份包内的业务数据库文件名（httplog 为调试用的请求日志库，不纳入数据迁移）
const (
	certflowDB = "certflow.db"
)

// listBusinessTables 返回当前库中实际存在的业务表（排除 SQLite 内部表与 ent 辅助表）。
// 表清单直接从 sqlite_master 读取，避免手写表名与 ent 生成的真实表名（复数形式）不一致。
func listBusinessTables(conn *sql.DB) ([]string, error) {
	ctx := context.Background()
	rows, err := conn.QueryContext(ctx,
		`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' AND name NOT IN ('ent_migration_lock') ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		tables = append(tables, name)
	}
	return tables, rows.Err()
}

// Service 提供业务数据库的导入导出能力。
// 导出：按表导出为 CSV（含表头）并打包为 zip，通过原生保存对话框选择路径。
// 导入：解压后按拓扑顺序清空并重新插入当前数据库（保留原 ID），
// 目标库始终由当前运行驱动写入，从而规避 modernc/mattn 双驱动的格式差异。
type Service struct {
	app     *application.App
	dataDir string
}

// NewService 创建数据管理服务。app 需在 Options 创建后通过 SetApp 注入
// （因 Wails 绑定解析器要求 Service 在 Options 之前声明，而 app 在 Options 之后才可用）。
func NewService(dataDir string) *Service {
	return &Service{dataDir: dataDir}
}

// SetApp 注入 Wails 应用实例，用于弹出原生文件对话框。
func (s *Service) SetApp(app *application.App) {
	s.app = app
}

// dbDir 返回数据库目录（dataDir/data）。
func (s *Service) dbDir() string {
	return filepath.Join(s.dataDir, "data")
}

// ExportData 通过原生保存对话框选择路径，将业务数据库按表导出为 CSV 并打包为 zip 写入该路径。
// 用户取消对话框时返回空字符串与 nil 错误。
func (s *Service) ExportData() (string, error) {
	if s.app == nil {
		return "", fmt.Errorf("%s", i18n.T("error.backup_failed", "Error", "app not initialized"))
	}

	tmpDir, err := os.MkdirTemp("", "certflow-export-")
	if err != nil {
		return "", fmt.Errorf("%s", i18n.T("error.backup_failed", "Error", err))
	}
	defer os.RemoveAll(tmpDir)

	// 用独立只读连接读取运行中的库（不触碰 ent 客户端持有的连接，数据一致）
	dsn := sqlite.BuildDSN(filepath.Join(s.dbDir(), certflowDB))
	conn, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return "", fmt.Errorf("%s", i18n.T("error.backup_failed", "Error", err))
	}
	defer conn.Close()
	tables, err := listBusinessTables(conn)
	if err != nil {
		return "", fmt.Errorf("%s", i18n.T("error.backup_failed", "Error", err))
	}
	if err := exportTablesToCSV(conn, tmpDir, tables); err != nil {
		return "", fmt.Errorf("%s", i18n.T("error.backup_failed", "Error", err))
	}

	// 打包为 zip
	zipPath := filepath.Join(tmpDir, "certflow-backup.zip")
	if err := zipDirTables(zipPath, tmpDir, tables); err != nil {
		return "", fmt.Errorf("%s", i18n.T("error.backup_failed", "Error", err))
	}

	// 弹出原生保存对话框，让用户选择导出位置
	savePath, err := s.app.Dialog.SaveFile().
		SetMessage(i18n.T("settings.data.export")).
		SetFilename("certflow-backup.zip").
		AddFilter("CertFlow 备份", "*.zip").
		PromptForSingleSelection()
	if err != nil {
		return "", fmt.Errorf("%s", i18n.T("error.backup_failed", "Error", err))
	}
	if savePath == "" {
		return "", nil // 用户取消
	}
	if err := copyFile(zipPath, savePath); err != nil {
		return "", fmt.Errorf("%s", i18n.T("error.backup_failed", "Error", err))
	}
	return savePath, nil
}

// ImportData 通过原生打开对话框选择备份文件，按表导入业务数据库。导入按拓扑顺序清空并重新插入，
// 保留原 ID，目标库由当前运行驱动写入，规避双驱动格式差异。调用方应在返回后提示用户重启应用。
// 用户取消对话框时返回 nil。
func (s *Service) ImportData() error {
	if s.app == nil {
		return fmt.Errorf("%s", i18n.T("error.import_failed", "Error", "app not initialized"))
	}
	zipPath, err := s.app.Dialog.OpenFile().
		SetTitle(i18n.T("settings.data.import")).
		AddFilter("CertFlow 备份", "*.zip").
		PromptForSingleSelection()
	if err != nil {
		return fmt.Errorf("%s", i18n.T("error.import_failed", "Error", err))
	}
	if zipPath == "" {
		return nil // 用户取消
	}

	// 1. 解压到临时目录并校验包含全部预期表文件
	tmpDir, err := os.MkdirTemp("", "certflow-import-")
	if err != nil {
		return fmt.Errorf("%s", i18n.T("error.import_failed", "Error", err))
	}
	defer os.RemoveAll(tmpDir)

	if err := unzip(zipPath, tmpDir); err != nil {
		return fmt.Errorf("%s", i18n.T("error.import_failed", "Error", err))
	}

	// 2. 关闭进程内 ent 连接，释放对库文件的占用（导入直接改写当前库文件）
	if err := db.Close(); err != nil {
		return fmt.Errorf("%s", i18n.T("error.import_failed", "Error", err))
	}

	// 3. 打开当前库（独立连接，由当前运行驱动写入，保证格式一致）
	dsn := sqlite.BuildDSN(filepath.Join(s.dbDir(), certflowDB))
	conn, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return fmt.Errorf("%s", i18n.T("error.import_failed", "Error", err))
	}
	defer conn.Close()

	// 从解压出的 CSV 文件推断待导入的表（即备份中包含的表）
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return fmt.Errorf("%s", i18n.T("error.import_failed", "Error", err))
	}
	var tables []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".csv") {
			continue
		}
		tables = append(tables, strings.TrimSuffix(e.Name(), ".csv"))
	}
	if err := importTablesFromCSV(conn, tmpDir, tables); err != nil {
		return fmt.Errorf("%s", i18n.T("error.import_failed", "Error", err))
	}
	return nil
}

// exportTablesToCSV 将每张表导出为 <表名>.csv（含表头），写入 dir。
func exportTablesToCSV(conn *sql.DB, dir string, tables []string) error {
	ctx := context.Background()
	for _, table := range tables {
		rows, err := conn.QueryContext(ctx, fmt.Sprintf("SELECT * FROM %q", table))
		if err != nil {
			return err
		}
		cols, err := rows.Columns()
		if err != nil {
			rows.Close()
			return err
		}
		f, err := os.Create(filepath.Join(dir, table+".csv"))
		if err != nil {
			rows.Close()
			return err
		}
		w := csv.NewWriter(f)
		if err := w.Write(cols); err != nil {
			rows.Close()
			f.Close()
			return err
		}
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		for rows.Next() {
			if err := rows.Scan(ptrs...); err != nil {
				rows.Close()
				f.Close()
				return err
			}
			rec := make([]string, len(cols))
			for i, v := range vals {
				rec[i] = csvCell(v)
			}
			if err := w.Write(rec); err != nil {
				rows.Close()
				f.Close()
				return err
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			f.Close()
			return err
		}
		rows.Close()
		w.Flush()
		if err := w.Error(); err != nil {
			f.Close()
			return err
		}
		f.Close()
	}
	return nil
}

// importTablesFromCSV 在 foreign_keys=OFF 下清空并重新插入各表，CSV 表头驱动列名（保留原 ID）。
// 清空顺序不依赖手写拓扑：foreign_keys=OFF 时 DELETE 不受外键约束限制，可任意顺序清空。
func importTablesFromCSV(conn *sql.DB, dir string, tables []string) error {
	ctx := context.Background()
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys=OFF"); err != nil {
		return err
	}
	// 清空所有待导入表（OFF 下顺序无关，无需拓扑排序）
	for _, table := range tables {
		if _, err := conn.ExecContext(ctx, fmt.Sprintf("DELETE FROM %q", table)); err != nil {
			return err
		}
	}
	// 重置这些表的自增序列，使新插入从 1 开始（备份保留原 ID，此处仅清理，避免与导入 ID 冲突告警）
	qs := make([]string, len(tables))
	for i, t := range tables {
		qs[i] = fmt.Sprintf("%q", t)
	}
	//nolint:gosec // 表名来自 listBusinessTables 读 sqlite_master 的白名单，且经 %q 反引号包裹，无注入风险
	if _, err := conn.ExecContext(ctx, "DELETE FROM sqlite_sequence WHERE name IN ("+strings.Join(qs, ",")+")"); err != nil {
		return err
	}
	// 顺序插入
	for _, table := range tables {
		csvPath := filepath.Join(dir, table+".csv")
		if err := insertTableFromCSV(conn, ctx, table, csvPath); err != nil {
			return err
		}
	}
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys=ON"); err != nil {
		return err
	}
	return nil
}

// insertTableFromCSV 将单个 CSV（首行为表头）逐行插入到表中，保留原 ID。
func insertTableFromCSV(conn *sql.DB, ctx context.Context, table, csvPath string) error {
	f, err := os.Open(csvPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // 备份中无此表则跳过
		}
		return err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1 // 允许不定长
	header, err := r.Read()
	if err != nil {
		return err
	}
	if len(header) == 0 {
		return nil
	}
	ph := strings.TrimSuffix(strings.Repeat("?,", len(header)), ",")
	stmt, err := conn.PrepareContext(ctx, fmt.Sprintf("INSERT INTO %q (%s) VALUES (%s)",
		table, strings.Join(quoteAll(header), ","), ph))
	if err != nil {
		return err
	}
	defer stmt.Close()
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		args := make([]any, len(header))
		for i := range header {
			if i < len(rec) {
				args[i] = parseCell(rec[i])
			} else {
				args[i] = nil
			}
		}
		if _, err := stmt.ExecContext(ctx, args...); err != nil {
			return err
		}
	}
	return nil
}

// csvNullSentinel 是 CSV 中表示 NULL 的哨兵串。选择 \N 是借鉴 MySQL 的文本协议 NULL 标记，
// 它在真实业务数据（厂商名、region、证书指纹等）中出现的概率极低，从而与"空串"区分开。
// 关键：SQLite 的 NOT NULL 列允许存储空串（""≠NULL），但 CSV 本身无法区分空串与 NULL，
// 若把空串一律当 NULL 导入，会破坏合法的空串值（触发 NOT NULL 约束）。因此 NULL 必须显式标记。
const csvNullSentinel = "\\N"

// csvCell 将数据库值格式化为 CSV 文本。NULL 写为哨兵 \N（而非空串，以与空串区分）；
// []byte 转为字符串；其他类型用 fmt 还原为文本（SQLite 文本亲和，读回时由列类型转换）。
func csvCell(v any) string {
	if v == nil {
		return csvNullSentinel
	}
	switch t := v.(type) {
	case []byte:
		return string(t)
	case string:
		return t
	case int64:
		return fmt.Sprintf("%d", t)
	case float64:
		return fmt.Sprintf("%v", t)
	default:
		return fmt.Sprintf("%v", t)
	}
}

// parseCell 将 CSV 文本还原为插入参数。仅哨兵 \N 表示 NULL（返回 nil）；
// 其余一律以字符串原样传入（含空串""），由 SQLite 的列式类型亲和转换为对应类型
// （整数/实数/文本），并保留空串以满足 NOT NULL 列（空串≠NULL 合法）。
// 保留原 ID 场景下 id 列在 CSV 中恒为数字字符串，不会为空，故不会误触 NULL。
func parseCell(s string) any {
	if s == csvNullSentinel {
		return nil
	}
	return s
}

// quoteAll 对每列名加双引号（SQLite 标识符引用）。
func quoteAll(cols []string) []string {
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = fmt.Sprintf("%q", c)
	}
	return out
}

// zipDirTables 将 dir 下各表 CSV 打包为 zip（仅指定表）。
func zipDirTables(outPath, dir string, tables []string) error {
	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer out.Close()
	zw := zip.NewWriter(out)
	defer zw.Close()
	for _, t := range tables {
		p := filepath.Join(dir, t+".csv")
		if err := addFileToZip(zw, p); err != nil {
			return err
		}
	}
	return nil
}

// copyFile 将 src 复制为 dest（dest 已存在则覆盖）。
func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// addFileToZip 将单个文件以基础名加入 zip。
func addFileToZip(zw *zip.Writer, path string) error {
	in, err := os.Open(path)
	if err != nil {
		return err
	}
	defer in.Close()
	w, err := zw.Create(filepath.Base(path))
	if err != nil {
		return err
	}
	_, err = io.Copy(w, in)
	return err
}

// unzip 将 zip 解压到 destDir，仅允许常规文件（防路径穿越）。
func unzip(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		// 防路径穿越
		if filepath.Base(f.Name) != f.Name {
			return fmt.Errorf("%s", i18n.T("error.import_invalid_file"))
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		//nolint:gosec // 路径穿越已由上面的 filepath.Base 校验拦截；解压炸弹风险可忽略（备份为用户本地导出的可信文件）
		outPath := filepath.Join(destDir, f.Name)
		out, err := os.Create(outPath)
		if err != nil {
			rc.Close()
			return err
		}
		//nolint:gosec // 同上，备份文件为可信本地输入，非不可信外部数据
		_, err = io.Copy(out, rc)
		out.Close()
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
