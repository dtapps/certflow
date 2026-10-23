# Coveralls 接入说明（certflow）

本文只讲一件事：**Coveralls 怎么接、怎么跑、怎么排错**。

- 上报入口：`.github/workflows/coveralls.yml`，独立工作流，不依赖其他流程。
- 上报内容：Go 与前端两条覆盖率，分别打 `go` / `node` 两个 flag，合并为一个 build。
- 仓库内**没有** `.coveralls.yml`：阈值、PR 注释、徽章都在 Coveralls 后台仓库设置里配。
- **Go 侧也用官方 action**：`coverallsapp/github-action@v2` + `file: coverage.out` + `format: golang`，
  不需要任何格式转换，工作流里也不装 goveralls（镜像里的 `GOVERALLS_VERSION=v0.0.12` 只供本地手测）。
- **Go 的 coverprofile 原样交给 action**：不剥模块前缀、不做 sed（见 3.3 ①）。

## 1. 涉及文件

| 文件                              | 作用                                                                                                |
| --------------------------------- | --------------------------------------------------------------------------------------------------- |
| `.github/workflows/coveralls.yml` | 唯一入口：装依赖 → 跑 Go/前端测试 → 分别上报 `flag=go` / `flag=node`                                |
| `.github/ci/Dockerfile`           | 提供 `ghcr.io/<repo>/ci:latest` 镜像（Go/Node/pnpm/GTK/WebKit，含 goveralls v0.0.12，仅本地手测用） |
| `Makefile`（`deps` 目标）         | `go mod download` + `pnpm --dir ./frontend install`                                                 |
| `coverage.out`                    | Go 覆盖率产物，**原样**以 `format: golang` 交给 action（路径带模块前缀，不要处理）                  |
| `frontend/coverage/lcov.info`     | 前端覆盖率产物（`pnpm test:coverage` 生成）                                                         |

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
- **多 job + parallel**：两侧都写 `parallel: true`，最后加一个 `parallel-finished: true` 的收尾 job。
  两个 job 用同一套 action 相对简单，但仍可能出现「build 一直不 finalize」或「两份报告各算各的」。

当前用单 job 顺序执行，用一点时间换确定性。将来真要拆（比如 Go 测试太慢），写法是：

```yaml
jobs:
  coverage-go: # action: parallel: true, flag-name: go, format: golang
  coverage-node: # action: parallel: true, flag-name: node
  finish: # needs: [coverage-go, coverage-node] → parallel-finished: true
```

拆之前先确认 action 的 `parallel` / `parallel-finished` 配对关系，别照抄。

### 2.3 运行环境

复用预构建镜像：

```yaml
container:
  image: ghcr.io/${{ github.repository }}/ci:latest
```

镜像里已有 Go / Node / pnpm / GTK4 / WebKit6，以及钉好版本的 Go 工具链：

| 工具      | 版本                        | 来源                                            |
| --------- | --------------------------- | ----------------------------------------------- |
| gotestsum | `GOTESTSUM_VERSION=v1.13.0` | `go install ...@版本`                           |
| gojq      | `GOJQ_VERSION=v0.12.19`     | 同上                                            |
| goveralls | `GOVERALLS_VERSION=v0.0.12` | 同上，装到 `$GOPATH/bin`（仅本地手测，CI 不用） |

`ENV PATH="$GOPATH/bin:$PATH"` 已经包含 `/go/bin`，所以本地手测时可以直接调 `goveralls`。
CI 里**不需要** `setup-go`、`setup-node`、`apt-get install`——Wails 的 CGO 链接依赖也齐了。
镜像由 `ci-image.yml` 推送，**首次运行前必须已存在**，否则容器起不来。

### 2.4 缓存

| 缓存           | 路径                        | key                                    |
| -------------- | --------------------------- | -------------------------------------- |
| Go module      | `/go/pkg/mod`               | `hashFiles('**/go.sum')`               |
| Go build cache | `$HOME/.cache/go-build`     | `hashFiles('**/go.sum')`               |
| pnpm store     | `pnpm config get store-dir` | `hashFiles('frontend/pnpm-lock.yaml')` |

## 3. Go 覆盖率：coverallsapp/github-action（`format: golang`）

### 3.1 用官方 action，靠 `format: golang` 直接吃 coverprofile

`coverallsapp/github-action@v2` 是 **composite action**，内部跑 `coverallsapp/coverage-reporter`，
`action.yml` 提供三个覆盖率输入：`file`、`files`、`format`。`format` 的说明原文：

