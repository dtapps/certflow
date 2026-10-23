# CertFlow 交接文档（Context Handoff）

> 更新时间：2026-10-24 当前主任务：Go 后端测试覆盖率攻坚（前端尚未开始）

## 一、当前进度

| 阶段                                                                                    | 状态                                                 |
|-----------------------------------------------------------------------------------------|------------------------------------------------------|
| 基线测量（Go + 前端）                                                                   | ✅ 完成                                              |
| 零覆盖小包（config / useragent / deploycredential / sqlite / httplog）                  | ✅ 完成（97%~100%）                                  |
| 核心域包（certificate / scanner / dnsprovider / scheduler / auth / ca / data / deploy） | ✅ 完成（deploy 除外，见下）                         |
| 覆盖率红线棘轮（CI 门禁）                                                               | ✅ 完成（`lines: 48` / `diffLines: 50`，已实测开启） |
| 非 Wails 低覆盖包收敛（monitor / logging / i18n / dnsprovider / certificate 注入点）    | ✅ 完成                                              |
| `internal/deploy` 深水区（各云厂商 SDK 流程）                                           | ⏸ 未开始（55.5%，需先加 endpoint/client 注入点）     |
| `internal/notification`（Wails 绑定）                                                   | ⏸ 用户要求暂缓（27.6%）                              |
| 前端 Vue/TS 覆盖率                                                                      | ⏸ 未开始（基线 2.05%）                               |

**Go 总覆盖率（CI 口径）：27.2% → 55.5%**

CI 口径命令（务必原样使用，别用裸 `go test ./...`，分母会含 ent 生成的 ~1.5 万行）：

```bash
go test -vet=off -count=1 \
  -coverpkg=$(go list ./internal/... | grep -vE '/internal/ent|/internal/httplog/db' | paste -sd, -) \
  -coverprofile=coverage.out ./internal/...
go tool cover -func=coverage.out | tail -1
```

当前各包（函数平均覆盖率，低→高）： `notification 27.6%`（Wails，暂缓） < `deploy 55.5%` < `network 77.0%` < `scanner 77.5%` < `db 77.9%` < `certificate 78.2%` < `scheduler 82.8%` < `data 82.9%` < `settings 84.8%` < `auth 85.3%` < `ca 86.4%` < `monitor 87.3%` < `dnsprovider 91.1%` < `logging 93.8%` < `config 97.4%` < `deploycredential 97.7%` < `httplog 97.9%` < `i18n 98.4%` < `sqlite` / `useragent 100%`

## 二、关键上下文

### 本轮改动的生产代码（都已验证，未提交）
1. `internal/ent/schema/types.go` — `DNSProviderConfig` 补 `APIUser` / `SecretAPIKey` / `Secret`（Namecheap / Porkbun / Xinnet 凭证此前被静默丢弃）。
2. `internal/dnsprovider/service.go`、`internal/certificate/dns_provider.go` — `isConfigEmpty` 同步识别这 3 个字段。
3. `internal/certificate/service.go` — 新增 `loadSignerFromContent`（EC → PKCS1 → PKCS8），吊销路径改用它，修复 RSA 证书无法吊销。
4. `internal/scheduler/service.go` — `GetJob` 增加 `s.scheduler == nil` 守卫，修复 `Start()` 之前调用的空指针 panic。
5. `internal/scanner/service.go` — `issuerName` / `subjectName` 兜底（CN → O / CN → SAN[0] → O）；`getPubKeyBits` 未知算法返回 0 而非 256。
6. `internal/i18n/locales/split/{zh-CN,en-US}/errors.json` — 修 `error.dns_provider_config_empty`、`error.dns_provider_unsupported` 的 `<no value>`（注意：改完要 `make i18n` 合并主文件）。
7. `internal/ca/service.go` — `TestConnection` 不再把译文当 `fmt.Sprintf` 格式串。 7.5 `internal/monitor/service.go` — 新增 `certIssuerName` / `certSubjectName`（CN → Organization / CN → SAN[0] → Organization），`checkHTTPS`/`checkHTTP` 改用，修复现代 CA 证书（无 CN）导致监控列表「颁发者/主体」空白。
8. `.cnb/workflows/{push,pull}-coverage.yml` — Go 覆盖率门禁开启（`lines: 48` / `diffLines: 50`）。注意：`lines: COVERAGE_LINES` 是 `exports` 输出变量，与阈值 `lines:` 不是同一个键。

