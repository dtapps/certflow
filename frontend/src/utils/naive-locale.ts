// naive-ui 语言配置：根据应用 i18n 语言解析出 naive-ui 的 locale / dateLocale，
// 供 n-config-provider 全局使用，让内置组件（日期选择器、分页、空状态等）文案跟随切换。
import { computed } from 'vue'
import { zhCN, dateZhCN, enUS, dateEnUS } from 'naive-ui'
import type { NLocale, NDateLocale } from 'naive-ui'
import { useI18nStore } from '../stores/i18n'

// 与 i18n store 保持一致的系统语言探测逻辑
function getSystemLocale(): 'zh-CN' | 'en-US' {
  const lang = navigator.language || ''
  return lang.startsWith('zh') ? 'zh-CN' : 'en-US'
}

export function useNaiveLocale() {
  const i18n = useI18nStore()

  const resolved = computed<'zh-CN' | 'en-US'>(() => {
    if (i18n.locale === 'auto') return getSystemLocale()
    return i18n.locale
  })

  const naiveLocale = computed<NLocale>(() => (resolved.value === 'zh-CN' ? zhCN : enUS))
  const naiveDateLocale = computed<NDateLocale>(() =>
    resolved.value === 'zh-CN' ? dateZhCN : dateEnUS,
  )

  return { naiveLocale, naiveDateLocale }
}
