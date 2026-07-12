<script setup lang="ts">
import { ref, reactive, computed, watch, watchEffect, inject, type Ref } from 'vue'
import {
  NCard,
  NButton,
  NInput,
  NSelect,
  NSwitch,
  NSpin,
  NModal,
  NForm,
  NFormItem,
  NEmpty,
  NTag,
  useMessage,
} from 'naive-ui'
import { useI18nStore } from '../stores/i18n'
import { showMessage, translateBackend } from '../utils/message'

const props = defineProps<{
  title: string
  subtitle: string
  emptyText: string
  createText: string
  editText: string
  providerTypes: { value: string; label: string }[]
  configSchema: Record<string, { key: string; label: string; type: 'text' | 'password' }[]>
  iconColor?: string
  loadItems: () => Promise<any[]>
  createItem: (data: any) => Promise<any>
  updateItem: (id: number, data: any) => Promise<any>
  deleteItem: (id: number) => Promise<void>
}>()

const i18nStore = useI18nStore()
const { t } = i18nStore
const message = useMessage()

const showCreateModal = inject<Ref<boolean>>('showCreateModal')

const items = ref<any[]>([])
const isLoading = ref(false)
const showModal = ref(false)
const editingId = ref<number | null>(null)

const formData = ref<{
  name: string
  provider_type: string
  config: Record<string, string>
  is_active: boolean
  comment: string
}>({
  name: '',
  provider_type: '',
  config: {},
  is_active: true,
  comment: '',
})

const configFields = reactive<Record<string, string>>({})

const currentFields = computed(() => props.configSchema[formData.value.provider_type] || [])

const providerTypeOptions = computed(() =>
  props.providerTypes.map((p) => ({ label: p.label, value: p.value })),
)

const syncConfigToMap = () => {
  const clean: Record<string, string> = {}
  for (const [k, v] of Object.entries(configFields)) {
    if (v) clean[k] = v
  }
  formData.value.config = clean
}

const parseConfigFromMap = (config: Record<string, string> | undefined) => {
  Object.assign(configFields, config || {})
}

const loadData = async () => {
  isLoading.value = true
  try {
    items.value = (await props.loadItems()) || []
  } catch (e) {
    console.error('加载失败', e)
  } finally {
    isLoading.value = false
  }
}

const openCreate = () => {
  editingId.value = null
  formData.value = {
    name: '',
    provider_type: props.providerTypes[0]?.value || '',
    config: {},
    is_active: true,
    comment: '',
  }
  const fields = props.configSchema[formData.value.provider_type] || []
  const newConfig: Record<string, string> = {}
  fields.forEach((f) => {
    newConfig[f.key] = ''
  })
  Object.assign(configFields, newConfig)
  showModal.value = true
}

const openEdit = (item: any) => {
  editingId.value = item.id
  formData.value = {
    name: item.name,
    provider_type: item.provider_type,
    config: (item.config ?? {}) as Record<string, string>,
    is_active: item.is_active,
    comment: item.comment,
  }
  parseConfigFromMap(item.config as Record<string, string>)
  showModal.value = true
}

const handleSave = async () => {
  try {
    syncConfigToMap()
    if (editingId.value) {
      await props.updateItem(editingId.value, formData.value)
    } else {
      await props.createItem(formData.value)
    }
    showModal.value = false
    await loadData()
    showMessage(t('dns.saveSuccess'), 'success')
  } catch (e: any) {
    showMessage(t('dns.saveFailed') + ': ' + translateBackend(e?.message || String(e)), 'error')
  }
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
    await props.deleteItem(id)
    items.value = items.value.filter((item: any) => item.id !== id)
    showMessage(t('dns.deleteSuccess'), 'success')
  } catch (e: any) {
    showMessage(t('dns.deleteProviderFailed') + ': ' + translateBackend(e?.message || String(e)), 'error')
  }
}

const getProviderLabel = (type: string) => {
  const pt = props.providerTypes.find((p) => p.value === type)
  return pt ? pt.label : type
}

const iconBgClass = computed(() => `bg-${props.iconColor || 'green'}-50 dark:bg-${props.iconColor || 'green'}-900/30`)
const iconTextClass = computed(() => `text-${props.iconColor || 'green'}-500`)

