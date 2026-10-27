# 快速开始

从零到第一张证书。安装包的下载地址见根目录 [README.md](../../README.md) 的 Release 徽章，构建与发布流程见 [development/release.md](../development/release.md)。

## 1. 系统与安装

发布产物按六个平台构建（`.github/workflows/release.yml` 的矩阵）：

| 平台    | 架构          | 产物                                                        |
|---------|---------------|-------------------------------------------------------------|
| macOS   | arm64 / amd64 | `CertFlow-<版本>-darwin-<arch>.zip`（解压得到 `.app`）      |
| Windows | amd64 / arm64 | `CertFlow-<版本>-windows-<arch>-install.exe`（NSIS 安装包） |
| Linux   | amd64 / arm64 | `.AppImage` / `.deb` / `.rpm` / `.pkg.tar.zst`              |

安装注意：

- **macOS**：CI 仅在配置了 Apple 证书与公证 Secret 时才签名 + 公证。未签名的包会被 Gatekeeper 拦（提示「无法验证开发者」或「文件已损坏」）。遇到时在「系统设置 → 隐私与安全性」里允许打开，或自行从源码构建。
- **Windows**：安装包与主程序在配置了签名 Secret 时才做 Authenticode 签名，未签名可能触发 SmartScreen 提示。
- **Linux**：发行版不强制验签，直接用对应包管理器安装，或给 AppImage 加执行权限后运行。

## 2. 首次启动

启动后应用会在用户主目录建数据目录（`main.go`）：

- 正式版：`~/.certflow`
- 开发版（`currentVersion == "dev"`，即 `make dev`）：`~/.certflow.dev`

首次启动会创建：

```
~/.certflow/
├── settings.json      # 默认设置（语言 auto、主题 auto、日志 INFO）
├── seeded.json        # 内置 CA 已种入的标记
├── data/certflow.db   # 业务库（自动建表）
└── logs/              # certflow.log / frontend.log / ent.log / gocron.log
```

同时会种入 5 个内置 CA（`internal/ca/service.go` 的 `SeedDefaults`）：Let's Encrypt、Buypass、ZeroSSL、LiteSSL、Let's Encrypt 测试环境。种子按 `directory_url` 幂等，不会覆盖你后来填的邮箱 / EAB。

**默认没有登录门槛**：没设口令时应用直接进入；启动门禁只看「是否设置了口令」，只配 TOTP / Passkey 不会拦启动。要加门槛先去「个人中心」设口令，见 [security-and-data.md](security-and-data.md)。

## 3. 界面导航

左侧栏与路由（`frontend/src/router/index.ts`）：

| 路由                  | 页面               | 用途                               |
|-----------------------|--------------------|------------------------------------|
| `/`                   | Dashboard          | 证书总览、到期分布                 |
| `/certificates`       | Certificates       | 证书列表、续期、吊销、删除         |
| `/certificates/apply` | CertApply          | 申请证书                           |
| `/certificates/:id`   | CertDetail         | 详情、导出、部署入口               |
| `/ca`                 | CAConfig           | CA 配置、启用、测试连接            |
| `/providers`          | Providers          | DNS 提供商凭证管理                 |
| `/ssl-deploy`         | DeployTargets      | 部署目标列表与执行                 |
| `/ssl-deploy/new`     | DeployTargetForm   | 新建部署目标（三步向导）           |
| `/ssl-deploy/:id`     | DeployTargetDetail | 部署历史、本地 / 云端证书对比      |
| `/monitor`            | Monitor            | 域名监控、历史趋势                 |
| `/scan`               | Scan               | 主动扫描任意域名证书               |
| `/settings`           | Settings           | 续期、通知、网络、日志、数据、关于 |
| `/personal-center`    | PersonalCenter     | 口令、TOTP、Passkey                |

窗口是无边框的，拖动靠标题栏。关闭窗口后进程仍驻留系统托盘（`ApplicationShouldTerminateAfterLastWindowClosed = false`），托盘菜单里有「申请证书 / 扫描 / 检查更新 / 退出」。

## 4. 第一张证书（以 Cloudflare DNS 自动验证为例）

1. **填 CA 邮箱**：进「CA 配置」，选一个内置 CA，填 `account_email` 并启用。邮箱是 ACME 账户注册的必填项，空着会在申请阶段直接报错（`error.ca_email_required`）。
2. **加 DNS 凭证**：进「DNS 提供商」→ 新建，类型选 Cloudflare，填 `api_token`（推荐）或 `email + api_key`，保存并启用。表单字段与后端支持的完整清单见 [dns-providers.md](dns-providers.md)。
3. **申请**：`证书 → 申请`，填主域名、可选 SAN、选 CA 与 DNS 提供商、密钥类型（默认 `EC256`）、是否自动续期与提前天数（默认 30）。提交后走 DNS-01：应用自动写 TXT、等服务商生效、签发、落库。
4. **确认结果**：证书列表里状态为 `active`，详情页能看颁发者、有效期、SAN，并可导出 PEM。
5. **可选：部署**：如果证书要下发到 CDN / 面板，见 [deployment.md](deployment.md)。

如果你没有 DNS API 权限，第 3 步可以不选 DNS 提供商，走手动 DNS：应用会把需要添加的 TXT 记录显示出来，加完再点继续。细节见 [certificates.md](certificates.md) 第 4 节。

## 5. 常用设置

「设置」页能改的东西（`frontend/src/views/Settings.vue`）：

- **续期**：全局自动续期开关、续期间隔（小时）、是否检查过期、检查间隔（小时）。
- **开机自启**、**桌面通知**（含测试通知）。
- **监控历史保留天数**（默认 90）。
- **网络**：自定义 DNS 服务器（内置 11 组）、HTTP/SOCKS5 代理（含用户名密码）。
- **日志**：级别 DEBUG/INFO/WARN/ERROR、查看 / 打开日志目录、复制。
- **数据管理**：导出 / 导入业务库。
- **关于**：版本、构建时间、提交号、是否检查预发布、「检查更新」按钮（按界面语言跳 CNB 或 GitHub 发布页）。

改语言或主题会即时生效；少数设置（如日志级别、续期开关的注册）需要重启，页面顶部有提示。

## 6. 开发者怎么跑起来

```bash
make deps        # Go modules + 前端依赖
make dev         # wails3 dev，使用 .certflow.dev 数据目录
make test-go     # 后端测试
make check       # 前端类型检查 + lint + 测试 + 模糊 + 漏洞检查
```

构建产物是 `bin/certflow`（`make go-build`）或 `wails3 task build VERSION=x.y.z`。细节见 [DEVELOPMENT.md](../../DEVELOPMENT.md)。

## 7. 下一步

- 证书申请、手动 DNS、续期、吊销：[certificates.md](certificates.md)
- DNS 提供商字段：[dns-providers.md](dns-providers.md)
- 部署到云厂商 / 面板：[deployment.md](deployment.md)
- 监控与扫描：[monitoring.md](monitoring.md)
- 认证、备份、代理、更新：[security-and-data.md](security-and-data.md)
- 出问题：[../troubleshooting.md](../troubleshooting.md)
