<script setup lang="ts">
import { ref, onMounted, computed, h } from 'vue'
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
  NDivider,
  NDescriptions,
  NDescriptionsItem,
  NPopconfirm,
  NTabs,
  NTabPane,
  useMessage,
  type DataTableColumns,
} from 'naive-ui'
import * as DeployService from '@bindings/cnb.cool/dtapp/certflow/deployservicewrapper'
import * as CertificateService from '@bindings/cnb.cool/dtapp/certflow/certificateservicewrapper'
import type {
  DeployTargetListItem,
  DeployLogListItem,
  CertificateListItem,
} from '@bindings/cnb.cool/dtapp/certflow/models'
import { storeToRefs } from 'pinia'
import { useI18nStore } from '../stores/i18n'
import { useThemeStore } from '../stores/theme'
import { showMessage, translateBackend } from '../utils/message'
import { regionOf, regionLabel } from '../utils/region'
import ProviderIcon from '../components/ProviderIcon.vue'

const router = useRouter()
const route = useRoute()
const i18nStore = useI18nStore()
const { t } = i18nStore
const { isDark } = storeToRefs(useThemeStore())
const message = useMessage()

const id = Number(route.params.id)

const loading = ref(true)
const target = ref<DeployTargetListItem | null>(null)
const logs = ref<DeployLogListItem[]>([])
const certificates = ref<CertificateListItem[]>([])

// 复制字段标记（用于显示复制成功图标）
const copiedField = ref('')
const copyToClipboard = async (text: string, field: string) => {
  try {
    await navigator.clipboard.writeText(text)
    copiedField.value = field
    setTimeout(() => {
      if (copiedField.value === field) copiedField.value = ''
    }, 2000)
  } catch (e) {
    console.error('copy failed', e)
  }
}

const activeTab = ref<string>('info')

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

const providerOptions = [
  { label: t('deploy.provider.aliyun'), value: 'aliyun' },
  { label: t('deploy.provider.tencentcloud'), value: 'tencentcloud' },
  { label: t('deploy.provider.huawei'), value: 'huawei' },
  { label: t('deploy.provider.baidu'), value: 'baiducloud' },
  { label: t('deploy.provider.ctyun'), value: 'ctyun' },
]
const serviceOptions = [
  { label: t('deploy.service.cdn'), value: 'cdn' },
  { label: t('deploy.service.dcdn'), value: 'dcdn' },
  { label: t('deploy.service.edgeone'), value: 'edgeone' },
  { label: t('deploy.service.esa'), value: 'esa' },
  { label: t('deploy.service.slb'), value: 'slb' },
  { label: t('deploy.service.waf'), value: 'waf' },
  { label: t('deploy.service.elb'), value: 'elb' },
  { label: t('deploy.service.scm'), value: 'scm' },
  { label: t('deploy.service.ga'), value: 'ga' },
  { label: t('deploy.service.drcdn'), value: 'drcdn' },
  { label: t('deploy.service.ecdn'), value: 'ecdn' },
  { label: t('deploy.service.ctcdn'), value: 'ctcdn' },
  { label: t('deploy.service.icdn'), value: 'icdn' },
  { label: t('deploy.service.accessone'), value: 'accessone' },
]

