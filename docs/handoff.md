# CertFlow 交接文档（Context Handoff）

> 更新时间：2026-10-24（lint 已清零） 当前主任务：Go 后端测试覆盖率攻坚（前端尚未开始）

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
9. `frontend/scripts/vitest-json-to-junit.mjs` — 新增 `toRepoRelative()`：Vitest json reporter 的 `testResults[].name` 是绝对路径，直接写进 JUnit `classname` 会让 Codecov Tests 面板显示 `/__w/certflow/certflow/frontend/...`（runner 专属前缀，换 CI 就变一条记录）。现统一转成 `frontend/src/test/x.test.ts`，且**与前缀无关**（`/home/runner/work/...`、`/__w/...`、`/workspace/...`、`/builds/...` 均实测归一到同一值）。详见 `docs/codecov.md` 3.3.1。 ⚠️ 已上传的旧绝对路径记录不会迁移，只能等 Codecov 60 天保留期淘汰；判断是否生效看最新一次 run。
10. `.github/workflows/release.yml` — 在 `sync-cnb` 后新增 `sync-gitlab`、`sync-gitcode`，`notify.needs` 同步补齐。三家用的是同一套语义：发布成功后 POST 触发对方流水线，非 2xx 只 `::warning::`、不影响 Release；未配置 secret 时靠 step 的 `if: env.X != ''` 整步跳过。
    - ⚠️ **`if` 要能读到 token，token 必须放 job 级 `env`**：step 自己 `env` 里的值在 `if` 求值阶段尚未展开，`if: env.X != ''` 会恒为 false（原 `sync-cnb` 就是这种写法，已一并修正）。三处现在都是 job 级 `env` + step 级 `if`，别再退回脚本内判空或 step 级 `env`。
    - **每个平台只需 1 个 secret**：`SYNC_CNB_TOKEN`、`GITLAB_TRIGGER_TOKEN`、`GITCODE_TOKEN`。仓库标识、端点、ref、workflow 名全是脚本开头常量；仓库地址的唯一来源是 README「镜像地址」表——**GitLab 是 `dtapps`（带 s），GitCode / CNB 是 `dtapp`（不带 s）**，写错会 404。
    - GitLab：`POST {API}/projects/{urlencode(PROJECT)}/trigger/pipeline`，`--form token/ref/variables[EVENT|VERSION|PRERELEASE]`；`GITLAB_TRIGGER_TOKEN` 是项目级 trigger token（`glptt-`），不是 PAT；若改用 PAT 需换成 `POST /projects/:id/pipeline` + `PRIVATE-TOKEN` 头。
    - GitCode：`POST {API}/repos/{REPO}/actions/workflows/{WORKFLOW}/dispatches?access_token=...`，body `{ref, inputs:{version, prerelease}}`；`GITCODE_TOKEN` 只能是个人访问令牌（平台无项目级触发器令牌），建议用专用账号签发；**平台侧 workflow 必须声明 `on: workflow_dispatch`**，否则无法 dispatch。
    - **下游实现**：`.gitlab-ci.yml` 的 `release`（stage `release`，`rules: $CI_PIPELINE_SOURCE == "trigger"`，`GIT_STRATEGY: none`，复用 default 自建镜像）与 `.gitcode/workflows/release.yml`（`on: workflow_dispatch`，inputs `version`/`prerelease`）。两者逻辑一致：拉 GitHub Release → 下载全部 assets → 建本平台 Release → 传附件，并把正文里 `github.com/dtapps` 换成自家域名。
      - GitLab 没有 Release 二进制附件 API，附件走 **Generic Package Registry**（`PUT /projects/:id/packages/generic/certflow/{tag}/{file}`），`assets.links` 用 `direct_asset_path` 生成 `/-/releases/{tag}/downloads/{file}` 直链；认证优先 `CI_JOB_TOKEN`，配 `GL_RELEASE_TOKEN`（PAT）可覆盖。
      - ⚠️ package 路径里的文件名必须 `jq -sRr @uri` 编码：产物含空格/中文（`certflow 1.0.0 安装包.exe`），不编码 curl 直接 exit 3，且 `-s` 下无任何日志，表现为静默中断（mock 测试才发现）。
      - GitCode 附件是两阶段：`GET /releases/{tag}/upload_url?file_name=` 取预签名 URL + headers，再 `PUT` 并把 headers 原样带回（`x-obs-*` 少一个就 403）。
      - **GitCode API 版本**：Release 在 `api/v5`，workflow dispatch 在 `api/v8`，混用即 404。
    - **Gitee 无 API 触发**：Gitee Go 只支持 push / PR / 定时触发，官方仍在索要 pipeline API（`oschina/git-osc#IHWZMT`），所以 GitHub 侧**没有** `sync-gitee` job。改为 `.workflow/release.yml`（`name: release`）单文件流水线：Gitee → 流水线 → 改全局参数 `VERSION` → 点「执行」。两个 step：`build@nodejs`（云端容器，**不需要主机组**，`shell@agent` 的 `hostGroupID` 才是必须挂 agent 的）用 python3 标准库下载 GitHub 资产并把文件名去掉版本段（`CertFlow-1.2.3-darwin-arm64.zip` → `darwin-arm64.zip`），再用平台插件 `release@gitee`（`upstreamArtifact` + `assertFiles` + `allowUpdate`）建 Release 传附件。**因此不需要 `GITEE_TOKEN`**，发版只靠插件 + 流水线自身的仓库权限；也没有独立脚本文件（原 `scripts/sync-gitee-release.sh` 已删除）。

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
3. ~~`internal/monitor` / `internal/logging`~~ ✅ 已完成（87.3% / 93.8%）；剩余 0% 点是 `network.queryDNSServer`（端口硬编码 53，无法离线起假 DNS）与 `certificate/manual_dns.go`（依赖 ACME 挑战流程）。
4. **前端覆盖率**：`cd frontend && pnpm test -- --coverage`（先确认 vitest 配置是否已接 coverage provider），然后从 `utils/`、`api/` 这类纯函数开始补。
5. **红线棘轮**：当前 `lines: 48` / `diffLines: 50` 已生效。Go 总量过 62% 后再上调（建议新基线-6），只升不降。注意 `lines: COVERAGE_LINES` 是 `exports` 输出变量，与阈值 `lines:` 不是一个键。
6. **收尾必做**：`go build -tags production ./...` + `make test` + `make i18n`（若改过 i18n split 文件）全绿后再汇报。

