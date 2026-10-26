# Go 单元测试覆盖率（CNB 流水线）

本文说明 CertFlow 的 Go 覆盖率如何统计、上报、做门禁，以及阈值当前状态。 覆盖率可视化见 [codecov.md](codecov.md)，前端侧见 [coverage-node.md](coverage-node.md)。

## 1. 当前状态

| 项          | 值                                                                                        |
|-------------|-------------------------------------------------------------------------------------------|
| 上报        | ✅ 开启（CNB 内置任务 `testing:coverage`）                                                |
| 红线门禁    | ⏸ 暂停（提交 5903820 将 `lines` / `diffLines` 注释掉，只上报数据、不阻断流水线）          |
| Go 总覆盖率 | **55.5%**（2026-10-26 实测，CI 口径）                                                     |
| 流水线      | `.cnb/workflows/push-coverage.yml`（全量）、`.cnb/workflows/pull-coverage.yml`（PR 增量） |
| 覆盖率 key  | `go`（前端为 `node`，两者互不覆盖）                                                       |
| 报告文件    | `coverage.out`（已在 `.gitignore`）                                                       |

## 2. 统计口径

- 只统计 `./internal/...`。根包 `main` 带 `//go:embed all:frontend/dist`，该目录不进仓库， 本地用 `./...` 会直接编译失败。
- `-coverpkg` 排除生成代码：
  - `internal/ent`：ent 生成，约占全树语句块 73%，计入会让分母暴涨、指标失去参考意义；
  - `internal/httplog/db`：sqlc 生成，同样不写测试。
- `-vet=off`：`i18n.T()` 等作为 printf 参数会触发 `non-constant format string`， 与 `make test-go` 保持一致。

本地复现（与 CI 完全相同的命令）：

```bash
go test -vet=off -count=1 \
  -coverpkg=$(go list ./internal/... | grep -vE '/internal/ent|/internal/httplog/db' | paste -sd, -) \
  -coverprofile=coverage.out ./internal/...
go tool cover -func=coverage.out | tail -1
```

2026-10-26 实测输出：`total: (statements) 55.5%`。

## 3. 流水线实现

push 与 pull_request 各有一条**独立并行**的 Go 覆盖率流水线，都复用 `.cnb/ci/Dockerfile`， 并挂载 Go 模块与 build cache：

```yaml
docker:
  build: .cnb/ci/Dockerfile
  volumes:
    - /root/.cache/go-build:copy-on-write
    - /go/pkg/mod:copy-on-write
```

stage 顺序：`make deps` → 生成覆盖率（上面的命令）→ `testing:coverage` 上报 → 打印 `coverage: lines=... diff=...`。

关键配置（摘自 `.cnb/workflows/push-coverage.yml`）：

```yaml
- name: 上报覆盖率（全量红线）
  type: testing:coverage
  options:
    key: go # 与 Node 侧（key: node）区分，各自写入 meta map
    name: Go Coverage
    # lines: 10 # ⏸ 当前暂停；恢复时按「基线-6」设置，不要沿用这个早期的低位兜底值
    lang: go # golang 格式报告必须指定
    pattern: 'coverage.out'
    breakIfNoCoverage: false # 报告缺失只丢数据、不阻断
  exports:
    lines: COVERAGE_LINES
    diff_pct: COVERAGE_DIFF_PCT
```

PR 侧额外计算增量覆盖率（`diffLines`，仅 PR 类事件生效），`diffLines` 与 `lines` 同样已注释。

> ⚠️ `lines: COVERAGE_LINES` 是 `exports` 输出变量，与阈值 `lines:` 不是同一个键，别改错位置。

## 4. 阈值恢复与调整

1. 先测基线：跑上面「本地复现」命令，记下 total。
2. 取消注释阈值：
   - push：`lines` = 当前基线 − 6（当前约 49）；
   - pull：`lines` 与 push 保持一致，另设 `diffLines: 50`（变更代码过半覆盖）。
3. 重跑一次 push / PR，确认日志 `coverage: lines=... diff=...` 与徽章一致。
4. 棘轮策略：只在总量真实上升后上调，只升不降。

暂停期间任何阈值都容易误伤（例如漏跑测试导致某包骤降），恢复时按实测基线设置， 不要直接沿用注释里的历史值。

## 5. 关键决策

- **独立并行流水线**：覆盖率不并入既有检查 pipeline 的 `stages`（会串行）； push / pull_request 各一条。
- **key 隔离**：Go 用 `key: go`、前端用 `key: node`。不指定 `key` 时所有报告写顶层默认字段， 后跑完的会覆盖先跑完的，徽章只显示最后一份。
- **`breakIfNoCoverage` 两侧不同**：Go 为 `false`（报告缺失只丢数据）；Node 为 `true`（缺 `lcov.info` 直接失败）。
- **命令内联在 CI**：覆盖率是门禁而非日常开发命令，不放进 Makefile；本地复现直接从 YAML 复制。
- **生成代码不计入**：ent / sqlc 生成目录必须排除，否则指标失去意义。

## 6. 排错

| 现象                         | 处理                                                                   |
|------------------------------|------------------------------------------------------------------------|
| 徽章不更新                   | 看 CNB 日志里 `coverage: lines=`；确认 `testing:coverage` stage 执行了 |
| 覆盖率骤降                   | 检查 `-coverpkg` 的排除项是否被改动，以及新增包是否没写测试            |
| `non-constant format string` | 确认命令带 `-vet=off`                                                  |
| 报告缺失但流水线仍然绿       | Go 侧 `breakIfNoCoverage: false` 属预期；需要严格时改成 `true`         |
| 阈值改完立刻红               | 多半是 `lines` 与 `exports` 的键写混了，或用了过期的基线值             |
