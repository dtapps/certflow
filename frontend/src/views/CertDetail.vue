<script setup lang="ts">
import { ref, onMounted, computed, h } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  NCard,
  NTabs,
  NTabPane,
  NButton,
  NTag,
  NSwitch,
  NInput,
  NSpin,
  NModal,
  NDescriptions,
  NDescriptionsItem,
  useMessage,
} from 'naive-ui'
import * as CertificateService from '@bindings/cnb.cool/dtapp/certflow/certificateservicewrapper'
import * as SchedulerService from '@bindings/cnb.cool/dtapp/certflow/schedulerservicewrapper'
import { copyToClipboard as copyText } from '../utils/clipboard'
import type { CertificateListItem, RenewalLogItem } from '@bindings/cnb.cool/dtapp/certflow/models'
import { useI18nStore } from '../stores/i18n'
import { getStatusBadge, getDaysLeft, getDaysLeftClass } from '../utils/certificate'
import { formatDateTime } from '../utils/format'
import { initMessage, showMessage } from '../utils/message'

const route = useRoute()
const router = useRouter()
const i18nStore = useI18nStore()
const { t } = i18nStore
const message = useMessage()
initMessage(message)

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
      console.debug(
        t('log.certDetailLoaded', {
          id: String(cert.id),
          notAfter: String(cert.not_after),
          notBefore: String(cert.not_before),
          createdAt: String(cert.created_at),
          status: String(cert.status),
        }),
      )
    } else {
      console.error(
        t('log.certDetailLoaded', {
          id: String(certId.value),
          notAfter: 'null',
          notBefore: 'null',
          createdAt: 'null',
          status: 'null',
        }),
      )
    }
    renewalLogs.value = logs ?? []
  } catch (e) {
    console.error(t('certDetail.loadFailed'), e)
  } finally {
    isLoading.value = false
  }
  loadCertDetails()
})

const daysLeft = computed(() => {
  if (!certificate.value?.not_after) return null
  const d = getDaysLeft(certificate.value.not_after, certificate.value.status)
  console.debug(
    t('log.certDetailDaysLeft', {
      notAfter: String(certificate.value?.not_after),
      status: String(certificate.value?.status),
      daysLeft: String(d),
    }),
  )
  return d
})

const handleRenew = async () => {
  try {
    await CertificateService.RenewCertificate(certId.value)
    certificate.value = await CertificateService.GetCertificateInfo(certId.value)
    showMessage(t('certDetail.renewSuccess'), 'success')
  } catch (e) {
    showMessage(t('certDetail.renewFailed') + ' ' + e, 'error')
  }
}

const showRevokeModal = ref(false)
const showDeleteModal = ref(false)

const handleRevoke = async () => {
  showRevokeModal.value = false
  try {
    await CertificateService.RevokeCertificate(certId.value)
    certificate.value = await CertificateService.GetCertificateInfo(certId.value)
    showMessage(t('certDetail.revokeSuccess'), 'success')
  } catch (e) {
    showMessage(t('certDetail.revokeFailed') + ' ' + e, 'error')
  }
}

