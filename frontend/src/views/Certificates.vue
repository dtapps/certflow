<script setup lang="ts">
import { ref, onMounted, computed, h } from 'vue'
import { useRouter } from 'vue-router'
import {
  NInput,
  NSelect,
  NButton,
  NDataTable,
  NTag,
  NSwitch,
  NModal,
  NSpin,
  NEmpty,
  useMessage,
} from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import * as CertificateService from '@bindings/cnb.cool/dtapp/certflow/certificateservicewrapper'
import type { CertificateListItem } from '@bindings/cnb.cool/dtapp/certflow/models'
import { useI18nStore } from '../stores/i18n'
import { getStatusBadge, getDaysLeft, getDaysLeftClass } from '../utils/certificate'
import { initMessage, showMessage } from '../utils/message'

const router = useRouter()
const i18nStore = useI18nStore()
const { t } = i18nStore
const message = useMessage()
initMessage(message)

const searchQuery = ref('')
const statusFilter = ref<string | null>('all')
const certificates = ref<CertificateListItem[]>([])
const isLoading = ref(false)
const switchingId = ref<number | null>(null)

const statusOptions = [
  { label: t('certs.allStatus'), value: 'all' },
  { label: t('certs.active'), value: 'active' },
  { label: t('certs.pending'), value: 'pending' },
  { label: t('certs.expired'), value: 'expired' },
  { label: t('certs.revoked'), value: 'revoked' },
  { label: t('certs.failed'), value: 'failed' },
]

onMounted(async () => {
  isLoading.value = true
  try {
    certificates.value = (await CertificateService.ListCertificates()) ?? []
  } catch (e) {
    console.error(t('certs.loadFailed'), e)
  } finally {
    isLoading.value = false
  }
})

const filteredCertificates = computed(() => {
  let result = certificates.value

  if (searchQuery.value) {
    const query = searchQuery.value.toLowerCase()
    result = result.filter(
      (c) => c.domain.toLowerCase().includes(query) || c.issuer.toLowerCase().includes(query),
    )
  }

  if (statusFilter.value && statusFilter.value !== 'all') {
    result = result.filter((c) => c.status === statusFilter.value)
  }

  return result
})

const showDeleteModal = ref(false)
const deleteTargetId = ref<number | null>(null)

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
    await CertificateService.DeleteCertificate(id)
    certificates.value = certificates.value.filter((c) => c.id !== id)
    showMessage(t('certs.deleteSuccess'), 'success')
  } catch (e) {
    showMessage(t('certs.deleteFailed') + ' ' + e, 'error')
  }
}

const handleRetry = (cert: CertificateListItem) => {
  if (cert.status === 'pending') {
    router.push('/certificates/apply?certId=' + cert.id)
  } else if (cert.status === 'failed') {
    router.push('/certificates/apply?domain=' + encodeURIComponent(cert.domain))
  }
}

const toggleAutoRenew = async (cert: CertificateListItem, val: boolean) => {
  if (switchingId.value !== null) return
  switchingId.value = cert.id
  const prev = cert.auto_renew
  cert.auto_renew = val
  try {
    await CertificateService.UpdateCertificateSettings(cert.id, val, cert.renewal_days)
    showMessage(val ? t('certs.autoRenewOn') : t('certs.autoRenewOff'), 'success')
  } catch (e) {
    cert.auto_renew = prev
    showMessage(t('certs.updateFailed') + ' ' + e, 'error')
  } finally {
    switchingId.value = null
  }
}

