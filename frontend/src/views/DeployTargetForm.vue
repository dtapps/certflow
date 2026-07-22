<script setup lang="ts">
import { ref, reactive, onMounted, onUnmounted, computed, watchEffect } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  NCard,
  NButton,
  NInput,
  NSelect,
  NForm,
  NFormItem,
  NRadioGroup,
  NRadioButton,
  NSpace,
  NSpin,
  NGrid,
  NGi,
  NSteps,
  NStep,
  NDescriptions,
  NDescriptionsItem,
} from 'naive-ui'
import * as DeployService from '@bindings/cnb.cool/dtapp/certflow/deployservicewrapper'
import * as DNSProviderService from '@bindings/cnb.cool/dtapp/certflow/dnsproviderservicewrapper'
import * as DeployCredentialService from '@bindings/cnb.cool/dtapp/certflow/deploycredentialservicewrapper'
import type {
  DeployTargetListItem,
  CreateDeployTargetRequest,
  UpdateDeployTargetRequest,
} from '@bindings/cnb.cool/dtapp/certflow/models'
import { useI18nStore } from '../stores/i18n'
import { showMessage } from '../utils/message'
import { regionOptions, defaultRegionFor } from '../utils/region'
import { useActionBarStore } from '../stores/actionBar'

const route = useRoute()
const router = useRouter()
const i18nStore = useI18nStore()
const { t } = i18nStore
const actionBar = useActionBarStore()

const editingId = ref<number | null>(null)
const saving = ref(false)
const loading = ref(false)
const fetchingDomains = ref(false)
const fetchingZones = ref(false)
const dnsProviders = ref<{ id: number; name: string; provider_type: string }[]>([])
const deployCredentials = ref<{ id: number; name: string; provider_type: string }[]>([])
const domainOptions = ref<{ label: string; value: string }[]>([])
const zoneOptions = ref<{ label: string; value: string }[]>([])

const form = reactive({
  name: '',
  provider_type: 'aliyun',
  deploy_service: 'cdn',
  credential_source: 'dns_provider',
  dns_provider_id: null as number | null,
  deploy_credential_id: null as number | null,
  access_key: '',
  secret_key: '',
  region: '',
  domains: [] as string[],
  cert_name: '',
  zone_id: '',
  zone_name: '',
  site_id: '',
  site_name: '',
  accelerator_id: '',
  listener_id: '',
  comment: '',
})

const providerOptions = [
  { label: t('deploy.provider.aliyun'), value: 'aliyun' },
  { label: t('deploy.provider.tencentcloud'), value: 'tencentcloud' },
  { label: t('deploy.provider.huawei'), value: 'huawei' },
  { label: t('deploy.provider.baidu'), value: 'baiducloud' },
  { label: t('deploy.provider.ctyun'), value: 'ctyun' },
  { label: t('deploy.provider.volcengine'), value: 'volcengine' },
]
// 部署服务随云厂商变化：不同厂商提供的可部署目标不同，只展示后端已实现的服务，
// 避免用户选到不属于该厂商、或后端未实现（只会上传不绑定）的服务。
const servicesByProvider = computed<{ label: string; value: string }[]>(() => {
  switch (form.provider_type) {
    case 'aliyun':
      return [
        { label: t('deploy.service.cdn'), value: 'cdn' },
        { label: t('deploy.service.dcdn'), value: 'dcdn' },
        { label: t('deploy.service.esa'), value: 'esa' },
        { label: t('deploy.service.ga'), value: 'ga' },
      ]
    case 'tencentcloud':
      return [
        { label: t('deploy.service.cdn'), value: 'cdn' },
        { label: t('deploy.service.edgeone'), value: 'edgeone' },
        { label: t('deploy.service.ecdn'), value: 'ecdn' },
      ]
    case 'huawei':
      return [
        { label: t('deploy.service.cdn'), value: 'cdn' },
        { label: t('deploy.service.waf'), value: 'waf' },
        { label: t('deploy.service.elb'), value: 'elb' },
      ]
    case 'baiducloud':
      return [
        { label: t('deploy.service.cdn'), value: 'cdn' },
        { label: t('deploy.service.drcdn'), value: 'drcdn' },
      ]
    case 'ctyun':
      return [
        { label: t('deploy.service.ctcdn'), value: 'ctcdn' },
        { label: t('deploy.service.icdn'), value: 'icdn' },
        { label: t('deploy.service.accessone'), value: 'accessone' },
      ]
    case 'volcengine':
      return [
        { label: t('deploy.service.cdn'), value: 'cdn' },
        { label: t('deploy.service.dcdn'), value: 'dcdn' },
      ]
    default:
      return [{ label: t('deploy.service.cdn'), value: 'cdn' }]
  }
})
const serviceOptions = computed(() => servicesByProvider.value)

