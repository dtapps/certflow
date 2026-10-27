# ADR-0001：用 Wails v3 作为桌面外壳

- 状态：已采纳
- 日期：2026-07（随项目建立）
- 相关：[architecture.md](../architecture.md)、[development/release.md](../development/release.md)

## 背景

CertFlow 需要同时具备：

- macOS / Windows / Linux 三平台的原生窗口、系统托盘、原生文件对话框、剪贴板、外部浏览器、桌面通知；
- 后端要长期跑 Go 生态里的库：lego（ACME）、各家云厂商 SDK、SQLite、gocron；
- 前端要 Vue 3 + TypeScript + Naive UI；
- 交付形态是单文件安装包，不想让用户额外装 Node 运行时。

同类方案的主要代价：Electron 是 Node 主进程 + 较大的运行时体积；Tauri 是 Rust 主进程，Go 侧要另起进程或换语言；Wails 是 Go 主进程 + 系统 WebView。

## 决定

采用 **Wails v3**（`github.com/wailsapp/wails/v3 v3.0.0`）：

- Go 是主进程，前端构建产物 `frontend/dist` 用 `//go:embed` 打进二进制；
- Go 服务通过 `application.NewService` 注册，`wails3 generate bindings` 生成 TypeScript 到 `frontend/bindings/`；
- 窗口、托盘、菜单、对话框、剪贴板、Dock、自更新全部用 Wails v3 的原生 API；
- 事件用 `application.RegisterEvent` + `app.Event.Emit`，前端用 `@wailsio/runtime` 的 `Events.On`。

## 后果

正面：

- 单进程、单二进制，Go 库直接可用，ACME / 云 SDK 不需要跨进程桥接。
- 前端只写 Web 技术，绑定自动生成，类型能对上。
- 原生能力齐全：托盘常驻、无边框窗口、原生保存 / 打开对话框、内置 updater。

负面 / 已知成本：

- **v3 仍是 beta**：API 会变，升级要跟着改。当前代码里能看到对返回值的特殊处理（`app.Event.Emit` 返回「是否被取消」而非「是否成功」）。
- **没有重启 API**：`SystemService.Quit` 只能退出，数据导入后要用户手动重开（代码注释里写明这是为了避免 SingleInstance 锁释放时序问题）。
- **Service 必须在 Options 之前构造**，而 `*application.App` 在 Options 之后才存在，于是有了「包装层 + `SetApp`」这套模式（见 architecture.md 第 2 节）。
- **测试不能构造 `application.App`**：涉及 Wails 的包只能测守卫分支，覆盖率天然受限（`internal/notification` 27.6%）。
- **平台依赖**：Linux 需要 webkit2gtk，macOS 不能交叉编译 Linux 产物，构建必须上 CI。
- 绑定是生成物：改了 Go 服务要 `make bindings`，否则前端类型对不上。

## 备选方案

- **Electron**：生态成熟，但运行时体积大、需要 Node 侧再实现 ACME / 云 SDK，或额外起 Go 子进程。
- **Tauri**：体积小，但主进程是 Rust，Go 侧能力要重写或做 sidecar，团队栈不匹配。

## 证据

- `go.mod`：`github.com/wailsapp/wails/v3 v3.0.0`
- `main.go`：`application.New`、Service 注册、事件注册、updater、窗口与菜单
- `frontend/bindings/`：生成的 TS 绑定
- `Makefile`：`bindings` 目标
- `system_service.go`：`Quit` 的注释（无 Restart API）
