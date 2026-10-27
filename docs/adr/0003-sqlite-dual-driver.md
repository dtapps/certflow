# ADR-0003：SQLite 双驱动（modernc 默认，mattn 可选）

- 状态：已采纳
- 日期：2026-07（双驱动与构建标签落地）、2026-08（基准测试补充）
- 相关：[database.md](../database.md)、[adr/0004-no-db-file-copy.md](0004-no-db-file-copy.md)

## 背景

桌面应用要嵌入式数据库。Go 生态有两条主流路线：

- `modernc.org/sqlite`：纯 Go 翻译实现，无 CGO，交叉编译简单；
- `github.com/mattn/go-sqlite3`：CGO 绑定 SQLite amalgamation，性能更好，但需要 C 工具链。

项目需要 macOS / Windows / Linux 各两种架构的构建，CI 里还包含 `windows/arm64`。CGO 会显著增加构建复杂度，但性能也不是无所谓——续期、监控、扫描都频繁读写。

`internal/sqlitetest/` 的基准（Apple M1，内存库，`benchtime=3s`）显示 mattn 全面更快：

| 操作                   | modernc   | mattn     | 差距     |
|------------------------|-----------|-----------|----------|
| Insert（单行）         | 8,603     | 5,095     | 快 1.69x |
| InsertBulk（100/事务） | 382,696   | 194,203   | 快 1.97x |
| Select（主键）         | 5,024     | 3,343     | 快 1.50x |
| Update                 | 4,848     | 3,065     | 快 1.58x |
| Delete                 | 7,968     | 6,489     | 快 1.23x |
| QueryLike（5K 行）     | 2,415,319 | 1,785,577 | 快 1.35x |

内存分配两者基本持平。完整表格见 [DEVELOPMENT.md](../../DEVELOPMENT.md)。

## 决定

**默认 modernc，mattn 通过构建标签可选**：

- 无标签：`internal/sqlite/sqlite_modernc.go`（`//go:build !sqlite_mattn`），注册 modernc 驱动，`BuildDSN` 用 `_pragma=` 语法。
- `-tags sqlite_mattn` + `CGO_ENABLED=1`：`internal/sqlite/sqlite_mattn.go`，用 mattn 的 `_foreign_keys=` 语法。
- 两者都注册为 `sqlite3`，业务代码、httplog、测试、备份统一用 `sqlite.BuildDSN` + `"sqlite3"`。
- CI 在 `release.yml` 的「配置 SQLite 驱动」步骤里：macOS / Linux 用 mattn（CGO），**Windows 一律用 modernc**（避免依赖 C 编译器，也绕开 `windows/arm64` 的限制）。

## 后果

正面：

- 默认构建零 CGO，六平台交叉编译最省事，用户不需要 C 工具链。
- 需要性能时切一行构建标签即可，不用改业务代码。
- 驱动选择集中在一个包，DSN 差异只出现在两个文件里。

负面 / 约束：

- **两套 DSN 参数语法互斥**：混用不会编译失败，只会在运行期悄悄丢外键或 WAL。`internal/sqlite/sqlite_test.go` 用测试钉死「只能出现其中一套」，并实际 Ping + 查 `PRAGMA foreign_keys` / `journal_mode` 验证生效。
- **两套驱动写出的库文件格式不保证互通**，直接把 `.db` 拷来拷去可能打不开——这是 [ADR-0004](0004-no-db-file-copy.md) 的直接原因。
- 新增依赖 SQLite 特性的代码时，要在两种驱动下都验证（尤其 JSON 函数、事务隔离行为）。
- 正式包与开发构建可能不是同一种驱动（CI 的 macOS/Linux 用 mattn，`make dev` 默认 modernc），所以跨机器迁移必须先导出再导入，不能拷库文件。CGO 构建（mattn）在 CI 里需要额外工具链，本地交叉编译也更麻烦。

## 备选方案

- **只用 modernc**：最省心，但放弃写性能；批量导入 / 大量查询时差距接近 1.5–2 倍。
- **只用 mattn**：性能最好，但 Windows/arm64 与无 CGO 环境直接不可用，发布矩阵要缩水。
- **纯 Go 的别的 SQLite 实现**：生态与稳定性都不如上面这两条路线。

## 证据

- `internal/sqlite/sqlite_modernc.go`、`internal/sqlite/sqlite_mattn.go`
- `internal/sqlite/sqlite_test.go`：DSN 形态、驱动可用性、外键与 WAL
- `internal/sqlitetest/`：基准
- `.github/workflows/release.yml`：「配置 SQLite 驱动」步骤
- `go.mod`：`modernc.org/sqlite v1.61.0`、`github.com/mattn/go-sqlite3 v1.14.52`
