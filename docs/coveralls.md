# Coveralls 接入说明（certflow）

本文只讲一件事：**Coveralls 怎么接、怎么跑、怎么排错**。

- 上报入口：`.github/workflows/coveralls.yml`，独立工作流，不依赖其他流程。
- 上报内容：Go 与前端两条覆盖率，分别打 `go` / `node` 两个 flag，合并为一个 build。
- 仓库内**没有** `.coveralls.yml`：阈值、PR 注释、徽章都在 Coveralls 后台仓库设置里配。
- **goveralls 不需要在工作流里安装**：CI 镜像已预装（`GOVERALLS_VERSION=v0.0.12`）。
- **Go 的 coverprofile 不要剥模块前缀**：goveralls 靠它定位源文件，剥了会直接失败（见 3.3）。

## 1. 涉及文件

| 文件                              | 作用                                                                                      |
| --------------------------------- | ----------------------------------------------------------------------------------------- |
| `.github/workflows/coveralls.yml` | 唯一入口：装依赖 → 跑 Go/前端测试 → 分别上报 `flag=go` / `flag=node`                      |
| `.github/ci/Dockerfile`           | 提供 `ghcr.io/<repo>/ci:latest` 镜像（Go/Node/pnpm/GTK/WebKit，**含 goveralls v0.0.12**） |
| `Makefile`（`deps` 目标）         | `go mod download` + `pnpm --dir ./frontend install`                                       |
| `coverage.out`                    | Go 覆盖率产物，**原样**交给 goveralls（路径带模块前缀，不要处理）                         |
| `frontend/coverage/lcov.info`     | 前端覆盖率产物（`pnpm test:coverage` 生成）                                               |

## 2. 工作流结构

### 2.1 触发与并发

```yaml
on:
  push:
  pull_request:

concurrency:
  group: coveralls-${{ github.ref }}
  cancel-in-progress: true
```

同一分支连续 push 会取消上一次运行，避免同一 commit 重复上传。

### 2.2 为什么是单个 job

Coveralls 以 **build** 为单位聚合报告。go / node 两份数据必须落进**同一个 build**，才能靠 flag 合并成一份总覆盖率。

- **单 job**：两次上传天然同 build、同 commit、同 job id，flag 直接合并，确定性最高。
- **多 job + parallel**：需要 `parallel: true` + 最后一个 `parallel-finished: true` 收尾，goveralls 侧还要
  `-parallel -jobid`。两套工具混用 parallel 协议，容易出现「build 一直不 finalize」或「两份报告各算各的」。

当前用单 job 顺序执行，用一点时间换确定性。将来真要拆（比如 Go 测试太慢），写法是：

```yaml
jobs:
  coverage-go: # goveralls -parallel -flagname=go -jobid=${{ github.job }}
  coverage-node: # action: parallel: true, flag-name: node
  finish: # needs: [coverage-go, coverage-node] → parallel-finished: true
```

拆之前先确认 goveralls 版本的 `-parallel` / `-jobid` 行为，别照抄。

### 2.3 运行环境

复用预构建镜像：

```yaml
container:
  image: ghcr.io/${{ github.repository }}/ci:latest
```

镜像里已有 Go / Node / pnpm / GTK4 / WebKit6，以及钉好版本的 Go 工具链：

| 工具      | 版本                        | 来源                     |
| --------- | --------------------------- | ------------------------ |
| gotestsum | `GOTESTSUM_VERSION=v1.13.0` | `go install ...@版本`    |
| gojq      | `GOJQ_VERSION=v0.12.19`     | 同上                     |
| goveralls | `GOVERALLS_VERSION=v0.0.12` | 同上，装到 `$GOPATH/bin` |

`ENV PATH="$GOPATH/bin:$PATH"` 已经包含 `/go/bin`，所以工作流里可以直接调 `goveralls`，
**不需要** `setup-go`、`setup-node`、`apt-get install`——Wails 的 CGO 链接依赖也齐了。
镜像由 `ci-image.yml` 推送，**首次运行前必须已存在**，否则容器起不来。

### 2.4 缓存

