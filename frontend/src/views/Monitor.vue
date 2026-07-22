<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import {
  NCard,
  NButton,
  NInput,
  NInputNumber,
  NSelect,
  NSwitch,
  NSpin,
  NModal,
  NForm,
  NFormItem,
  NEmpty,
  NTag,
  NStatistic,
  useMessage,
} from 'naive-ui'
import * as MonitorService from '@bindings/cnb.cool/dtapp/certflow/monitorservicewrapper'
import type { MonitoredDomainItem } from '@bindings/cnb.cool/dtapp/certflow/internal/monitor/models'
import { useI18nStore } from '../stores/i18n'
import { initMessage, showMessage } from '../utils/message'
import MonitorTrendChart from '../components/MonitorTrendChart.vue'

const i18nStore = useI18nStore()
const { t } = i18nStore
const message = useMessage()
initMessage(message)

const domains = ref<MonitoredDomainItem[]>([])
const isLoading = ref(false)
const showAddModal = ref(false)
const showEditModal = ref(false)
const showDeleteModal = ref(false)
const deleteTargetId = ref<number | null>(null)
const editingId = ref<number | null>(null)
const checkingId = ref<number | null>(null)

const formData = ref({
  domain: '',
  port: 443,
  check_type: 'ssl',
  url: '',
  check_interval: 3600,
})

const loadDomains = async () => {
  isLoading.value = true
  try {
    domains.value = ((await MonitorService.List()) ?? []).filter(
      (item): item is MonitoredDomainItem => item !== null,
    )
  } catch (e) {
    console.error(t('monitor.loadDomainsFailed'), e)
  } finally {
    isLoading.value = false
  }
}

onMounted(loadDomains)

const openCreate = () => {
  editingId.value = null
  formData.value = {
    domain: '',
    port: 443,
    check_type: 'https',
    url: '',
    check_interval: 3600,
  }
  showAddModal.value = true
}

const openEdit = (item: MonitoredDomainItem) => {
  editingId.value = item.id
  formData.value = {
    domain: item.domain,
    port: item.port,
    check_type: item.check_type,
    url: item.url,
    check_interval: item.check_interval,
  }
  showEditModal.value = true
}

const handleSave = async () => {
  try {
    if (editingId.value) {
      await MonitorService.Update(editingId.value, formData.value)
    } else {
      await MonitorService.Create(formData.value)
    }
    showAddModal.value = false
    showEditModal.value = false
    await loadDomains()
    showMessage(t('monitor.saveSuccess'), 'success')
  } catch (e) {
    showMessage(t('monitor.saveFailed') + ' ' + e, 'error')
  }
}

const openDeleteModal = (id: number) => {
  deleteTargetId.value = id
  showDeleteModal.value = true
}

const handleDelete = async () => {
  if (deleteTargetId.value === null) return
  const id = deleteTargetId.value
  showDeleteModal.value = false
  deleteTargetId.value = null
  try {
    await MonitorService.Delete(id)
    await loadDomains()
    showMessage(t('monitor.deleteSuccess'), 'success')
  } catch (e) {
    showMessage(t('monitor.deleteFailed') + ' ' + e, 'error')
  }
}

const handleCheckNow = async (id: number) => {
  checkingId.value = id
  try {
    await MonitorService.CheckNow(id)
    await loadDomains()
    showMessage(t('monitor.checkSuccess'), 'success')
  } catch (e) {
    showMessage(t('monitor.checkFailed') + ' ' + e, 'error')
  } finally {
    checkingId.value = null
  }
}

const getStatusColor = (status: string): 'success' | 'error' | 'warning' | 'info' => {
  switch (status) {
    case 'ok':
      return 'success'
    case 'warning':
      return 'warning'
    case 'error':
      return 'error'
    case 'expired':
      return 'error'
    default:
      return 'info'
  }
}

const getStatusBadge = (status: string): 'success' | 'error' | 'warning' | 'info' => {
  switch (status) {
    case 'ok':
      return 'success'
    case 'warning':
      return 'warning'
    case 'error':
      return 'error'
    case 'expired':
      return 'error'
    default:
      return 'info'
  }
}

