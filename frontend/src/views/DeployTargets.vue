<script setup lang="ts">
import { ref, reactive, onMounted, computed, h } from 'vue'
import { useRouter } from 'vue-router'
import {
  NCard,
  NButton,
  NInput,
  NSelect,
  NModal,
  NForm,
  NFormItem,
  NEmpty,
  NTag,
  NSpin,
  NList,
  NListItem,
  NThing,
  NSpace,
  useMessage,
} from 'naive-ui'
import * as DeployService from '@bindings/cnb.cool/dtapp/certflow/deployservicewrapper'
import * as CertificateService from '@bindings/cnb.cool/dtapp/certflow/certificateservicewrapper'
import type {
  DeployTargetListItem,
  CertificateListItem,
  DeployLogListItem,
} from '@bindings/cnb.cool/dtapp/certflow/models'
import { useI18nStore } from '../stores/i18n'
import { showMessage, translateBackend } from '../utils/message'
import { regionOf, regionLabel } from '../utils/region'

const router = useRouter()
const i18nStore = useI18nStore()
const { t } = i18nStore
const message = useMessage()

const loading = ref(false)
const targets = ref<DeployTargetListItem[]>([])
const certificates = ref<CertificateListItem[]>([])

// 部署弹窗
const showDeployModal = ref(false)
const deployTarget = ref<DeployTargetListItem | null>(null)
const deployDomains = ref<string[]>([])
const deployCerts = ref<number[]>([])
const deploying = ref(false)
const deployResults = ref<{ cert: string; domain: string; success: boolean; message: string }[]>([])

// 部署历史弹窗
const showHistoryModal = ref(false)
const historyTarget = ref<DeployTargetListItem | null>(null)
const deployLogs = ref<DeployLogListItem[]>([])
const loadingHistory = ref(false)

async function openHistory(target: DeployTargetListItem) {
  historyTarget.value = target
  showHistoryModal.value = true
  deployLogs.value = []
  expandedLogs.value = {}
  loadingHistory.value = true
  try {
    const logs = await DeployService.ListDeployLogs(target.id)
    deployLogs.value = logs || []
  } catch (e: any) {
    showMessage(t('deploy.loadFailed') + ': ' + translateBackend(e?.message || String(e)), 'error')
  } finally {
    loadingHistory.value = false
  }
}

// 部署历史：接口反馈默认折叠，过长时截断预览，点击「展开/收起」查看完整内容
const RESPONSE_PREVIEW_LEN = 200
const expandedLogs = ref<Record<number, boolean>>({})
function toggleLog(id: number) {
  expandedLogs.value[id] = !expandedLogs.value[id]
}
function previewResponse(text: string): string {
  return text.length > RESPONSE_PREVIEW_LEN ? text.slice(0, RESPONSE_PREVIEW_LEN) + '…' : text
}
async function copyResponse(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    showMessage(t('deploy.copied'), 'success')
  } catch {
    showMessage(t('deploy.copy') + ' ' + t('deploy.operationFailed'), 'error')
  }
}

const providerOptions = [
  { label: t('deploy.provider.aliyun'), value: 'aliyun' },
  { label: t('deploy.provider.tencentcloud'), value: 'tencentcloud' },
  { label: t('deploy.provider.huawei'), value: 'huawei' },
  { label: t('deploy.provider.baidu'), value: 'baiducloud' },
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
]

function providerLabel(v?: string) {
  return providerOptions.find((o) => o.value === v)?.label || v || ''
}
function serviceLabel(v?: string) {
  return serviceOptions.find((o) => o.value === v)?.label || v || ''
}
// regionName 返回部署目标区域的中文显示名（找不到则回退代码），用于列表展示
function regionName(target: DeployTargetListItem): string {
  return regionLabel(target.provider_type, target.deploy_service, regionOf(target.config))
}
// siteName 返回部署目标站点名（ESA 存 site_name，EdgeOne 存 zone_name），用于列表展示
function siteName(target: DeployTargetListItem): string {
  const cf = (target.config || {}) as Record<string, any>
  return cf.site_name || cf.zone_name || ''
}

