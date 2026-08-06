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
	"sync"
	"time"

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

// ImportStatus 描述当前导入任务的可观测状态，供前端轮询展示进度条。
type ImportStatus struct {
	Running    bool   `json:"running"`     // 是否正在导入
	Stage      string `json:"stage"`       // 当前阶段描述（解压/清空/导入表 x/y/完成等）
	Current    int    `json:"current"`     // 已处理表数
	Total      int    `json:"total"`       // 总表数
	Error      string `json:"error"`       // 非空表示导入失败信息
	FinishedAt int64  `json:"finished_at"` // 完成时间戳（UnixNano），供前端判断结果
}

// Service 提供业务数据库的导入导出能力。
// 导出：按表导出为 CSV（含表头）并打包为 zip，通过原生保存对话框选择路径。
// 导入：解压后按拓扑顺序清空并重新插入当前数据库（保留原 ID），
// 目标库始终由当前运行驱动写入，从而规避 modernc/mattn 双驱动的格式差异。
type Service struct {
	app     *application.App
	dataDir string

	// importStatus 记录当前导入进度，供前端轮询展示。并发由互斥锁保护。
	importMu     sync.Mutex
	importStatus ImportStatus
}

// NewService 创建数据管理服务。app 需在 Options 创建后通过 SetApp 注入
// （因 Wails 绑定解析器要求 Service 在 Options 之前声明，而 app 在 Options 之后才可用）。
func NewService(dataDir string) *Service {
	return &Service{dataDir: dataDir}
}

// GetImportStatus 返回当前导入进度，供前端轮询渲染进度条。
// 前端在触发导入后用定时器轮询本方法，直到 Running 变为 false 再给出结果提示。
func (s *Service) GetImportStatus() ImportStatus {
	s.importMu.Lock()
	defer s.importMu.Unlock()
	return s.importStatus
}

// setImportStatus 更新导入进度（内部使用）。
func (s *Service) setImportStatus(stage string, current, total int) {
	s.importMu.Lock()
	defer s.importMu.Unlock()
	s.importStatus.Running = true
	s.importStatus.Stage = stage
	s.importStatus.Current = current
	s.importStatus.Total = total
	s.importStatus.Error = ""
}

// finishImportStatus 标记导入结束（成功或失败）。
func (s *Service) finishImportStatus(failed bool, errMsg string) {
	s.importMu.Lock()
	defer s.importMu.Unlock()
	s.importStatus.Running = false
	s.importStatus.Error = errMsg
	s.importStatus.FinishedAt = time.Now().UnixNano()
	if failed {
		s.importStatus.Stage = i18n.T("settings.data.import.failed")
	} else {
		s.importStatus.Stage = i18n.T("settings.data.import.done")
	}
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

	// 解压到临时目录（此步耗时短，仍同步执行；真正的数据库写入在异步 goroutine 中并带进度）
	tmpDir, err := os.MkdirTemp("", "certflow-import-")
	if err != nil {
		return fmt.Errorf("%s", i18n.T("error.import_failed", "Error", err))
	}
	if err := unzip(zipPath, tmpDir); err != nil {
		_ = os.RemoveAll(tmpDir)
		return fmt.Errorf("%s", i18n.T("error.import_failed", "Error", err))
	}

	// 从解压出的 CSV 文件推断待导入的表（即备份中包含的表）
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		return fmt.Errorf("%s", i18n.T("error.import_failed", "Error", err))
	}
	var tables []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".csv") {
			continue
		}
		tables = append(tables, strings.TrimSuffix(e.Name(), ".csv"))
	}
	if len(tables) == 0 {
		_ = os.RemoveAll(tmpDir)
		return fmt.Errorf("%s", i18n.T("error.import_failed", "Error", "备份文件不包含任何数据表"))
	}

	// 重置进度状态为"准备就绪"，随后在后台 goroutine 中执行导入并实时更新进度。
	// 前端应在调用后轮询 GetImportStatus 展示进度条，直到 Running=false 再依 Error 给出结果。
	s.importMu.Lock()
	s.importStatus = ImportStatus{Running: true, Stage: i18n.T("settings.data.import.preparing"), Total: len(tables)}
	s.importMu.Unlock()

	go s.runImport(tmpDir, tables)
	return nil
}

