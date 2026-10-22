# Node 单元测试覆盖率实现文案（可复制到其他项目交给 AI 实现）

> 来源：CertFlow 项目实战方案（CNB 云原生构建 + testing:coverage 内置任务 + vitest）。

# 任务：为 Node/Vite/Vue 项目实现单元测试覆盖率上报与红线门禁（CNB 云原生构建）

## 背景与目标

- 构建平台是 CNB（cnb.cool），流水线配置在 `.cnb.yml`（或 `.cnb/workflows/*.yml`）。
- 使用 CNB 内置任务 `type: testing:coverage` 上报覆盖率：push 全量红线（lines），PR 增量红线（diffLines，仅 PR 类事件计算）。
- 技术栈：Vite + Vue 3 + TypeScript + pnpm。测试框架选 **vitest**（不用 jest：jest 配 ESM+Vue SFC 需要 babel/ts-jest 全套改造，vitest 与 Vite 原生集成、零 transform 配置）。

## 实现步骤

### 第 1 步：安装依赖

```bash
pnpm add -D vitest @vitest/coverage-v8
```

### 第 2 步：创建 vitest.config.ts（放在前端目录根）

```ts
import { defineConfig, mergeConfig } from "vitest/config";
import viteConfig from "./vite.config";

// 复用 vite.config 的插件与别名，仅额外声明测试环境与覆盖率。
export default mergeConfig(
  viteConfig,
  defineConfig({
    test: {
      environment: "node", // 纯函数/store 测试无需 DOM；后续组件测试再切 jsdom
      setupFiles: ["./src/test/setup.ts"],
      coverage: {
        provider: "v8",
        reporter: ["lcov", "text"], // lcov 供 CI 插件解析，text 供本地阅读
        reportsDirectory: "./coverage",
        include: ["src/**/*.{ts,vue}"],
        exclude: [
          "src/test/**", // 测试自身不计入
          "src/main.ts", // 入口：导入期即挂载 DOM/读 navigator，node 下会崩
          "src/logviewer/main.ts", // 其他入口同理，按项目实际增删
          "src/**/*.d.ts",
        ],
        all: true, // 全量度量：未测试文件按 0% 计入分母，指标不虚高
      },
    },
  }),
);
```

### 第 3 步：创建测试环境桩 src/test/setup.ts

node 环境没有 localStorage/navigator，而 store 常在导入期读取它们，不补桩会加载即崩：

```ts
const memory = new Map<string, string>();

const localStorageStub: Storage = {
  get length() {
    return memory.size;
  },
  clear() {
    memory.clear();
  },
  getItem(key: string) {
    return memory.get(key) ?? null;
  },
  key(index: number) {
    return Array.from(memory.keys())[index] ?? null;
  },
  removeItem(key: string) {
    memory.delete(key);
  },
  setItem(key: string, value: string) {
    memory.set(key, value);
  },
};

Object.defineProperty(globalThis, "localStorage", {
  value: localStorageStub,
  configurable: true,
  writable: true,
});

// navigator.language：node 无此属性；i18n 'auto' 模式解析系统语言时需要
Object.defineProperty(globalThis, "navigator", {
  value: { language: "zh-CN" },
  configurable: true,
  writable: true,
});

// 屏蔽 debug 噪音；console.error 保留可见，避免掩盖真实问题
console.debug = () => {};
```

### 第 4 步：编写测试（从纯函数与 store 入手，不要一上来写组件测试）

每个测试文件头部：

```ts
import { describe, it, expect, beforeEach } from "vitest";
import { createPinia, setActivePinia } from "pinia";

beforeEach(() => {
  localStorage.clear();
  setActivePinia(createPinia());
});
```

优先覆盖：工具函数（时间/格式化/配置映射）、Pinia store（t() 翻译/参数替换/语言切换/持久化）、纯逻辑模块。

### 第 5 步：package.json 增加脚本

```json
"scripts": {
  "test": "vitest run",
  "test:coverage": "vitest run --coverage"
}
```

### 第 6 步：.gitignore 增加 `frontend/coverage`（或项目对应路径）

### 第 7 步：流水线配置（push 与 PR 各用独立并行流水线）

CNB 语义：事件键下的值是 Pipeline 数组，同事件多个条目 = 多流水线并行；并入同一 pipeline 的 stages 会串行，不要这样做。

