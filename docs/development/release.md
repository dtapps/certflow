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

Gitee 镜像端手动执行时**不接受裸 `nightly`**：它拿不到 tag 历史，推不出基版本与日期，只接收展开后的最终 tag。

上面这段版本号规则在**所有发布 / 同步工作流的文件头都写了一份**（改规则时三处一起改）：

| 文件                            | 角色                                                                          |
|---------------------------------|-------------------------------------------------------------------------------|
| `.github/workflows/release.yml` | 发布（GitHub Release）+ 触发各镜像同步                                        |
| `.cnb/workflows/release.yml`    | 同步到 CNB Release（由 `.cnb.yml` 的 `include` 引入；发布配置已从此文件拆出） |
| `.workflow/release.yml`         | 同步到 Gitee Release                                                          |

Gitee 镜像端只接收**最终 tag**（如 `v1.2.4-nightly.20260101`），传裸 `nightly` 会直接报错——裸 `nightly` 只能由 GitHub 的定时任务展开成带日期的版本。

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
   - **commit 的取舍**：commit 是**构建元数据**（ldflags 注入二进制），与版本号相互独立、不参与任何判定；它本来就写在二进制、Release 正文和 SHA256SUMS 里，镜像端资产也是那份 commit 编出来的。所以**发出去的请求一律带上**（CNB 的 API env、Gitee 的 webhook params）。
     - **commit 的真正用途：让下游跑「发布那一版」的代码，而不是分支最新**。发版可能用的是还没进分支头的提交（`inputs.commit`），下游若拉分支最新，跑的就是另一版脚本（下载规则、正文渲染都可能不一致）。
     - 各平台的落地方式（都带回退：取不到就沿用分支头，不阻断同步）：
       - **CNB**：第一个 stage 先 `git checkout <commit>`（依次试本地 → 自己的 `origin`，不跨平台拉），再 `source scripts/ci/resolve-prerelease.sh`。commit 走 API env，**无需平台侧配置**。
       - **Gitee**：step 里先 `git checkout <commit>`，再 `node scripts/ci/gitee-release-*.js`。需要在 Gitee 后台加一个**可选**全局参数 `COMMIT`；不加则沿用分支头（行为与从前一致）。
5. `notify`：等所有同步 job 结束，用 `SERVERCHAN_SENDKEY` 推送构建完成通知（可选）。通知内容：版本 / 构建时间（UTC+8）/ 提交 / 三个平台的 Release 链接（GitHub / CNB / Gitee）+ 一行「镜像同步」状态（`CNB ✅ 已触发 · Gitee ⏭ 未配置（跳过）`，状态取各 sync job 的 `outputs.status`）。版式与 kai 项目保持一致。

## 6. 发布平台范围

按根目录 README 与交接记录：

- **正式发布平台只有三个**：GitHub、CNB、Gitee。
- Gitee 走 `.workflow/release.yml`（Gitee Go 流水线），由 webhook 触发；也可在页面上改全局参数 `VERSION` 后手动执行。
- CNB 侧发布配置在 `.cnb/workflows/release.yml`（由根目录 `.cnb.yml` 的 `include` 引入，`.cnb.yml` 自身只留 include 列表）：`web_trigger_release`（页面按钮）与 `api_trigger_release`（GitHub 触发）两者共用同一段 `trigger_release`，从 GitHub Release 拉资产 → 删旧 Release/Tag → `git:release` → 上传附件。⚠️ CNB 的 include 不支持跨文件 YAML 锚点，所以 `.trigger_release` 锚点定义与两处 `*trigger_release` 引用必须留在同一个 `release.yml` 里。

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

| 现象                      | 原因 / 处置                                                                                                  |
|---------------------------|--------------------------------------------------------------------------------------------------------------|
| Release 建了但没附件      | artifact 下载失败或重命名步骤没产出，看 `build` job 的 `ls -lh`                                              |
| 更新提示「无更新」        | 中文走 CNB；确认 CNB 已有同名 Release 与 `updater-*` 资产                                                    |
| mac 包打不开              | 未签名 / 未公证；检查三个 Apple Secret 是否齐全                                                              |
| Windows 安装包被拦        | 未签名；SmartScreen 属预期                                                                                   |
| 同步 job 全绿但镜像没更新 | `if: env.X != ''` 整步跳过；确认 Secret 在 job 级 `env` 里可见                                               |
| Linux 构建失败            | 缺 webkit2gtk 等系统依赖，用官方 runner 或 `ci-image` 镜像                                                   |
| 公告没自动创建            | 该 stage 只告警不阻断；看 stage 日志里的 HTTP 码；也可在 Issue 页面用「发版公告」模板手工补一条              |
| GitHub 侧没发公告         | 仓库没打开 Discussions（分类接口 404）；看 announce job 的 `::warning::`；确认 Settings 里已开启 Discussions |