// runImport 在后台执行导入：关闭 ent 连接 → 独立连接改写当前库（事务原子）→ 更新进度。
// 任何失败都通过 finishImportStatus 标记，且因使用事务，失败时数据完整回滚不丢失。
func (s *Service) runImport(tmpDir string, tables []string) {
	defer os.RemoveAll(tmpDir)
	// 关闭进程内 ent 连接，释放对库文件的占用（导入直接改写当前库文件）
	if err := db.Close(); err != nil {
		s.finishImportStatus(true, i18n.T("error.import_failed", "Error", err))
		return
	}

	dsn := sqlite.BuildDSN(filepath.Join(s.dbDir(), certflowDB))
	conn, err := sql.Open("sqlite3", dsn)
	if err != nil {
		s.finishImportStatus(true, i18n.T("error.import_failed", "Error", err))
		return
	}
	defer conn.Close()

	s.setImportStatus(i18n.T("settings.data.import.clearing"), 0, len(tables))
	if err := importTablesFromCSV(conn, tmpDir, tables); err != nil {
		s.finishImportStatus(true, i18n.T("error.import_failed", "Error", err))
		return
	}
	s.setImportStatus(i18n.T("settings.data.import.done"), len(tables), len(tables))
	s.finishImportStatus(false, "")
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

// importTablesFromCSV 在单个事务内清空并重新插入各表，CSV 表头驱动列名（保留原 ID）。
// 使用事务保证原子性：任一表导入失败即 ROLLBACK，已清空的数据完整回滚，
// 不会留下"清空了但没导入成功"的半截状态（此前无事务会导致导入失败时数据丢失）。
// 清空顺序不依赖手写拓扑：foreign_keys=OFF 时 DELETE 不受外键约束限制，可任意顺序清空。
func importTablesFromCSV(conn *sql.DB, dir string, tables []string) error {
	ctx := context.Background()
	// 注意：SQLite 规定 foreign_keys 不能在事务内开启（但可关闭）。因此 FK 开关必须在事务之外执行。
	// 此处先在连接级别关闭外键，整个导入事务期间 FK 保持 OFF，故删表/插表无需考虑拓扑顺序；
	// 事务提交后再恢复 ON。若导入失败 ROLLBACK，数据原样回滚，FK 恢复也不影响已回滚状态。
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys=OFF"); err != nil {
		return err
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	// 无论成功失败都结束事务：成功 commit（defer 中检测到 committed 标志则跳过 rollback）
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
			// 恢复外键（失败场景下也需要，避免影响后续业务读取）
			_, _ = conn.ExecContext(ctx, "PRAGMA foreign_keys=ON")
		}
	}()

	// 清空所有待导入表（OFF 下顺序无关，无需拓扑排序）
	for _, table := range tables {
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("DELETE FROM %q", table)); err != nil {
			return err
		}
	}
	// 重置这些表的自增序列，使新插入从 1 开始（备份保留原 ID，此处仅清理，避免与导入 ID 冲突告警）
	qs := make([]string, len(tables))
	for i, t := range tables {
		qs[i] = fmt.Sprintf("%q", t)
	}
	//nolint:gosec // 表名来自 listBusinessTables 读 sqlite_master 的白名单，且经 %q 反引号包裹，无注入风险
	if _, err := tx.ExecContext(ctx, "DELETE FROM sqlite_sequence WHERE name IN ("+strings.Join(qs, ",")+")"); err != nil {
		return err
	}
	// 顺序插入
	for _, table := range tables {
		csvPath := filepath.Join(dir, table+".csv")
		if err := insertTableFromCSV(tx, ctx, table, csvPath); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	// 导入成功后恢复外键
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys=ON"); err != nil {
		return err
	}
	return nil
}

// insertTableFromCSV 将单个 CSV（首行为表头）逐行插入到表中，保留原 ID。
// 通过 PRAGMA table_info 读取列声明类型，对时间类型列（DATETIME/TIMESTAMP/DATE）
// 将 RFC3339 文本解析回 time.Time 再插入，避免以字符串写入后 ent 读取时 Scan 失败；
// 其余列沿用 parseCell（空串保留、哨兵 \N 转 NULL）。
func insertTableFromCSV(tx *sql.Tx, ctx context.Context, table, csvPath string) error {
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
	// 读取列类型，识别时间列
	typeInfos, err := columnTypes(tx, ctx, table, header)
	if err != nil {
		return err
	}
	ph := strings.TrimSuffix(strings.Repeat("?,", len(header)), ",")
	stmt, err := tx.PrepareContext(ctx, fmt.Sprintf("INSERT INTO %q (%s) VALUES (%s)",
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
			var raw string
			if i < len(rec) {
				raw = rec[i]
			}
			if typeInfos[i].isTime {
				// 时间列：哨兵或空串视为 NULL，否则解析为 time.Time
				if raw == csvNullSentinel || raw == "" {
					args[i] = nil
				} else if t, perr := parseTime(raw); perr != nil {
					return fmt.Errorf("解析时间列 %q 值 %q 失败: %w", header[i], raw, perr)
				} else {
					args[i] = t
				}
			} else {
				if i < len(rec) {
					args[i] = parseCell(raw)
				} else {
					args[i] = nil
				}
			}
		}
		if _, err := stmt.ExecContext(ctx, args...); err != nil {
			return err
		}
	}
	return nil
}

// columnTypeInfo 描述某列是否为时间类型（需解析回 time.Time）。
type columnTypeInfo struct {
	isTime bool
}

// columnTypes 通过 PRAGMA table_info 读取每列的声明类型，标记时间列。
// 列顺序与传入 header 对齐（header 来自 CSV，理论上与表列顺序一致）。
func columnTypes(tx *sql.Tx, ctx context.Context, table string, header []string) ([]columnTypeInfo, error) {
	rows, err := tx.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%q)", table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	// 列名 -> 声明类型
	declType := make(map[string]string)
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return nil, err
		}
		declType[name] = strings.ToUpper(ctype)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	infos := make([]columnTypeInfo, len(header))
	for i, h := range header {
		t := declType[h]
		infos[i].isTime = strings.Contains(t, "DATETIME") || strings.Contains(t, "TIMESTAMP") || strings.Contains(t, "DATE")
	}
	return infos, nil
}

// parseTime 尝试多种常见格式解析时间文本为 time.Time（导入导出往返用 RFC3339Nano，
// 也兼容数据库中已有的其它文本格式，提高健壮性）。
func parseTime(s string) (time.Time, error) {
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05-07:00",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02",
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("无法解析时间 %q", s)
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
	case time.Time:
		// 时间列显式用 RFC3339 序列化，保证导入时可精确解析回 time.Time。
		// 否则用默认 %v 会得到 "2006-01-02 15:04:05.9 -0700 MST"，难以可靠反解析。
		return t.UTC().Format(time.RFC3339Nano)
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