### 测试基建约定（后续补测必须遵守）
- **绝不联网**：不访问真实 ACME / 云厂商 API。构建期就打云 API 的厂商（`huawei`、`azuredns`）用 `t.Skip`；HTTP 一律 `httptest`。
- **不构造真实 Wails 对象**：`application.App`、`notification.NewNotificationService()` 在测试进程里会 panic（`CheckPermission` 依赖 Wails runtime）。涉及对话框/通知的只测守卫分支（如 `app == nil`）。
- **认证包用独立临时库**：`internal/auth` 原有测试共用 `file:ent?mode=memory&cache=shared`，新用例必须 `enttest.Open(t, dialect.SQLite, sqlite.BuildDSN(filepath.Join(t.TempDir(),"x.db")))`。
- **`go test` 必须带 `-vet=off`**：包内历史遗留 `fmt.Sprintf(译文)` 会触发 `non-constant format string`，`make test-go` 也是这么跑的。
- 断言 i18n 文案时统一检查不含 `error.` / `settings.` / `{{` / `%!`（防译文未翻译或格式串错用）。

## 三、下一步计划（To-Do）

1. **`internal/notification` 27.6%**：⚠️ 用户 2026-10-24 明确「Wails 相关可以先不写测试」，暂缓，不要主动开工。 若之后要做，需先抽接口（下面这段保留作为预案）：
   - 在 `NotificationService` 里把 `*notifications.NotificationService` 抽成 `type notifier interface { ... }` 字段，生产用真实现，测试注入 fake。
   - 然后覆盖 `SendCertApplied` / `SendCertExpiring` / `SendCertFailed` / `SendCertRevoked` / `SendDeployFailed` / `SendDeploySuccess` / `SendNotification` 的分支与 `CheckPermission`。
2. **`internal/deploy` 55.5%**：⚠️ 剩余全是官方 SDK 流程（aliyun/tencent/baidu/huawei/ctyun/volcengine），SDK 固定 endpoint 且自带 transport，`httptest` 拦不到、无注入点。要先给每个厂商加 `endpoint`/`httpClient` 注入参数（生产改动），再补测。
   - 逐厂商起 `httptest` 假服务端：优先 `aliyun`（CDN/ESA/OSS）、`tencentcloud`（CDN/EdgeOne）、`btpanel`、`1panel`、`acepanel`。
   - 注意 `provider_helpers_test.go` 已覆盖纯函数（`certKeyMatch`、`splitCertChain`、`hostMatchesPattern`、`parseAAWafSite`、`acepanel` 辅助等），别重复。
   - 签名算法（HMAC-SHA256）可用固定时间戳 + 已知期望签名做 httptest 断言。
3. **`internal/monitor` 71.3% / `internal/logging` 74.4%**：补 DB 错误分支与日志轮转参数边界。
4. **前端覆盖率**：`cd frontend && pnpm test -- --coverage`（先确认 vitest 配置是否已接 coverage provider），然后从 `utils/`、`api/` 这类纯函数开始补。
5. **红线棘轮第二步**：Go 总量过 60% 后把 `.cnb/workflows/*-coverage.yml` 的 `lines` 从 45 上调（建议新基线-6）。
6. **收尾必做**：`go build -tags production ./...` + `make test` + `make i18n`（若改过 i18n split 文件）全绿后再汇报。

## 四、关键约束

- **禁止**未经用户同意 `git commit` / `push` / `reset`（AGENTS.md）。
- `internal/data/backup.go` 有 **最高优先级硬约束**（见 AGENTS.md）：禁止复制 `.db` 文件实现导入导出、必须走 SQLite 数据搬运（CSV/ATTACH）、新增 ent 表必须同步 `orderedTables`、导入用事务 + `PRAGMA foreign_keys` 开关、`httplog.db` 不纳入迁移。现有测试已锁定这些行为，改实现前先看 `internal/data/backup_test.go`。
- `*ent.Client` 没有 `.DB()`；读写底层库一律 `sql.Open("sqlite3", sqlite.BuildDSN(path))`。
- UUID 必须 v7（`uuid.NewV7()`，`github.com/google/uuid`）。
- Go 注释中文；所有用户可见文案必须走 `i18n.T()`（Go）/ `t()`（Vue）。
- 全量测试别在 `internal/...` 之外跑（根包需要 Wails runtime，会 panic/挂起）。
