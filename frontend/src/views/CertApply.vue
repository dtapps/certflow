<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watchEffect, watch } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import {
  NCard,
  NInput,
  NSelect,
  NButton,
  NSteps,
  NStep,
  NSwitch,
  NSpin,
  NTag,
  NAlert,
} from 'naive-ui'
import * as CertificateService from '@bindings/cnb.cool/dtapp/certflow/certificateservicewrapper'
import * as CAService from '@bindings/cnb.cool/dtapp/certflow/caservicewrapper'
import * as DNSProviderService from '@bindings/cnb.cool/dtapp/certflow/dnsproviderservicewrapper'
import * as ClipboardService from '@bindings/cnb.cool/dtapp/certflow/clipboardservicewrapper'
import * as BrowserService from '@bindings/cnb.cool/dtapp/certflow/browserservicewrapper'
import type { CAListItem, DNSProviderListItem } from '@bindings/cnb.cool/dtapp/certflow/models'
import { useI18nStore } from '../stores/i18n'
import { useActionBarStore } from '../stores/actionBar'
import { useThemeStore } from '../stores/theme'
import { storeToRefs } from 'pinia'
import CAIcon from '../components/CAIcon.vue'
import ProviderIcon from '../components/ProviderIcon.vue'

const themeStore = useThemeStore()
const { isDark } = storeToRefs(themeStore)

const router = useRouter()
const route = useRoute()
const i18nStore = useI18nStore()
const { t } = i18nStore

const currentStep = ref(1)
const totalSteps = 4

// 选择卡片的统一样式（用 isDark 驱动，不依赖 Tailwind 的 dark: 变体，本项目未挂 .dark class）
const selectedCardCls = computed(() =>
  isDark.value ? 'border-blue-700 bg-blue-900/20' : 'border-blue-300 bg-blue-50/50',
)
const normalCardCls = computed(() =>
  isDark.value
    ? 'border-neutral-700 hover:border-neutral-600'
    : 'border-neutral-200 hover:border-neutral-300',
)

// 是否处于"继续申请"模式（从证书列表跳转过来）
const resumeCertID = ref<number | null>(null)
const isResumeMode = ref(false)

const formData = ref({
  domain: '',
  sans: '',
  keyType: 'EC256',
  caId: 0,
  dnsProviderId: 0,
  autoRenew: true,
  renewalDays: 30,
})

// 续期天数输入框：用 n-input 替代 n-input-number，规避 vue 3.6 beta 下 n-input-number 不渲染/不可输入的问题
const renewalDaysStr = ref(String(formData.value.renewalDays))
const allowDigits = (value: string) => /^\d*$/.test(value)
watch(renewalDaysStr, (val) => {
  if (val === '') return
  const n = parseInt(val, 10)
  if (isNaN(n)) return
  const clamped = Math.min(90, Math.max(1, n))
  formData.value.renewalDays = clamped
  if (clamped !== n) renewalDaysStr.value = String(clamped)
})

const isSubmitting = ref(false)
const applyResult = ref<{ success: boolean; message: string } | null>(null)
const cas = ref<CAListItem[]>([])
const dnsProviders = ref<DNSProviderListItem[]>([])

// 手动 DNS 挑战状态
const manualDNSChallenge = ref<{ records: { name: string; value: string }[] } | null>(null)
const isVerifying = ref(false)
const verifyFailed = ref(false)

const casOptions = computed(() =>
  cas.value.map((c) => ({
    label: c.name,
    value: c.id,
    disabled: c.account_email === '',
  })),
)

const dnsOptions = computed(() => {
  const manual = { label: t('apply.manualDNS'), value: 0, providerType: '' }
  return [
    manual,
    ...dnsProviders.value.map((p) => ({
      label: p.name,
      value: p.id,
      providerType: p.provider_type,
    })),
  ]
})