// domainMatch 判断证书名 name（可能含通配符 *.）是否覆盖域名 domain
// 规则：完全相等；或 *.example.com 匹配同级单层子域（www.example.com），
// 且按 RFC 6125，*.example.com 也视为覆盖裸域 example.com（常见证书签发含裸域 SAN，这里做宽松匹配）
function domainMatch(name: string, domain: string): boolean {
  name = name.toLowerCase()
  domain = domain.toLowerCase()
  if (name === domain) return true
  if (name.startsWith('*.')) {
    const base = name.slice(2) // example.com
    if (domain === base) return true // 裸域
    // 单层子域：sub.example.com，sub 不含点
    if (domain.endsWith('.' + base)) {
      const sub = domain.slice(0, domain.length - base.length - 1)
      return sub.length > 0 && !sub.includes('.')
    }
  }
  return false
}

// certOptions 根据已选域名过滤：证书任一名称（主域名/SAN，支持通配符）命中任一选中域名即显示
const certOptions = computed(() =>
  certificates.value
    .filter((c) => {
      if (deployDomains.value.length === 0) return true
      const names = [c.domain, ...(c.sans || [])]
      return deployDomains.value.some((d) => names.some((n) => domainMatch(n, d)))
    })
    .map((c) => ({ label: c.domain, value: c.id, cert: c })),
)

// remainingDays 计算证书剩余有效天数（null 表示无数据）
function remainingDays(notAfter?: string): number | null {
  if (!notAfter) return null
  const t = new Date(notAfter.replace(' ', 'T')).getTime()
  if (isNaN(t)) return null
  return Math.ceil((t - Date.now()) / 86400000)
}

// renderCertTag 已选证书标签：multiple 模式下只显示域名，避免 render-label 的多行内容撑大标签
function renderCertTag({ option }: any) {
  return h(NTag, { size: 'small', type: 'info', bordered: false }, { default: () => option.label })
}

// renderCertLabel 自定义证书下拉选项：展示域名、颁发者、有效期与剩余天数
function renderCertLabel(option: any) {
  const c = option.cert as {
    domain: string
    sans?: string[] | null
    issuer: string
    not_before: string
    not_after: string
  }
  const days = remainingDays(c.not_after)
  return h('div', { class: 'cert-option py-1' }, [
    h('div', { class: 'font-medium leading-tight' }, [
      c.domain,
      c.sans && c.sans.length
        ? h('span', { class: 'text-xs opacity-50 ml-1' }, `(SAN×${c.sans.length})`)
        : null,
    ]),
    h('div', { class: 'text-xs opacity-60 mt-0.5 flex items-center gap-2 flex-wrap' }, [
      h('span', c.issuer || '-'),
      h('span', '·'),
      h('span', c.not_before ? `${c.not_before} ~ ${c.not_after}` : c.not_after || ''),
      days !== null
        ? h(
            NTag,
            {
              size: 'small',
              bordered: false,
              type: days <= 0 ? 'error' : days <= 30 ? 'warning' : 'success',
            },
            { default: () => (days <= 0 ? t('deploy.expired') : `${days}${t('deploy.days')}`) },
          )
        : null,
    ]),
  ])
}

async function loadAll() {
  loading.value = true
  try {
    const [tlist, clist] = await Promise.all([
      DeployService.ListDeployTargets(),
      CertificateService.ListCertificates(),
    ])
    targets.value = tlist || []
    certificates.value = clist || []
  } catch (e: any) {
    showMessage(t('deploy.loadFailed') + ': ' + translateBackend(e?.message || String(e)), 'error')
  } finally {
    loading.value = false
  }
}

async function remove(target: DeployTargetListItem) {
  if (!confirm(t('deploy.deleteConfirm'))) return
  try {
    await DeployService.DeleteDeployTarget(target.id)
    await loadAll()
  } catch (e: any) {
    showMessage(
      t('deploy.operationFailed') + ': ' + translateBackend(e?.message || String(e)),
      'error',
    )
  }
}

function openDeploy(target: DeployTargetListItem) {
  deployTarget.value = target
  deployDomains.value = targetDomains(target)
  deployCerts.value = []
  deployResults.value = []
  showDeployModal.value = true
}

// targetDomains 解析部署目标已配置的 CDN 域名列表（兼容单域名旧数据）
function targetDomains(target: DeployTargetListItem | null): string[] {
  if (!target) return []
  const cf = (target.config || {}) as Record<string, any>
  if (cf.domains) {
    try {
      return JSON.parse(cf.domains)
    } catch {
      return []
    }
  }
  if (cf.domain) return [cf.domain]
  return []
}
const domainOptions = computed(() =>
  targetDomains(deployTarget.value).map((d) => ({ label: d, value: d })),
)

