<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRouter } from 'vue-router'
import { NCard, NStatistic, NSpin, NButton, NTag, NEmpty, NTimeline, NTimelineItem } from 'naive-ui'
import * as CertificateService from '@bindings/cnb.cool/dtapp/certflow/certificateservicewrapper'
import * as CAService from '@bindings/cnb.cool/dtapp/certflow/caservicewrapper'
import * as SchedulerService from '@bindings/cnb.cool/dtapp/certflow/schedulerservicewrapper'
import type { CertificateListItem, RenewalLogItem } from '@bindings/cnb.cool/dtapp/certflow/models'
import { useI18nStore } from '../stores/i18n'
import { getDaysLeftBgClass } from '../utils/certificate'
import { formatRelativeTime, parseDateTime, formatDateTime } from '../utils/format'

const router = useRouter()
const i18nStore = useI18nStore()
const { t } = i18nStore

const certificates = ref<CertificateListItem[]>([])
const expiringCerts = ref<CertificateListItem[]>([])
const renewalLogs = ref<RenewalLogItem[]>([])
const cas = ref<import('@bindings/cnb.cool/dtapp/certflow/models').CAListItem[]>([])
const isLoading = ref(false)

const stats = computed(() => ({
  totalCerts: certificates.value.length,
  activeCerts: certificates.value.filter((c) => c.status === 'active').length,
  expiringCerts: expiringCerts.value.length,
  cas: cas.value.length,
}))

const recentActivity = computed(() => {
  return renewalLogs.value.map((log) => ({
    id: log.id,
    action:
      log.status === 'success'
        ? t('dashboard.renewSuccess')
        : log.status === 'failed'
          ? t('dashboard.renewFailed')
          : t('dashboard.renew'),
    domain: log.domain,
    time: log.attempt_at,
    status: log.status === 'success' ? 'success' : log.status === 'failed' ? 'error' : 'warning',
  }))
})

const expiringCertificates = computed(() => {
  return expiringCerts.value.map((c) => {
    // 统一走 parseDateTime（处理 time.DateTime 空格格式 + WebKit 兼容）。
    const expiry = parseDateTime(c.not_after)
    const daysLeft = expiry ? Math.floor((expiry.getTime() - Date.now()) / 86400000) : null
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

const getStatusColor = (status: string): 'success' | 'error' | 'warning' | 'info' => {
  switch (status) {
    case 'success':
      return 'success'
    case 'warning':
      return 'warning'
    case 'error':
      return 'error'
    default:
      return 'info'
  }
}
</script>

<template>
  <div class="page">
    <!-- 页面标题 -->
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold">{{ t('dashboard.title') }}</h1>
        <p class="text-sm mt-1 opacity-60">{{ t('dashboard.subtitle') }}</p>
      </div>
      <n-button type="primary" @click="router.push('/certificates/apply')">
        <template #icon>
          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M12 4v16m8-8H4"
            />
          </svg>
        </template>
        {{ t('dashboard.apply') }}
      </n-button>
    </div>

    <!-- 统计卡片 -->
    <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
      <n-card size="small" hoverable>
        <n-statistic :label="t('dashboard.totalCerts')" :value="stats.totalCerts" />
      </n-card>
      <n-card size="small" hoverable>
        <n-statistic :label="t('dashboard.activeCerts')" :value="stats.activeCerts">
          <template #suffix>
            <n-tag type="success" size="small" :bordered="false">{{ t('certs.active') }}</n-tag>
          </template>
        </n-statistic>
      </n-card>
      <n-card size="small" hoverable>
        <n-statistic :label="t('dashboard.expiringCerts')" :value="stats.expiringCerts">
          <template #suffix>
            <n-tag type="warning" size="small" :bordered="false">{{ t('common.daysLeft') }}</n-tag>
          </template>
        </n-statistic>
      </n-card>
      <n-card size="small" hoverable>
        <n-statistic :label="t('dashboard.caCount')" :value="stats.cas" />
      </n-card>
    </div>

    <!-- 主要内容区 -->
    <div class="grid grid-cols-1 lg:grid-cols-3 gap-6">
      <!-- 即将过期证书 -->
      <div class="lg:col-span-2">
        <n-card :title="t('dashboard.expiringTitle')" size="small" class="h-full">
          <template #header-extra>
            <n-button text type="primary" @click="router.push('/certificates')">
              {{ t('dashboard.viewAll') }}
            </n-button>
          </template>

          <n-spin :show="isLoading">
            <n-empty
              v-if="!isLoading && expiringCertificates.length === 0"
              :description="t('dashboard.noExpiring')"
              class="empty-state"
            />

            <div v-else class="space-y-3">
              <div
                v-for="cert in expiringCertificates"
                :key="cert.id"
                class="flex items-center justify-between p-4 rounded-xl bg-neutral-50 dark:bg-neutral-800/50 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors cursor-pointer"
                @click="router.push(`/certificates/${cert.id}`)"
              >
                <div class="flex items-center gap-3">
                  <div
                    class="w-10 h-10 rounded-lg bg-blue-50 dark:bg-blue-900/30 flex items-center justify-center"
                  >
                    <svg
                      class="w-5 h-5 text-blue-500"
                      fill="none"
                      stroke="currentColor"
                      viewBox="0 0 24 24"
                    >
                      <path
                        stroke-linecap="round"
                        stroke-linejoin="round"
                        stroke-width="2"
                        d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z"
                      />
                    </svg>
                  </div>
                  <div>
                    <p class="font-medium">{{ cert.domain }}</p>
                    <p class="text-sm opacity-60">
                      {{ t('dashboard.expiryTime') }} {{ formatDateTime(cert.notAfter) }}
                    </p>
                  </div>
                </div>
                <n-tag :type="(cert.daysLeft ?? 999) <= 7 ? 'error' : 'warning'" size="small">
                  {{
                    cert.daysLeft === null
                      ? '—'
                      : t('dashboard.daysLeft').replace('{count}', String(cert.daysLeft))
                  }}
                </n-tag>
              </div>
            </div>
          </n-spin>
        </n-card>
      </div>

      <!-- 最近活动 -->
      <div>
        <n-card :title="t('dashboard.recentActivity')" size="small" class="h-full">
          <n-spin :show="isLoading">
            <n-empty
              v-if="!isLoading && recentActivity.length === 0"
              :description="t('dashboard.noActivity')"
              class="empty-state"
            />

            <n-timeline v-else>
              <n-timeline-item
                v-for="activity in recentActivity"
                :key="activity.id"
                :type="getStatusColor(activity.status)"
                :title="activity.action"
                :content="activity.domain"
                :time="formatRelativeTime(activity.time)"
              />
            </n-timeline>
          </n-spin>
        </n-card>
      </div>
    </div>
  </div>
</template>

<style scoped>
.empty-state {
  min-height: 180px;
  display: flex;
  align-items: center;
  justify-content: center;
}
</style>
