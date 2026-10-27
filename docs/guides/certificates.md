# 证书：申请、续期、吊销

本文覆盖证书生命周期的实际操作与实现边界。部署部分见 [deployment.md](deployment.md)，ACME 之外的健康检查见 [monitoring.md](monitoring.md)。

## 1. CA 配置

页面：`CA 配置`（`/ca`）。数据表 `cas`，一个 CA 就是一套 ACME 目录 + 账户邮箱。

| 字段                 | 说明                                                                 |
|----------------------|----------------------------------------------------------------------|
| `name`               | 显示名称                                                             |
| `directory_url`      | ACME 目录地址，例如 `https://acme-v02.api.letsencrypt.org/directory` |
| `account_email`      | 账户邮箱，**申请前必填**；为空时申请直接失败                         |
| `eab_kid`/`eab_hmac` | 外部账户绑定，LiteSSL / ZeroSSL 这类 CA 需要；两项都填才启用 EAB     |
| `is_active`          | 是否启用                                                             |
| `is_builtin`         | 内置标记，创建后不可改；内置 CA 不能删除                             |

内置 5 个（`internal/ca/service.go` `SeedDefaults`）：Let's Encrypt、Buypass、ZeroSSL、LiteSSL、Let's Encrypt 测试环境。你可以给它们填邮箱、启停，但删不掉。

「测试连接」调 `TestCAConnection`：走当前网络设置（代理 / 自定义 DNS）拉取目录并回显状态；「检查目录 URL」只做格式与可达性检查。失败时先看 [../troubleshooting.md](../troubleshooting.md) 第 1 节。

## 2. 申请证书

页面：`证书 → 申请`（`/certificates/apply`）。请求结构是 `ApplyCertRequest`：

| 字段              | 说明                                                                     |
|-------------------|--------------------------------------------------------------------------|
| `domain`          | 主域名（必填）                                                           |
| `sans`            | 备用域名列表，可空                                                       |
| `ca_id`           | 目标 CA                                                                  |
| `dns_provider_id` | DNS 提供商 ID；**为空表示手动 DNS 模式**                                 |
| `auto_renew`      | 是否自动续期                                                             |
| `renewal_days`    | 到期前几天续期（默认 30）                                                |
| `key_type`        | `EC256`（默认）/ `EC384` / `RSA2048` / `RSA3072` / `RSA4096` / `RSA8192` |

实现路径（`internal/certificate/service.go` `ApplyCertificate`）：

1. 读 CA，校验邮箱；
2. 生成**两把不同私钥**：ACME 账户密钥、证书密钥（都按 `key_type`）；
3. 构造 lego 客户端，`CADirURL` 取 CA，HTTP 客户端用 `network.BuildHTTPClient`（代理 + 自定义 DNS）；
4. 把启用的 DNS 服务器作为 `dns01` 的递归解析器（用于检查 TXT 是否生效）；
5. 注册账户：有 EAB 走 `RegisterWithExternalAccountBinding`，否则 `Register`；注册失败再尝试 `ResolveAccountByKey`；
6. 选验证方式：有 DNS 提供商 → 创建 lego provider 并 `SetDNS01Provider`；没有 → 手动 DNS；
7. 先落库 `certificates`（`pending`），再 `Obtain`（`Bundle: true`，含中间链）；
8. 解析 x509，回写 `cert_content` / `key_content` / `issuer` / `not_before` / `not_after` / `status=active`；
9. 发通知（成功或失败都会落 `notifications`）。

通配符证书（`*.example.com`）只能走 DNS-01，必须选 DNS 提供商或使用手动 DNS。

## 3. DNS-01 自动验证

前提：DNS 提供商记录已启用，且 API 凭证有该域的解析权限。执行时应用会：

1. 通过厂商 API 写 `_acme-challenge` TXT；
2. 用设置里的 DNS 服务器轮询，确认传播生效；
3. 让 CA 校验并签发；
4. 清理（部分厂商在 `CleanUp` 里删除记录）。

常见失败点：凭证只有读权限、域名不在该账号、CAA 记录限制、DNS 服务器缓存时间过长。排查看 [../troubleshooting.md](../troubleshooting.md) 第 2 节。

## 4. 手动 DNS

不选 DNS 提供商即进入手动模式，它是**两段式**的：

1. 前端调 `StartManualDNSChallenge`，应用创建 ACME 订单并把 `Present` 收到的 TXT 记录暂存（通配符会有多条）。返回结构 `ManualDNSChallenge{domain, records[]}`，每条含 FQDN 与值。
2. 你到域名商控制台添加这些 TXT 记录。
3. 前端调 `CompleteManualDNSChallenge(domain)` 继续签发。等待策略是**每 10 秒轮询一次 TXT、最长 10 分钟**（`internal/certificate/manual_dns.go`），生效即继续，不必等满。

