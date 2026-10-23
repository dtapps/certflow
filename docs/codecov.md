# Codecov 接入说明（certflow）

本仓库的覆盖率 / 打包体积 / 测试结果统一上报到 **Codecov**，由 GitHub Actions 工作流驱动（与 CNB 的硬性红线门禁相互独立）。

## 1. 总览：三个支柱 + 职责分离

| 支柱                     | 内容            | 本仓库实现                                                                                                | Codecov 里看什么                      |
|--------------------------|-----------------|-----------------------------------------------------------------------------------------------------------|---------------------------------------|
| 覆盖率 Coverage          | 代码行覆盖率    | `codecov/codecov-action` 上传 Go `coverage.out` 与前端 `lcov.info`                                        | Coverage 面板、PR 评论、patch 状态    |
| 打包体积 Bundle Analysis | 前端 chunk 体积 | `@codecov/vite-plugin`（构建时上传）                                                                      | Bundle 面板（体积趋势 / 回归）        |
| 测试结果 Test Analytics  | 单测运行结果    | Go：`gotestsum --junitfile`；Vue：Vitest `json` → 脚本转 JUnit；合并由 `codecov/test-results-action` 上传 | Tests 面板（时长 / flaky / 失败分析） |

> **职责分离（重要）**：CNB 流水线（`testing:coverage`）负责**硬性红线门禁**（`lines` / `diffLines`），会阻断低覆盖率合并；GitHub Actions 只负责**把数据上报到 Codecov** 做可视化（PR 评论、徽章、patch 状态）。因此 `.github/codecov.yml` 里的 `status` 全部设为 `informational`，避免与 CNB 双重门禁。

## 2. 涉及文件清单

| 文件                                        | 作用                                                                                                                               |
|---------------------------------------------|------------------------------------------------------------------------------------------------------------------------------------|
| `.github/codecov.yml`                       | Codecov 规则：忽略 `internal/ent` 等生成代码、`flags`（go / node）、`component_management`（Go/Vue 组件）、`status: informational` |
| `.github/workflows/codecov.yml`             | GitHub Actions 流水线：装依赖 → 跑 Go/前端测试 → 构建前端 → 上传覆盖率 / 测试结果                                                  |
| `frontend/vite.config.ts`                   | 条件加载 `@codecov/vite-plugin`（仅当 `CODECOV_TOKEN` 存在时实例化）                                                               |
| `frontend/scripts/vitest-json-to-junit.mjs` | 零依赖脚本：Vitest JSON → JUnit XML                                                                                                |
| `frontend/package.json`                     | `devDependencies`：`@codecov/vite-plugin`、`@vitest/...` 等                                                                        |

## 3. 各功能详解

### 3.1 覆盖率（Coverage）

- Go：复用 CNB 的同款口径

  ```
  go test -vet=off -coverpkg=$(go list ./internal/... | grep -v '/internal/ent' | paste -sd, -) -coverprofile=coverage.out ./internal/...
  ```

  排除 `internal/ent`（Ent 生成代码，占比极大，避免稀释指标）。
- 前端：`pnpm test:coverage`（Vitest 5.0.5 覆盖率，产出 `frontend/coverage/lcov.info`）。
- 上传：`codecov/codecov-action@v5`，分别带 `flags: go` / `flags: node`。

### 3.2 打包体积（Bundle Analysis）

- 插件 `@codecov/vite-plugin@2.0.1` 在 `frontend/vite.config.ts` 中**条件加载**：

  ```ts
  ...(process.env.CODECOV_TOKEN
    ? [codecovVitePlugin({ enableBundleAnalysis: true, bundleName: "certflow-frontend", uploadToken: process.env.CODECOV_TOKEN })]
    : [])
  ```
- 上传发生在 `pnpm build`（= `vue-tsc && vite build --mode production`）这一步。
- ⚠️ **兼容性注意**：该插件 peer 仅声明支持 Vite `4/5/6`，而本仓库为 **Vite 8.3.4**。因此特意用「条件数组」而非无条件 `import`——CNB 生产构建与本地构建都**没有** `CODECOV_TOKEN`，插件根本不会实例化，绝不影响主构建。若日后插件更新支持 Vite 8，逻辑无需改动。
- 构建步骤设了 `continue-on-error: true`：万一插件与 Vite 8 不兼容导致 `vite build` 崩溃，也**不会拖垮**上面的覆盖率上传。

### 3.3 测试结果（Test Analytics）

Go 与 Vue 都不原生产出 JUnit，分别用不同方式转换后合并上传到同一个 `codecov/test-results-action@v1`：

- **Vue（Vitest 5.0.5）**：无内置 junit reporter（社区 `@vitest/junit` 包在 npm 也已不可得），改用内置 `json` reporter 产出 `test-results.json`，再由零依赖脚本转 JUnit：
  1. 测试步骤额外挂内置 `json` reporter 产出 `test-results.json`：

     ```
     pnpm test:coverage --reporter=default --reporter=json --outputFile=test-results.json
     ```

     （`coverage/lcov.info` 仍正常产出，不受影响。）
  2. 转换步骤用 `node scripts/vitest-json-to-junit.mjs` 把 JSON 转成 `test-report.xml`（JUnit 格式）。
  3. 上传步骤用 `codecov/test-results-action@v1` 上传 `frontend/test-report.xml`。