## 10. 发版公告 Issue

Release 发布成功后，CNB 侧流水线会**自动开一条 Issue** 作为发版公告（模板见 `.cnb/ISSUE_TEMPLATE/release-announcement.yml`）。

| 项        | 说明                                                                                                                                                                                                               |
|-----------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| 触发位置  | `.cnb/workflows/release.yml` 的「准备发版公告」+「创建发版公告」两个 stage（`web_trigger_release` 与 `api_trigger_release` 共用）                                                                                  |
| 建 Issue  | 官方 `cnbcool/cnb-cli` 插件（`cnb --module issues --tool create-issue`），CNB_TOKEN 由流水线自动注入                                                                                                               |
| 正文来源  | `scripts/ci/render-announcement-body.sh` 生成；下载表 / 校验 / 更新内容取自 Release 正文（`release_body.md`），不另编一套                                                                                          |
| 标题      | `[发版公告] CertFlow <版本号>`                                                                                                                                                                                     |
| 标签      | `发版公告`；标签不存在时先用官方 CLI 建好，带标签被拒则去掉标签重试，公告照发                                                                                                                                      |
| 幂等      | 同一版本重跑（Release 会先删后建）时改写已有公告，不重复开一条                                                                                                                                                     |
| 跳过条件  | `nightly` 不建公告（每天一个 tag，否则 Issue 列表会变成日报）：三个公告 stage 都挂 `if` 锚点，`case "${INTERIOR_VERSION}" in *-nightly.*) exit 1` 即跳过（CNB 的 `if` 是一段 shell 脚本，退出码 0 才执行该 stage） |
| 失败处理  | 只告警、不让 stage 变红（发布已完成，变红会被 GitHub 侧 `sync-cnb` 误记成「同步失败」）                                                                                                                            |
| 评论      | 公告建好后追加一条评论，@ 本仓库自定义 NPC「发版日志助手」整理用户友好的更新日志；开关与内容见 10.3                                                                                                                |
| GitHub 侧 | 不在 Issue 发公告，改在仓库 **Discussions** 开一条讨论（正文同一份脚本渲染），细节见 10.4                                                                                                                          |

### 10.1 为什么用官方 CLI 而不是直接调 OpenAPI

CNB **没有创建 Issue 的内置步骤**：`git:issue-update` 只能改状态 / 标签 / 处理人（`state` / `label.add` / `assignee.add`）， 既不能创建、也不能改标题与正文（`git:issue-create`、`git:issue-comment` 在官方内置任务清单里并不存在）。 能创建 Issue 的官方工具是 `cnbcool/cnb-cli` 插件——官方 OpenAPI 命令行工具，鉴权、接口路径、字段名都由官方维护。

因此创建 / 更新 / 建标签统一走官方 CLI：

```bash
cnb --module repo-labels --tool post-label  --path '{"repo":"dtapp/certflow"}' --data '{"name":"发版公告",...}'
cnb --module issues --tool list-issues     --path '{"repo":"dtapp/certflow"}' --query '{"state":"open","keyword":"v1.2.3"}'
cnb --module issues --tool create-issue    --path '{"repo":"dtapp/certflow"}' --data '{"title":"...","body":"...","labels":["发版公告"]}'
cnb --module issues --tool update-issue    --path '{"repo":"dtapp/certflow","number":123}' --data '{"title":"...","body":"..."}'
```

### 10.2 两个踩过的坑（改脚本前先看）

1. **`--data` 必须是单行 JSON**：官方 CLI 解析 `--data` 时会先把控制字符（真实换行）转义成两字符的 `\n`， 而 JSON 的空白只能是真实换行——多行 JSON 会被转成非法 JSON，CLI 解析失败后会把整段文本当字符串原样发出去 （服务端收到双重编码的请求体）。所以拼 payload 用 `jq -nc`（紧凑单行），不能用 `jq -n`。
2. **`--path` 不要写成 `"{\"repo\":\"...\"}"`**：多层引号 + 反斜杠转义会在 YAML → shell → argv 之间被吃掉， 实测把 `repo` 传成了 `[object Object]`，请求直接 404。统一用 `printf '{"repo":"%s"}' "${REPO}"` 拼成变量再传。

