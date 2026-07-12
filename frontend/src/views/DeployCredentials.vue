<script setup lang="ts">
import { ref, reactive, onMounted, computed, watch, inject, watchEffect, type Ref } from 'vue'
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
import * as DeployCredentialService from '@bindings/cnb.cool/dtapp/certflow/deploycredentialservicewrapper'
import type { DeployCredentialListItem } from '@bindings/cnb.cool/dtapp/certflow/models'
import { useI18nStore } from '../stores/i18n'
import { showMessage, translateBackend } from '../utils/message'

const i18nStore = useI18nStore()
const { t } = i18nStore
const message = useMessage()

const showCreateModal = inject<Ref<boolean>>('showCreateModal')

const credentials = ref<DeployCredentialListItem[]>([])
const isLoading = ref(false)
const showModal = ref(false)
const editingId = ref<number | null>(null)

watchEffect(() => {
  if (showCreateModal?.value) {
    openCreate()
    showCreateModal.value = false
  }
})

const formData = ref<{
  name: string
  provider_type: string
  config: Record<string, string>
  is_active: boolean
  comment: string
}>({
  name: '',
  provider_type: 'aliyun',
  config: {},
  is_active: true,
  comment: '',
})

// 动态配置表单字段
const configFields = reactive<Record<string, string>>({})

// 各提供商的配置字段定义
const providerConfigSchema: Record<
  string,
  { key: string; label: string; type: 'text' | 'password' }[]
> = {
  aliyun: [
    { key: 'access_key_id', label: 'Access Key ID', type: 'text' },
    { key: 'access_key_secret', label: 'Access Key Secret', type: 'password' },
    { key: 'region', label: '区域', type: 'text' },
  ],
  tencentcloud: [
    { key: 'secret_id', label: 'Secret ID', type: 'text' },
    { key: 'secret_key', label: 'Secret Key', type: 'password' },
    { key: 'region', label: '区域', type: 'text' },
  ],
  huawei: [
    { key: 'access_key_id', label: 'Access Key ID', type: 'text' },
    { key: 'secret_access_key', label: 'Secret Access Key', type: 'password' },
    { key: 'region', label: '区域', type: 'text' },
  ],
  baiducloud: [
    { key: 'access_key_id', label: 'Access Key ID', type: 'text' },
    { key: 'access_key_secret', label: 'Access Key Secret', type: 'password' },
  ],
  btpanel: [
    { key: 'panel_url', label: '面板地址', type: 'text' },
    { key: 'api_key', label: 'API Key', type: 'password' },
  ],
  '1panel': [
    { key: 'panel_url', label: '面板地址', type: 'text' },
    { key: 'api_key', label: 'API Key', type: 'password' },
  ],
  acepanel: [
    { key: 'panel_url', label: '面板地址', type: 'text' },
    { key: 'api_key', label: 'API Key', type: 'password' },
  ],
}

const currentFields = computed(() => providerConfigSchema[formData.value.provider_type] || [])

const providerTypes = [
  { value: 'aliyun', label: '阿里云' },
  { value: 'tencentcloud', label: '腾讯云' },
  { value: 'huawei', label: '华为云' },
  { value: 'baiducloud', label: '百度云' },
  { value: 'btpanel', label: '宝塔面板' },
  { value: '1panel', label: '1Panel' },
  { value: 'acepanel', label: 'AcePanel' },
]

