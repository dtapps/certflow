<script setup lang="ts">
import { ref, onMounted } from 'vue'
import {
  NCard,
  NButton,
  NInput,
  NSwitch,
  NSpin,
  NModal,
  NForm,
  NFormItem,
  NEmpty,
  NTag,
} from 'naive-ui'
import * as CAService from '@bindings/cnb.cool/dtapp/certflow/caservicewrapper'
import type { CAListItem } from '@bindings/cnb.cool/dtapp/certflow/models'
import { useI18nStore } from '../stores/i18n'

const i18nStore = useI18nStore()
const { t } = i18nStore

const cas = ref<CAListItem[]>([])
const isLoading = ref(false)
const showModal = ref(false)
const editingCA = ref<number | null>(null)

const formData = ref({
  name: '',
  directory_url: '',
  account_email: '',
  is_default: false,
  is_active: true,
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
  formData.value = {
    name: '',
    directory_url: '',
    account_email: '',
    is_default: false,
    is_active: true,
  }
  showModal.value = true
}

const openEdit = (ca: (typeof cas.value)[0]) => {
  editingCA.value = ca.id
  formData.value = {
    name: ca.name,
    directory_url: ca.directory_url,
    account_email: ca.account_email,
    is_default: ca.is_default,
    is_active: ca.is_active,
  }
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
    cas.value = cas.value.filter((c) => c.id !== id)
  } catch (e) {
    console.error(t('ca.deleteFailed'), e)
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
        <h1 class="text-2xl font-bold">{{ t('ca.title') }}</h1>
        <p class="text-sm mt-1 opacity-60">{{ t('ca.subtitle') }}</p>
      </div>
      <n-button type="primary" @click="openCreate">
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
        {{ t('ca.addCA') }}
      </n-button>
    </div>

    <n-card size="small">
      <n-spin :show="isLoading">
        <n-empty v-if="!isLoading && cas.length === 0" :description="t('ca.noCA')">
          <template #extra>
            <p class="text-sm opacity-50">{{ t('ca.noCADesc') }}</p>
          </template>
        </n-empty>

        <div v-else class="divide-y divide-neutral-200 dark:divide-neutral-700">
          <div v-for="ca in cas" :key="ca.id" class="flex items-center justify-between px-6 py-4">
            <div class="flex items-center gap-4">
              <div
                class="w-12 h-12 rounded-xl bg-purple-50 dark:bg-purple-900/30 flex items-center justify-center"
              >
                <svg
                  class="w-6 h-6 text-purple-500"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M19 21V5a2 2 0 00-2-2H7a2 2 0 00-2 2v16m14 0h2m-2 0h-5m-9 0H3m2 0h5M9 7h1m-1 4h1m4-4h1m-1 4h1m-5 10v-5a1 1 0 011-1h2a1 1 0 011 1v5m-4 0h4"
                  />
                </svg>
              </div>
              <div>
                <div class="flex items-center gap-2">
                  <h3 class="font-medium">{{ ca.name }}</h3>
                  <n-tag v-if="ca.is_default" type="primary" size="small" :bordered="false">{{
                    t('ca.default')
                  }}</n-tag>
                  <n-tag v-if="!ca.is_active" size="small" :bordered="false">{{
                    t('ca.disabled')
                  }}</n-tag>
                </div>
                <p class="text-sm mt-1 opacity-50">{{ ca.directory_url }}</p>
                <p class="text-xs mt-1 opacity-50">{{ ca.account_email }}</p>
              </div>
            </div>
            <div class="flex items-center gap-1">
              <n-button
                v-if="!ca.is_default"
                quaternary
                circle
                size="small"
                @click="handleSetDefault(ca.id)"
                :title="t('ca.setTitle')"
              >
                <template #icon>
                  <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      stroke-width="2"
                      d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"
                    />
                  </svg>
                </template>
              </n-button>
              <n-button
                quaternary
                circle
                size="small"
                @click="openEdit(ca)"
                :title="t('ca.editTitle')"
              >
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
              <n-button
                quaternary
                circle
                size="small"
                type="error"
                @click="openDeleteModal(ca.id)"
                :title="t('ca.deleteTitle')"
              >
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
              </n-button>
            </div>
          </div>
        </div>
      </n-spin>
    </n-card>

    <!-- 模态框 -->
    <n-modal
      v-model:show="showModal"
      preset="card"
      :title="editingCA ? t('ca.editCA') : t('ca.addCA')"
      style="max-width: 480px"
    >
      <n-form label-placement="top">
        <n-form-item :label="t('ca.name')">
          <n-input v-model:value="formData.name" :placeholder="t('ca.namePlaceholder')" />
        </n-form-item>
        <n-form-item :label="t('ca.directoryURL')">
          <n-input
            v-model:value="formData.directory_url"
            :placeholder="t('ca.directoryURLPlaceholder')"
          />
        </n-form-item>
        <n-form-item :label="t('ca.accountEmail')">
          <n-input
            v-model:value="formData.account_email"
            :placeholder="t('ca.accountEmailPlaceholder')"
          />
        </n-form-item>
        <n-form-item :label="t('ca.enabled')">
          <n-switch v-model:value="formData.is_active" />
        </n-form-item>
        <n-form-item :label="t('ca.setAsDefault')">
          <n-switch v-model:value="formData.is_default" />
        </n-form-item>
      </n-form>
      <template #footer>
        <div class="flex justify-end gap-2">
          <n-button @click="showModal = false">{{ t('ca.cancel') }}</n-button>
          <n-button type="primary" @click="handleSave">{{
            editingCA ? t('ca.save') : t('ca.addCA')
          }}</n-button>
        </div>
      </template>
    </n-modal>

    <!-- 删除确认弹窗 -->
    <n-modal v-model:show="showDeleteModal" preset="dialog" :title="t('ca.deleteTitle')">
      <p>{{ t('ca.deleteConfirm') }}</p>
      <template #action>
        <n-button @click="showDeleteModal = false">{{ t('common.cancel') }}</n-button>
        <n-button type="error" @click="handleDelete">{{ t('common.confirm') }}</n-button>
      </template>
    </n-modal>
  </div>
</template>
