# GitLab 覆盖率（自带能力，自成一套）

本仓库同时托管在多个平台，覆盖率是**各平台各自出数**（各平台文档见该平台自己的说明）。
本文只讲 GitLab 这一套：跑在 GitLab 自己的 runner 上、用 GitLab 自己 registry 里的镜像、
数据只进 GitLab，不与任何其它平台的流程共用引用。

- 入口：`.gitlab-ci.yml`（五个 job：`build-image-amd64` / `build-image-arm64` / `merge-image` 建 CI 镜像，
  `go-coverage` / `node-coverage` 出覆盖率）
- 镜像：`.gitlab/ci/Dockerfile`（GitLab 自建，本平台专用）
- 配置文件：无。GitLab 没有 `.coveralls.yml` 这类仓库内配置，阈值/徽章都在项目设置里
- 产物：`coverage.out`、`coverage-go.xml`、`frontend/coverage/*`，保留 1 周

## 1. GitLab 的两套机制（互不联动）

官方原文：**read different inputs, produce different outputs, and do not share configuration.
Configuring one does not enable the other.** 所以两条都得配。

| 机制         | 关键字                              | 输入                            | 产出                                                                                  |
| ------------ | ----------------------------------- | ------------------------------- | ------------------------------------------------------------------------------------- |
| 覆盖率上报   | `coverage: '<正则>'`                | job 日志里的一行数字            | job 页覆盖率、MR 小部件覆盖率与涨跌、流水线列表覆盖率列、覆盖率徽章、Analytics 趋势图 |
| 覆盖率可视化 | `artifacts:reports:coverage_report` | **只认 Cobertura XML / JaCoCo** | MR diff 里逐行注释：绿=已覆盖、红=未覆盖、橙=（仅 Cobertura）已加载未执行             |

关键限制：

- `coverage_report` **不认 lcov，更不认 go 的 coverprofile**——这是和 Coveralls 最大的差别。
  Go 要过一层 `gocover-cobertura`，前端让 vitest 多出一个 `cobertura` reporter。
- `coverage_report` **不产生百分比**，`coverage:` **不产生行级注释**。
- 行级注释只出现在 **MR diff 中改动的行**上；没改动的文件即使报告里有也不注释。

## 2. 六个 job

### 2.1 CI 镜像：`check-image` / `build-image-amd64` / `build-image-arm64` / `merge-image`

见 §4.1。四个 job 都在 stage `image`，串成一条链：`check-image` 算 `.gitlab/ci/Dockerfile` 的内容哈希、
去 registry 里查对应 tag 在不在 → 缺哪个架构建哪个（各自在自己的 runner 上**原生**构建）→
`merge-image` 把可用的单平台镜像合成 manifest list `$CI_REGISTRY_IMAGE/ci:latest`。
Dockerfile 没改且镜像在时，四个 job 加起来约 1 分钟就全绿。

### 2.2 `go-coverage`

```yaml
- PKGS=$(go list ./internal/... | grep -v '/internal/ent' | paste -sd, -)
- go test -vet=off -coverpkg="$PKGS" -coverprofile=coverage.out ./internal/...
- go tool cover -func=coverage.out | tail -1        # total:	(statements)	2.4%
- gocover-cobertura < coverage.out > coverage-go.xml   # 预装在 CI 镜像里，见 §4.1
coverage: '/^total:\s+\(statements\)\s+([0-9.]+)%$/'
```

- 测试命令与 `.cnb/workflows/push-coverage.yml`、`docs/coveralls.md` 里**逐字一致**：`-vet=off`
  （`i18n.T()` 作 printf 参数会触发 vet 的 non-constant format）、`-coverpkg` 排除 `internal/ent`
  （生成代码占约 73% 语句块，计入会把指标稀释到失去意义）。