// 卡片选中态：直接复用 Naive 主题变量，明暗自适应（用 --primary-color，不依赖 Tailwind dark:）
function cardCls(selected: boolean): string {
  return selected ? 'brandcard brandcard-selected' : 'brandcard'
}

const providerLabel = computed(
  () => providerOptions.find((p) => p.value === form.provider_type)?.label || form.provider_type,
)
const serviceLabel = computed(
  () =>
    serviceOptions.value.find((s) => s.value === form.deploy_service)?.label || form.deploy_service,
)
const credentialLabel = computed(() => {
  if (form.credential_source === 'dns_provider')
    return dnsOptions.value.find((o) => o.value === form.dns_provider_id)?.label || '-'
  return (
    deployCredentialOptions.value.find((o) => o.value === form.deploy_credential_id)?.label || '-'
  )
})
const domainStepVisible = computed(() =>
  ['cdn', 'dcdn', 'drcdn', 'edgeone', 'ecdn', 'ga', 'esa', 'ctcdn', 'icdn', 'accessone'].includes(
    form.deploy_service,
  ),
)

// ---- 分步向导 ----
const totalSteps = 4
const currentStep = ref(1)

const canNext = computed(() => {
  switch (currentStep.value) {
    case 1:
      return form.name.trim() !== ''
    case 2:
      if (form.credential_source === 'dns_provider') return !!form.dns_provider_id
      if (form.credential_source === 'deploy_credential') return !!form.deploy_credential_id
      return false
    case 3:
      if (form.deploy_service === 'edgeone') return !!form.zone_id
      if (form.deploy_service === 'esa') return !!form.site_id && !!form.region
      if (form.deploy_service === 'ga') return !!form.accelerator_id
      return true
    default:
      return true
  }
})

function nextStep() {
  if (currentStep.value < totalSteps && canNext.value) currentStep.value++
}
function prevStep() {
  if (currentStep.value > 1) currentStep.value--
}
function selectProvider(v: string) {
  if (form.provider_type === v) return
  form.provider_type = v
  onProviderChange()
}
function selectService(v: string) {
  if (form.deploy_service === v) return
  form.deploy_service = v
  onServiceChange()
}

// 底部操作栏：上一步 / 下一步 / 保存（与证书申请页一致）
watchEffect(() => {
  if (currentStep.value === 1) {
    actionBar.setLeft({
      text: t('deploy.back'),
      withIcon: 'none',
      onClick: () => router.push('/ssl-deploy'),
    })
  } else {
    actionBar.setLeft({
      text: t('deploy.prevStep'),
      type: 'tertiary',
      withIcon: 'prev',
      onClick: prevStep,
    })
  }
  if (currentStep.value < totalSteps) {
    actionBar.setRight({
      text: t('deploy.nextStep'),
      type: 'primary',
      withIcon: 'next',
      disabled: !canNext.value,
      onClick: nextStep,
    })
  } else {
    actionBar.setRight({
      text: t('deploy.save'),
      type: 'primary',
      withIcon: 'none',
      loading: saving.value,
      disabled: saving.value,
      onClick: save,
    })
  }
})

