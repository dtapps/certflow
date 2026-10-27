# 构建与发布

发布由 GitHub Actions 的 `Release` 工作流驱动：一次手动触发，六平台并行构建，然后发布到 GitHub，再同步到其它平台。

## 1. 触发参数

`.github/workflows/release.yml` 的 `workflow_dispatch` 输入：

| 输入           | 说明                                      |
|----------------|-------------------------------------------|
| `version`      | 版本号（`1.2.3`，不带 `v`）；Tag 会加 `v` |
| `commit`       | 要构建的提交，留空用默认分支最新提交      |
| `prerelease`   | 勾选即预发布                              |
| `nsis_version` | NSIS 版本号，留空用默认                   |

`prepare` job 先算两个值：`build_time`（`scripts/get-build-time.sh`）与 `git_commit`（短 SHA），二者会注入构建，也用于更新比对。

## 2. 六平台矩阵

| OS  runner         | `goos`    | `goarch` | 产物形态                           |
|--------------------|-----------|----------|------------------------------------|
| `macos-26`         | `darwin`  | `arm64`  | `CertFlow.app` → `.zip`            |
| `macos-26-intel`   | `darwin`  | `amd64`  | 同上                               |
| `ubuntu-26.04`     | `linux`   | `amd64`  | AppImage / deb / rpm / pkg.tar.zst |
| `ubuntu-26.04-arm` | `linux`   | `arm64`  | 同上                               |
| `windows-2025`     | `windows` | `amd64`  | `-install.exe`（NSIS）             |
| `windows-11-arm`   | `windows` | `arm64`  | 同上                               |

每个平台跑 `wails3 task <os>:package ARCH=<arch>`，版本 / 构建时间 / 提交 / Token 通过 Task 变量拼进 `-ldflags`（`Taskfile.yml` 与各平台 Taskfile）。macOS 交叉编译 Linux 不可行（需要 CGO + webkit2gtk），所以必须用 CI。

还有一个「配置 SQLite 驱动」步骤（见 [../database.md](../database.md) 第 6 节）：**macOS / Linux 用 mattn（CGO）**，**Windows 全部用 modernc（纯 Go，不要求 C 编译器）**，所以正式包在 Windows 与 macOS/Linux 上跑的可能是不同驱动。

## 3. 签名与公证

仅当仓库配置了对应 Secret 才执行，未配置时整步跳过，不影响构建：

| 平台    | 步骤                                       | Secret                                                                                                                                   |
|---------|--------------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------|
| macOS   | `wails3 task darwin:sign:notarize`         | `APPLE_DEVELOPER_ID_CERTIFICATE`、`APPLE_DEVELOPER_ID_CERTIFICATE_PASSWORD`、`APPLE_NOTARIZER_KEYCHAIN_PROFILE`                          |
| Windows | `windows:sign:installer` 再 `windows:sign` | `WINDOWS_SIGN_CERTIFICATE`（.pfx 的 base64）、`WINDOWS_SIGN_CERTIFICATE_PASSWORD`、`WINDOWS_SIGN_THUMBPRINT`、`WINDOWS_TIMESTAMP_SERVER` |
| Linux   | 无                                         | 发行版不强制验签                                                                                                                         |

未签名 / 未公证的 macOS 包会被 Gatekeeper 拦；Windows 未签名会触发 SmartScreen。两者都属预期，不是构建失败。

## 4. 资产命名与更新文件

重命名步骤在 `bin/renamed/` 生成：

| 类型     | 命名                                                                                                                | 说明                       |
|----------|---------------------------------------------------------------------------------------------------------------------|----------------------------|
| 安装包   | `CertFlow-<版本>-<os>-<arch>-install.exe` / `.AppImage` / `.deb` / `.rpm` / `.pkg.tar.zst` / `.zip`（macOS `.app`） | 首次安装，显示在下载表格   |
| 升级专用 | `updater-CertFlow-<版本>-<os>-<arch>.zip`（macOS/Windows）、`.tar.gz`（Linux）                                      | 自更新用，不在下载表格显示 |

自更新只认 `updater-` 前缀的压缩包（`mirror_updater.go` 的 `AssetMatcher`），安装包不参与替换。macOS 的升级包是**整个 `.app` 的 zip**，因为要保留 Gatekeeper 签名。

每个平台跑一次 `sha256sum` 生成自己的 `SHA256SUMS`，发布 job 再统一重算一份（排除 `SHA256SUMS` 自身；预发布还排除 `BUILD_TIME` / `GIT_COMMIT`）。

预发布额外生成两个文件，供 nightly 判断「是否同一份构建」：

- `BUILD_TIME`：构建时间（UTC）
- `GIT_COMMIT`：短提交

## 5. 发布流程

