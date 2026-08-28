<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed, h } from 'vue'
import { useRouter } from 'vue-router'
import { Events } from '@wailsio/runtime'
import {
  NCard,
  NButton,
  NInput,
  NDataTable,
  NTag,
  NTooltip,
  NSpin,
  NEmpty,
  NPopconfirm,
  type DataTableColumns,
} from 'naive-ui'
import * as DeployService from '@bindings/cnb.cool/dtapp/certflow/deployservicewrapper'
import * as WindowService from '@bindings/cnb.cool/dtapp/certflow/windowservicewrapper'
import type { DeployTargetListItem } from '@bindings/cnb.cool/dtapp/certflow/models'
import { useI18nStore } from '../stores/i18n'
import { showMessage, translateBackend } from '../utils/message'
import { regionOf, regionLabel } from '../utils/region'
import { EventWindowResized, type WindowResizedPayload } from '../utils/events'
import ProviderIcon from '../components/ProviderIcon.vue'
import { isPanelProvider, providerLabel, serviceLabel } from '../utils/deploy'
import { formatDateTime } from '../utils/format'

const router = useRouter()
const i18nStore = useI18nStore()
const { t } = i18nStore

const loading = ref(false)
const targets = ref<DeployTargetListItem[]>([])
const search = ref('')

function regionName(target: DeployTargetListItem): string {
  const r = regionLabel(target.provider_type, target.deploy_service, regionOf(target.config))
  console.debug(
    t('log.deployTargetsRegionName', {
      name: target.name,
      regionRaw: JSON.stringify(regionOf(target.config)),
      result: JSON.stringify(r),
    }),
  )
  return r
}
function credName(target: DeployTargetListItem): string {
  const c =
    target.credential_source === 'dns_provider'
      ? target.dns_provider_name || ''
      : target.deploy_credential_name || ''
  console.debug(
    t('log.deployTargetsCredName', {
      name: target.name,
      source: String(target.credential_source),
      cred: JSON.stringify(c),
    }),
  )
  return c
}
function statusText(s?: string) {
  if (!s) return t('deploy.status.never')
  if (s === 'success') return t('deploy.status.success')
  if (s === 'failed') return t('deploy.status.failed')
  return s
}
function statusType(s?: string) {
  if (s === 'success') return 'success'
  if (s === 'failed') return 'error'
  return 'default'
}
function domainList(target: DeployTargetListItem): string[] {
  const cfg = target.config
  if (!cfg) return []
  // 面板/防火墙类：网站名称存在 config.site_name
  if (isPanelProvider(target.provider_type)) {
    return cfg.site_name || []
  }
  return cfg.domains || []
}

const filteredTargets = computed(() => {
  const kw = search.value.trim().toLowerCase()
  if (!kw) return targets.value
  return targets.value.filter(
    (t) =>
      t.name.toLowerCase().includes(kw) ||
      providerLabel(t.provider_type).toLowerCase().includes(kw) ||
      serviceLabel(t.deploy_service).toLowerCase().includes(kw) ||
      credName(t).toLowerCase().includes(kw),
  )
})

const winWidth = ref(1280)
// 窗口尺寸由 Go 端 window.Size() 获取（见 window_service.go GetWindowSize），
// 并订阅 Go 广播的 window_resized 事件实时更新。
function applyWindowWidth(size: { width: number; height: number }) {
  winWidth.value = size.width
}

