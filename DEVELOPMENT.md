# CertFlow 开发文档

中文 | [English](DEVELOPMENT_EN.md)

## 项目概述

CertFlow 是一个 SSL 证书管理工具，支持证书申请、续期、过期监控等功能。采用 Go + Vue 3 跨平台桌面应用架构。

---

## 技术栈

### 后端

| 技术 | 用途 |
|------|------|
| Go 1.26 | 后端编程语言 |
| Wails v3 | 桌面应用框架（Go + 前端混合） |
| Ent (entgo.io/ent) | ORM 框架（代码生成式） |
| lego v5 (go-acme/lego) | ACME 协议客户端（申请/续期证书） |
| gocron/v2 | 定时任务调度（自动续期、过期检查） |
| modernc.org/sqlite | 嵌入式数据库（纯 Go，无需 CGO） |
| golang.org/x/crypto | 密码学工具（bcrypt 等） |
| go-webauthn/webauthn | Passkey / WebAuthn 认证 |
| pquerna/otp | TOTP 双因素认证（2FA） |
| google/uuid | 唯一标识生成（v7） |
| spf13/viper | 配置管理 |
| fsnotify/fsnotify | 文件系统监听 |

### 前端

| 技术 | 用途 |
|------|------|
| Vue 3 | 渐进式前端框架 |
| vue-router | SPA 路由管理 |
| TypeScript | 类型安全语言 |
| Vite 8 | 构建工具/开发服务器 |
| Tailwind CSS v4 | 原子化 CSS 框架 |
| Naive UI v2 | Vue 3 组件库 |
| Pinia | 状态管理（替代 Vuex） |
| @vicons/ionicons5 | 图标库 |
| @vueuse/core | Vue 组合式 API 工具集 |
| @wailsio/runtime | Wails 前后端通信运行时 |
| qrcode | TOTP 二维码生成 |

### 构建与部署

| 工具 | 用途 |
|------|------|
| Makefile | 开发命令封装 |
| Taskfile.yml | 跨平台构建系统 |
| golangci-lint v2 | Go 代码检查 |
| Prettier | 前端代码格式化 |
| pnpm | 前端包管理器 |
| wails3 CLI | Wails 构建/开发/代码生成 |

---

## 项目结构