| 缓存           | 路径                        | key                                    |
| -------------- | --------------------------- | -------------------------------------- |
| Go module      | `/go/pkg/mod`               | `hashFiles('**/go.sum')`               |
| Go build cache | `$HOME/.cache/go-build`     | `hashFiles('**/go.sum')`               |
| pnpm store     | `pnpm config get store-dir` | `hashFiles('frontend/pnpm-lock.yaml')` |

## 3. Go 覆盖率：goveralls

### 3.1 为什么不用 action

`coverallsapp/github-action@v2` 的 `action.yml` 只有 `path-to-lcov` 一个覆盖率输入，**没有** `file`，
也**没有** `format`。所以不存在 `format: golang`——Go 的 `coverage.out` 不能直接喂给它。

### 3.2 步骤（两步，没有安装、没有路径加工）

```yaml
- name: 生成 Go 测试覆盖率
  run: |
    gotestsum --junitfile go-test-report.xml -- -vet=off -coverpkg=$(go list ./internal/... | grep -v '/internal/ent' | paste -sd, -) -coverprofile=coverage.out ./internal/...

- name: 上传 Go 覆盖率到 Coveralls
  if: env.COVERALLS_REPO_TOKEN != ''
  continue-on-error: true
  run: |
    goveralls \
      -coverprofile=coverage.out \
      -service=github \
      -repotoken="$COVERALLS_REPO_TOKEN" \
      -flagname=go
```

**goveralls 来自镜像，工作流里不要 `go install`。** 镜像用 `GOVERALLS_VERSION=v0.0.12` 预装，
v0.0.12 实测支持本次用到的全部参数：`-coverprofile` / `-service` / `-repotoken` / `-flagname`
/ `-parallel` / `-jobid` / `-shallow` / `-endpoint`。

在工作流里 `go install ...@latest` 是负优化：每次跑都要下载编译、拖慢 CI，还会把镜像钉死的版本
覆盖成某个不确定的 latest，破坏可重现性。升级 goveralls 改的是 Dockerfile 的 ARG，重推镜像。

### 3.3 两个必须知道的细节

**① 模块前缀必须保留，绝不能剥。**

`go test -coverprofile` 写出的路径是模块限定的：

```
cnb.cool/dtapp/certflow/internal/useragent/useragent.go:18.2,21.1 3 0
```

goveralls 的 `toSF()` 走的是这条链：

```
findRootPackage()  → 读 go.mod 的 module 行，得到 cnb.cool/dtapp/certflow
findFile()         → filepath.Rel(模块路径, 文件路径)
                   → internal/useragent/useragent.go（不以 .. 开头，快路径命中）
                   → filepath.Join(os.Getwd(), 上面那个) = 磁盘真实路径
```

**这条快路径只在前缀存在时成立。** 一旦把前缀剥掉：

```
Rel("cnb.cool/dtapp/certflow", "internal/useragent/useragent.go")
  → "../../../../internal/useragent/useragent.go"    // 以 .. 开头，快路径失效
```

于是 goveralls 退化成 `go/build.Import("internal/useragent", ".", FindOnly)`，把**目录当 import path**
丢给 Go 去 GOROOT/GOPATH 里找，报：

```
cannot find file "internal/auth/passkey.go": can't find "passkey.go":
package internal/auth is not in std (/usr/local/go/src/internal/auth)
```

exit 1，整份报告不生成。所以：**`-coverprofile=coverage.out` 直接传，不做任何 sed/替换。**

**② `-service=github`**：让 goveralls 从 `GITHUB_*` / `GITHUB_EVENT_PATH` 取分支、commit、PR 编号。
v0.0.12 里 `service_job_id` 取 `GITHUB_RUN_ID`。

### 3.4 统计口径

只统计 `internal/...`，并在 `-coverpkg` 里排除 `internal/ent`（80 个生成文件，会严重稀释指标）。
`gotestsum` 产出的 `go-test-report.xml` 只用于命令对齐与本地排查，Coveralls **不接收** JUnit。

## 4. 前端覆盖率：coveralls action

