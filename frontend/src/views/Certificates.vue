<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRouter } from 'vue-router'
import * as CertificateService from '@bindings/cnb.cool/dtapp/certflow/certificateservicewrapper'
import type { CertificateListItem } from '@bindings/cnb.cool/dtapp/certflow/models'
import { useI18n } from '../stores/i18n'
import { getStatusBadge, getDaysLeft, getDaysLeftClass } from '../utils/certificate'

const router = useRouter()
const { t } = useI18n()

const searchQuery = ref('')
const statusFilter = ref('all')
const certificates = ref<CertificateListItem[]>([])
const isLoading = ref(false)

onMounted(async () => {
  isLoading.value = true
  try {
    certificates.value = (await CertificateService.ListCertificates()) ?? []
  } catch (e) {
    console.error(t('certs.loadFailed'), e)
  } finally {
    isLoading.value = false
  }
})

const filteredCertificates = computed(() => {
  let result = certificates.value

  if (searchQuery.value) {
    const query = searchQuery.value.toLowerCase()
    result = result.filter(c =>
      c.domain.toLowerCase().includes(query) ||
      c.issuer.toLowerCase().includes(query)
    )
  }

  if (statusFilter.value !== 'all') {
    result = result.filter(c => c.status === statusFilter.value)
  }

  return result
})

const showDeleteModal = ref(false)
const deleteTargetId = ref<number | null>(null)

const openDeleteModal = (id: number) => {
  deleteTargetId.value = id
  showDeleteModal.value = true
}

const handleDelete = async () => {
  if (deleteTargetId.value === null) return
  const id = deleteTargetId.value
  showDeleteModal.value = false
  deleteTargetId.value = null
  try {
    await CertificateService.DeleteCertificate(id)
    certificates.value = certificates.value.filter(c => c.id !== id)
  } catch (e) {
    console.error(t('certs.deleteFailed'), e)
  }
}

const handleRetry = (cert: CertificateListItem) => {
  if (cert.status === 'pending') {
    router.push('/certificates/apply?certId=' + cert.id)
  } else if (cert.status === 'failed') {
    router.push('/certificates/apply?domain=' + encodeURIComponent(cert.domain))
  }
}
</script>

