# ADR-0002：业务库用 ent，请求日志库用 sqlc

- 状态：已采纳
- 日期：2026-07 至 2026-08（随数据层建立）
- 相关：[database.md](../database.md)

## 背景

项目有两类完全不同的数据访问需求：

1. **业务库**（`certflow.db`）：15 个实体、多组一对多关系、状态枚举、JSON 配置、需要自动迁移。查询以「按条件取实体 + 走关联」为主。
2. **HTTP 请求日志库**（`httplog.db`）：单表 `http_log`，高频 INSERT、按时间 DELETE、按条件分页查询，没有关系，也不需要迁移框架。

用一套工具硬吃两种负载，要么业务层手写 SQL 太啰嗦，要么日志层背上一整套 ORM 生成物。

## 决定

- **业务库用 ent**（`entgo.io/ent v0.14.6`）：schema 在 `internal/ent/schema/`，`make ent` 生成类型安全的 client；启动时 `client.Schema.Create(ctx)` 自动迁移；关系、枚举、JSON 字段、唯一索引都用 schema 声明。
- **请求日志库用 sqlc**：`internal/httplog/schema.sql`（建表 DDL）+ `query.sql`（INSERT/DELETE）+ `sqlc.yaml`，`make sqlc` 生成 `internal/httplog/db/`；运行时用 `database/sql` 直接读写，不经过 ent。
- 两个库物理分离：`data/certflow.db` 与 `data/httplog.db`，连接、迁移、清理各自独立。

## 后果

正面：

- 业务侧改 schema 只写 Go 结构，迁移与查询代码自动生成，关系查询不用手拼 JOIN。
- 日志侧保持极薄：一条 INSERT、一条 DELETE，SQL 可读，性能路径短。
- 生成物边界清晰：`internal/ent`、`internal/httplog/db` 都是生成代码，覆盖率统计排除它们。
- 日志库出问题不会污染业务库；导出 / 导入也不碰 `httplog.db`。

负面 / 成本：

- 两套生成流程：改 schema 要 `make ent`，改 SQL 要 `make sqlc`，CI 里都要先跑。
- ent v0.14 移除了 `*ent.Client.DB()`，需要底层连接时只能另开 `sql.Open("sqlite3", sqlite.BuildDSN(path))`（导入导出、一次性修复都这么做）。
- ent 的自动迁移只增不删，历史列会留下；复杂数据修复仍要手写 SQL（如 `MigrateConfigDoubleEncoding`）。
- sqlc 的字段类型由 DDL 推导，`schema.sql` 与 `migration.sql` 要同时维护，避免历史库缺列。

## 备选方案

- **全 ent**：日志表也进 schema，会带来无意义的实体 / 关系代码，且日志库与业务库耦合，迁移和清理都变重。
- **全 sqlc / 手写 SQL**：业务层 15 个实体 + 关联 + 枚举校验要大量手写，迁移也要自己维护。
- **GORM**：迁移与关联能力够，但 schema 不是声明式生成，类型安全与「schema 即代码」的体验不如 ent。

## 证据

- `go.mod`：`entgo.io/ent v0.14.6`
- `internal/ent/schema/`：15 个实体定义
- `internal/httplog/{schema.sql,query.sql,sqlc.yaml}` 与 `internal/httplog/db/`
- `Makefile`：`ent`、`sqlc` 目标
- `internal/db/db.go`、`internal/httplog/httplog.go`：两套初始化