- `gocover-cobertura` 钉 `v1.5.0`，且**装进 CI 镜像**（`ARG GOCOVER_COBERTURA_VERSION`），不在 job 里
  `go install`——本流水线把 `GOBIN` 指到了 `$CI_PROJECT_DIR/.cache/bin`，job 里 `go install` 会装到那儿，
  而 PATH 上只有镜像构建期的 `/go/bin`，结果就是 `command not found`（exit 127）。**实测：它输出的 `filename` 已经是仓库相对路径**
  （`filename="internal/i18n/i18n.go"`），不需要再剥模块前缀——这与 `docs/coveralls.md` 3.3①
  「coverprofile 不要剥前缀」不冲突，那是给 goveralls 在磁盘上找文件用的，两回事。

### 2.3 `node-coverage`

```yaml
- pnpm --dir frontend exec vitest run --coverage \
    --coverage.reporter=lcov --coverage.reporter=text \
    --coverage.reporter=cobertura --coverage.reporter=json-summary
- sed -i 's#filename="src/#filename="frontend/src/#g' frontend/coverage/cobertura-coverage.xml
- node -e "...console.log('Total lines: '+t.lines.pct+'%')"
```

- reporter 用 **CLI 覆盖**，不改 `frontend/vitest.config.ts`：本地仍是 `['lcov','text']`，
  CI 多要 cobertura / json-summary。这样本地开发产物不变。
- **实测产物名** `frontend/coverage/cobertura-coverage.xml`；里面的 `filename="src/App.vue"`
  是 **frontend 相对路径**，GitLab 按仓库根匹配，所以要补 `frontend/` 前缀（43 个 class 全命中，
  无残留）。
- 前端实测覆盖率 **2.05%**（`all: true` 把未测文件按 0% 计入分母，口径刻意不虚高），
  不是「前端高、Go 低」——两侧都在个位数区间。

## 3. 为什么只有 `go-coverage` 配了 `coverage:`

**GitLab 会把同一次流水线里所有带 `coverage:` 的 job 的百分比取算术平均**，显示在 MR 小部件和
徽章上。Go 是**语句**覆盖率、前端是**行**覆盖率，两个口径平均出来是一个既非 A 也非 B 的数，
趋势变化也解释不清，所以默认只让 Go 提供这个数（与 CNB 侧「go 做红线」的判断一致）。

要改显示前端：把 `node-coverage` 里的 `# coverage:` 取消注释，同时注释掉 `go-coverage` 的，
二选一，别两个都开。

## 4. 前置条件（不满足会静默不跑）

1. **项目要开 CI/CD。** pull mirror 仓库默认可能是关的：Settings → CI/CD → General pipelines；
   镜像仓库还要勾 **Run pipelines for mirrored repositories**（否则镜像同步了也不触发流水线）。
2. **要开 Container Registry。** CI 镜像由 GitLab 自己构建并存在 `$CI_REGISTRY_IMAGE/ci`，
   需要 Settings → General → Visibility, project features → **Container Registry 打开**。
   不需要任何 secret：同实例 registry，Runner 用 `CI_JOB_TOKEN` 自动认证。

3. 容器内 `git` 会因 dubious ownership 报错（root 进程 + 非 root 属主目录），`before_script` 里
   已加 `git config --global --add safe.directory "$CI_PROJECT_DIR"`。

### 4.1 CI 镜像：GitLab 自己构建（`.gitlab/ci/Dockerfile`）

镜像不跨平台拉，GitLab 侧自成一套：

| 项         | 值                                                                                                                             |
| ---------- | ------------------------------------------------------------------------------------------------------------------------------ |
| Dockerfile | `.gitlab/ci/Dockerfile`（本平台专用；本身已支持 aarch64，见其中 `uname -m` 分支）                                              |
| 构建 job   | `check-image` → `build-image-amd64` / `build-image-arm64`（并行，缺哪个建哪个）→ `merge-image`，都在 stage `image`             |
| 构建引擎   | `moby/buildkit:rootless` + `buildctl-daemonless.sh`，**不需要 privileged、不需要 dind**；两架构各自**原生**构建，不用 QEMU     |
| 产物       | `ci:latest` = manifest list（linux/amd64 + linux/arm64）；单平台 tag 内容寻址：`ci:sha-<Dockerfile sha256 前 12 位>-<arch>`    |
| 合并       | `merge-image` 用 `docker buildx imagetools create`（纯 registry API，不需要 daemon）                                           |
| 缓存       | registry cache，**分架构**：`ci:buildcache-amd64` / `ci:buildcache-arm64`                                                      |
| 触发       | 默认分支 push 自动判断（内容哈希 tag 在不在 registry 里，缺了就建）；`CI_FORCE_REBUILD_IMAGE=true` 强制重建；MR / 其它分支不建 |

