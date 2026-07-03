<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import * as CertificateService from '@bindings/cnb.cool/dtapp/certflow/certificateservicewrapper'
import * as SchedulerService from '@bindings/cnb.cool/dtapp/certflow/schedulerservicewrapper'
import type { CertificateListItem, RenewalLogItem } from '@bindings/cnb.cool/dtapp/certflow/models'
import { useI18n } from '../stores/i18n'
import { getStatusBadge, getDaysLeft, getDaysLeftClass } from '../utils/certificate'
import { formatDateTime } from '../utils/format'

const route = useRoute()
const router = useRouter()
const { t } = useI18n()

const certId = computed(() => Number(route.params.id))

const certificate = ref<CertificateListItem | null>(null)
const renewalLogs = ref<RenewalLogItem[]>([])
const certDetails = ref<any>(null)
const isLoading = ref(true)
const activeTab = ref('info')
const editingSettings = ref(false)
const editAutoRenew = ref(false)
const editRenewalDays = ref(30)
const copiedField = ref('')

onMounted(async () => {
  try {
    const [cert, logs] = await Promise.all([
      CertificateService.GetCertificateInfo(certId.value),
      SchedulerService.GetRenewalLogs(certId.value),
    ])
    if (cert) {
      certificate.value = cert
      editAutoRenew.value = cert.auto_renew
      editRenewalDays.value = cert.renewal_days
    }
    renewalLogs.value = logs ?? []
  } catch (e) {
    console.error('Failed to load certificate details:', e)
  } finally {
    isLoading.value = false
  }
})

const daysLeft = computed(() => {
  if (!certificate.value?.not_after) return null
  return getDaysLeft(certificate.value.not_after, certificate.value.status)
})

const handleRenew = async () => {
  await CertificateService.RenewCertificate(certId.value)
  certificate.value = await CertificateService.GetCertificateInfo(certId.value)
}

const showRevokeModal = ref(false)
const showDeleteModal = ref(false)

const handleRevoke = async () => {
  showRevokeModal.value = false
  try {
    await CertificateService.RevokeCertificate(certId.value)
    certificate.value = await CertificateService.GetCertificateInfo(certId.value)
  } catch (e) {
    console.error('Failed to revoke certificate', e)
  }
}

const handleDelete = async () => {
  showDeleteModal.value = false
  try {
    await CertificateService.DeleteCertificate(certId.value)
    router.push('/certificates')
  } catch (e) {
    console.error('Failed to delete certificate', e)
  }
}

const startEditSettings = () => {
  editingSettings.value = true
  editAutoRenew.value = certificate.value!.auto_renew
  editRenewalDays.value = certificate.value!.renewal_days
}

const cancelEditSettings = () => {
  editingSettings.value = false
}

const saveSettings = async () => {
  await CertificateService.UpdateCertificateSettings(certId.value, editAutoRenew.value, editRenewalDays.value)
  certificate.value = await CertificateService.GetCertificateInfo(certId.value)
  editingSettings.value = false
}

const copyToClipboard = async (text: string, field: string) => {
  await navigator.clipboard.writeText(text)
  copiedField.value = field
  setTimeout(() => { copiedField.value = '' }, 2000)
}

const loadCertDetails = async () => {
  if (certDetails.value) return // 已加载过
  try {
    certDetails.value = await CertificateService.ParseCertificateDetails(certId.value)
  } catch (e) {
    console.error('Failed to parse certificate details', e)
  }
}
</script>

