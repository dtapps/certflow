import * as SystemService from '@bindings/cnb.cool/dtapp/certflow/systemservicewrapper'

/**
 * 前端错误统一上报：把运行时错误与 console 输出发到后端，由后端的 logging 包
 * 落盘到独立的 frontend.log 文件（不混入 certflow.log）。
 *
 * 用途：发布版（production）esbuild 会 drop 掉 console/debugger，前端错误被静默吞噬，
 * 无法像开发版那样在控制台查看。统一拦截（window 错误、Promise 拒绝、Vue 错误、
 * console.error/warn/info）后上报，便于在生产环境定位问题。
 */

// 去重时间戳缓存：同一错误 5s 内只上报一次，避免刷屏。
const dedupCache = new Map<string, number>()
const DEDUP_TTL = 5000 // ms

// 保存原始 console 引用（必须在 patchConsole 改写 console 之前）。
// send() 的开发版调试打印、patchConsole 的 original 都必须走这里，否则
// 用被包装后的 console.* 会再次触发 send()，造成无限递归、日志自嵌套刷屏。
const origConsole: Pick<Console, 'error' | 'warn' | 'info' | 'log' | 'debug'> = {
  error: console.error.bind(console),
  warn: console.warn.bind(console),
  info: console.info.bind(console),
  log: console.log.bind(console),
  debug: console.debug.bind(console),
}

function fingerprint(level: string, message: string): string {
  return level + ':' + message.slice(0, 200)
}

function send(level: string, message: string, stack?: string, url?: string) {
  const fp = fingerprint(level, message)
  const now = Date.now()
  const last = dedupCache.get(fp) || 0
  if (now - last < DEDUP_TTL) return
  dedupCache.set(fp, now)

  // 开发版同步打印到控制台，便于立即确认拦截器是否生效。
  // 必须用原始 console（origConsole），否则触发包装后的 console 再次进入 send() 形成递归。
  if (import.meta.env.DEV) {
    origConsole.error(`[errorReporter] ${level}: ${message}`, stack)
  }

  // 异步上报，不阻塞主流程；失败静默忽略（避免二次报错循环）。
  SystemService.ReportFrontendError(level, message, stack || '', url || '').catch(() => {})
}

export function reportError(message: string, stack?: string, url?: string) {
  send('error', message, stack, url)
}

export function reportWarn(message: string, stack?: string, url?: string) {
  send('warn', message, stack, url)
}

// 将 console 的变参格式化为可读 message 与 stack（若参数含 Error 实例则取 stack）。
function formatArgs(args: unknown[]): { message: string; stack: string } {
  const parts: string[] = []
  let stack = ''
  for (const a of args) {
    if (a instanceof Error) {
      stack = a.stack || stack
      parts.push(a.message)
    } else if (typeof a === 'object' && a !== null) {
      try {
        parts.push(JSON.stringify(a))
      } catch {
        parts.push(String(a))
      }
    } else {
      parts.push(String(a))
    }
  }
  return { message: parts.join(' '), stack }
}

// 包装 console.error / console.warn / console.info：
// 保留原始输出（开发版仍可见），同时上报到 frontend.log。
// 注意：必须在 vite.config 取消 production 下对 console 的 esbuild.drop，否则
// 生产版这些 console 语句会被编译期删除，包装也一并失效。
function patchConsole() {
  const targets: Array<['error' | 'warn' | 'info' | 'log' | 'debug', string]> = [
    ['error', 'error'],
    ['warn', 'warn'],
    ['info', 'info'],
    ['log', 'info'],
    ['debug', 'debug'],
  ]
  for (const [method, level] of targets) {
    const original = origConsole[method]
    console[method] = (...args: unknown[]) => {
      original(...args)
      const { message, stack } = formatArgs(args)
      send(level, message, stack, location.href)
    }
  }
}

/**
 * 安装全局错误拦截器。应在 createApp 之后、mount 之前尽早调用，
 * 以覆盖模块加载期到运行期的完整错误生命周期。
 */
export function installErrorReporter() {
  // 1. 捕获阶段监听器（Webkit/Wails WebView 下比 window.onerror 更可靠，
  //    且能捕获资源加载错误）。
  window.addEventListener(
    'error',
    (event) => {
      const e = event as ErrorEvent
      const message = e.message || String(event)
      const stack = e.error?.stack || [e.filename, `:${e.lineno}:${e.colno}`].join('')
      reportError(message, stack, e.filename || undefined)
    },
    true,
  )

  // 2. window.onerror 作为补充（部分环境仅此能触发）。
  window.onerror = function (msg, source, line, col, error) {
    const message = typeof msg === 'string' ? msg : String(msg)
    const stack = error?.stack || [source, `:${line}:${col}`].join('')
    reportError(message, stack, source || undefined)
    return false
  }

  // 3. 未处理的 Promise 拒绝（async 异常、fetch 失败等）。
  window.addEventListener('unhandledrejection', (event) => {
    const reason = event.reason
    const message = reason?.message || String(reason)
    const stack = reason?.stack || ''
    reportError('Unhandled Promise Rejection: ' + message, stack)
  })

  // 4. 拦截 console.error / warn / info，统一上报。
  patchConsole()
}

// Vue 的 errorHandler 需要绑定到 app 实例，单独导出供 main.ts 使用。
export function vueErrorHandler(err: unknown, _instance: unknown, info: string) {
  const e = err as Error
  const message = e?.message || String(err)
  const stack = e?.stack || ''
  reportError('Vue ' + info + ': ' + message, stack)
}