const allColumns = computed<DataTableColumns<DeployTargetListItem>>(() => [
  {
    title: t('deploy.name'),
    key: 'name',
    minWidth: 150,
    render: (row: DeployTargetListItem) =>
      h('div', { class: 'flex items-center gap-2' }, [
        h(ProviderIcon, {
          providerType: row.provider_type,
          name: providerLabel(row.provider_type),
          size: 18,
        }),
        h('span', { class: 'font-medium' }, row.name),
      ]),
  },
  {
    title: t('deploy.service'),
    key: 'deploy_service',
    minWidth: 140,
    render: (row: DeployTargetListItem) =>
      h(
        'div',
        {
          class:
            winWidth.value >= 1440
              ? 'flex items-center gap-1.5 flex-wrap'
              : 'flex flex-col items-start gap-1',
        },
        [
          h(
            NTag,
            { size: 'small', type: 'info', bordered: false },
            { default: () => serviceLabel(row.deploy_service, row.provider_type) },
          ),
          h(
            NTag,
            { size: 'small', type: 'default', bordered: false },
            { default: () => providerLabel(row.provider_type) },
          ),
        ],
      ),
  },
  {
    title: t('deploy.domainsOrSites'),
    key: 'domains',
    minWidth: 160,
    render: (row: DeployTargetListItem) => {
      const doms = domainList(row)
      if (!doms.length) return h('span', { class: 'opacity-40' }, '-')
      const shown = doms.slice(0, 2)
      const rest = doms.length - shown.length
      return h(
        'div',
        { class: 'flex flex-col gap-0.5' },
        [
          ...shown.map((d) => h('span', { class: 'text-xs leading-tight' }, d)),
          rest > 0 ? h('span', { class: 'text-xs opacity-50' }, `+${rest}`) : null,
        ].filter(Boolean) as any,
      )
    },
  },
  {
    title: t('deploy.region'),
    key: 'region',
    width: 110,
    render: (row: DeployTargetListItem) => regionName(row) || '-',
  },
  {
    title: t('deploy.credential'),
    key: 'credential',
    minWidth: 140,
    render: (row: DeployTargetListItem) => {
      const parts: any[] = [
        h(
          NTag,
          { size: 'small', bordered: false },
          {
            default: () =>
              row.credential_source === 'dns_provider'
                ? t('deploy.credFromDns')
                : t('deploy.credFromCredential'),
          },
        ),
      ]
      const cn = credName(row)
      if (cn) {
        parts.push(h(NTag, { size: 'small', bordered: false }, { default: () => cn }))
      }
      return h(
        'div',
        {
          class:
            winWidth.value >= 1440
              ? 'flex items-center gap-1.5 flex-wrap'
              : 'flex flex-col items-start gap-1',
        },
        parts,
      )
    },
  },
  {
    title: t('deploy.status'),
    key: 'last_status',
    width: 130,
    render: (row: DeployTargetListItem) => {
      const statusTag = h(
        NTag,
        { size: 'small', bordered: false, type: statusType(row.last_status) as any },
        { default: () => statusText(row.last_status) },
      )
      const activeTag = h(
        NTag,
        {
          size: 'small',
          bordered: false,
          type: row.is_active ? 'success' : 'default',
        },
        { default: () => (row.is_active ? t('deploy.enabled') : t('deploy.disabled')) },
      )
      const node = h('div', { class: 'flex items-center gap-1.5 flex-wrap' }, [
        statusTag,
        activeTag,
      ])
      if (row.last_status === 'failed' && row.last_error) {
        return h(NTooltip, {}, { trigger: () => node, default: () => row.last_error })
      }
      return node
    },
  },
  {
    title: t('deploy.time'),
    key: 'last_deployed_at',
    width: 160,
    render: (row: DeployTargetListItem) => {
      console.debug(
        t('log.deployTargetsLastDeployedAt', {
          name: row.name,
          raw: JSON.stringify(row.last_deployed_at),
        }),
      )
      return row.last_deployed_at
        ? h('span', { class: 'text-xs opacity-50' }, formatDateTime(row.last_deployed_at))
        : '-'
    },
  },
  {
    title: t('common.actions'),
    key: 'actions',
    align: 'right',
    width: 200,
    render: (row: DeployTargetListItem) =>
      h('div', { class: 'flex items-center justify-end gap-2' }, [
        h(
          NButton,
          {
            size: 'tiny',
            type: 'primary',
            secondary: true,
            onClick: (e: MouseEvent) => {
              e.stopPropagation()
              goDeploy(row)
            },
          },
          { default: () => t('deploy.deploy') },
        ),
        h(
          NButton,
          {
            size: 'tiny',
            secondary: true,
            onClick: (e: MouseEvent) => {
              e.stopPropagation()
              goEdit(row)
            },
          },
          { default: () => t('deploy.edit') },
        ),
        h(
          NPopconfirm,
          { onPositiveClick: () => doRemove(row) },
          {
            trigger: () =>
              h(
                NButton,
                {
                  size: 'tiny',
                  type: 'error',
                  quaternary: true,
                  onClick: (e: MouseEvent) => e.stopPropagation(),
                },
                {
                  icon: () =>
                    h(
                      'svg',
                      {
                        class: 'w-4 h-4',
                        fill: 'none',
                        stroke: 'currentColor',
                        viewBox: '0 0 24 24',
                      },
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
            default: () => t('deploy.deleteConfirm'),
          },
        ),
      ]),
  },
])

const columns = computed<DataTableColumns<DeployTargetListItem>>(() => {
  const visible = new Set(
    winWidth.value >= 1440
      ? [
          'name',
          'deploy_service',
          'domains',
          'region',
          'credential',
          'last_status',
          'last_deployed_at',
          'actions',
        ]
      : winWidth.value >= 1080
        ? ['name', 'deploy_service', 'domains', 'credential', 'last_status', 'actions']
        : ['name', 'deploy_service', 'last_status', 'actions'],
  )
  return allColumns.value.filter((c) => visible.has((c as any).key as string))
})

async function loadAll() {
  loading.value = true
  try {
    const tlist = await DeployService.ListDeployTargets()
    targets.value = tlist || []
    console.debug(
      t('log.deployTargetsLoad', {
        count: (tlist || []).length,
        first: JSON.stringify(
          (tlist || [])[0]
            ? {
                name: (tlist || [])[0].name,
                provider: (tlist || [])[0].provider_type,
                last_deployed_at: (tlist || [])[0].last_deployed_at,
              }
            : null,
        ),
      }),
    )
  } catch (e: any) {
    showMessage(t('deploy.loadFailed') + ': ' + translateBackend(e?.message || String(e)), 'error')
  } finally {
    loading.value = false
  }
}

async function doRemove(target: DeployTargetListItem) {
  try {
    await DeployService.DeleteDeployTarget(target.id)
    await loadAll()
    showMessage(t('deploy.operationSuccess'), 'success')
  } catch (e: any) {
    showMessage(
      t('deploy.operationFailed') + ': ' + translateBackend(e?.message || String(e)),
      'error',
    )
  }
}

function goCreate() {
  router.push('/ssl-deploy/new')
}
function goEdit(target: DeployTargetListItem) {
  router.push(`/ssl-deploy/${target.id}/edit`)
}
function goDeploy(target: DeployTargetListItem) {
  router.push(`/ssl-deploy/${target.id}?tab=deploy`)
}

onMounted(async () => {
  loadAll()
  // 初始尺寸：调用 Go 端 window.Size()
  try {
    const size = await WindowService.GetWindowSize()
    winWidth.value = size.width
  } catch {
    winWidth.value = 1280
  }
  // 实时跟随：订阅 Go 端 window_resized 事件（Go 用 WindowDidResize 广播）
  Events.On(EventWindowResized, (ev: { data: WindowResizedPayload }) => {
    applyWindowWidth(ev.data)
  })
})
onUnmounted(() => {
  Events.Off(EventWindowResized)
})
</script>

<template>
  <div class="page">
    <!-- 页面标题 -->
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold">{{ t('deploy.title') }}</h1>
        <p class="text-sm mt-1 opacity-60">{{ t('deploy.subtitle') }}</p>
      </div>
      <n-button type="primary" @click="goCreate">
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
        {{ t('deploy.create') }}
      </n-button>
    </div>

    <n-card size="small">
      <n-spin :show="loading">
        <div class="flex flex-col sm:flex-row gap-4 mb-4">
          <n-input
            v-model:value="search"
            :placeholder="t('common.search')"
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
        </div>
        <n-empty v-if="!loading && filteredTargets.length === 0" :description="t('deploy.empty')" />
        <n-data-table
          v-else
          :columns="columns"
          :data="filteredTargets"
          :bordered="false"
          :single-line="false"
          :row-key="(row: DeployTargetListItem) => row.id"
          :row-props="
            (row: DeployTargetListItem) => ({
              style: 'cursor: pointer',
              onClick: () => goDeploy(row),
            })
          "
        />
      </n-spin>
    </n-card>
  </div>
</template>