```yaml
format:
  description: "Force coverage format, supported formats: lcov, simplecov, cobertura, jacoco, gcov, golang, python"
```

所以 Go 侧不需要任何转换：`file: coverage.out` + `format: golang` 即可。

> ⚠️ **查文档必踩的坑**：该仓库 `master` 分支还是旧的 v1 实现（`using: node16`，输入只有
> `path-to-lcov`，没有 `file` / `format`）。只看 master 的 `action.yml` 会得出「action 不支持 Go」
> 的错误结论——**必须看 `v2` 分支的 `action.yml`**。本节曾据此写错，已更正。

Go 与前端现在是**同一个上传器**，差异只剩 format 一项：

| flag   | file                          | format                        |
| ------ | ----------------------------- | ----------------------------- |
| `go`   | `coverage.out`                | `golang`                      |
| `node` | `frontend/coverage/lcov.info` | 省略（按扩展名自动识别 lcov） |

### 3.2 步骤（两步，无安装、无格式转换）

```yaml
- name: 生成 Go 测试覆盖率
  run: |
    gotestsum --junitfile go-test-report.xml -- -vet=off -coverpkg=$(go list ./internal/... | grep -v '/internal/ent' | paste -sd, -) -coverprofile=coverage.out ./internal/...

- name: 上传 Go 覆盖率到 Coveralls
  continue-on-error: true
  uses: coverallsapp/github-action@v2
  with:
    github-token: ${{ secrets.GITHUB_TOKEN }}
    file: coverage.out
    format: golang
    flag-name: go
```

**Go 步骤没有 `if: env.COVERALLS_REPO_TOKEN != ''` 门禁**：Go 侧凭据是 `GITHUB_TOKEN`，不再依赖
那个 secret，所以不拿它决定是否跳过（前端那步的门禁保持原样，要删可一起删）。

**工作流里不装 goveralls。** 上传器由 action 自带（运行时下载 `coverage-reporter` 二进制）；
镜像里的 `GOVERALLS_VERSION` 只服务于 §6 的本地手测，两条链路互不影响。

### 3.3 三个必须知道的细节

**① 模块前缀必须保留，绝不能剥。**（结论来自 goveralls 时代；现已换成 coverage-reporter，
但"不改路径"这条仍然安全。若 Coveralls 上文件路径对不上，先怀疑这里或 `base-path`。）

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

**② `service_name` / `service_job_id` 由 action 自己填。** 命令行版需要显式 `-service=github`
（从 `GITHUB_*` / `GITHUB_EVENT_PATH` 取分支、commit、PR 编号，`service_job_id` 取 `GITHUB_RUN_ID`）；
action 内部做的就是同一件事，工作流里不用再写。

**③ 容器 job 的 git 元数据（goveralls 时代需要，现在不需要）。**

goveralls 的 git 元数据（commit、作者、message）是**直接跑 git 命令**拿的（`gitinfo.go`：
`git show -s --format=%cE HEAD` 等），任何一个命令失败就整体丢弃 git 信息，且 stderr 被
`CombinedOutput` 吞掉，日志里只有 `exit status 128`。

容器 job 里进程是 root（`uid=0`），checkout 目录属主是宿主 UID（`1001`），git 会以
dubious ownership 拒绝操作（exit 128）。实测：加 `safe.directory` 前三条命令全挂，加之后
`rev-parse HEAD` / `show -s --format=%cE HEAD` / `branch --contains` 全部 exit 0。

**但 git 信息是否完整与 422 无关**（实测：git 修好后仍 422）。它的影响只是上报的 commit/作者
字段为空。goveralls 吞掉了 git 的 stderr，只透出 exit code，很容易把两件事误连成因果。

**切到 action 后不需要这步**：coverage-reporter 用 action 上下文里的 commit/分支，不跑 git。
保留这段是因为：若哪天又换回 bin/goveralls，`exit 128` 会无声无息地让 commit 字段变空。

### 3.4 422 "Couldn't find a repository matching this job"

先把错误语义钉死。直接 POST 伪造 payload 到 `https://coveralls.io/api/v1/jobs`（multipart
`json_file`，与两个上传器的实际发法一致）实测：

