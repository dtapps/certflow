import { describe, it, expect, beforeEach } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useI18nStore } from '../stores/i18n'

beforeEach(() => {
  // 每个用例fresh store + 清空语言持久化，保证默认中文且互不影响
  localStorage.clear()
  setActivePinia(createPinia())
})

describe('i18n store', () => {
  it('默认（auto + 系统中文）解析为 zh-CN', () => {
    const store = useI18nStore()
    expect(store.getLocale()).toBe('auto')
    expect(store.getResolvedLocale()).toBe('zh-CN')
  })

  it('t() 替换参数占位符', () => {
    const { t } = useI18nStore()
    expect(t('topbar.minutesAgo', { count: 5 })).toBe('5 分钟前')
    expect(t('common.intervalSeconds', { count: 30 })).toBe('30 秒')
  })

  it('未知 key 原样返回', () => {
    const { t } = useI18nStore()
    expect(t('no.such.key')).toBe('no.such.key')
  })

  it('切换到 en-US 后返回英文文案', () => {
    const store = useI18nStore()
    store.setLocale('en-US')
    expect(store.getResolvedLocale()).toBe('en-US')
    expect(store.t('monitor.title')).toBe('Domain Monitor')
    expect(store.t('common.intervalSeconds', { count: 30 })).toBe('30s')
  })

  it('setLocale 持久化到 localStorage', () => {
    const store = useI18nStore()
    store.setLocale('en-US')
    expect(localStorage.getItem('certflow-locale')).toBe('en-US')
  })
})
