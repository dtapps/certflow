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
├── main.go                    # 入口文件
├── system_service.go          # 系统服务（主题/窗口管理）
├── cert_service.go            # 证书服务（Wails 包装层）
├── ca_service.go              # CA 管理服务
├── dns_service.go             # DNS 提供商服务
├── auth_service.go            # 认证服务
├── scheduler_service.go       # 定时任务服务
├── monitor_service.go         # 域名监控服务
├── notification_service_wrapper.go  # 通知服务
├── settings_service.go        # 设置服务
├── logging_service_wrapper.go # 日志服务
├── ent/                       # Ent ORM 生成的代码
│   ├── schema/                # 数据库模型定义
│   │   ├── ca.go              # CA 实体
│   │   ├── certificate.go     # 证书实体
│   │   ├── dns_provider.go    # DNS 提供商实体
│   │   ├── monitored_domain.go # 监控域名实体
│   │   ├── notification.go    # 通知实体
│   │   └── renewal_log.go     # 续期日志实体
│   └── ...
├── internal/                  # 内部实现
│   ├── auth/                  # 认证服务
│   ├── ca/                    # CA 管理
│   ├── certificate/           # 证书申请/续期/撤销
│   ├── dnsprovider/           # DNS 提供商管理
│   ├── db/                    # 数据库初始化
│   ├── i18n/                  # 国际化（嵌入式语言文件）
│   ├── logging/               # 日志系统（轮转/压缩）
│   ├── monitor/               # 域名监控
│   ├── network/               # 网络工具（DNS/代理）
│   ├── notification/          # 通知服务
│   ├── scheduler/             # 定时任务调度
│   └── settings/              # 应用设置
├── frontend/                  # 前端应用
│   ├── src/
│   │   ├── views/             # 页面视图
│   │   │   ├── Dashboard.vue
│   │   │   ├── Certificates.vue
│   │   │   ├── CertApply.vue
│   │   │   ├── CertDetail.vue
│   │   │   ├── CAConfig.vue
│   │   │   ├── DNSProviders.vue
│   │   │   ├── Monitor.vue
│   │   │   ├── Settings.vue
│   │   │   ├── PersonalCenter.vue
│   │   │   └── LogViewer.vue
│   │   ├── components/        # 公共组件
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
│   ├── darwin/                # macOS 构建
│   ├── windows/               # Windows 构建
│   └── linux/                 # Linux 构建
├── .github/workflows/         # GitHub Actions 工作流
│   └── release.yml            # 六平台发布
├── .cnb/workflows/            # CNB 工作流
│   └── release.yml            # 从 GitHub 下载发布
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
make package           # 打包 macOS 应用（make package VERSION=1.0.0）
make go-build          # 快速编译（仅 Go 后端）
make lint              # Go 代码检查（golangci-lint）
make lint-fix          # Go 代码检查（自动修复）
make check             # 前端 TypeScript 类型检查
make test              # Go 后端测试
make bindings          # 生成 Wails TypeScript 绑定
make ent               # 生成 Ent ORM 代码
make format            # 格式化所有代码（Go + Vue/TS）
make format-go         # 格式化 Go 代码
make format-frontend   # 格式化前端代码（Prettier）
make clean             # 清理构建产物
make push              # 推送到所有远程仓库
```

---

## 版本号注入

构建时通过 Taskfile 的 BUILD_FLAGS 注入版本号、构建时间和 Git 提交 ID：

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

---

## 跨平台构建

| 平台 | 命令 | 产物 |
|------|------|------|
| macOS | `wails3 task package` | `bin/certflow.app` |
| Windows | `wails3 task build` | `bin/certflow.exe` |
| Linux | `wails3 task build` | `bin/certflow` |

### 发布工作流

- **GitHub Actions**：手动输入版本号，六个平台并行构建，发布 GitHub Release
- **CNB**：手动输入版本号，从 GitHub Release 下载产物并发布

---

## 数据库

使用 SQLite 作为嵌入式数据库，通过 Ent ORM 进行数据管理。实体模型定义在 `ent/schema/` 目录下：

- **CA** — 证书颁发机构配置
- **Certificate** — SSL 证书信息
- **DNSProvider** — DNS 提供商配置
- **MonitoredDomain** — 监控域名
- **Notification** — 通知记录
- **RenewalLog** — 证书续期日志

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
make lint              # 运行 golangci-lint
make lint-fix          # 自动修复
```

启用的检查器：errcheck、govet、staticcheck、ineffassign、misspell、unconvert、nilerr、errorlint、bodyclose、contextcheck、noctx、gosec。

---

## 测试

```bash
make test              # 运行所有测试
```

12 个包均有测试覆盖：auth、ca、certificate、db、dnsprovider、i18n、logging、monitor、network、notification、scheduler、settings。

---

## 注意事项

1. **Ent 代码生成**：修改 `ent/schema/` 后需要运行 `make ent` 重新生成 ORM 代码
2. **绑定生成**：修改 Go 服务后需要运行 `make bindings` 重新生成前端绑定代码
3. **`wails3 build` 不支持 `-ldflags`**：版本号通过 Taskfile BUILD_FLAGS 注入
4. **Linux 交叉编译**：从 macOS 无法交叉编译 Linux（需要 CGO + webkit2gtk），使用 GitHub Actions 构建
5. **Naive UI 按需引入**：直接在模板中使用 `<n-xxx>` 组件，无需全局注册
6. **Pinia 状态管理**：使用 `useXxxStore()` 获取 store，`storeToRefs()` 解构响应式属性
