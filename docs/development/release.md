# 构建与发布

发布由 GitHub Actions 的 `Release` 工作流驱动：手动触发或每天定时触发，六平台并行构建，然后发布到 GitHub，再同步到其它平台。

## 1. 触发参数

`.github/workflows/release.yml` 有两种触发方式：

- `workflow_dispatch`：手动填版本号。
- `schedule`：每天 UTC 00:00（北京时间 08:00）自动构建，等价于手动填 `nightly`。

| 输入      | 说明                                                                                                                                                                                                                                                       |
|-----------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `version` | 版本号，正式版 `1.2.3`（带不带 `v` 均可，tag 统一为 `v1.2.3`）；预发布看后缀：`-rc.1` / `-beta.1` / `-alpha.1` / `-nightly.20260101`；填 `nightly` 自动展开为 `v<最近正式版+1补丁>-nightly.<UTC日期>`（如最近正式版 `v1.2.3` → `v1.2.4-nightly.20260101`） |
| `commit`  | 要构建的提交，留空用默认分支最新提交                                                                                                                                                                                                                       |

**没有 `prerelease` 勾选框**：是否预发布只由版本号后缀决定，避免布尔输入与版本号打架。归一化与判定统一走 `scripts/ci/resolve-prerelease.sh`，GitHub 与 CNB 共用同一份实现。

**手动执行填 `nightly` 与定时任务完全等价**：两条路径都进 `resolve-prerelease.sh`，都会展开成 `v<最近正式版+1补丁>-nightly.<执行当天UTC日期>`（基版本取最近正式版 tag，日期取**执行当天**的 UTC 日期）。差别只有两点：

- 定时任务固定在 UTC 00:00（北京时间 08:00）跑；手动执行是随时，日期按点执行那天算。
- 同一天内手动重跑会得到**同一个 tag**。nightly 允许同日重跑（`release` job 会删掉旧 Release 与 tag 重建）；正式版 / rc / beta / alpha 则不允许——tag 已存在会直接失败，避免把历史 Release 的对比基准挪走。

镜像平台（Gitee / GitLab / GitCode）手动执行时**不接受裸 `nightly`**：它们拿不到 tag 历史，推不出基版本与日期，只接收展开后的最终 tag。

上面这段版本号规则在**所有发布 / 同步工作流的文件头都写了一份**（改规则时五处一起改）：

| 文件                             | 角色                                                                          |
|----------------------------------|-------------------------------------------------------------------------------|
| `.github/workflows/release.yml`  | 发布（GitHub Release）+ 触发各镜像同步                                        |
| `.cnb/workflows/release.yml`     | 同步到 CNB Release（由 `.cnb.yml` 的 `include` 引入；发布配置已从此文件拆出） |
| `.workflow/release.yml`          | 同步到 Gitee Release                                                          |
| `.gitlab/workflows/release.yml`  | 同步到 GitLab Release                                                         |
| `.gitcode/workflows/release.yml` | 同步到 GitCode Release                                                        |

镜像端（Gitee / GitLab / GitCode）只接收**最终 tag**（如 `v1.2.4-nightly.20260101`），传裸 `nightly` 会直接报错——裸 `nightly` 只能由 GitHub 的定时任务展开成带日期的版本。

`prepare` job 输出：`tag`（归一化 tag，如 `v1.2.3-rc.1` / `v1.2.4-nightly.20260101`）、`version`（去掉 `v`，注入 `main.currentVersion` 与产物名）、`app_version`（纯数字，供 NSIS / plist / nfpm）、`pre_release`、`release_title`、`build_time`（`scripts/get-build-time.sh`）与 `git_commit` / `git_commit_full`。

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