<template>
  <div v-if="isLoading" class="flex items-center justify-center py-20">
    <div class="spinner animate-spin"></div>
  </div>

  <div v-if="certificate" class="page">
    <div class="flex items-center justify-between">
      <div class="flex items-center gap-4">
        <button @click="router.push('/certificates')" class="icon-btn">
          <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 19l-7-7 7-7" />
          </svg>
        </button>
        <div>
          <h1 class="text-2xl font-bold text-base-content flex items-center gap-2">
            {{ certificate.domain }}
            <button @click="copyToClipboard(certificate.domain, 'domain')" class="icon-btn text-content-50 hover:text-primary" :title="copiedField === 'domain' ? t('cert.copied') : t('cert.copy')">
              <svg v-if="copiedField !== 'domain'" class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" /></svg>
              <svg v-else class="w-4 h-4 text-success" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" /></svg>
            </button>
          </h1>
          <p class="text-content-70 text-sm mt-1">{{ t('cert.detail') }}</p>
        </div>
      </div>
      <div class="flex items-center gap-3">
        <button @click="handleRenew" class="btn btn-secondary">
          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" /></svg>
          {{ t('cert.renew') }}
        </button>
        <button @click="showRevokeModal = true" class="btn btn-error">
          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M18.364 18.364A9 9 0 005.636 5.636m12.728 12.728A9 9 0 015.636 5.636m12.728 12.728L5.636 5.636" /></svg>
          {{ t('cert.revoke') }}
        </button>
        <button @click="showDeleteModal = true" class="btn btn-error btn-outline">
          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" /></svg>
          {{ t('certs.delete') }}
        </button>
      </div>
    </div>

    <div class="grid grid-cols-1 md:grid-cols-4 gap-4">
      <div class="stat-card">
        <p class="text-content-70 text-sm">{{ t('cert.status') }}</p>
        <span class="inline-flex px-2.5 py-1 rounded-full text-xs font-medium border mt-2" :class="getStatusBadge(certificate.status).class">{{ getStatusBadge(certificate.status).text }}</span>
      </div>
      <div class="stat-card">
        <p class="text-content-70 text-sm">{{ t('cert.daysLeft') }}</p>
        <p v-if="daysLeft !== null" class="text-2xl font-bold mt-1" :class="getDaysLeftClass(daysLeft)">{{ daysLeft }} {{ t('common.daysLeft') }}</p>
        <p v-else class="text-2xl font-bold mt-1 text-content-50">--</p>
      </div>
      <div class="stat-card">
        <p class="text-content-70 text-sm">{{ t('cert.issuer') }}</p>
        <p class="text-lg font-semibold text-base-content mt-1">{{ certificate.issuer }}</p>
        <span v-if="certificate.key_type" class="px-2 py-0.5 rounded-full text-[10px] font-mono bg-accent-soft text-accent border border-accent-soft mt-1 inline-block">{{ certificate.key_type }}</span>
      </div>
      <div class="stat-card">
        <p class="text-content-70 text-sm">{{ t('cert.autoRenew') }}</p>
        <div v-if="!editingSettings" class="flex items-center gap-2 mt-1">
          <p class="text-lg font-semibold" :class="certificate.auto_renew ? 'text-success' : 'text-content-50'">{{ certificate.auto_renew ? t('cert.enabled') : t('cert.disabled') }}</p>
          <button @click="startEditSettings" class="icon-btn" :title="t('cert.edit')">
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" /></svg>
          </button>
        </div>
        <div v-else class="space-y-3 mt-2">
          <label class="flex items-center gap-2 cursor-pointer">
            <input type="checkbox" v-model="editAutoRenew" class="checkbox checkbox-primary checkbox-sm" />
            <span class="text-sm text-base-content">{{ t('cert.enableAutoRenew') }}</span>
          </label>
          <div v-if="editAutoRenew" class="flex items-center gap-2">
            <span class="text-sm text-content-70">{{ t('cert.renewBeforeDays') }}</span>
            <input v-model.number="editRenewalDays" type="number" min="1" max="90" class="input input-sm w-16 text-center" />
            <span class="text-sm text-content-70">{{ t('common.days') }}</span>
          </div>
          <div class="flex gap-2">
            <button @click="saveSettings" class="btn btn-primary btn-xs">{{ t('common.save') }}</button>
            <button @click="cancelEditSettings" class="btn btn-ghost btn-xs">{{ t('common.cancel') }}</button>
          </div>
        </div>
      </div>
    </div>

    <div class="glass-panel rounded-2xl overflow-hidden">
      <div class="flex border-b border-base-300">
        <button @click="activeTab = 'info'" class="px-6 py-3 text-sm font-medium transition-colors" :class="activeTab === 'info' ? 'text-primary border-b-2 border-primary' : 'text-content-70 hover:text-base-content'">{{ t('cert.certInfo') }}</button>
        <button @click="activeTab = 'sans'" class="px-6 py-3 text-sm font-medium transition-colors" :class="activeTab === 'sans' ? 'text-primary border-b-2 border-primary' : 'text-content-70 hover:text-base-content'">{{ t('cert.domainList') }}</button>
        <button @click="activeTab = 'pem'" class="px-6 py-3 text-sm font-medium transition-colors" :class="activeTab === 'pem' ? 'text-primary border-b-2 border-primary' : 'text-content-70 hover:text-base-content'">PEM</button>
        <button @click="activeTab = 'logs'" class="px-6 py-3 text-sm font-medium transition-colors" :class="activeTab === 'logs' ? 'text-primary border-b-2 border-primary' : 'text-content-70 hover:text-base-content'">{{ t('cert.renewalLogs') }}</button>
        <button @click="activeTab = 'details'; loadCertDetails()" class="px-6 py-3 text-sm font-medium transition-colors" :class="activeTab === 'details' ? 'text-primary border-b-2 border-primary' : 'text-content-70 hover:text-base-content'">{{ t('cert.certDetails') }}</button>
      </div>

      <div class="p-6">
        <div v-if="activeTab === 'info'" class="grid grid-cols-2 gap-4">
          <div class="p-4 rounded-xl bg-base-200-faint">
            <p class="text-content-50 text-xs">{{ t('cert.domain') }}</p>
            <div class="flex items-center gap-1 mt-1">
              <p class="text-base-content font-medium">{{ certificate.domain }}</p>
              <button @click="copyToClipboard(certificate.domain, 'info-domain')" class="icon-btn text-content-50 hover:text-primary" :title="copiedField === 'info-domain' ? t('cert.copied') : t('cert.copy')">
                <svg v-if="copiedField !== 'info-domain'" class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" /></svg>
                <svg v-else class="w-3.5 h-3.5 text-success" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" /></svg>
              </button>
            </div>
          </div>
          <div class="p-4 rounded-xl bg-base-200-faint">
            <p class="text-content-50 text-xs">{{ t('cert.issuer') }}</p>
            <div class="flex items-center gap-1 mt-1">
              <p class="text-base-content font-medium">{{ certificate.issuer }}</p>
              <button @click="copyToClipboard(certificate.issuer, 'info-issuer')" class="icon-btn text-content-50 hover:text-primary" :title="copiedField === 'info-issuer' ? t('cert.copied') : t('cert.copy')">
                <svg v-if="copiedField !== 'info-issuer'" class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" /></svg>
                <svg v-else class="w-3.5 h-3.5 text-success" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" /></svg>
              </button>
            </div>
          </div>
          <div class="p-4 rounded-xl bg-base-200-faint"><p class="text-content-50 text-xs">{{ t('cert.effectiveTime') }}</p><p class="text-base-content font-medium mt-1">{{ formatDateTime(certificate.not_before) }}</p></div>
          <div class="p-4 rounded-xl bg-base-200-faint"><p class="text-content-50 text-xs">{{ t('cert.expiryTime') }}</p><p class="text-base-content font-medium mt-1" :class="daysLeft !== null && daysLeft <= 30 ? 'text-warning' : ''">{{ formatDateTime(certificate.not_after) }}</p></div>
          <div class="p-4 rounded-xl bg-base-200-faint"><p class="text-content-50 text-xs">{{ t('cert.autoRenew') }}</p><p class="text-base-content font-medium mt-1">{{ certificate.auto_renew ? t('cert.renewalDays').replace('{days}', String(certificate.renewal_days)) : t('cert.disabled') }}</p></div>
          <div class="p-4 rounded-xl bg-base-200-faint"><p class="text-content-50 text-xs">{{ t('cert.keyType') }}</p><p class="text-base-content font-medium font-mono mt-1">{{ certificate.key_type || 'EC256' }}</p></div>
          <div v-if="certificate.ca_name" class="p-4 rounded-xl bg-base-200-faint"><p class="text-content-50 text-xs">{{ t('cert.caName') }}</p><p class="text-base-content font-medium mt-1">{{ certificate.ca_name }}</p></div>
          <div v-if="certificate.dns_provider_name" class="p-4 rounded-xl bg-base-200-faint"><p class="text-content-50 text-xs">{{ t('cert.dnsProvider') }}</p><p class="text-base-content font-medium mt-1">{{ certificate.dns_provider_name }}</p></div>
          <div class="p-4 rounded-xl bg-base-200-faint"><p class="text-content-50 text-xs">{{ t('cert.createdAt') }}</p><p class="text-base-content font-medium mt-1">{{ formatDateTime(certificate.created_at) }}</p></div>
          <div class="p-4 rounded-xl bg-base-200-faint"><p class="text-content-50 text-xs">{{ t('cert.updatedAt') }}</p><p class="text-base-content font-medium mt-1">{{ formatDateTime(certificate.updated_at) }}</p></div>
          <div v-if="certificate.last_renewed_at" class="p-4 rounded-xl bg-base-200-faint"><p class="text-content-50 text-xs">{{ t('cert.lastRenewedAt') }}</p><p class="text-base-content font-medium mt-1">{{ formatDateTime(certificate.last_renewed_at) }}</p></div>
        </div>

        <div v-if="activeTab === 'sans'" class="space-y-3">
          <div class="flex items-center gap-3 p-4 rounded-xl bg-primary-faint border border-primary-soft">
            <div class="w-10 h-10 rounded-lg bg-primary-soft flex items-center justify-center"><svg class="w-5 h-5 text-primary" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z" /></svg></div>
            <div><p class="text-base-content font-medium flex items-center gap-1">{{ certificate.domain }}
              <button @click="copyToClipboard(certificate.domain, 'san-main')" class="icon-btn text-content-50 hover:text-primary">
                <svg v-if="copiedField !== 'san-main'" class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" /></svg>
                <svg v-else class="w-3.5 h-3.5 text-success" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" /></svg>
              </button>
            </p><p class="text-content-50 text-xs">{{ t('cert.mainDomain') }}</p></div>
          </div>
          <div v-for="san in certificate.sans" :key="san" class="flex items-center gap-3 p-4 rounded-xl bg-base-200-faint">
            <div class="w-10 h-10 rounded-lg bg-base-300 flex items-center justify-center"><svg class="w-5 h-5 text-content-70" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z" /></svg></div>
            <div><p class="text-base-content font-medium flex items-center gap-1">{{ san }}
              <button @click="copyToClipboard(san, 'san-' + san)" class="icon-btn text-content-50 hover:text-primary">
                <svg v-if="copiedField !== 'san-' + san" class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" /></svg>
                <svg v-else class="w-3.5 h-3.5 text-success" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" /></svg>
              </button>
            </p><p class="text-content-50 text-xs">{{ t('cert.san') }}</p></div>
          </div>
        </div>

        <div v-if="activeTab === 'logs'" class="space-y-3">
          <div v-if="renewalLogs.length === 0" class="text-center py-12">
            <svg class="w-16 h-16 mx-auto text-content-30 mb-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" /></svg>
            <p class="text-content-70">{{ t('cert.noLogs') }}</p>
          </div>
          <div v-for="log in renewalLogs" :key="log.id" class="flex items-center justify-between p-4 rounded-xl bg-base-200-faint">
            <div class="flex items-center gap-3">
              <div class="w-2 h-2 rounded-full" :class="log.status === 'success' ? 'bg-success' : log.status === 'failed' ? 'bg-error' : 'bg-warning'"></div>
              <div>
                <p class="text-base-content text-sm font-medium">{{ log.status === 'success' ? t('cert.renewSuccess') : log.status === 'failed' ? t('cert.renewFailed') : t('cert.renewing') }}</p>
                <p v-if="log.error_message" class="text-error text-xs">{{ log.error_message }}</p>
              </div>
            </div>
            <span class="text-content-50 text-xs">{{ formatDateTime(log.attempt_at) }}</span>
          </div>
        </div>

        <div v-if="activeTab === 'pem'" class="space-y-4">
          <div class="relative">
            <div class="flex items-center justify-between mb-2">
              <p class="text-sm font-medium text-base-content">{{ t('cert.certificate') }}</p>
              <button @click="copyToClipboard(certificate?.cert_content || '', 'cert')" class="icon-btn text-content-50 hover:text-primary" :title="copiedField === 'cert' ? t('cert.copied') : t('cert.copy')">
                <svg v-if="copiedField !== 'cert'" class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" /></svg>
                <svg v-else class="w-4 h-4 text-success" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" /></svg>
              </button>
            </div>
            <pre class="p-4 rounded-xl bg-base-200-faint text-xs text-content-70 font-mono overflow-x-auto max-h-48">{{ certificate?.cert_content }}</pre>
          </div>
          <div class="relative">
            <div class="flex items-center justify-between mb-2">
              <p class="text-sm font-medium text-base-content">{{ t('cert.privateKey') }}</p>
              <button @click="copyToClipboard(certificate?.key_content || '', 'key')" class="icon-btn text-content-50 hover:text-primary" :title="copiedField === 'key' ? t('cert.copied') : t('cert.copy')">
                <svg v-if="copiedField !== 'key'" class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" /></svg>
                <svg v-else class="w-4 h-4 text-success" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" /></svg>
              </button>
            </div>
            <pre class="p-4 rounded-xl bg-base-200-faint text-xs text-content-70 font-mono overflow-x-auto max-h-48">{{ certificate.key_content }}</pre>
          </div>
        </div>

        <!-- 证书详情 tab (x509 解析) -->
        <div v-if="activeTab === 'details'" class="space-y-4">
          <div v-if="!certDetails" class="flex items-center justify-center py-8">
            <div class="spinner animate-spin"></div>
          </div>
          <div v-else class="grid grid-cols-2 gap-4">
            <div class="p-4 rounded-xl bg-base-200-faint col-span-2">
              <p class="text-content-50 text-xs">{{ t('cert.detail.serialNumber') }}</p>
              <p class="text-base-content font-medium font-mono text-sm mt-1 break-all">{{ certDetails.serial_number }}</p>
            </div>
            <div class="p-4 rounded-xl bg-base-200-faint">
              <p class="text-content-50 text-xs">{{ t('cert.detail.signatureAlgo') }}</p>
              <p class="text-base-content font-medium font-mono text-sm mt-1">{{ certDetails.signature_algorithm }}</p>
            </div>
            <div class="p-4 rounded-xl bg-base-200-faint">
              <p class="text-content-50 text-xs">{{ t('cert.detail.publicKeyAlgo') }}</p>
              <p class="text-base-content font-medium font-mono text-sm mt-1">{{ certDetails.public_key_algorithm }} {{ certDetails.public_key_size }}bit</p>
            </div>
            <div class="p-4 rounded-xl bg-base-200-faint col-span-2">
              <p class="text-content-50 text-xs">{{ t('cert.detail.fingerprint') }} (SHA-256)</p>
              <p class="text-base-content font-medium font-mono text-xs mt-1 break-all">{{ certDetails.fingerprint_sha256 }}</p>
              <button @click="copyToClipboard(certDetails.fingerprint_sha256, 'fingerprint')" class="icon-btn text-content-50 hover:text-primary mt-1" :title="copiedField === 'fingerprint' ? t('cert.copied') : t('cert.copy')">
                <svg v-if="copiedField !== 'fingerprint'" class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" /></svg>
                <svg v-else class="w-3.5 h-3.5 text-success" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" /></svg>
              </button>
            </div>
            <div class="p-4 rounded-xl bg-base-200-faint">
              <p class="text-content-50 text-xs">{{ t('cert.detail.keyUsage') }}</p>
              <p class="text-base-content text-sm mt-1">{{ certDetails.key_usage || '—' }}</p>
            </div>
            <div class="p-4 rounded-xl bg-base-200-faint">
              <p class="text-content-50 text-xs">{{ t('cert.detail.extKeyUsage') }}</p>
              <p class="text-base-content text-sm mt-1">{{ certDetails.ext_key_usage || '—' }}</p>
            </div>
            <div class="p-4 rounded-xl bg-base-200-faint">
              <p class="text-content-50 text-xs">{{ t('cert.detail.isCA') }}</p>
              <p class="text-base-content font-medium text-sm mt-1">{{ certDetails.is_ca ? t('cert.detail.yes') : t('cert.detail.no') }}</p>
            </div>
            <div class="p-4 rounded-xl bg-base-200-faint">
              <p class="text-content-50 text-xs">{{ t('cert.detail.version') }}</p>
              <p class="text-base-content font-medium text-sm mt-1">v{{ certDetails.version + 1 }}</p>
            </div>
            <div v-if="certDetails.dns_names?.length" class="p-4 rounded-xl bg-base-200-faint col-span-2">
              <p class="text-content-50 text-xs">{{ t('cert.detail.dnsNames') }}</p>
              <div class="flex flex-wrap gap-2 mt-1">
                <span v-for="name in certDetails.dns_names" :key="name" class="px-2 py-0.5 rounded bg-primary-soft text-primary text-xs font-mono">{{ name }}</span>
              </div>
            </div>
            <div v-if="certDetails.ip_addresses?.length" class="p-4 rounded-xl bg-base-200-faint col-span-2">
              <p class="text-content-50 text-xs">{{ t('cert.detail.ipAddresses') }}</p>
              <div class="flex flex-wrap gap-2 mt-1">
                <span v-for="ip in certDetails.ip_addresses" :key="ip" class="px-2 py-0.5 rounded bg-base-300 text-content-80 text-xs font-mono">{{ ip }}</span>
              </div>
            </div>
            <div v-if="certDetails.email_addresses?.length" class="p-4 rounded-xl bg-base-200-faint col-span-2">
              <p class="text-content-50 text-xs">{{ t('cert.detail.emailAddresses') }}</p>
              <div class="flex flex-wrap gap-2 mt-1">
                <span v-for="email in certDetails.email_addresses" :key="email" class="px-2 py-0.5 rounded bg-base-300 text-content-80 text-xs font-mono">{{ email }}</span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>

    <!-- 吊销确认弹窗 -->
    <dialog v-if="showRevokeModal" class="modal modal-open">
      <div class="modal-box glass-panel">
        <h3 class="font-bold text-lg">{{ t('cert.revoke') }}</h3>
        <p class="py-4">{{ t('cert.revokeConfirm') }}</p>
        <div class="modal-action">
          <button class="btn" @click="showRevokeModal = false">{{ t('common.cancel') }}</button>
          <button class="btn btn-error" @click="handleRevoke">{{ t('common.confirm') }}</button>
        </div>
      </div>
      <form method="dialog" class="modal-backdrop">
        <button @click="showRevokeModal = false">close</button>
      </form>
    </dialog>

    <!-- 删除确认弹窗 -->
    <dialog v-if="showDeleteModal" class="modal modal-open">
      <div class="modal-box glass-panel">
        <h3 class="font-bold text-lg">{{ t('certs.delete') }}</h3>
        <p class="py-4">{{ t('cert.deleteConfirm') }}</p>
        <div class="modal-action">
          <button class="btn" @click="showDeleteModal = false">{{ t('common.cancel') }}</button>
          <button class="btn btn-error" @click="handleDelete">{{ t('common.confirm') }}</button>
        </div>
      </div>
      <form method="dialog" class="modal-backdrop">
        <button @click="showDeleteModal = false">close</button>
      </form>
    </dialog>
</template>
