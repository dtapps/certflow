# 数据模型与存储

本文讲清三件事：数据存在哪、表之间怎么连、迁移与备份怎么保证不丢。部署与监控的业务含义见 [guides/deployment.md](guides/deployment.md)、[guides/monitoring.md](guides/monitoring.md)。

## 1. 数据目录

所有持久化数据都在用户主目录下的固定目录（`main.go`）：

| 版本                                | 目录              |
|-------------------------------------|-------------------|
| 正式版（注入了 VERSION）            | `~/.certflow`     |
| 开发版（`currentVersion == "dev"`） | `~/.certflow.dev` |

两者完全隔离，`make dev` 不会污染正式数据。

```
~/.certflow/
├── settings.json         # 应用设置（viper，含代理密码、日志级别、DNS 配置）
├── seeded.json           # 内置 CA 是否已种入（settings 包独立 viper 实例）
├── data/
│   ├── certflow.db       # 业务库（ent，15 张表）
│   ├── certflow.db-wal   # WAL 日志
│   ├── certflow.db-shm
│   ├── httplog.db        # HTTP 请求日志库（sqlc，单表 http_log）
│   └── httplog.db-wal
└── logs/
    ├── certflow.log      # 主日志（轮转 + gzip）
    ├── frontend.log      # 前端运行时错误
    ├── ent.log           # 仅 DEBUG 级别下的 SQL 日志
    └── gocron.log        # 定时任务日志
```

目录用 `os.MkdirAll(..., 0755)` 创建；日志文件显式 `0644`，数据库与 `settings.json` 按进程默认权限创建——**没有加密，也没有 `0600` 收紧**，见 [operations/security.md](operations/security.md)。

## 2. 实体总览

Schema 定义在 `internal/ent/schema/`，代码由 `make ent` 生成到 `internal/ent/`（生成物不计覆盖率）。共 15 个实体：

| 实体                | 表名                  | 职责                            | 关键字段                                                                                                           |
|---------------------|-----------------------|---------------------------------|--------------------------------------------------------------------------------------------------------------------|
| `CA`                | `cas`                 | ACME 证书颁发机构               | `name`、`directory_url`、`account_email`、`eab_kid`/`eab_hmac`、`is_active`、`is_builtin`（不可变）                |
| `Certificate`       | `certificates`        | 已申请 / 已签发的证书           | `domain`、`sans[]`、`cert_content`、`key_content`、`not_after`、`status`、`auto_renew`、`renewal_days`、`key_type` |
| `CertUpload`        | `cert_uploads`        | 证书 → 云厂商证书 ID 的去重映射 | `provider`、`access_key_id`、`region`、`cert_fingerprint`、`cloud_cert_id`                                         |
| `DNSProvider`       | `dns_providers`       | DNS 提供商凭证                  | `name`、`provider_type`、`config`、`is_active`                                                                     |
| `DeployTarget`      | `deploy_targets`      | 部署目标（云服务 / 面板站点）   | `name`、`provider_type`、`deploy_service`、`config`、`credential_source`、`last_status`                            |
| `DeployCredential`  | `deploy_credentials`  | 独立的部署凭证                  | `name`、`provider_type`、`config`、`is_active`                                                                     |
| `DeployLog`         | `deploy_logs`         | 每次部署尝试                    | `cert_id`、`cert_domain`、`deploy_domain`、`success`、`message`、`cloud_cert_id`                                   |
| `MonitoredDomain`   | `monitored_domains`   | 被监控域名与最近一次快照        | `domain`（唯一）、`check_type`、`check_interval`、`enabled`、`status`、`cert_not_after`                            |
| `MonitorCheckLog`   | `monitor_check_logs`  | 监控历史（趋势图数据源）        | `checked_at`、`status`、`cert_remaining_days`、`response_time_ms`、`http_status_code`                              |
| `Notification`      | `notifications`       | 站内通知                        | `title`、`body`、`category`、`level`、`read`                                                                       |
| `RenewalLog`        | `renewal_logs`        | 续期尝试                        | `status`、`error_message`、`attempt_at`、`completed_at`                                                            |
| `ScanResult`        | `scan_results`        | 主动扫描结果                    | `domain`、`port`、`scan_type`、`scanned_at`、`cert_issuer`、`cert_remaining_days`                                  |
| `AuthMethod`        | `auth_methods`        | 认证方式（是否激活）            | `method`（password/totp/passkey）、`is_active`、`password_hash`（bcrypt）                                          |
| `TOTPCredential`    | `totp_credentials`    | TOTP 密钥                       | `secret`（Base32）、`issuer`、`account_name`、`auth_method_id`                                                     |
| `PasskeyCredential` | `passkey_credentials` | WebAuthn 凭据                   | `credential_id`、`public_key`、`sign_count`、`auth_method_id`                                                      |