其他入口：

- `GetPendingChallengeInfo(certID)`：查看已落库的 `challenge_records`（重启后仍在）。
- `ResumeManualDNSChallenge(certID)`：为 pending 证书恢复挑战。

**边界**：手动 DNS 申请的证书不会关联 `dns_provider`。`ListAutoRenew` 会显式排除「没有 DNS 提供商」的证书，续期路径也没有手动 DNS 分支，所以这类证书**不能自动续期**，到期前需要重新申请（或改用有 API 的 DNS 提供商）。`renewal_days` 对它没有实际作用。

## 5. 续期

### 5.1 手动续期

- 证书列表 / 详情页对单张证书触发 `RenewCertificate(id)`。
- 「设置 → 维护 → 立即执行续期」调 `RunRenewalNow()`，对**所有满足条件**的证书跑一轮，并忽略全局自动续期开关。

### 5.2 自动续期

`internal/scheduler/service.go`：

- 任务 `auto_renew_certificates`，间隔取设置 `renew_interval`（小时，默认 1）。
- 先看全局开关 `auto_renew_enabled`；关闭则整轮跳过（手动触发除外）。
- 候选集 `ListAutoRenew`：`status=active` + `auto_renew=true` + **已关联 DNS 提供商** + `not_after <= now + renewal_days`。没有 DNS 提供商的证书（手动 DNS 申请）会被显式排除，并记一条统计日志。
- 每张证书先写 `renewal_logs(pending)`，成功回写新证书内容与 `completed_at`，失败写 `error_message`，并分别发通知。
- 续期成功会更新 `last_renewed_at`，证书状态保持 `active`。

### 5.3 续期的实现边界

- **续期不会自动重新部署**。调度器没有调用 `DeployService`，续期后需要你手动重新部署，见 [deployment.md](deployment.md)。
- **续期不使用 EAB**。`RenewCertificate` 只做 `ResolveAccountByKey` → 失败再 `Register`（不带 EAB 参数）。如果你的 CA 强制 EAB 且账户密钥无法解析，续期会失败，需要重新申请或修正账户。
- **私钥会轮换**：每次续期都新生成证书密钥，不复用旧私钥。

## 6. 吊销

`RevokeCertificate(id)`（`internal/certificate/service.go`）：

1. 读证书 PEM 与私钥，`loadSignerFromContent` 依次尝试 EC / PKCS1 / PKCS8（修复过 RSA 证书无法吊销的问题）；
2. 构造 lego 客户端（CA 目录 + 邮箱）；
3. `client.Certificate.Revoke(ctx, certPEM)`；
4. 成功则把状态置为 `revoked`。

吊销是不可逆的 CA 侧操作：证书会立刻失效，已部署到 CDN / 面板的副本不会自动清理。

## 7. 删除

`DeleteCertificate(id)` 只删 `certificates` 里这一行，不会去删部署目标、部署日志或续期日志。

注意：`renewal_logs` 对证书的外键是 `OnDelete: NoAction`，而删除逻辑没有先清理续期日志。**有续期历史的证书可能删除失败**（外键约束），现象与处置见 [../troubleshooting.md](../troubleshooting.md) 第 7 节。证书对 CA / DNS 提供商 / 部署目标的外键是 `SET NULL`，删除证书不影响这些父记录。

## 8. 导出与查看

- 列表项携带 `cert_content` / `key_content`，详情页可复制 PEM。
- `ParseCertificateDetails` 解析 SAN、序列号、签名算法、公钥算法与位数、有效期，用于详情展示。
- `ExportCertificate` 返回 PEM 字节，供导出文件。
- 私钥是明文：导出、复制、备份都按密钥对待，见 [security-and-data.md](security-and-data.md)。

## 9. 关于「上传证书」

当前代码**没有导入外部已有证书的入口**：没有上传表单，`certificate` 包也没有「导入 PEM/KEY」的方法。README 的证书管理只写申请 / 续期 / 撤销，不再声称支持上传。

和「上传」相关的只有部署链路：把本应用签发的证书上传到云厂商证书服务，用 `cert_uploads` 表按「厂商 + AccessKeyId + 区域 + 证书指纹」去重，避免重复上传（`internal/deploy/service.go` `ensureUploaded`）。如果你需要管理外部证书，目前只能通过 API 之外的流程手工维护，或等后续实现。

## 10. 相关代码

- 申请 / 续期 / 吊销 / 删除：`internal/certificate/service.go`
- DNS-01 provider 构造：`internal/certificate/dns_provider.go`
- 手动 DNS：`internal/certificate/manual_dns.go`
- 调度：`internal/scheduler/service.go`
- 绑定层：`cert_service.go`、`scheduler_service.go`
