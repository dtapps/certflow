# 测试

测试分两条线：Go 后端（`go test`）与前端（Vitest）。两者都由 `make` 和 CI 驱动，口径以 CI 为准。

## 1. 命令

| 命令                                | 实际执行                                                                 | 用途           |
|-------------------------------------|--------------------------------------------------------------------------|----------------|
| `make test-go`                      | `go test -vet=off -v ./internal/... -count=1`                            | 后端测试       |
| `make fuzz-go`                      | `go test -vet=off -fuzz=$(FUZZ) -fuzztime=$(TIME 或 30s) ./internal/...` | 模糊测试       |
| `make lint-go`                      | `golangci-lint run ./...`                                                | 静态检查       |
| `make lint-frontend`                | `pnpm --dir ./frontend exec vue-tsc --noEmit`                            | 前端类型检查   |
| `make vuln-go`                      | `govulncheck -show verbose ./...`                                        | 依赖漏洞检查   |
| `make check`                        | 按序执行五项（前端类型检查 → Go lint → 测试 → 模糊 → 漏洞）              | 提交前全量检查 |
| `pnpm --dir frontend test`          | `vitest run`                                                             | 前端单测       |
| `pnpm --dir frontend test:coverage` | `vitest run --coverage`                                                  | 前端覆盖率     |

CI 的 `.github/workflows/push.yml` 会先生成绑定 / ent / sqlc / i18n，再跑 `make format` 和 `make check`；覆盖率另见 [../coverage-go.md](../coverage-go.md)、[../coverage-node.md](../coverage-node.md)。

## 2. 为什么必须 `-vet=off`

包里有历史遗留的 `fmt.Sprintf(译文)` 写法，`go vet` 会判为 `non-constant format string` 直接失败。这些调用是「用译文当格式串」的既有模式，不是 bug 现场，所以测试统一带 `-vet=off`。`.golangci.yml` 里也对 `govet` 的这条规则做了排除，两处口径一致。

不要为了去掉 `-vet=off` 去批量改写调用点——那是一次独立的重构，不是补测试的副产品。

## 3. 写测试的硬约束

这些是踩过坑之后定下来的（`docs/handoff.md` 有同样记录）：

1. **绝不联网**。不访问真实 ACME、真实云 API。HTTP 一律 `httptest`；构建期就要打云 API 的 provider（如 `huawei`、`azuredns`）用 `t.Skip`。
2. **不构造真实 Wails 对象**。`application.App`、`notification.NewNotificationService()` 在测试进程里会 panic（依赖 Wails runtime）。涉及对话框 / 通知的，只测守卫分支（如 `app == nil`）。
3. **认证包用独立临时库**。`internal/auth` 的新用例用 `enttest.Open(t, dialect.SQLite, sqlite.BuildDSN(filepath.Join(t.TempDir(), "x.db")))`，不要复用过期的共享内存库。
4. **断言 i18n 文案要看形状**：检查不含 `error.` / `settings.` / `{{` / `%!`，防止 key 没翻译或格式串写错。
5. **只跑 `./internal/...`**。根包需要 Wails runtime，`go test ./...` 会 panic 或挂起。
6. **测试里不放真实凭证**。固定假字符串可以，`gosec` 的 G101 / G402 已对 `_test.go` 做文件级豁免，不用逐个 `//nolint`。

## 4. Go 覆盖率

### 4.1 当前数字（2026-10-26 实测）

| 项                     | 值                                   |
|------------------------|--------------------------------------|
| Go 总覆盖率（CI 口径） | **55.5%**                            |
| 测试文件               | 70 个，分布在 22 个包                |
| lint                   | 0 issues                             |
| 红线门禁               | **暂停**（只上报数据，不阻断流水线） |

各包大致分布（函数平均，低 → 高）：`notification 27.6%`（Wails，暂缓） < `deploy 55.5%` < `network 77.0%` < `scanner 77.5%` < `db 77.9%` < `certificate 78.2%` < `scheduler 82.8%` < `data 82.9%` < `settings 84.8%` < `auth 85.3%` < `ca 86.4%` < `monitor 87.3%` < `dnsprovider 91.1%` < `logging 93.8%` < `config 97.4%` < `deploycredential 97.7%` < `httplog 97.9%` < `i18n 98.4%` < `sqlite`/`useragent 100%`。

