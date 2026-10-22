import { defineConfig, mergeConfig } from 'vitest/config'
import viteConfig from './vite.config'

// 单元测试配置：复用 vite.config 的插件与别名（@bindings / naive-ui 解析器等），
// 仅额外声明测试环境与覆盖率。本区块自包含，可随 package.json 的 test 脚本
// 一起复制到其他 Vite + Vue 项目使用。
//
// 环境为 node：当前测试面向纯函数与 Pinia store，无需 DOM；
// 后续如需组件测试（@vue/test-utils），将 environment 改为 jsdom/happy-dom 并安装对应依赖。
export default mergeConfig(
  viteConfig,
  defineConfig({
    test: {
      environment: 'node',
      setupFiles: ['./src/test/setup.ts'],
      coverage: {
        provider: 'v8',
        // lcov 供 CI（CNB testing:coverage）解析，text 供本地阅读
        reporter: ['lcov', 'text'],
        reportsDirectory: './coverage',
        include: ['src/**/*.{ts,vue}'],
        exclude: [
          'src/test/**', // 测试自身不计入
          'src/main.ts', // 入口：挂载 DOM / 读 navigator.userAgent，导入即有副作用
          'src/logviewer/main.ts', // 第二个入口（log-viewer.html），同上
          'src/**/*.d.ts', // 生成/环境声明文件
        ],
        // 全量度量：未测试文件按 0% 计入分母，指标不虚高（与 Go 侧整包度量同思路）
        all: true,
      },
    },
  }),
)