```yaml
"**":
  push:
    - runner:
        tags: cnb:arch:amd64
      docker:
        build: .cnb/Dockerfile # 复用已预装 Node + pnpm 的构建环境
        volumes:
          - /root/.local/share/pnpm/store:copy-on-write # pnpm 存储缓存
      stages:
        - name: 安装前端依赖
          script: pnpm --dir ./frontend install --frozen-lockfile
        - name: 生成前端测试覆盖率
          script: pnpm --dir ./frontend test:coverage # 产出 frontend/coverage/lcov.info
        - name: 上报覆盖率（全量红线）
          type: testing:coverage
          options:
            key:
              node # 多报告数据键：仓库内有多份覆盖率报告（如同时有 Go 后端）时必须指定，
              # 否则各报告都写顶层默认字段，后跑完的覆盖先跑完的
            name: Node Coverage # 徽章显示名，与 key 一对一对应
            lines: 2 # 初始值=当前基线往下留余量（本项目基线 2.14%，视图组件尚未建测试）
            pattern: "frontend/coverage/lcov.info"
            breakIfNoCoverage: true
          exports:
            lines: NODE_COVERAGE_LINES
            diff_pct: NODE_COVERAGE_DIFF_PCT
        - name: 输出覆盖率
          script: 'echo "node coverage: lines=$NODE_COVERAGE_LINES diff=$NODE_COVERAGE_DIFF_PCT"'
  pull_request:
    - runner:
        tags: cnb:arch:amd64
      docker:
        build: .cnb/Dockerfile
        volumes:
          - /root/.local/share/pnpm/store:copy-on-write
      stages:
        - name: 安装前端依赖
          script: pnpm --dir ./frontend install --frozen-lockfile
        - name: 生成前端测试覆盖率
          script: pnpm --dir ./frontend test:coverage
        - name: 上报覆盖率（增量红线）
          type: testing:coverage
          options:
            key: node # 与 push 侧保持同一个 key，数据才落在同一槽位
            name: Node Coverage
            diffLines: 50 # 本 PR 变更代码覆盖率，低于此值阻断
            lines: 2 # 全量底线与 push 一致
            pattern: "frontend/coverage/lcov.info"
            breakIfNoCoverage: true
          exports:
            lines: NODE_COVERAGE_LINES
            diff_pct: NODE_COVERAGE_DIFF_PCT
        - name: 输出覆盖率
          script: 'echo "node coverage: lines=$NODE_COVERAGE_LINES diff=$NODE_COVERAGE_DIFF_PCT"'
```

## 关键决策（实现时遵循）

1. **vitest 而非 jest**：Vite/Vue/TS 原生集成；mergeConfig 复用 vite.config，别名与插件不丢。
2. **`all: true` 全量度量**：未测试文件按 0% 计入分母。初始数字会很低（视图组件没测试时本项目仅 2.14%），但诚实；真正驱动质量的是 PR 增量红线。
3. **排除入口文件**（main.ts 等）：导入期即 `app.mount()`/读 `navigator`，node 环境导入即崩。
4. **不设 `lang`**：lang=go 仅 golang 格式报告需要；lcov 由插件自动识别。
5. **红线棘轮**：lines 初始贴基线下沿；diffLines: 50（变更代码过半覆盖）长期不动或逐步上调。
6. **测试从纯函数/store 开始**：组件测试需要 @vue/test-utils + jsdom/happy-dom，作为第二步。
7. **YAML 细节**：script 含 `": "` 等特殊字符时整体单引号包裹；独立并行流水线，勿并入既有 stages。
8. **多报告必须用 `key` 隔离**：若仓库内还有其他语言的覆盖率报告（如 Go 后端），每份报告的 `testing:coverage` 都必须指定不同的 `key`（如 node / go）与对应 `name`。不指定 `key` 时所有报告都写顶层默认字段，**后跑完的会覆盖先跑完的**（徽章只显示最后一份）。指定 `key` 后数据各自写入 meta map 互不冲突；单仓库 key 上限 30 个。

## 验收标准

- `pnpm test` 全绿；`pnpm test:coverage` 产出 `frontend/coverage/lcov.info`，total 与预期一致。
- `vue-tsc --noEmit` 与 eslint 对新测试文件零报错（测试文件也在类型检查范围内）。
- YAML 解析正确；首次推送后确认徽章生成、pattern 解析正常、多流水线并行启动。