1. `prepare`：算版本、时间、提交。
2. `build`（六路并行）：打包 → 签名 → 重命名 → 平台内 `SHA256SUMS` → 上传 artifact（保留 1 天）。
3. `release`：下载全部 artifact → 预发布时写 `BUILD_TIME`/`GIT_COMMIT` → 统一 `SHA256SUMS` → 生成 Release 正文（下载表格 + 变更列表）→ 删除同名旧 Release → `softprops/action-gh-release@v3` 建 `v<version>`，`prerelease` 跟随输入。
4. 同步 job（全部 `needs: release`，失败只 `::warning::`，不影响 Release）：
   - `sync-cnb`：延迟 8 秒等 CDN，POST `api.cnb.cool/.../-/build/start`，`event=api_trigger_release`，env 带 `version` / `pre_release`。需要 `SYNC_CNB_TOKEN`（权限 `repo-cnb-trigger:rw`）。
   - `sync-gitee`：POST Gitee Go webhook（`eventType=certflow-release`，参数 `VERSION` / `PRERELEASE`）。需要 `GITEE_WEBHOOK_URL` + `GITEE_TOKEN`。
   - `sync-gitlab`：`POST /projects/.../trigger/pipeline`，form 里带 `token` / `ref` / `variables[EVENT|VERSION|PRERELEASE]`。需要 `GITLAB_TRIGGER_TOKEN`（`glptt-` 项目触发器，不是 PAT）。
   - `sync-gitcode`：dispatch GitCode workflow（body `{ref, inputs:{version, prerelease}}`）。需要 `GITCODE_TOKEN`（个人令牌）。**该 job 与 `.gitcode/workflows/release.yml` 保留，但当前不作为发布平台**。
5. `notify`：等所有同步 job 结束，用 `SERVERCHAN_SENDKEY` 推送构建完成通知（可选）。

## 6. 发布平台范围

按根目录 README 与交接记录：

- **正式发布平台只有四个**：GitHub、CNB、GitLab、Gitee。
- GitCode 当前不发布版本；`.gitcode/workflows/release.yml`、`sync-gitcode` 属备用 / 历史配置。
- Gitee 走 `.workflow/release.yml`（Gitee Go 流水线），由 webhook 触发；也可在页面上改全局参数 `VERSION` 后手动执行。
- CNB 侧入口在 `.cnb.yml`：`web_trigger_release`（页面按钮）与 `api_trigger_release`（GitHub 触发），两者共用同一段 `trigger_release`：从 GitHub Release 拉资产 → 删旧 Release/Tag → `git:release` → 上传附件。

GitLab / GitCode 的下游细节（Generic Package Registry、附件预签名、API 版本差异）见 [../gitlab-ci.md](../gitlab-ci.md) 与 `.cnb.yml` 注释；那些是实现备忘，不是日常发布步骤。

## 7. 本地构建

```bash
# 快速编译（只验证 Go 能过）
make go-build

# 走 Task，注入版本号
wails3 task build VERSION=1.2.3
wails3 task package VERSION=1.2.3

# Makefile 封装
make build VERSION=1.2.3
make package VERSION=1.2.3
```

约定：

- 先 `build` 再 `package`；所有平台都这样。
- 版本号只通过 `VERSION=` 等 Task 变量传，`wails3 task build` 不接受裸 `-ldflags`。
- `make dev` 的 `currentVersion` 是 `dev`，会跳过启动检查更新并使用 `.certflow.dev` 数据目录。
- 本地构建不会替你签名；正式分发以 CI 产物为准。

## 8. 发布前检查清单

- [ ] `make check` 全绿（前端类型、Go lint、测试、模糊、漏洞）。
- [ ] `make i18n` 已跑，split 与主文件一起提交。
- [ ] `make ent` / `make sqlc` / `make bindings` 生成的代码已提交（改了 schema / SQL / Service 时）。
- [ ] 版本号与 README 徽章、更新源一致。
- [ ] 升级文件（`updater-*`）确实生成，`SHA256SUMS` 覆盖到它。
- [ ] 预发布时确认 `BUILD_TIME` / `GIT_COMMIT` 存在。
- [ ] 同步用的 Secret 就位（缺了只是跳过，不会让 Release 失败，但镜像不会更新）。

## 9. 常见问题

| 现象                      | 原因 / 处置                                                     |
|---------------------------|-----------------------------------------------------------------|
| Release 建了但没附件      | artifact 下载失败或重命名步骤没产出，看 `build` job 的 `ls -lh` |
| 更新提示「无更新」        | 中文走 CNB；确认 CNB 已有同名 Release 与 `updater-*` 资产       |
| mac 包打不开              | 未签名 / 未公证；检查三个 Apple Secret 是否齐全                 |
| Windows 安装包被拦        | 未签名；SmartScreen 属预期                                      |
| 同步 job 全绿但镜像没更新 | `if: env.X != ''` 整步跳过；确认 Secret 在 job 级 `env` 里可见  |
| Linux 构建失败            | 缺 webkit2gtk 等系统依赖，用官方 runner 或 `ci-image` 镜像      |