其余字段（`issuer`、`not_before`、`last_error`、`last_renewed_at`、扫描的 SAN / 签名算法 / 序列号、监控响应码等）以 `internal/ent/schema/` 为准，本文只列定位与排障最常用的。

主键统一是自增 `id`（ent 默认 `TypeInt`）。`CertUpload` 上有唯一索引 `(provider, access_key_id, region, cert_fingerprint)`，用于跨进程去重。

## 3. 关系

```
cas 1 ──< certificates >── 1 dns_providers
                 │ 1
                 ├──< renewal_logs
                 └── 1 deploy_targets（见下方说明）

deploy_credentials 1 ──< deploy_targets
dns_providers      1 ──< deploy_targets
deploy_targets     1 ──< deploy_logs

monitored_domains 1 ──< monitor_check_logs

auth_methods 1 ──< totp_credentials
auth_methods 1 ──< passkey_credentials
```

外键列名规则是「反向边名 + 表名」，实际库里的列名是：

| 列                                 | 所在表                | 指向                    | 删除行为   |
|------------------------------------|-----------------------|-------------------------|------------|
| `ca_certificates`                  | `certificates`        | `cas.id`                | `SET NULL` |
| `dns_provider_certificates`        | `certificates`        | `dns_providers.id`      | `SET NULL` |
| `deploy_target_certificates`       | `certificates`        | `deploy_targets.id`     | `SET NULL` |
| `dns_provider_deploy_targets`      | `deploy_targets`      | `dns_providers.id`      | —          |
| `deploy_credential_deploy_targets` | `deploy_targets`      | `deploy_credentials.id` | —          |
| `deploy_target_deploy_logs`        | `deploy_logs`         | `deploy_targets.id`     | —          |
| `monitored_domain_check_logs`      | `monitor_check_logs`  | `monitored_domains.id`  | —          |
| `auth_method_id`                   | `totp_credentials`    | `auth_methods.id`       | —          |
| `auth_method_id`                   | `passkey_credentials` | `auth_methods.id`       | —          |

**一处需要注意的实现差异**：`internal/ent/schema/deploy_target.go` 的注释写「关联证书（多对多）：一个目标可部署多个证书，一个证书可部署到多个目标」，但 `Certificate` 侧没有声明反向边，ent 生成的是 `certificates.deploy_target_certificates` 这个**可空外键**（多对一）。也就是说，库层面一张证书只能指向一个部署目标；`LinkCert` 调用 `AddCertificateIDs` 会把证书的外键改指到新目标，而不是新增一条关联。写代码或写测试时不要按多对多假设。

## 4. 枚举与 config JSON

三种配置结构体都在 `internal/ent/schema/types.go`，以 `field.JSON` 存进 SQLite：

- `DNSProviderConfig`：所有 DNS 厂商字段的并集。
- `DeployCredentialConfig`：云厂商 AK/SK、面板 APIKey/Token、区域等。
- `DeployTargetConfig`：`region`/`region_id`、`domains[]`、`cert_name`、`zone_id`/`zone_name`、`site_id[]`/`site_name[]`、`accelerator_id`/`listener_id`。

字段加 `secret:"true"` 只是**标记**，`internal/config.StripSecrets` 会通过反射把它剔除；但它不是加密，也不改变落库内容。当前生产代码里没有调用 `StripSecrets` 的位置（只在 `internal/config/config_test.go` 里验证），所以不能把它当成「密钥不会到达前端」的保证。

枚举值由 `schema` 里的切片生成，改动后必须 `make ent`：

- DNS：`cloudflare, aliyun, tencentcloud, huawei, aws, googlecloud, baiducloud, jdcloud, volcengine, edgeone, aliesa, ucloud, westcn, com35, rainyun, todaynic, dnsla, dns51, xinnet`
- 部署 / 凭证：`aliyun, tencentcloud, huawei, baiducloud, ctyun, volcengine, btpanel, 1panel, acepanel, aapanel, openrestymanager, uuwaf, aawaf, safeline`
- 证书状态：`pending, active, expired, revoked, failed`；密钥类型：`EC256, EC384, RSA2048, RSA3072, RSA4096, RSA8192`

