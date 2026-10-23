# CI 基础镜像说明（certflow）

本文只讲一件事：**两个平台的 CI 镜像怎么来的、装了什么、怎么改版本**。

- 两份 Dockerfile，同一个目标：把「系统库 + Node + pnpm + Go 工具链」预装进镜像，让 CI 直接复用，不在每次运行里重复 `apt-get`、`go install`、下载 Node。
- 基础镜像相同：`golang:1.27.3-trixie`（自带 Go 工具链与 Debian trixie 的 apt 源）。
- **工具版本全部用 Dockerfile 里的 ARG 钉死**。改版本 = 改 Dockerfile → 触发重建。 不要在流水线里 `go install ...@latest`，那会覆盖钉死的版本、破坏可重现性。

## 1. 两个 Dockerfile

|          | GitHub 侧                                                             | CNB 侧                                                              |
|----------|-----------------------------------------------------------------------|---------------------------------------------------------------------|
| 文件     | `.github/ci/Dockerfile`                                               | `.cnb/ci/Dockerfile`                                                |
| 构建方式 | `ci-image.yml` 用 docker/buildx 构建（只 linux/amd64），**推到 GHCR** | `push.yml` 里 `docker.build:` 每次流水线**现场构建**，不推仓库      |
| 镜像地址 | `ghcr.io/<owner>/<repo>/ci:latest`（另带 `sha-` 短标签）              | 仅当前流水线内使用（CNB 的 `docker.build` 产物）                    |
| 下载加速 | 直连官方源                                                            | 全套国内镜像：APT/Go 模块/npm/pnpm 走腾讯云，Node 下载预留 daocloud |
| 额外工具 | 多装 `gotestsum` / `gojq` / `goveralls`                               | 不装这三个                                                          |

### 1.1 为什么工具集不一样

- `gotestsum` / `gojq`：GitHub 侧 `push.yml` / `codecov.yml` / `coveralls.yml` 用； CNB 侧直接跑 `go test`，不用这两个。
- `goveralls`：只有 GitHub 侧 `coveralls.yml` 用；CNB 的覆盖率走内置 `testing:coverage` 任务（`.cnb/workflows/push-coverage.yml`），不经过 Coveralls。
- 两边改工具版本时**记得对照改另一份**，只改一边会让两个平台的工具链悄悄分叉。

## 2. 镜像内容（两份共有）

| 层        | 内容                                                                    | 说明                                                                                                                       |
|-----------|-------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------|
| 基础      | `golang:1.27.3-trixie`                                                  | Go 工具链 + Debian trixie                                                                                                  |
| 环境变量  | `GOINSECURE=go.dtapp.net`、`GOPATH=/go`、`PATH=$GOPATH/bin:$PATH`       | 私有模块域名免校验；`/go/bin` 已在 PATH，流水线可直接调预装工具                                                            |
| 系统库    | `libgtk-4-dev`、`libwebkitgtk-6.0-dev` + 基础工具（git/make/jq/zip 等） | Wails3 编译 webview 必需（提供 `gtk4.pc` / `webkitgtk-6.0.pc`），否则 `make tool-deps` / `make check` 在 pkg-config 处失败 |
| Node      | 官方二进制包解压到 `/usr/local`（按 `uname -m` 选 x64/arm64）           | 按构建机架构自动选；两平台都只做单架构原生构建，`ARCH` 分支是给「换架构」留的口子                                          |
| pnpm      | `npm install -g pnpm@<PNPM_VERSION>`                                    | CNB 侧额外设置腾讯源 + `ignore-scripts false`                                                                              |
| Go 工具链 | 见下表，全部 `go install ...@<固定版本>`                                | 装到 `$GOPATH/bin`                                                                                                         |
| 收尾      | 卸载 `wget`/`xz-utils`，清 apt 缓存                                     | 减小体积                                                                                                                   |

预装的 Go 工具链（两份共有部分）：

| 工具          | ARG                     | 版本                                                                          |
|---------------|-------------------------|-------------------------------------------------------------------------------|
| wails3        | `WAILS3_VERSION`        | v3.0.0-beta.34                                                                |
| ent / entc    | `ENT_VERSION`           | v0.14.6                                                                       |
| sqlc          | `SQLC_VERSION`          | v1.31.1                                                                       |
| golangci-lint | `GOLANGCI_LINT_VERSION` | v2.15.0                                                                       |
| govulncheck   | `GOVULNCHECK_VERSION`   | v1.9.0                                                                        |
| shfmt         | `SHFMT_VERSION`         | v3.14.1                                                                       |
| dockerfmt     | `DOCKERFMT_VERSION`     | v0.5.4                                                                        |
| yamlfmt       | `YAMLFMT_VERSION`       | v0.21.0                                                                       |
| gojq          | `GOJQ_VERSION`          | v0.12.19（**仅 GitHub 侧**）                                                  |
| markdownfmt   | `MARKDOWNFMT_VERSION`   | v3.1.0                                                                        |
| gotestsum     | `GOTESTSUM_VERSION`     | v1.13.0（**仅 GitHub 侧**）                                                   |
| goveralls     | `GOVERALLS_VERSION`     | v0.0.12（**仅 GitHub 侧**，`coveralls.yml` 直接调用，勿在工作流里再 install） |

## 3. GitHub 侧：GHCR 镜像（`.github/ci/Dockerfile` + `ci-image.yml`）

### 3.1 触发条件

```yaml
on:
  push:
    branches: ["**"]
    paths: [".github/ci/Dockerfile"] # 改 Dockerfile = 改版本 = 重建
  pull_request:
    paths: [".github/ci/Dockerfile"] # PR 只构建校验，不推送
  schedule:
    - cron: "0 0 * * 1" # 每周一 UTC 0 点重建，拉基础镜像的 apt 安全更新
  workflow_dispatch: # 手动触发
```