// onServiceChange：切换部署服务后，若当前 region 不在新服务的候选内（例如切到 ESA），
// 自动修正为该服务默认 region，避免残留不兼容的 region。
function onServiceChange() {
  const valid = regionOptions(form.provider_type, form.deploy_service).some(
    (o) => o.value === form.region,
  )
  if (!valid) form.region = defaultRegionFor(form.provider_type, form.deploy_service)
}

// onZoneChange / onSiteChange：用户从站点下拉选中后，把对应站点名一并写入表单，
// 供保存时持久化（config 里 zone_name / site_name），编辑回填即可看到名字而非纯 ID。
function onZoneChange(val: string | null) {
  const opt = zoneOptions.value.find((o) => o.value === val)
  form.zone_name = opt ? opt.label : ''
}
function onSiteChange(val: string | null) {
  const opt = zoneOptions.value.find((o) => o.value === val)
  form.site_name = opt ? opt.label : ''
}

// 部署厂商类型 → DNS 提供商枚举值的映射。
// 注意 DNS 提供商的「百度云」枚举值为 baiducloud，而部署目标使用 baidu，二者不同，
// 直接按 provider_type 相等过滤会导致百度部署目标复用时找不到 DNS 提供商。
const dnsTypeByDeployType: Record<string, string[]> = {
  aliyun: ['aliyun'],
  tencentcloud: ['tencentcloud'],
  huawei: ['huawei'],
  baiducloud: ['baiducloud'],
  volcengine: ['volcengine'],
}

const dnsOptions = computed(() => {
  const want = dnsTypeByDeployType[form.provider_type] || []
  return dnsProviders.value
    .filter((d) => want.includes(d.provider_type))
    .map((d) => ({ label: d.name, value: d.id }))
})

const deployCredentialOptions = computed(() => {
  return deployCredentials.value
    .filter((c) => c.provider_type === form.provider_type)
    .map((c) => ({ label: c.name, value: c.id }))
})

// 用户手动切换云厂商时调用：当前部署服务可能不属于新厂商，重置为第一项；
// 同时清掉只属于特定服务（EdgeOne/ESA）的 ZoneId / SiteId 配置，避免脏数据带入保存。
// 注意：不要放在 watch(form.provider_type) 里，否则编辑页 loadEditTarget 同步回填时
// watch 的异步后置触发会覆盖刚恢复的 deploy_service / zone_id / site_id。
function onProviderChange() {
  const opts = servicesByProvider.value
  if (!opts.find((o) => o.value === form.deploy_service)) {
    form.deploy_service = opts.length ? opts[0].value : 'cdn'
  }
  form.zone_id = ''
  form.zone_name = ''
  form.site_id = ''
  form.site_name = ''
  form.accelerator_id = ''
  form.listener_id = ''
  zoneOptions.value = []
  // 切换厂商后按新厂商 + 服务带出默认 region，避免残留旧厂商的 region。
  form.region = defaultRegionFor(form.provider_type, form.deploy_service)
}

async function loadDnsProviders() {
  try {
    const dlist = await DNSProviderService.ListDNSProviders()
    dnsProviders.value = (dlist || []).map((d) => ({
      id: d.id,
      name: d.name,
      provider_type: d.provider_type,
    }))
  } catch {
    dnsProviders.value = []
  }
}

async function loadDeployCredentials() {
  try {
    const clist = await DeployCredentialService.ListDeployCredentials()
    deployCredentials.value = (clist || []).map((c) => ({
      id: c.id,
      name: c.name,
      provider_type: c.provider_type,
    }))
  } catch {
    deployCredentials.value = []
  }
}

function parseDomains(cf: Record<string, any>): string[] {
  let doms: string[] = []
  if (cf.domains) {
    try {
      doms = JSON.parse(cf.domains)
    } catch {
      doms = []
    }
  }
  if (doms.length === 0 && cf.domain) doms = [cf.domain]
  return doms
}