1. `prepare`：归一化版本号（`scripts/ci/resolve-prerelease.sh`），算 `tag` / `version` / `app_version` / 预发布标记 / 构建时间 / 提交。
2. `build` / `build-linux`（六路并行）：打包 → 签名 → 重命名 → 平台内 `SHA256SUMS` → 上传 artifact（保留 1 天）。
3. `release`：下载全部 artifact → 预发布时写 `BUILD_TIME`/`GIT_COMMIT` → 统一 `SHA256SUMS`（追加 `# build_time` / `# git_commit` 注释，供更新器判定 nightly 是否真更新）→ `release-changelog.sh` 按渠道选对比基准 → `render-release-body.sh` 生成正文 → 删除同名旧 Release → `softprops/action-gh-release@v3` 用归一化 tag 发布 → nightly 发布后清理旧 Release，GitHub 只保留最近 3 个。版本 tag 必须比已有同渠道版本新且不可重复；nightly 的日期 tag 正常情况下每天新建，同一天重跑时允许删掉重建（镜像平台暂不做同等清理）。
4. 同步 job（全部 `needs: release`，失败只 `::warning::`，不影响 Release；只传归一化 tag，预发布由各端按后缀自行判定）：
   - 每个 job 用一个 `if: always()` 的「记录同步状态」步骤（`id: status`）产出唯一的 `outputs.status`，取值 `triggered`（已触发）/ `skipped`（未配置 secret，触发步骤被跳过）/ `failed`（触发失败或步骤报错）——job 结果区分不了「没配 secret」和「同步成功」，只能靠这一步。
   - `sync-cnb`：延迟 8 秒等 CDN，POST `api.cnb.cool/.../-/build/start`，`event=api_trigger_release`，env 带 `version` 与 `commit`。需要 `SYNC_CNB_TOKEN`（权限 `repo-cnb-trigger:rw`）。
   - `sync-gitee`：POST Gitee Go webhook（`eventType=certflow-release`，params 带 `VERSION` 与 `COMMIT`）。需要 GitHub 仓库 secret `SYNC_GITEE_WEBHOOK_URL` + `SYNC_GITEE_TOKEN`。`COMMIT` 只是搭车带上（构建元数据，不参与判定），Gitee 端没声明同名全局参数就忽略它，**不需要在 Gitee 后台做任何配置**。
   - `sync-gitlab`：`POST /projects/.../trigger/pipeline`，form 里带 `token` / `ref` / `variables[EVENT|VERSION|COMMIT]`。需要 GitHub 仓库 secret `SYNC_GITLAB_TRIGGER_TOKEN`（`glptt-` 项目触发器，不是 PAT）。
   - `sync-gitcode`：dispatch GitCode workflow（body `{ref, inputs:{version, commit}}`，`commit` 必须在 `.gitcode/workflows/release.yml` 的 `workflow_dispatch.inputs` 里声明，否则 dispatch 会被拒）。需要 GitHub 仓库 secret `SYNC_GITCODE_TOKEN`（个人令牌）。**该 job 与 `.gitcode/workflows/release.yml` 保留，但当前不作为发布平台**。
   - **commit 的取舍**：commit 是**构建元数据**（ldflags 注入二进制），与版本号相互独立、不参与任何判定；它本来就写在二进制、Release 正文和 SHA256SUMS 里，镜像端资产也是那份 commit 编出来的。所以**发出去的请求一律带上**（CNB 的 API env、Gitee 的 webhook params、GitLab 的 trigger variables、GitCode 的 dispatch input）。
     - **commit 的真正用途：让下游跑「发布那一版」的代码，而不是分支最新**。发版可能用的是还没进分支头的提交（`inputs.commit`），下游若拉分支最新，跑的就是另一版脚本（下载规则、正文渲染都可能不一致）。
     - 各平台的落地方式（都带回退：取不到就沿用分支头，不阻断同步）：
       - **CNB**：第一个 stage 先 `git checkout <commit>`（依次试本地 → 自己的 `origin`，不跨平台拉），再 `source scripts/ci/resolve-prerelease.sh`。commit 走 API env，**无需平台侧配置**。
       - **Gitee**：step 里先 `git checkout <commit>`，再 `node scripts/ci/gitee-release-*.js`。需要在 Gitee 后台加一个**可选**全局参数 `COMMIT`；不加则沿用分支头（行为与从前一致）。
       - **GitCode**：dispatch 的 `ref` 直接用 commit（跑那一版的 workflow 文件）；镜像还没同步该 commit 导致 dispatch 失败时，自动回退到分支重试一次。**无需平台侧配置**。
       - **GitLab**：不检出（`GIT_STRATEGY: none`，产物直接来自 GitHub Release），commit 只记日志。
5. `notify`：等所有同步 job 结束，用 `SERVERCHAN_SENDKEY` 推送构建完成通知（可选）。通知内容：版本 / 构建时间（UTC+8）/ 提交 / 五个平台的 Release 链接（GitHub / CNB / Gitee / GitLab / GitCode）+ 一行「镜像同步」状态（`CNB ✅ 已触发 · Gitee ⏭ 未配置（跳过） · GitLab ❌ 触发失败 · GitCode ✅ 已触发`，状态取各 sync job 的 `outputs.status`）。版式与 kai 项目保持一致。

## 6. 发布平台范围

按根目录 README 与交接记录：

- **正式发布平台只有四个**：GitHub、CNB、GitLab、Gitee。
- GitCode 当前不发布版本；`.gitcode/workflows/release.yml`、`sync-gitcode` 属备用 / 历史配置。
- Gitee 走 `.workflow/release.yml`（Gitee Go 流水线），由 webhook 触发；也可在页面上改全局参数 `VERSION` 后手动执行。
- CNB 侧发布配置在 `.cnb/workflows/release.yml`（由根目录 `.cnb.yml` 的 `include` 引入，`.cnb.yml` 自身只留 include 列表）：`web_trigger_release`（页面按钮）与 `api_trigger_release`（GitHub 触发）两者共用同一段 `trigger_release`，从 GitHub Release 拉资产 → 删旧 Release/Tag → `git:release` → 上传附件。⚠️ CNB 的 include 不支持跨文件 YAML 锚点，所以 `.trigger_release` 锚点定义与两处 `*trigger_release` 引用必须留在同一个 `release.yml` 里。

GitLab / GitCode 的下游细节（Generic Package Registry、附件预签名、API 版本差异）见 [../gitlab-ci.md](../gitlab-ci.md) 与 `.cnb/workflows/release.yml` 注释；那些是实现备忘，不是日常发布步骤。

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