const keyTypeOptions = [
  { label: 'ECDSA P-256 (' + t('apply.keyTypeEc256') + ')', value: 'EC256' },
  { label: 'ECDSA P-384 (' + t('apply.keyTypeEc384') + ')', value: 'EC384' },
  { label: 'RSA 2048 (' + t('apply.keyTypeRsa2048') + ')', value: 'RSA2048' },
  { label: 'RSA 3072 (' + t('apply.keyTypeRsa3072') + ')', value: 'RSA3072' },
  { label: 'RSA 4096 (' + t('apply.keyTypeRsa4096') + ')', value: 'RSA4096' },
  { label: 'RSA 8192 (' + t('apply.keyTypeRsa8192') + ')', value: 'RSA8192' },
]

onMounted(async () => {
  try {
    const [caList, dnsList] = await Promise.all([
      CAService.ListCA(),
      DNSProviderService.ListDNSProviders(),
    ])
    cas.value = (caList ?? []).filter((c) => c.is_active)
    dnsProviders.value = dnsList ?? []

    // 检查是否是继续申请模式（从证书列表跳转过来）
    const certIdParam = route.query.certId
    if (certIdParam) {
      const certId = parseInt(certIdParam as string, 10)
      if (!isNaN(certId)) {
        resumeCertID.value = certId
        isResumeMode.value = true
        // 加载待完成的挑战信息
        const info = await CertificateService.GetPendingChallengeInfo(certId)
        if (info) {
          formData.value.domain = info.domain
          manualDNSChallenge.value = {
            records: info.records || [],
          }
          applyResult.value = { success: false, message: t('apply.addTXTFirst') }
          currentStep.value = totalSteps // 直接跳到确认步骤
        }
      }
    } else if (route.query.domain) {
      // 从失败证书重新申请，预填域名
      formData.value.domain = decodeURIComponent(route.query.domain as string)
      currentStep.value = 2 // 跳过域名步骤，从 CA 选择开始
    }
  } catch (e) {
    console.error(t('apply.loadFailed'), e)
  }
})

const parsedSans = computed(() => {
  if (!formData.value.sans) return []
  return formData.value.sans
    .split(',')
    .map((s) => s.trim())
    .filter((s) => s)
})

// 域名格式校验（支持通配符 *.example.com 与纯域名 example.com）
const domainValid = computed(() => {
  const d = formData.value.domain.trim()
  if (!d) return false
  const core = d.startsWith('*.') ? d.slice(2) : d
  const re = /^(?=.{1,253}$)(?:(?!-)[a-zA-Z0-9-]{1,63}(?<!-)\.)+[a-zA-Z]{2,}$/
  return re.test(core)
})

// 检测是否为通配符域名
const isWildcard = computed(() => {
  return formData.value.domain.trim().startsWith('*.')
})

// 从通配符域名提取根域名（例如 *.example.com → example.com）
const rootDomain = computed(() => {
  const domain = formData.value.domain.trim()
  if (domain.startsWith('*.')) {
    return domain.substring(2)
  }
  return ''
})

// 最终包含的所有域名（主域名 + SANs + 自动添加的根域名）
const allDomains = computed(() => {
  const main = formData.value.domain.trim()
  const sans = parsedSans.value
  const result = [main]
  // 通配符证书不包含根域名，自动添加
  if (
    isWildcard.value &&
    rootDomain.value &&
    !sans.includes(rootDomain.value) &&
    rootDomain.value !== main
  ) {
    result.push(rootDomain.value)
  }
  for (const san of sans) {
    if (!result.includes(san)) {
      result.push(san)
    }
  }
  return result
})

// 提交时使用的 SANs（包含自动添加的根域名）
const effectiveSans = computed(() => {
  const sans = [...parsedSans.value]
  if (isWildcard.value && rootDomain.value && !sans.includes(rootDomain.value)) {
    sans.push(rootDomain.value)
  }
  return sans
})

const canNext = computed(() => {
  switch (currentStep.value) {
    case 1:
      return domainValid.value
    case 2:
      return formData.value.caId > 0
    case 3:
      return formData.value.dnsProviderId >= 0
    case 4:
      return true
    default:
      return false
  }
})

