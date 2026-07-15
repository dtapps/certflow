import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";
import wails from "@wailsio/runtime/plugins/vite";
import tailwindcss from "@tailwindcss/vite";
import { fileURLToPath, URL } from "node:url";

// https://vitejs.dev/config/
export default defineConfig({
  base: "./",
  server: {
    host: "127.0.0.1",
    port: Number(process.env.WAILS_VITE_PORT) || 9245,
    strictPort: true,
  },
  // Vue 3.6：Vapor Mode（实验性编译策略，跳过虚拟 DOM 提升性能）。
  // 采用组件级 opt-in：在目标组件的 <script setup> 中加 defineOptions({ vapor: true }) 即可，
  // 不要在此全局开启 features.vapor（会把所有 <script setup> SFC 强制编译为 Vapor，
  // 而 naive-ui 重度依赖 slot/teleport/自定义指令，全局开启会编译失败）。
  plugins: [vue(), wails("./bindings"), tailwindcss()],
  resolve: {
    alias: {
      "@bindings": fileURLToPath(new URL("./bindings", import.meta.url)),
    },
  },
  build: {
    target: "es2020",
    rollupOptions: {
      input: {
        main: "index.html",
        "log-viewer": "log-viewer.html",
      },
    },
  },
  esbuild: {
    drop: process.env.NODE_ENV === "production" ? ["console", "debugger"] : [],
  },
});
