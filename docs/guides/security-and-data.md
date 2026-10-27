# 安全与数据

本文是用户视角的安全操作说明；威胁模型与代码事实见 [../operations/security.md](../operations/security.md)。

## 1. 登录与认证方式

页面：`个人中心`（`/personal-center`）。认证方式存在 `auth_methods` 表；切换激活方式时会把其它条目置为未激活。

| 方式     | 存储                                             | 说明                                       |
|----------|--------------------------------------------------|--------------------------------------------|
| 访问口令 | `password_hash`（bcrypt，`Sensitive`）           | 至少 6 位；未设置时验证直接通过            |
| TOTP     | `totp_credentials.secret`（Base32，`Sensitive`） | 发行者 `CertFlow`，账户名 `user`；扫码绑定 |
| Passkey  | `passkey_credentials`（凭据 ID、公钥、计数器）   | WebAuthn；触发系统生物识别或安全密钥       |

行为要点：

- **启动门禁只看「有没有设口令」**。`App.vue` 先问 `IsPasswordSet()`：没设口令就直接进入主界面，即使你配了 TOTP 或 Passkey 也不会在启动时弹验证。想让启动有门槛，必须先设口令；TOTP / Passkey 是在登录框里替代口令的验证方式。
- 设置 / 修改 / 清除口令都需要旧口令（`ChangePassword`）；清除后回到无门槛状态，界面会提示。
- TOTP 是「先建后激活」：`SetupTOTP` 生成密钥并返回 `otpauth://` URL（前端转二维码），`VerifyTOTPSetup(code)` 验证通过后才可用；`CancelTOTP` 可撤销未完成的设置。
- Passkey 注册走 `navigator.credentials.create`，后端用 go-webauthn 校验；配置写死在 `internal/auth/passkey.go`：`RPDisplayName=CertFlow`、`RPID=localhost`、`RPOrigins=["wails://localhost"]`。换域名 / 换外壳时这里必须同步，否则浏览器会拒绝。
- 生物识别（Touch ID / Windows Hello / Face ID）不是独立认证方式，而是 Passkey 由系统认证器完成用户验证的体现。仓库里的 `internal/biometric/binaries/` 有原生 helper 二进制，但 Go 侧没有引用，不在当前认证链路里。

## 2. 数据在哪

| 内容                                   | 位置                           | 形式                    |
|----------------------------------------|--------------------------------|-------------------------|
| 业务数据（证书、私钥、凭证、日志记录） | `~/.certflow/data/certflow.db` | SQLite，明文字段        |
| HTTP 请求日志                          | `~/.certflow/data/httplog.db`  | SQLite，仅 DEBUG 记录   |
| 应用设置（含代理密码）                 | `~/.certflow/settings.json`    | JSON 明文               |
| 运行日志                               | `~/.certflow/logs/*.log`       | 文本，按大小轮转 + gzip |
| 开发数据                               | `~/.certflow.dev/`             | 同上，与正式版隔离      |

没有系统钥匙串（Keychain / Credential Manager）集成，没有数据库加密。**文件权限就是唯一边界**：同机器上的其他进程 / 其他用户只要能读这些文件，就能拿到私钥和云凭证。

## 3. 备份与迁移（数据管理）

设置 → 数据管理：

- **导出**：把业务库按表导出为 CSV 并打包成 `certflow-backup.zip`，用原生保存对话框选位置。
- **导入**：选 zip，确认后后台执行（前端轮询进度），完成后提示退出应用；重新打开才生效。

必须知道的几点：

