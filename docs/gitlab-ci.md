# GitLab 侧 CI（只做发布同步）

`.gitlab-ci.yml` 现在只有 **一个 job：`release`** —— 把 GitHub Release 同步成 GitLab Release。 不跑测试、不算覆盖率、不自建 CI 镜像。

```yaml
stages: [release]
default:
  image: debian:trixie-slim                 # 自带 bash；before_script 里 apt-get 装 curl / jq
release:                                    # rules: trigger（GitHub 触发）| web + $VERSION（手动）
```

## 1. 为什么删掉了覆盖率（2026-10-24）

原来这里有一整套覆盖率采集：`go-coverage` / `node-coverage`，外加为它们自建 CI 镜像的 `check-image` → `build-image` → `merge-image`，以及 `.gitlab/ci/Dockerfile`。整段删除，原因三条：

1. **产出太弱。** GitLab 自带覆盖率能给的只有：一个百分比（多个 job 取**算术平均**）、MR diff 的 行级注释、以及「分析 → 仓库分析」里默认分支的一条折线。没有可下钻的 HTML 报告，不能按文件/目录 看，也没有 flags 分组（Codecov 能分 `go` / `node` 两条线，GitLab 不能）。同样的数据本仓库另外三处 都有且都更强：

   | 平台       | 形态                                          | 看什么         |
   |------------|-----------------------------------------------|----------------|
   | CNB        | 硬红线门禁（`lines` / `diffLines`，当前暂停） | 会拦合并       |
   | Codecov    | 文件树 + patch 状态 + bundle + tests          | 可视化主力     |
   | Coveralls  | 逐行 HTML 报告                                | 逐行查看       |
   | ~~GitLab~~ | ~~百分比 + 一条折线~~                         | ~~信息量最低~~ |

2. **有价值的产出触达不到。** MR 小部件（覆盖率涨跌）和 MR diff 行级注释只在 **GitLab 上开 MR** 时 出现。本仓库 GitLab 是 pull mirror，开发发生在 GitHub / CNB 一侧，这两项永远触发不了。

3. **成本实打实。** 一次流水线约 10–16 compute minutes（含自建镜像的固定开销），GitLab 免费私有项目 只有 400 min/月，跑几十次就耗尽，还会挤掉发布同步的额度 —— 而发布同步才是 GitLab 侧不可替代的 能力。

历史实现与全部踩坑记录见 `docs/2026-10-23.md`、`docs/2026-10-24.md`，以及本文件的 git 历史。

## 2. 如果以后要恢复覆盖率（知识保留）

GitLab 把这件事拆成两个**互不联动**的机制（官方原话：read different inputs, produce different outputs, and do not share configuration. Configuring one does not enable the other）：

| 机制         | 关键字                              | 输入                        | 产出                                                                      |
|--------------|-------------------------------------|-----------------------------|---------------------------------------------------------------------------|
| 覆盖率上报   | `coverage: '<正则>'`                | job 日志里的一行数字        | job 页数字、MR 小部件涨跌、流水线列表 `Coverage` 列、徽章、仓库分析折线图 |
| 覆盖率可视化 | `artifacts:reports:coverage_report` | **只认 Cobertura / JaCoCo** | MR diff 逐行注释（绿=已覆盖 / 红=未覆盖 / 橙=已加载未执行）               |

关键限制：

- `coverage_report` **不认 lcov，更不认 go 的 coverprofile**。Go 要过一层 `gocover-cobertura` （输出已是仓库相对路径，不要再剥模块前缀）；前端让 vitest 多出一个 `cobertura` reporter， 并把 `filename="src/` 补成 `filename="frontend/src/`。
- `coverage_report` 不产生百分比，`coverage:` 不产生行级注释。
- 行级注释只出现在 **MR diff 里改动的行**上。
- 同一流水线里所有带 `coverage:` 的 job，百分比会被**取算术平均**成一个数。所以 Go（语句覆盖率） 和前端（行覆盖率）**只能二选一**，否则得到一个既非 A 也非 B 的数。
- 覆盖率图/徽章只统计**默认分支**，且需要 Reporter 及以上角色；私有项目取 badge SVG 需要 token， README 里贴了对外也是裂图。

恢复步骤：把 git 历史里删除的那版 `.gitlab-ci.yml` 与 `.gitlab/ci/Dockerfile` 取回来即可，注意 `GOBIN` 被覆盖成 `$CI_PROJECT_DIR/.cache/bin` 的坑（`go install` 的工具在 PATH 上找不到， 工具一律装进镜像）。

## 3. 交付环境与前提

| 项         | 值 / 说明                                                                                                                                                                                                                                                                                |
|------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| 镜像       | `debian:trixie-slim`（**自带 bash**）+ `apt-get install curl jq ca-certificates`。不能用 `alpine` + `apk add bash`：Runner 在跑 `before_script` **之前**就探测 shell 并锁定，before_script 里装 bash 改变不了已选定的 shell，脚本里 bash 数组 `AUTH=(-H ...)` 在 ash 下直接 syntax error |
| 前置条件 1 | 项目要开 CI/CD。pull mirror 仓库默认可能是关的：Settings → CI/CD → General pipelines，镜像仓库还要勾 **Run pipelines for mirrored repositories**                                                                                                                                         |
| 前置条件 2 | 不再需要 Container Registry（自建镜像已删）                                                                                                                                                                                                                                              |
| 触发方式   | ① GitHub Actions `sync-gitlab` 用 trigger token POST；② 网页 Run pipeline 填 `VERSION`                                                                                                                                                                                                   |
| 令牌       | `GITLAB_TRIGGER_TOKEN`（`glptt-`，GitHub 侧用）；本 job 内认证优先 `CI_JOB_TOKEN`，可配 `GL_RELEASE_TOKEN`（PAT，`api` scope）覆盖；GitHub 侧读 `CERTFLOW_GITHUB_TOKEN`（Masked、**不要**勾 Protected）                                                                                  |

## 4. 发布同步的两个必知细节

- **GitLab 没有 Release 二进制附件 API**：附件走 Generic Package Registry （`PUT /projects/:id/packages/generic/certflow/{tag}/{file}`），`assets.links` 用 `direct_asset_path` 生成 `/-/releases/{tag}/downloads/{file}` 直链。
- **文件名必须 URL 编码**：产物含空格/中文（`certflow 1.0.0 安装包.exe`），不编码 curl 直接 exit 3， `-s` 下无日志，表现为静默中断。脚本里用 `jq -sRr @uri`。
- 正文链接改写顺序不能颠倒：GitLab 直链是 `/-/releases/<tag>/downloads/<file>`（比 GitHub 多一个 `/-`，`download` 变 `downloads`）。改完若仍残留 `github.com/dtapps/certflow`，job 主动失败。

## 5. 怎么验证

1. 默认分支 push 后流水线应当**只有 `release` 的规则判断**（普通 push 不产生 job，属正常）。
2. 发版后在 GitHub Actions 看 `sync-gitlab` step 是否 2xx；对应 GitLab 流水线 `release` job 绿。
3. 打开 `${CI_PROJECT_URL}/-/releases/<tag>`，逐个点下载直链，确认不是 404。