async function runDeploy() {
  if (!deployTarget.value || deployCerts.value.length === 0) return
  // ESA 为站点级部署，无需逐个选择域名；其余服务（CDN/EdgeOne 等）需先选域名
  if (deployDomains.value.length === 0 && deployTarget.value.deploy_service !== 'esa') {
    showMessage(t('deploy.selectDomainFirst'), 'warning')
    return
  }
  deploying.value = true
  deployResults.value = []
  try {
    // ESA 为站点级部署，无需逐个域名；deployDomains 为空时退化为单次空域名部署
    const isSiteLevel = deployTarget.value.deploy_service === 'esa'
    const domainsToDeploy =
      deployDomains.value.length > 0 ? deployDomains.value : isSiteLevel ? [''] : []
    for (const certId of deployCerts.value) {
      const cert = certificates.value.find((c) => c.id === certId)
      for (const domain of domainsToDeploy) {
        try {
          const r = await DeployService.DeployCertificate(deployTarget.value.id, certId, domain)
          deployResults.value.push({
            cert: cert?.domain || `#${certId}`,
            domain,
            success: r?.success ?? false,
            message: r?.message || '',
          })
        } catch (e: any) {
          deployResults.value.push({
            cert: cert?.domain || `#${certId}`,
            domain,
            success: false,
            message: translateBackend(e?.message || String(e)),
          })
        }
      }
    }
  } finally {
    deploying.value = false
    await loadAll()
  }
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

function goCreate() {
  router.push('/deploy/new')
}
function goEdit(target: DeployTargetListItem) {
  router.push(`/deploy/${target.id}/edit`)
}

onMounted(loadAll)
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

    <n-card :bordered="false">
      <n-spin :show="loading">
        <n-empty v-if="!loading && targets.length === 0" :description="t('deploy.empty')" />
        <n-list v-else>
          <n-list-item v-for="target in targets" :key="target.id">
            <n-thing :title="target.name">
              <template #description>
                <n-space :size="6" :wrap="true">
                  <n-tag size="small" :bordered="false">{{
                    providerLabel(target.provider_type)
                  }}</n-tag>
                  <n-tag size="small" :bordered="false" type="info">{{
                    serviceLabel(target.deploy_service)
                  }}</n-tag>
                  <n-tag v-if="regionName(target)" size="small" :bordered="false" type="warning">{{
                    regionName(target)
                  }}</n-tag>
                  <n-tag v-if="siteName(target)" size="small" :bordered="false" type="success">{{
                    siteName(target)
                  }}</n-tag>
                  <n-tag size="small" :bordered="false">
                    {{
                      target.credential_source === 'dns_provider'
                        ? t('deploy.credFromDns') +
                          (target.dns_provider_name ? ' · ' + target.dns_provider_name : '')
                        : t('deploy.credFromSelf')
                    }}
                  </n-tag>
                  <n-tag size="small" :bordered="false" :type="statusType(target.last_status)">
                    {{ statusText(target.last_status) }}
                  </n-tag>
                  <span v-if="target.last_deployed_at" class="text-xs opacity-50">{{
                    target.last_deployed_at
                  }}</span>
                </n-space>
                <p v-if="target.last_error" class="text-xs text-red-400 mt-1 break-all">
                  {{ target.last_error }}
                </p>
              </template>
              <template #action>
                <n-space>
                  <n-button size="small" type="primary" @click="openDeploy(target)">{{
                    t('deploy.deploy')
                  }}</n-button>
                  <n-button size="small" @click="openHistory(target)">{{
                    t('deploy.history')
                  }}</n-button>
                  <n-button size="small" @click="goEdit(target)">{{ t('deploy.edit') }}</n-button>
                  <n-button size="small" type="error" @click="remove(target)">{{
                    t('deploy.delete')
                  }}</n-button>
                </n-space>
              </template>
            </n-thing>
          </n-list-item>
        </n-list>
      </n-spin>
    </n-card>

    <!-- 部署弹窗 -->
    <n-modal
      v-model:show="showDeployModal"
      :title="t('deploy.deployTo') + '：' + (deployTarget?.name || '')"
      preset="card"
      style="width: 520px; max-width: 90vw"
    >
      <n-form label-placement="top">
        <n-form-item :label="t('deploy.domains')">
          <n-select
            v-model:value="deployDomains"
            :options="domainOptions"
            multiple
            :placeholder="t('deploy.selectDomains')"
          />
        </n-form-item>
        <n-form-item :label="t('deploy.cert')">
          <n-select
            v-model:value="deployCerts"
            :options="certOptions"
            :render-label="renderCertLabel"
            :render-tag="renderCertTag"
            multiple
            :placeholder="t('deploy.selectCertHint')"
          />
        </n-form-item>
      </n-form>
      <div v-if="deployResults.length" class="deploy-results">
        <div
          v-for="(r, i) in deployResults"
          :key="i"
          class="result-item"
          :class="r.success ? 'is-ok' : 'is-err'"
        >
          <div class="flex items-center gap-2 mb-1">
            <n-tag size="small" :bordered="false" :type="r.success ? 'success' : 'error'">
              {{ r.success ? t('deploy.status.success') : t('deploy.status.failed') }}
            </n-tag>
            <span class="text-xs font-medium">{{ r.cert }} @ {{ r.domain }}</span>
          </div>
          <p
            class="text-xs break-all whitespace-pre-wrap"
            :class="r.success ? 'opacity-60' : 'text-red-400'"
          >
            {{ r.message }}
          </p>
        </div>
      </div>
      <template #footer>
        <n-space justify="end">
          <n-button @click="showDeployModal = false">{{ t('deploy.close') }}</n-button>
          <n-button type="primary" :loading="deploying" @click="runDeploy">{{
            t('deploy.deploy')
          }}</n-button>
        </n-space>
      </template>
    </n-modal>

    <!-- 部署历史弹窗 -->
    <n-modal
      v-model:show="showHistoryModal"
      :title="t('deploy.history') + '：' + (historyTarget?.name || '')"
      preset="card"
      style="width: 640px"
    >
      <n-spin :show="loadingHistory">
        <n-empty
          v-if="!loadingHistory && deployLogs.length === 0"
          :description="t('deploy.historyEmpty')"
        />
        <n-list v-else>
          <n-list-item v-for="log in deployLogs" :key="log.id">
            <n-thing>
              <template #description>
                <n-space :size="6" :wrap="true" align="center">
                  <n-tag size="small" :bordered="false" :type="log.success ? 'success' : 'error'">
                    {{ log.success ? t('deploy.status.success') : t('deploy.status.failed') }}
                  </n-tag>
                  <span class="text-xs opacity-60"
                    >{{ t('deploy.certDomain') }}: {{ log.cert_domain }}</span
                  >
                  <span class="text-xs opacity-60">@ {{ log.deploy_domain }}</span>
                  <span v-if="log.cloud_cert_id" class="text-xs opacity-60"
                    >· {{ t('deploy.cloudCertId') }}: {{ log.cloud_cert_id }}</span
                  >
                  <span class="text-xs opacity-50">{{ log.created_at }}</span>
                </n-space>
                <p
                  v-if="log.message"
                  class="text-xs mt-1 break-all"
                  :class="log.success ? 'opacity-60' : 'text-red-400'"
                >
                  {{ log.message }}
                </p>
                <div v-if="log.response" class="text-xs mt-1">
                  <div class="flex items-center gap-2">
                    <span class="opacity-50">{{ t('deploy.response') }}：</span>
                    <n-button
                      v-if="log.response.length > RESPONSE_PREVIEW_LEN"
                      text
                      size="tiny"
                      type="primary"
                      @click="toggleLog(log.id)"
                      >{{
                        expandedLogs[log.id] ? t('deploy.collapse') : t('deploy.expand')
                      }}</n-button
                    >
                    <n-button text size="tiny" @click="copyResponse(log.response)">{{
                      t('deploy.copy')
                    }}</n-button>
                  </div>
                  <pre
                    v-if="expandedLogs[log.id] || log.response.length <= RESPONSE_PREVIEW_LEN"
                    class="mt-1 p-2 rounded bg-black/30 text-[11px] leading-relaxed break-all whitespace-pre-wrap max-h-60 overflow-auto font-mono opacity-80"
                    >{{ log.response }}</pre>
                  <span v-else class="break-all font-mono opacity-70">{{
                    previewResponse(log.response)
                  }}</span>
                </div>
              </template>
            </n-thing>
          </n-list-item>
        </n-list>
      </n-spin>
      <template #footer>
        <n-space justify="end">
          <n-button @click="showHistoryModal = false">{{ t('deploy.close') }}</n-button>
        </n-space>
      </template>
    </n-modal>
  </div>
</template>

<style scoped>
.deploy-results {
  max-height: 40vh;
  overflow-y: auto;
}
.result-item {
  padding: 8px 10px;
  border-radius: 8px;
  margin-bottom: 8px;
  word-break: break-all;
}
.result-item.is-ok {
  background: rgba(34, 197, 94, 0.1);
}
.result-item.is-err {
  background: rgba(239, 68, 68, 0.1);
}
</style>
