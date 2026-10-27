# 架构

本文描述 CertFlow 的运行结构：进程里有什么、谁调用谁、数据从哪来到哪去。操作步骤见 [guides/](guides/getting-started.md)，表结构见 [database.md](database.md)。

## 1. 总体形态

CertFlow 是**单进程桌面应用**，没有独立后端服务：

- Go 主进程负责窗口、托盘、数据库、ACME、云厂商调用、定时任务。
- 界面是 WebView 里的 Vue 3 SPA，构建产物 `frontend/dist` 通过 `//go:embed all:frontend/dist` 打进二进制（`main.go`）。自更新窗口模板 `updater_window.html` 同样嵌入。
- 前后端通过 Wails v3 的绑定通信：Go 侧导出方法，`wails3 generate bindings` 生成 TypeScript 到 `frontend/bindings/`，前端以普通 `async` 函数调用。
- 单实例：`application.SingleInstanceOptions{UniqueID: "net.dtapp.certflow"}`，重复启动会唤醒并前置已有窗口。
- 关窗不退出应用：`MacOptions.ApplicationShouldTerminateAfterLastWindowClosed = false`，驻留系统托盘。

构建期用 `-ldflags -X` 注入四个变量（`main.go`）：

| 变量                       | 用途                                                  |
|----------------------------|-------------------------------------------------------|
| `currentVersion`           | 版本号；值为 `dev` 时走开发数据目录并跳过启动检查更新 |
| `buildTime`                | 构建时间（UTC RFC3339），用于更新比对与「关于」       |
| `gitCommit`                | 短提交 ID，用于 nightly 比对与「关于」                |
| `githubToken` / `cnbToken` | 更新源鉴权（CI 注入，本地为空）                       |

## 2. 分层

```
frontend/src/views, components        Vue 页面与组件（只用 t() 取文案）
        │
frontend/src/stores                   Pinia：i18n / theme / notifications / actionBar
        │
frontend/bindings                     wails3 生成的 TS 绑定（勿手改）
        │  Promise RPC
根目录 *_service.go / *_service_wrapper.go   Wails 服务包装层
        │
internal/<domain>                     领域服务（业务逻辑、ent、SDK、ACME）
        │
internal/ent, database/sql            SQLite：certflow.db / httplog.db
```

包装层（wrapper）存在的理由，是 Wails 只导出「结构体的导出方法」，而领域服务返回 ent 实体、接受内部类型：

- 把 ent 实体转成带 `json` tag 的 DTO，时间统一格式化为 RFC3339 字符串；
- 用 `s.app.Context()` 作为 RPC 的 context（应用生命周期），而不是 `context.Background()`；
- 在边界处写 i18n 日志，并把领域错误原样抛给前端；
- 通过 `SetApp(app)` 注入 `*application.App`，因为 `application.New(Options)` 要求 Service 在 Options 之前构造，而 app 在 Options 之后才存在（`main.go` 注释里写明）。

约定：**业务规则不进包装层**。包装层只做类型转换、日志和生命周期转发。

## 3. main.go 启动装配

顺序在 `main()` 里是硬编码的，改动顺序会破坏依赖关系（尤其是 i18n 与日志）：