const handleDelete = async () => {
  showDeleteModal.value = false
  try {
    await CertificateService.DeleteCertificate(certId.value)
    router.push('/certificates')
    showMessage(t('certDetail.deleteSuccess'), 'success')
  } catch (e) {
    showMessage(t('certDetail.deleteFailed') + ' ' + e, 'error')
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
  try {
    await CertificateService.UpdateCertificateSettings(
      certId.value,
      editAutoRenew.value,
      editRenewalDays.value,
    )
    certificate.value = await CertificateService.GetCertificateInfo(certId.value)
    editingSettings.value = false
    showMessage(t('certDetail.settingsSuccess'), 'success')
  } catch (e) {
    showMessage(t('certDetail.settingsFailed') + ' ' + e, 'error')
  }
}

const copyToClipboard = async (text: string, field: string) => {
  const ok = await copyText(text)
  if (ok) {
    copiedField.value = field
    setTimeout(() => {
      copiedField.value = ''
    }, 2000)
  }
}

const loadCertDetails = async () => {
  if (certDetails.value) return // 已加载过
  try {
    certDetails.value = await CertificateService.ParseCertificateDetails(certId.value)
  } catch (e) {
    console.error(t('certDetail.parseFailed'), e)
  }
}
</script>

<template>
  <n-spin :show="isLoading">
    <div v-if="certificate" class="page">
      <div class="flex items-center justify-between">
        <div class="flex items-center gap-4">
          <n-button quaternary circle @click="router.push('/certificates')">
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
          <div>
            <h1 class="text-2xl font-bold flex items-center gap-2">
              {{ certificate.domain }}
              <n-button text size="tiny" @click="copyToClipboard(certificate.domain, 'domain')">
                <template #icon>
                  <svg
                    v-if="copiedField !== 'domain'"
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
            </h1>
            <p class="text-sm mt-1 opacity-60">{{ t('cert.detail') }}</p>
          </div>
        </div>
        <div class="flex items-center gap-3">
          <n-button secondary @click="handleRenew">
            <template #icon>
              <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"
                />
              </svg>
            </template>
            {{ t('cert.renew') }}
          </n-button>
          <n-button type="error" @click="showRevokeModal = true">
            <template #icon>
              <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M18.364 18.364A9 9 0 005.636 5.636m12.728 12.728A9 9 0 015.636 5.636m12.728 12.728L5.636 5.636"
                />
              </svg>
            </template>
            {{ t('cert.revoke') }}
          </n-button>
          <n-button type="error" secondary @click="showDeleteModal = true">
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
            {{ t('certs.delete') }}
          </n-button>
        </div>
      </div>

      <div class="grid grid-cols-1 md:grid-cols-4 gap-4">
        <n-card size="small">
          <p class="text-sm opacity-60">{{ t('cert.status') }}</p>
          <n-tag
            :type="getStatusBadge(certificate.status).type as any"
            size="small"
            style="margin-top: 8px"
          >
            {{ getStatusBadge(certificate.status).text }}
          </n-tag>
        </n-card>
        <n-card size="small">
          <p class="text-sm opacity-60">{{ t('cert.daysLeft') }}</p>
          <p
            v-if="daysLeft !== null"
            class="text-2xl font-bold mt-1"
            :class="getDaysLeftClass(daysLeft)"
          >
            {{ daysLeft }}
          </p>
          <p v-else class="text-2xl font-bold mt-1 opacity-50">--</p>
        </n-card>
        <n-card size="small">
          <p class="text-sm opacity-60">{{ t('cert.issuer') }}</p>
          <p class="text-lg font-semibold mt-1">{{ certificate.issuer }}</p>
          <n-tag v-if="certificate.key_type" size="small" :bordered="false" style="margin-top: 4px">
            {{ certificate.key_type }}
          </n-tag>
        </n-card>
        <n-card size="small">
          <p class="text-sm opacity-60">{{ t('cert.autoRenew') }}</p>
          <div v-if="!editingSettings" class="flex items-center gap-2 mt-1">
            <p
              class="text-lg font-semibold"
              :class="certificate.auto_renew ? 'text-green-500' : 'opacity-50'"
            >
              {{ certificate.auto_renew ? t('cert.enabled') : t('cert.disabled') }}
            </p>
            <n-button text size="tiny" @click="startEditSettings">
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
            </n-button>
          </div>
          <div v-else class="space-y-3 mt-2">
            <div class="flex items-center gap-2">
              <n-switch v-model:value="editAutoRenew" size="small" />
              <span class="text-sm">{{ t('cert.enableAutoRenew') }}</span>
            </div>
            <div v-if="editAutoRenew" class="flex items-center gap-2">
              <span class="text-sm opacity-60">{{ t('cert.renewBeforeDays') }}</span>
              <n-input-number
                v-model:value="editRenewalDays"
                :min="1"
                :max="90"
                size="small"
                style="width: 64px"
              />
              <span class="text-sm opacity-60">{{ t('common.days') }}</span>
            </div>
            <div class="flex gap-2">
              <n-button size="tiny" type="primary" @click="saveSettings">{{
                t('common.save')
              }}</n-button>
              <n-button size="tiny" quaternary @click="cancelEditSettings">{{
                t('common.cancel')
              }}</n-button>
            </div>
          </div>
        </n-card>
      </div>

      <n-card size="small">
        <n-tabs v-model:value="activeTab" type="line">
          <n-tab-pane name="info" :tab="t('cert.certInfo')">
            <n-descriptions :column="2" bordered>
              <n-descriptions-item :label="t('cert.domain')">
                <div class="flex items-center gap-1">
                  {{ certificate.domain }}
                  <n-button
                    text
                    size="tiny"
                    @click="copyToClipboard(certificate.domain, 'info-domain')"
                  >
                    <template #icon>
                      <svg
                        v-if="copiedField !== 'info-domain'"
                        class="w-3.5 h-3.5"
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
                        class="w-3.5 h-3.5 text-green-500"
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
              </n-descriptions-item>
              <n-descriptions-item :label="t('cert.issuer')">{{
                certificate.issuer
              }}</n-descriptions-item>
              <n-descriptions-item :label="t('cert.effectiveTime')">{{
                formatDateTime(certificate.not_before)
              }}</n-descriptions-item>
              <n-descriptions-item :label="t('cert.expiryTime')">
                <span :class="daysLeft !== null && daysLeft <= 30 ? 'text-yellow-500' : ''">{{
                  formatDateTime(certificate.not_after)
                }}</span>
              </n-descriptions-item>
              <n-descriptions-item :label="t('cert.autoRenew')">
                {{
                  certificate.auto_renew
                    ? t('cert.renewalDays').replace('{days}', String(certificate.renewal_days))
                    : t('cert.disabled')
                }}
              </n-descriptions-item>
              <n-descriptions-item :label="t('cert.keyType')">
                <span class="font-mono">{{ certificate.key_type || 'EC256' }}</span>
              </n-descriptions-item>
              <n-descriptions-item v-if="certificate.ca_name" :label="t('cert.caName')">{{
                certificate.ca_name
              }}</n-descriptions-item>
              <n-descriptions-item
                v-if="certificate.dns_provider_name"
                :label="t('cert.dnsProvider')"
                >{{ certificate.dns_provider_name }}</n-descriptions-item
              >
              <n-descriptions-item :label="t('cert.createdAt')">{{
                formatDateTime(certificate.created_at)
              }}</n-descriptions-item>
              <n-descriptions-item :label="t('cert.updatedAt')">{{
                formatDateTime(certificate.updated_at)
              }}</n-descriptions-item>
              <n-descriptions-item
                v-if="certificate.last_renewed_at"
                :label="t('cert.lastRenewedAt')"
                >{{ formatDateTime(certificate.last_renewed_at) }}</n-descriptions-item
              >
            </n-descriptions>
          </n-tab-pane>

          <n-tab-pane name="sans" :tab="t('cert.domainList')">
            <div class="space-y-3">
              <div
                class="flex items-center gap-3 p-4 rounded-xl bg-blue-50 dark:bg-blue-900/30 border border-blue-200 dark:border-blue-800"
              >
                <div
                  class="w-10 h-10 rounded-lg bg-blue-100 dark:bg-blue-800 flex items-center justify-center"
                >
                  <svg
                    class="w-5 h-5 text-blue-500"
                    fill="none"
                    stroke="currentColor"
                    viewBox="0 0 24 24"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      stroke-width="2"
                      d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z"
                    />
                  </svg>
                </div>
                <div>
                  <p class="font-medium flex items-center gap-1">
                    {{ certificate.domain }}
                    <n-button
                      text
                      size="tiny"
                      @click="copyToClipboard(certificate.domain, 'san-main')"
                    >
                      <template #icon>
                        <svg
                          v-if="copiedField !== 'san-main'"
                          class="w-3.5 h-3.5"
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
                          class="w-3.5 h-3.5 text-green-500"
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
                  </p>
                  <p class="text-xs opacity-50">{{ t('cert.mainDomain') }}</p>
                </div>
              </div>
              <div
                v-for="san in certificate.sans"
                :key="san"
                class="flex items-center gap-3 p-4 rounded-xl bg-neutral-50 dark:bg-neutral-800/50"
              >
                <div
                  class="w-10 h-10 rounded-lg bg-neutral-100 dark:bg-neutral-700 flex items-center justify-center"
                >
                  <svg
                    class="w-5 h-5 opacity-50"
                    fill="none"
                    stroke="currentColor"
                    viewBox="0 0 24 24"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      stroke-width="2"
                      d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z"
                    />
                  </svg>
                </div>
                <div>
                  <p class="font-medium flex items-center gap-1">
                    {{ san }}
                    <n-button text size="tiny" @click="copyToClipboard(san, 'san-' + san)">
                      <template #icon>
                        <svg
                          v-if="copiedField !== 'san-' + san"
                          class="w-3.5 h-3.5"
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
                          class="w-3.5 h-3.5 text-green-500"
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
                  </p>
                  <p class="text-xs opacity-50">{{ t('cert.san') }}</p>
                </div>
              </div>
            </div>
          </n-tab-pane>

          <n-tab-pane name="pem" tab="PEM">
            <div class="space-y-4">
              <div>
                <div class="flex items-center justify-between mb-2">
                  <p class="text-sm font-medium">{{ t('cert.certificate') }}</p>
                  <n-button
                    text
                    size="tiny"
                    @click="copyToClipboard(certificate?.cert_content || '', 'cert')"
                  >
                    <template #icon>
                      <svg
                        v-if="copiedField !== 'cert'"
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
                  class="p-4 rounded-xl bg-neutral-50 dark:bg-neutral-800/50 text-xs font-mono overflow-x-auto max-h-48"
                  >{{ certificate?.cert_content }}</pre>
              </div>
              <div>
                <div class="flex items-center justify-between mb-2">
                  <p class="text-sm font-medium">{{ t('cert.privateKey') }}</p>
                  <n-button
                    text
                    size="tiny"
                    @click="copyToClipboard(certificate?.key_content || '', 'key')"
                  >
                    <template #icon>
                      <svg
                        v-if="copiedField !== 'key'"
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
                  class="p-4 rounded-xl bg-neutral-50 dark:bg-neutral-800/50 text-xs font-mono overflow-x-auto max-h-48"
                  >{{ certificate.key_content }}</pre>
              </div>
            </div>
          </n-tab-pane>

          <n-tab-pane name="logs" :tab="t('cert.renewalLogs')">
            <div v-if="renewalLogs.length === 0" class="text-center py-12 opacity-50">
              {{ t('cert.noLogs') }}
            </div>
            <div v-else class="space-y-3">
              <div
                v-for="log in renewalLogs"
                :key="log.id"
                class="flex items-center justify-between p-4 rounded-xl bg-neutral-50 dark:bg-neutral-800/50"
              >
                <div class="flex items-center gap-3">
                  <n-tag
                    :type="
                      log.status === 'success'
                        ? 'success'
                        : log.status === 'failed'
                          ? 'error'
                          : 'warning'
                    "
                    size="small"
                    :bordered="false"
                  >
                    {{
                      log.status === 'success'
                        ? t('cert.renewSuccess')
                        : log.status === 'failed'
                          ? t('cert.renewFailed')
                          : t('cert.renewing')
                    }}
                  </n-tag>
                  <p v-if="log.error_message" class="text-red-500 text-xs">
                    {{ log.error_message }}
                  </p>
                </div>
                <span class="text-xs opacity-50">{{ formatDateTime(log.attempt_at) }}</span>
              </div>
            </div>
          </n-tab-pane>

          <n-tab-pane name="details" :tab="t('cert.certDetails')">
            <div v-if="!certDetails" class="flex items-center justify-center py-8">
              <n-spin size="medium" />
            </div>
            <n-descriptions v-else :column="2" bordered>
              <n-descriptions-item :label="t('cert.detail.serialNumber')" :span="2">
                <span class="font-mono text-sm break-all">{{ certDetails.serial_number }}</span>
              </n-descriptions-item>
              <n-descriptions-item :label="t('cert.detail.signatureAlgo')">
                <span class="font-mono text-sm">{{ certDetails.signature_algorithm }}</span>
              </n-descriptions-item>
              <n-descriptions-item :label="t('cert.detail.publicKeyAlgo')">
                <span class="font-mono text-sm"
                  >{{ certDetails.public_key_algorithm }} {{ certDetails.public_key_size }}bit</span
                >
              </n-descriptions-item>
              <n-descriptions-item :label="t('cert.detail.fingerprint') + ' (SHA-256)'" :span="2">
                <div class="flex items-center gap-2">
                  <span class="font-mono text-xs break-all">{{
                    certDetails.fingerprint_sha256
                  }}</span>
                  <n-button
                    text
                    size="tiny"
                    @click="copyToClipboard(certDetails.fingerprint_sha256, 'fingerprint')"
                  >
                    <template #icon>
                      <svg
                        v-if="copiedField !== 'fingerprint'"
                        class="w-3.5 h-3.5"
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
                        class="w-3.5 h-3.5 text-green-500"
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
              </n-descriptions-item>
              <n-descriptions-item :label="t('cert.detail.keyUsage')">{{
                certDetails.key_usage || '—'
              }}</n-descriptions-item>
              <n-descriptions-item :label="t('cert.detail.extKeyUsage')">{{
                certDetails.ext_key_usage || '—'
              }}</n-descriptions-item>
              <n-descriptions-item :label="t('cert.detail.isCA')">{{
                certDetails.is_ca ? t('cert.detail.yes') : t('cert.detail.no')
              }}</n-descriptions-item>
              <n-descriptions-item :label="t('cert.detail.version')"
                >v{{ certDetails.version + 1 }}</n-descriptions-item
              >
              <n-descriptions-item
                v-if="certDetails.dns_names?.length"
                :label="t('cert.detail.dnsNames')"
                :span="2"
              >
                <div class="flex flex-wrap gap-2">
                  <n-tag
                    v-for="name in certDetails.dns_names"
                    :key="name"
                    size="small"
                    :bordered="false"
                    >{{ name }}</n-tag
                  >
                </div>
              </n-descriptions-item>
              <n-descriptions-item
                v-if="certDetails.ip_addresses?.length"
                :label="t('cert.detail.ipAddresses')"
                :span="2"
              >
                <div class="flex flex-wrap gap-2">
                  <n-tag
                    v-for="ip in certDetails.ip_addresses"
                    :key="ip"
                    size="small"
                    :bordered="false"
                    >{{ ip }}</n-tag
                  >
                </div>
              </n-descriptions-item>
              <n-descriptions-item
                v-if="certDetails.email_addresses?.length"
                :label="t('cert.detail.emailAddresses')"
                :span="2"
              >
                <div class="flex flex-wrap gap-2">
                  <n-tag
                    v-for="email in certDetails.email_addresses"
                    :key="email"
                    size="small"
                    :bordered="false"
                    >{{ email }}</n-tag
                  >
                </div>
              </n-descriptions-item>
            </n-descriptions>
          </n-tab-pane>
        </n-tabs>
      </n-card>
    </div>
  </n-spin>

  <!-- 吊销确认弹窗 -->
  <n-modal v-model:show="showRevokeModal" preset="dialog" :title="t('cert.revoke')">
    <p>{{ t('cert.revokeConfirm') }}</p>
    <template #action>
      <n-button @click="showRevokeModal = false">{{ t('common.cancel') }}</n-button>
      <n-button type="error" @click="handleRevoke">{{ t('common.confirm') }}</n-button>
    </template>
  </n-modal>

  <!-- 删除确认弹窗 -->
  <n-modal v-model:show="showDeleteModal" preset="dialog" :title="t('certs.delete')">
    <p>{{ t('cert.deleteConfirm') }}</p>
    <template #action>
      <n-button @click="showDeleteModal = false">{{ t('common.cancel') }}</n-button>
      <n-button type="error" @click="handleDelete">{{ t('common.confirm') }}</n-button>
    </template>
  </n-modal>
</template>