### 10.3 公告评论（一个 NPC 总开关 + 一个内容变量）

公告 Issue 建好后，流水线会再追加一条评论，@ 本仓库**自定义 NPC「发版日志助手」**，让它把「整理一份用户友好的更新日志」作为评论发回公告里。这是本仓库目前唯一触发 NPC 的地方——不发评论就等于不触发 NPC。

#### 自定义 NPC 角色（`.cnb/settings.yml`）

角色定义在仓库自己的 `npc.roles` 里，不依赖通用的 `npc/CodeBuddy`——好处是整理要求固化在 `prompt` 里，只维护一份，手聊 @ 它也能得到同样规范的日志：

```yaml
# .cnb/settings.yml
npc:
  imports:
    list:
      # ...（原有导入保持不动）
  roles:
    - name: 发版日志助手
      slogan: 整理发版更新日志
      prompt: |-
        你是 CertFlow 的发版日志助手……（忽略测试/文档、按「新增/变更/修复/破坏性变更」分组、
        与公告「更新内容」一致、取不到 git 历史就按公告改写、把日志作为评论发回）
```

- **提及格式**：自定义 NPC 必须写全 `@仓库路径(角色名)`，即 `@dtapp/certflow(发版日志助手)`，不像系统 `@CodeBuddy` 能简写；
- **@ 后的路径用 `{{REPOSLUG}}` 占位符**：评论模板里写 `@{{REPOSLUG}}(发版日志助手)`，「发布公告评论」stage 发评论前用流水线注入的 `CNB_REPO_SLUG` 替换（兜底 `dtapp/certflow`），不写死仓库地址——改名 / fork 后仍能 @ 到本仓库的角色；与其它占位符一样走 sed，分隔符 `|`、替换串 `&` 不可用；
- **无需额外代码**：只定义角色即可，CNB 用默认运行环境 `cnbcool/default-npc:latest` + 内置 `npc:go` 任务兜底，不必写 `.cnb.yml` 事件流水线或 Dockerfile；被 @ 后走 `issue.comment@npc` 事件；
- **prompt 里不要写 `{{...}}` 占位符**：流水线只替换评论正文里的占位符，settings.yml 的 prompt 是静态的；让 NPC 从 @ 它的评论里读真实 tag。

| 环境变量                    | 位置             | 说明                                                                          |
|-----------------------------|------------------|-------------------------------------------------------------------------------|
| `NPC_ENABLED`               | job 级 `imports` | **NPC 全局总开关**（来自 `envs/npc.yml`）；关掉后仓库里所有 NPC 功能都不触发  |
| `ANNOUNCEMENT_COMMENT_TEXT` | job 级 `env`     | 评论正文，@「发版日志助手」并给出要对比的两个 tag；整理要求写在角色 prompt 里 |

`NPC_ENABLED` 识别 `true` / `1` / `yes` / `on`（大小写不敏感），关闭时「准备发版公告 / 创建发版公告 / 发布公告评论」三个 stage 一起被 `if` 跳过——公告不建、评论不发、NPC 不触发；缺省按 `true` 处理，避免 `envs` 暂时取不到时误伤发版。`NPC_ENABLED` 放在公共 `envs` 仓库，便于一处统一关闭多个仓库的 NPC 功能。

> 早先的 `ANNOUNCEMENT_COMMENT_ENABLED` 已删除：它与 `NPC_ENABLED` 语义重复，现在只保留一个总开关。

```yaml
# .cnb/workflows/release.yml 的 .trigger_release
  imports:
    - https://cnb.cool/dtapp/envs/-/blob/main/github.yml
    - https://cnb.cool/dtapp/envs/-/blob/main/certflow.yml
    - https://cnb.cool/dtapp/envs/-/blob/main/npc.yml   # NPC_ENABLED: 'true'
  env:
    ANNOUNCEMENT_COMMENT_TEXT: |-
      @{{REPOSLUG}}(发版日志助手) 请对比标签 {{PREVIOUSTAG}} 和 {{CURRENTTAG}} 的差异（git log / git diff {{PREVIOUSTAG}}...{{CURRENTTAG}}），整理一份用户友好的更新日志，并作为评论发布在这里。
```