```
certflow/
├── main.go                    # 入口文件（版本/构建信息注入、应用装配、自更新）
├── *_service.go               # Wails 服务包装层（每个业务一个文件）
│   ├── system_service.go      # 系统服务（主题/窗口/菜单）
│   ├── cert_service.go        # 证书服务
│   ├── ca_service.go          # CA 管理服务
│   ├── dns_service.go         # DNS 提供商服务
│   ├── auth_service.go        # 认证服务
│   ├── scheduler_service.go   # 定时任务服务
│   ├── monitor_service.go     # 域名监控服务
│   ├── scanner_service.go     # 证书扫描服务
│   ├── deploy_service.go      # 证书部署服务（云厂商 CDN/WAF/LB 等）
│   ├── deploy_credential_service.go # 部署凭证管理服务
│   ├── notification_service_wrapper.go # 通知服务
│   ├── settings_service.go    # 设置服务
│   ├── logging_service_wrapper.go # 日志服务
│   ├── clipboard_service.go   # 剪贴板服务
│   ├── browser_service.go     # 浏览器打开服务
│   ├── file_service.go        # 文件选择服务
│   ├── dock_service.go        # macOS Dock 服务
│   ├── window_service.go      # 窗口服务
│   ├── systray_service.go     # 系统托盘服务
│   └── autostart_service.go   # 开机自启动服务
├── internal/                  # 内部实现
│   ├── ent/                   # Ent ORM 生成的代码（原根目录 ent/ 已移入）
│   │   ├── schema/            # 数据库模型定义
│   │   │   ├── ca.go          # CA 实体
│   │   │   ├── certificate.go # 证书实体
│   │   │   ├── cert_upload.go # 上传证书实体
│   │   │   ├── dns_provider.go # DNS 提供商实体
│   │   │   ├── deploy_target.go # 部署目标实体（云厂商/服务/区域配置）
│   │   │   ├── deploy_credential.go # 部署凭证实体
│   │   │   ├── deploy_log.go  # 部署日志实体
│   │   │   ├── provider_types.go # 厂商/服务类型枚举定义
│   │   │   ├── monitored_domain.go # 监控域名实体
│   │   │   ├── notification.go # 通知实体
│   │   │   ├── renewal_log.go # 续期日志实体
│   │   │   ├── auth_method.go # 认证方式实体
│   │   │   ├── totp_credential.go # TOTP 凭据实体（2FA）
│   │   │   ├── passkey_credential.go # Passkey 凭据实体（WebAuthn）
│   │   │   └── scan_result.go # 扫描结果实体
│   │   └── ...
│   ├── httplog/               # HTTP 请求日志（sqlc 生成，落独立库）
│   ├── auth/                  # 认证服务（口令/TOTP/Passkey）
│   ├── ca/                    # CA 管理
│   ├── certificate/           # 证书申请/续期/撤销/上传
│   ├── db/                    # 数据库初始化
│   ├── deploy/                # 证书部署（各云厂商实现：aliyun/tencent/huawei/baidu/ctyun/volcengine）
│   ├── deploycredential/      # 部署凭证管理
│   ├── dnsprovider/           # DNS 提供商管理
│   ├── events/                # 前后端事件定义
│   ├── i18n/                  # 国际化（嵌入式语言文件）
│   ├── logging/               # 日志系统（轮转/压缩）
│   ├── monitor/               # 域名监控
│   ├── network/               # 网络工具（DNS/代理）
│   ├── notification/          # 通知服务
│   ├── scanner/               # 证书扫描
│   ├── scheduler/             # 定时任务调度
│   └── settings/              # 应用设置
├── frontend/                  # 前端应用
│   ├── src/
│   │   ├── views/             # 页面视图
│   │   │   ├── Dashboard.vue          # 仪表盘（总览/统计）
│   │   │   ├── Certificates.vue        # 证书列表（管理/续期/撤销）
│   │   │   ├── CertApply.vue           # 证书申请（表单/提交）
│   │   │   ├── CertDetail.vue          # 证书详情（查看/导出/部署）
│   │   │   ├── CAConfig.vue            # CA 配置（私有 CA 管理）
│   │   │   ├── Providers.vue           # DNS 提供商入口
│   │   │   ├── DNSProviders.vue        # DNS 提供商（凭证管理/验证）
│   │   │   ├── DeployTargets.vue       # 部署目标列表（管理/执行部署）
│   │   │   ├── DeployTargetForm.vue    # 部署目标新建/编辑表单
│   │   │   ├── DeployCredentials.vue   # 部署凭证入口
│   │   │   ├── CredentialList.vue      # 部署凭证列表（管理）
│   │   │   ├── Monitor.vue             # 域名监控（健康检查/过期预警）
│   │   │   ├── Scan.vue                # 证书扫描（归集/有效期）
│   │   │   ├── Settings.vue            # 设置（应用/更新/关于）
│   │   │   ├── PersonalCenter.vue      # 个人中心（账户/2FA/Passkey）
│   │   │   └── LogViewer.vue           # 日志查看（运行/续期日志）
│   │   ├── components/        # 公共组件
│   │   │   ├── TitleBar.vue   # 自定义标题栏（窗口控制）
│   │   │   ├── Sidebar.vue    # 侧边栏导航
│   │   │   ├── TopBar.vue     # 顶部工具栏
│   │   │   └── LoginDialog.vue # 登录弹窗
│   │   ├── router/            # 路由配置
│   │   ├── stores/            # Pinia 状态管理
│   │   │   ├── i18n.ts        # 国际化
│   │   │   ├── theme.ts       # 主题管理
│   │   │   └── notifications.ts # 通知管理
│   │   ├── utils/             # 工具函数
│   │   ├── style.css          # 全局样式
│   │   └── main.ts            # 入口文件
│   ├── package.json
│   ├── .prettierrc            # Prettier 配置
│   └── vite.config.ts
├── build/                     # 构建配置
│   ├── config.yml             # Wails 应用配置
│   ├── Taskfile.yml           # 公共构建任务
│   ├── darwin/                # macOS 构建/签名
│   ├── windows/               # Windows 构建/签名
│   └── linux/                 # Linux 构建/签名
├── .github/workflows/         # GitHub Actions 工作流
│   ├── release.yml            # 正式发布（六平台）
│   └── nightly.yml            # 每日构建（nightly 预发布）
├── .cnb/workflows/            # CNB 工作流
│   └── release.yml            # 从 GitHub Release 下载产物并发布到 CNB
├── .golangci.yml              # Go 代码检查配置
├── Makefile                   # Make 命令
├── Taskfile.yml               # Task 构建入口
└── go.mod                     # Go 模块定义
```