### 3.2 构建与推送要点

- **只建 amd64**：`platforms: linux/amd64`。三个消费方（`push.yml` / `codecov.yml` / `coveralls.yml`）的 `runs-on` 都是 `ubuntu-latest`，arm64 那份没有任何人拉；而多平台构建里 arm64 那条腿是 QEMU 模拟，`apt` 装 GTK/WebKit + `go install` 十几个工具在模拟下会慢到几十分钟，纯粹白烧 Actions 额度。2026-10-23 起去掉 arm64 与 `setup-qemu-action`（无 QEMU 时 buildx 只能出本机架构，正好符合预期）。
- **加回 arm64 的方式**（真需要在 Apple Silicon 上 `docker run` 这个镜像时）：`platforms` 改回 `linux/amd64,linux/arm64` + 恢复 `docker/setup-qemu-action@v4` 步骤，也接受它慢；或者更快的做法是在 `ci-image.yml` 里加一个 `runs-on: ubuntu-24.04-arm` 的 job 各自原生构建（公开仓库免费），再用 `buildx imagetools create` 合成 manifest。
- **标签**：默认分支推送时打 `latest`；每次构建另推一个 `sha-<短哈希>` 标签用于回滚。
- **缓存**：`cache-from/cache-to: type=gha`，复用 Actions 缓存加速层重建。
- **权限**：`packages: write`；认证用内置 `GITHUB_TOKEN` 登录 ghcr.io。
- **无 secrets**：构建过程不依赖任何仓库 secret。

### 3.3 旧版本清理

每次构建都推一个新 `sha-` 标签，旧标签会无限堆积、撑爆 GHCR 存储配额（本镜像含 GTK/WebKit 开发库，单版本好几 GB）。末尾用 `actions/delete-package-versions@v5` 只保留 `latest` + 最近 5 个版本。

**注意包名**：镜像名是 `ghcr.io/<owner>/<repo>/ci`，但 GHCR 的包名是 `<repo>/ci`（含仓库名段，不是末尾的 `ci`）——`package-name` 写错会找不到包、静默不删。

### 3.4 使用方式

```yaml
container:
  image: ghcr.io/${{ github.repository }}/ci:latest
```

**首次使用前必须先跑一次 `ci-image.yml`** 把镜像推上去，否则 `push.yml` / `codecov.yml` / `coveralls.yml` 的容器起不来。

## 4. CNB 侧：流水线内构建（`.cnb/ci/Dockerfile`）

`.cnb/workflows/*.yml` 里每个 job 声明：

```yaml
docker:
  build: .cnb/ci/Dockerfile
  volumes:
    - /root/.cache/go-build:copy-on-write # Go 构建缓存
    - /root/.local/share/pnpm/store:copy-on-write # pnpm 全局存储
    - /go/pkg/mod:copy-on-write # 仅缓存 Go 模块下载，避免覆盖镜像内预装的 /go/bin 工具
```

- **每次流水线现场构建镜像**，不推远端仓库——没有 latest/sha 标签、没有清理问题， 代价是改动 Dockerfile 后每个 job 都要重新 build（有 Docker 层缓存时增量很快）。
- **三个卷都是 `copy-on-write`**：CI 在容器内对缓存目录的写入不回写宿主，下次构建 仍能复用宿主缓存。`/go/pkg/mod` 单独列一条的原因写在注释里：只缓存模块下载， **不要覆盖镜像内预装的 `/go/bin` 工具**。
- **全套国内加速**：APT 源与 Go 模块/npm/pnpm 走 `mirrors.cloud.tencent.com`； 顶部 `GITHUB_MIRROR`（daocloud）ARG 目前预留未用，Node 直连官方下载。
- **不装** gotestsum / gojq / goveralls（原因见 1.1），覆盖率与报告由 CNB 内置任务处理。

## 5. 改版本的步骤

1. 改对应 Dockerfile 的 ARG（GitHub 侧与 CNB 侧**对照着一起改**，见 1.1）。
2. GitHub 侧：push 后 `ci-image.yml` 自动重建并推 `latest` + 新 `sha-` 标签； 下一次 `push.yml` / `codecov.yml` / `coveralls.yml` 运行即用新镜像。 需要提前验证时手动 `workflow_dispatch` 跑一次。
3. CNB 侧：push 后流水线直接用新 Dockerfile 构建，无需额外动作。

## 6. 排错

| 现象                                                     | 原因 / 处理                                                                            |
|----------------------------------------------------------|----------------------------------------------------------------------------------------|
| 容器 job 起不来，拉不到 `ghcr.io/.../ci:latest`          | 镜像还没推过（先跑 `ci-image.yml`），或默认分支从未构建过                              |
| `make check` 在 pkg-config 报找不到 gtk4 / webkitgtk-6.0 | 镜像里 GTK/WebKit dev 库缺失（两份 Dockerfile 都有，通常是改镜像时误删）               |
| 工具版本和 ARG 对不上                                    | 流水线里有人 `go install ...@latest` 覆盖了预装版本；删掉，改版本只动 Dockerfile       |
| 本地（Apple Silicon）`docker run` 报 `exec format error` | 镜像只出 amd64（§3.2）；加 `--platform linux/amd64` 走 QEMU，或按 §3.2 把 arm64 加回来 |
| GHCR 旧版本没被清理                                      | `package-name` 必须是 `<repo>/ci`，不是 `ci`（见 3.3）                                 |
| CNB 侧构建变慢                                           | 多半是 Docker 层缓存失效（改了靠前的层）；把不常变的层保持在前面                       |
