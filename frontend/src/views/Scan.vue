<script setup lang="ts">
import { ref, onMounted } from 'vue'
import {
  NCard,
  NInput,
  NInputNumber,
  NSelect,
  NButton,
  NSpin,
  NEmpty,
  NModal,
  NTag,
} from 'naive-ui'
import * as ScannerService from '@bindings/cnb.cool/dtapp/certflow/scannerservicewrapper'
import type { ScanResultItem } from '@bindings/cnb.cool/dtapp/certflow/internal/scanner/models'
import { useI18nStore } from '../stores/i18n'
import { useThemeStore } from '../stores/theme'
import { storeToRefs } from 'pinia'

const i18nStore = useI18nStore()
const { t } = i18nStore
const themeStore = useThemeStore()
const { isDark } = storeToRefs(themeStore)

const domainInput = ref('')
const portInput = ref(443)
const scanType = ref('https')
const isScanning = ref(false)
const history = ref<ScanResultItem[]>([])
const isLoadingHistory = ref(false)
const expandedId = ref<number | null>(null)
const showDeleteModal = ref(false)
const deleteTargetId = ref(0)

const handleScan = async () => {
  const domain = domainInput.value.trim()
  if (!domain) return
  isScanning.value = true
  try {
    const result = await ScannerService.Scan(domain, portInput.value, scanType.value)
    if (result) expandedId.value = result.id
    await loadHistory()
  } catch (e) {
    console.error(t('scan.scanFailed'), e)
  } finally {
    isScanning.value = false
  }
}

const loadHistory = async () => {
  isLoadingHistory.value = true
  try {
    history.value = ((await ScannerService.ListHistory()) ?? []).filter(Boolean) as ScanResultItem[]
  } catch (e) {
    console.error(t('scan.loadHistoryFailed'), e)
  } finally {
    isLoadingHistory.value = false
  }
}

const toggleExpand = (id: number) => {
  expandedId.value = expandedId.value === id ? null : id
}

const confirmDelete = (id: number) => {
  deleteTargetId.value = id
  showDeleteModal.value = true
}

const handleDelete = async () => {
  showDeleteModal.value = false
  try {
    await ScannerService.DeleteResult(deleteTargetId.value)
    if (expandedId.value === deleteTargetId.value) {
      expandedId.value = null
    }
    await loadHistory()
  } catch (e) {
    console.error(t('scan.deleteFailed'), e)
  }
}

const handleClearHistory = async () => {
  try {
    await ScannerService.ClearHistory()
    expandedId.value = null
    await loadHistory()
  } catch (e) {
    console.error(t('scan.clearHistoryFailed'), e)
  }
}

const scanTypeOptions = [
  { label: 'HTTPS', value: 'https' },
  { label: 'HTTP', value: 'http' },
]

const formatRemainingDays = (days: number) => {
  if (days <= 0) return t('monitor.expired')
  return `${days} ${t('common.daysLeft')}`
}

onMounted(loadHistory)
</script>