watch(
  () => formData.value.provider_type,
  (newType) => {
    if (editingId.value) return
    const fields = props.configSchema[newType] || []
    const newConfig: Record<string, string> = {}
    fields.forEach((f) => {
      newConfig[f.key] = ''
    })
    Object.assign(configFields, newConfig)
  },
)

watchEffect(() => {
  if (showCreateModal?.value) {
    openCreate()
    showCreateModal.value = false
  }
})

// 暴露 loadData 方法给父组件
defineExpose({ loadData })

// 初始化加载
loadData()
</script>

<template>
  <div>
    <n-card size="small">
      <n-spin :show="isLoading">
        <n-empty v-if="!isLoading && items.length === 0" :description="emptyText" />

        <div v-else class="divide-y divide-neutral-200 dark:divide-neutral-700">
          <div
            v-for="item in items"
            :key="item.id"
            class="flex items-center justify-between px-6 py-4"
          >
            <div class="flex items-center gap-4">
              <div
                class="w-12 h-12 rounded-xl flex items-center justify-center"
                :class="iconBgClass"
              >
                <svg
                  class="w-6 h-6"
                  :class="iconTextClass"
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
              <div>
                <div class="flex items-center gap-2">
                  <h3 class="font-medium">{{ item.name }}</h3>
                  <n-tag size="small" :bordered="false">{{ getProviderLabel(item.provider_type) }}</n-tag>
                  <n-tag v-if="!item.is_active" size="small" :bordered="false">{{ t('dns.disabled') }}</n-tag>
                </div>
                <p v-if="item.comment" class="text-sm mt-1 opacity-50">{{ item.comment }}</p>
              </div>
            </div>
            <div class="flex items-center gap-1">
              <n-button quaternary circle size="small" @click="openEdit(item)" :title="t('dns.editTitle')">
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
              <n-button quaternary circle size="small" type="error" @click="openDeleteModal(item.id)" :title="t('dns.deleteTitle')">
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

    <!-- Modal -->
    <n-modal
      v-model:show="showModal"
      preset="card"
      :title="editingId ? editText : createText"
      style="max-width: 560px"
    >
      <n-form label-placement="top">
        <n-form-item :label="t('dns.name')">
          <n-input v-model:value="formData.name" :placeholder="t('dns.namePlaceholder')" />
        </n-form-item>
        <n-form-item :label="t('dns.providerType')">
          <n-select v-model:value="formData.provider_type" :options="providerTypeOptions" />
        </n-form-item>
        <div v-if="currentFields.length > 0">
          <n-form-item :label="t('dns.config')">
            <div class="w-full space-y-3">
              <div v-for="field in currentFields" :key="field.key">
                <label class="block text-xs opacity-60 mb-1">{{ field.label }}</label>
                <n-input
                  v-model:value="configFields[field.key]"
                  :type="field.type"
                  :placeholder="field.label"
                  show-password-on="click"
                />
              </div>
            </div>
          </n-form-item>
        </div>
        <div v-else>
          <n-form-item :label="t('dns.config')">
            <n-input
              type="textarea"
              :value="JSON.stringify(formData.config, null, 2)"
              @update:value="(v: string) => { try { formData.config = JSON.parse(v) } catch {} }"
              :placeholder="t('dns.configPlaceholder')"
              :rows="4"
              style="font-family: monospace; font-size: 12px"
            />
          </n-form-item>
        </div>
        <n-form-item :label="t('dns.comment')">
          <n-input v-model:value="formData.comment" :placeholder="t('dns.commentPlaceholder')" />
        </n-form-item>
        <n-form-item :label="t('dns.enabled')">
          <n-switch v-model:value="formData.is_active" />
        </n-form-item>
      </n-form>
      <template #footer>
        <div class="flex justify-end gap-2">
          <n-button @click="showModal = false">{{ t('dns.cancel') }}</n-button>
          <n-button type="primary" @click="handleSave">{{ t('dns.save') }}</n-button>
        </div>
      </template>
    </n-modal>

    <!-- 删除确认弹窗 -->
    <n-modal v-model:show="showDeleteModal" preset="dialog" :title="t('dns.deleteTitle')">
      <p>{{ t('dns.deleteConfirm') }}</p>
      <template #action>
        <n-button @click="showDeleteModal = false">{{ t('common.cancel') }}</n-button>
        <n-button type="error" @click="handleDelete">{{ t('common.confirm') }}</n-button>
      </template>
    </n-modal>
  </div>
</template>