<template>
  <div class="page">
    <!-- 页面标题 -->
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-base-content">{{ t('certs.title') }}</h1>
        <p class="text-content-70 text-sm mt-1">{{ t('certs.subtitle') }}</p>
      </div>
      <button @click="router.push('/certificates/apply')" class="btn btn-primary">
        <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
        </svg>
        {{ t('certs.apply') }}
      </button>
    </div>

    <!-- 搜索和筛选 -->
    <div class="flex flex-col sm:flex-row gap-4">
      <div class="relative flex-1">
        <svg class="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-content-50" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
        </svg>
        <input
          v-model="searchQuery"
          type="text"
          :placeholder="t('certs.search')"
          class="input pl-10 w-full"
        />
      </div>
      <select v-model="statusFilter" class="select select-bordered w-auto">
        <option value="all">{{ t('certs.allStatus') }}</option>
        <option value="active">{{ t('certs.active') }}</option>
        <option value="pending">{{ t('certs.pending') }}</option>
        <option value="expired">{{ t('certs.expired') }}</option>
        <option value="revoked">{{ t('certs.revoked') }}</option>
        <option value="failed">{{ t('certs.failed') }}</option>
      </select>
    </div>

    <!-- 证书列表 -->
    <div class="glass-panel rounded-2xl overflow-hidden">
      <div v-if="isLoading" class="flex items-center justify-center py-20">
        <div class="spinner animate-spin"></div>
      </div>

      <div v-else-if="filteredCertificates.length === 0" class="text-center py-20">
        <svg class="w-20 h-20 mx-auto text-content-30 mb-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z" />
        </svg>
        <p class="text-content-70 text-lg">{{ t('certs.noRecords') }}</p>
        <p class="text-content-50 text-sm mt-2">{{ t('certs.noRecordsDesc') }}</p>
      </div>

      <table v-else class="w-full">
        <thead>
          <tr class="border-b border-base-300">
            <th class="text-left py-4 px-6 text-content-70 font-medium text-sm">{{ t('certs.domain') }}</th>
            <th class="text-left py-4 px-6 text-content-70 font-medium text-sm">{{ t('certs.issuer') }}</th>
            <th class="text-left py-4 px-6 text-content-70 font-medium text-sm">{{ t('certs.status') }}</th>
            <th class="text-left py-4 px-6 text-content-70 font-medium text-sm">{{ t('certs.daysLeft') }}</th>
            <th class="text-left py-4 px-6 text-content-70 font-medium text-sm">{{ t('certs.autoRenew') }}</th>
            <th class="text-right py-4 px-6 text-content-70 font-medium text-sm">{{ t('certs.actions') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="cert in filteredCertificates"
            :key="cert.id"
            class="border-b border-base-300-faint hover:bg-base-300-faint transition-colors cursor-pointer"
            @click="router.push('/certificates/' + cert.id)"
          >
            <td class="py-4 px-6">
              <div class="flex items-center gap-3">
                <div class="w-10 h-10 rounded-lg bg-primary-soft flex items-center justify-center">
                  <svg class="w-5 h-5 text-primary" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z" />
                  </svg>
                </div>
                <div>
                  <p class="text-base-content font-medium">{{ cert.domain }}</p>
                  <p v-if="cert.sans?.length" class="text-content-50 text-xs">
                    +{{ cert.sans.length }} {{ t('cert.san') }}
                  </p>
                </div>
              </div>
            </td>
            <td class="py-4 px-6 text-content-80">{{ cert.issuer }}</td>
            <td class="py-4 px-6">
              <div class="flex flex-col gap-1">
                <span
                  class="px-2.5 py-1 rounded-full text-xs font-medium border w-fit"
                  :class="getStatusBadge(cert.status).class"
                >
                  {{ getStatusBadge(cert.status).text }}
                </span>
                <span
                  v-if="cert.status === 'failed' && cert.last_error"
                  class="text-error text-xs truncate max-w-[200px]"
                  :title="cert.last_error"
                >
                  {{ cert.last_error }}
                </span>
              </div>
            </td>
            <td class="py-4 px-6">
              <span
                v-if="getDaysLeft(cert.not_after, cert.status) !== null"
                class="text-sm font-medium"
                :class="getDaysLeftClass(getDaysLeft(cert.not_after, cert.status))"
              >
                {{ getDaysLeft(cert.not_after, cert.status) }} {{ t('cert.daysLeft') }}
              </span>
              <span v-else class="text-content-50 text-sm">—</span>
            </td>
            <td class="py-4 px-6">
              <span v-if="cert.auto_renew" class="text-success text-sm flex items-center gap-1">
                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
                </svg>
                {{ t('certs.enabled') }}
              </span>
              <span v-else class="text-content-50 text-sm">{{ t('certs.disabled') }}</span>
            </td>
            <td class="py-4 px-6">
              <div class="flex items-center justify-end gap-2" @click.stop>
                <button
                  v-if="cert.status === 'pending'"
                  @click="handleRetry(cert)"
                  class="btn btn-xs btn-secondary"
                  :title="t('certs.continueApply')"
                >
                  {{ t('certs.continueApply') }}
                </button>
                <button
                  v-else-if="cert.status === 'failed'"
                  @click="handleRetry(cert)"
                  class="btn btn-xs btn-error"
                  :title="t('certs.retryApply')"
                >
                  {{ t('certs.retryApply') }}
                </button>
                <button
                  @click="openDeleteModal(cert.id)"
                  class="icon-btn icon-btn-danger"
                  :title="t('certs.delete')"
                >
                  <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                  </svg>
                </button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- 删除确认弹窗 -->
    <dialog v-if="showDeleteModal" class="modal modal-open">
      <div class="modal-box glass-panel">
        <h3 class="font-bold text-lg">{{ t('certs.delete') }}</h3>
        <p class="py-4">{{ t('certs.deleteConfirm') }}</p>
        <div class="modal-action">
          <button class="btn" @click="showDeleteModal = false">{{ t('common.cancel') }}</button>
          <button class="btn btn-error" @click="handleDelete">{{ t('common.confirm') }}</button>
        </div>
      </div>
      <form method="dialog" class="modal-backdrop">
        <button @click="showDeleteModal = false">close</button>
      </form>
    </dialog>
  </div>
</template>