const nextStep = () => {
  if (currentStep.value < totalSteps && canNext.value) currentStep.value++
}

const prevStep = () => {
  if (currentStep.value > 1) currentStep.value--
}

const submitApply = async () => {
  isSubmitting.value = true
  applyResult.value = null
  manualDNSChallenge.value = null
  verifyFailed.value = false
  try {
    const sans = effectiveSans.value.length > 0 ? effectiveSans.value : []
    const dnsProviderId =
      formData.value.dnsProviderId > 0 ? formData.value.dnsProviderId : undefined

    if (formData.value.dnsProviderId === 0) {
      // 手动 DNS 模式
      let challenge
      if (isResumeMode.value && resumeCertID.value) {
        // 继续申请模式：恢复之前的挑战
        challenge = await CertificateService.ResumeManualDNSChallenge(resumeCertID.value)
      } else {
        // 全新申请模式
        challenge = await CertificateService.StartManualDNSChallenge({
          domain: formData.value.domain,
          sans,
          ca_id: formData.value.caId,
          dns_provider_id: dnsProviderId,
          auto_renew: formData.value.autoRenew,
          renewal_days: formData.value.renewalDays,
          key_type: formData.value.keyType,
        })
      }
      if (challenge) {
        manualDNSChallenge.value = {
          records: challenge.records || [],
        }
        applyResult.value = { success: false, message: t('apply.addTXTFirst') }
      } else {
        applyResult.value = { success: false, message: t('apply.startManualFailed') }
      }
    } else {
      // 自动 DNS 模式
      const result = await CertificateService.ApplyCertificate({
        domain: formData.value.domain,
        sans,
        ca_id: formData.value.caId,
        dns_provider_id: dnsProviderId,
        auto_renew: formData.value.autoRenew,
        renewal_days: formData.value.renewalDays,
        key_type: formData.value.keyType,
      })
      if (result) {
        applyResult.value = {
          success: result.success,
          message: result.success
            ? t('apply.applySuccess')
            : result.error || t('apply.applyFailed'),
        }
        if (result.success) {
          setTimeout(() => router.push('/certificates'), 2000)
        }
      }
    }
  } catch (e: any) {
    applyResult.value = { success: false, message: e.message || t('apply.applyFailed') }
  } finally {
    isSubmitting.value = false
  }
}

const copyToClipboard = async (text: string) => {
  await ClipboardService.SetText(text)
}

const verifyDNSOnline = (record: { name: string; value: string }) => {
  if (manualDNSChallenge.value) {
    let domain = record.name
    // 去掉 _acme-challenge. 前缀用于在线查询
    if (domain.startsWith('_acme-challenge.')) {
      domain = domain.substring('_acme-challenge.'.length)
    }
    if (domain.startsWith('*.')) {
      domain = domain.substring(2)
    }
    // 去掉 FQDN 尾部的点
    if (domain.endsWith('.')) {
      domain = domain.substring(0, domain.length - 1)
    }
    const url = `https://myssl.com/dns_check.html?brand=2&type=2&domain=${domain}&txt=${record.value}#ssl_verify`
    BrowserService.OpenURL(url)
  }
}

const verifyDNS = async () => {
  isVerifying.value = true
  verifyFailed.value = false
  try {
    const result = await CertificateService.CompleteManualDNSChallenge(formData.value.domain)
    if (result && result.success) {
      applyResult.value = { success: true, message: t('apply.applySuccess') }
      manualDNSChallenge.value = null
      setTimeout(() => router.push('/certificates'), 2000)
    } else {
      applyResult.value = { success: false, message: result?.error || t('apply.dnsVerifyFailed') }
      verifyFailed.value = true
    }
  } catch (e: any) {
    applyResult.value = { success: false, message: e.message || t('apply.dnsVerifyError') }
    verifyFailed.value = true
  } finally {
    isVerifying.value = false
  }
}