## 四、lint 约定（2026-10-24）

- `make lint-go` 当前 **0 issues**，提交前必须保持。
- 测试文件里的"固定假凭证字符串"（gosec **G101**）与"自签证书跳过 TLS 校验"（gosec **G402**）已在 `.golangci.yml` 按 `_test\.go$` 做文件级豁免，**不要**再逐个加 `//nolint`；生产代码检查强度不变。
- 能改代码就不加豁免：`errcheck` 用 `_ =`；废弃 API 换等价新 API（例：`ecdsa.PrivateKey.D` → `key.PublicKey.Equal(&orig.PublicKey)`，`D` 自 Go 1.26 起 SA1019）。
- 已知无关告警：`govulncheck` 报 `golang.org/x/crypto/openpgp`（GO-2026-5932），"不让你的代码受影响"，无需处理。

## 五、关键约束

- **禁止**未经用户同意 `git commit` / `push` / `reset`（AGENTS.md）。
- `internal/data/backup.go` 有 **最高优先级硬约束**（见 AGENTS.md）：禁止复制 `.db` 文件实现导入导出、必须走 SQLite 数据搬运（CSV/ATTACH）、新增 ent 表必须同步 `orderedTables`、导入用事务 + `PRAGMA foreign_keys` 开关、`httplog.db` 不纳入迁移。现有测试已锁定这些行为，改实现前先看 `internal/data/backup_test.go`。
- `*ent.Client` 没有 `.DB()`；读写底层库一律 `sql.Open("sqlite3", sqlite.BuildDSN(path))`。
- UUID 必须 v7（`uuid.NewV7()`，`github.com/google/uuid`）。
- Go 注释中文；所有用户可见文案必须走 `i18n.T()`（Go）/ `t()`（Vue）。
- 全量测试别在 `internal/...` 之外跑（根包需要 Wails runtime，会 panic/挂起）。