const getStatusLabel = (status: string) => {
  switch (status) {
    case 'ok':
      return t('monitor.statusOk')
    case 'warning':
      return t('monitor.statusWarning')
    case 'error':
      return t('monitor.statusError')
    default:
      return t('monitor.statusUnknown')
  }
}

const formatRemainingDays = (days: number) => {
  if (days <= 0) return t('monitor.expired')
  if (days <= 30) return `${days} ${t('common.daysLeft')} ⚠️`
  return `${days} ${t('common.daysLeft')}`
}

const truncateFingerprint = (fp: string) => {
  if (!fp) return ''
  return fp.length > 32 ? fp.substring(0, 32) + '...' : fp
}

const formatResponseTime = (ms: number) => {
  if (ms <= 0) return '—'
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(1)}s`
}

const expandedId = ref<number | null>(null)
const toggleExpand = (id: number) => {
  expandedId.value = expandedId.value === id ? null : id
}

// ===== 批量选择 =====
const selectedIds = ref<number[]>([])
const isSelected = (id: number) => selectedIds.value.includes(id)
const isAllSelected = computed(
  () =>
    filteredDomains.value.length > 0 &&
    filteredDomains.value.every((d) => selectedIds.value.includes(d.id)),
)
const isIndeterminate = computed(() => {
  const n = filteredDomains.value.filter((d) => selectedIds.value.includes(d.id)).length
  return n > 0 && n < filteredDomains.value.length
})
const toggleSelect = (id: number) => {
  const i = selectedIds.value.indexOf(id)
  if (i >= 0) selectedIds.value.splice(i, 1)
  else selectedIds.value.push(id)
}
const toggleSelectAll = () => {
  selectedIds.value = isAllSelected.value ? [] : filteredDomains.value.map((d) => d.id)
}
const clearSelection = () => {
  selectedIds.value = []
}

// ===== 批量操作 =====
const batchBusy = ref(false)
const showBatchDeleteModal = ref(false)

const runBatch = async (fn: (id: number) => Promise<any>, successMsg: string) => {
  if (selectedIds.value.length === 0) return
  batchBusy.value = true
  try {
    await Promise.all(selectedIds.value.map(fn))
    await loadDomains()
    showMessage(successMsg, 'success')
  } catch (e) {
    showMessage(t('monitor.batchFailed') + ' ' + String(e), 'error')
  } finally {
    batchBusy.value = false
    clearSelection()
  }
}
const batchEnable = (enabled: boolean) =>
  runBatch(
    (id) => MonitorService.SetActive(id, enabled),
    enabled
      ? t('monitor.batchEnabled', { count: selectedIds.value.length })
      : t('monitor.batchDisabled', { count: selectedIds.value.length }),
  )
const batchRefresh = () =>
  runBatch(
    (id) => MonitorService.CheckNow(id),
    t('monitor.batchRefreshed', { count: selectedIds.value.length }),
  )
const batchDelete = async () => {
  if (selectedIds.value.length === 0) return
  batchBusy.value = true
  try {
    await Promise.all(selectedIds.value.map((id) => MonitorService.Delete(id)))
    await loadDomains()
    showMessage(t('monitor.batchDeleted', { count: selectedIds.value.length }), 'success')
    showBatchDeleteModal.value = false
  } catch (e) {
    showMessage(t('monitor.batchFailed') + ' ' + String(e), 'error')
  } finally {
    batchBusy.value = false
    clearSelection()
  }
}

const searchQuery = ref('')
const statusFilter = ref('all')

const statusOptions = computed(() => [
  { label: t('monitor.allStatus'), value: 'all' },
  { label: t('monitor.statusOk'), value: 'ok' },
  { label: t('monitor.statusWarning'), value: 'warning' },
  { label: t('monitor.statusError'), value: 'error' },
  { label: t('monitor.statusExpired'), value: 'expired' },
])

const filteredDomains = computed(() => {
  let list = domains.value

  if (searchQuery.value.trim()) {
    const q = searchQuery.value.trim().toLowerCase()
    list = list.filter((item) => item.domain.toLowerCase().includes(q))
  }

  if (statusFilter.value !== 'all') {
    list = list.filter((item) => item.status === statusFilter.value)
  }

  return list
})

const totalCount = computed(() => domains.value.length)
const okCount = computed(() => domains.value.filter((d) => d.status === 'ok').length)
const warnCount = computed(
  () => domains.value.filter((d) => d.status === 'warning' || d.status === 'expired').length,
)
const errorCount = computed(() => domains.value.filter((d) => d.status === 'error').length)

const checkTypeOptions = [
  { label: 'HTTPS 健康检查', value: 'https' },
  { label: 'HTTP 健康检查', value: 'http' },
]

const togglingId = ref<number | null>(null)
const handleToggleEnabled = async (item: any, value: boolean) => {
  const prev = item.enabled
  togglingId.value = item.id
  item.enabled = value
  try {
    await MonitorService.SetActive(item.id, value)
    showMessage(value ? t('common.enabledSuccess') : t('common.disabledSuccess'), 'success')
  } catch (e: any) {
    item.enabled = prev
    showMessage(t('common.toggleFailed') + ' ' + (e?.message || String(e)), 'error')
  } finally {
    togglingId.value = null
  }
}
</script>

<template>
  <div class="page">
    <!-- 页面标题 -->
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold">{{ t('monitor.title') }}</h1>
        <p class="text-sm mt-1 opacity-60">{{ t('monitor.subtitle') }}</p>
      </div>
      <n-button type="primary" @click="openCreate">
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
        {{ t('monitor.add') }}
      </n-button>
    </div>

    <!-- 概览统计 -->
    <div v-if="domains.length > 0" class="grid grid-cols-4 gap-4">
      <n-card size="small" hoverable>
        <n-statistic :label="t('monitor.total')" :value="totalCount" />
      </n-card>
      <n-card size="small" hoverable>
        <n-statistic :label="t('monitor.statusOk')" :value="okCount" />
      </n-card>
      <n-card size="small" hoverable>
        <n-statistic :label="t('monitor.statusWarning')" :value="warnCount" />
      </n-card>
      <n-card size="small" hoverable>
        <n-statistic :label="t('monitor.statusError')" :value="errorCount" />
      </n-card>
    </div>

    <!-- 监控列表 -->
    <n-card size="small">
      <n-spin :show="isLoading">
        <n-empty v-if="!isLoading && domains.length === 0" :description="t('monitor.noRecords')">
          <template #extra>
            <p class="text-sm opacity-50">{{ t('monitor.noRecordsDesc') }}</p>
          </template>
        </n-empty>

        <template v-else>
          <!-- 搜索和状态筛选 -->
          <div class="flex items-center gap-3 mb-3">
            <n-input
              v-model:value="searchQuery"
              :placeholder="t('monitor.searchPlaceholder')"
              size="small"
              clearable
              class="flex-1"
            />
            <n-select
              v-model:value="statusFilter"
              :options="statusOptions"
              size="small"
              style="width: 130px"
            />
          </div>

          <!-- 批量操作栏 -->
          <div class="flex items-center gap-3 mb-3">
            <n-checkbox
              :checked="isAllSelected"
              :indeterminate="isIndeterminate"
              @update:checked="toggleSelectAll"
            >
              {{ t('monitor.selectAll') }}
            </n-checkbox>
            <span v-if="selectedIds.length > 0" class="text-sm opacity-60">
              {{ t('monitor.selectedCount', { count: selectedIds.length }) }}
            </span>
            <div class="flex-1"></div>
            <template v-if="selectedIds.length > 0">
              <n-button
                size="small"
                type="primary"
                secondary
                :disabled="batchBusy"
                @click="batchEnable(true)"
              >
                {{ t('monitor.batchEnable') }}
              </n-button>
              <n-button size="small" secondary :disabled="batchBusy" @click="batchEnable(false)">
                {{ t('monitor.batchDisable') }}
              </n-button>
              <n-button
                size="small"
                secondary
                :loading="batchBusy"
                :disabled="batchBusy"
                @click="batchRefresh"
              >
                {{ t('monitor.batchRefresh') }}
              </n-button>
              <n-button
                size="small"
                type="error"
                secondary
                :disabled="batchBusy"
                @click="showBatchDeleteModal = true"
              >
                {{ t('monitor.batchDelete') }}
              </n-button>
            </template>
          </div>

          <n-empty v-if="filteredDomains.length === 0" :description="t('monitor.noMatch')" />

          <div v-else class="space-y-3">
            <div
              v-for="item in filteredDomains"
              :key="item.id"
              class="rounded-xl border border-neutral-200 dark:border-neutral-700 p-4 hover:border-blue-300 dark:hover:border-blue-700 transition-colors cursor-pointer"
              @click="toggleExpand(item.id)"
            >
              <div class="flex items-center justify-between">
                <div class="flex items-center gap-3">
                  <span @click.stop>
                    <n-checkbox
                      size="small"
                      :checked="isSelected(item.id)"
                      @update:checked="toggleSelect(item.id)"
                    />
                  </span>
                  <div
                    class="w-10 h-10 rounded-lg flex items-center justify-center"
                    :class="
                      item.status === 'ok'
                        ? 'bg-green-50 dark:bg-green-900/30'
                        : item.status === 'warning'
                          ? 'bg-yellow-50 dark:bg-yellow-900/30'
                          : item.status === 'error' || item.status === 'expired'
                            ? 'bg-red-50 dark:bg-red-900/30'
                            : 'bg-neutral-100 dark:bg-neutral-800'
                    "
                  >
                    <svg
                      class="w-5 h-5"
                      :class="
                        getStatusColor(item.status) === 'success'
                          ? 'text-green-500'
                          : getStatusColor(item.status) === 'warning'
                            ? 'text-yellow-500'
                            : getStatusColor(item.status) === 'error'
                              ? 'text-red-500'
                              : 'text-neutral-500'
                      "
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
                    <div class="flex items-center gap-2">
                      <p class="font-medium">{{ item.domain }}</p>
                      <n-tag :type="getStatusBadge(item.status)" size="small" :bordered="false">{{
                        getStatusLabel(item.status)
                      }}</n-tag>
                    </div>
                    <div class="flex items-center gap-4 text-xs mt-1 opacity-50">
                      <span>{{ item.check_type === 'https' ? 'HTTPS' : 'HTTP' }}</span>
                      <span v-if="item.check_interval">{{ item.check_interval }}s</span>
                      <span v-if="item.last_check_at"
                        >{{ t('monitor.lastCheck') }}: {{ item.last_check_at }}</span
                      >
                    </div>
                  </div>
                </div>
                <div class="flex items-center gap-2" @click.stop>
                  <n-switch
                    :value="item.enabled"
                    :loading="togglingId === item.id"
                    size="small"
                    @update:value="(v: boolean) => handleToggleEnabled(item, v)"
                    :title="item.enabled ? t('monitor.disable') : t('monitor.enable')"
                  />
                  <n-button
                    quaternary
                    circle
                    size="small"
                    @click.stop="handleCheckNow(item.id)"
                    :disabled="checkingId === item.id"
                    :title="t('monitor.checkNow')"
                  >
                    <template #icon>
                      <svg
                        v-if="checkingId === item.id"
                        class="w-4 h-4 animate-spin"
                        fill="none"
                        viewBox="0 0 24 24"
                      >
                        <circle
                          class="opacity-25"
                          cx="12"
                          cy="12"
                          r="10"
                          stroke="currentColor"
                          stroke-width="4"
                        ></circle>
                        <path
                          class="opacity-75"
                          fill="currentColor"
                          d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"
                        ></path>
                      </svg>
                      <svg
                        v-else
                        class="w-4 h-4"
                        fill="none"
                        stroke="currentColor"
                        viewBox="0 0 24 24"
                      >
                        <path
                          stroke-linecap="round"
                          stroke-linejoin="round"
                          stroke-width="2"
                          d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"
                        />
                      </svg>
                    </template>
                  </n-button>
                  <n-button
                    quaternary
                    circle
                    size="small"
                    @click.stop="openEdit(item)"
                    :title="t('dns.editTitle')"
                  >
                    <template #icon>
                      <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path
                          stroke-linecap="round"
                          stroke-linejoin="round"
                          stroke-width="2"
                          d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z"
                        />
                      </svg>
                    </template>
                  </n-button>
                  <n-button
                    quaternary
                    circle
                    size="small"
                    type="error"
                    @click.stop="openDeleteModal(item.id)"
                    :title="t('dns.deleteTitle')"
                  >
                    <template #icon>
                      <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path
                          stroke-linecap="round"
                          stroke-linejoin="round"
                          stroke-width="2"
                          d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"
                        />
                      </svg>
                    </template>
                  </n-button>
                </div>
              </div>

              <!-- HTTPS 详情展开区 -->
              <div
                v-if="expandedId === item.id && item.check_type === 'https' && item.cert_issuer"
                class="mt-3 pt-3 border-t border-neutral-200 dark:border-neutral-700 grid grid-cols-2 md:grid-cols-4 gap-3 text-xs"
              >
                <div>
                  <p class="opacity-50">{{ t('monitor.issuer') }}</p>
                  <p class="font-medium truncate">{{ item.cert_issuer }}</p>
                </div>
                <div>
                  <p class="opacity-50">{{ t('monitor.remainingDays') }}</p>
                  <p
                    :class="
                      item.cert_remaining_days <= 30 ? 'text-yellow-500 font-medium' : 'font-medium'
                    "
                  >
                    {{ formatRemainingDays(item.cert_remaining_days) }}
                  </p>
                </div>
                <div>
                  <p class="opacity-50">{{ t('cert.detail.signatureAlgo') }}</p>
                  <p class="font-medium font-mono text-[11px]">
                    {{ item.cert_signature_algo || '—' }}
                  </p>
                </div>
                <div>
                  <p class="opacity-50">{{ t('cert.detail.publicKeyAlgo') }}</p>
                  <p class="font-medium font-mono text-[11px]">
                    {{ item.cert_public_key_algo }} {{ item.cert_public_key_bits }}bit
                  </p>
                </div>
                <div class="col-span-2">
                  <p class="opacity-50">{{ t('monitor.fingerprint') }}</p>
                  <p class="font-mono truncate text-[11px]" :title="item.cert_fingerprint">
                    {{ truncateFingerprint(item.cert_fingerprint) }}
                  </p>
                </div>
                <div>
                  <p class="opacity-50">{{ t('monitor.responseTime') }}</p>
                  <p class="font-medium">{{ formatResponseTime(item.response_time_ms) }}</p>
                </div>
                <div>
                  <p class="opacity-50">{{ t('monitor.statusCode') }}</p>
                  <p class="font-mono font-medium">{{ item.http_status_code }}</p>
                </div>
              </div>

              <!-- HTTP 详情 -->
              <div
                v-if="
                  expandedId === item.id && item.check_type === 'http' && item.http_status_code > 0
                "
                class="mt-3 pt-3 border-t border-neutral-200 dark:border-neutral-700 grid grid-cols-3 gap-3 text-xs"
              >
                <div>
                  <p class="opacity-50">{{ t('monitor.statusCode') }}</p>
                  <p class="font-mono font-medium">{{ item.http_status_code }}</p>
                </div>
                <div>
                  <p class="opacity-50">{{ t('monitor.responseTime') }}</p>
                  <p class="font-medium">{{ formatResponseTime(item.response_time_ms) }}</p>
                </div>
                <div>
                  <p class="opacity-50">{{ t('monitor.lastCheck') }}</p>
                  <p class="font-medium">{{ item.last_check_at || '—' }}</p>
                </div>
              </div>

              <!-- 错误信息 -->
              <div
                v-if="expandedId === item.id && item.last_check_error"
                class="mt-3 pt-3 border-t border-neutral-200 dark:border-neutral-700"
              >
                <p class="text-red-500 text-xs">
                  {{ t('monitor.error') }}: {{ item.last_check_error }}
                </p>
              </div>
              <MonitorTrendChart v-if="expandedId === item.id" :domain-id="item.id" @click.stop />
            </div>
          </div>
        </template>
      </n-spin>
    </n-card>

    <!-- 添加弹窗 -->
    <n-modal
      v-model:show="showAddModal"
      preset="card"
      :title="t('monitor.addDomain')"
      style="max-width: 480px"
    >
      <n-form label-placement="top">
        <n-form-item :label="t('monitor.domain') + ' *'">
          <n-input v-model:value="formData.domain" :placeholder="t('monitor.domainPlaceholder')" />
        </n-form-item>
        <n-form-item :label="t('monitor.checkType')">
          <n-select v-model:value="formData.check_type" :options="checkTypeOptions" />
        </n-form-item>
        <n-form-item
          v-if="formData.check_type === 'ssl'"
          :label="t('monitor.domain') + ' ' + t('monitor.statusCode')"
        >
          <n-input-number v-model:value="formData.port" :min="1" :max="65535" style="width: 100%" />
        </n-form-item>
        <n-form-item v-if="formData.check_type === 'http'" label="URL">
          <n-input v-model:value="formData.url" :placeholder="t('monitor.urlPlaceholder')" />
        </n-form-item>
        <n-form-item :label="t('monitor.checkInterval') + ' (' + t('monitor.intervalHint') + ')'">
          <n-input-number v-model:value="formData.check_interval" :min="60" style="width: 100%" />
        </n-form-item>
      </n-form>
      <template #footer>
        <div class="flex justify-end gap-2">
          <n-button @click="showAddModal = false">{{ t('common.cancel') }}</n-button>
          <n-button type="primary" @click="handleSave" :disabled="!formData.domain">{{
            t('common.confirm')
          }}</n-button>
        </div>
      </template>
    </n-modal>

    <!-- 编辑弹窗 -->
    <n-modal
      v-model:show="showEditModal"
      preset="card"
      :title="t('monitor.editDomain')"
      style="max-width: 480px"
    >
      <n-form label-placement="top">
        <n-form-item :label="t('monitor.domain') + ' *'">
          <n-input v-model:value="formData.domain" />
        </n-form-item>
        <n-form-item :label="t('monitor.checkType')">
          <n-select v-model:value="formData.check_type" :options="checkTypeOptions" />
        </n-form-item>
        <n-form-item v-if="formData.check_type === 'ssl'" label="Port">
          <n-input-number v-model:value="formData.port" :min="1" :max="65535" style="width: 100%" />
        </n-form-item>
        <n-form-item v-if="formData.check_type === 'http'" label="URL">
          <n-input v-model:value="formData.url" :placeholder="t('monitor.urlPlaceholder')" />
        </n-form-item>
        <n-form-item :label="t('monitor.checkInterval') + ' (' + t('monitor.intervalHint') + ')'">
          <n-input-number v-model:value="formData.check_interval" :min="60" style="width: 100%" />
        </n-form-item>
      </n-form>
      <template #footer>
        <div class="flex justify-end gap-2">
          <n-button @click="showEditModal = false">{{ t('common.cancel') }}</n-button>
          <n-button type="primary" @click="handleSave" :disabled="!formData.domain">{{
            t('common.confirm')
          }}</n-button>
        </div>
      </template>
    </n-modal>

    <!-- 删除确认弹窗 -->
    <n-modal v-model:show="showDeleteModal" preset="dialog" :title="t('dns.deleteTitle')">
      <p>{{ t('monitor.deleteConfirm') }}</p>
      <template #action>
        <n-button @click="showDeleteModal = false">{{ t('common.cancel') }}</n-button>
        <n-button type="error" @click="handleDelete">{{ t('common.confirm') }}</n-button>
      </template>
    </n-modal>

    <!-- 批量删除确认弹窗 -->
    <n-modal v-model:show="showBatchDeleteModal" preset="dialog" :title="t('monitor.batchDelete')">
      <p>{{ t('monitor.batchDeleteConfirm', { count: selectedIds.length }) }}</p>
      <p class="text-xs opacity-60 mt-1">{{ t('monitor.batchDeleteConfirmDesc') }}</p>
      <template #action>
        <n-button @click="showBatchDeleteModal = false">{{ t('common.cancel') }}</n-button>
        <n-button type="error" :loading="batchBusy" @click="batchDelete">{{
          t('common.confirm')
        }}</n-button>
      </template>
    </n-modal>
  </div>
</template>