---

## 开发命令

```bash
make help              # 查看所有命令
make deps              # 安装所有依赖（Go + 前端）
make dev               # 启动 Wails 开发模式
make build             # 构建生产包（make build VERSION=1.0.0）
make package           # 打包应用（make package VERSION=1.0.0）
make go-build          # 快速编译（仅 Go 后端）
make lint-go           # Go 代码检查（golangci-lint）
make lint-go-fix       # Go 代码检查（自动修复）
make lint-frontend     # 前端 TypeScript 类型检查（vue-tsc）
make check             # 代码检查与测试（lint-go + lint-frontend + test-go）
make test              # Go 后端测试
make bindings          # 生成 Wails TypeScript 绑定
make ent               # 生成 Ent ORM 代码
make sqlc              # 生成 sqlc 代码（internal/httplog）
make format            # 格式化所有代码（Go + Vue/TS）
make format-go         # 格式化 Go 代码
make format-frontend   # 格式化前端代码（Prettier）
make clean             # 清理构建产物
make push              # 推送到所有远程仓库
```

---

## 版本号注入

构建时通过各平台 Taskfile 的 `-ldflags -X` 注入版本号、构建时间、Git 提交 ID 与更新令牌（默认值 `dev` / 空字符串），由 `wails3 task build VERSION=x BUILD_TIME=... GIT_COMMIT=... GITHUB_TOKEN=...` 控制：

```bash
# 通过 wails3 task（推荐）
wails3 task build VERSION=1.0.0

# 通过 Makefile
make build VERSION=1.0.0
```

注入的变量（`main.go`）：
- `currentVersion` — 版本号（默认 `dev`）
- `buildTime` — 构建时间（UTC）
- `gitCommit` — Git 短提交 ID
- `githubToken` — 自更新用的 GitHub Token（CI 注入，本地为空）

---

## 跨平台构建

| 平台 | 命令 | 产物 |
|------|------|------|
| macOS | `wails3 task package` | `bin/certflow.app` |
| Windows | `wails3 task build` | `bin/certflow.exe` |
| Linux | `wails3 task build` | `bin/certflow` |

### 发布工作流

- **GitHub Actions（Release）**：手动输入版本号，六个平台并行构建（macOS arm64 / amd64、Linux amd64 / arm64、Windows amd64 / arm64），发布 GitHub Release
- **GitHub Actions（Nightly）**：每日 UTC 00:00 自动构建 `X.Y.Z-nightly` 预发布版本（tag 为 `nightly`）
- **CNB**：手动输入版本号，从 GitHub Release 下载产物并发布

### 代码签名

CI 会在打包完成后对各平台产物进行代码签名。**仅当仓库配置了对应 Secret 时执行，未配置则步骤自动跳过，不影响构建**：

- **macOS**：Apple Developer ID 证书 + 公证（`darwin:sign:notarize`），依赖 `APPLE_DEVELOPER_ID_CERTIFICATE` / `APPLE_DEVELOPER_ID_CERTIFICATE_PASSWORD` / `APPLE_NOTARIZER_KEYCHAIN_PROFILE` 三个 Secret。未签名+公证的应用会被 Gatekeeper 拦截（"无法验证开发者 / 文件已损坏"）
- **Windows**：Authenticode 签名，先签名安装包（`windows:sign:installer`）再签名独立 exe（`windows:sign`），依赖 `WINDOWS_SIGN_CERTIFICATE`（.pfx 的 base64）/ `WINDOWS_SIGN_CERTIFICATE_PASSWORD` / `WINDOWS_SIGN_THUMBPRINT` / `WINDOWS_TIMESTAMP_SERVER` 四个 Secret，用于缓解 SmartScreen 拦截
- **Linux**：操作系统不强制验签，无需代码签名

