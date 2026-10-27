# 证书部署

把本应用签发（或已续期）的证书下发到云厂商的 CDN / WAF / 负载均衡，或面板 / WAF 的站点。页面：`部署`（`/ssl-deploy`）。

## 1. 支持矩阵

厂商与服务来自前端表单 `frontend/src/utils/deploy.ts` 的 `servicesByProvider`，后端按 `provider_type` + `deploy_service` 分发到 `internal/deploy/` 下对应部署器。

| 厂商 / 面板       | `provider_type`    | 服务（`deploy_service`）                                           |
|-------------------|--------------------|--------------------------------------------------------------------|
| 阿里云            | `aliyun`           | `cdn`、`dcdn`（全站加速）、`esa`（边缘安全加速）、`ga`（全球加速） |
| 腾讯云            | `tencentcloud`     | `cdn`、`edgeone`、`ecdn`                                           |
| 华为云            | `huawei`           | `cdn`、`waf`、`elb`                                                |
| 百度云            | `baiducloud`       | `cdn`、`drcdn`（动态加速）                                         |
| 天翼云            | `ctyun`            | `ctcdn`、`icdn`、`accessone`                                       |
| 火山引擎          | `volcengine`       | `cdn`、`dcdn`                                                      |
| 宝塔面板          | `btpanel`          | `site`                                                             |
| aaPanel           | `aapanel`          | `site`                                                             |
| 1Panel            | `1panel`           | `site`                                                             |
| AcePanel          | `acepanel`         | `site`                                                             |
| 堡塔云 WAF        | `aawaf`            | `site`                                                             |
| OpenResty Manager | `openrestymanager` | `site`                                                             |
| UUWAF             | `uuwaf`            | `site`（**后端无部署器，会报 unsupported**）                       |
| 雷池 SafeLine     | `safeline`         | `site`                                                             |

面板 / 防火墙类（`isPanelProvider`）统一用 `site`，具体站点在执行部署时现场选择。

## 2. 凭证

两种来源，由部署目标的 `credential_source` 决定：

| 来源           | 取值                | 说明                                                        |
|----------------|---------------------|-------------------------------------------------------------|
| DNS 提供商凭证 | `dns_provider`      | 复用「DNS 提供商」里已保存的云账号密钥（同厂商才通用）      |
| 部署凭证       | `deploy_credential` | 在「部署凭证」里单独保存，适合 DNS 与 CDN 用不同账号/子账号 |

部署凭证的字段（`frontend/src/utils/deployProviderConfig.ts`）：

| 类型                                                              | 字段                                           |
|-------------------------------------------------------------------|------------------------------------------------|
| `aliyun`                                                          | `access_key_id`、`access_key_secret`、`region` |
| `tencentcloud`                                                    | `secret_id`、`secret_key`、`region`            |
| `huawei`                                                          | `access_key_id`、`secret_access_key`、`region` |
| `baiducloud`                                                      | `access_key_id`、`access_key_secret`           |
| `ctyun`                                                           | `access_key_id`、`access_key_secret`           |
| `volcengine`                                                      | `access_key_id`、`access_key_secret`           |
| `btpanel` / `1panel` / `aapanel` / `aawaf` / `uuwaf` / `safeline` | `panel_url`、`api_key`                         |
| `acepanel`                                                        | `panel_url`、`token_id`、`token_secret`        |
| `openrestymanager`                                                | `panel_url`、`jwt_secret`                      |

部署目标的 `config` 只落区域：阿里云写 `region_id`，其它厂商写 `region`，面板类不写区域。结构体 `DeployTargetConfig` 仍保留 `domains[]`、`cert_name`、`zone_id`/`zone_name`、`site_id[]`/`site_name[]`、`accelerator_id`/`listener_id` 等字段（兼容旧数据与后端构造），但**新版表单不再写入站点 / 域名**：部署与对比时按凭证实时拉取真实列表。

## 3. 新建部署目标

`DeployTargetForm` 是三步向导：

1. **基础**：名称、厂商、服务（服务选项由厂商决定）。
2. **凭证**：选来源，再选对应记录。
3. **确认**：区域（下拉取 `frontend/src/utils/region.ts`，可自定义输入）+ 备注。

区域是预置候选 + 可手填：阿里云默认 `cn-hangzhou`，腾讯云 `ap-guangzhou`，华为云 `cn-north-4`。阿里云 ESA 只允许 `cn-hangzhou` 与 `ap-southeast-1`（SDK endpoint 限制，选其它 region 连不上）。

保存后目标出现在列表，`is_active` 默认 `true`。具体部署到哪个域名 / 站点，在详情页执行时现场选择（`ListCDNDomains` 拉列表，EdgeOne / ESA / 面板需要先选站点再拉域名，走 `FetchCDNDomains`）。

## 4. 执行部署

三个入口，底层都是 `DeployService`：

- 部署目标详情：对某个目标执行一次，参数 `DeployCertificate(targetID, certID, domain, siteID)`。
- 证书详情：`DeployAllForCert(certID)`，把这张证书推到所有已关联目标。
- 列表批量：前端循环调用，逐目标返回结果。