**为什么不做「一条命令出多架构」**：官方文档给的是 `--opt platform=linux/amd64,linux/arm64`，
但那条路要靠宿主机的 binfmt/QEMU 模拟另一个架构。本镜像的 `RUN` 是 apt 装 GTK4 + WebKitGTK、
`go install` 十几个工具，模拟执行慢 5~10 倍（十几分钟 → 一小时起），GitLab.com 免费额度
**400 分钟/月**根本撑不住；而且 amd64 宿主机是否注册了 aarch64 binfmt 不可控，没注册就是
`exec format error`。所以改成「各架构用自己的原生 runner 各建一次 + 合并 manifest」：

| 架构  | runner tag               | 说明                                        |
| ----- | ------------------------ | ------------------------------------------- |
| amd64 | `saas-linux-small-amd64` | 默认免费 runner，2 vCPU / 8 GB              |
| arm64 | `saas-linux-small-arm64` | 免费层可用，2 vCPU / 8 GB，机房 us-central1 |

两个 build job **并行**，wall time 与单架构相同；代价是 **compute minutes 翻倍**（每次 Dockerfile 变更
约 2×10 分钟）。如果项目没有 arm64 runner 权限（`build-image-arm64` 一直 pending），把它的 `rules`
设成 `when: never` 即可——`merge-image` 的 `needs` 是 `optional: true`，且会按实际查到的 tag 合并，
只有 amd64 时也能出 index。

`ci:latest` 是 manifest list，`docker pull` 会按当前机器架构自动选；CI 里 `default.image: $CI_IMAGE`
同理，所以 coverage job 不管跑在哪种 runner 上都不用改配置。

关于构建引擎——GitLab 官方文档《Use BuildKit to build Docker images》的对照表：

| 方法              | 安全要求                   | 使用的命令               |
| ----------------- | -------------------------- | ------------------------ |
| BuildKit rootless | **不需要 privileged 容器** | `buildctl-daemonless.sh` |
| Docker Buildx     | 需要 `docker:dind`         | `docker buildx`          |
| Native BuildKit   | 需要 `docker:dind`         | `buildctl`               |

原文还写明：rootless 方式是 **Kaniko 的替代品**，且 "Hosted runners on GitLab.com permit these calls
and need no extra configuration"。所以 GitLab.com 上开箱可用，不用自备 runner。

`build-image-*` / `merge-image` 的几个必要细节（写错就挂）：

- `entrypoint: [""]` **必须覆盖**——`moby/buildkit:rootless` 默认启动常驻 daemon，
  不覆盖的话 job 会一直挂着直到超时。
- `BUILDKITD_FLAGS: --oci-worker-no-process-sandbox` 是 rootless 的必需参数。
- 认证要手写 `~/.docker/config.json`（rootless 模式不会自动用 `CI_JOB_TOKEN`）：

  ```yaml
  - echo "{\"auths\":{\"$CI_REGISTRY\":{\"username\":\"$CI_REGISTRY_USER\",\"password\":\"$CI_REGISTRY_PASSWORD\"}}}" > ~/.docker/config.json
  ```

- job 级 `before_script` 会**整体替换** `default.before_script`，所以这个 job 里没有
  `git safe.directory`（它不读 git，无所谓）。
- **`--output` 只推一个 tag**：`name=$CI_REGISTRY_IMAGE/ci:sha-$IMAGE_TAG-$CI_IMAGE_ARCH`。写成
  `name=repo:a,repo:b` 会直接失败——它的值按逗号切成多段 `key=value`，第二段不是 `key=value`，
  报 `error: invalid value registry.gitlab.com/dtapps/certflow/ci:49d1c92c`（实测踩过）。
  想额外打可读 tag 不要紧，用 `merge-image` 里的 `imagetools create -t` 补，别在 buildctl 里写两个。