const columns: DataTableColumns<CertificateListItem> = [
  {
    title: t('certs.domain'),
    key: 'domain',
    render(row) {
      return h('div', { class: 'flex items-center gap-3' }, [
        h(
          'div',
          {
            class:
              'w-8 h-8 rounded-lg bg-blue-50 dark:bg-blue-900/30 flex items-center justify-center',
          },
          [
            h(
              'svg',
              {
                class: 'w-4 h-4 text-blue-500',
                fill: 'none',
                stroke: 'currentColor',
                viewBox: '0 0 24 24',
              },
              [
                h('path', {
                  'stroke-linecap': 'round',
                  'stroke-linejoin': 'round',
                  'stroke-width': '2',
                  d: 'M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z',
                }),
              ],
            ),
          ],
        ),
        h('div', [
          h('p', { class: 'font-medium' }, row.domain),
          row.sans?.length
            ? h('p', { class: 'text-xs opacity-50' }, `+${row.sans.length} ${t('cert.san')}`)
            : null,
        ]),
      ])
    },
  },
  {
    title: t('certs.issuer'),
    key: 'issuer',
  },
  {
    title: t('cert.caName'),
    key: 'ca_name',
    render(row) {
      return row.ca_name ? h('span', row.ca_name) : h('span', { class: 'opacity-50' }, '—')
    },
  },
  {
    title: t('certs.status'),
    key: 'status',
    render(row) {
      const badge = getStatusBadge(row.status)
      return h(
        NTag,
        {
          type: badge.type as any,
          size: 'small',
          bordered: false,
        },
        { default: () => badge.text },
      )
    },
  },
  {
    title: t('certs.daysLeft'),
    key: 'daysLeft',
    render(row) {
      const days = getDaysLeft(row.not_after, row.status)
      if (days === null) return h('span', { class: 'opacity-50' }, '—')
      return h('span', { class: `font-medium ${getDaysLeftClass(days)}` }, String(days))
    },
  },
  {
    title: t('certs.actions'),
    key: 'actions',
    align: 'right',
    render(row) {
      return h('div', { class: 'flex items-center justify-end gap-2' }, [
        h('div', { onClick: (e: MouseEvent) => e.stopPropagation() }, [
          h(NSwitch, {
            value: row.auto_renew,
            size: 'small',
            loading: switchingId.value === row.id,
            'onUpdate:value': (val: boolean) => toggleAutoRenew(row, val),
          }),
        ]),
        row.status === 'pending'
          ? h(
              NButton,
              {
                size: 'tiny',
                secondary: true,
                onClick: (e) => {
                  e.stopPropagation()
                  handleRetry(row)
                },
              },
              { default: () => t('certs.continueApply') },
            )
          : null,
        row.status === 'failed'
          ? h(
              NButton,
              {
                size: 'tiny',
                type: 'error',
                secondary: true,
                onClick: (e) => {
                  e.stopPropagation()
                  handleRetry(row)
                },
              },
              { default: () => t('certs.retryApply') },
            )
          : null,
        h(
          NButton,
          {
            size: 'tiny',
            type: 'error',
            quaternary: true,
            onClick: (e) => {
              e.stopPropagation()
              openDeleteModal(row.id)
            },
          },
          {
            icon: () =>
              h(
                'svg',
                { class: 'w-4 h-4', fill: 'none', stroke: 'currentColor', viewBox: '0 0 24 24' },
                [
                  h('path', {
                    'stroke-linecap': 'round',
                    'stroke-linejoin': 'round',
                    'stroke-width': '2',
                    d: 'M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16',
                  }),
                ],
              ),
          },
        ),
      ])
    },
  },
]
</script>

<template>
  <div class="page">
    <!-- 页面标题 -->
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold">{{ t('certs.title') }}</h1>
        <p class="text-sm mt-1 opacity-60">{{ t('certs.subtitle') }}</p>
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
        {{ t('certs.apply') }}
      </n-button>
    </div>

    <!-- 搜索和筛选 -->
    <div class="flex flex-col sm:flex-row gap-4">
      <n-input
        v-model:value="searchQuery"
        :placeholder="t('certs.search')"
        clearable
        style="flex: 1"
      >
        <template #prefix>
          <svg class="w-4 h-4 opacity-50" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"
            />
          </svg>
        </template>
      </n-input>
      <n-select v-model:value="statusFilter" :options="statusOptions" style="width: 150px" />
    </div>

    <!-- 证书列表 -->
    <n-card size="small">
      <n-spin :show="isLoading">
        <n-empty
          v-if="!isLoading && filteredCertificates.length === 0"
          :description="t('certs.noRecords')"
        />
        <n-data-table
          v-else
          :columns="columns"
          :data="filteredCertificates"
          :bordered="false"
          :single-line="false"
          :row-props="
            (row: CertificateListItem) => ({
              style: 'cursor: pointer;',
              onClick: () => router.push('/certificates/' + row.id),
            })
          "
        />
      </n-spin>
    </n-card>

    <!-- 删除确认弹窗 -->
    <n-modal v-model:show="showDeleteModal" preset="dialog" :title="t('certs.delete')">
      <p>{{ t('certs.deleteConfirm') }}</p>
      <template #action>
        <n-button @click="showDeleteModal = false">{{ t('common.cancel') }}</n-button>
        <n-button type="error" @click="handleDelete">{{ t('common.confirm') }}</n-button>
      </template>
    </n-modal>
  </div>
</template>
