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
  if (!notAfter || status === 'failed' || status === 'pending') {
    return null
  }
  const expiry = new Date(notAfter)
  if (isNaN(expiry.getTime())) {
    return null
  }
  return Math.ceil((expiry.getTime() - Date.now()) / 86400000)
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
