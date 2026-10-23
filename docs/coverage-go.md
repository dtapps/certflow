# Go 单元测试覆盖率实现文案（可复制到其他项目交给 AI 实现）

> 来源：CertFlow 项目实战方案（CNB 云原生构建 + testing:coverage 内置任务）。

# 任务：为 Go 项目实现单元测试覆盖率上报与红线门禁（CNB 云原生构建）

## 背景与目标

- 构建平台是 CNB（cnb.cool），流水线配置在仓库根的 `.cnb.yml`（或 `.cnb/workflows/*.yml`，由 .cnb.yml include）。
- 使用 CNB 内置任务 `type: testing:coverage` 上报覆盖率：push 事件上报全量覆盖率并设红线，pull_request 事件额外计算增量覆盖率并设红线（diffLines 仅 PR 类事件生效）。
- 红线采用「棘轮策略」：初始值贴着当前基线往下留余量，防止误伤，后续随覆盖率提升逐步上调。

## 实现步骤

### 第 1 步：先测量当前覆盖率基线（定红线前必须做）

在本地运行并记录 total 覆盖率。**测试范围按项目实际写**（本项目测试全部位于 `./internal/...`；根目录 main 包带 `//go:embed all:frontend/dist` 嵌入依赖、 该目录不进仓库，用 `./...` 会在本地直接编译失败）：

```bash
go test -vet=off -coverprofile=/tmp/cov.out ./internal/... && go tool cover -func=/tmp/cov.out | tail -1
```

（本项目实测基线：含 ent 生成代码时 5.0%，排除后 27.2%——所以第 2 步必须排除生成代码。）

### 第 2 步：.gitignore 增加

```
coverage.out
coverage.html
```

（覆盖率命令不放进 Makefile：覆盖率是 CI 质量门禁而非日常开发命令，命令直接内联到流水线脚本里，CI 自包含、无间接层。若团队有本地复现需求，可从流水线 YAML 复制命令执行。）

### 第 3 步：流水线配置（push 与 pull_request 各用独立并行流水线，不并入既有 CI）

CNB 语义：事件键（如 `'**'`）下的值是 **Pipeline 数组，同事件多个数组条目 = 多条流水线并行执行**（并入同一个 pipeline 的 stages 会变串行，不要这样做）。

`.cnb.yml`（或 workflows 文件）中新增：