1. **zip 里是明文**，包括 `certificates.key_content` 和 `renewal_logs.key_content`（私钥 PEM）、云厂商 AccessKey。备份文件按密钥保管，不要放公共网盘 / 聊天工具。
2. **不含** `settings.json`、`seeded.json`、日志、`httplog.db`。换机时这些要单独考虑：新机器要重新配代理 / DNS / 认证方式（认证数据在 `certflow.db` 里，会随备份迁移）。
3. **不要用复制 `.db` 文件的方式迁移**：项目支持两种 SQLite 驱动（默认纯 Go 的 modernc，可选 CGO 的 mattn），驱动之间的文件格式不保证互通。导出导入走的是 SQLite 自身的数据搬运，目标库由当前驱动写入。
4. 导入是单事务：失败会整体回滚，不会留下「清空了但没导入」的状态。
5. 导入前会关闭数据库连接，**必须重启应用**（界面上的「退出应用」按钮）才能重新打开库。

细节与硬约束见 [../database.md](../database.md) 第 7 节。

## 4. 网络：代理与 DNS

设置 → 网络：

- **自定义 DNS**：内置 11 组（系统、阿里云、腾讯、华为、114、百度、360、CNNIC、火山、Google、Cloudflare），可添加自定义服务器列表，逐条启用。
- **代理**：`http` / `https` / `socks5`，主机、端口、可选用户名密码。

作用范围（`internal/network` 的 `BuildHTTPClient` 与 `ClientCache`）：

- ACME 申请 / 续期、CA 目录连通性测试；
- 域名监控、证书扫描；
- 应用更新检查与下载。

**云厂商部署 SDK 不读取这两项设置**：`internal/deploy` 下的各 SDK 自带 endpoint 与 transport，没有走 `BuildHTTPClient`。所以「代理开了但部署还是连不上云端」通常不是配置丢了，而是这条链路本来就不走代理。

DNS 选择还会影响 DNS-01 的传播检查：所有已启用的 DNS 服务器都会作为递归解析器，配得太多会拖慢检查。

## 5. 更新校验

- 启动后 1–3 分钟随机延迟检查一次（`dev` 版本跳过），也可以在菜单 / 设置里手动检查。
- 更新源按界面语言分流（`mirror_updater.go`）：中文走 CNB，英文走 GitHub；两边独立，不互相回退。
- 校验依赖 Release 资产里的 `SHA256SUMS`：校验和来自同一发布渠道；中文源从 CNB 拉，英文源从 GitHub 拉。CNB 的 nightly 资产自带 `hash_value`，不用侧车文件。
- 自更新只认文件名以 `updater-` 开头的压缩包（`.zip` / `.tar.gz` / `.tgz`），安装包不参与自更新。
- 预发布版额外生成 `BUILD_TIME` / `GIT_COMMIT`，用于比对是否同一份构建。
- **没有签名验签**：只校验 SHA-256，不验证发布者身份。信任链是「HTTPS + 发布渠道」，不是密码学签名。

## 6. 日志与隐私

- `certflow.log`：应用主日志，可能包含域名、错误信息、云端返回片段。
- `frontend.log`：前端运行时错误（发布版也会上报，因为发布构建会 drop `console`）。
- `ent.log`：仅 DEBUG 下的 SQL；`gocron.log`：定时任务。
- `httplog.db`：DEBUG 下的出站请求日志，会记录 URL / 头 / 响应片段。排查完把级别调回 INFO，并按 `http_log_retention_days` 清理（默认 30 天）。
- 日志查看器支持按文件读取末尾 N 行；「打开日志目录」调系统文件管理器。

不要把含真实域名、Token、私钥片段的日志贴到公开 Issue。

## 7. 日常建议

1. 先设口令（启动门禁只认它），再决定是否用 TOTP / Passkey 替代；TOTP 没有恢复码，清理由「个人中心」完成。
2. 备份前想清楚存放位置；备份泄露等于私钥泄露。
3. 不用代理时关掉，避免代理失效导致 ACME / 更新全部超时。
4. 只在需要排查时开 DEBUG，排完切回 INFO。
5. 更新后如遇启动异常，先保留 `logs/` 和旧备份，再回滚版本。
6. 共享机器上使用完退出应用，不要让托盘常驻在未锁屏的会话里。
