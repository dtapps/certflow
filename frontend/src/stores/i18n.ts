// ⚠️ 此文件由 scripts/merge-frontend-i18n.js 自动生成，请勿手动编辑！
// 修改请编辑 src/locales/split/ 目录下的 JSON 文件后重新运行 make i18n

import { ref, computed } from 'vue'
import { defineStore } from 'pinia'

import zhCN from '../locales/zh-CN.json'
import enUS from '../locales/en-US.json'

export type Locale = 'zh-CN' | 'en-US' | 'auto'

type ResolvedLocale = 'zh-CN' | 'en-US'

function getSystemLocale(): ResolvedLocale {
  const lang = navigator.language || ''
  return lang.startsWith('zh') ? 'zh-CN' : 'en-US'
}

const messages: Record<ResolvedLocale, Record<string, string>> = {
  'zh-CN': zhCN,
  'en-US': enUS,
}

export const useI18nStore = defineStore('i18n', () => {
  const locale = ref<Locale>((localStorage.getItem('certflow-locale') as Locale) || 'auto')
  const LOCALE_KEY = 'certflow-locale'

  // 解析后的语言（不含 auto）：auto 跟随系统语言
  const resolved = computed<ResolvedLocale>(() => {
    if (locale.value === 'auto') return getSystemLocale()
    return locale.value
  })

  // 读取原始语言（含 auto）：返回用户的选择，用于设置项展示与持久化。
  const getLocale = (): Locale => locale.value

  // 读取解析后的语言（不含 auto）：用于翻译、cnb/GitHub 源判断、后端同步等。
  const getResolvedLocale = (): ResolvedLocale => resolved.value

  const setLocale = (newLocale: Locale) => {
    locale.value = newLocale
    localStorage.setItem(LOCALE_KEY, newLocale)
  }

  const t = (key: string, params?: Record<string, string | number>): string => {
    const msg = messages[resolved.value]?.[key] || messages['zh-CN']?.[key] || key
    if (!params) return msg
    return Object.entries(params).reduce(
      (str, [k, v]) => str.replace(new RegExp(`\\{${k}\\}`, 'g'), String(v)),
      msg,
    )
  }

  return {
    locale,
    resolved,
    getLocale,
    getResolvedLocale,
    t,
    setLocale,
  }
})

export type { ResolvedLocale }