> DNS 枚举当前只覆盖 19 家，而前端表单与 `internal/dnsprovider.GetProviderTypes()` / `certificate` 的 `Parse` 已支持 32 家（含 `porkbun`、`namecheap`、`godaddy` 等）。新增厂商时枚举、`GetProviderTypes`、前端 schema 要一起改，否则 ent 的枚举校验会在写库时拒绝。详见 [guides/dns-providers.md](guides/dns-providers.md)。

## 5. 迁移

### 5.1 自动迁移

`db.Init` 调用 `ent.Open` 后执行 `client.Schema.Create(ctx)`（`internal/db/db.go`）。这是 ent 的增量迁移：建表、加列、加索引，不会删列。升级后首次启动即完成，无需手写 SQL。

### 5.2 一次性数据修复

`internal/db/migrate.go` 的 `MigrateConfigDoubleEncoding` 在每次启动、业务读取前执行：把历史上被双重编码的 `deploy_targets.config` 字段（形如 `"domains":"[\"a\",\"b\"]"`）重写成真实 JSON 数组。函数幂等，已是数组的字段不动；用独立 `sql.Open("sqlite3", sqlite.BuildDSN(path))` 连接执行，不用 ent。

### 5.3 请求日志库迁移

`internal/httplog/` 的建表与历史兼容分开：

- `schema.sql`：建表 DDL，同时是 sqlc 类型推导的输入（`//go:embed`）。
- `migration.sql`：追加列 / 索引的兼容脚本。
- 通过 `make sqlc` 生成 `internal/httplog/db/`，不要手改生成物。

## 6. SQLite 双驱动

`internal/sqlite` 用**构建标签**切换驱动，两个文件提供同名 `BuildDSN`：

| 构建条件                               | 驱动                          | DSN 形态                                                                                  |
|----------------------------------------|-------------------------------|-------------------------------------------------------------------------------------------|
| 默认（无标签）                         | `modernc.org/sqlite`（纯 Go） | `file:<path>?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout=5000` |
| `-tags sqlite_mattn` + `CGO_ENABLED=1` | `github.com/mattn/go-sqlite3` | `file:<path>?_foreign_keys=on&_journal_mode=WAL&_busy_timeout=5000`                       |

两者都把驱动注册成 `sqlite3`，所以业务代码、`httplog`、测试、备份全部用同一个驱动名。两套参数语法互不兼容，`internal/sqlite/sqlite_test.go` 用测试钉死了「只允许出现其中一套」，防止运行时悄悄丢掉外键或 WAL。

选择理由与基准数据见 [DEVELOPMENT.md](../DEVELOPMENT.md) 的「SQLite 驱动性能对比」。CI 的「配置 SQLite 驱动」步骤（`release.yml`）是：macOS / Linux 用 mattn，**Windows 一律用 modernc**。也就是说，正式包与 `make dev` 开发库可能由不同驱动写出。决策记录见 [adr/0003-sqlite-dual-driver.md](adr/0003-sqlite-dual-driver.md)。

**双驱动是「禁止复制 .db 文件」的根因**：不同驱动写出的库文件格式不保证互通，把 A 机器（mattn）的库直接拷给 B 机器（modernc）可能打不开。

## 7. 导入导出（`internal/data/backup.go`）

设置页「数据管理」的导出 / 导入**只搬数据，不搬文件**：

### 7.1 导出

1. 用独立连接 `sql.Open("sqlite3", sqlite.BuildDSN(data/certflow.db))` 打开运行中的库（只执行 SELECT，不写库）。
2. `sqlite_master` 取业务表清单：排除 `sqlite_%` 与 `ent_migration_lock`，按名称排序。
3. 每张表 `SELECT *` 写 `表名.csv`（含表头，时间列转 RFC3339，NULL 用哨兵 `\N` 表示）。
4. 按**实际导出的表清单**打 zip（`certflow-backup.zip`），避免把目录里其它 CSV 混进来。
5. 原生保存对话框选路径；取消返回空字符串且不报错。

### 7.2 导入

