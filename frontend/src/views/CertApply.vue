<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import * as CertificateService from '@bindings/cnb.cool/dtapp/certflow/certificateservicewrapper'
import * as CAService from '@bindings/cnb.cool/dtapp/certflow/caservicewrapper'
import * as DNSProviderService from '@bindings/cnb.cool/dtapp/certflow/dnsproviderservicewrapper'
import * as ClipboardService from '@bindings/cnb.cool/dtapp/certflow/clipboardservicewrapper'
import * as BrowserService from '@bindings/cnb.cool/dtapp/certflow/browserservicewrapper'
import type { CAListItem, DNSProviderListItem } from '@bindings/cnb.cool/dtapp/certflow/models'
import { useI18n } from '../stores/i18n'

const router = useRouter()
const route = useRoute()
const { t } = useI18n()

const currentStep = ref(1)
const totalSteps = 4

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

const isSubmitting = ref(false)
const applyResult = ref<{ success: boolean; message: string } | null>(null)
const cas = ref<CAListItem[]>([])
const dnsProviders = ref<DNSProviderListItem[]>([])

// 手动 DNS 挑战状态
const manualDNSChallenge = ref<{ records: { name: string; value: string }[] } | null>(null)
const isVerifying = ref(false)
const verifyFailed = ref(false)

const casOptions = computed(() => cas.value.map(c => ({
  id: c.id,
  name: c.name,
  directory_url: c.directory_url,
})))

const dnsOptions = computed(() => {
  const manual = { id: 0, name: t('apply.manualDNS'), provider_type: 'manual' }
  return [manual, ...dnsProviders.value.map(p => ({
    id: p.id,
    name: p.name,
    provider_type: p.provider_type,
  }))]
})

