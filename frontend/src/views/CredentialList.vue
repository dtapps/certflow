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
  NCheckbox,
  useMessage,
} from 'naive-ui'
import { useI18nStore } from '../stores/i18n'
import { showMessage, translateBackend } from '../utils/message'
import ProviderIcon from '../components/ProviderIcon.vue'

const props = defineProps<{
  title: string
  subtitle: string
  emptyText: string
  createText: string
  editText: string
  providerTypes: { value: string; labelKey: string }[]
  configSchema: Record<string, { key: string; labelKey: string; type: 'text' | 'password' }[]>
  loadItems: () => Promise<any[]>
  createItem: (data: any) => Promise<any>
  updateItem: (id: number, data: any) => Promise<any>
  deleteItem: (id: number) => Promise<void>
  setActiveItem?: (id: number, active: boolean) => Promise<void>
}>()

const i18nStore = useI18nStore()
const { t } = i18nStore
const message = useMessage()

const showCreateModal = inject<Ref<boolean>>('showCreateModal')

const items = ref<any[]>([])
const isLoading = ref(false)
const showModal = ref(false)
const editingId = ref<number | null>(null)
const togglingId = ref<number | null>(null)

const handleToggleActive = async (item: any, value: boolean) => {
  if (!props.setActiveItem) return
  const prev = item.is_active
  togglingId.value = item.id
  item.is_active = value
  try {
    await props.setActiveItem(item.id, value)
    showMessage(value ? t('common.enabledSuccess') : t('common.disabledSuccess'), 'success')
  } catch (e: any) {
    item.is_active = prev
    showMessage(
      t('common.toggleFailed') + ': ' + translateBackend(e?.message || String(e)),
      'error',
    )
  } finally {
    togglingId.value = null
  }
}

const formData = ref<{
  name: string
  provider_type: string
  config: Record<string, string>
  comment: string
}>({
  name: '',
  provider_type: '',
  config: {},
  comment: '',
})

const configFields = reactive<Record<string, string>>({})

const currentFields = computed(() => props.configSchema[formData.value.provider_type] || [])

const providerTypeOptions = computed(() =>
  props.providerTypes.map((p) => ({ label: t(p.labelKey), value: p.value })),
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
    console.debug(
      t('log.credentialLoad', {
        count: items.value.length,
        first: JSON.stringify(
          items.value[0]
            ? { name: items.value[0].name, provider_type: items.value[0].provider_type }
            : null,
        ),
      }),
    )
  } catch (e) {
    console.error(t('log.credentialLoadFailed', { err: String(e) }))
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
    showMessage(
      t('dns.deleteProviderFailed') + ': ' + translateBackend(e?.message || String(e)),
      'error',
    )
  }
}

const getProviderLabel = (type: string) => {
  const pt = props.providerTypes.find((p) => p.value === type)
  return pt ? t(pt.labelKey) : type
}

// 批量操作
const selectedIds = ref<number[]>([])
const isSelected = (id: number) => selectedIds.value.includes(id)
const isAllSelected = computed(
  () => items.value.length > 0 && items.value.every((i) => selectedIds.value.includes(i.id)),
)
const isIndeterminate = computed(() => {
  const n = items.value.filter((i) => selectedIds.value.includes(i.id)).length
  return n > 0 && n < items.value.length
})
const toggleSelect = (id: number) => {
  const i = selectedIds.value.indexOf(id)
  if (i >= 0) selectedIds.value.splice(i, 1)
  else selectedIds.value.push(id)
}
const toggleSelectAll = () => {
  selectedIds.value = isAllSelected.value ? [] : items.value.map((i) => i.id)
}
const clearSelection = () => {
  selectedIds.value = []
}

async function runBatch(fn: (id: number) => Promise<unknown>, successKey: string) {
  const ids = selectedIds.value.slice()
  let ok = 0
  await Promise.all(
    ids.map(async (id) => {
      try {
        await fn(id)
        ok++
      } catch (e: any) {
        showMessage(
          t('credential.batchFailed') + translateBackend(e?.message || String(e)),
          'error',
        )
      }
    }),
  )
  if (ok > 0) showMessage(t(successKey, { count: ok }), 'success')
  clearSelection()
  await loadData()
}

const batchEnable = () =>
  props.setActiveItem
    ? runBatch((id) => props.setActiveItem!(id, true), 'credential.batchEnabled')
    : Promise.resolve()
const batchDisable = () =>
  props.setActiveItem
    ? runBatch((id) => props.setActiveItem!(id, false), 'credential.batchDisabled')
    : Promise.resolve()

const showBatchDeleteModal = ref(false)
const batchDeleting = ref(false)

function batchDelete() {
  if (selectedIds.value.length === 0) return
  showBatchDeleteModal.value = true
}

