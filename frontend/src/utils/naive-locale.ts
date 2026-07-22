// naive-ui 语言配置：根据应用 i18n 语言解析出 naive-ui 的 locale / dateLocale，
// 供 n-config-provider 全局使用，让内置组件（日期选择器、分页、空状态等）文案跟随切换。
import { computed } from 'vue'
import { zhCN, dateZhCN, enUS, dateEnUS } from 'naive-ui'
import type { NLocale, NDateLocale } from 'naive-ui'
import { useI18nStore } from '../stores/i18n'

// 直接复用 i18n store 的 resolved（auto 已在 store 内解析），避免重复探测逻辑。
export function useNaiveLocale() {
  const i18n = useI18nStore()

  const naiveLocale = computed<NLocale>(() => (i18n.resolved === 'zh-CN' ? zhCN : enUS))
  const naiveDateLocale = computed<NDateLocale>(() =>
    i18n.resolved === 'zh-CN' ? dateZhCN : dateEnUS,
  )

  return { naiveLocale, naiveDateLocale }
}