onMounted(async () => {
  try {
    const [caList, dnsList] = await Promise.all([
      CAService.ListCA(),
      DNSProviderService.ListDNSProviders(),
    ])
    cas.value = caList ?? []
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
  return formData.value.sans.split(',').map(s => s.trim()).filter(s => s)
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
  if (isWildcard.value && rootDomain.value && !sans.includes(rootDomain.value) && rootDomain.value !== main) {
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
    case 1: return formData.value.domain.trim() !== ''
    case 2: return formData.value.caId > 0
    case 3: return formData.value.dnsProviderId >= 0
    case 4: return true
    default: return false
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
    const dnsProviderId = formData.value.dnsProviderId > 0 ? formData.value.dnsProviderId : undefined

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
          message: result.success ? t('apply.applySuccess') : (result.error || t('apply.applyFailed')),
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
  const titles = ['', t('apply.inputDomain'), t('apply.selectCAStep'), t('apply.selectDNSStep'), t('apply.confirmConfig')]
  return titles[step] || ''
}
</script>

<template>
  <div class="w-full space-y-6">
    <div class="text-center">
      <h1 class="text-2xl font-bold text-base-content">{{ isResumeMode ? t('apply.resumeTitle') : t('apply.title') }}</h1>
      <p class="text-content-70 text-sm mt-1">{{ isResumeMode ? t('apply.resumeSubtitle') : t('apply.subtitle') }}</p>
    </div>

    <!-- 继续申请提示 -->
    <div v-if="isResumeMode" class="p-4 rounded-xl bg-info-soft border border-info-soft flex items-center gap-3">
      <svg class="w-5 h-5 text-info flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
      </svg>
      <div>
        <p class="text-info font-medium text-sm">{{ t('apply.resumeMode') }}</p>
        <p class="text-content-70 text-xs mt-0.5">{{ t('apply.resumeDesc') }}</p>
      </div>
    </div>

    <!-- 步骤指示器 -->
    <div v-if="!isResumeMode" class="glass-panel rounded-2xl p-6">
      <div class="flex items-center justify-between">
        <div v-for="step in totalSteps" :key="step" class="flex items-center" :class="step < totalSteps ? 'flex-1' : ''">
          <div class="flex items-center gap-3">
            <div
              class="w-10 h-10 rounded-full flex items-center justify-center text-sm font-bold transition-all duration-300"
              :class="[
                currentStep > step ? 'bg-success text-white' :
                currentStep === step ? 'bg-primary text-white ring-4 ring-primary-soft' :
                'bg-base-300 text-content-70'
              ]"
            >
              <svg v-if="currentStep > step" class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
              </svg>
              <span v-else>{{ step }}</span>
            </div>
            <span class="text-sm font-medium hidden sm:block" :class="currentStep === step ? 'text-base-content' : 'text-content-50'">
              {{ getStepTitle(step) }}
            </span>
          </div>
          <div v-if="step < totalSteps" class="flex-1 h-0.5 mx-4 rounded-full transition-all duration-300" :class="currentStep > step ? 'bg-success' : 'bg-base-300'"></div>
        </div>
      </div>
    </div>

    <!-- 表单内容 -->
    <div class="glass-panel rounded-2xl p-8">
      <!-- 步骤 1: 输入域名 -->
      <div v-if="currentStep === 1" class="space-y-6">
        <div>
          <label class="block text-content-80 text-sm font-medium mb-2">{{ t('apply.mainDomain') }}</label>
          <input v-model="formData.domain" type="text" :placeholder="t('apply.mainDomainPlaceholder')" class="input w-full" />
          <p class="text-content-50 text-xs mt-2">{{ t('apply.mainDomainHint') }}</p>
        </div>
        <div>
          <label class="block text-content-80 text-sm font-medium mb-2">{{ t('apply.sans') }}</label>
          <input v-model="formData.sans" type="text" :placeholder="t('apply.sansPlaceholder')" class="input w-full" />
          <p class="text-content-50 text-xs mt-2">{{ t('apply.sansHint') }}</p>
        </div>
        <div v-if="isWildcard" class="p-3 rounded-lg bg-primary-faint border border-primary-soft">
          <p class="text-primary text-xs font-medium">{{ t('apply.wildcardHint').replace('{domain}', formData.domain) }}</p>
          <p class="text-content-50 text-xs mt-1">{{ t('apply.rootDomainHint').replace('{domain}', rootDomain) }}</p>
        </div>
        <div v-if="allDomains.length > 0" class="space-y-2">
          <p class="text-content-70 text-xs">{{ t('apply.includedDomains') }}</p>
          <div class="flex flex-wrap gap-2">
            <span class="px-2 py-1 rounded-md bg-primary-soft text-primary text-xs border border-primary-soft">{{ formData.domain }}</span>
            <span v-for="san in parsedSans" :key="san" class="px-2 py-1 rounded-md bg-base-300 text-content-80 text-xs border border-base-300">{{ san }}</span>
            <span v-if="isWildcard && rootDomain && !parsedSans.includes(rootDomain)" class="px-2 py-1 rounded-md bg-success-soft text-success text-xs border border-success-soft">{{ rootDomain }} {{ t('apply.autoAdded') }}</span>
          </div>
        </div>
        <div>
          <label class="block text-content-80 text-sm font-medium mb-2">{{ t('apply.keyType') }}</label>
          <select v-model="formData.keyType" class="select w-full">
            <option value="EC256">ECDSA P-256 ({{ t('apply.keyTypeEc256') }})</option>
            <option value="EC384">ECDSA P-384 ({{ t('apply.keyTypeEc384') }})</option>
            <option value="RSA2048">RSA 2048 ({{ t('apply.keyTypeRsa2048') }})</option>
            <option value="RSA3072">RSA 3072 ({{ t('apply.keyTypeRsa3072') }})</option>
            <option value="RSA4096">RSA 4096 ({{ t('apply.keyTypeRsa4096') }})</option>
            <option value="RSA8192">RSA 8192 ({{ t('apply.keyTypeRsa8192') }})</option>
          </select>
          <p class="text-content-50 text-xs mt-2">{{ t('apply.keyTypeHint') }}</p>
        </div>
      </div>

      <!-- 步骤 2: 选择 CA -->
      <div v-if="currentStep === 2" class="space-y-4">
        <label class="block text-content-80 text-sm font-medium mb-4">{{ t('apply.selectCA') }}</label>
        <div v-for="caItem in cas" :key="caItem.id"
          class="p-4 rounded-xl border-2 transition-all duration-200"
          :class="[
            caItem.account_email === '' ? 'border-amber-moderate bg-primary-faint cursor-not-allowed opacity-70' : 'cursor-pointer',
            formData.caId === caItem.id ? 'border-primary bg-primary-faint' : 'border-base-300 hover:border-base-content/30'
          ]"
          @click="caItem.account_email !== '' && (formData.caId = caItem.id)"
        >
          <div class="flex items-center gap-3">
            <div class="w-10 h-10 rounded-lg bg-accent-soft flex items-center justify-center">
              <svg class="w-5 h-5 text-accent" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 21V5a2 2 0 00-2-2H7a2 2 0 00-2 2v16m14 0h2m-2 0h-5m-9 0H3m2 0h5M9 7h1m-1 4h1m4-4h1m-1 4h1m-5 10v-5a1 1 0 011-1h2a1 1 0 011 1v5m-4 0h4" />
              </svg>
            </div>
            <div class="flex-1">
              <div class="flex items-center gap-2">
                <p class="text-base-content font-medium">{{ caItem.name }}</p>
                <span v-if="caItem.account_email === ''" class="px-2 py-0.5 rounded-full text-xs bg-amber-soft text-warning border border-amber-soft">{{ t('apply.unconfigured') }}</span>
              </div>
              <p class="text-content-50 text-xs">{{ caItem.directory_url }}</p>
            </div>
          </div>
        </div>
      </div>

      <!-- 步骤 3: 选择 DNS 提供商 -->
      <div v-if="currentStep === 3" class="space-y-4">
        <label class="block text-content-80 text-sm font-medium mb-4">{{ t('apply.selectDNS') }}</label>
        <div v-for="dns in dnsOptions" :key="dns.id"
          class="p-4 rounded-xl border-2 cursor-pointer transition-all duration-200"
          :class="formData.dnsProviderId === dns.id ? 'border-primary bg-primary-faint' : 'border-base-300 hover:border-base-content/30'"
          @click="formData.dnsProviderId = dns.id"
        >
          <div class="flex items-center gap-3">
            <div class="w-10 h-10 rounded-lg bg-success-soft flex items-center justify-center">
              <svg class="w-5 h-5 text-success" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 12a9 9 0 01-9 9m9-9a9 9 0 00-9-9m9 9H3m9 9a9 9 0 01-9-9m9 9c1.657 0 3-4.03 3-9s-1.343-9-3-9m0 18c-1.657 0-3-4.03-3-9s1.343-9 3-9" />
              </svg>
            </div>
            <div>
              <p class="text-base-content font-medium">{{ dns.name }}</p>
              <p class="text-content-50 text-xs">{{ dns.provider_type === 'manual' ? t('apply.manualVerify') : t('apply.autoVerify') }}</p>
            </div>
          </div>
        </div>
        <div v-if="formData.dnsProviderId === 0" class="mt-4 p-4 rounded-xl bg-amber-soft border border-amber-soft">
          <p class="text-warning text-sm font-medium">{{ t('apply.manualMode') }}</p>
          <p class="text-content-70 text-xs mt-1">{{ t('apply.manualModeDesc') }}</p>
        </div>
      </div>

      <!-- 步骤 4: 确认配置 -->
      <div v-if="currentStep === 4" class="space-y-6">
        <div class="space-y-4">
          <h3 class="text-base-content font-medium">{{ t('apply.confirmConfig') }}</h3>
          <div class="space-y-3">
            <div class="flex justify-between py-3 border-b border-base-300">
              <span class="text-content-70">{{ t('apply.mainDomainLabel') }}</span>
              <span class="text-base-content font-medium">{{ formData.domain }}</span>
            </div>
            <div v-if="parsedSans.length > 0" class="py-3 border-b border-base-300">
              <div class="flex justify-between mb-2">
                <span class="text-content-70">{{ t('apply.sansLabel') }}</span>
                <span class="text-base-content text-sm">{{ parsedSans.join(', ') }}</span>
              </div>
              <div v-if="isWildcard && rootDomain && !parsedSans.includes(rootDomain)" class="flex justify-between">
                <span class="text-content-70">{{ t('apply.rootDomainLabel') }}</span>
                <span class="text-success text-sm">{{ rootDomain }}</span>
              </div>
            </div>
            <div class="flex justify-between py-3 border-b border-base-300">
              <span class="text-content-70">{{ t('apply.caLabel') }}</span>
              <span class="text-base-content">{{ cas.find(c => c.id === formData.caId)?.name }}</span>
            </div>
            <div class="flex justify-between py-3 border-b border-base-300">
              <span class="text-content-70">{{ t('apply.dnsLabel') }}</span>
              <span class="text-base-content">{{ dnsOptions.find(d => d.id === formData.dnsProviderId)?.name }}</span>
            </div>
            <div class="flex justify-between py-3 border-b border-base-300">
              <span class="text-content-70">{{ t('apply.keyType') }}</span>
              <span class="text-base-content font-mono text-sm">{{ formData.keyType }}</span>
            </div>
          </div>
        </div>

        <div class="p-4 rounded-xl space-y-4">
          <div class="flex items-center justify-between">
            <div>
              <p class="text-base-content font-medium">{{ t('apply.autoRenew') }}</p>
              <p class="text-content-50 text-xs">{{ t('apply.autoRenewDesc') }}</p>
            </div>
            <button
              @click="formData.autoRenew = !formData.autoRenew"
              class="relative w-12 h-6 rounded-full transition-colors duration-200"
              :class="formData.autoRenew ? 'bg-primary' : 'bg-base-300'"
            >
              <span
                class="absolute top-1 w-4 h-4 rounded-full bg-white transition-transform duration-200"
                :class="formData.autoRenew ? 'left-7' : 'left-1'"
              ></span>
            </button>
          </div>
          <div v-if="formData.autoRenew">
            <label class="block text-content-80 text-sm font-medium mb-2">{{ t('apply.renewalDays') }}</label>
            <input v-model.number="formData.renewalDays" type="number" min="1" max="90" class="input w-32" />
          </div>
        </div>

        <!-- 手动 DNS TXT 记录信息 -->
        <div v-if="manualDNSChallenge" class="p-4 rounded-xl bg-info-soft border border-info-soft space-y-3">
          <p class="text-info font-medium">{{ t('apply.addTXT') }}</p>
          <div class="space-y-3">
            <div v-for="(record, index) in manualDNSChallenge.records" :key="index" class="space-y-2">
              <div class="flex items-center gap-2">
                <span class="text-content-50 text-xs">{{ t('apply.record') }} {{ index + 1 }}</span>
              </div>
              <div class="flex items-center gap-2">
                <span class="text-content-70 text-sm w-20">{{ t('apply.recordName') }}</span>
                <code class="px-2 py-1 rounded bg-base-300 text-base-content text-sm font-mono">{{ record.name }}</code>
                <button @click="copyToClipboard(record.name)" class="btn btn-ghost btn-xs" :title="t('common.copy')">
                  <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
                  </svg>
                </button>
              </div>
              <div class="flex items-center gap-2">
                <span class="text-content-70 text-sm w-20">{{ t('apply.recordValue') }}</span>
                <code class="px-2 py-1 rounded bg-base-300 text-base-content text-sm font-mono break-all">{{ record.value }}</code>
                <button @click="copyToClipboard(record.value)" class="btn btn-ghost btn-xs" :title="t('common.copy')">
                  <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
                  </svg>
                </button>
              </div>
              <div class="flex items-center gap-2">
                <button @click="verifyDNSOnline(record)" class="btn btn-ghost btn-xs">
                  <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14" />
                  </svg>
                  {{ t('apply.verifyOnline') }}
                </button>
              </div>
              <div v-if="index < manualDNSChallenge.records.length - 1" class="border-b border-info/20 my-2"></div>
            </div>
          </div>
          <p class="text-content-50 text-xs mt-2">{{ t('apply.verifyHint') }}</p>
        </div>

        <!-- 申请结果 -->
        <div v-if="applyResult && !manualDNSChallenge" class="p-4 rounded-xl" :class="applyResult.success ? 'bg-success-soft border border-success-soft' : 'bg-error-soft border border-error-soft'">
          <p :class="applyResult.success ? 'text-success' : 'text-error'" class="font-medium">{{ applyResult.message }}</p>
        </div>
      </div>

      <!-- 操作按钮 -->
      <div class="flex justify-between mt-8 pt-6 border-t border-base-300">
        <button v-if="currentStep > 1" @click="prevStep" class="btn btn-secondary" :disabled="isSubmitting || isVerifying">
          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 19l-7-7 7-7" />
          </svg>
          {{ t('apply.prevStep') }}
        </button>
        <div v-else></div>

        <button
          v-if="currentStep < totalSteps"
          @click="nextStep"
          :disabled="!canNext"
          class="btn btn-primary disabled:opacity-50 disabled:cursor-not-allowed"
        >
          {{ t('apply.nextStep') }}
          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
          </svg>
        </button>
        <button
          v-else-if="manualDNSChallenge"
          @click="verifyDNS"
          :disabled="isVerifying || verifyFailed"
          class="btn disabled:opacity-50 disabled:cursor-not-allowed"
          :class="verifyFailed ? 'btn-error' : 'btn-success'"
        >
          <svg v-if="isVerifying" class="w-4 h-4 animate-spin" fill="none" viewBox="0 0 24 24">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
          </svg>
          {{ isVerifying ? t('apply.verifying') : (verifyFailed ? t('apply.verifyFailedHint') : t('apply.verifyAndGet')) }}
        </button>
        <button
          v-else
          @click="submitApply"
          :disabled="isSubmitting"
          class="btn btn-primary disabled:opacity-50"
        >
          <svg v-if="isSubmitting" class="w-4 h-4 animate-spin" fill="none" viewBox="0 0 24 24">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
          </svg>
          {{ isSubmitting ? t('apply.applying') : t('apply.submit') }}
        </button>
      </div>
    </div>
  </div>
</template>
