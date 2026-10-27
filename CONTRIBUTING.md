# 贡献指南

感谢参与 CertFlow。本文只写「怎么在本仓库改代码、怎么提交」，产品功能见 [README.md](README.md)，架构与数据模型见 [docs/architecture.md](docs/architecture.md)、[docs/database.md](docs/database.md)。

## 1. 环境要求

| 依赖      | 版本 / 说明                                                                            |
|-----------|----------------------------------------------------------------------------------------|
| Go        | 1.27.x（以 `go.mod` 为准）                                                             |
| Node.js   | 25.9.0（`frontend/.nvmrc`）                                                            |
| pnpm      | 12.15.0（`frontend/package.json` 的 `packageManager`）                                 |
| Wails CLI | v3（`go install github.com/wailsapp/wails/v3/cmd/wails3@latest`，或 `make tool-deps`） |
| 系统依赖  | macOS：Xcode Command Line Tools；Linux：GTK4 + WebKitGTK 6；Windows：WebView2          |

`make deps` 只安装 Go / 前端依赖；`make tool-deps` 才会安装 wails3、ent、sqlc、golangci-lint、govulncheck、markdownfmt 等工具。

## 2. 初始化与开发

```bash
make tool-deps # 首次：安装 wails3 / ent / sqlc / golangci-lint 等工具
make deps      # 安装 Go 与前端依赖
make setup     # deps + bindings + ent + sqlc（首次或生成物缺失时）
make dev       # 启动 Wails 开发模式（开发数据目录 ~/.certflow.dev）
```

常用命令：

```bash
make go-build  # 只编译 Go 后端到 bin/certflow
make build     # 构建生产包（make build VERSION=x.y.z）
make package   # 打包当前平台安装包
make check     # 前端类型检查 + Go lint + 单测 + 模糊测试 + 漏洞检查
make test-go   # Go 单测（go test -vet=off -v ./internal/... -count=1）
```

⚠️ 没有 `make test` 这个目标；Go 测试用 `make test-go`，全量检查用 `make check`。

## 3. 改动流程

1. 从 `master` 切分支开发，PR 目标为 `master`。
2. 改到以下内容时，先重新生成再用：
   - Go 服务方法 / DTO → `make bindings`（生成 `frontend/bindings/`）
   - `internal/ent/schema/` → `make ent`
   - `internal/httplog/{schema.sql,query.sql}` → `make sqlc`
   - i18n split 文件 → `make i18n`
3. 提交前本地跑：

   ```bash
   go build -tags production ./...
   (cd frontend && pnpm exec vite build --mode development)
   make check
   ```

   > 根包用 `//go:embed all:frontend/dist`，全新检出先构建一次前端（或建一个占位 `frontend/dist/index.html`），否则 Go 编译与 lint 都会失败。
4. 改过 Markdown 时跑 `make format-markdown`（或 `markdownfmt -w`）。
5. 走 CI：GitHub 的 `push.yml` 会先生成绑定 / ent / sqlc / i18n，再跑 `make format` 与 `make check`；CNB 侧也有对应检查与覆盖率流水线。

### 生成物不要手改

- `frontend/bindings/`（Wails）
- `internal/ent/`（ent）
- `internal/httplog/db/`（sqlc）
- `frontend/src/locales/{zh-CN,en-US}.json`（由 `scripts/merge-frontend-i18n.sh` 合并）
- `internal/i18n/locales/*.json`（由 `scripts/merge-go-i18n.sh` 合并）

## 4. 代码约定

- **国际化**：所有用户可见文本必须走 Go 的 `i18n.T()` / Vue 的 `t()`；新增 key 改 `split/` 下的拆分文件，再 `make i18n`。
- **Go 注释用中文**，Vue 模板文本用 `t()` 包裹。
- **UUID 必须 v7**：`uuid.NewV7()`，导入 `github.com/google/uuid`。
- **数据导入导出**：禁止复制 `.db` 文件，必须走 SQLite 数据搬运；表清单来自 `sqlite_master`；`httplog.db` 不参与迁移。详见 [docs/database.md](docs/database.md) 第 7 节。
- **不要提交**：`~/.certflow` 数据、`*.db`、私钥 / 证书、Token / AK、`coverage.out`、`frontend/coverage`、`frontend/dist`、日志文件。
- 代码风格以 `make format` 与 `make lint-go` 为准；能改代码就不加 lint 豁免。

## 5. 测试约定

- **测试不联网**：不访问真实 ACME / 云厂商 API；HTTP 用 `httptest`，构建期就打云 API 的 provider 用 `t.Skip`。
- **不构造真实 Wails 对象**：`application.App`、通知服务在测试进程里会 panic；只测守卫分支。
- **Go 测试必须带 `-vet=off`**：历史遗留的 `fmt.Sprintf(译文)` 会触发 `non-constant format string`。
- **只跑 `./internal/...`**：根包需要 Wails runtime。
- 覆盖率口径与红线状态见 [docs/development/testing.md](docs/development/testing.md)；当前门禁阈值暂停。

## 6. 提交信息

使用 Conventional Commits，scope 可省略；subject 用中文，描述「做了什么」而不是「改了哪个文件」：

```text
feat(deploy): 新增面板站点证书部署
fix(data): 导入备份时校验表名白名单
docs: 补充架构与数据模型文档
chore(ci): 新增 Codecov 覆盖率上报
test(auth): 补充 Passkey 与 TOTP 用例
```

常用类型：`feat`、`fix`、`refactor`、`perf`、`test`、`docs`、`build`、`chore`、`ci`。依赖升级可用 `build(deps):`（Dependabot 会自动生成）。

## 7. 文档

- 新文档放到 `docs/` 对应分类下，并回填 [docs/README.md](docs/README.md) 索引。
- 文档写「代码里实际成立的行为」，给出来源路径；不确定的标「未实现 / 待确认」，不要猜。
- 所有 Markdown 过 `markdownfmt`。
- 用户手册在 `docs/guides/`，排障在 `docs/troubleshooting.md`，架构决策在 `docs/adr/`。

## 8. 安全

发现安全问题不要开公开 Issue，按 [SECURITY.md](SECURITY.md) 的渠道报告。提交前确认没有把真实域名、Token、私钥、日志带进仓库。

## 9. 许可

提交即表示同意以仓库的 [MIT License](LICENSE) 授权你的贡献。
