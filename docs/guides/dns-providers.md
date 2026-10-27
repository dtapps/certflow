# DNS 提供商

DNS 提供商有两个用途：

1. **ACME DNS-01 验证**：申请 / 续期时自动写 `_acme-challenge` TXT。
2. **部署凭证复用**：部署目标可以把 `credential_source` 设为 `dns_provider`，直接借用这里保存的云账号密钥（见 [deployment.md](deployment.md)）。

页面：`DNS 提供商`（`/providers`）。数据表 `dns_providers`，`config` 是 JSON。

## 1. 支持范围

以当前代码为准，**表单与后端解析都支持 32 家**：

- 前端表单：`frontend/src/utils/dnsProviderConfig.ts` 的 `dnsProviderTypes` / `dnsProviderConfigSchema`
- 后端类型清单：`internal/dnsprovider/service.go` 的 `GetProviderTypes()`
- 后端配置解析：`internal/dnsprovider/config.go` 的 `Parse`
- lego provider 构造：`internal/certificate/dns_provider.go` 的 `createDNSProvider`

> **枚举范围不一致**：`internal/ent/schema/types.go` 的 `DnsProviderTypes` 只有 19 家（到 `xinnet` 为止），ent 生成的 `provider_type` 枚举校验也只认这 19 个。前端能选后 13 家，但保存时会被 ent 的枚举校验拒绝（`invalid enum value`），表格里能选不等于能存。新增 / 启用厂商时，需要把 `DnsProviderTypes` 补全并 `make ent` 重新生成。

## 2. 字段清单

字段名即 `DNSProviderConfig` 的 JSON key；类型「密码」表示表单以密码框渲染（`secret:"true"` 只作标记，见 [../operations/security.md](../operations/security.md)）。

| 厂商         | `provider_type` | 表单字段                                                                               |
|--------------|-----------------|----------------------------------------------------------------------------------------|
| Cloudflare   | `cloudflare`    | `email`、`api_key`（密码）、`api_token`（密码）；API Token 优先                        |
| 阿里云       | `aliyun`        | `access_key_id`、`access_key_secret`（密码）、`region_id`                              |
| 华为云       | `huawei`        | `access_key_id`、`secret_access_key`（密码）、`region`                                 |
| 腾讯云       | `tencentcloud`  | `secret_id`、`secret_key`（密码）、`region`                                            |
| AWS Route 53 | `aws`           | `access_key_id`、`secret_access_key`（密码）、`region`                                 |
| Google Cloud | `googlecloud`   | `client_id`、`email`、`password`（密码）                                               |
| 百度云       | `baiducloud`    | `access_key_id`、`secret_access_key`（密码）                                           |
| 京东云       | `jdcloud`       | `access_key_id`、`access_key_secret`（密码）、`region_id`                              |
| 火山引擎     | `volcengine`    | `access_key`、`secret_key`（密码）、`region`                                           |
| 腾讯 EdgeOne | `edgeone`       | `secret_id`、`secret_key`（密码）、`region`                                            |
| 阿里云 ESA   | `aliesa`        | `api_key`、`secret_key`（密码）、`region_id`                                           |
| UCloud       | `ucloud`        | `public_key`、`private_key`（密码）、`region`                                          |
| 西部数码     | `westcn`        | `username`、`password`（密码）                                                         |
| 三五互联     | `com35`         | `username`、`password`（密码）                                                         |
| 雨云         | `rainyun`       | `api_key`                                                                              |
| 今天网       | `todaynic`      | `auth_user_id`、`api_key`                                                              |
| DNSLA        | `dnsla`         | `api_id`、`api_secret`（密码）                                                         |
| 51DNS        | `dns51`         | `api_key`、`api_secret`（密码）                                                        |
| 新网         | `xinnet`        | `secret`（密码）、`agent_id`                                                           |
| Porkbun      | `porkbun`       | `api_key`、`secret_api_key`（密码）                                                    |
| Namecheap    | `namecheap`     | `api_user`、`api_key`、`client_ip`                                                     |
| GoDaddy      | `godaddy`       | `api_key`、`api_secret`（密码）                                                        |
| Gandi        | `gandiv5`       | `personal_access_token`（密码）                                                        |
| Dynadot      | `dynadot`       | `api_key`、`api_secret`（密码）                                                        |
| Azure DNS    | `azuredns`      | `subscription_id`、`resource_group`、`client_id`、`client_secret`（密码）、`tenant_id` |
| DigitalOcean | `digitalocean`  | `auth_token`（密码）                                                                   |
| Vultr        | `vultr`         | `api_key`（密码）                                                                      |
| Hetzner      | `hetzner`       | `api_token`（密码）                                                                    |
| Linode       | `linode`        | `token`（密码）                                                                        |
| OVH          | `ovh`           | `application_key`、`application_secret`（密码）、`consumer_key`（密码）                |
| DNSimple     | `dnsimple`      | `access_token`（密码）                                                                 |
| NS1          | `ns1`           | `api_key`（密码）                                                                      |

## 3. 配置如何被使用

1. 保存时 `DNSProviderService.Create/Update` 把结构体序列化成 JSON 存 `config`，并校验「不能为空」（`isConfigEmpty` 会识别全部字段，包括曾经被静默丢掉的 `api_user` / `secret_api_key` / `secret`）。
2. 申请 / 续期时 `internal/dnsprovider/config.go` 的 `Parse` 按 `provider_type` 反序列化成强类型结构体（如 `CloudflareConfig`），再交给 `createDNSProvider` 构造 lego provider。
3. 类型不支持或配置为空会返回 `error.dns_provider_unsupported` / `error.dns_provider_config_empty`。

## 4. 传播检查

`dns01` 的默认客户端会拿「设置 → 网络 → 自定义 DNS」里**所有已启用**的服务器作为递归解析器（`internal/certificate/service.go`）：

- 默认使用系统 DNS；
- 内置可选项有阿里云、腾讯云、华为云、114DNS、百度、360、CNNIC、火山引擎、Google、Cloudflare；
- 可以添加自定义 DNS（ID 自定义、`servers[]` 填 IP）。

传播检查失败通常不是证书问题，而是本地 DNS 缓存或递归服务器没刷新。可以临时启用公共 DNS 再试，见 [../troubleshooting.md](../troubleshooting.md) 第 2 节。

## 5. 与部署的关系

部署目标有两种凭证来源（`credential_source`）：

- `dns_provider`：复用这里某条 DNS 凭证。适合「同一家云厂商既管 DNS 又管 CDN」的场景。
- `deploy_credential`：使用单独的部署凭证（`deploy_credentials` 表）。

注意凭证字段名并不统一（阿里云用 `access_key_id`，火山引擎用 `access_key`，华为云用 `secret_access_key`，腾讯云用 `secret_id`/`secret_key`），部署器读取时会做映射。填错字段的现象是「保存成功但调用返回鉴权失败」，见 [deployment.md](deployment.md) 第 3 节。

## 6. 测试

- `internal/dnsprovider/config_test.go`：32 家配置到强类型的映射。
- `internal/certificate/dns_provider_test.go`：类型到 lego provider 的构造（含「配置空 → 报错」）。
- 涉及真实云 API 的构建型 provider（如 `huawei`、`azuredns`）在测试里会 `t.Skip`，测试不联网。