1. 原生打开对话框选 zip；解压到临时目录。
2. 从 CSV 文件名推断待导入表；空备份直接报错。
3. 后台 goroutine 执行（前端轮询 `GetImportStatus` 显示进度）：先 `db.Close()` 释放 ent 连接，再开独立连接。
4. `businessTableSet` 校验每个表名必须是本库真实业务表（防注入），随后 `PRAGMA foreign_keys=OFF`。
5. 单事务内：按 CSV 表名 `DELETE` 各表 → 清 `sqlite_sequence` 对应行 → 逐表按表头 `INSERT`（保留原 ID）。
6. 提交后 `PRAGMA foreign_keys=ON`；失败则 ROLLBACK，数据保持导入前状态。
7. 时间列按 `PRAGMA table_info` 判断类型，用 `parseTime` 解析回 `time.Time` 再插入，避免 ent 读取时 Scan 失败。
8. 导入完成后必须**重启应用**：ent 连接已在第 3 步关闭，`db.Init` 只在启动时执行。前端提示后调用 `SystemService.Quit`。

### 7.3 硬约束

以下几条是项目级约束，改 `internal/data` 前先看 `internal/data/backup_test.go`：

- **禁止用「复制 `.db` 文件」实现导入导出**（双驱动格式不互通，见上节）。
- **不要调 `ent.Client.DB()`**：ent v0.14 已移除该方法，底层库统一走 `sql.Open`。
- **导入用事务 + 外键开关**，失败必须整体回滚。
- **`httplog.db` 不参与迁移**：它是调试用请求日志，不随业务数据导出。
- **表名只来自 `sqlite_master` 或备份文件名 + 白名单校验**，不接受外部任意表名拼 SQL。
- **导出包含私钥**：`certificates.key_content` 与 `renewal_logs.key_content` 都是明文 PEM，备份 zip 必须当作密钥文件保管。

> **历史修正（2026-10-27）**：早期 `AGENTS.md` / `handoff.md` 曾要求「新增 ent 表必须同步更新 `orderedTables` 切片」，但代码里从未有过该切片——`internal/data/backup.go` 从首个提交（`a83450d`）起就是「表清单从 `sqlite_master` 动态读取 + `foreign_keys=OFF` 使清空顺序无关」，`git log -S orderedTables -- internal/data/backup.go` 无命中。两处规则已按实现改正：新增 ent 表不需要改手写拓扑，但导出/导入测试与表名白名单必须覆盖新表。其余约束（禁止复制 `.db`、走 SQLite 搬运、`httplog.db` 不迁移）不变。

## 8. `httplog.db`

- 独立库 `data/httplog.db`，单表 `http_log`，sqlc 实现。
- 只在**日志级别 DEBUG** 时包裹 `http.DefaultTransport` / `DefaultClient` 并记录；其它级别完全透传，避免性能损耗。
- 常驻连接只做 INSERT；`Cleanup` 用保存的 DSN 临时开连接执行 DELETE 后关闭。
- 保留天数取设置 `http_log_retention_days`（默认 30，`<=0` 表示不清理）；`main.go` 启动时先清一次，之后每 24 小时一次。
- 记录内容可能包含请求头/响应体，排查完建议把级别调回 INFO 并清理。

## 9. 日常操作

| 场景             | 做法                                                                                     |
|------------------|------------------------------------------------------------------------------------------|
| 常规备份         | 设置 → 数据管理 → 导出，得到 `certflow-backup.zip`；异地保存时按密钥强度保护             |
| 迁移到另一台机器 | 目标机安装同版本 → 导入备份 → 按提示退出并重新打开                                       |
| 手工看库         | 应用退出后用 `sqlite3 ~/.certflow/data/certflow.db`（WAL 模式下会看到 `-wal`/`-shm`）    |
| 手工拷库         | 只在同驱动、同版本、应用完全退出时可行；跨驱动/跨版本请用导出导入                        |
| 换驱动构建       | `CGO_ENABLED=1 go build -tags 'production sqlite_mattn'`，首次启动同样跑自动迁移         |
| 数据异常恢复     | 保留 `logs/` 与旧备份；`settings.json` 与 `seeded.json` 不包含在业务备份里，需要单独留存 |

## 10. 相关测试

- `internal/db/db_test.go`、`internal/db/migrate_test.go`：初始化、关闭、迁移。
- `internal/data/backup_test.go`：导出清单、白名单、事务回滚、时间列往返、表名注入防护。
- `internal/sqlite/sqlite_test.go`：DSN 形态、驱动可用性、外键与 WAL 生效。
- `internal/ent/schema/types_verify_test.go`：`DeployTargetConfig` 的数组字段可直接（反）序列化。

测试口径与运行方式见 [development/testing.md](development/testing.md)。