const getStepTitle = (step: number) => {
  const titles = [
    '',
    t('apply.inputDomain'),
    t('apply.selectCAStep'),
    t('apply.selectDNSStep'),
    t('apply.confirmConfig'),
  ]
  return titles[step] || ''
}

// ---- 公共底部操作栏（ActionBar）控制 ----
const actionBar = useActionBarStore()

// 根据当前步骤与状态同步底部按钮；相关依赖变更时自动重算
watchEffect(() => {
  // 左侧：上一步（第 1 步不显示）
  actionBar.setLeft(
    currentStep.value > 1
      ? {
          text: t('apply.prevStep'),
          type: 'default',
          disabled: isSubmitting.value || isVerifying.value,
          withIcon: 'prev',
          onClick: prevStep,
        }
      : null,
  )

  // 右侧：下一步 / 验证并获取 / 提交申请
  if (currentStep.value < totalSteps) {
    actionBar.setRight({
      text: t('apply.nextStep'),
      type: 'primary',
      disabled: !canNext.value,
      withIcon: 'next',
      onClick: nextStep,
    })
  } else if (manualDNSChallenge.value) {
    actionBar.setRight({
      text: isVerifying.value
        ? t('apply.verifying')
        : verifyFailed.value
          ? t('apply.verifyFailedHint')
          : t('apply.verifyAndGet'),
      type: verifyFailed.value ? 'error' : 'success',
      disabled: isVerifying.value || verifyFailed.value,
      loading: isVerifying.value,
      onClick: verifyDNS,
    })
  } else {
    actionBar.setRight({
      text: isSubmitting.value ? t('apply.applying') : t('apply.submit'),
      type: 'primary',
      disabled: isSubmitting.value,
      loading: isSubmitting.value,
      onClick: submitApply,
    })
  }
})

onMounted(() => actionBar.show())
onUnmounted(() => actionBar.hide())
</script>