### 4.2 CI 口径

不要用裸 `go test ./...` 算覆盖率，分母会含 `internal/ent` 生成的约 1.5 万行。用这条：

```bash
go test -vet=off -count=1 \
  -coverpkg=$(go list ./internal/... | grep -vE '/internal/ent|/internal/httplog/db' | paste -sd, -) \
  -coverprofile=coverage.out ./internal/...
go tool cover -func=coverage.out | tail -1
```

`internal/ent` 与 `internal/httplog/db` 是生成代码，排除在统计之外，与 `.cnb/workflows/push-coverage.yml` 保持一致。

### 4.3 红线状态

阈值在提交 `5903820` 被注释掉（`lines` / `diffLines`），现在只上报不阻断。恢复时按「当前基线 - 6」设置，且只升不降；注意 `lines: COVERAGE_LINES` 是导出变量，和阈值 `lines:` 不是同一个键。细节与排错见 [../coverage-go.md](../coverage-go.md)。

### 4.4 已知的低覆盖区

- `internal/notification`（27.6%）：Wails 绑定，用户要求暂缓。要补得先抽 `notifier` 接口，生产用真实现、测试注入 fake。
- `internal/deploy`（55.5%）：剩余基本是官方 SDK 流程，endpoint 固定、transport 自带，`httptest` 拦不到。补测前要先给各厂商加 `endpoint` / `httpClient` 注入点（生产改动）。
- `internal/network.queryDNSServer`：端口硬编码 53，离线起不了假 DNS。
- `internal/certificate/manual_dns.go`：依赖真实 ACME 挑战流程。

## 5. 前端测试

- 框架：Vitest + `@vitest/coverage-v8`，配置在 `frontend/vitest.config.ts`。
- 当前：3 个测试文件（`frontend/src/test/{format,i18n,region}.test.ts`）/ 32 个用例；statements **2.14%**、lines **2.05%**（2026-10-26 实测）。
- 覆盖策略：先补 `utils/`、`stores/` 的纯逻辑，再引 `@vue/test-utils` + jsdom 做组件测试；视图组件尚未开始。
- 红线同样暂停，恢复规则与 Go 一致；详见 [../coverage-node.md](../coverage-node.md)。

## 6. 补测试的流程

1. 先看目标包是否已有测试文件，避免重复覆盖（例如 `deploy/provider_helpers_test.go` 已覆盖 `certKeyMatch`、`splitCertChain`、`hostMatchesPattern` 等纯函数）。
2. 只测能离线验证的行为：纯函数、解析、状态机、错误分支、DB 读写（临时库）。
3. 需要注入的依赖优先给生产代码加最小接口 / 参数，而不是在测试里打补丁。
4. 跑 `make test-go`；改过代码再跑 `go build -tags production ./...`。
5. 改过 i18n split 文件要 `make i18n`；并发 / 重复跑用 `-count=1` 避免缓存假通过。
6. 收尾：`go build -tags production ./...` + `make test-go` +（如改过 i18n）`make i18n` 全绿再汇报。

## 7. 常见失败

| 现象                         | 原因 / 处置                                                    |
|------------------------------|----------------------------------------------------------------|
| `non-constant format string` | 没带 `-vet=off`                                                |
| 测试 panic 在 Wails 初始化   | 构造了真实 `application.App` / 通知服务，改成只测守卫分支      |
| 认证测试互相污染             | 共用了内存库，改用 `enttest` + 临时文件库                      |
| 覆盖率掉了一截               | 可能把生成代码算进分母，检查 `-coverpkg` 的排除项              |
| `go test ./...` 卡住         | 根包需要 runtime，只跑 `./internal/...`                        |
| 模糊测试报错                 | 用 `make fuzz-go FUZZ=FuzzXxx TIME=30s` 定点复现，再补回归用例 |

CI 专题与阈值细节：[../coverage-go.md](../coverage-go.md)、[../coverage-node.md](../coverage-node.md)、[../codecov.md](../codecov.md)、[../coveralls.md](../coveralls.md)。