const providerTypeOptions = computed(() =>
  providerTypes.map((p) => ({ label: p.label, value: p.value })),
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

onMounted(async () => {
  isLoading.value = true
  try {
    const list = await DeployCredentialService.ListDeployCredentials()
    credentials.value = list || []
  } catch (e) {
    console.error('加载失败', e)
  } finally {
    isLoading.value = false
  }
})

const openCreate = () => {
  editingId.value = null
  formData.value = {
    name: '',
    provider_type: 'aliyun',
    config: {},
    is_active: true,
    comment: '',
  }
  const fields = providerConfigSchema['aliyun'] || []
  const newConfig: Record<string, string> = {}
  fields.forEach((f) => {
    newConfig[f.key] = ''
  })
  Object.assign(configFields, newConfig)
  showModal.value = true
}

const openEdit = (c: DeployCredentialListItem) => {
  editingId.value = c.id
  formData.value = {
    name: c.name,
    provider_type: c.provider_type,
    config: (c.config ?? {}) as Record<string, string>,
    is_active: c.is_active,
    comment: c.comment,
  }
  parseConfigFromMap(c.config as Record<string, string>)
  showModal.value = true
}

const handleSave = async () => {
  try {
    syncConfigToMap()
    if (editingId.value) {
      await DeployCredentialService.UpdateDeployCredential(editingId.value, formData.value)
    } else {
      await DeployCredentialService.CreateDeployCredential(formData.value)
    }
    showModal.value = false
    credentials.value = (await DeployCredentialService.ListDeployCredentials()) ?? []
    showMessage(t('deploy.credentialSaveSuccess'), 'success')
  } catch (e: any) {
    showMessage(
      t('deploy.credentialSaveFailed') + ': ' + translateBackend(e?.message || String(e)),
      'error',
    )
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
    await DeployCredentialService.DeleteDeployCredential(id)
    credentials.value = credentials.value.filter((c) => c.id !== id)
    showMessage(t('deploy.credentialDeleteSuccess'), 'success')
  } catch (e: any) {
    showMessage(
      t('deploy.credentialDeleteFailed') + ': ' + translateBackend(e?.message || String(e)),
      'error',
    )
  }
}

const getProviderLabel = (type: string) => {
  const pt = providerTypes.find((p) => p.value === type)
  return pt ? pt.label : type
}

// 监听提供商类型变化，重置配置字段（仅新建时）
watch(
  () => formData.value.provider_type,
  (newType) => {
    // 编辑时不重置配置字段
    if (editingId.value) return
    const fields = providerConfigSchema[newType] || []
    const newConfig: Record<string, string> = {}
    fields.forEach((f) => {
      newConfig[f.key] = ''
    })
    Object.assign(configFields, newConfig)
  },
)
</script>

<template>
  <div class="mt-4">
    <n-card size="small">
      <n-spin :show="isLoading">
        <n-empty
          v-if="!isLoading && credentials.length === 0"
          :description="t('deploy.credentialEmpty')"
        />

        <div v-else class="divide-y divide-neutral-200 dark:divide-neutral-700">
          <div
            v-for="c in credentials"
            :key="c.id"
            class="flex items-center justify-between px-6 py-4"
          >
            <div class="flex items-center gap-4">
              <div
                class="w-12 h-12 rounded-xl bg-green-50 dark:bg-green-900/30 flex items-center justify-center"
              >
                <svg
                  class="w-6 h-6 text-green-500"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z"
                  />
                </svg>
              </div>
              <div>
                <div class="flex items-center gap-2">
                  <h3 class="font-medium">{{ c.name }}</h3>
                  <n-tag size="small" :bordered="false">{{
                    getProviderLabel(c.provider_type)
                  }}</n-tag>
                  <n-tag v-if="!c.is_active" size="small" :bordered="false">{{
                    t('dns.disabled')
                  }}</n-tag>
                </div>
                <p v-if="c.comment" class="text-sm mt-1 opacity-50">{{ c.comment }}</p>
              </div>
            </div>
            <div class="flex items-center gap-1">
              <n-button
                quaternary
                circle
                size="small"
                @click="openEdit(c)"
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
                @click="openDeleteModal(c.id)"
                :title="t('deploy.delete')"
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
      :title="editingId ? t('deploy.credentialEdit') : t('deploy.credentialCreate')"
      style="max-width: 560px"
    >
      <n-form label-placement="top">
        <n-form-item :label="t('deploy.name')">
          <n-input v-model:value="formData.name" placeholder="例如: 阿里云 CDN" />
        </n-form-item>
        <n-form-item :label="t('deploy.credentialProviderType')">
          <n-select v-model:value="formData.provider_type" :options="providerTypeOptions" />
        </n-form-item>
        <!-- 动态配置字段 -->
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
              @update:value="
                (v: string) => {
                  try {
                    formData.config = JSON.parse(v)
                  } catch {}
                }
              "
              placeholder='{"api_key": "your-key"}'
              :rows="4"
              style="font-family: monospace; font-size: 12px"
            />
          </n-form-item>
        </div>
        <n-form-item :label="t('dns.comment')">
          <n-input v-model:value="formData.comment" placeholder="可选备注" />
        </n-form-item>
        <n-form-item :label="t('dns.enabled')">
          <n-switch v-model:value="formData.is_active" />
        </n-form-item>
      </n-form>
      <template #footer>
        <div class="flex justify-end gap-2">
          <n-button @click="showModal = false">{{ t('deploy.cancel') }}</n-button>
          <n-button type="primary" @click="handleSave">{{ t('deploy.save') }}</n-button>
        </div>
      </template>
    </n-modal>

    <!-- 删除确认弹窗 -->
    <n-modal v-model:show="showDeleteModal" preset="dialog" :title="t('deploy.delete')">
      <p>{{ t('deploy.credentialDeleteConfirm') }}</p>
      <template #action>
        <n-button @click="showDeleteModal = false">{{ t('deploy.cancel') }}</n-button>
        <n-button type="error" @click="handleDelete">{{ t('common.confirm') }}</n-button>
      </template>
    </n-modal>
  </div>
</template>