async function confirmBatchDelete() {
  const ids = selectedIds.value.slice()
  batchDeleting.value = true
  let ok = 0
  try {
    await Promise.all(
      ids.map(async (id) => {
        try {
          await props.deleteItem(id)
          ok++
        } catch (e: any) {
          showMessage(
            t('credential.batchFailed') + translateBackend(e?.message || String(e)),
            'error',
          )
        }
      }),
    )
    if (ok > 0) showMessage(t('credential.batchDeleted', { count: ok }), 'success')
  } finally {
    batchDeleting.value = false
    showBatchDeleteModal.value = false
    clearSelection()
    await loadData()
  }
}

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
    <!-- 批量操作栏 -->
    <div v-if="items.length > 0" class="flex items-center justify-between mb-3 px-1">
      <div class="flex items-center gap-3">
        <n-checkbox
          :checked="isAllSelected"
          :indeterminate="isIndeterminate"
          @update:checked="toggleSelectAll"
        >
          {{ t('credential.selectAll') }}
        </n-checkbox>
        <span v-if="selectedIds.length > 0" class="text-sm opacity-60">
          {{ t('credential.selectedCount', { count: selectedIds.length }) }}
        </span>
      </div>
      <div v-if="selectedIds.length > 0" class="flex items-center gap-2">
        <template v-if="props.setActiveItem">
          <n-button size="small" type="primary" secondary @click="batchEnable">{{
            t('credential.batchEnable')
          }}</n-button>
          <n-button size="small" secondary @click="batchDisable">{{
            t('credential.batchDisable')
          }}</n-button>
        </template>
        <n-button size="small" type="error" secondary @click="batchDelete">{{
          t('credential.batchDelete')
        }}</n-button>
        <n-button size="small" quaternary @click="clearSelection">{{
          t('common.cancel')
        }}</n-button>
      </div>
    </div>
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
              <span @click.stop>
                <n-checkbox
                  size="small"
                  :checked="isSelected(item.id)"
                  @update:checked="toggleSelect(item.id)"
                />
              </span>
              <ProviderIcon :provider-type="item.provider_type" :name="item.name" :size="36" />
              <div>
                <div class="flex items-center gap-2">
                  <h3 class="font-medium">{{ item.name }}</h3>
                  <n-tag size="small" :bordered="false">{{
                    getProviderLabel(item.provider_type)
                  }}</n-tag>
                  <n-tag v-if="!item.is_active" size="small" :bordered="false">{{
                    t('dns.disabled')
                  }}</n-tag>
                </div>
                <p v-if="item.comment" class="text-sm mt-1 opacity-50">{{ item.comment }}</p>
              </div>
            </div>
            <div class="flex items-center gap-1">
              <n-switch
                v-if="props.setActiveItem"
                :value="item.is_active"
                :loading="togglingId === item.id"
                size="small"
                :title="item.is_active ? t('common.disableTitle') : t('common.enableTitle')"
                @update:value="(v: boolean) => handleToggleActive(item, v)"
              />
              <n-button
                quaternary
                circle
                size="small"
                @click="openEdit(item)"
                :title="t('dns.editTitle')"
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
                @click="openDeleteModal(item.id)"
                :title="t('dns.deleteTitle')"
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
          <n-select
            v-model:value="formData.provider_type"
            :options="providerTypeOptions"
            :placeholder="t('dns.providerTypePlaceholder')"
          />
        </n-form-item>
        <div v-if="currentFields.length > 0">
          <n-form-item :label="t('dns.config')">
            <div class="w-full space-y-3">
              <div v-for="field in currentFields" :key="field.key">
                <label class="block text-xs opacity-60 mb-1">{{ t(field.labelKey) }}</label>
                <n-input
                  v-model:value="configFields[field.key]"
                  :type="field.type"
                  :placeholder="t(field.labelKey)"
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
              @update:value="
                (v: string) => {
                  try {
                    formData.config = JSON.parse(v)
                  } catch {}
                }
              "
              :placeholder="t('dns.configPlaceholder')"
              :rows="4"
              style="font-family: monospace; font-size: 12px"
            />
          </n-form-item>
        </div>
        <n-form-item :label="t('dns.comment')">
          <n-input v-model:value="formData.comment" :placeholder="t('dns.commentPlaceholder')" />
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

    <!-- 批量删除确认弹窗 -->
    <n-modal
      v-model:show="showBatchDeleteModal"
      preset="dialog"
      :title="t('credential.batchDelete')"
    >
      <p>{{ t('credential.batchDeleteConfirm', { count: selectedIds.length }) }}</p>
      <p class="text-sm opacity-60 mt-2">{{ t('credential.batchDeleteConfirmDesc') }}</p>
      <template #action>
        <n-button @click="showBatchDeleteModal = false">{{ t('common.cancel') }}</n-button>
        <n-button type="error" :loading="batchDeleting" @click="confirmBatchDelete">{{
          t('common.confirm')
        }}</n-button>
      </template>
    </n-modal>
  </div>
</template>