1. **数据目录**：`os.UserHomeDir()` + `.certflow`；`currentVersion == "dev"` 时用 `.certflow.dev`，与正式数据隔离。
2. **设置**：`settings.NewService(dataDir)` 读取/创建 `settings.json`，并启动 viper + fsnotify 监听；`seeded.json` 由独立 viper 实例管理。
3. **i18n**：`i18n.SetLocale(settings.Language)`；注册 `settingsService.OnChange`，运行时切语言会同步后端翻译。
4. **日志**：`logging.InitGlobalLogger(<dataDir>/logs, ...)`（`certflow.log`）+ `logging.InitFrontendLogger`（`frontend.log`）；先注册 `recover` 兜底，再 `defer logging.Global().Close()`。
5. **请求日志库**：`httplog.Init(dataDir)`（`data/httplog.db`，仅 DEBUG 生效）+ 24 小时清理 goroutine（保留天数取 `HttpLogRetentionDays`）。
6. **业务库**：`db.Init(dataDir)` → `data/certflow.db`，ent 打开、`Schema.Create` 自动迁移、`MigrateConfigDoubleEncoding` 规整历史 config；`defer db.Close()`。
7. **领域服务**：依次构造 ca / dnsprovider / certificate / notification / deploy / deploycredential / scheduler / monitor / scanner / auth，并注入设置提供者、通知服务、数据目录。
8. **内置 CA 种子**：`caService.SeedDefaults`（仅首次启动，按 `directory_url` 幂等）。
9. **包装层**：20 个根目录 Service 文件 + `data.NewService(dataDir)`。
10. **创建应用**：`application.New`，注册 21 个 Service、`Assets`（嵌入前端）、单实例、Mac 行为。
11. **注入 app 引用**：逐个 `SetApp`；同时给 monitor 注入 `monitorEventEmitter`（把后台检查结果转成前端事件）。
12. **更新器**：`network.BuildHTTPClient(settings)` 复用全局代理 / 自定义 DNS / User-Agent；`newMirrorProvider(...)` 按语言选 GitHub 或 CNB；`app.Updater.Init` 挂内置窗口。
13. **主窗口**：1280×800、无边框、透明背景、`URL: "/"`；`WebViewDidFinishNavigation` 后 `Show` + `Focus`。
14. **窗口与托盘引用**：`windowSvc.setMainWindow`、`systraySvc.setMainWindow`、`systemSvc.setMainWindow`。
15. **应用菜单**：App 子菜单（设置 / 检查更新 / 申请证书 / 扫描 / 退出）、`application.EditMenu`（macOS 快捷键依赖它）、帮助 → 关于。
16. **退出与事件**：`ctx` + goroutine 在取消时停调度器；`EventUpdateReady` 触发 `cleanup()`（停调度器、停监控、取消 ctx，但**不关数据库**）；系统主题变化转发给前端。
17. **启动检查更新**：仅非 `dev` 版本，延迟 1–3 分钟随机后 `app.Updater.Check`，有更新发通知。
18. **`app.Run()`**：此时各 Service 的 `ServiceStartup` 才执行（调度器与监控循环在这里启动）。

## 4. 服务包装层清单

根目录共 20 个 Service 文件：18 个 `*_service.go` + 2 个 `*_service_wrapper.go`；加上 `internal/data.Service`，共 **21 个** 注册进 `application.Options.Services`。

| 文件                              | 绑定的 ServiceName      | 领域实现                     | 职责                                       |
|-----------------------------------|-------------------------|------------------------------|--------------------------------------------|
| `auth_service.go`                 | AuthService             | `internal/auth`              | 口令 / TOTP / Passkey 的设置、验证、切换   |
| `autostart_service.go`            | AutostartService        | Wails `app.AutostartManager` | 开机自启开关                               |
| `browser_service.go`              | BrowserService          | Wails `app.Browser`          | 打开外部链接（发布页等）                   |
| `ca_service.go`                   | CAService               | `internal/ca`                | CA 增删改查、启用、测试 ACME 目录          |
| `cert_service.go`                 | CertificateService      | `internal/certificate`       | 申请、续期、吊销、删除、手动 DNS、证书解析 |
| `clipboard_service.go`            | ClipboardService        | Wails `app.Clipboard`        | 写剪贴板                                   |
| `deploy_credential_service.go`    | DeployCredentialService | `internal/deploycredential`  | 部署凭证增删改查、启用                     |
| `deploy_service.go`               | DeployService           | `internal/deploy`            | 部署目标 CRUD、执行部署、日志、当前证书    |
| `dns_service.go`                  | DNSProviderService      | `internal/dnsprovider`       | DNS 提供商增删改查、启用、类型列表         |
| `dock_service.go`                 | DockService             | Wails `app.Dock`             | macOS Dock 图标 / 徽标                     |
| `file_service.go`                 | FileService             | Wails `app`                  | 打开目录（日志目录等）                     |
| `logging_service_wrapper.go`      | LoggingService          | `internal/logging`           | 日志文件列表 / 读取 / 级别                 |
| `monitor_service.go`              | MonitorService          | `internal/monitor`           | 监控域名 CRUD、立即检查、历史              |
| `notification_service_wrapper.go` | NotificationService     | `internal/notification`      | 桌面通知、站内通知列表与已读               |
| `scanner_service.go`              | ScannerService          | `internal/scanner`           | 证书扫描、历史、清空                       |
| `scheduler_service.go`            | SchedulerService        | `internal/scheduler`         | 续期日志、手动触发续期 / 过期检查          |
| `settings_service.go`             | SettingsService         | `internal/settings`          | 读写 `settings.json`                       |
| `system_service.go`               | SystemService           | Wails env / dialog           | 深色模式、环境信息、版本、UA、前端错误落盘 |
| `systray_service.go`              | SysTrayService          | Wails tray                   | 托盘创建、菜单、显示 / 隐藏窗口            |
| `window_service.go`               | WindowService           | Wails windows                | 窗口尺寸、HTML 窗口                        |
| `internal/data/backup.go`         | 无（Wails 按类型注册）  | `internal/data`              | 业务库导出 / 导入、导入进度                |

