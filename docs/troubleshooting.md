# 排障手册

按「症状 → 原因 → 处置」组织。日志位置在最后，先看日志往往比猜快。操作步骤见 [guides/](guides/getting-started.md)。

## 0. 先确定看哪里

| 现象                 | 首查                                                       |
|----------------------|------------------------------------------------------------|
| 申请 / 续期失败      | `~/.certflow/logs/certflow.log`，搜 `cert_apply` / `renew` |
| 部署失败             | 部署目标详情的部署历史 + `certflow.log`，搜 `deploy`       |
| 界面报错但后端没日志 | `frontend.log`                                             |
| 定时任务没跑         | `gocron.log`                                               |
| 想知道发了什么请求   | 设置里把日志级别调 DEBUG，出站请求进 `httplog.db`          |

## 1. 申请证书失败

| 症状 / 报错                                 | 原因                                                  | 处置                                                                              |
|---------------------------------------------|-------------------------------------------------------|-----------------------------------------------------------------------------------|
| `CA 邮箱未配置` / `error.ca_email_required` | CA 的 `account_email` 为空                            | CA 配置里补邮箱并启用                                                             |
| `注册 ACME 账户失败`                        | 网络不通、代理错、CA 目录不可达                       | 用「测试连接」验证目录；检查代理 / DNS；换网络再试                                |
| 注册返回 401 / `unauthorized`               | CA 需要 EAB（LiteSSL / ZeroSSL 等）                   | 填 `eab_kid` 与 `eab_hmac`，两项都填才会走 EAB 注册                               |
| `rate limit` / `too many certificates`      | CA 侧频率限制                                         | 等冷却期；测试用 Let's Encrypt 测试环境 CA                                        |
| `dns provider ... unsupported/empty`        | `provider_type` 不在支持列表，或配置全空              | 检查字段名是否与该厂商一致，见 [guides/dns-providers.md](guides/dns-providers.md) |
| 保存 DNS 提供商报 `invalid enum value`      | ent 的 `provider_type` 枚举只覆盖 19 家（代码未同步） | 先用枚举内的厂商；或补 `DnsProviderTypes` 后 `make ent` 重新生成                  |
| `no such host` / 超时                       | 自定义 DNS 配错、代理失效                             | 设置 → 网络：临时只留「默认（系统 DNS）」，关代理再试                             |
| 证书签发成功但列表没变化                    | 前端缓存 / 事件未刷新                                 | 切页刷新；看 `certflow.log` 是否已 `cert_apply_success`                           |

