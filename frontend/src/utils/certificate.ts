import { useI18n } from '../stores/i18n'

export function getStatusBadge(status: string) {
  const { t } = useI18n()
  const map: Record<string, { text: string; class: string }> = {
    active: { text: t('certs.active'), class: 'bg-success-soft text-success border-success-soft' },
    pending: { text: t('certs.pending'), class: 'bg-amber-soft text-warning border-amber-soft' },
    expired: { text: t('certs.expired'), class: 'bg-error-soft text-error border-error-soft' },
    revoked: { text: t('certs.revoked'), class: 'bg-base-300-moderate text-content-70 border-base-300' },
    failed: { text: t('certs.failed'), class: 'bg-error-soft text-error border-error-soft' },
  }
  return map[status] || { text: status, class: 'bg-base-300-moderate text-content-70 border-base-300' }
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
  if (days === null) return 'text-content-50'
  if (days <= 7) return 'text-error'
  if (days <= 30) return 'text-warning'
  return 'text-success'
}

export function getDaysLeftBgClass(days: number | null): string {
  if (days === null) return 'text-content-50 bg-base-300'
  if (days <= 7) return 'text-error bg-error-soft'
  if (days <= 14) return 'text-warning bg-amber-soft'
  return 'text-success bg-success-soft'
}