```yaml
"**":
  push:
    # ...既有流水线保持不动...
    # 新增：测试覆盖率流水线（与既有流水线并行）
    - runner:
        tags: cnb:arch:amd64
      docker:
        build: .cnb/Dockerfile # 复用项目构建环境
        volumes:
          - /root/.cache/go-build:copy-on-write # Go 构建缓存
          - /go/pkg/mod:copy-on-write # Go 模块缓存
      stages:
        - name: 生成测试覆盖率
          # 命令直接内联于 CI（不经 Makefile）：覆盖率是 CI 质量门禁，非日常开发命令。
          # -vet=off：若项目有「i18n.T() 等函数作 printf 参数」类 vet 告警会导致 go test 失败，需关闭（按项目实际取舍）。
          # -coverpkg 排除生成代码目录（如 ent/mocks/protobuf）：生成代码占语句块比例极高
          # （本项目实测占 73%），计入会把总覆盖率从 27% 稀释到 5%，指标失去意义，与排除 vendor 同理。
          # go list ... | grep -v ... | paste -sd, -：动态生成排除后的包清单，避免硬编码。
          script:
            - go mod download
            - go test -vet=off -coverpkg=$(go list ./internal/... | grep -v '/internal/ent' | paste -sd, -) -coverprofile=coverage.out ./internal/...
            - go tool cover -func=coverage.out # 产出 coverage.out + 终端按函数摘要
        - name: 上报覆盖率（全量红线）
          type: testing:coverage
          options:
            key: go # 多报告数据键：仓库内有多份覆盖率报告（如同时有前端）时必须指定，否则各报告都写顶层默认字段，后跑完的覆盖先跑完的
            name: Go Coverage # 徽章显示名，与 key 一对一对应
            lines: 20 # 全量红线(%)：低于此值阻断流水线。本项目基线 27.2%，取 20 留约 7 个百分点余量，逐步上调
            lang: go # golang 格式报告必须指定，否则计算有误差
            pattern: "coverage.out"
            breakIfNoCoverage: true # 报告缺失直接失败，避免测试挂了却静默跳过
          exports:
            lines: COVERAGE_LINES
            diff_pct: COVERAGE_DIFF_PCT
        - name: 输出覆盖率
          script: 'echo "coverage: lines=$COVERAGE_LINES diff=$COVERAGE_DIFF_PCT"'
  pull_request:
    # ...既有 PR 流水线保持不动...
    - runner:
        tags: cnb:arch:amd64
      docker:
        build: .cnb/Dockerfile
        volumes:
          - /root/.cache/go-build:copy-on-write
          - /go/pkg/mod:copy-on-write
      stages:
        - name: 生成测试覆盖率
          # 命令与 push 侧保持一致（内联，不经 Makefile）
          script:
            - go mod download
            - go test -vet=off -coverpkg=$(go list ./internal/... | grep -v '/internal/ent' | paste -sd, -) -coverprofile=coverage.out ./internal/...
            - go tool cover -func=coverage.out
        - name: 上报覆盖率（增量红线）
          type: testing:coverage
          options:
            key: go # 与 push 侧保持同一个 key，数据才落在同一槽位
            name: Go Coverage
            diffLines: 50 # 增量红线(%)：本 PR 变更代码的覆盖率，低于此值阻断
            lines: 20 # 全量底线与 push 一致，防总量被单 PR 拉低
            lang: go
            pattern: "coverage.out"
            breakIfNoCoverage: true
          exports:
            lines: COVERAGE_LINES
            diff_pct: COVERAGE_DIFF_PCT
        - name: 输出覆盖率
          script: 'echo "coverage: lines=$COVERAGE_LINES diff=$COVERAGE_DIFF_PCT"'
```

## 关键决策（实现时遵循，有冲突按项目实际调整并说明）

1. **排除生成代码**：ent/mock/protobuf 等生成目录必须排除，否则指标被稀释到失去意义。
2. **红线初始值 = 当前基线减去 5~7 个百分点**，防止新增少量未覆盖代码就误触红线；每季度上调一次（棘轮）。
3. **PR 用 diffLines 管增量**：新代码必须过半覆盖，这是长期提升覆盖率的主要驱动力。
4. **breakIfNoCoverage: true**：防止测试编译失败时覆盖率阶段静默通过造成假绿。
5. **独立并行流水线**：与既有 CI 并行，互不阻塞；不要合并进既有 pipeline 的 stages（会串行）。
6. **YAML 细节**：script 值含 `": "` 等特殊字符时整体用单引号包裹；`go mod download` 前置覆盖冷缓存场景。
7. **多报告必须用 `key` 隔离**：若仓库内还有其他语言的覆盖率报告（如前端 Node），每份报告的 `testing:coverage` 都必须指定不同的 `key`（如 go / node）与对应 `name`。不指定 `key` 时所有报告都写顶层默认字段，**后跑完的会覆盖先跑完的**（徽章只显示最后一份）。指定 `key` 后数据各自写入 meta map 互不冲突；单仓库 key 上限 30 个。

## 验收标准

- 本地跑通流水线中的覆盖率命令（从 YAML 复制执行），产出 coverage.out，`go tool cover -func` 有 total 且与第 1 步基线一致（排除生成代码后）。
- YAML 解析正确；push/pull_request 各新增一条独立流水线。
- 首次推送后确认：仓库「洞察 → 单元测试覆盖率」徽章生成，红线数值符合预期。