```yaml
- name: 生成前端测试覆盖率
  working-directory: frontend
  run: pnpm test:coverage

- name: 上传前端覆盖率到 Coveralls
  if: env.COVERALLS_REPO_TOKEN != ''
  continue-on-error: true
  uses: coverallsapp/github-action@v2
  with:
    github-token: ${{ secrets.GITHUB_TOKEN }}
    path-to-lcov: ./frontend/coverage/lcov.info
    flag-name: node
```

- 输入名是 **`path-to-lcov`**，不是 `file`。
- `flag-name: node` 与 Go 的 `-flagname=go` 配对。
- `github-token` 用内置 `GITHUB_TOKEN`；job 级已注入 `COVERALLS_REPO_TOKEN` 环境变量，私有仓库用它做回退认证。

## 5. 密钥

- `COVERALLS_REPO_TOKEN`：GitHub 仓库 **Settings → Secrets → Actions** 添加。
  取值路径：Coveralls → 选择仓库 → **Settings / Tokens**。

行为约定：

- **fork PR**：GitHub 不向 fork 的 PR 传递 secrets，token 为空 → 上传步骤被
  `if: env.COVERALLS_REPO_TOKEN != ''` 跳过，不会因为缺 token 而红。
- **上传失败不阻断 CI**：两个上传步骤都有 `continue-on-error: true`。Coveralls 是观测工具，
  不该成为合并的阻塞项。代价是**失败时 job 仍是绿的**，要看得进 step 日志才会发现。

## 6. 验证与排错

跑完后到 Coveralls 仓库页确认：**最新 build 下同时有 `go` 与 `node` 两个 flag，且合并出一个总覆盖率**。
也可以直接在 Actions 日志里搜 `Job #` / `https://coveralls.io/...`，goveralls 成功时会打印这两个值。

| 现象                                              | 原因 / 处理                                                                                         |
| ------------------------------------------------- | --------------------------------------------------------------------------------------------------- |
| 上传步骤显示 skipped                              | `COVERALLS_REPO_TOKEN` 为空（fork PR 正常；主仓 PR 则是 secret 没配）                               |
| `cannot find file "...": ... is not in std (...)` | 剥了模块前缀。`-coverprofile=coverage.out` 原样传入，不做 sed 替换                                  |
| `goveralls: command not found`                    | 镜像里 `GOVERALLS_VERSION` 那步没装成功，或 `/go/bin` 不在 PATH；**改镜像，别用 `go install` 兜底** |
| 文件路径对不上 / 覆盖率 0%                        | 必须在仓库根目录执行；`coverage.out` 首行应为 `mode:`，后续是 `模块路径/internal/...`               |
| 只看到 `go` 或只看到 `node`                       | 另一次上传失败被 `continue-on-error` 吞了，展开该 step 日志看报错                                   |
| 两个 flag 各算各的、没有合并                      | 两次上传不在同一 build；确认仍在同一个 job 内（拆分 job 就必须上 parallel 协议）                    |
| 私有仓库 401 / 未授权                             | Coveralls 后台未授权该仓库，或 token 类型不对；确认 `COVERALLS_REPO_TOKEN` 属于该仓库               |

本地预演（不真的上传）：

```bash
# 在仓库根目录执行，coverage.out 保持模块前缀
go test -vet=off \
  -coverpkg=$(go list ./internal/... | grep -v '/internal/ent' | paste -sd, -) \
  -coverprofile=coverage.out ./internal/...
head -2 coverage.out   # 应为 mode: 与 cnb.cool/dtapp/certflow/internal/...

# 验证 goveralls 能找到源文件（本地已装 v0.0.12 时）
goveralls -coverprofile=coverage.out -debug -shallow 2>&1 | head -50
```

## 7. 注意事项

- `flag` 名一旦定了别乱改，改了等于新建一条趋势线，历史会断档。
- 文档命名：`docs/*-*-*.md`（双横线）被 `.gitignore` 忽略，本文档用单横线（`docs/coveralls.md`）。
- goveralls 版本由 `.github/ci/Dockerfile` 的 `GOVERALLS_VERSION`（当前 `v0.0.12`）钉死；
  升级改 ARG 后重推镜像，**不要**在工作流里 `go install`。
- coverprofile 里的路径是 goveralls 的定位依据，任何"美化路径"的处理都会让它失败。
- Coveralls 后台需先授权该仓库，否则上传返回 401。