`ServiceStartup` / `ServiceShutdown` 有实际行为的只有三个：调度器（`scheduler.Start/Stop`）、监控（`monitor.Start/Stop`）、托盘（菜单初始化）。其余包装层是空实现（仅满足接口）；`internal/data.Service` 不实现这两个方法，只提供导出 / 导入绑定。

## 5. `internal/` 包职责

| 包                   | 职责                                                                   |
|----------------------|------------------------------------------------------------------------|
| `auth`               | bcrypt 口令、TOTP（pquerna/otp）、Passkey（go-webauthn）、认证方式激活 |
| `biometric/binaries` | macOS / Windows 原生 helper 二进制；当前 Go 代码无调用点，不在认证链路 |
| `ca`                 | CA 记录、内置 CA 种子、ACME 目录连通性测试                             |
| `certificate`        | lego ACME 申请 / 续期 / 吊销、密钥生成、DNS-01、手动 DNS、证书解析     |
| `cloudcred`          | 云凭证中性类型，避免 `deploy` 与解析包循环依赖                         |
| `config`             | 泛型 JSON 配置解析、`AsMap`、`StripSecrets`（反射读 `secret:"true"`）  |
| `data`               | 业务库按表导出 CSV→zip、导入、导入进度                                 |
| `db`                 | 打开 ent、自动迁移、一次性 config 双重编码修复                         |
| `deploy`             | 云厂商 / 面板 / WAF 部署器注册表、部署、去重、当前证书查询             |
| `deploycredential`   | 部署凭证存储与查询                                                     |
| `dnsprovider`        | DNS 提供商存储、配置解析（`Parse` 到强类型）                           |
| `ent`                | ent 生成代码 + `schema/` 实体定义（生成物，不计覆盖率）                |
| `events`             | 前后端事件名常量与 payload 结构体                                      |
| `httplog`            | 出站 HTTP 请求日志（sqlc + 独立 `httplog.db`）                         |
| `i18n`               | 嵌入式翻译、`T()`、带 key 的错误信封                                   |
| `logging`            | `certflow.log` / `frontend.log` / 轮转 / 读取                          |
| `monitor`            | 域名健康检查、证书信息采集、历史与通知                                 |
| `network`            | 按设置构建 HTTP 客户端（代理 / 自定义 DNS）、客户端缓存指纹            |
| `notification`       | 桌面通知 + 站内通知落库 + 事件                                         |
| `scanner`            | 单次证书扫描与历史                                                     |
| `scheduler`          | gocron 调度：自动续期、过期检查、续期日志                              |
| `settings`           | `settings.json`、默认值、内置 DNS 列表、文件监听                       |
| `sqlite`             | `BuildDSN` 与双驱动注册（构建标签切换）                                |
| `sqlitetest`         | 驱动基准测试                                                           |
| `useragent`          | 全局 User-Agent（前端启动时写入）                                      |

## 6. 事件

事件名常量在 `internal/events`，前端镜像在 `frontend/src/utils/events.ts`。`main.go` 的 `init()` 为 8 个事件调用 `application.RegisterEvent`，payload 类型必须在编译期登记。

| 事件              | 发送点                                                    | 前端消费                  |
|-------------------|-----------------------------------------------------------|---------------------------|
| `time`            | 无（已注册，保留）                                        | 无                        |
| `notification`    | `internal/notification`、`main.go` 启动检查更新           | `stores/notifications.ts` |
| `auth_verified`   | 无（已注册，登录由登录框直接回调）                        | `App.vue`（监听）         |
| `theme_changed`   | `main.go` 监听 `Common.ThemeChanged`                      | `stores/theme.ts`         |
| `locale_changed`  | 无（语言经 settings 落盘 + 后端 OnChange 同步）           | 无                        |
| `navigate`        | `main.go` 应用菜单、`systray_service.go` 托盘菜单         | `App.vue`（路由跳转）     |
| `window_resized`  | `main.go` 主窗口 `WindowDidResize`                        | `views/DeployTargets.vue` |
| `monitor_updated` | `main.go` 的 `monitorEventEmitter`（后台 / 手动检查完成） | `views/Monitor.vue`       |

一个容易踩的坑：Wails v3 的 `app.Event.Emit` 返回的是 **事件是否被取消**，不是是否成功。代码里只有在返回 `true` 时才告警（`main.go` 注释与 `monitorEventEmitter` 均如此），不要写成 `if !ok { 报错 }`。

## 7. 数据流

### 7.1 申请 → 签发