内部流程（`internal/deploy/service.go`）：

1. `loadCredsAndConfig`：按来源加载凭证与目标配置（解密？没有——配置是明文 JSON）。
2. `UploadCert`：把证书 PEM/KEY 上传到云厂商证书服务，返回 `cloud_cert_id`。
   - **去重**：先查内存缓存，再查 `cert_uploads`（键：`provider + access_key_id + region + cert_fingerprint`）。命中就跳过上传，直接复用云端证书 ID。
   - 指纹是证书内容的 SHA-256；续期后证书变化，指纹变化，会重新上传。
3. `DeployCert`：把云端证书 ID 绑定到目标资源（CDN 域名 / WAF 域名 / ELB 监听器 / 面板站点）。
4. `recordLog`：写 `deploy_logs`（成功 / 失败、消息、云端证书 ID、原始响应），并更新目标的 `last_status`、`last_error`、`last_deployed_at`。
5. 失败会发通知，并把错误消息落库；云厂商的业务错误通常 HTTP 200 + 错误码，所以 `response` 字段会保留原始响应体，排障先看它。

> **关联语义**：部署目标与证书在界面上像「多对多」，但库层面是 `certificates.deploy_target_certificates` 一个可空外键（见 [../database.md](../database.md) 第 3 节）。`LinkCert` 会把证书的外键改指到新目标——一张证书同时只归属一个部署目标。需要「一张证书到多个目标」时，当前实现做不到，只能逐个目标手动部署。

## 5. 当前证书对比

部署目标详情页可以并排看「本地证书」与「云端/面板当前生效证书」：

- 调用 `GetCurrentCerts(targetID)`，返回 `map[资源key]CurrentCertResult`。
- 所有已注册部署器都实现了 `currentCertGetter`；查询结果是 `CurrentCert{common_name, sans, issuer, not_before, not_after, serial_number}`，未实现时 `Supported=false`（保留给后续部署器）。
- 配置里没存站点 / 域名时，会先按凭证实时拉列表再查（`buildLiveCertResources`）。
- 限流：并发 5、全局 5 QPS（`currentCertMaxConcurrency`、`currentCertMaxQPS`），因为部分厂商一次查询要调 2 个 API（如腾讯 EdgeOne、阿里 GA、百度）。
- 对比依据看 `serial_number` 与 `not_after`：一致说明云端就是本地这张；不一致通常是还没部署，或云端被别的流程改过。

## 6. 日志与排查

- 页面：目标详情 → 部署历史（`ListDeployLogs`），含时间、结果、消息、云端证书 ID。
- 文件：`~/.certflow/logs/certflow.log`；DEBUG 下还有 `ent.log`；出站请求可在 `httplog.db` 看到（需 DEBUG）。
- 典型错误：
  - **401 / 403**：AK/SK 错、子账号无权限、IP 白名单、区域不匹配。见 [../troubleshooting.md](../troubleshooting.md) 第 3 节。
  - **域名不在账号下**：`ListCDNDomains` 拉不到，或部署时报资源不存在。
  - **面板证书不生效**：面板 API 返回成功但站点未重载 / 证书路径不对，见 [../troubleshooting.md](../troubleshooting.md) 第 4 节。
  - **重复上传**：检查 `cert_uploads` 是否有同指纹记录；换账号（AccessKeyId 不同）会重新上传，这是预期。

## 7. 部署器接口

新增厂商时实现 `internal/deploy/types.go` 的 `Deployer`：

```go
type Deployer interface {
	Provider() string
	UploadCert(ctx, creds, cert, svcConfig) (certID string, rawResponse string, err error)
	DeployCert(ctx, creds, certID, svc, svcConfig) (*DeployResult, error)
	ListDomains(ctx, creds, svc, region, zoneID) ([]string, error)
	ListSites(ctx, creds, svc, region, zoneID) ([]string, error)
}
```

可选接口：`currentCertGetter`（当前生效证书）、`currentCertBatch`（批量查询前重置内部缓存）。注册靠包级 `init()` 里的 `RegisterDeployer(...)`，注册表在 `service.go`。

新增厂商还要同步：

1. `internal/ent/schema/types.go` 的 `DeployProviderTypes`（ent 枚举），并 `make ent`；
2. 前端 `deployProviderConfig.ts`（凭证字段）、`utils/deploy.ts`（服务列表）、`utils/region.ts`（区域候选）；
3. 覆盖部署器测试（参考 `internal/deploy/provider_helpers_test.go` 的纯函数测试）。

## 8. 已知边界

- 部署不会自动发生：申请、续期都不会触发部署，需要手动执行（或在证书详情点「部署到全部目标」）。
- 没有部署回滚：部署日志只记录结果，不回滚云端配置。
- 云端证书不会自动删除：去重表只记映射，不负责清理云厂商侧的旧证书。
- 面板类部署依赖面板 API 的站点标识，站点改名 / 重建后需要重新选站点。
- `uuwaf` 只在前端候选与 ent 枚举里，`internal/deploy` 没有对应部署器，执行会返回 `error.deploy_unsupported_provider`。
