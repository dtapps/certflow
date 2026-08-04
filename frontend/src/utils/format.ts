import { useI18nStore } from '../stores/i18n'

/**
 * 统一解析后端时间字符串为 Date。
 *
 * 后端时间字段格式不统一，需全部归一到此函数解析：
 *  - time.DateTime："2026-08-04 10:12:24"（空格分隔、无时区），Safari/WebKit 需替换空格为 'T'。
 *  - RFC3339Nano："2026-08-04 10:12:24.680589+08:00"（带微秒和时区偏移）。
 *  - Go time.Time.String()："2026-08-03 16:02:31.918573 +0800 CST m=+70.648478917"
 *    （含时区名 CST 与单调时钟 m=+... 后缀，JS 无法解析，需剥离）。
 *
 * 所有时间解析都必须走此函数，避免各处重复处理导致漏改（见证书剩余天数正式版不显示 bug）。
 */
export function parseDateTime(ts: string | undefined | null): Date | null {
  if (!ts) return null
  let s = ts.trim()

  // 剥离 Go time.Time.String() 的额外后缀：单调时钟 " m=+1.23" 与时区名（CST/UTC 等）。
  // 例："2026-08-03 16:02:31.918573 +0800 CST m=+70.64" -> "2026-08-03 16:02:31.918573 +0800"
  s = s.replace(/\s+m=\+[\d.]+\s*$/, '')
  s = s.replace(/\s+[A-Z]{2,5}\s+m=/, ' ').replace(/\s+[A-Z]{2,5}$/, '')

  // 归一多余空格（不影响时区偏移符号）
  s = s.replace(/\s+/g, ' ')

  // 空格分隔转 ISO：仅替换第一个空格（日期与时间之间），保留时区偏移里的符号
  const normalized = s.includes(' ') ? s.replace(' ', 'T') : s
  const d = new Date(normalized)
  return isNaN(d.getTime()) ? null : d
}

export function formatRelativeTime(ts: string) {
  const { t } = useI18nStore()
  const d = parseDateTime(ts)
  if (!d) return ts
  const now = new Date()
  const diffMs = now.getTime() - d.getTime()
  const diffMin = Math.floor(diffMs / 60000)
  if (diffMin < 1) return t('topbar.justNow')
  if (diffMin < 60) return t('topbar.minutesAgo').replace('{count}', String(diffMin))
  const diffHr = Math.floor(diffMin / 60)
  if (diffHr < 24) return t('topbar.hoursAgo').replace('{count}', String(diffHr))
  return `${d.getFullYear()}-${pad2(d.getMonth() + 1)}-${pad2(d.getDate())}`
}

function pad2(n: number): string {
  return n < 10 ? '0' + n : String(n)
}

/**
 * 统一时间显示格式：YYYY-MM-DD HH:mm（日期用“-”分隔，时间用“:”分隔）。
 * 不使用 toLocaleString 的本地化输出（“/”分隔），避免中英环境下显示不统一、看着奇怪。
 */
export function formatDateTime(ts: string) {
  if (!ts) return '--'
  const d = parseDateTime(ts)
  if (!d) return ts
  return (
    `${d.getFullYear()}-${pad2(d.getMonth() + 1)}-${pad2(d.getDate())} ` +
    `${pad2(d.getHours())}:${pad2(d.getMinutes())}`
  )
}
