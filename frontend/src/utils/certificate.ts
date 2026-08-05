import { parseDateTime } from './format'
import { useI18nStore } from '../stores/i18n'

export function getStatusBadge(status: string) {
  const { t } = useI18nStore()
  const map: Record<string, { text: string; type: string }> = {
    active: { text: t('certs.active'), type: 'success' },
    pending: { text: t('certs.pending'), type: 'warning' },
    expired: { text: t('certs.expired'), type: 'error' },
    revoked: { text: t('certs.revoked'), type: 'default' },
    failed: { text: t('certs.failed'), type: 'error' },
  }
  return map[status] || { text: status, type: 'default' }
}

export function getDaysLeft(notAfter: string, status?: string): number | null {
  const { t } = useI18nStore()
  if (!notAfter || status === 'failed' || status === 'pending') {
    return null
  }
  // 统一走 parseDateTime（处理后端 time.DateTime 空格格式 + WebKit 兼容 + 非法值兜底）。
  const expiry = parseDateTime(notAfter)
  if (!expiry) {
    console.error(
      t('log.getDaysLeft', { notAfter, status: String(status), expiry: 'null', days: 'null' }),
    )
    return null
  }
  console.debug(
    t('log.getDaysLeft', {
      notAfter,
      status: String(status),
      expiry: String(expiry),
      days: String(Math.floor((expiry.getTime() - Date.now()) / 86400000)),
    }),
  )
  // 用 floor 而非 ceil：剩余天数向下取整（已过期返回负数），避免 ceil 把「还差不到 1 天」算成 2 天、把「刚过期」掩盖成 0 天。
  return Math.floor((expiry.getTime() - Date.now()) / 86400000)
}

export function getDaysLeftClass(days: number | null): string {
  if (days === null) return 'opacity-50'
  if (days <= 7) return 'text-red-500'
  if (days <= 30) return 'text-yellow-500'
  return 'text-green-500'
}

export function getDaysLeftBgClass(days: number | null): string {
  if (days === null) return 'opacity-50 bg-neutral-100 dark:bg-neutral-800'
  if (days <= 7) return 'text-red-500 bg-red-50 dark:bg-red-900/30'
  if (days <= 14) return 'text-yellow-500 bg-yellow-50 dark:bg-yellow-900/30'
  return 'text-green-500 bg-green-50 dark:bg-green-900/30'
}
