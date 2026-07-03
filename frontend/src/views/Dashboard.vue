<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRouter } from 'vue-router'
import * as CertificateService from '@bindings/cnb.cool/dtapp/certflow/certificateservicewrapper'
import * as CAService from '@bindings/cnb.cool/dtapp/certflow/caservicewrapper'
import * as SchedulerService from '@bindings/cnb.cool/dtapp/certflow/schedulerservicewrapper'
import type { CertificateListItem, RenewalLogItem } from '@bindings/cnb.cool/dtapp/certflow/models'
import { useI18n } from '../stores/i18n'
import { getDaysLeftBgClass } from '../utils/certificate'
import { formatRelativeTime } from '../utils/format'

const router = useRouter()
const { t } = useI18n()

const certificates = ref<CertificateListItem[]>([])
const expiringCerts = ref<CertificateListItem[]>([])
const renewalLogs = ref<RenewalLogItem[]>([])
const cas = ref<import('@bindings/cnb.cool/dtapp/certflow/models').CAListItem[]>([])
const isLoading = ref(false)

const stats = computed(() => ({
  totalCerts: certificates.value.length,
  activeCerts: certificates.value.filter(c => c.status === 'active').length,
  expiringCerts: expiringCerts.value.length,
  cas: cas.value.length,
}))

const recentActivity = computed(() => {
  return renewalLogs.value.map(log => ({
    id: log.id,
    action: log.status === 'success' ? t('dashboard.renewSuccess') : log.status === 'failed' ? t('dashboard.renewFailed') : t('dashboard.renew'),
    domain: log.domain,
    time: log.attempt_at,
    status: log.status === 'success' ? 'success' : log.status === 'failed' ? 'error' : 'warning',
  }))
})

const expiringCertificates = computed(() => {
  return expiringCerts.value.map(c => {
    const daysLeft = Math.ceil((new Date(c.not_after).getTime() - Date.now()) / 86400000)
    return { id: c.id, domain: c.domain, notAfter: c.not_after, daysLeft }
  })
})

onMounted(async () => {
  isLoading.value = true
  try {
    const [certs, expiring, logs, caList] = await Promise.all([
      CertificateService.ListCertificates(),
      CertificateService.GetExpiringCertificates(30),
      SchedulerService.GetRecentRenewalLogs(5),
      CAService.ListCA(),
    ])
    certificates.value = certs ?? []
    expiringCerts.value = expiring ?? []
    renewalLogs.value = logs ?? []
    cas.value = caList ?? []
  } catch (e) {
    console.error(t('dashboard.loadFailed'), e)
  } finally {
    isLoading.value = false
  }
})

const getStatusColor = (status: string) => {
  switch (status) {
    case 'success': return 'text-success'
    case 'warning': return 'text-warning'
    case 'error': return 'text-error'
    default: return 'text-content-80'
  }
}
</script>