- **不写 `--opt platform`**：每个 job 跑在自己架构的 runner 上，构建目标天然就是本机架构，
  `buildctl` 不需要（也不该）指定平台。线上镜像对应哪份 Dockerfile，看 tag 里的哈希即可。

**什么时候会重建**（判断依据是「Dockerfile 内容哈希对应的 tag 在不在 registry 里」，不是 commit 差分）：

| 场景                                                                         | 会怎样                                                                                                                                                 |
| ---------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------ |
| 默认分支 push，Dockerfile 内容变了                                           | `sha-<hash>` 变了 → 两个架构都建 → 合并出新的 `latest`                                                                                                 |
| 默认分支 push，Dockerfile 没变、镜像在                                       | `check-image` 查到 tag 已存在 → 两个 build job 十几秒 `exit 0` 跳过；`merge-image` 仍跑一遍把 `latest` 重新指向（约 30 秒，顺带修好被误删的 `latest`） |
| 默认分支 push，Dockerfile 没变、镜像**不在**（从没建成 / 上次失败 / 被清理） | tag 查不到 → **自动补建**。这就是自愈，不用人工干预                                                                                                    |
| MR / 其它分支 push                                                           | 整个 image stage 不创建，coverage 直接拉默认分支上已有的 `latest`                                                                                      |
| Run pipeline 传 `CI_FORCE_REBUILD_IMAGE=true`                                | 无视 tag 是否存在，两架构重建（刷基础镜像、上次构建失败时用）                                                                                          |

**为什么不用 `rules:changes` 判断要不要重建**（这就是 `manifest unknown` 那次的根因）：
`rules:changes` 比较的是「这次 commit 相对上一次有没有改 Dockerfile」，而不是「registry 里到底
有没有拿得到的镜像」。这两件事不等价——镜像一次都没建成过时，你只改 `.gitlab-ci.yml` 推上来，
Dockerfile 没变 → build job 不创建 → coverage 拉 `ci:latest` 直接 `manifest unknown` 失败，而且
**下一条流水线还是同样结果**，死循环，只能人工 Run pipeline 才跳得出来。另外 `changes` 在
schedule / web / API / tag 这类没有 push 事件的流水线里恒为 true（无从比较 `CI_COMMIT_BEFORE_SHA`），
写成 `- if: web` + `- changes:` 就变成每次都重建。

现在两者都不依赖：`check-image` 用 `sha256sum .gitlab/ci/Dockerfile | cut -c1-12` 算内容哈希，
拿 `sha-<hash>-<arch>` 去 registry 里查，缺了就建、有就跳过——判断的是**客观事实**，不是「这次改没改」。

**代价**：每次默认分支 push 都会多出 `check-image`（约 20 秒）+ 两个空跑的 build job（各约 20 秒）

- `merge-image`（约 30 秒），约 1 分钟固定开销；换来的是「镜像缺了自动补」和「不白烧 10 分钟构建」。

**Dockerfile 归属**：`.gitlab/ci/Dockerfile` 是 GitLab 专用的一份，其它平台各自也有一份，互相不引用——
任何一个平台的 registry / runner 出问题都不会拖累别的平台。代价是升 Go / Node / pnpm 版本时每份都要改。

## 5. GitLab 特有的坑