async function loadEditTarget() {
  loading.value = true
  try {
    const list = await DeployService.ListDeployTargets()
    const target = (list || []).find((x: DeployTargetListItem) => x.id === editingId.value)
    if (!target) {
      showMessage(t('deploy.operationFailed'), 'error')
      return
    }
    const cf = (target.config || {}) as Record<string, any>
    const doms = parseDomains(cf)
    domainOptions.value = doms.map((d) => ({ label: d, value: d }))
    form.name = target.name
    form.provider_type = target.provider_type
    form.deploy_service = target.deploy_service
    form.credential_source = target.credential_source
    form.dns_provider_id = target.dns_provider_id
    form.deploy_credential_id = target.deploy_credential_id
    form.access_key = ''
    form.secret_key = ''
    form.region = cf.region || cf.region_id || ''
    form.domains = doms
    form.cert_name = cf.cert_name || ''
    form.zone_id = cf.zone_id || ''
    form.zone_name = cf.zone_name || ''
    form.site_id = cf.site_id || ''
    form.site_name = cf.site_name || ''
    form.accelerator_id = cf.accelerator_id || ''
    form.listener_id = cf.listener_id || ''
    // EdgeOne / ESA 站点下拉复用 zoneOptions：编辑回填时先放入已存 Id + 名称作为占位，
    // 待用户点“获取站点”后再替换为真实列表，避免下拉无显示且能直接看到站点名。
    if (form.deploy_service === 'edgeone' && form.zone_id) {
      zoneOptions.value = [{ label: form.zone_name || form.zone_id, value: form.zone_id }]
    }
    if (form.deploy_service === 'esa' && form.site_id) {
      zoneOptions.value = [{ label: form.site_name || form.site_id, value: form.site_id }]
    }
    form.comment = target.comment || ''
  } catch (e: any) {
    showMessage(t('deploy.loadFailed') + ': ' + (e?.message || String(e)), 'error')
  } finally {
    loading.value = false
  }
}

function buildConfig(): Record<string, string> {
  const cfg: Record<string, string> = {}
  if (form.provider_type === 'aliyun') {
    cfg.region_id = form.region
  } else {
    cfg.region = form.region
  }
  if (form.domains.length) cfg.domains = JSON.stringify(form.domains)
  if (form.cert_name) cfg.cert_name = form.cert_name
  if (form.deploy_service === 'edgeone' && form.zone_id) {
    cfg.zone_id = form.zone_id
    if (form.zone_name) cfg.zone_name = form.zone_name
  }
  if (form.deploy_service === 'esa' && form.site_id) {
    cfg.site_id = form.site_id
    if (form.site_name) cfg.site_name = form.site_name
  }
  if (form.deploy_service === 'ga') {
    if (form.accelerator_id) cfg.accelerator_id = form.accelerator_id
    if (form.listener_id) cfg.listener_id = form.listener_id
  }
  return cfg
}

