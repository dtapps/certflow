import { useI18nStore } from '../stores/i18n'

export function formatRelativeTime(ts: string) {
  const { t } = useI18nStore()
  const d = new Date(ts.replace(' ', 'T'))
  if (isNaN(d.getTime())) return ts
  const now = new Date()
  const diffMs = now.getTime() - d.getTime()
  const diffMin = Math.floor(diffMs / 60000)
  if (diffMin < 1) return t('topbar.justNow')
  if (diffMin < 60) return t('topbar.minutesAgo').replace('{count}', String(diffMin))
  const diffHr = Math.floor(diffMin / 60)
  if (diffHr < 24) return t('topbar.hoursAgo').replace('{count}', String(diffHr))
  return d.toLocaleDateString('zh-CN', { month: 'short', day: 'numeric' })
}

export function formatDateTime(ts: string) {
  if (!ts) return '--'
  const d = new Date(ts.replace(' ', 'T'))
  if (isNaN(d.getTime())) return ts
  return d.toLocaleString('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })
}