<template>
  <div class="page apply-page">
    <div class="text-center">
      <h1 class="text-2xl font-bold">
        {{ isResumeMode ? t('apply.resumeTitle') : t('apply.title') }}
      </h1>
      <p class="text-sm mt-1 opacity-60">
        {{ isResumeMode ? t('apply.resumeSubtitle') : t('apply.subtitle') }}
      </p>
    </div>

    <!-- 继续申请提示 -->
    <n-alert v-if="isResumeMode" type="info" :title="t('apply.resumeMode')">
      {{ t('apply.resumeDesc') }}
    </n-alert>

    <!-- 步骤指示器 -->
    <n-card v-if="!isResumeMode" size="small">
      <n-steps :current="currentStep" :status="'process'">
        <n-step :title="getStepTitle(1)" />
        <n-step :title="getStepTitle(2)" />
        <n-step :title="getStepTitle(3)" />
        <n-step :title="getStepTitle(4)" />
      </n-steps>
    </n-card>

    <!-- 表单内容 -->
    <n-card size="small" class="form-card">
      <div class="form-content">
        <!-- 步骤 1: 输入域名 -->
        <div v-if="currentStep === 1" class="space-y-6">
          <div>
            <label class="block text-sm font-medium mb-2">{{ t('apply.mainDomain') }}</label>
            <n-input
              v-model:value="formData.domain"
              :placeholder="t('apply.mainDomainPlaceholder')"
              :status="formData.domain.trim() && !domainValid ? 'error' : undefined"
            />
            <p class="text-xs mt-2 opacity-50">{{ t('apply.mainDomainHint') }}</p>
            <p v-if="formData.domain.trim() && !domainValid" class="text-xs mt-2 text-red-500">
              {{ t('apply.domainInvalid') }}
            </p>
          </div>
          <div>
            <label class="block text-sm font-medium mb-2">{{ t('apply.sans') }}</label>
            <n-input v-model:value="formData.sans" :placeholder="t('apply.sansPlaceholder')" />
            <p class="text-xs mt-2 opacity-50">{{ t('apply.sansHint') }}</p>
          </div>
          <n-alert
            v-if="isWildcard"
            type="info"
            :title="t('apply.wildcardHint').replace('{domain}', formData.domain)"
          >
            {{ t('apply.rootDomainHint').replace('{domain}', rootDomain) }}
          </n-alert>
          <div v-if="allDomains.length > 0" class="space-y-2">
            <p class="text-xs opacity-60">{{ t('apply.includedDomains') }}</p>
            <div class="flex flex-wrap gap-2">
              <n-tag type="info" size="small">{{ formData.domain }}</n-tag>
              <n-tag v-for="san in parsedSans" :key="san" size="small">{{ san }}</n-tag>
              <n-tag
                v-if="isWildcard && rootDomain && !parsedSans.includes(rootDomain)"
                type="success"
                size="small"
              >
                {{ rootDomain }} {{ t('apply.autoAdded') }}
              </n-tag>
            </div>
          </div>
          <div>
            <label class="block text-sm font-medium mb-2">{{ t('apply.keyType') }}</label>
            <n-select v-model:value="formData.keyType" :options="keyTypeOptions" />
            <p class="text-xs mt-2 opacity-50">{{ t('apply.keyTypeHint') }}</p>
          </div>
        </div>

        <!-- 步骤 2: 选择 CA -->
        <div v-if="currentStep === 2" class="space-y-4">
          <label class="block text-sm font-medium mb-4">{{ t('apply.selectCA') }}</label>
          <div
            v-for="caItem in cas"
            :key="caItem.id"
            class="p-4 rounded-xl border transition-colors"
            :class="[
              caItem.account_email === '' ? 'opacity-70 cursor-not-allowed' : 'cursor-pointer',
              formData.caId === caItem.id ? selectedCardCls : normalCardCls,
            ]"
            @click="caItem.account_email !== '' && (formData.caId = caItem.id)"
          >
            <div class="flex items-center gap-3">
              <CAIcon :directory-url="caItem.directory_url" :name="caItem.name" :size="28" />
              <div class="flex-1">
                <div class="flex items-center gap-2">
                  <p class="font-medium">{{ caItem.name }}</p>
                  <n-tag
                    v-if="caItem.account_email === ''"
                    type="warning"
                    size="small"
                    :bordered="false"
                  >
                    {{ t('apply.unconfigured') }}
                  </n-tag>
                </div>
                <p class="text-xs opacity-50">{{ caItem.directory_url }}</p>
              </div>
              <svg
                v-if="formData.caId === caItem.id"
                class="w-5 h-5 text-blue-500 dark:text-blue-400 shrink-0"
                fill="currentColor"
                viewBox="0 0 20 20"
              >
                <path
                  fill-rule="evenodd"
                  d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.7-9.3a1 1 0 00-1.4-1.4L9 10.6 7.7 9.3a1 1 0 00-1.4 1.4l2 2a1 1 0 001.4 0l4-4z"
                  clip-rule="evenodd"
                />
              </svg>
            </div>
          </div>
        </div>

        <!-- 步骤 3: 选择 DNS 提供商 -->
        <div v-if="currentStep === 3" class="space-y-4">
          <label class="block text-sm font-medium mb-4">{{ t('apply.selectDNS') }}</label>
          <div
            v-for="dns in dnsOptions"
            :key="dns.value"
            class="p-4 rounded-xl border cursor-pointer transition-colors"
            :class="formData.dnsProviderId === dns.value ? selectedCardCls : normalCardCls"
            @click="formData.dnsProviderId = dns.value"
          >
            <div class="flex items-center gap-3">
              <template v-if="dns.value === 0">
                <div class="w-10 h-10 rounded-xl bg-green-500 flex items-center justify-center">
                  <svg
                    class="w-6 h-6 text-white"
                    fill="none"
                    stroke="currentColor"
                    viewBox="0 0 24 24"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      stroke-width="2"
                      d="M21 12a9 9 0 01-9 9m9-9a9 9 0 00-9-9m9 9H3m9 9a9 9 0 01-9-9m9 9c1.657 0 3-4.03 3-9s-1.343-9-3-9m0 18c-1.657 0-3-4.03-3-9s1.343-9 3-9"
                    />
                  </svg>
                </div>
              </template>
              <ProviderIcon v-else :provider-type="dns.providerType" :name="dns.label" :size="28" />
              <div class="flex-1">
                <p class="font-medium">{{ dns.label }}</p>
                <p class="text-xs opacity-50">
                  {{ dns.value === 0 ? t('apply.manualVerify') : t('apply.autoVerify') }}
                </p>
              </div>
              <svg
                v-if="formData.dnsProviderId === dns.value"
                class="w-5 h-5 text-blue-500 dark:text-blue-400 shrink-0"
                fill="currentColor"
                viewBox="0 0 20 20"
              >
                <path
                  fill-rule="evenodd"
                  d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.7-9.3a1 1 0 00-1.4-1.4L9 10.6 7.7 9.3a1 1 0 00-1.4 1.4l2 2a1 1 0 001.4 0l4-4z"
                  clip-rule="evenodd"
                />
              </svg>
            </div>
          </div>
          <n-alert
            v-if="formData.dnsProviderId === 0"
            type="warning"
            :title="t('apply.manualMode')"
          >
            {{ t('apply.manualModeDesc') }}
          </n-alert>
        </div>

        <!-- 步骤 4: 确认配置 -->
        <div v-if="currentStep === 4" class="space-y-6">
          <div class="space-y-4">
            <h3 class="font-medium">{{ t('apply.confirmConfig') }}</h3>
            <div class="space-y-3">
              <div
                class="flex justify-between py-3 border-b border-neutral-200 dark:border-neutral-700"
              >
                <span class="opacity-60">{{ t('apply.mainDomainLabel') }}</span>
                <span class="font-medium">{{ formData.domain }}</span>
              </div>
              <div
                v-if="parsedSans.length > 0"
                class="py-3 border-b border-neutral-200 dark:border-neutral-700"
              >
                <div class="flex justify-between mb-2">
                  <span class="opacity-60">{{ t('apply.sansLabel') }}</span>
                  <span class="text-sm">{{ parsedSans.join(', ') }}</span>
                </div>
                <div
                  v-if="isWildcard && rootDomain && !parsedSans.includes(rootDomain)"
                  class="flex justify-between"
                >
                  <span class="opacity-60">{{ t('apply.rootDomainLabel') }}</span>
                  <n-tag type="success" size="small">{{ rootDomain }}</n-tag>
                </div>
              </div>
              <div
                class="flex justify-between py-3 border-b border-neutral-200 dark:border-neutral-700"
              >
                <span class="opacity-60">{{ t('apply.caLabel') }}</span>
                <span class="flex items-center gap-2">
                  <CAIcon
                    :directory-url="cas.find((c) => c.id === formData.caId)?.directory_url"
                    :name="cas.find((c) => c.id === formData.caId)?.name || ''"
                    :size="16"
                  />
                  {{ cas.find((c) => c.id === formData.caId)?.name }}
                </span>
              </div>
              <div
                class="flex justify-between py-3 border-b border-neutral-200 dark:border-neutral-700"
              >
                <span class="opacity-60">{{ t('apply.dnsLabel') }}</span>
                <span class="flex items-center gap-2">
                  <div
                    v-if="formData.dnsProviderId === 0"
                    class="w-7 h-7 rounded-lg bg-green-500 flex items-center justify-center"
                  >
                    <svg
                      class="w-4 h-4 text-white"
                      fill="none"
                      stroke="currentColor"
                      viewBox="0 0 24 24"
                    >
                      <path
                        stroke-linecap="round"
                        stroke-linejoin="round"
                        stroke-width="2"
                        d="M21 12a9 9 0 01-9 9m9-9a9 9 0 00-9-9m9 9H3m9 9a9 9 0 01-9-9m9 9c1.657 0 3-4.03 3-9s-1.343-9-3-9m0 18c-1.657 0-3-4.03-3-9s1.343-9 3-9"
                      />
                    </svg>
                  </div>
                  <ProviderIcon
                    v-else
                    :provider-type="
                      dnsOptions.find((d) => d.value === formData.dnsProviderId)?.providerType || ''
                    "
                    :name="dnsOptions.find((d) => d.value === formData.dnsProviderId)?.label || ''"
                    :size="16"
                  />
                  {{ dnsOptions.find((d) => d.value === formData.dnsProviderId)?.label }}
                </span>
              </div>
              <div
                class="flex justify-between py-3 border-b border-neutral-200 dark:border-neutral-700"
              >
                <span class="opacity-60">{{ t('apply.keyType') }}</span>
                <span class="font-mono text-sm">{{ formData.keyType }}</span>
              </div>
            </div>
          </div>

          <div
            class="p-4 rounded-xl border space-y-1"
            :class="
              isDark ? 'bg-neutral-800/40 border-neutral-700' : 'bg-neutral-50 border-neutral-200'
            "
          >
            <div class="flex items-center justify-between py-2">
              <div>
                <p class="font-medium">{{ t('apply.autoRenew') }}</p>
                <p class="text-xs opacity-50">{{ t('apply.autoRenewDesc') }}</p>
              </div>
              <n-switch v-model:value="formData.autoRenew" />
            </div>
            <div v-if="formData.autoRenew" class="flex items-center justify-between py-2">
              <span class="opacity-60">{{ t('apply.renewalDays') }}</span>
              <n-input
                v-model:value="renewalDaysStr"
                :allow-input="allowDigits"
                style="width: 128px"
              />
            </div>
          </div>

          <!-- 手动 DNS TXT 记录信息 -->
          <n-alert v-if="manualDNSChallenge" type="info" :title="t('apply.addTXT')">
            <div class="space-y-3">
              <div
                v-for="(record, index) in manualDNSChallenge.records"
                :key="index"
                class="space-y-2"
              >
                <div class="flex items-center gap-2">
                  <span class="text-xs opacity-50">{{ t('apply.record') }} {{ index + 1 }}</span>
                </div>
                <div class="flex items-center gap-2">
                  <span class="text-sm w-20 opacity-60">{{ t('apply.recordName') }}</span>
                  <n-input :value="record.name" readonly size="small" style="flex: 1">
                    <template #suffix>
                      <n-button text size="tiny" @click="copyToClipboard(record.name)">
                        <template #icon>
                          <svg
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
                        </template>
                      </n-button>
                    </template>
                  </n-input>
                </div>
                <div class="flex items-center gap-2">
                  <span class="text-sm w-20 opacity-60">{{ t('apply.recordValue') }}</span>
                  <n-input :value="record.value" readonly size="small" style="flex: 1">
                    <template #suffix>
                      <n-button text size="tiny" @click="copyToClipboard(record.value)">
                        <template #icon>
                          <svg
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
                        </template>
                      </n-button>
                    </template>
                  </n-input>
                </div>
                <div class="flex items-center gap-2">
                  <n-button text size="small" @click="verifyDNSOnline(record)">
                    <template #icon>
                      <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path
                          stroke-linecap="round"
                          stroke-linejoin="round"
                          stroke-width="2"
                          d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14"
                        />
                      </svg>
                    </template>
                    {{ t('apply.verifyOnline') }}
                  </n-button>
                </div>
                <div
                  v-if="index < manualDNSChallenge.records.length - 1"
                  class="border-b border-blue-200 dark:border-blue-800 my-2"
                ></div>
              </div>
            </div>
            <p class="text-xs mt-2 opacity-50">{{ t('apply.verifyHint') }}</p>
          </n-alert>

          <!-- 申请结果 -->
          <n-alert
            v-if="applyResult && !manualDNSChallenge"
            :type="applyResult.success ? 'success' : 'error'"
          >
            {{ applyResult.message }}
          </n-alert>
        </div>
      </div>
    </n-card>
  </div>
</template>