实现要点：

- 评论用官方 CLI 发：`cnb --module issues --tool post-issue-comment`（需 `repo-notes:rw`，可信事件的 `CNB_TOKEN` 自带）；
- 评论正文只放 @ 提及 + 两个 tag，不再重复写整理要求（那些已固化在角色 prompt 里，避免两处维护）；更新内容就在那条公告的正文里，NPC 自己会读；
- **开关判断放在 stage 级 `if`，三个公告 stage 共用锚点**：「准备发版公告」「创建发版公告」「发布公告评论」都引用 `if: *npc_workflow`（定义在第一个 stage 上）。它是一段 shell 脚本：先判非 `nightly`，再判 `NPC_ENABLED`，不满足就 `exit 1` 让整个 stage 跳过——脚本逻辑里不再有开关判断，条件也只维护一份；缺省按 `true`。
- **评论也去重**：发之前先 `list-issue-comments`，正文首行已存在就跳过（评论删不掉，重跑不该重复发）；列表查询失败时会打告警说明「无法确认是否发过」，但仍会发一次，避免 NPC 永远收不到活；
- `work_mode` 不传：官方接口这个字段缺省就是 true（通知 NPC 进入工作模式），显式传反而多余；
- 公告没建成（`nightly` / 正文渲染失败）时不会有评论：`nightly` 由 stage 的 `if` 拦掉；正文渲染失败时上一步没落盘 Issue 编号，脚本里跳过；
- 开关关闭（`NPC_ENABLED=false`）时三个公告 stage 整个被 `if` 跳过：公告不建、评论不发、NPC 不触发。

### 10.4 GitHub 侧：发到 Discussions

GitHub 侧不在 Issue 里发公告，而是**在仓库 Discussions 里开一条讨论**（公告是对外的版本说明，讨论区比 Issue 更贴切）， 正文与 CNB 侧同一份脚本渲染，只有下载表指向自家平台。

| 项       | 说明                                                                                                                          |
|----------|-------------------------------------------------------------------------------------------------------------------------------|
| 触发位置 | `.github/workflows/release.yml` 的 `announce` job（`needs: [release, prepare]`）                                              |
| 正文来源 | 取已发布 Release 的正文（`gh api repos/…/releases/tags/<tag> --jq .body`）再交给 `render-announcement-body.sh` 渲染           |
| 发布方式 | 官方 `actions/github-script@v9` + **GraphQL**（`github.graphql`）；GitHub 没有专门「发讨论」的第一方 action                   |
| 标题     | `[发版公告] CertFlow <版本号>`（与 CNB 侧同一份格式，由渲染脚本 `TITLE_FILE` 产出）                                           |
| 分类     | `ANNOUNCEMENT_DISCUSSION_CATEGORY`（默认 `General`）；也可用 `ANNOUNCEMENT_DISCUSSION_CATEGORY_ID` 直接给分类 node id         |
| 令牌     | 默认 `GITHUB_TOKEN`；读不到 Discussions 时用 secret `ANNOUNCEMENT_TOKEN`（fine-grained PAT 勾 Discussions: Read and write）   |
| 幂等     | 按标题找出**所有**同版本讨论，逐条 `updateDiscussion` 更新成同一份内容；**不关闭任何一条**（关掉像是版本被撤）                |
| 跳过条件 | `nightly` **在 job 级 `if: !contains(tag, 'nightly')` 整个跳过**（两个 step 都不跑）；`ANNOUNCEMENT_ENABLED` 关掉则脚本内跳过 |
| 失败处理 | 只 `::warning::` 告警，不让流水线变红（Release 已经发出去了）                                                                 |

⚠️ **仓库 Discussions 只能用 GraphQL，REST 没有对应端点**：REST 里只有 org/team discussions， `/repos/{owner}/{repo}/discussions*` 这条路径不存在——所以 `GET /repos/…/discussions/categories` 无论用 `GITHUB_TOKEN` 还是带「Discussions: Read and write」的 fine-grained PAT，都会稳定返回 404。 四个调用全部改成 GraphQL：