- **Go**：`go test` 也不原生产 JUnit，用 `gotestsum` 包裹（参数原样透传，`coverage.out` 照常产出），额外生成 `go-test-report.xml`：

  ```
  go install gotest.tools/gotestsum@v1.13.0
  export PATH="$(go env GOPATH)/bin:$PATH"
  gotestsum --junitfile go-test-report.xml -- -vet=off -coverpkg=$(go list ./internal/... | grep -v '/internal/ent' | paste -sd, -) -coverprofile=coverage.out ./internal/...
  ```
  - ⚠️ `gotestsum` 必须固定为 **v1.13.0**：v1.12.0 依赖的旧版 `golang.org/x/tools@v0.19.0` 无法在 Go 1.27 上编译（`invalid array length`），v1.13.0 升到 x/tools v0.36.0 后正常。
  - 上传步骤的 `files` 已合并两份报告：`frontend/test-report.xml,./go-test-report.xml`。
- 最终 Codecov 的 **Tests** 面板同时包含 Go 与 Vue 的测试结果。

### 3.4 组件（Components）

把整仓覆盖率拆成 **Go 后端 / Vue 前端** 两个逻辑单元，在 Codecov 的 **Components** 面板分别看各自的覆盖率与趋势。

- 配置在 `.github/codecov.yml` 的 `component_management`（**不是**根目录 `codecov.yaml`）：

  ```yaml
  component_management:
    default_selection: 2 # 面板默认同时展示 2 个组件
    individual_components:
      - component_id: "go-backend"
        name: "Go 后端"
        paths: ["internal/**"]
      - component_id: "vue-frontend"
        name: "Vue 前端"
        paths: ["frontend/src/**"]
  ```
- 组件 `paths` 与上面的 `flags`（go / node）划分一致；全局 `ignore`（ent 生成代码 / 测试 / 入口）仍生效， 这些文件不计入组件覆盖率。
- ⚠️ **配置文件位置**：Codecov 搜索顺序为「**仓库根目录 → `.github/`**」。**不要把 `codecov.yaml` 放根目录**—— 若根目录存在，它会优先于 `.github/codecov.yml` 生效并**完全覆盖**后者（丢失门禁 / 忽略 / flags 规则）。 本仓库统一放在 `.github/codecov.yml`。

## 4. 必需的密钥

- `CODECOV_TOKEN`：在 GitHub 仓库 **Settings → Secrets → Actions** 中配置（Codecov 项目页获取）。覆盖率、Bundle、测试结果三个动作共用此 token。

## 5. 运行环境

- `runs-on: ubuntu-24.04`（**必须 24.04**，因为只有它有 `libwebkitgtk-6.0-dev`）。
- 工作流开头 `sudo apt-get install libgtk-4-dev libwebkitgtk-6.0-dev`：本项目用 Wails，Go 测试会 CGO 链接 GTK4/WebKit 系统库，标准 runner 缺这些库会在 `ld` 阶段失败。

## 6. 验证与排错

在 Codecov 项目页应能看到三个面板：**Coverage / Bundle / Tests**。

- **Go 测试 `ld` 报错 `cannot find -lwebkitgtk-6.0` 等**：系统库未装，确认 `apt-get install` 步骤在跑且为 `ubuntu-24.04`。
- **Bundle 面板空白**：检查「构建前端」步骤是否真的上传成功；插件与 Vite 8 不兼容时该步骤会 `continue-on-error` 失败但不阻断。
- **Tests 面板空白**：检查 `test-results.json` 是否生成、`vitest-json-to-junit.mjs` 是否产出 `test-report.xml`、`codecov/test-results-action` 是否拿到 token。
- **覆盖率双份/互相覆盖**：确认 `.github/codecov.yml` 的 `flags` 已区分 `go` / `node`，且 `status` 为 `informational`。

## 7. 注意事项

- GA 与 CNB 都跑一遍测试，各管各的、互不依赖（GA 生成产物并上报 Codecov，CNB 负责门禁）。
- `frontend/package.json` 的 `packageManager` 为 `pnpm@12.14.0`，`frontend/.nvmrc` 为 `25.9.0`——工作流用 `node-version-file` / `go-version-file` 动态读取，不要写死版本号。
- 文档命名注意 `docs/*-*-*.md`（双横线）被 `.gitignore` 忽略，本文档用单横线命名（`docs/codecov.md`）。
- **Codecov 配置文件位置**：统一放 `.github/codecov.yml`。不要新增根目录 `codecov.yaml`——Codecov 先搜根目录， 会覆盖 `.github` 下的配置并丢掉既有门禁 / 忽略 / flags 规则。