// fetchDomains 调用云接口获取该账号下的可部署域名，用于下拉选择。
// - cdn：拉取 CDN 加速域名
// - edgeone：需在选好 ZoneId 后，按站点拉取 EdgeOne 加速域名（hosts），zone_id 通过 config 传入
async function fetchDomains() {
  fetchingDomains.value = true
  try {
    const cfg: Record<string, string> = {}
    if (form.provider_type === 'aliyun') cfg.region_id = form.region
    else cfg.region = form.region
    if (form.deploy_service === 'edgeone' && form.zone_id) cfg.zone_id = form.zone_id
    if (form.deploy_service === 'esa' && form.site_id) cfg.site_id = form.site_id
    const list = await DeployService.FetchCDNDomains({
      provider_type: form.provider_type,
      deploy_service: form.deploy_service,
      credential_source: form.credential_source,
      dns_provider_id: form.credential_source === 'dns_provider' ? form.dns_provider_id : null,
      deploy_credential_id:
        form.credential_source === 'deploy_credential' ? form.deploy_credential_id : null,
      region: form.region,
      config: cfg,
    })
    if (!list || list.length === 0) {
      // 百度云 CDN 与全站加速（DRCDN）域名共用同一列表但类型不同：选了 CDN 服务却拉不到域名，
      // 极可能是域名实为 DRCDN 类型。给出针对性提示，引导用户切换部署服务。
      if (form.provider_type === 'baiducloud' && form.deploy_service === 'cdn') {
        showMessage(t('deploy.baidu.cdnNoDomainsHint'), 'warning')
      } else {
        showMessage(t('deploy.noDomains'), 'warning')
      }
    } else {
      domainOptions.value = list.map((d) => ({ label: d, value: d }))
      showMessage(t('deploy.fetchDomains') + ': ' + list.length, 'success')
    }
  } catch (e: any) {
    showMessage(t('deploy.operationFailed') + ': ' + (e?.message || String(e)), 'error')
  } finally {
    fetchingDomains.value = false
  }
}

// fetchZones 拉取站点列表（EdgeOne 站点 / ESA 站点），结果形如 "站点名||ID"，
// 按 "||" 拆分后填充站点下拉，避免用户手填 ZoneId / SiteId。
// 不传站点 ID（zone_id/site_id）时，后端返回站点列表。
async function fetchZones() {
  fetchingZones.value = true
  try {
    const cfg: Record<string, string> = {}
    if (form.provider_type === 'aliyun') cfg.region_id = form.region
    else cfg.region = form.region
    const list = await DeployService.FetchCDNDomains({
      provider_type: form.provider_type,
      deploy_service: form.deploy_service,
      credential_source: form.credential_source,
      dns_provider_id: form.credential_source === 'dns_provider' ? form.dns_provider_id : null,
      deploy_credential_id:
        form.credential_source === 'deploy_credential' ? form.deploy_credential_id : null,
      region: form.region,
      config: cfg,
    })
    if (!list || list.length === 0) {
      showMessage(t('deploy.noZones'), 'warning')
    } else {
      zoneOptions.value = list.map((z) => {
        const idx = z.indexOf('||')
        const id = idx >= 0 ? z.slice(idx + 2) : z
        const name = idx >= 0 ? z.slice(0, idx) : z
        return { label: name || id, value: id }
      })
      showMessage(t('deploy.fetchZones') + ': ' + list.length, 'success')
    }
  } catch (e: any) {
    showMessage(t('deploy.operationFailed') + ': ' + (e?.message || String(e)), 'error')
  } finally {
    fetchingZones.value = false
  }
}

async function save() {
  saving.value = true
  try {
    const cfg = buildConfig()
    if (editingId.value) {
      const input: UpdateDeployTargetRequest = {
        name: form.name,
        provider_type: form.provider_type,
        deploy_service: form.deploy_service,
        credential_source: form.credential_source,
        dns_provider_id: form.credential_source === 'dns_provider' ? form.dns_provider_id : null,
        deploy_credential_id:
          form.credential_source === 'deploy_credential' ? form.deploy_credential_id : null,
        config: cfg,
        comment: form.comment,
      }
      await DeployService.UpdateDeployTarget(editingId.value, input)
    } else {
      const input: CreateDeployTargetRequest = {
        name: form.name,
        provider_type: form.provider_type,
        deploy_service: form.deploy_service,
        credential_source: form.credential_source,
        dns_provider_id: form.credential_source === 'dns_provider' ? form.dns_provider_id : null,
        deploy_credential_id:
          form.credential_source === 'deploy_credential' ? form.deploy_credential_id : null,
        config: cfg,
        is_active: true,
        comment: form.comment,
      }
      await DeployService.CreateDeployTarget(input)
    }
    showMessage(t('deploy.saved'), 'success')
    router.push('/ssl-deploy')
  } catch (e: any) {
    showMessage(t('deploy.operationFailed') + ': ' + (e?.message || String(e)), 'error')
  } finally {
    saving.value = false
  }
}