| 现象                                                  | 原因 / 处理                                                                                                                                                                                       |
| ----------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 流水线根本没有触发                                    | 项目没开 CI/CD，或镜像仓库没勾 "Run pipelines for mirrored repositories"                                                                                                                          |
| `coverage` 列是空的                                   | `coverage:` 正则没命中日志。GitLab 按**行**匹配，`^...$` 锚定到单行；先本地跑一遍命令看输出格式                                                                                                   |
| 百分比 = 两个 job 的平均                              | 两个 job 都配了 `coverage:`。见 §3，只留一个                                                                                                                                                      |
| MR 里没有行级注释                                     | ① 报告不是 Cobertura/JaCoCo；② `filename` 不是仓库相对路径（见 §2）；③ 该文件在本 MR 没改动；④ 镜像仓库没有 MR，本来就看不到                                                                      |
| `jobs config should contain at least one visible job` | 所有 job 都被 `rules` 过滤掉了（当前规则：MR 事件 + 默认分支 push）                                                                                                                               |
| 缓存命中但依赖还是全量装                              | `cache.paths` 只能写**项目目录内**路径，故 `GOMODCACHE`/`GOBIN`/pnpm store 全部改到 `$CI_PROJECT_DIR/.cache/`                                                                                     |
| 覆盖率报告解析失败、job 红                            | `coverage_report.path` 写错（Go 是 `coverage-go.xml`，前端在 `frontend/coverage/` 下）                                                                                                            |
| 拉镜像 `unauthorized` / `manifest unknown`            | ① 项目没开 Container Registry；② 镜像确实还没建成。新流水线里 `check-image` 会自动补建（先跑一次默认分支 push 即可）；急着用就 Run pipeline 带 `CI_FORCE_REBUILD_IMAGE=true`                      |
| `build-image-*` 一直跑到超时、无日志输出              | 忘了 `entrypoint: [""]`，container 里起的是 BuildKit 常驻 daemon                                                                                                                                  |
| `改完 Dockerfile 没重建` / `每次都重建`               | 现在按内容哈希判断，理论上不会误判；若真不建，看 `check-image` 日志里算出的 `IMAGE_TAG` 与 `imagetools inspect` 的结果。注意 MR / 其它分支**故意**不建（避免覆盖 `latest`）                       |
| `error: invalid value <registry>/ci:<sha>`            | `--output` 里写了两个 tag：逗号被当成属性分隔符。只留一个 `name=`，见 §4.1                                                                                                                        |
| `build-image-arm64` 一直 pending                      | 项目没有 arm64 runner 权限或该 runner 被禁用。把它的 `rules` 设成 `when: never`（`merge-image` 会自动只合并 amd64）                                                                               |
| `exec format error` / `qemu: uncaught target signal`  | 有人把命令改成 `--opt platform=linux/amd64,linux/arm64`，宿主机没注册 aarch64 binfmt。本仓库不走这条路，见 §4.1                                                                                   |
| `merge-image` 报「没有任何可用架构的镜像」            | 两个架构的 `sha-<hash>-<arch>` tag 都不在 registry 里：先看 build job 是失败还是被跳过，再看 `check-image` 的 `IMAGE_TAG` 是否与 build 用的不一致（例如 Dockerfile 在流水线执行期间又被改过）     |
| `gocover-cobertura: command not found`（exit 127）    | 在 job 里 `go install` 了工具，但 `GOBIN` 被覆盖成 `$CI_PROJECT_DIR/.cache/bin`（PATH 上没有它）。工具二进制一律装进 `.gitlab/ci/Dockerfile`；确需临时装就用 `"$(go env GOBIN)/xxx"` 绝对路径调用 |

## 6. 怎么验证

1. 推一次默认分支，打开 **Build → Pipelines**：`image` stage 四个 job（`check-image` → 两个 build →
   `merge-image`）绿，`go-coverage` / `node-coverage` 绿，`Coverage` 列有数字（如 `2.4%`）。
   镜像建好后在 **Deployments → Container Registry** 里能看到 `ci:latest` 是 manifest list（两个架构）。
2. 打开 `go-coverage` job 日志，搜 `total:`；`node-coverage` 日志搜 `Total lines:`。
3. 徽章：`https://gitlab.com/dtapps/certflow/badges/main/coverage.svg`（私有项目需 token）。
4. 趋势：**Analyze → Repository analytics / CI/CD analytics** 里的覆盖率曲线。
5. 行级：在 GitLab 上开一个真的 MR 改几行 Go/Vue 代码，MR → Changes 里应出现绿/红注释条。

## 7. 顺带发现（未改）

`.cnb/workflows/push-coverage.yml` 里 node 侧的 `pattern: 'cmd/frontend/coverage/lcov.info'`
是 fnmusic-sync 的路径，本仓库实际产物是 `frontend/coverage/lcov.info`。该值不匹配时
`breakIfNoCoverage: true` 会让 CNB 那条 node 流水线报错——建议核对一次（本次未改动，避免超出范围）。