function providerLabel(v?: string) {
  return providerOptions.find((o) => o.value === v)?.label || v || ''
}
function serviceLabel(v?: string) {
  return serviceOptions.find((o) => o.value === v)?.label || v || ''
}
function regionName(): string {
  if (!target.value) return ''
  return regionLabel(
    target.value.provider_type,
    target.value.deploy_service,
    regionOf(target.value.config),
  )
}
function siteName(): string {
  const cf = (target.value?.config || {}) as Record<string, any>
  return cf.site_name || cf.zone_name || ''
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
function remainingDays(notAfter?: string): string {
  if (!notAfter) return '-'
  const diff = new Date(notAfter).getTime() - Date.now()
  const days = Math.floor(diff / 86400000)
  if (days < 0) return t('deploy.expired')
  return days + ' ' + t('deploy.days')
}

// ---- 部署 ----
// 与 DeployTargets.vue 的 domainList 逻辑保持一致：config.domains 为 JSON 字符串数组，直接 JSON.parse
const targetDomains = computed<string[]>(() => {
  const raw = (target.value?.config as Record<string, any> | undefined)?.['domains']
  if (!raw) return []
  try {
    const arr = JSON.parse(raw)
    return Array.isArray(arr) ? arr : []
  } catch {
    return []
  }
})

const selectedDomains = ref<string[]>([])
const domainOptions = ref<{ label: string; value: string }[]>([])

const certOptions = computed(() => {
  const list = certificates.value || []
  // 已选择域名时，只展示能覆盖所选域名的证书（按 domain / san 匹配）
  const sel = selectedDomains.value
  const filtered = sel.length ? list.filter((c) => domainMatch(c, sel)) : list
  return filtered.map((c) => ({
    label: renderCertLabel(c),
    value: c.id,
  }))
})

function renderCertLabel(c: CertificateListItem) {
  return `${c.domain} (${remainingDays(c.not_after)})`
}
function renderCertTag(c: CertificateListItem) {
  return `${c.domain} · ${remainingDays(c.not_after)}`
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

const selectedCertIds = ref<number[]>([])
const deploying = ref(false)
const deployResults = ref<
  { domain: string; cloud_cert_id?: string; success: boolean; message: string }[]
>([])

async function runDeploy() {
  if (!target.value) return
  if (selectedDomains.value.length === 0) {
    showMessage(t('deploy.selectDomains'), 'warning')
    return
  }
  if (selectedCertIds.value.length === 0) {
    showMessage(t('deploy.selectCertHint'), 'warning')
    return
  }
  deploying.value = true
  deployResults.value = []
  const certMap = new Map(certificates.value.map((c) => [c.id, c]))
  try {
    for (const certId of selectedCertIds.value) {
      const cert = certMap.get(certId)
      if (!cert) continue
      const patterns = [cert.domain, ...(cert.sans || [])].filter(Boolean)
      const matched = selectedDomains.value.filter((d) => patterns.some((p) => patternCovers(p, d)))
      const domainsToDeploy =
        matched.length > 0
          ? matched
          : targetDomains.value.filter((d) => patterns.some((p) => patternCovers(p, d)))
      for (const domain of domainsToDeploy) {
        try {
          const resp = await DeployService.DeployCertificate(target.value.id, certId, domain)
          deployResults.value.push({
            domain,
            cloud_cert_id: resp?.cloud_cert_id || undefined,
            success: resp?.success ?? true,
            message: resp?.message || t('deploy.deploySuccess'),
          })
        } catch (e: any) {
          deployResults.value.push({
            domain,
            success: false,
            message: translateBackend(e?.message || String(e)),
          })
        }
      }
    }
    await loadTarget()
    await loadLogs()
  } catch (e: any) {
    showMessage(
      t('deploy.operationFailed') + ': ' + translateBackend(e?.message || String(e)),
      'error',
    )
  } finally {
    deploying.value = false
  }
}

const historyColumns: DataTableColumns<DeployLogListItem> = [
  {
    title: t('deploy.time'),
    key: 'created_at',
    width: 170,
    render: (row) => row.created_at || '-',
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
  const certIds = (target.value as any).cert_ids as number[] | undefined
  if (certIds?.length) {
    selectedCertIds.value = [...certIds]
  }
}
async function loadLogs() {
  logs.value = (await DeployService.ListDeployLogs(id)) || []
}
async function loadCertificates() {
  certificates.value = (await CertificateService.ListCertificates()) || []
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
    // 域名已在创建/编辑时的 config 中配置，进入部署 tab 直接作为可选项并预选，无需从云端拉取即可一键部署
    const doms = targetDomains.value
    domainOptions.value = doms.map((d) => ({ label: d, value: d }))
    if (doms.length) selectedDomains.value = [...doms]
    if (route.query.tab === 'deploy') activeTab.value = 'deploy'
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
              {{ providerLabel(target.provider_type) }} · {{ serviceLabel(target.deploy_service) }}
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
            <div class="mt-1">{{ target.last_deployed_at || t('deploy.status.never') }}</div>
          </n-card>
        </div>

        <!-- 标签页 -->
        <n-card :bordered="false">
          <n-tabs v-model:value="activeTab" type="line">
            <!-- 信息（默认） -->
            <n-tab-pane name="info" :tab="t('deploy.info')">
              <div class="space-y-4">
                <n-descriptions :column="1" label-placement="left" bordered>
                  <n-descriptions-item :label="t('deploy.name')">{{
                    target.name
                  }}</n-descriptions-item>
                  <n-descriptions-item :label="t('deploy.provider')">{{
                    providerLabel(target.provider_type)
                  }}</n-descriptions-item>
                  <n-descriptions-item :label="t('deploy.service')">{{
                    serviceLabel(target.deploy_service)
                  }}</n-descriptions-item>
                  <n-descriptions-item :label="t('deploy.region')">{{
                    regionName() || '-'
                  }}</n-descriptions-item>
                  <n-descriptions-item :label="t('deploy.domains')">{{
                    siteName() || '-'
                  }}</n-descriptions-item>
                  <n-descriptions-item :label="t('deploy.credential')">
                    {{
                      target.credential_source === 'dns_provider'
                        ? t('deploy.credFromDns')
                        : t('deploy.credFromCredential')
                    }}
                    <span v-if="credName()" class="opacity-70"> · {{ credName() }}</span>
                  </n-descriptions-item>
                  <n-descriptions-item :label="t('deploy.cert')">
                    {{
                      ((target as any).cert_ids || []).length
                        ? ((target as any).cert_ids || []).length + ' ' + t('deploy.certCount')
                        : t('deploy.none')
                    }}
                  </n-descriptions-item>
                  <n-descriptions-item :label="t('common.createdAt')">{{
                    target.created_at || '-'
                  }}</n-descriptions-item>
                  <n-descriptions-item :label="t('common.updatedAt')">{{
                    target.updated_at || '-'
                  }}</n-descriptions-item>
                  <n-descriptions-item v-if="target.last_error" :label="t('deploy.error')">
                    <span class="text-red-400 break-all">{{ target.last_error }}</span>
                  </n-descriptions-item>
                </n-descriptions>

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
                <div>
                  <div class="text-sm opacity-60 mb-1">{{ t('deploy.domains') }}</div>
                  <n-select
                    v-model:value="selectedDomains"
                    :options="domainOptions"
                    multiple
                    :placeholder="t('deploy.selectDomains')"
                    style="max-width: 640px"
                  />
                </div>

                <div>
                  <div class="text-sm opacity-60 mb-1">{{ t('deploy.cert') }}</div>
                  <n-select
                    v-model:value="selectedCertIds"
                    :options="certOptions"
                    multiple
                    :placeholder="t('deploy.selectCertHint')"
                    style="max-width: 640px"
                  />
                </div>

                <n-space>
                  <n-button type="primary" :loading="deploying" @click="runDeploy">
                    {{ t('deploy.deploy') }}
                  </n-button>
                </n-space>

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
