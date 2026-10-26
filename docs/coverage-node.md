# Node / Vue 单元测试覆盖率（CNB 流水线）

本文说明 CertFlow 前端的覆盖率如何生成、上报、做门禁。Go 侧见 [coverage-go.md](coverage-go.md)， 覆盖率可视化见 [codecov.md](codecov.md)。

## 1. 当前状态

| 项         | 值                                                                                        |
|------------|-------------------------------------------------------------------------------------------|
| 上报       | ✅ 开启（CNB 内置任务 `testing:coverage`）                                                |
| 红线门禁   | ⏸ 暂停（提交 5903820 将 `lines` / `diffLines` 注释掉，只上报数据、不阻断流水线）          |
| 前端覆盖率 | **2.14% statements / 2.05% lines**（2026-10-26 实测：3 个测试文件、32 个用例）            |
| 流水线     | `.cnb/workflows/push-coverage.yml`（全量）、`.cnb/workflows/pull-coverage.yml`（PR 增量） |
| 覆盖率 key | `node`（Go 侧为 `go`）                                                                    |
| 报告文件   | `frontend/coverage/lcov.info`（`frontend/coverage` 已在 `.gitignore`）                    |

数字低是预期的：目前只覆盖了 3 个纯函数 / store 模块，17 个视图组件尚无测试； 配置开了 `all: true`，未测试文件按 0% 计入分母，所以指标不虚高。

## 2. 测试与覆盖率配置

- 框架：**Vitest 5.0.5** + `@vitest/coverage-v8` 5.0.5。不用 jest：Vite / Vue SFC / TS 原生集成， 不需要 babel / ts-jest 改造。
- 配置文件：[frontend/vitest.config.ts](../frontend/vitest.config.ts)
  - `environment: 'node'`：当前只测纯函数与 Pinia store，不需要 DOM；组件测试再切 jsdom / happy-dom。
  - `setupFiles: ['./src/test/setup.ts']`：给 node 环境补 `localStorage` / `navigator.language` 桩，并屏蔽 `console.debug` 噪音。
  - coverage：`provider: 'v8'`、`reporter: ['lcov','text']`、`reportsDirectory: './coverage'`、 `include: ['src/**/*.{ts,vue}']`、`all: true`。
  - 排除：`src/test/**`、两个入口 `src/main.ts` 与 `src/logviewer/main.ts`、`src/**/*.d.ts`。
- 脚本（[frontend/package.json](../frontend/package.json)）：`pnpm test`（`vitest run`）、 `pnpm test:coverage`（`vitest run --coverage`）。
- 现有测试：`frontend/src/test/{format,i18n,region}.test.ts`，共 32 个用例。

本地复现：

```bash
cd frontend
pnpm test:coverage          # 或 ./node_modules/.bin/vitest run --coverage
```

2026-10-26 实测：3 个测试文件 / 32 个用例全绿，statements 2.14%、lines 2.05%。

## 3. 流水线实现

Node 覆盖率与 Go 覆盖率是两条**独立并行**流水线，复用同一个 `.cnb/ci/Dockerfile`（已预装 Node + pnpm + 腾讯云源）：

stage 顺序：`make deps` → `pnpm --dir ./frontend test:coverage` → `testing:coverage` 上报 → 打印 `node coverage: lines=... diff=...`。

关键配置（摘自 `.cnb/workflows/push-coverage.yml`）：

```yaml
- name: 上报覆盖率（Node 全量红线）
  type: testing:coverage
  options:
    key: node # 与 Go 侧（key: go）区分，各自写入 meta map
    name: Node Coverage
    # lines: 90 # ⏸ 当前暂停；注释里的「基线 97.43%」是过期数据，恢复前按实测值重设
    pattern: 'frontend/coverage/lcov.info'
    breakIfNoCoverage: true # 找不到 lcov 直接失败，避免测试挂了却静默通过
  exports:
    lines: NODE_COVERAGE_LINES
    diff_pct: NODE_COVERAGE_DIFF_PCT
```

PR 侧额外计算增量覆盖率（`# diffLines: 80`），`diffLines` 与 `lines` 同样已注释。

## 4. 阈值恢复与调整

1. 先测基线：本地跑 `pnpm test:coverage`，或看 CI 日志里 `node coverage: lines=`。
2. 恢复顺序建议：**先补测试再开门禁**。当前行覆盖只有 2.05%，此时设 `lines` 只能取 1~2， 几乎没有门禁意义；至少把 `src/utils/`、`src/stores/` 补到可用水平后再恢复。
3. 恢复时：
   - push：`lines` = 当前基线 − 6（按当时实测值算）；
   - pull：`diffLines: 50`（变更代码过半覆盖）比历史注释里的 80 更现实。
4. 同步修正 `.cnb/workflows/*-coverage.yml` 里「当前基线 97.43%」「lines: 90」这类过期注释。

## 5. 关键决策

- **vitest 而非 jest**：与 Vite 原生集成，`mergeConfig` 复用 `vite.config`，别名与插件不丢。
- **`all: true`**：未测试文件按 0% 计入分母，指标诚实；真正驱动质量的是 PR 增量覆盖率。
- **排除入口文件**：`main.ts` 导入期就挂载 DOM / 读 `navigator.userAgent`，node 环境导入即崩。
- **不设 `lang`**：`lang: go` 只用于 golang 格式报告；lcov 由插件按扩展名识别。
- **key 隔离**：Node 用 `key: node`、Go 用 `key: go`，否则徽章互相覆盖。
- **组件测试是下一步**：需要 `@vue/test-utils` + jsdom / happy-dom，并把 `environment` 切过去； 在那之前视图层覆盖率保持 0 是已知状态。

## 6. 排错

| 现象               | 处理                                                                        |
|--------------------|-----------------------------------------------------------------------------|
| 找不到 `lcov.info` | 确认 `pnpm test:coverage` 成功、`reportsDirectory` 仍是 `frontend/coverage` |
| 报告路径解析失败   | `pattern` 必须相对仓库根：`frontend/coverage/lcov.info`                     |
| 流水线直接失败     | `breakIfNoCoverage: true`：先修测试；测试挂了不会有覆盖率报告               |
| 覆盖率异常低       | 看 `all: true` 是否把大量未测文件计入；新增目录是否漏了测试                 |
| `vue-tsc` 报错     | 测试文件也在类型检查范围，先跑 `pnpm exec vue-tsc --noEmit`                 |
| 徽章不更新         | 确认 `testing:coverage` stage 执行、`key` 未被其他报告覆盖                  |