onMounted(async () => {
  actionBar.show()
  await Promise.all([loadDnsProviders(), loadDeployCredentials()])
  const idParam = route.params.id
  if (idParam !== undefined && idParam !== '') {
    editingId.value = Number(idParam)
    await loadEditTarget()
    currentStep.value = totalSteps // 编辑时直接到确认步骤，可返回逐步修改
  } else {
    // 新建时按默认厂商 + 服务预置 region，用户无需手敲。
    form.region = defaultRegionFor(form.provider_type, form.deploy_service)
  }
})

onUnmounted(() => {
  actionBar.hide()
})
</script>

<template>
  <div class="page">
    <n-spin :show="loading">
      <div class="w-full">
        <div class="mb-5">
          <h1 class="text-2xl font-bold">
            {{ editingId ? t('deploy.edit') : t('deploy.create') }}
          </h1>
          <p class="text-sm opacity-60 mt-1">{{ t('deploy.formSubtitle') }}</p>
        </div>

        <n-card :bordered="false" class="mb-4">
          <n-steps :current="currentStep" :status="'process'">
            <n-step :title="t('deploy.step.basic')" />
            <n-step :title="t('deploy.step.credential')" />
            <n-step :title="t('deploy.step.target')" />
            <n-step :title="t('deploy.step.confirm')" />
          </n-steps>
        </n-card>

        <n-card :bordered="false">
          <n-form :model="form" label-placement="top">
            <!-- 步骤 1：基本信息 -->
            <template v-if="currentStep === 1">
              <n-form-item :label="t('deploy.name')">
                <n-input v-model:value="form.name" :placeholder="t('deploy.name')" />
              </n-form-item>
              <n-form-item :label="t('deploy.provider')">
                <n-grid :cols="3" :x-gap="12" :y-gap="12" responsive="screen" item-responsive>
                  <n-gi v-for="p in providerOptions" :key="p.value">
                    <div
                      :class="cardCls(form.provider_type === p.value)"
                      @click="selectProvider(p.value)"
                    >
                      <div class="font-medium">{{ p.label }}</div>
                    </div>
                  </n-gi>
                </n-grid>
              </n-form-item>
              <n-form-item :label="t('deploy.service')">
                <n-grid :cols="3" :x-gap="12" :y-gap="12" responsive="screen" item-responsive>
                  <n-gi v-for="s in serviceOptions" :key="s.value">
                    <div
                      :class="cardCls(form.deploy_service === s.value)"
                      @click="selectService(s.value)"
                    >
                      <div class="font-medium">{{ s.label }}</div>
                    </div>
                  </n-gi>
                </n-grid>
              </n-form-item>
            </template>

            <!-- 步骤 2：凭证与区域 -->
            <template v-else-if="currentStep === 2">
              <n-form-item :label="t('deploy.credentialSource')">
                <n-radio-group v-model:value="form.credential_source">
                  <n-radio-button value="dns_provider">{{
                    t('deploy.credFromDns')
                  }}</n-radio-button>
                  <n-radio-button value="deploy_credential">{{
                    t('deploy.credFromCredential')
                  }}</n-radio-button>
                </n-radio-group>
              </n-form-item>
              <n-form-item
                v-if="form.credential_source === 'dns_provider'"
                :label="t('deploy.dnsProvider')"
              >
                <n-select
                  v-model:value="form.dns_provider_id"
                  :options="dnsOptions"
                  :placeholder="t('deploy.selectDns')"
                  clearable
                />
              </n-form-item>
              <n-form-item v-else :label="t('deploy.credentialSource')">
                <n-select
                  v-model:value="form.deploy_credential_id"
                  :options="deployCredentialOptions"
                  :placeholder="t('deploy.selectDns')"
                  clearable
                />
              </n-form-item>
              <n-form-item
                v-if="form.provider_type !== 'baiducloud'"
                :label="t('deploy.config.region')"
              >
                <n-select
                  v-model:value="form.region"
                  :options="regionOptions(form.provider_type, form.deploy_service)"
                  filterable
                  tag
                  :placeholder="t('deploy.config.regionHint')"
                />
              </n-form-item>
            </template>

            <!-- 步骤 3：部署目标 -->
            <template v-else-if="currentStep === 3">
              <n-form-item
                v-if="form.deploy_service === 'edgeone'"
                :label="t('deploy.config.zoneId')"
              >
                <div class="w-full">
                  <n-space :size="8" class="mb-2">
                    <n-button size="small" :loading="fetchingZones" @click="fetchZones">
                      {{ fetchingZones ? t('deploy.fetchingZones') : t('deploy.fetchZones') }}
                    </n-button>
                  </n-space>
                  <n-select
                    v-model:value="form.zone_id"
                    :options="zoneOptions"
                    filterable
                    clearable
                    @update:value="onZoneChange"
                    :placeholder="t('deploy.config.zoneIdHint')"
                  />
                </div>
              </n-form-item>
              <n-form-item v-if="form.deploy_service === 'esa'" :label="t('deploy.config.siteId')">
                <div class="w-full">
                  <n-space :size="8" class="mb-2">
                    <n-button
                      size="small"
                      :loading="fetchingZones"
                      :disabled="!form.region"
                      @click="fetchZones"
                    >
                      {{ fetchingZones ? t('deploy.fetchingZones') : t('deploy.fetchESASites') }}
                    </n-button>
                    <span v-if="!form.region" class="text-sm text-gray-400">
                      {{ t('deploy.needRegionFirst') }}
                    </span>
                  </n-space>
                  <n-select
                    v-model:value="form.site_id"
                    :options="zoneOptions"
                    filterable
                    clearable
                    @update:value="onSiteChange"
                    :placeholder="t('deploy.config.siteIdHint')"
                  />
                </div>
              </n-form-item>
              <n-form-item
                v-if="form.deploy_service === 'ga'"
                :label="t('deploy.config.acceleratorId')"
              >
                <n-input
                  v-model:value="form.accelerator_id"
                  :placeholder="t('deploy.config.acceleratorIdHint')"
                />
              </n-form-item>
              <n-form-item
                v-if="form.deploy_service === 'ga'"
                :label="t('deploy.config.listenerId')"
              >
                <n-input
                  v-model:value="form.listener_id"
                  :placeholder="t('deploy.config.listenerIdHint')"
                />
              </n-form-item>
              <n-form-item v-if="domainStepVisible" :label="t('deploy.domains')">
                <div class="w-full">
                  <n-space
                    v-if="
                      form.deploy_service === 'cdn' ||
                      form.deploy_service === 'dcdn' ||
                      form.deploy_service === 'drcdn' ||
                      form.deploy_service === 'edgeone' ||
                      form.deploy_service === 'ecdn' ||
                      form.deploy_service === 'esa' ||
                      form.deploy_service === 'ctcdn' ||
                      form.deploy_service === 'icdn' ||
                      form.deploy_service === 'accessone'
                    "
                    :size="8"
                    class="mb-2"
                  >
                    <n-button
                      size="small"
                      :loading="fetchingDomains"
                      :disabled="
                        (form.deploy_service === 'edgeone' && !form.zone_id) ||
                        (form.deploy_service === 'esa' && (!form.site_id || !form.region))
                      "
                      @click="fetchDomains"
                    >
                      {{ fetchingDomains ? t('deploy.fetchingDomains') : t('deploy.fetchDomains') }}
                    </n-button>
                    <span
                      v-if="
                        (form.deploy_service === 'edgeone' && !form.zone_id) ||
                        (form.deploy_service === 'esa' && !form.site_id)
                      "
                      class="text-sm text-gray-400"
                    >
                      {{ t('deploy.needZoneFirst') }}
                    </span>
                    <span
                      v-if="form.deploy_service === 'esa' && form.site_id && !form.region"
                      class="text-sm text-gray-400"
                    >
                      {{ t('deploy.needRegionFirst') }}
                    </span>
                  </n-space>
                  <n-select
                    v-model:value="form.domains"
                    :options="domainOptions"
                    multiple
                    filterable
                    :tag="
                      form.deploy_service === 'edgeone' ||
                      form.deploy_service === 'dcdn' ||
                      form.deploy_service === 'drcdn' ||
                      form.deploy_service === 'ctcdn' ||
                      form.deploy_service === 'icdn' ||
                      form.deploy_service === 'accessone'
                    "
                    :placeholder="t('deploy.selectDomain')"
                  />
                </div>
              </n-form-item>
              <n-form-item :label="t('deploy.config.certName')">
                <n-input
                  v-model:value="form.cert_name"
                  :placeholder="t('deploy.config.certNameHint')"
                />
              </n-form-item>
            </template>

            <!-- 步骤 4：确认 -->
            <template v-else>
              <n-descriptions :column="1" bordered size="small" label-placement="left">
                <n-descriptions-item :label="t('deploy.name')">{{ form.name }}</n-descriptions-item>
                <n-descriptions-item :label="t('deploy.provider')">{{
                  providerLabel
                }}</n-descriptions-item>
                <n-descriptions-item :label="t('deploy.service')">{{
                  serviceLabel
                }}</n-descriptions-item>
                <n-descriptions-item :label="t('deploy.credentialSource')">{{
                  form.credential_source === 'dns_provider'
                    ? t('deploy.credFromDns')
                    : t('deploy.credFromCredential')
                }}</n-descriptions-item>
                <n-descriptions-item :label="t('deploy.selectDns')">{{
                  credentialLabel
                }}</n-descriptions-item>
                <n-descriptions-item v-if="form.region" :label="t('deploy.config.region')">{{
                  form.region
                }}</n-descriptions-item>
                <n-descriptions-item v-if="form.zone_id" :label="t('deploy.config.zoneId')">{{
                  form.zone_name || form.zone_id
                }}</n-descriptions-item>
                <n-descriptions-item v-if="form.site_id" :label="t('deploy.config.siteId')">{{
                  form.site_name || form.site_id
                }}</n-descriptions-item>
                <n-descriptions-item
                  v-if="form.accelerator_id"
                  :label="t('deploy.config.acceleratorId')"
                  >{{ form.accelerator_id }}</n-descriptions-item
                >
                <n-descriptions-item
                  v-if="form.listener_id"
                  :label="t('deploy.config.listenerId')"
                  >{{ form.listener_id }}</n-descriptions-item
                >
                <n-descriptions-item :label="t('deploy.domains')">{{
                  form.domains.join(', ') || '-'
                }}</n-descriptions-item>
                <n-descriptions-item :label="t('deploy.config.certName')">{{
                  form.cert_name || '-'
                }}</n-descriptions-item>
              </n-descriptions>
              <n-form-item :label="t('deploy.comment')" class="mt-4">
                <n-input v-model:value="form.comment" type="textarea" />
              </n-form-item>
            </template>
          </n-form>
        </n-card>
      </div>
    </n-spin>
  </div>
</template>

<style scoped>
.brandcard {
  cursor: pointer;
  border: 1px solid var(--border-color, rgba(128, 128, 128, 0.25));
  border-radius: 10px;
  padding: 12px 14px;
  transition: all 0.15s ease;
  background: transparent;
  min-height: 44px;
  display: flex;
  align-items: center;
}
.brandcard:hover {
  border-color: var(--primary-color);
}
.brandcard-selected {
  border-color: var(--primary-color);
  box-shadow: 0 0 0 2px color-mix(in srgb, var(--primary-color) 28%, transparent);
  background: color-mix(in srgb, var(--primary-color) 8%, transparent);
}
</style>
