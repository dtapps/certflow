// 单元测试环境桩：为 node 环境补齐浏览器 API。
// i18n store 等在模块导入期即读取 localStorage / navigator.language，
// 不补桩会导致存储区测试在加载阶段崩溃。

const memory = new Map<string, string>()

const localStorageStub: Storage = {
  get length() {
    return memory.size
  },
  clear() {
    memory.clear()
  },
  getItem(key: string) {
    return memory.get(key) ?? null
  },
  key(index: number) {
    return Array.from(memory.keys())[index] ?? null
  },
  removeItem(key: string) {
    memory.delete(key)
  },
  setItem(key: string, value: string) {
    memory.set(key, value)
  },
}

Object.defineProperty(globalThis, 'localStorage', {
  value: localStorageStub,
  configurable: true,
  writable: true,
})

// navigator.language：node 无此属性；i18n 'auto' 模式解析系统语言时需要。
// 固定为 zh-CN，使测试默认走中文文案（需英文用例时用 setLocale('en-US') 切换）。
Object.defineProperty(globalThis, 'navigator', {
  value: { language: 'zh-CN' },
  configurable: true,
  writable: true,
})

// 屏蔽 debug 噪音（parseDateTime 等函数每次调用都打 debug 日志）；
// console.error 保留可见，避免掩盖真实问题。
console.debug = () => {}