通配符证书（`*.example.com`）必须走 DNS-01：要么选 DNS 提供商，要么用 [手动 DNS](guides/certificates.md#4-手动-dns)。HTTP-01 不支持通配符。

## 2. DNS 传播 / 手动 DNS

| 症状                      | 原因                                        | 处置                                                               |
|---------------------------|---------------------------------------------|--------------------------------------------------------------------|
| 自动验证一直等，最后超时  | TXT 还没生效；递归 DNS 缓存旧记录           | 用 `dig TXT _acme-challenge.example.com @1.1.1.1` 核对；换公共 DNS |
| 手动 DNS 加了记录仍不通过 | 记录名多了 / 少了域名段；值带引号；TTL 太长 | 记录名用应用给出的 FQDN 原样；值不要加引号；TTL 调到 60            |
| 提示记录已存在但校验失败  | 同名 TXT 有多条，CA 取到旧值                | 删掉旧 TXT 再等 TTL；通配符需要两条不同 TXT，都要加                |
| 10 分钟后失败             | 手动 DNS 等待上限是 10 分钟（每 10 秒轮询） | 确认记录生效后重新发起；不要干等                                   |
| `CAA` 相关错误            | 域名 CAA 记录不允许当前 CA                  | 调整 CAA 或换 CA                                                   |
| 厂商 API 报权限不足       | Token 只有读权限，或域名不在该账号          | 换有写权限的 Token；确认域名归属                                   |

## 3. 部署 401 / 403 / 鉴权失败

| 症状                          | 原因                                           | 处置                                                                                                      |
|-------------------------------|------------------------------------------------|-----------------------------------------------------------------------------------------------------------|
| 401 Unauthorized              | AK/SK 错、被禁用、复制时带了空格               | 重新粘贴；注意字段名：阿里云 `access_key_id`，腾讯云 `secret_id`/`secret_key`，华为云 `secret_access_key` |
| 403 Forbidden                 | 子账号缺权限、IP 白名单、区域不匹配            | 给最小权限策略；把本机公网 IP 加白；核对 `region`                                                         |
| 面板 API 401                  | `api_key` 错、面板 URL 带/不带协议、Token 过期 | `panel_url` 用 `https://host:port`；AcePanel 用 `token_id` + `token_secret`                               |
| 拉不到域名 / 站点列表         | 凭证对该资源无权限，或区域选错                 | 换区域重试；确认资源在该账号下                                                                            |
| 部署成功但 CDN 仍旧证书       | CDN 需要刷新 / 预热，或绑错域名                | 在云控制台确认证书已绑定到目标域名；等 CDN 配置下发                                                       |
| `deploy_unsupported_provider` | 厂商未注册部署器                               | 看 [guides/deployment.md](guides/deployment.md) 的支持矩阵                                                |

云的业务错误常是 HTTP 200 + 错误码，`deploy_logs.response` 里保留了原始响应，先看它再改配置。

## 4. 面板 / 防火墙证书不生效

| 症状                                      | 原因                                            | 处置                                                                  |
|-------------------------------------------|-------------------------------------------------|-----------------------------------------------------------------------|
| 面板返回成功但浏览器还是旧证书            | 站点没重载 nginx / apache，或证书路径被面板缓存 | 在面板里重载站点；确认面板「SSL」里已启用该证书                       |
| 面板里能看到证书但站点没绑定              | 部署只上传证书，绑定依赖站点 ID / 站点名        | 重新在部署目标里选站点；站点改名后要重新选                            |
| `site not found`                          | 站点 ID 与名称不匹配，或面板版本差异            | 用「获取网站」重新拉列表；AcePanel / 雷池的字段口径不同，按表单提示填 |
| WAF 类（雷池 / UUWAF / 堡塔云 WAF）不生效 | WAF 有自己的证书管理入口，需要站点级绑定        | 在 WAF 控制台确认站点证书；重新执行部署                               |
| 1Panel / 宝塔 API 403                     | API 未开启、IP 限制、API Key 权限不足           | 面板设置里开启 API 并放行本机 IP                                      |

## 5. 更新与代理

| 症状                         | 原因                                                                     | 处置                                                                                    |
|------------------------------|--------------------------------------------------------------------------|-----------------------------------------------------------------------------------------|
| 检查更新一直「无更新」       | 中文走 CNB，未注入 `cnbToken` 时匿名 API 返回 401，被视为无更新          | 用正式发布包（CI 注入 token）；或看 `certflow.log` 的 `updater` 日志                    |
| 下载更新失败 / 校验失败      | 代理拦了下载、`SHA256SUMS` 不匹配、资产缺失                              | 关代理重试；确认 Release 里有 `SHA256SUMS` 与 `updater-*` 资产                          |
| 更新后打不开（macOS）        | 未签名 / 未公证，Gatekeeper 拦截                                         | 系统设置 → 隐私与安全性里允许；或换用已签名发布包                                       |
| Windows 提示未知发布者       | 未配置签名 Secret，安装包未签名                                          | 属预期；选「仍要运行」，或用已签名版本                                                  |
| 开了代理后 ACME / 监控全超时 | 代理不可用，或代理不支持该协议                                           | 临时关代理；SOCKS5 注意端口与鉴权                                                       |
| 部署能连但监控不能（或反之） | 代理只作用于 `BuildHTTPClient`（ACME/监控/扫描/更新），部署 SDK 不走代理 | 这是当前实现边界，见 [guides/security-and-data.md](guides/security-and-data.md) 第 4 节 |

## 6. 数据库 / 驱动

| 症状                                       | 原因                                                   | 处置                                           |
|--------------------------------------------|--------------------------------------------------------|------------------------------------------------|
| 启动即退出，日志 `open db failed`          | 数据目录不可写、磁盘满、库文件损坏                     | 检查 `~/.certflow` 权限与磁盘；用备份恢复      |
| 换构建后打不开库                           | modernc 与 mattn 写的库文件不保证互通                  | 用应用内导出 / 导入迁移，不要直接拷 `.db`      |
| `CGO_ENABLED=0` 下编译 `sqlite_mattn` 失败 | mattn 需要 CGO                                         | 去掉 `sqlite_mattn` 标签，或用 `CGO_ENABLED=1` |
| `windows/arm64` 构建失败                   | mattn 不支持该组合                                     | 回退 modernc（CI 已自动处理）                  |
| 导入成功但数据没变                         | 没重启应用（ent 连接已关闭，需 `db.Init` 重开）        | 按提示退出应用再打开                           |
| 导入报「备份文件包含非法表」               | zip 里的 CSV 名不是本库业务表（被改过 / 混入其它文件） | 用应用导出的原始备份；不要手工往 zip 里塞 CSV  |

驱动细节见 [database.md](database.md) 第 6 节。

## 7. 续期与删除

| 症状                          | 原因                                                          | 处置                                                                                    |
|-------------------------------|---------------------------------------------------------------|-----------------------------------------------------------------------------------------|
| 手动 DNS 申请的证书续期失败   | 续期只认已关联的 DNS 提供商，没有手动 DNS 分支                | 改用有 API 的 DNS 提供商重新申请，或到期前手动重签                                      |
| EAB CA 首次申请成功、续期失败 | 续期路径不使用 EAB，只 `ResolveAccountByKey` → `Register`     | 重新申请；或换不需要 EAB 的 CA                                                          |
| 「立即续期」没反应            | 没有满足条件的证书（未到期 / `auto_renew=false` / 非 active） | 看 `gocron.log` 与 `renewal_logs`；确认证书设置                                         |
| 自动续期不跑                  | 全局 `auto_renew_enabled` 关了，或间隔设置异常                | 设置 → 续期里打开；重启应用让间隔生效                                                   |
| 到期提醒重复出现              | 过期检查对 7 天内到期的证书每轮都发通知（无去重）             | 调大 `check_interval`，或先处理该证书                                                   |
| 删除证书报外键错误            | `renewal_logs` 外键是 `NoAction`，删证书前没清日志            | 保留证书或先清续期记录；现象见 [guides/certificates.md](guides/certificates.md) 第 7 节 |

## 8. 认证与登录

| 症状                     | 原因                                             | 处置                                                                                       |
|--------------------------|--------------------------------------------------|--------------------------------------------------------------------------------------------|
| 忘了口令，其他方式也没配 | 无找回流程                                       | 退出应用后清掉 `auth_methods` 里的 password 行（等于移除门槛），或导入旧备份；操作前先备份 |
| Passkey 注册被浏览器拒绝 | 运行环境不是 `wails://localhost`，或系统无认证器 | 在应用内操作；确认系统已设置 Touch ID / Windows Hello                                      |
| TOTP 验证一直失败        | 本机时间偏差超过窗口                             | 校准系统时间；重新绑定                                                                     |
| 清掉认证后应用直接进入   | 这是设计行为（无激活方式即放行）                 | 需要门槛就重新设置                                                                         |

## 9. 日志位置与常用命令

```
~/.certflow/logs/certflow.log     # 主日志
~/.certflow/logs/frontend.log     # 前端错误
~/.certflow/logs/ent.log          # DEBUG 下的 SQL
~/.certflow/logs/gocron.log       # 定时任务
~/.certflow/data/certflow.db      # 业务库
~/.certflow/data/httplog.db       # 请求日志（DEBUG）
```

```bash
# 跟踪主日志
tail -f ~/.certflow/logs/certflow.log

# 找某次申请
grep -n "cert_apply\|obtain" ~/.certflow/logs/certflow.log | tail -50

# 找部署错误
grep -n "deploy" ~/.certflow/logs/certflow.log | tail -50

# 看定时任务有没有跑
tail -50 ~/.certflow/logs/gocron.log

# 查证书状态（应用退出后）
sqlite3 ~/.certflow/data/certflow.db \
  "select id,domain,status,not_after,last_error from certificates order by id desc limit 20;"

# 查最近的部署日志
sqlite3 ~/.certflow/data/certflow.db \
  "select id,cert_domain,deploy_domain,success,message from deploy_logs order by id desc limit 20;"
```

DEBUG 级别在「设置 → 日志」里切换，立即生效；请求日志需要 DEBUG 才会写 `httplog.db`，保留天数默认 30 天。日志与备份都可能含私钥 / 凭证，外发前先脱敏。
