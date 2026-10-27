# ADR-0004：导入导出禁止复制 `.db` 文件

- 状态：已采纳（约束长期有效）
- 日期：2026-08（数据管理功能落地），2026-10 修订实现细节
- 相关：[database.md](../database.md)、[adr/0003-sqlite-dual-driver.md](0003-sqlite-dual-driver.md)

## 背景

设置页要提供「数据管理」：导出业务数据成备份、在另一台机器导入。最直觉的实现是复制 `certflow.db` 文件。

但项目是 SQLite **双驱动**（[ADR-0003](0003-sqlite-dual-driver.md)）：

- 默认 modernc（纯 Go）；
- 可选 mattn（CGO）；
- 构建标签不同，写出的库文件格式不保证互通；
- 还有 WAL / SHM 侧车文件、运行中连接、ent 持有的句柄。

在 A 机器（mattn 构建）导出的 `.db` 拷到 B 机器（modernc 构建）可能直接打不开；运行中复制还可能拿到不一致的快照。更糟的是，这类故障在开发机上往往复现不了。

## 决定

**导入导出只搬数据，不搬文件**：

- 导出：用独立连接（只执行 SELECT，不写库）读取运行中的库，按 `sqlite_master` 的业务表清单逐表写成 CSV（含表头），打包为 zip。绝不复制 `.db`。
- 导入：解压 → 校验表名白名单 → 关闭 ent 连接 → 独立连接上 `PRAGMA foreign_keys=OFF` → 单事务清空并逐表 `INSERT`（保留原 ID）→ 提交后 `PRAGMA foreign_keys=ON`。
- 目标库始终由**当前运行的驱动**写入，所以格式永远与当前进程兼容。
- `httplog.db` 不参与迁移（调试用请求日志，独立库）。
- 导入后必须重启应用（ent 连接已关闭，`db.Init` 只在启动时执行）。

## 后果

正面：

- 跨驱动、跨平台、跨版本迁移都能用；备份是可读的 CSV，排障时能直接看。
- 导入是单事务：失败整体回滚，不会留下「清空了没导入」的半截状态。
- 表清单来自 `sqlite_master` 或备份文件名 + 白名单校验，新表自动纳入，也不给 SQL 注入留口子。
- 时间列按 `PRAGMA table_info` 判断类型，往返不丢精度。

负面 / 约束：

- 实现比复制文件复杂得多（CSV 转义、NULL 哨兵、时间解析、自增序列重置、外键开关、进度上报）。
- 导出是**明文**：私钥与云凭证都在 CSV 里，备份文件必须当密钥保管（见 [operations/security.md](../operations/security.md)）。
- 需要重启应用才生效。
- 备份不含 `settings.json` / `seeded.json` / 日志 / `httplog.db`。

## 历史修正：`orderedTables` 从未存在

早期 `AGENTS.md` 与 `docs/handoff.md` 都写着「新增 ent 表必须同步更新 `orderedTables` 切片」。2026-10-27 核对后确认：**这个切片从未进入已提交的代码**，两处规则已按实现改正。

- `internal/data/backup.go` 从首个提交 `a83450d` 起就是动态取表：导出用 `sqlite_master` 清单，导入用备份里的 CSV 文件名 + `businessTableSet` 白名单。
- `git log -S orderedTables -- internal/data/backup.go` 没有命中；该字符串只出现在 `internal/data/backup_test.go` 的一句历史注释中。
- 现行实现靠 `PRAGMA foreign_keys=OFF` 让清空顺序无关，因此**不需要拓扑排序**，新增表也不用改代码。

所以本 ADR 的实质约束（禁止复制 `.db`、走 SQLite 搬运、`httplog.db` 不迁移、导入用事务 + 外键开关）仍然有效，现行行为由 `internal/data/backup_test.go` 锁定。新增 ent 表时要做的是：让导出/导入测试覆盖新表，并确认 `businessTableSet` 白名单认得它。

## 备选方案

- **复制 `.db`**：跨驱动不可靠，运行中复制还可能损坏，已否决。
- **`VACUUM INTO` / SQLite Backup API**（未采用）：能拿到一致快照，但产出的仍是驱动相关的库文件，跨驱动问题不变。
- **`ATTACH` + `INSERT SELECT`**：AGENTS.md 把它与 CSV 并列为允许的实现方式；当前实现选 CSV，因为备份可读、可人工检查，也不依赖源库能被当前驱动打开。

## 证据

- `internal/data/backup.go`：导出 / 导入、白名单、事务、外键开关
- `internal/data/backup_test.go`：导出清单、注入防护、回滚、时间列往返
- `AGENTS.md` / `handoff.md`：数据管理硬约束（`orderedTables` 部分已于 2026-10-27 按实现修正）
- `git log -S orderedTables -- internal/data/backup.go`：无命中，说明该切片从未进入已提交的实现