| 请求                                 | 响应                                                                           |
| ------------------------------------ | ------------------------------------------------------------------------------ |
| `repo_token` = 32 个 `0`             | `422 {"message":"Couldn't find a repository matching this job.","error":true}` |
| 完全不带 `repo_token`                | 同上，一字不差                                                                 |
| token 随便给、但 `source_files` 为空 | `422 {"message":"JSON file not found or failed to parse."}`                    |

所以这句 422 **只表示 `repo_token` 没能解析出任何仓库**，与 git 信息、`service_job_id`、
`service_number`、覆盖率内容都无关。

**成因（goveralls 时代）**：两侧凭据不同源。当时 node 走 action（v1 的 `dist/index.js` 一开头就
`process.env.COVERALLS_REPO_TOKEN = githubToken`），用的是 `GITHUB_TOKEN`；go 走 `goveralls`
命令行，用的是 `-repotoken="$COVERALLS_REPO_TOKEN"`。于是 node 成功完全不能证明那个 secret 有效。

于是 422 只剩一种解释：**这个 secret 的值不是本仓库在 Coveralls 上的有效 repo token**。
常见成因：

1. 从别的仓库（如 `kai`）复制了 token —— 每个仓库一个 token，复制过来必然 422；
2. 复制时带进了换行/空格 —— 看着有值，实际不匹配；
3. Coveralls 侧仓库被删除重建过，旧 token 失效；
4. secret 配置早于 Coveralls 上该仓库的首个 build（本仓库就是 node 首次上传时才出现在
   `https://coveralls.io/github/dtapps/certflow.json`），此时拿到的 token 来源存疑。

**定位方法**（不需要看到 token 明文）：Go 上传步骤里打一行指纹，

```bash
echo "token len=${#COVERALLS_REPO_TOKEN} sha=$(printf %s "$COVERALLS_REPO_TOKEN" | sha256sum | cut -c1-12)"
```

在 `certflow` 与 `kai` 各跑一次对比：`sha` 相同 → 是复制来的（成因 1）；`len` 与 Coveralls
后台 token 长度不一致 → 有空白（成因 2）。定位完删掉这行。

**当时的修复**：Coveralls → `dtapps/certflow` → Settings 复制 repo token → 更新 GitHub 仓库的
`COVERALLS_REPO_TOKEN` secret → 重跑。

**最终采用的方案（见 §3）**：Go 侧改用 `coverallsapp/github-action@v2` + `format: golang`，
与 node 走**同一个上传器、同一个 `GITHUB_TOKEN`**，从根上消除"两条链路、两套凭据"这个差异面。
`COVERALLS_REPO_TOKEN` 至此只剩前端步骤里一个历史遗留 `if` 门禁引用。

### 3.5 统计口径

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

- 前端用 **`path-to-lcov`**；Go 侧用 `file` + `format: golang`。两者都是 v2 的合法输入
  （`path-to-lcov` 是 lcov 的专用别名，`file` 配合 `format` 用）。
- `flag-name: node` 与 Go 的 `flag-name: go` 配对。
- `github-token` 用内置 `GITHUB_TOKEN`。v1 的实现会把它覆盖进 `process.env.COVERALLS_REPO_TOKEN`；
  **v2（composite + coverage-reporter）直接把 token 传给 reporter**。无论哪种，node 成功都只说明
  `GITHUB_TOKEN` 可用，不能说明 `COVERALLS_REPO_TOKEN` 有效（见 3.4）。

## 5. 凭据

**两个上传步骤实际都只用内置 `GITHUB_TOKEN`**（`github-token: ${{ secrets.GITHUB_TOKEN }}`）。
`COVERALLS_REPO_TOKEN` 现在只剩一处引用：前端步骤的历史遗留 `if` 门禁。Go 侧已完全不读它。

- Coveralls 后台若还没授权该仓库，`GITHUB_TOKEN` 同样会失败（401 / 422）。此时去
  Coveralls → **Add Repo** 添加 `dtapps/certflow`，再重跑。
- 若以后改回 `goveralls` 命令行版，`COVERALLS_REPO_TOKEN` 就必须是**本仓库**的 repo token，
  长度与内容一字不差（粘贴带换行是最常见的坑）。校验方式见 3.4：打长度 + sha256 前 12 位
  与其他仓库对比，别打印明文。GitHub secrets 只写不可读，配错只能重设。

行为约定：

- **fork PR**：`GITHUB_TOKEN` 存在但只读，上传可能 401；由 `continue-on-error: true` 兜住，
  不会因为上传失败而红。
