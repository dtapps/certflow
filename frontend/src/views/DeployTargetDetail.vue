<script setup lang="ts">
import { ref, onMounted, computed, h, watch } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import {
  NButton,
  NTag,
  NCard,
  NSpace,
  NSpin,
  NEmpty,
  NSelect,
  NDataTable,
  NCollapse,
  NCollapseItem,
  NPopconfirm,
  NTabs,
  NTabPane,
  type DataTableColumns,
} from 'naive-ui'
import * as DeployService from '@bindings/cnb.cool/dtapp/certflow/deployservicewrapper'
import * as CertificateService from '@bindings/cnb.cool/dtapp/certflow/certificateservicewrapper'
import { copyToClipboard as copyText } from '../utils/clipboard'
import type {
  DeployTargetListItem,
  DeployLogListItem,
  CertificateListItem,
  CurrentCertDTO,
} from '@bindings/cnb.cool/dtapp/certflow/models'
import { storeToRefs } from 'pinia'
import { useI18nStore } from '../stores/i18n'
import { useThemeStore } from '../stores/theme'
import { showMessage, translateBackend } from '../utils/message'
import { regionOf, regionLabel } from '../utils/region'
import ProviderIcon from '../components/ProviderIcon.vue'
import { isPanelProvider, providerLabel, serviceLabel } from '../utils/deploy'
import { formatDateTime } from '../utils/format'

const router = useRouter()
const route = useRoute()
const i18nStore = useI18nStore()
const { t } = i18nStore
const { isDark } = storeToRefs(useThemeStore())

const id = Number(route.params.id)

const loading = ref(true)
const target = ref<DeployTargetListItem | null>(null)
const logs = ref<DeployLogListItem[]>([])
const certificates = ref<CertificateListItem[]>([])

// 复制字段标记（用于显示复制成功图标）
const copiedField = ref('')
const copyToClipboard = async (text: string, field: string) => {
  const ok = await copyText(text)
  if (ok) {
    copiedField.value = field
    setTimeout(() => {
      if (copiedField.value === field) copiedField.value = ''
    }, 2000)
  }
}

const activeTab = ref<string>('info')

// 进入部署 tab 时实时拉取云端/面板当前生效证书（B 方案：本地 + 云端并排）
watch(activeTab, (tab) => {
  if (tab === 'deploy') loadCurrentCerts()
})

// ---- JSON 高亮（信息 tab 原始配置）----
function escapeHtml(str: string): string {
  return str.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}
function syntaxHighlight(json: string): string {
  const escaped = escapeHtml(json)
  return escaped.replace(
    /("(\\u[a-zA-Z0-9]{4}|\\[^u]|[^\\"])*"(\s*:)?|\b(true|false|null)\b|-?\d+(?:\.\d*)?(?:[eE][+-]?\d+)?)/g,
    (match) => {
      let cls = 'json-num'
      if (/^"/.test(match)) {
        cls = /:$/.test(match) ? 'json-key' : 'json-str'
      } else if (/true|false/.test(match)) {
        cls = 'json-bool'
      } else if (/null/.test(match)) {
        cls = 'json-null'
      }
      return `<span class="${cls}">${match}</span>`
    },
  )
}
// 后端 config 为 map[string]string，部分 value 本身又是 JSON 字符串
// （如 domains 数组被序列化成字符串），需递归展开，否则会显示成带转义引号的整行
function tryExpand(v: any): any {
  if (typeof v === 'string') {
    const s = v.trim()
    if ((s.startsWith('[') && s.endsWith(']')) || (s.startsWith('{') && s.endsWith('}'))) {
      try {
        return tryExpand(JSON.parse(s))
      } catch {
        return v
      }
    }
    return v
  }
  if (Array.isArray(v)) return v.map(tryExpand)
  if (v && typeof v === 'object') {
    const out: Record<string, any> = {}
    for (const k of Object.keys(v)) out[k] = tryExpand(v[k])
    return out
  }
  return v
}
// 原始配置从后端可能拿到对象，也可能是 JSON 字符串，统一解析为对象后递归展开再格式化
const normalizedConfig = computed(() => {
  const cfg = target.value?.config
  if (!cfg) return null
  if (typeof cfg === 'string') {
    try {
      return tryExpand(JSON.parse(cfg))
    } catch {
      return cfg
    }
  }
  return tryExpand(cfg)
})
const rawConfigText = computed(() => {
  const cfg = normalizedConfig.value
  if (cfg == null) return ''
  return typeof cfg === 'string' ? cfg : JSON.stringify(cfg, null, 2)
})
const highlightedConfig = computed(() => {
  if (!rawConfigText.value) return ''
  return syntaxHighlight(rawConfigText.value)
})

