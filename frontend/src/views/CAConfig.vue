<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import * as CAService from '@bindings/cnb.cool/dtapp/certflow/caservicewrapper'
import type { CAListItem } from '@bindings/cnb.cool/dtapp/certflow/models'
import { useI18n } from '../stores/i18n'

const { t } = useI18n()

const cas = ref<CAListItem[]>([])
const isLoading = ref(false)
const showModal = ref(false)
const editingCA = ref<number | null>(null)

const formData = ref({
  name: '', directory_url: '', account_email: '', is_default: false, is_active: true,
})

onMounted(async () => {
  isLoading.value = true
  try {
    cas.value = (await CAService.ListCA()) ?? []
  } catch (e) {
    console.error(t('ca.loadFailed'), e)
  } finally {
    isLoading.value = false
  }
})

const openCreate = () => {
  editingCA.value = null
  formData.value = { name: '', directory_url: '', account_email: '', is_default: false, is_active: true }
  showModal.value = true
}

const openEdit = (ca: typeof cas.value[0]) => {
  editingCA.value = ca.id
  formData.value = { name: ca.name, directory_url: ca.directory_url, account_email: ca.account_email, is_default: ca.is_default, is_active: ca.is_active }
  showModal.value = true
}

const handleSave = async () => {
  if (editingCA.value) {
    await CAService.UpdateCA(editingCA.value, formData.value)
  } else {
    await CAService.CreateCA(formData.value)
  }
  showModal.value = false
  cas.value = (await CAService.ListCA()) ?? []
}

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
    await CAService.DeleteCA(id)
    cas.value = cas.value.filter(c => c.id !== id)
  } catch (e) {
    console.error('Failed to delete CA', e)
  }
}

const handleSetDefault = async (id: number) => {
  await CAService.SetDefaultCA(id)
  cas.value = (await CAService.ListCA()) ?? []
}
</script>

<template>
  <div class="page">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-base-content">{{ t('ca.title') }}</h1>
        <p class="text-content-70 text-sm mt-1">{{ t('ca.subtitle') }}</p>
      </div>
      <button @click="openCreate" class="btn btn-primary">
        <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" /></svg>
        {{ t('ca.addCA') }}
      </button>
    </div>

    <div class="glass-panel rounded-2xl overflow-hidden">
        <div v-if="isLoading" class="flex items-center justify-center py-20">
        <div class="spinner animate-spin"></div>
      </div>

      <div v-else-if="cas.length === 0" class="text-center py-20">
        <svg class="w-20 h-20 mx-auto text-content-50 mb-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M19 21V5a2 2 0 00-2-2H7a2 2 0 00-2 2v16m14 0h2m-2 0h-5m-9 0H3m2 0h5M9 7h1m-1 4h1m4-4h1m-1 4h1m-5 10v-5a1 1 0 011-1h2a1 1 0 011 1v5m-4 0h4" /></svg>
        <p class="text-content-70 text-lg">{{ t('ca.noCA') }}</p>
        <p class="text-content-50 text-sm mt-2">{{ t('ca.noCADesc') }}</p>
      </div>

      <div v-else class="list-divider">
        <div v-for="ca in cas" :key="ca.id" class="list-item">
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-4">
              <div class="w-12 h-12 rounded-xl bg-accent-soft flex items-center justify-center">
                <svg class="w-6 h-6 text-accent" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 21V5a2 2 0 00-2-2H7a2 2 0 00-2 2v16m14 0h2m-2 0h-5m-9 0H3m2 0h5M9 7h1m-1 4h1m4-4h1m-1 4h1m-5 10v-5a1 1 0 011-1h2a1 1 0 011 1v5m-4 0h4" /></svg>
              </div>
              <div>
                <div class="flex items-center gap-2">
                  <h3 class="text-base-content font-medium">{{ ca.name }}</h3>
                  <span v-if="ca.is_default" class="badge-tag badge-tag-primary">{{ t('ca.default') }}</span>
                  <span v-if="!ca.is_active" class="badge-tag badge-tag-muted">{{ t('ca.disabled') }}</span>
                </div>
                <p class="text-content-50 text-sm mt-1">{{ ca.directory_url }}</p>
                <p class="text-content-50 text-xs mt-1">{{ ca.account_email }}</p>
              </div>
            </div>
            <div class="flex items-center gap-1">
              <button v-if="!ca.is_default" @click="handleSetDefault(ca.id)" class="icon-btn" :title="t('ca.setTitle')">
                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" /></svg>
              </button>
              <button @click="openEdit(ca)" class="icon-btn" :title="t('ca.editTitle')">
                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" /></svg>
              </button>
              <button @click="openDeleteModal(ca.id)" class="icon-btn icon-btn-danger" :title="t('ca.deleteTitle')">
                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" /></svg>
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 模态框 -->
    <dialog v-if="showModal" class="modal modal-open">
      <div class="modal-box glass-panel max-w-lg">
        <h2 class="text-xl font-bold text-base-content mb-6">{{ editingCA ? t('ca.editCA') : t('ca.addCA') }}</h2>
        <div class="space-y-4">
          <div>
            <label class="label">
              <span class="label-text text-base-content">{{ t('ca.name') }}</span>
            </label>
            <input v-model="formData.name" type="text" :placeholder="t('ca.namePlaceholder')" class="input input-bordered w-full" />
          </div>
          <div>
            <label class="label">
              <span class="label-text text-base-content">{{ t('ca.directoryURL') }}</span>
            </label>
            <input v-model="formData.directory_url" type="url" :placeholder="t('ca.directoryURLPlaceholder')" class="input input-bordered w-full" />
          </div>
          <div>
            <label class="label">
              <span class="label-text text-base-content">{{ t('ca.accountEmail') }}</span>
            </label>
            <input v-model="formData.account_email" type="email" :placeholder="t('ca.accountEmailPlaceholder')" class="input input-bordered w-full" />
          </div>
          <div class="flex items-center gap-6">
            <label class="label cursor-pointer">
              <span class="label-text text-base-content">{{ t('ca.enabled') }}</span>
              <input v-model="formData.is_active" type="checkbox" class="checkbox checkbox-primary" />
            </label>
            <label class="label cursor-pointer">
              <span class="label-text text-base-content">{{ t('ca.setAsDefault') }}</span>
              <input v-model="formData.is_default" type="checkbox" class="checkbox checkbox-primary" />
            </label>
          </div>
        </div>
        <div class="modal-action">
          <button @click="showModal = false" class="btn btn-secondary">{{ t('ca.cancel') }}</button>
          <button @click="handleSave" class="btn btn-primary">{{ editingCA ? t('ca.save') : t('ca.addCA') }}</button>
        </div>
      </div>
    </dialog>

    <!-- 删除确认弹窗 -->
    <dialog v-if="showDeleteModal" class="modal modal-open">
      <div class="modal-box glass-panel">
        <h3 class="font-bold text-lg">{{ t('ca.deleteTitle') }}</h3>
        <p class="py-4">{{ t('ca.deleteConfirm') }}</p>
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
