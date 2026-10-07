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
import {
  servicesByProvider,
  isPanelProvider,
  providerLabel as providerLabelFn,
  serviceLabel as serviceLabelFn,
} from '../utils/deploy'
import { deployProviderOptions } from '../utils/deployProviderConfig'

const route = useRoute()
const router = useRouter()
const i18nStore = useI18nStore()
const { t } = i18nStore
const actionBar = useActionBarStore()

const editingId = ref<number | null>(null)
const saving = ref(false)
const loading = ref(false)
const dnsProviders = ref<{ id: number; name: string; provider_type: string }[]>([])
const deployCredentials = ref<{ id: number; name: string; provider_type: string }[]>([])

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
  comment: '',
})

// 部署厂商下拉选项（从配置文件导入，含已解析的 label）
const providerOptions = computed(() => deployProviderOptions(t))
// 部署服务选项（使用公共方法，面板/防火墙类直接显示 provider 名称）
const serviceOptions = computed(() => servicesByProvider(form.provider_type))

// 卡片选中态：直接复用 Naive 主题变量，明暗自适应（用 --primary-color，不依赖 Tailwind dark:）
function cardCls(selected: boolean): string {
  return selected ? 'brandcard brandcard-selected' : 'brandcard'
}

const providerLabel = computed(() => providerLabelFn(form.provider_type))
const serviceLabel = computed(() => serviceLabelFn(form.deploy_service, form.provider_type))
const credentialLabel = computed(() => {
  if (form.credential_source === 'dns_provider')
    return dnsOptions.value.find((o) => o.value === form.dns_provider_id)?.label || '-'
  return (
    deployCredentialOptions.value.find((o) => o.value === form.deploy_credential_id)?.label || '-'
  )
})
// ---- 分步向导 ----
const totalSteps = 3
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
  // 面板/防火墙类不能复用 DNS 凭证，强制使用部署凭证并预选站点服务，且无需区域。
  if (isPanelProvider(form.provider_type)) {
    form.credential_source = 'deploy_credential'
    form.deploy_service = 'site'
    form.region = ''
    return
  }
  const opts = servicesByProvider(form.provider_type)
  if (!opts.find((o) => o.value === form.deploy_service)) {
    form.deploy_service = opts.length ? opts[0].value : 'cdn'
  }
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

async function loadEditTarget() {
  loading.value = true
  try {
    const list = await DeployService.ListDeployTargets()
    const target = (list || []).find((x: DeployTargetListItem) => x.id === editingId.value)
    if (!target) {
      showMessage(t('deploy.operationFailed'), 'error')
      return
    }
    const cf = target.config
    form.name = target.name
    form.provider_type = target.provider_type
    form.deploy_service = target.deploy_service
    form.credential_source = target.credential_source
    form.dns_provider_id = target.dns_provider_id
    form.deploy_credential_id = target.deploy_credential_id
    form.access_key = ''
    form.secret_key = ''
    // 部署目标不再记录站点/域名/证书名称，仅回显区域（部署参数）。
    // 站点/域名与证书在部署与核对时按凭证实时拉取，见 DeployTargetDetail。
    form.region = cf?.region || cf?.region_id || ''
    form.comment = target.comment || ''
  } catch (e: any) {
    showMessage(t('deploy.loadFailed') + ': ' + (e?.message || String(e)), 'error')
  } finally {
    loading.value = false
  }
}

function buildConfig(): Record<string, any> {
  const cfg: Record<string, any> = {}
  if (form.provider_type === 'aliyun') {
    cfg.region_id = form.region
  } else if (!isPanelProvider(form.provider_type)) {
    cfg.region = form.region
  }
  // 部署目标不再落库站点/域名/zone/加速器、也不记录证书名称等标识，
  // 部署与核对时按凭证实时拉取真实列表（证书名由后端按域名+指纹自动生成）。
  return cfg
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
                  <n-radio-button
                    value="dns_provider"
                    :disabled="isPanelProvider(form.provider_type)"
                    >{{ t('deploy.credFromDns') }}</n-radio-button
                  >
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
                v-if="form.provider_type !== 'baiducloud' && !isPanelProvider(form.provider_type)"
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

            <!-- 步骤 3：确认（站点/域名/证书名称均不在表单记录，部署时按凭证实时拉取） -->
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
/* 步骤条：naive-ui 默认每个 n-step 为 flex:1，末步 splitor 被隐藏，
   导致末步只占其槽位左侧、右侧残留空白（看着像被删步骤留下的空位）。
   让末步按内容宽度收缩并贴右，便均分整行、无残留空位。 */
:deep(.n-steps .n-step:last-child) {
  flex: 0 0 auto;
}
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