<template>
  <div class="page">
    <!-- 页面标题 -->
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-base-content">{{ t('dashboard.title') }}</h1>
        <p class="text-content-70 text-sm mt-1">{{ t('dashboard.subtitle') }}</p>
      </div>
      <button @click="router.push('/certificates/apply')" class="btn btn-primary">
        <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
        </svg>
        {{ t('dashboard.apply') }}
      </button>
    </div>

    <!-- 统计卡片 -->
    <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
      <div class="stat-card group">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-content-70 text-sm">{{ t('dashboard.totalCerts') }}</p>
            <p class="text-3xl font-bold text-base-content mt-1">{{ stats.totalCerts }}</p>
          </div>
          <div class="w-12 h-12 rounded-xl bg-primary-soft flex items-center justify-center group-hover:bg-primary-soft transition-colors">
            <svg class="w-6 h-6 text-primary" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z" />
            </svg>
          </div>
        </div>
      </div>

      <div class="stat-card group">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-content-70 text-sm">{{ t('dashboard.activeCerts') }}</p>
            <p class="text-3xl font-bold text-success mt-1">{{ stats.activeCerts }}</p>
          </div>
          <div class="w-12 h-12 rounded-xl bg-success-soft flex items-center justify-center group-hover:bg-success-soft transition-colors">
            <svg class="w-6 h-6 text-success" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
            </svg>
          </div>
        </div>
      </div>

      <div class="stat-card group">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-content-70 text-sm">{{ t('dashboard.expiringCerts') }}</p>
            <p class="text-3xl font-bold text-warning mt-1">{{ stats.expiringCerts }}</p>
          </div>
          <div class="w-12 h-12 rounded-xl bg-amber-soft flex items-center justify-center group-hover:bg-amber-soft transition-colors">
            <svg class="w-6 h-6 text-warning" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
            </svg>
          </div>
        </div>
      </div>

      <div class="stat-card group">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-content-70 text-sm">{{ t('dashboard.caCount') }}</p>
            <p class="text-3xl font-bold text-primary mt-1">{{ stats.cas }}</p>
          </div>
          <div class="w-12 h-12 rounded-xl bg-accent-soft flex items-center justify-center group-hover:bg-accent-soft transition-colors">
            <svg class="w-6 h-6 text-accent" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 21V5a2 2 0 00-2-2H7a2 2 0 00-2 2v16m14 0h2m-2 0h-5m-9 0H3m2 0h5M9 7h1m-1 4h1m4-4h1m-1 4h1m-5 10v-5a1 1 0 011-1h2a1 1 0 011 1v5m-4 0h4" />
            </svg>
          </div>
        </div>
      </div>
    </div>

    <!-- 主要内容区 -->
    <div class="grid grid-cols-1 lg:grid-cols-3 gap-6">
      <!-- 即将过期证书 -->
      <div class="lg:col-span-2 glass-panel rounded-2xl p-6">
        <div class="flex items-center justify-between mb-4">
          <h2 class="text-lg font-semibold text-base-content">{{ t('dashboard.expiringTitle') }}</h2>
          <router-link to="/certificates" class="text-sm text-primary hover:text-primary transition-colors">
            {{ t('dashboard.viewAll') }}
          </router-link>
        </div>

        <div v-if="isLoading" class="flex items-center justify-center py-12">
          <div class="spinner-sm animate-spin"></div>
        </div>

        <div v-else-if="expiringCertificates.length === 0" class="text-center py-12">
          <svg class="w-16 h-16 mx-auto text-content-30 mb-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
          </svg>
          <p class="text-content-70">{{ t('dashboard.noExpiring') }}</p>
        </div>

        <div v-else class="space-y-3">
          <div
            v-for="cert in expiringCertificates"
            :key="cert.id"
            class="flex items-center justify-between p-4 rounded-xl bg-base-200-faint hover:bg-base-200 transition-colors cursor-pointer"
            @click="router.push(`/certificates/${cert.id}`)"
          >
            <div class="flex items-center gap-3">
              <div class="w-10 h-10 rounded-lg bg-base-300 flex items-center justify-center">
                <svg class="w-5 h-5 text-content-80" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z" />
                </svg>
              </div>
              <div>
                <p class="text-base-content font-medium">{{ cert.domain }}</p>
                <p class="text-content-70 text-sm">{{ t('dashboard.expiryTime') }} {{ cert.notAfter }}</p>
              </div>
            </div>
            <span
              class="px-3 py-1 rounded-full text-sm font-medium"
                  :class="getDaysLeftBgClass(cert.daysLeft)"
            >
              {{ t('dashboard.daysLeft').replace('{count}', String(cert.daysLeft)) }}
            </span>
          </div>
        </div>
      </div>

      <!-- 最近活动 -->
      <div class="glass-panel rounded-2xl p-6">
        <h2 class="text-lg font-semibold text-base-content mb-4">{{ t('dashboard.recentActivity') }}</h2>

        <div v-if="isLoading" class="flex items-center justify-center py-12">
          <div class="spinner-sm animate-spin"></div>
        </div>

        <div v-else class="space-y-4">
          <div
            v-for="activity in recentActivity"
            :key="activity.id"
            class="flex items-start gap-3"
          >
            <div
              class="w-2 h-2 rounded-full mt-2 flex-shrink-0"
              :class="{
                'bg-success': activity.status === 'success',
                'bg-warning': activity.status === 'warning',
                'bg-error': activity.status === 'error',
              }"
            ></div>
            <div class="flex-1 min-w-0">
              <p class="text-base-content text-sm font-medium">{{ activity.action }}</p>
              <p class="text-content-70 text-xs truncate">{{ activity.domain }}</p>
            </div>
            <span class="text-content-50 text-xs flex-shrink-0">{{ formatRelativeTime(activity.time) }}</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
