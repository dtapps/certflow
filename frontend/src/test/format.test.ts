import { describe, it, expect, beforeEach } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { parseDateTime, formatInterval, formatDateTime, formatRelativeTime } from '../utils/format'

beforeEach(() => {
  setActivePinia(createPinia())
})

describe('parseDateTime', () => {
  it('空值返回 null', () => {
    expect(parseDateTime('')).toBeNull()
    expect(parseDateTime(null)).toBeNull()
    expect(parseDateTime(undefined)).toBeNull()
  })

  it('解析 time.DateTime 格式（空格分隔无时区）', () => {
    const d = parseDateTime('2026-08-04 10:12:24')
    expect(d).not.toBeNull()
    expect(d!.getFullYear()).toBe(2026)
    expect(d!.getMonth()).toBe(7) // 8 月 = index 7
    expect(d!.getDate()).toBe(4)
    expect(d!.getHours()).toBe(10)
    expect(d!.getMinutes()).toBe(12)
  })

  it('解析 RFC3339Nano 格式（带微秒与时区偏移）', () => {
    const d = parseDateTime('2026-08-04T10:12:24.680589+08:00')
    expect(d).not.toBeNull()
    expect(d!.getFullYear()).toBe(2026)
    expect(d!.getMonth()).toBe(7)
  })

  it('解析 Go time.Time.String() 格式（剥离 CST 与单调时钟后缀）', () => {
    const d = parseDateTime('2026-08-03 16:02:31.918573 +0800 CST m=+70.648478917')
    expect(d).not.toBeNull()
    expect(d!.getFullYear()).toBe(2026)
    expect(d!.getMonth()).toBe(7)
    expect(d!.getDate()).toBe(3)
  })

  it('无法解析的输入返回 null', () => {
    expect(parseDateTime('not-a-date')).toBeNull()
  })
})

describe('formatInterval', () => {
  it('非正值显示占位符', () => {
    expect(formatInterval(0)).toBe('—')
    expect(formatInterval(-5)).toBe('—')
  })

  it('按量级切换秒/分钟/小时/天', () => {
    expect(formatInterval(30)).toBe('30 秒')
    expect(formatInterval(90)).toBe('2 分钟') // Math.round(90/60)
    expect(formatInterval(3600)).toBe('1 小时')
    expect(formatInterval(5400)).toBe('1.5 小时') // 非整数保留 1 位小数
    expect(formatInterval(90000)).toBe('1 天') // 1.04 天四舍五入为 1
    expect(formatInterval(129600)).toBe('1.5 天')
  })
})

describe('formatDateTime', () => {
  it('空值显示占位符', () => {
    expect(formatDateTime('')).toBe('--')
  })

  it('统一输出 YYYY-MM-DD HH:mm', () => {
    expect(formatDateTime('2026-08-04 10:12:24')).toBe('2026-08-04 10:12')
  })

  it('无法解析时原样返回', () => {
    expect(formatDateTime('garbage')).toBe('garbage')
  })
})

describe('formatRelativeTime', () => {
  it('1 分钟内显示“刚刚”', () => {
    const ts = new Date(Date.now() - 30 * 1000).toISOString()
    expect(formatRelativeTime(ts)).toBe('刚刚')
  })

  it('1 小时内显示分钟', () => {
    const ts = new Date(Date.now() - 5 * 60 * 1000).toISOString()
    expect(formatRelativeTime(ts)).toBe('5 分钟前')
  })

  it('24 小时内显示小时', () => {
    const ts = new Date(Date.now() - 210 * 60 * 1000).toISOString() // 3.5 小时，避开边界抖动
    expect(formatRelativeTime(ts)).toBe('3 小时前')
  })

  it('超过 24 小时显示日期', () => {
    const d = new Date(Date.now() - 3 * 24 * 60 * 60 * 1000)
    const pad = (n: number) => (n < 10 ? '0' + n : String(n))
    const expected = `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
    expect(formatRelativeTime(d.toISOString())).toBe(expected)
  })

  it('无法解析时原样返回', () => {
    expect(formatRelativeTime('garbage')).toBe('garbage')
  })
})