```graphql
query    { repository(owner:$owner, name:$repo) { id discussionCategories(first:25) { nodes { id name } } } }
query    { repository(owner:$owner, name:$repo) { discussions(first:100, after:$cursor) { nodes { id number title } pageInfo { hasNextPage endCursor } } } }
mutation ($repositoryId: ID!, $categoryId: ID!, $title: String!, $body: String!) {
  createDiscussion(input: {repositoryId:$repositoryId, categoryId:$categoryId, title:$title, body:$body}) { discussion { number url } }
}
mutation ($id: ID!, $title: String!, $body: String!) {
  updateDiscussion(input: {discussionId:$id, title:$title, body:$body}) { discussion { number } }
}
```

分类与讨论都用 **node id**（`DIC_…` / `D_…`），不是数字 id。

前置条件：仓库要在 **Settings → General 里打开 Discussions**；令牌需具备 Discussions 读写权限 （`GITHUB_TOKEN` 在部分仓库读不到，配 secret `ANNOUNCEMENT_TOKEN` 覆盖）。

### 10.5 公告清理（定时任务）

公告挂久了没人维护就该收掉。两个平台各有一个定时任务，**每天 0 点（北京时间）**清理超过 7 天的版本公告：

| 平台   | 文件                                                                    | 触发                                                                                                    | 清理对象                                                                                               |
|--------|-------------------------------------------------------------------------|---------------------------------------------------------------------------------------------------------|--------------------------------------------------------------------------------------------------------|
| CNB    | `.cnb/workflows/announcement-cleanup.yml`（由 `.cnb.yml` include 引入） | `master` 下的字符串键 `"crontab: 0 0 * * *"`；时区是 CNB 系统的 `Asia/Shanghai`（写 `0 0` 即中国 0 点） | 带「发版公告」标签、**更新时间**早于 7 天前的开放 Issue；CLI 只负责筛，关闭交给官方 `git:issue-update` |
| GitHub | `.github/workflows/announcement-cleanup.yml`                            | `schedule: "0 16 * * *"`（Actions 的 schedule 只认 UTC，UTC 16:00 = 北京 0 点）                         | 标题以 `[发版公告] CertFlow ` 开头、创建时间超过 7 天的讨论                                            |

要点：

- 过期天数用 job 级 `env: ANNOUNCEMENT_EXPIRE_DAYS`（默认 7）调整；
- CNB 用 `updated_time_end` 筛而不是创建时间：接口没有按创建时间筛的参数。公告 Issue 建出来就不再动， updated ≈ created；只有重发版本会刷新 updated——刚刷新过的公告本来就该继续挂着，这个口径反而更合理；
- GitHub 走 GraphQL 列讨论（REST 没有仓库 Discussions 端点），按 `createdAt` 筛； 关闭用 `closeDiscussion` mutation，**尽力而为**：仓库/令牌不支持时只 `::warning::` 列出待人工关闭的条目；
- **定时任务放在独立文件**（`.cnb/workflows/announcement-cleanup.yml`），由 `.cnb.yml` 的 `include` 引入； ⚠️ include 引入的 crontab 只在 **`.cnb.yml` 本身变更推送**时才会同步，只改模板文件不生效—— 改完记得在 `.cnb.yml` 里做一次无害改动（例如调一下注释）一起提交；
- **关闭走官方内置步骤** `git:issue-update`（`options: {fromFile: close-issue-refs.txt, state: close}`）： 第一步的 CLI 只把过期公告编号写进文本，第二步一次调用批量关闭，比逐条调 CLI 快得多； `fromFile` 的解析规则是文本里出现 ` #123`（**`#` 前必须有空格**），所以每行写成 ` #151` 这种形式； 解析不到编号时（无过期 / 查询失败）写一句不含编号的占位文本，等于空操作。
- **分支键必须是仓库的默认分支**（本仓库是 `master`，不是 `main`）：写错分支定时任务挂在不存在的分支上，永远不会触发；
- 清理失败只告警，不把定时任务标红（下一轮还会再跑一遍）；
- CNB 的定时任务以最后修改该配置的用户身份执行，`CNB_TOKEN` 默认只能访问当前仓库（够用：只关本仓库的 Issue）；

（CNB 侧）手工补一条时，在 Issue 页面选「发版公告」模板提交，字段与自动生成的公告一致。

> ⚠️ 公告正文由「发布用的那个提交」里的脚本渲染。若发布的是早于该脚本的提交，公告会被跳过（stage 会打印提示）。