<template>
  <div class="page">
    <div>
      <h1 class="text-2xl font-bold">{{ t('scan.title') }}</h1>
      <p class="text-sm mt-1 opacity-50">{{ t('scan.subtitle') }}</p>
    </div>

    <!-- 扫描输入 -->
    <n-card size="small">
      <div class="flex items-center gap-3">
        <n-select
          v-model:value="scanType"
          :options="scanTypeOptions"
          style="width: 100px"
          size="small"
        />
        <n-input
          v-model:value="domainInput"
          :placeholder="t('scan.domainPlaceholder')"
          @keyup.enter="handleScan"
          class="flex-1"
        />
        <n-input-number
          v-model:value="portInput"
          :min="1"
          :max="65535"
          size="small"
          style="width: 100px"
        />
        <n-button
          type="primary"
          :loading="isScanning"
          :disabled="!domainInput.trim()"
          @click="handleScan"
        >
          {{ isScanning ? t('scan.scanning') : t('scan.scan') }}
        </n-button>
      </div>
    </n-card>

    <!-- 扫描历史 -->
    <div>
      <div class="flex items-center justify-between mb-3">
        <p class="font-medium text-sm">{{ t('scan.history') }}</p>
        <n-button
          v-if="history.length > 0"
          text
          type="error"
          size="small"
          @click="handleClearHistory"
        >
          {{ t('scan.clearHistory') }}
        </n-button>
      </div>

      <n-spin :show="isLoadingHistory">
        <div v-if="history.length > 0" class="space-y-3">
          <div
            v-for="item in history"
            :key="item.id"
            class="p-4 rounded-xl border transition-colors cursor-pointer"
            :class="
              expandedId === item.id
                ? 'border-blue-300 dark:border-blue-700 bg-blue-50/50 dark:bg-blue-900/20'
                : 'border-neutral-200 dark:border-neutral-700 hover:border-neutral-300 dark:hover:border-neutral-600'
            "
            @click="toggleExpand(item.id)"
          >
            <!-- 卡片头部 -->
            <div class="flex items-center justify-between">
              <div class="flex items-center gap-3">
                <div
                  class="w-2 h-2 rounded-full"
                  :class="
                    item.error_message
                      ? 'bg-red-500'
                      : item.cert_remaining_days <= 30
                        ? 'bg-yellow-500'
                        : 'bg-green-500'
                  "
                />
                <div>
                  <p class="font-medium font-mono text-sm">
                    {{ item.scan_type?.toUpperCase() || 'HTTPS' }}://{{ item.domain }}:{{
                      item.port || 443
                    }}
                  </p>
                  <p class="text-xs mt-0.5 opacity-50">
                    {{ item.scanned_at }}
                    <span v-if="item.response_time_ms"> · {{ item.response_time_ms }}ms</span>
                  </p>
                </div>
              </div>
              <div class="flex items-center gap-2">
                <n-tag
                  v-if="!item.error_message"
                  size="small"
                  :bordered="false"
                  :type="
                    item.cert_remaining_days <= 0
                      ? 'error'
                      : item.cert_remaining_days <= 30
                        ? 'warning'
                        : 'success'
                  "
                >
                  {{
                    item.error_message
                      ? t('scan.error')
                      : formatRemainingDays(item.cert_remaining_days)
                  }}
                </n-tag>
                <n-tag v-else size="small" type="error" :bordered="false">
                  {{ t('scan.error') }}
                </n-tag>
                <n-button text size="tiny" type="error" @click.stop="confirmDelete(item.id)">
                  {{ t('common.delete') }}
                </n-button>
              </div>
            </div>

            <!-- 展开详情 -->
            <div
              v-if="expandedId === item.id"
              class="mt-3 pt-3 border-t border-neutral-200 dark:border-neutral-700"
            >
              <!-- 错误信息 -->
              <div
                v-if="item.error_message"
                class="p-3 rounded-lg bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800"
              >
                <p class="text-red-500 text-sm">{{ item.error_message }}</p>
              </div>

              <!-- 证书详情 -->
              <div v-else class="space-y-3 text-xs">
                <div class="grid grid-cols-2 md:grid-cols-4 gap-3">
                  <div>
                    <p class="opacity-50">{{ t('scan.issuer') }}</p>
                    <p class="font-medium truncate">{{ item.cert_issuer }}</p>
                  </div>
                  <div>
                    <p class="opacity-50">{{ t('scan.subject') }}</p>
                    <p class="font-medium truncate">{{ item.cert_subject }}</p>
                  </div>
                  <div>
                    <p class="opacity-50">{{ t('scan.validFrom') }}</p>
                    <p class="font-medium">{{ item.cert_not_before }}</p>
                  </div>
                  <div>
                    <p class="opacity-50">{{ t('scan.validTo') }}</p>
                    <p
                      class="font-medium"
                      :class="item.cert_remaining_days <= 30 ? 'text-yellow-500' : ''"
                    >
                      {{ item.cert_not_after }}
                    </p>
                  </div>
                </div>
                <div class="grid grid-cols-2 md:grid-cols-4 gap-3">
                  <div>
                    <p class="opacity-50">{{ t('scan.signatureAlgo') }}</p>
                    <p class="font-medium font-mono">{{ item.cert_signature_algo || '—' }}</p>
                  </div>
                  <div>
                    <p class="opacity-50">{{ t('scan.publicKeyAlgo') }}</p>
                    <p class="font-medium font-mono">
                      {{ item.cert_public_key_algo }} {{ item.cert_public_key_bits }}bit
                    </p>
                  </div>
                  <div>
                    <p class="opacity-50">{{ t('scan.serialNumber') }}</p>
                    <p class="font-medium font-mono break-all">
                      {{ item.cert_serial_number || '—' }}
                    </p>
                  </div>
                  <div>
                    <p class="opacity-50">{{ t('scan.responseTime') }}</p>
                    <p class="font-medium">{{ item.response_time_ms }}ms</p>
                  </div>
                </div>
                <div>
                  <p class="opacity-50">{{ t('scan.fingerprint') }}</p>
                  <p class="font-mono break-all" :title="item.cert_fingerprint">
                    {{ item.cert_fingerprint || '—' }}
                  </p>
                </div>
                <div v-if="item.cert_sans?.length">
                  <p class="opacity-50">{{ t('scan.sans') }}</p>
                  <div class="flex flex-wrap gap-1 mt-1">
                    <n-tag v-for="san in item.cert_sans" :key="san" size="tiny" :bordered="false">
                      {{ san }}
                    </n-tag>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
        <n-empty v-else :description="t('scan.noHistoryDesc')" />
      </n-spin>
    </div>

    <!-- 删除确认弹窗 -->
    <n-modal v-model:show="showDeleteModal" preset="dialog" :title="t('scan.deleteConfirm')">
      <template #action>
        <n-button @click="showDeleteModal = false">{{ t('common.cancel') }}</n-button>
        <n-button type="error" @click="handleDelete">{{ t('common.confirm') }}</n-button>
      </template>
    </n-modal>
  </div>
</template>
