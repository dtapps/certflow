# 安全模型

本文写代码里**实际成立**的安全事实，不写期望。功能操作见 [../guides/security-and-data.md](../guides/security-and-data.md)。

## 1. 信任边界

CertFlow 是单用户本地桌面应用：

```
用户 ── WebView（Vue） ── Wails RPC ── Go 主进程 ── SQLite / 文件 / 云 API
```

- 没有服务端，没有多租户，没有远程登录。
- 登录 / 口令 / Passkey 是**界面层的门**：`App.vue` 与 `LoginDialog.vue` 决定是否展示主界面。Go 侧的绑定方法（`cert_service.go` 等）**不做授权检查**，任何能驱动该进程或直接读库的人都能绕过。
- 因此安全边界是**操作系统账户 + 文件权限**，不是应用内认证。口令适合防「同事顺手点一下」，不适合防本机恶意进程。

## 2. 认证事实

| 项            | 事实                                                                                                                                                                    |
|---------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| 口令          | bcrypt（`bcrypt.DefaultCost`），最少 6 位；空 / 未设置时 `VerifyPassword` 直接返回 true                                                                                 |
| 认证方式      | `auth_methods` 表可存 password / totp / passkey 三条；`SetActiveMethod` 会先把全部置 false 再激活一条，但 `SetPassword` 新建时直接 `is_active=true`（不清理其它激活项） |
| TOTP          | `pquerna/otp`，密钥 Base32 明文存 `totp_credentials.secret`；发行者 `CertFlow`、账户 `user`；无恢复码                                                                   |
| Passkey       | go-webauthn，存凭据 ID、公钥、签名计数；私钥在系统认证器里，服务端只有公钥                                                                                              |
| WebAuthn 参数 | 硬编码 `RPID=localhost`、`RPOrigins=["wails://localhost"]`（`internal/auth/passkey.go`）                                                                                |
| 启动门禁      | `App.vue` 只看 `IsPasswordSet()`：没设口令就直接进入。**只配 TOTP / Passkey 而不设口令时，启动不会弹验证**                                                              |
| 找回          | 没有找回流程；忘记口令且无其它方式时，只能清库里的 `auth_methods` 或导入旧备份                                                                                          |

`RPID` / `RPOrigins` 写死意味着：换应用外壳、换 scheme、换端口后 Passkey 会失效，需要同步改代码并重新注册。

## 3. 密钥与敏感数据存储

**没有加密层**。所有敏感数据以明文（或可逆编码）落盘：

| 数据                      | 位置                                                | 形式                                             |
|---------------------------|-----------------------------------------------------|--------------------------------------------------|
| 证书私钥                  | `certificates.key_content`                          | PEM 明文                                         |
| 续期产生的新私钥          | `renewal_logs.key_content`                          | PEM 明文                                         |
| 云厂商 AccessKey / Secret | `dns_providers.config`、`deploy_credentials.config` | JSON 明文                                        |
| 面板 API Key / JWT Secret | 同上                                                | JSON 明文                                        |
| 代理密码                  | `settings.json` 的 `proxy.password`                 | JSON 明文                                        |
| TOTP 密钥                 | `totp_credentials.secret`                           | Base32 明文                                      |
| 口令                      | `auth_methods.password_hash`                        | bcrypt 哈希（唯一非明文项）                      |
| 明文请求 / 响应           | `httplog.db`（DEBUG）                               | 含请求头（可能带 Authorization）、请求体、响应体 |

`secret:"true"` 与 ent 的 `Sensitive()` 常被误解，实际作用有限：

- `secret:"true"` 只是结构体 tag，供 `internal/config.StripSecrets` 反射剔除。**当前生产代码没有调用 `StripSecrets`**（只有测试），所以它不影响 RPC 返回值，更不是加密。
- ent 的 `Sensitive()` 只让生成的 `String()` 把值打成 `<sensitive>`（如 `password_hash=<sensitive>`、`secret=<sensitive>`），避免调试输出泄露。
- 权限：目录 `0755`，日志文件显式 `0644`，数据库与 `settings.json` 按进程默认权限创建；没有任何 `0600` 收紧。

结论：**能读 `~/.certflow` 的人等于拿到了所有云账号和证书私钥**。备份 zip（按表 CSV）同理，且不含任何加密。

## 4. 备份风险