- **上传失败不阻断 CI**：两个上传步骤都有 `continue-on-error: true`。Coveralls 是观测工具，
  不该成为合并的阻塞项。代价是**失败时 job 仍是绿的**，要看得进 step 日志才会发现。

## 6. 验证与排错

跑完后到 Coveralls 仓库页确认：**最新 build 下同时有 `go` 与 `node` 两个 flag，且合并出一个总覆盖率**。
也可以直接在 Actions 日志里搜 `Job #` / `https://coveralls.io/...`：两个上传步骤（coverage-reporter）
成功时都会打印 job id 与网页链接。

| 现象                                                   | 原因 / 处理                                                                                                                                                                                |
| ------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| 前端上传步骤显示 skipped                               | 前端那步仍带 `if: env.COVERALLS_REPO_TOKEN != ''`；Go 步骤已无此门禁                                                                                                                       |
| `cannot find file "...": ... is not in std (...)`      | 剥了模块前缀。`-coverprofile=coverage.out` 原样传入，不做 sed 替换                                                                                                                         |
| `gitinfo.go: fail to run "git ...": exit status 128`   | 只有用 bin/goveralls 时才出现：容器内 dubious ownership（`uid=0` vs 目录属主 1001），加 `git config --global --add safe.directory "$GITHUB_WORKSPACE"`。**只影响 commit 字段，不导致 422** |
| `422 Couldn't find a repository matching this job`     | `repo_token` 解析不出仓库（已实测复现）：secret 值不是本仓库的 token / 带空白 / 已失效。见 3.4                                                                                             |
| `Unsupported format` / `could not determine format`    | `format:` 拼错或漏写；Go 必须写 `golang`（不是 `go`），前端可省略（按 `.info` 识别 lcov）                                                                                                  |
| `ENOENT` / `no such file: coverage.out`                | `file:` 相对 workspace 根；Go 在仓库根生成 `coverage.out`，前端是 `./frontend/coverage/lcov.info`                                                                                          |
| 文件路径对不上 / 覆盖率 0%                             | 必须在仓库根目录执行；`coverage.out` 首行应为 `mode:`，后续是 `模块路径/internal/...`                                                                                                      |
| 只看到 `go` 或只看到 `node`                            | 另一次上传失败被 `continue-on-error` 吞了，展开该 step 日志看报错                                                                                                                          |
| 两个 flag 各算各的、没有合并                           | 两次上传不在同一 build；确认仍在同一个 job 内（拆分 job 就必须上 parallel 协议）                                                                                                           |
| 私有仓库 401 / 未授权                                  | Coveralls 后台未授权该仓库；先去 Coveralls 添加仓库再取 token                                                                                                                              |
| 只有 `node` 有数据、`go` 一直 422（旧 goveralls 时代） | 那时 go 用 `COVERALLS_REPO_TOKEN`、node 用 `GITHUB_TOKEN`，两侧凭据不同源。现在两侧都走 action + `GITHUB_TOKEN`；若还 422，就是 Coveralls 后台未授权该仓库                                 |

本地预演（不真的上传）：

```bash
# 在仓库根目录执行，coverage.out 保持模块前缀
go test -vet=off \
  -coverpkg=$(go list ./internal/... | grep -v '/internal/ent' | paste -sd, -) \
  -coverprofile=coverage.out ./internal/...
head -2 coverage.out   # 应为 mode: 与 cnb.cool/dtapp/certflow/internal/...

# 可选：用镜像里的 goveralls 验证它能否找到源文件（仅供手测，CI 不走这条路）
goveralls -coverprofile=coverage.out -debug -shallow 2>&1 | head -50
```

## 7. 注意事项

- `flag` 名一旦定了别乱改，改了等于新建一条趋势线，历史会断档。
- 文档命名：`docs/*-*-*.md`（双横线）被 `.gitignore` 忽略，本文档用单横线（`docs/coveralls.md`）。
- 上传器版本由 `uses: coverallsapp/github-action@v2` 决定，升级改那一行；
  **不要**在工作流里 `go install goveralls`。
- 看该 action 的文档认准 **`v2` 分支**：`master` 仍是 v1 实现，没有 `file` / `format` 输入，
  照它写会得出错误结论（见 3.1）。
- coverprofile 里的路径是文件定位依据，任何"美化路径"的处理都可能让它对不上源文件。
- Coveralls 后台需先授权该仓库，否则上传返回 401。