```
CertApply.vue → CertificateService.ApplyCertificate
  → internal/certificate.ApplyCertificate
      CA 校验（邮箱必填）
      生成账户密钥 + 证书密钥（key_type）
      lego 配置：CADirURL + network.BuildHTTPClient（代理 / 自定义 DNS）
      dns01 默认客户端 RecursiveNameservers 取启用的 DNS 配置
      注册 ACME 账户（有 EAB 走 EAB，失败回退 ResolveAccountByKey）
      分支 A：选了 DNS 提供商 → createDNSProvider → SetDNS01Provider
      分支 B：未选 → StartManualDNSChallenge（返回 TXT 记录，success=false）
      落库 certificates（pending）→ Obtain → 解析 x509
      落库（active / cert_content / key_content / not_after）→ 通知
```

手动 DNS 是两段式：第一次调用返回 `challenge_records`，前端让用户加 TXT；`CompleteManualDNSChallenge(domain)` 再做实际签发，等待策略是每 10 秒轮询 TXT、最长 10 分钟（`internal/certificate/manual_dns.go`）。

### 7.2 部署

```
CertDetail / DeployTargets → DeployService.DeployCertificate(targetID, certID, domain, siteID)
  → internal/deploy.DeployService
      loadCredsAndConfig：按 credential_source 取 DNS 提供商凭证或部署凭证
      Deployer.UploadCert：按「厂商 + AccessKeyId + 区域 + 证书指纹」去重
        （内存缓存 + cert_uploads 表，命中则复用 cloud_cert_id）
      Deployer.DeployCert：绑定到 CDN / WAF / ELB / 面板站点
      写 deploy_logs、更新 deploy_targets.last_status / last_error
```

`DeployAllForCert` 是「一张证书部署到所有关联目标」，返回每个目标的结果。当前证书对比走 `GetCurrentCerts`：部署器实现可选的 `currentCertGetter` 接口；未实现时返回 `Supported=false`，前端显示「暂不支持」。云 API 查询有全局限速：并发 5、5 QPS（`internal/deploy/service.go` 常量）。

### 7.3 续期

```
gocron（RenewInterval 小时）→ scheduler.runAutoRenew
  全局开关 AutoRenewEnabled（手动触发 force 忽略）
  ListAutoRenew：active + auto_renew + 已关联 DNS 提供商 + not_after <= now + renewal_days
  每张证书：写 renewal_logs(pending) → RenewCertificate → 成功/失败回写 → 通知
```

**续期不会自动重新部署**：`internal/scheduler/service.go` 里没有调用 `DeployService`，证书更新后需要重新执行部署。这是当前实现的边界，别在文档里写成「续期后自动下发」。

### 7.4 监控

```
MonitorService（后台循环，按域名 check_interval）
  https：GET URL → 2xx/3xx ok、4xx warning、5xx error
         解析 TLS 叶子证书；剩余 ≤30 天降为 warning，≤0 天 expired
  http ：同状态规则，不解析证书（除非被重定向到 HTTPS）
  写 monitor_check_logs + 更新 monitored_domains 快照
  状态变化时发通知，并 emit monitor_updated（前端刷新）
  按 MonitorHistoryDays 清理历史
```

### 7.5 扫描

`ScannerService.Scan(domain, port, type)` 主动连接并抓取证书链，写 `scan_results`；不落证书私钥，不影响已管理证书。历史可单条删除或清空。

### 7.6 数据导入导出

`internal/data` 不走「复制 .db」：导出按 `sqlite_master` 清单逐表写 CSV 再打 zip；导入解压、校验表名白名单、`PRAGMA foreign_keys=OFF`、单事务清空并按原 ID 重插。细节与硬约束见 [database.md](database.md) 第 5 节。

## 8. 并发与生命周期要点

- **调度器与监控**在 `ServiceStartup` 启动，在 `ServiceShutdown` 或 updater 的 `cleanup()` 停止；`cleanup()` 故意不关数据库，因为进程还在跑、前端仍会调 RPC。
- **HTTP 客户端**按「代理 + 启用的 DNS」指纹缓存（`internal/network`），设置变更自动重建，避免每次检查都新建 Transport。
- **ent 连接**由 `main` 的 `defer db.Close()` 统一关闭；导入时会先 `db.Close()` 再改库，导入完成后必须重启应用（`SystemService.Quit`）。
- **派发到前端的后台结果**（监控、通知）都走 `app.Event`，不要在 goroutine 里直接调 WebView API。

## 9. 相关文档

- 表与迁移：[database.md](database.md)
- 使用与操作：[guides/getting-started.md](guides/getting-started.md)
- 测试与发布：[development/testing.md](development/testing.md)、[development/release.md](development/release.md)
- 安全事实：[operations/security.md](operations/security.md)