> 自动更新仅依赖 GitHub Release 的 `SHA256SUMS` 校验和验证（wails 原生支持），不对更新包做 ed25519 签名验签

---

## 数据库

使用 SQLite 作为嵌入式数据库，通过 Ent ORM 进行数据管理。实体模型定义在 `internal/ent/schema/` 目录下：

- **CA** — 证书颁发机构配置
- **Certificate** — SSL 证书信息
- **CertUpload** — 上传的外部证书
- **DNSProvider** — DNS 提供商配置
- **DeployTarget** — 部署目标（厂商/服务/区域）
- **DeployCredential** — 部署凭证
- **DeployLog** — 部署执行日志
- **MonitoredDomain** — 监控域名
- **Notification** — 通知记录
- **RenewalLog** — 证书续期日志
- **ScanResult** — 证书扫描结果

> HTTP 请求日志使用独立的 sqlc 实现的 `internal/httplog/`（单表 `http_log`），落在独立数据库 `dataDir/data/httplog.db`，仅 DEBUG 级别启用。

### SQLite 驱动性能对比

测试环境：Apple M1, Go 1.24+, 内存库 (`:memory:`), `benchtime=3s`。现代码位于 `internal/sqlitetest/`。

#### 耗时对比（ns/op，越小越好）

| 操作 | modernc (纯Go) | mattn (CGO) | mattn 优势 |
|------|:---:|:---:|:---:|
| **Insert** (单行) | 8,603 | 5,095 | 快 1.69x |
| **InsertBulk** (100行/事务) | 382,696 | 194,203 | 快 1.97x |
| **Select** (主键查询) | 5,024 | 3,343 | 快 1.50x |
| **SelectRows** (100行扫描) | 41,587 | 38,949 | 快 1.07x |
| **Update** | 4,848 | 3,065 | 快 1.58x |
| **Delete** | 7,968 | 6,489 | 快 1.23x |
| **QueryLike** (5K行LIKE) | 2,415,319 | 1,785,577 | 快 1.35x |

#### 内存分配对比（B/op | allocs/op）

| 操作 | modernc | mattn |
|------|:---:|:---:|
| **Insert** | 344 / 14 | 400 / 14 |
| **InsertBulk** | 31,886 / 1309 | 30,419 / 1118 |
| **Select** | 735 / 27 | 798 / 30 |
| **SelectRows** | 6,952 / 517 | 7,024 / 520 |
| **Update** | 176 / 7 | 248 / 8 |
| **Delete** | 184 / 9 | 248 / 10 |
| **QueryLike** | 397,722 / 29663 | 397,770 / 29664 |

#### 结论

- **mattn (CGO) 在所有操作上都更快**，写操作优势尤为明显（InsertBulk 快 97%），原因是直接调用 C 语言 SQLite amalgamation，而 modernc 是纯 Go 翻译实现，多了一层模拟开销。
- **内存分配两者基本持平**，modernc 单行操作略优（少 20~30B），mattn 批量写入时少分配 ~5%。
- **大量读操作差距缩小**（SelectRows 仅 1.07x），驱动层开销被 SQL 执行本身稀释。
- **当前默认策略**：默认 modernc（纯Go，零 CGO 依赖，跨平台编译最简单），mattn 通过构建标签 `sqlite_mattn` + `CGO_ENABLED=1` 可选启用；windows/arm64 不支持 mattn，CI 强制回退 modernc。此策略合理，无需调整。

---

## 证书部署

将已签发/上传的证书部署到云厂商的各类服务。实现位于 `internal/deploy/`，按厂商拆分文件（`aliyun.go`、`tencent.go`、`huawei.go`、`baidu.go`、`ctyun.go`、`volcengine.go`），`service.go` 统一分发。

支持的厂商与服务：

| 厂商 | 支持服务 |
|------|----------|
| 阿里云（aliyun） | CDN、全站加速 DCDN、边缘安全加速 ESA、全球加速 GA |
| 腾讯云（tencentcloud） | CDN、边缘安全加速平台 EdgeOne、ECDN |
| 华为云（huawei） | CDN、WAF、负载均衡 ELB |
| 百度云（baiducloud） | CDN、动态加速 DRCDN |
| 天翼云（ctyun） | 内容分发 CTCDN、多云 CDN ICDN、边缘安全加速 AccessOne |
| 火山引擎（volcengine） | CDN、全站加速 DCDN |