function regionName(): string {
  if (!target.value) return ''
  const r = regionLabel(
    target.value.provider_type,
    target.value.deploy_service,
    regionOf(target.value.config),
  )
  console.debug(
    t('log.deployTargetRegionName', {
      provider: String(target.value.provider_type),
      service: String(target.value.deploy_service),
      result: JSON.stringify(r),
    }),
  )
  return r
}
function siteName(): string {
  const cf = (target.value?.config || {}) as Record<string, any>
  const raw = cf.site_name || cf.zone_name || cf.domains || ''
  if (!raw) return ''
  // 面板类 site_name 为 JSON 数组，展示为逗号分隔
  try {
    const arr = JSON.parse(raw)
    if (Array.isArray(arr)) {
      console.debug(
        t('log.deployTargetSiteName', { raw: JSON.stringify(raw), result: arr.join(', ') }),
      )
      return arr.join(', ')
    }
  } catch {
    // 非 JSON 直接返回
  }
  console.debug(t('log.deployTargetSiteName', { raw: JSON.stringify(raw), result: raw }))
  return raw
}
function credName(): string {
  if (!target.value) return ''
  return target.value.credential_source === 'dns_provider'
    ? target.value.dns_provider_name || ''
    : target.value.deploy_credential_name || ''
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
// parseNotAfter 将证书到期时间解析为毫秒时间戳。
// 兼容 RFC3339（"2026-09-29T07:28:29Z"）与 Go time.DateTime（"2026-09-29 07:28:29"，按 UTC 处理）。
function parseNotAfter(s?: string): number | null {
  if (!s) return null
  console.debug(i18nStore.t('log.deployTargetParseNotAfterInput', { input: String(s) }))
  const direct = new Date(s).getTime()
  if (!isNaN(direct)) {
    console.debug(
      i18nStore.t('log.deployTargetParseNotAfterDirect', {
        result: new Date(direct).toISOString(),
      }),
    )
    return direct
  }
  const m = s.match(/^(\d{4}-\d{2}-\d{2})[ T](\d{2}:\d{2}:\d{2})/)
  if (m) {
    const t = new Date(m[1] + 'T' + m[2] + 'Z').getTime()
    if (!isNaN(t)) {
      console.debug(
        i18nStore.t('log.deployTargetParseNotAfterFallback', { result: new Date(t).toISOString() }),
      )
      return t
    }
  }
  console.error(i18nStore.t('log.deployTargetParseNotAfterFailed', { input: String(s) }))
  return null
}
function remainingDays(notAfter?: string): string {
  const ts = parseNotAfter(notAfter)
  console.debug(t('log.deployTargetRemainingDays', { notAfter: String(notAfter), ts: String(ts) }))
  if (ts == null) return '-'
  const diff = ts - Date.now()
  const days = Math.floor(diff / 86400000)
  if (days < 0) return t('deploy.expired')
  return days + ' ' + t('deploy.days')
}

// ---- 部署 ----
const domainOptions = ref<{ label: string; value: string }[]>([])
// domainMeta 以选项 value 为键，记录每个站点/域名的 name 与 id，
// 部署时据此分别传「站点名」与「站点 ID」，避免界面出现「名称||ID」这类拼接串。
const domainMeta = ref<Record<string, { name: string; id: string }>>({})

// 将后端返回的「网站名||站点ID」拆为独立字段
function parseSiteEntry(s: string): { name: string; id: string } {
  const idx = s.indexOf('||')
  if (idx >= 0) return { name: s.slice(0, idx), id: s.slice(idx + 2) }
  return { name: s, id: '' }
}

// 初始根据已保存配置填充下拉选项（配置里站点名与站点 ID 各自独立存储）
function buildInitialDomainOptions(): { label: string; value: string }[] {
  domainMeta.value = {}
  const cfg = target.value?.config
  if (!cfg) return []
  const isPanel = isPanelProvider(target.value?.provider_type || '')
  const opts: { label: string; value: string }[] = []
  if (isPanel) {
    const names = cfg.site_name || []
    const ids = cfg.site_id || []
    names.forEach((name, i) => {
      const id = ids[i] || ''
      const key = id || name
      domainMeta.value[key] = { name, id }
      opts.push({ label: name, value: key })
    })
  } else {
    ;(cfg.domains || []).forEach((d) => {
      domainMeta.value[d] = { name: d, id: '' }
      opts.push({ label: d, value: d })
    })
  }
  return opts
}

const fetchingDomains = ref(false)

// 获取域名：调用云接口拉取该部署目标已配置凭证下的真实可部署域名，刷新列表
const fetchDomains = async () => {
  if (!target.value) return
  fetchingDomains.value = true
  try {
    const list = await DeployService.ListCDNDomains(target.value.id)
    if (!list || list.length === 0) {
      showMessage(t('deploy.noDomains'), 'warning')
    } else {
      const isPanel = isPanelProvider(target.value?.provider_type || '')
      domainMeta.value = {}
      domainOptions.value = list.map((raw) => {
        if (isPanel) {
          // 面板/防火墙类返回「网站名||站点ID」，拆开：展示站点名，value 用站点 ID
          const { name, id } = parseSiteEntry(raw)
          const key = id || name
          domainMeta.value[key] = { name, id }
          return { label: name, value: key }
        }
        domainMeta.value[raw] = { name: raw, id: '' }
        return { label: raw, value: raw }
      })
      showMessage(t('deploy.fetchDomains') + ': ' + list.length, 'success')
    }
  } catch (e: any) {
    showMessage(
      t('deploy.operationFailed') + ': ' + translateBackend(e?.message || String(e)),
      'error',
    )
  } finally {
    fetchingDomains.value = false
  }
}

function renderCertLabel(c: CertificateListItem) {
  const label = `${c.domain} (${remainingDays(c.not_after)})`
  console.debug(
    t('log.deployTargetRenderCertLabel', {
      domain: c.domain,
      notAfter: String(c.not_after),
      label,
    }),
  )
  return label
}
// 判断证书中的某个域名条目（支持通配符 *.example.com）是否覆盖目标域名
function patternCovers(pattern: string, domain: string): boolean {
  if (!pattern || !domain) return false
  if (pattern === domain) return true
  if (pattern.startsWith('*.')) {
    const base = pattern.slice(2)
    // *.base 只匹配 base 的单级子域名（如 *.wifi235.cn 匹配 admin.wifi235.cn，不匹配 wifi235.cn 也不匹配 a.b.wifi235.cn）
    return (
      domain.length > base.length &&
      domain.endsWith('.' + base) &&
      !domain.slice(0, domain.length - base.length - 1).includes('.')
    )
  }
  return false
}
function domainMatch(c: CertificateListItem, domains: string[]): boolean {
  const patterns = [c.domain, ...(c.sans || [])].filter(Boolean)
  return domains.some((d) => patterns.some((p) => patternCovers(p, d)))
}

// 部署列表：每行一个站点/域名，行内显示匹配的本地证书，支持批量/单个部署
interface DeployRow {
  key: string
  name: string
  siteID: string
  matched: CertificateListItem[]
  certOptions: { label: string; value: number }[]
  selectedCertId: number | null
}
const selectedRowKeys = ref<string[]>([])
const rowCertSelection = ref<Record<string, number>>({})
const rowDeploying = ref<Record<string, boolean>>({})
const deployRows = computed<DeployRow[]>(() => {
  const linkedCertIds: number[] = (target.value as any)?.cert_ids || []
  return domainOptions.value.map((opt) => {
    const key = opt.value
    const meta = domainMeta.value[key] || { name: opt.label, id: '' }
    const name = meta.name || opt.label
    const matched = (certificates.value || []).filter((c) => domainMatch(c, [name]))
    const certOptions = matched.map((c) => ({ label: renderCertLabel(c), value: c.id }))
    // 默认优先选「目标已关联且覆盖该域名的证书」，否则取第一个匹配证书
    const preferred = matched.find((c) => linkedCertIds.includes(c.id))
    const selectedCertId = rowCertSelection.value[key] ?? preferred?.id ?? matched[0]?.id ?? null
    return { key, name, siteID: meta.id, matched, certOptions, selectedCertId }
  })
})

// 列表行全选 / 清空 / 切换
const selectAllRows = () => {
  selectedRowKeys.value = deployRows.value.map((r) => r.key)
}
const clearRows = () => {
  selectedRowKeys.value = []
}
const toggleRow = (key: string, checked: boolean) => {
  if (checked) {
    if (!selectedRowKeys.value.includes(key))
      selectedRowKeys.value = [...selectedRowKeys.value, key]
  } else {
    selectedRowKeys.value = selectedRowKeys.value.filter((k) => k !== key)
  }
}
const onRowCertChange = (key: string, v: number) => {
  rowCertSelection.value = { ...rowCertSelection.value, [key]: v }
}

const deploying = ref(false)
const deployResults = ref<
  { domain: string; cloud_cert_id?: string; success: boolean; message: string }[]
>([])

// 部署单个站点/域名（用行内选定的证书）
async function deployRow(row: DeployRow) {
  if (!target.value) return
  const certId = row.selectedCertId
  if (!certId) {
    showMessage(t('deploy.selectCertHint'), 'warning')
    return
  }
  rowDeploying.value = { ...rowDeploying.value, [row.key]: true }
  try {
    const resp = await DeployService.DeployCertificate(
      target.value.id,
      certId,
      row.name,
      row.siteID,
    )
    deployResults.value.push({
      domain: row.name,
      cloud_cert_id: resp?.cloud_cert_id || undefined,
      success: resp?.success ?? true,
      message: resp?.message || t('deploy.deploySuccess'),
    })
  } catch (e: any) {
    deployResults.value.push({
      domain: row.name,
      success: false,
      message: translateBackend(e?.message || String(e)),
    })
  } finally {
    rowDeploying.value = { ...rowDeploying.value, [row.key]: false }
    await loadTarget()
    await loadLogs()
    await loadCurrentCerts()
  }
}

// 批量部署选中的行
async function deploySelected() {
  if (selectedRowKeys.value.length === 0) {
    showMessage(t('deploy.selectDomains'), 'warning')
    return
  }
  deploying.value = true
  try {
    for (const key of selectedRowKeys.value) {
      const row = deployRows.value.find((r) => r.key === key)
      if (row) await deployRow(row)
    }
  } finally {
    deploying.value = false
  }
}

// —— 实时查询云端/面板当前生效证书（B 方案：本地 + 云端并排对比，预留 C 对比）——
const currentCerts = ref<Record<string, CurrentCertDTO>>({})
const fetchingCurrentCerts = ref(false)

async function loadCurrentCerts() {
  if (!target.value) return
  fetchingCurrentCerts.value = true
  try {
    const resp = await DeployService.GetCurrentCerts(target.value.id)
    const m: Record<string, CurrentCertDTO> = {}
    if (resp?.results) {
      for (const [k, v] of Object.entries(resp.results)) {
        if (v) m[k] = v
      }
    }
    currentCerts.value = m
    console.debug(
      t('log.deployTargetLoadCurrentCerts', {
        results: JSON.stringify(
          Object.entries(m).map(([k, v]) => ({
            key: k,
            common_name: v.common_name,
            not_after: v.not_after,
            supported: v.supported,
            error: v.error,
          })),
        ),
      }),
    )
  } catch (e: any) {
    console.error(t('log.deployTargetLoadCurrentCertsError', { err: String(e) }))
    showMessage(translateBackend(e?.message || String(e)), 'error')
  } finally {
    fetchingCurrentCerts.value = false
  }
}

// 本地已选证书的到期时间（用于与云端当前证书对比）
function localCertNotAfter(row: DeployRow): string {
  const c = (certificates.value || []).find((x) => x.id === row.selectedCertId)
  return c?.not_after || ''
}

// 对比状态：same=本地与云端到期一致（已基本是最新）；diff=不一致（待更新）；none=无法对比。
// 按天粒度比较：部分面板（如 aaPanel）只返回到期日期，忽略时分秒避免误判。
function compareStatus(row: DeployRow): 'same' | 'diff' | 'none' {
  const cloud = currentCerts.value[row.key]
  if (!cloud || !cloud.supported || cloud.error || !cloud.not_after) return 'none'
  const local = localCertNotAfter(row)
  if (!local) return 'none'
  const lt = parseNotAfter(local)
  const ct = parseNotAfter(cloud.not_after)
  if (lt == null || ct == null) return 'none'
  const dayOf = (ts: number) => Math.floor(ts / 86400000) // UTC 天序号
  const status = dayOf(lt) === dayOf(ct) ? 'same' : 'diff'
  console.debug(
    t('log.deployTargetCompareStatus', {
      key: String(row.key),
      local: JSON.stringify(local),
      cloud: JSON.stringify(cloud.not_after),
      status,
    }),
  )
  return status
}

const historyColumns: DataTableColumns<DeployLogListItem> = [
  {
    title: t('deploy.time'),
    key: 'created_at',
    width: 170,
    render: (row) => formatDateTime(row.created_at) || '-',
  },
  {
    title: t('deploy.certDomain'),
    key: 'deploy_domain',
    render: (row) => row.deploy_domain || row.cert_domain || '-',
  },
  {
    title: t('deploy.status'),
    key: 'success',
    width: 90,
    render: (row) =>
      h(
        NTag,
        { size: 'small', bordered: false, type: row.success ? 'success' : 'error' },
        { default: () => (row.success ? t('deploy.status.success') : t('deploy.status.failed')) },
      ),
  },
  {
    title: t('deploy.response'),
    key: 'message',
    render: (row) => h('span', { class: 'text-xs opacity-70' }, row.message || '-'),
  },
]

async function loadTarget() {
  const t2 = await DeployService.GetDeployTarget(id)
  target.value = t2
  console.debug(
    t('log.deployTargetLoaded', {
      id: String(t2?.id),
      name: String(t2?.name),
      provider: String(t2?.provider_type),
      service: String(t2?.deploy_service),
    }),
  )
}
async function loadLogs() {
  logs.value = (await DeployService.ListDeployLogs(id)) || []
}
async function loadCertificates() {
  certificates.value = (await CertificateService.ListCertificates()) || []
  console.debug(
    t('log.deployTargetLoadCertificates', {
      count: certificates.value.length,
      first: JSON.stringify(
        certificates.value[0]
          ? { domain: certificates.value[0].domain, not_after: certificates.value[0].not_after }
          : null,
      ),
    }),
  )
}

async function doRemove() {
  try {
    await DeployService.DeleteDeployTarget(id)
    showMessage(t('deploy.operationSuccess'), 'success')
    router.push('/ssl-deploy')
  } catch (e: any) {
    showMessage(
      t('deploy.operationFailed') + ': ' + translateBackend(e?.message || String(e)),
      'error',
    )
  }
}

function goBack() {
  router.push('/ssl-deploy')
}
function goEdit() {
  router.push(`/ssl-deploy/${id}/edit`)
}
function goDeployTab() {
  activeTab.value = 'deploy'
}

onMounted(async () => {
  try {
    await Promise.all([loadTarget(), loadCertificates(), loadLogs()])
    // 域名/站点已在创建/编辑时的 config 中配置，进入部署 tab 直接作为可选项（默认不预选，由用户点「全选」或勾选）
    domainOptions.value = buildInitialDomainOptions()
    if (route.query.tab === 'deploy') {
      activeTab.value = 'deploy'
      await loadCurrentCerts()
    }
  } catch (e: any) {
    showMessage(t('deploy.loadFailed') + ': ' + translateBackend(e?.message || String(e)), 'error')
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <div class="page">
    <n-spin :show="loading">
      <template v-if="target">
        <!-- 头部 -->
        <div class="flex items-center justify-between mb-4">
          <div class="flex items-center gap-3">
            <n-button quaternary circle @click="goBack">
              <template #icon>
                <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M15 19l-7-7 7-7"
                  />
                </svg>
              </template>
            </n-button>
            <ProviderIcon
              :provider-type="target.provider_type"
              :name="providerLabel(target.provider_type)"
              :size="28"
            />
            <div>
              <h1 class="text-2xl font-bold flex items-center gap-2">
                {{ target.name }}
              </h1>
              <p class="text-sm mt-0.5 opacity-60">{{ t('deploy.detail') }}</p>
            </div>
          </div>
          <n-space>
            <n-button type="primary" @click="goDeployTab">
              <template #icon>
                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-8l-4-4m0 0L8 8m4-4v12"
                  />
                </svg>
              </template>
              {{ t('deploy.deploy') }}
            </n-button>
            <n-button @click="goEdit">
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
              {{ t('deploy.edit') }}
            </n-button>
            <n-popconfirm @positive-click="doRemove">
              <template #trigger>
                <n-button type="error">
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
                  {{ t('deploy.delete') }}
                </n-button>
              </template>
              {{ t('deploy.deleteConfirm') }}
            </n-popconfirm>
          </n-space>
        </div>

        <!-- 概览卡片 -->
        <div class="grid grid-cols-2 md:grid-cols-4 gap-3 mb-4">
          <n-card :bordered="false" size="small">
            <div class="text-xs opacity-60">{{ t('deploy.status') }}</div>
            <div class="mt-1">
              <n-tag :type="statusType(target.last_status) as any" :bordered="false" size="small">
                {{ statusText(target.last_status) }}
              </n-tag>
            </div>
          </n-card>
          <n-card :bordered="false" size="small">
            <div class="text-xs opacity-60">
              {{ t('deploy.provider') }} / {{ t('deploy.service') }}
            </div>
            <div class="mt-1">
              {{ providerLabel(target.provider_type) }} ·
              {{ serviceLabel(target.deploy_service, target.provider_type) }}
            </div>
          </n-card>
          <n-card :bordered="false" size="small">
            <div class="text-xs opacity-60">{{ t('deploy.credential') }}</div>
            <div class="mt-1">
              {{
                target.credential_source === 'dns_provider'
                  ? t('deploy.credFromDns')
                  : t('deploy.credFromCredential')
              }}
              <span v-if="credName()" class="opacity-70"> · {{ credName() }}</span>
            </div>
          </n-card>
          <n-card :bordered="false" size="small">
            <div class="text-xs opacity-60">{{ t('deploy.time') }}</div>
            <div class="mt-1">
              {{ formatDateTime(target.last_deployed_at) || t('deploy.status.never') }}
            </div>
          </n-card>
        </div>

        <!-- 标签页 -->
        <n-card :bordered="false">
          <n-tabs v-model:value="activeTab" type="line">
            <!-- 信息（默认） -->
            <n-tab-pane name="info" :tab="t('deploy.info')">
              <div class="space-y-4">
                <div class="deploy-info">
                  <div class="info-row">
                    <span class="info-label">{{ t('deploy.name') }}</span>
                    <span class="info-value">{{ target.name }}</span>
                  </div>
                  <div class="info-row">
                    <span class="info-label">{{ t('deploy.provider') }}</span>
                    <span class="info-value">{{ providerLabel(target.provider_type) }}</span>
                  </div>
                  <div class="info-row">
                    <span class="info-label">{{ t('deploy.service') }}</span>
                    <span class="info-value">{{
                      serviceLabel(target.deploy_service, target.provider_type)
                    }}</span>
                  </div>
                  <div class="info-row">
                    <span class="info-label">{{ t('deploy.region') }}</span>
                    <span class="info-value">{{ regionName() || '-' }}</span>
                  </div>
                  <div class="info-row">
                    <span class="info-label">{{
                      isPanelProvider(target?.provider_type || '')
                        ? t('deploy.sites')
                        : t('deploy.domains')
                    }}</span>
                    <span class="info-value">{{ siteName() || '-' }}</span>
                  </div>
                  <div class="info-row">
                    <span class="info-label">{{ t('deploy.credential') }}</span>
                    <span class="info-value">
                      {{
                        target.credential_source === 'dns_provider'
                          ? t('deploy.credFromDns')
                          : t('deploy.credFromCredential')
                      }}<span v-if="credName()" class="opacity-70"> · {{ credName() }}</span>
                    </span>
                  </div>
                  <div class="info-row">
                    <span class="info-label">{{ t('deploy.cert') }}</span>
                    <span class="info-value">
                      {{
                        ((target as any).cert_ids || []).length
                          ? ((target as any).cert_ids || []).length + ' ' + t('deploy.certCount')
                          : t('deploy.none')
                      }}
                    </span>
                  </div>
                  <div class="info-row">
                    <span class="info-label">{{ t('common.createdAt') }}</span>
                    <span class="info-value">{{ formatDateTime(target.created_at) || '-' }}</span>
                  </div>
                  <div class="info-row">
                    <span class="info-label">{{ t('common.updatedAt') }}</span>
                    <span class="info-value">{{ formatDateTime(target.updated_at) || '-' }}</span>
                  </div>
                  <div v-if="target.last_error" class="info-row full">
                    <span class="info-label">{{ t('deploy.error') }}</span>
                    <span class="info-value text-red-400 break-all">{{ target.last_error }}</span>
                  </div>
                </div>

                <div>
                  <div class="flex items-center justify-between mb-1">
                    <span class="text-sm opacity-60">{{ t('deploy.rawConfig') }}</span>
                    <n-button text size="tiny" @click="copyToClipboard(rawConfigText, 'rawConfig')">
                      <template #icon>
                        <svg
                          v-if="copiedField !== 'rawConfig'"
                          class="w-4 h-4"
                          fill="none"
                          stroke="currentColor"
                          viewBox="0 0 24 24"
                        >
                          <path
                            stroke-linecap="round"
                            stroke-linejoin="round"
                            stroke-width="2"
                            d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"
                          />
                        </svg>
                        <svg
                          v-else
                          class="w-4 h-4 text-green-500"
                          fill="none"
                          stroke="currentColor"
                          viewBox="0 0 24 24"
                        >
                          <path
                            stroke-linecap="round"
                            stroke-linejoin="round"
                            stroke-width="2"
                            d="M5 13l4 4L19 7"
                          />
                        </svg>
                      </template>
                    </n-button>
                  </div>
                  <pre
                    class="json-view"
                    :class="isDark ? 'json-dark' : 'json-light'"
                    v-html="highlightedConfig"
                  ></pre>
                </div>
              </div>
            </n-tab-pane>

            <!-- 部署 -->
            <n-tab-pane name="deploy" :tab="t('deploy.deploy')">
              <div class="space-y-4">
                <!-- 工具栏 -->
                <div class="flex items-center justify-between gap-2 flex-wrap">
                  <n-space size="small">
                    <n-button size="small" :loading="fetchingDomains" @click="fetchDomains">
                      {{
                        isPanelProvider(target?.provider_type || '')
                          ? t('deploy.fetchSites')
                          : t('deploy.fetchDomains')
                      }}
                    </n-button>
                    <n-button
                      size="small"
                      :disabled="deployRows.length === 0"
                      @click="selectAllRows"
                    >
                      {{ t('deploy.selectAll') }}
                    </n-button>
                    <n-button
                      size="small"
                      :disabled="selectedRowKeys.length === 0"
                      @click="clearRows"
                    >
                      {{ t('deploy.clearSelection') }}
                    </n-button>
                    <n-button
                      size="small"
                      :loading="fetchingCurrentCerts"
                      @click="loadCurrentCerts"
                    >
                      {{ t('deploy.fetchCurrentCert') }}
                    </n-button>
                  </n-space>
                  <n-button
                    type="primary"
                    size="small"
                    :loading="deploying"
                    :disabled="selectedRowKeys.length === 0"
                    @click="deploySelected"
                  >
                    {{ t('deploy.deploySelected') }} ({{ selectedRowKeys.length }})
                  </n-button>
                </div>

                <!-- 站点/域名列表 -->
                <div v-if="deployRows.length" class="deploy-rows">
                  <div v-for="row in deployRows" :key="row.key" class="deploy-row">
                    <n-checkbox
                      :checked="selectedRowKeys.includes(row.key)"
                      @update:checked="(v: boolean) => toggleRow(row.key, v)"
                    />
                    <div class="row-name" :title="row.name">{{ row.name }}</div>
                    <div class="row-cert">
                      <n-select
                        v-if="row.certOptions.length"
                        :value="row.selectedCertId ?? null"
                        :options="row.certOptions"
                        size="small"
                        :placeholder="t('deploy.selectCertHint')"
                        style="min-width: 220px; max-width: 360px"
                        @update:value="(v: number | null) => onRowCertChange(row.key, v as number)"
                      />
                      <span v-else class="row-cert-empty">{{ t('deploy.noCertMatched') }}</span>
                    </div>
                    <div class="row-actions">
                      <n-button
                        size="small"
                        :loading="!!rowDeploying[row.key]"
                        :disabled="!row.selectedCertId"
                        @click="deployRow(row)"
                      >
                        {{ t('deploy.deploy') }}
                      </n-button>
                    </div>
                    <!-- 云端当前生效证书（与本地并排展示，预留 C 对比） -->
                    <div class="row-cloud">
                      <template v-if="currentCerts[row.key]">
                        <template v-if="!currentCerts[row.key].supported">
                          <span class="cloud-badge cloud-muted">{{
                            t('deploy.cloudCertUnsupported')
                          }}</span>
                        </template>
                        <template v-else-if="currentCerts[row.key].error">
                          <span class="cloud-badge cloud-error">{{
                            translateBackend(currentCerts[row.key].error)
                          }}</span>
                        </template>
                        <template v-else>
                          <span class="cloud-label">{{ t('deploy.currentCloudCert') }}</span>
                          <span class="cloud-cn" :title="currentCerts[row.key].common_name">{{
                            currentCerts[row.key].common_name || '-'
                          }}</span>
                          <n-tag
                            v-if="(currentCerts[row.key].sans || []).length"
                            size="tiny"
                            :bordered="false"
                            type="info"
                            :title="(currentCerts[row.key].sans || []).join(', ')"
                          >
                            {{ (currentCerts[row.key].sans || []).slice(0, 2).join(', ') }}
                            <template v-if="(currentCerts[row.key].sans || []).length > 2">
                              +{{ (currentCerts[row.key].sans || []).length - 2 }}</template
                            >
                          </n-tag>
                          <span
                            class="cloud-after"
                            :title="formatDateTime(currentCerts[row.key].not_after)"
                          >
                            {{ formatDateTime(currentCerts[row.key].not_after) }}
                            <template v-if="remainingDays(currentCerts[row.key].not_after) !== '-'">
                              ({{ remainingDays(currentCerts[row.key].not_after) }})
                            </template>
                          </span>
                          <n-tag
                            v-if="compareStatus(row) !== 'none'"
                            size="tiny"
                            :bordered="false"
                            :type="compareStatus(row) === 'same' ? 'success' : 'warning'"
                          >
                            {{
                              compareStatus(row) === 'same'
                                ? t('deploy.sameAsLocal')
                                : t('deploy.needUpdate')
                            }}
                          </n-tag>
                        </template>
                      </template>
                      <span v-else-if="fetchingCurrentCerts" class="cloud-badge cloud-muted">{{
                        t('common.loading')
                      }}</span>
                    </div>
                  </div>
                </div>
                <n-empty v-else :description="t('deploy.noDomains')" />

                <!-- 部署结果 -->
                <div v-if="deployResults.length > 0">
                  <n-collapse :default-expanded-names="deployResults.map((_, i) => i)">
                    <n-collapse-item v-for="(r, i) in deployResults" :key="i" :name="i">
                      <template #header>
                        <span class="flex items-center gap-2">
                          <n-tag
                            :type="r.success ? 'success' : 'error'"
                            :bordered="false"
                            size="small"
                          >
                            {{ r.success ? t('deploy.status.success') : t('deploy.status.failed') }}
                          </n-tag>
                          {{ r.domain }}
                        </span>
                      </template>
                      <div class="text-xs opacity-80 break-all">
                        <div v-if="r.cloud_cert_id">
                          {{ t('deploy.cloudCertId') }}:
                          <span class="font-mono">{{ r.cloud_cert_id }}</span>
                        </div>
                        <div>{{ t('deploy.response') }}: {{ r.message }}</div>
                      </div>
                    </n-collapse-item>
                  </n-collapse>
                </div>
              </div>
            </n-tab-pane>

            <!-- 部署记录 -->
            <n-tab-pane name="history" :tab="t('deploy.history')">
              <n-empty
                v-if="logs.length === 0"
                :description="t('deploy.historyEmpty')"
                class="py-8"
              />
              <n-data-table
                v-else
                :columns="historyColumns"
                :data="logs"
                :row-key="(row: DeployLogListItem) => row.id"
                :scroll-x="640"
                size="small"
              />
            </n-tab-pane>
          </n-tabs>
        </n-card>
      </template>
      <n-empty v-else :description="t('deploy.loadFailed')" />
    </n-spin>
  </div>
</template>

<style>
/* 全局（非 scoped）：v-html 注入的 span 没有 data-v 属性，scoped/:deep 不可靠，故用全局样式保证高亮颜色生效 */
.json-view {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, 'Liberation Mono', monospace;
  font-size: 12px;
  line-height: 1.6;
  padding: 12px 14px;
  border-radius: 8px;
  overflow: auto;
  margin: 0;
  white-space: pre;
  word-break: break-word;
}
.json-light {
  background: #f6f8fa;
  color: #24292e;
}
.json-dark {
  background: #1e1e1e;
  color: #d4d4d4;
}
.json-light .json-key {
  color: #005cc5;
}
.json-light .json-str {
  color: #22863a;
}
.json-light .json-num {
  color: #b6019c;
}
.json-light .json-bool {
  color: #d73a49;
}
.json-light .json-null {
  color: #6f42c1;
}
.json-dark .json-key {
  color: #79b8ff;
}
.json-dark .json-str {
  color: #7ee787;
}
.json-dark .json-num {
  color: #f8c555;
}
.json-dark .json-bool {
  color: #ff7b72;
}
.json-dark .json-null {
  color: #d2a8ff;
}
</style>

<style scoped>
/* 信息 tab 字段网格：宽屏两列、窄屏单列，label 固定宽、value 占满并自动换行 */
.deploy-info {
  display: grid;
  grid-template-columns: 1fr;
  border: 1px solid var(--n-border-color, rgba(128, 128, 128, 0.22));
  border-radius: 8px;
  overflow: hidden;
}
@media (min-width: 768px) {
  .deploy-info {
    grid-template-columns: 1fr 1fr;
  }
}
.info-row {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 10px 12px;
  min-width: 0;
  border-bottom: 1px solid var(--n-border-color, rgba(128, 128, 128, 0.16));
  border-right: 1px solid var(--n-border-color, rgba(128, 128, 128, 0.16));
}
/* 两列时最后一行去掉底边，避免与外框双重边框 */
@media (min-width: 768px) {
  .info-row:nth-last-child(-n + 2):not(.full) {
    border-bottom: none;
  }
}
.info-row.full {
  grid-column: 1 / -1;
  border-right: none;
  border-bottom: none;
}
.info-label {
  flex: 0 0 96px;
  font-size: 13px;
  opacity: 0.62;
  line-height: 1.5;
}
.info-value {
  flex: 1 1 auto;
  min-width: 0;
  font-size: 13px;
  line-height: 1.5;
  word-break: break-word;
  overflow-wrap: anywhere;
}

/* 部署 tab：站点/域名列表 */
.deploy-rows {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.deploy-row {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 12px;
  border: 1px solid var(--n-border-color, rgba(128, 128, 128, 0.2));
  border-radius: 8px;
  flex-wrap: wrap;
}
.deploy-row .row-name {
  font-weight: 500;
  min-width: 140px;
  max-width: 280px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.deploy-row .row-cert {
  flex: 1 1 240px;
  min-width: 0;
}
.deploy-row .row-cert-empty {
  opacity: 0.5;
  font-size: 13px;
}
.deploy-row .row-actions {
  flex-shrink: 0;
}
/* 云端当前生效证书（与本地并排，预留 C 对比） */
.deploy-row .row-cloud {
  flex-basis: 100%;
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  padding-top: 6px;
  margin-top: 2px;
  border-top: 1px dashed var(--n-border-color, rgba(128, 128, 128, 0.18));
  font-size: 12px;
}
.deploy-row .row-cloud .cloud-label {
  opacity: 0.6;
}
.deploy-row .row-cloud .cloud-cn {
  font-weight: 600;
  max-width: 320px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.deploy-row .row-cloud .cloud-after {
  opacity: 0.7;
  font-variant-numeric: tabular-nums;
}
.cloud-badge {
  display: inline-flex;
  align-items: center;
  padding: 1px 8px;
  border-radius: 999px;
  font-size: 12px;
}
.cloud-muted {
  opacity: 0.55;
  background: rgba(128, 128, 128, 0.12);
}
.cloud-error {
  color: #d03050;
  background: rgba(208, 48, 80, 0.12);
}
</style>