- 导出 zip 内含 `certificates.key_content`、`renewal_logs.key_content`、`dns_providers.config`、`deploy_credentials.config` —— 私钥 + 云凭证一次性打包。
- 导入会清空并重写业务库（单事务），恶意备份可以把数据替换成任意内容；表名白名单只防 SQL 注入，不验数据语义。
- `settings.json`、`seeded.json`、日志、`httplog.db` 不在备份里，需要单独考虑。
- 迁移必须用导出 / 导入，不要复制 `.db`（双驱动格式不保证互通，见 [../database.md](../database.md) 第 6 节）。

## 5. 网络与更新

- 出站请求走 `network.BuildHTTPClient`：自定义 DNS、代理、User-Agent、DEBUG 下的请求日志。覆盖 ACME、CA 目录测试、监控、扫描、更新器。
- 云厂商部署 SDK **不使用**这个客户端（各 SDK 自带 endpoint/transport），所以代理设置对部署不生效。
- 更新校验：只校验 Release 资产里的 `SHA256SUMS`，**不验证签名**。信任链是「HTTPS + 发布渠道」；渠道被攻破即可同时替换包和校验和。
- 更新资产只接受 `updater-` 前缀的 `.zip` / `.tar.gz` / `.tgz`，安装包不参与自更新。
- 中文用户走 CNB、英文用户走 GitHub，两源独立；CNB 匿名 API 返回 401，未注入 Token 时表现为「无更新」。
- 代理是可选中间人：启用后，代理方能看到请求元数据；若安装了 mitm 证书，也能看到明文内容。代理密码以明文存在 `settings.json`。

## 6. 攻击面清单

| 面                    | 说明                                                            | 现有缓解                                   |
|-----------------------|-----------------------------------------------------------------|--------------------------------------------|
| 本地数据目录          | 明文私钥 / 凭证 / 设置                                          | 仅依赖 OS 账户权限；无加密、无钥匙串       |
| 备份文件              | 明文 CSV，含私钥与 AK/SK                                        | 无                                         |
| 请求日志 `httplog.db` | DEBUG 下记录 URL、请求头（可能含 Authorization）、请求 / 响应体 | 默认 INFO 不记录；保留天数可配；无自动脱敏 |
| 运行日志              | `certflow.log` / `frontend.log` 可能含域名、错误、云端响应片段  | 轮转 + 手动清理；外发需脱敏                |
| 应用内认证            | 仅前端门禁，绑定层无授权检查                                    | 适合单人桌面场景，不防本机进程             |
| 导入备份              | 可整体替换业务库                                                | 表名白名单、事务回滚、用户确认             |
| 更新通道              | 校验和不签名                                                    | HTTPS、渠道分离、仅接受 `updater-*`        |
| Passkey 注册          | RPID / Origin 写死                                              | 改动外壳需同步代码                         |
| 单实例锁              | `SingleInstance.UniqueID = net.dtapp.certflow`                  | 防止多实例争库，不是安全边界               |
| 明文 HTTP 日志与通知  | 通知正文含域名、错误码                                          | 不发往远端，仅本机                         |

## 7. 加固建议（按投入排序）

1. 设口令或 Passkey；共享机器上随手锁屏。
2. 数据目录与备份放加密卷 / 加密磁盘；备份不要进公共云盘。
3. 不用 DEBUG 常驻；排完立刻调回 INFO，并清理 `httplog.db`。
4. 代理只填可信节点；不用时关闭。
5. 云厂商用子账号 + 最小权限 + IP 白名单，别用主账号 AK。
6. 升级前核对 Release 的 `SHA256SUMS`（手动下载时）。

以上都是使用侧措施；代码侧目前**没有**加密、钥匙串、审计日志、权限校验这些机制，不要假设它们存在。

## 8. 漏洞报告

仓库里没有 `SECURITY.md`，也没有公开的安全邮箱。发现漏洞时：

- 优先用 GitHub 仓库的 Security 面板（若启用了 Private vulnerability reporting）提交私下报告；
- 没有该入口时，联系维护者私下说明，避免在公开 Issue 里贴利用细节、真实域名、Token 或私钥；
- 报告中请带上版本号（关于页）、复现步骤、影响范围，以及是否已在最新 Release 复现。

已知的非问题：`govulncheck` 对 `golang.org/x/crypto/openpgp`（GO-2026-5932）的告警不影响本应用代码路径（见 `docs/handoff.md`）。