- 凭证来源支持两种：独立的**部署凭证**（`internal/deploycredential/`）或**复用 DNS 提供商凭证**（`credential_source`）。
- 前端页面：`DeployTargets.vue`（列表/执行）、`DeployTargetForm.vue`（新建/编辑）、`DeployCredentials.vue` + `CredentialList.vue`（凭证管理）。

---

## HTTP 请求日志

`internal/httplog/` 在 DEBUG 日志级别下，把出站 HTTP 流量记录到独立的 SQLite 库（sqlc 实现，单表 `http_log`）：

- 提供 `WrapTransport(base)` / `WrapClient(client)` 助手，DEBUG 下包裹、否则透传。
- 已注入 `internal/network.BuildHTTPClient` 与各云 SDK 客户端，覆盖 scanner / monitor / 证书申请（lego）/ 证书部署等出站请求。
- 存储实现：`schema.sql`（建表）+ `query.sql`（INSERT/DELETE）+ `sqlc.yaml`（sqlc 配置），经 `make sqlc`（即 `cd internal/httplog && sqlc generate`）生成 `internal/httplog/db/` 包。
- 连接模型：常驻 `*sql.DB` 仅 append-only（只 INSERT）；`Cleanup` 定时删除旧日志时用保存的 DSN 临时开独立连接执行 DELETE 后关闭。

---

## 国际化

- **后端**：`internal/i18n/` 使用 go-i18n，语言文件嵌入二进制（`//go:embed`）
- **前端**：`frontend/src/stores/i18n.ts` 内置中英文（Pinia store）

所有用户可见文本必须使用 `i18n.T()`（Go）或 `t()`（Vue）。

---

## 代码格式化

```bash
make format            # 格式化所有代码
make format-go         # 格式化 Go 代码（gofmt）
make format-frontend   # 格式化前端代码（Prettier）
```

前端代码使用 Prettier 格式化，配置文件位于 `frontend/.prettierrc`。

---

## 代码检查

```bash
make lint-go           # 运行 golangci-lint
make lint-go-fix       # 自动修复
```

启用的检查器：errcheck、govet、staticcheck、ineffassign、misspell、unconvert、nilerr、errorlint、bodyclose、contextcheck、noctx、gosec。

---

## 测试

```bash
make test              # 运行所有测试
```

14 个包均有测试覆盖：auth、ca、certificate、db、deploy、dnsprovider、i18n、logging、monitor、network、notification、scanner、scheduler、settings。

---

## 调试技巧

### 标题栏平台调试

标题栏根据平台自动适配 macOS / Windows / Linux 样式。开发时可通过 localStorage 强制切换平台样式：

```js
// 模拟 macOS 标题栏
localStorage.setItem('debug-platform', 'mac')

// 模拟 Windows 标题栏
localStorage.setItem('debug-platform', 'win32')

// 模拟 Linux 标题栏
localStorage.setItem('debug-platform', 'linux')

// 恢复自动检测
localStorage.removeItem('debug-platform')
```

设置后刷新页面即可看到效果。

---

## 注意事项

1. **Ent 代码生成**：修改 `internal/ent/schema/` 后需要运行 `make ent` 重新生成 ORM 代码
2. **HTTP 日志代码生成**：修改 `internal/httplog/` 下 `schema.sql` / `query.sql` 后需要运行 `make sqlc` 重新生成
3. **绑定生成**：修改 Go 服务后需要运行 `make bindings` 重新生成前端绑定代码
3. **版本号注入方式**：`wails3 task build` 不接受裸 `-ldflags` 参数，需通过 `VERSION=` 等 Task 变量传入，由各平台 Taskfile 自动拼接成 `-ldflags "-X main.currentVersion=..."`
4. **Linux 交叉编译**：从 macOS 无法交叉编译 Linux（需要 CGO + webkit2gtk），使用 GitHub Actions 构建
5. **Naive UI 按需引入**：直接在模板中使用 `<n-xxx>` 组件，无需全局注册
6. **Pinia 状态管理**：使用 `useXxxStore()` 获取 store，`storeToRefs()` 解构响应式属性
