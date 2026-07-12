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
  NInputNumber,
  useMessage,
} from 'naive-ui'
import * as DNSProviderService from '@bindings/cnb.cool/dtapp/certflow/dnsproviderservicewrapper'
import type { DNSProviderListItem } from '@bindings/cnb.cool/dtapp/certflow/models'
import { useI18nStore } from '../stores/i18n'
import { initMessage, showMessage } from '../utils/message'

const i18nStore = useI18nStore()
const { t } = i18nStore
const message = useMessage()
initMessage(message)

const showCreateModal = inject<Ref<boolean>>('showCreateModal')

const providers = ref<DNSProviderListItem[]>([])
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
  provider_type: 'cloudflare',
  config: {},
  is_active: true,
  comment: '',
})

// 动态配置表单字段
const configFields = reactive<Record<string, string>>({})

// 各提供商的配置字段定义
const providerConfigSchema: Record<
  string,
  { key: string; labelKey: string; type: 'text' | 'password' }[]
> = {
  cloudflare: [
    { key: 'email', labelKey: 'dns.config.email', type: 'text' },
    { key: 'api_key', labelKey: 'dns.config.api_key', type: 'password' },
    { key: 'api_token', labelKey: 'dns.config.api_token', type: 'password' },
  ],
  aliyun: [
    { key: 'access_key_id', labelKey: 'dns.config.access_key_id', type: 'text' },
    { key: 'access_key_secret', labelKey: 'dns.config.access_key_secret', type: 'password' },
    { key: 'region_id', labelKey: 'dns.config.region_id', type: 'text' },
  ],
  huawei: [
    { key: 'access_key_id', labelKey: 'dns.config.access_key_id', type: 'text' },
    { key: 'secret_access_key', labelKey: 'dns.config.secret_access_key', type: 'password' },
    { key: 'region', labelKey: 'dns.config.region', type: 'text' },
  ],
  tencentcloud: [
    { key: 'secret_id', labelKey: 'dns.config.secret_id', type: 'text' },
    { key: 'secret_key', labelKey: 'dns.config.secret_key', type: 'password' },
    { key: 'region', labelKey: 'dns.config.region', type: 'text' },
  ],
  aws: [
    { key: 'access_key_id', labelKey: 'dns.config.access_key_id', type: 'text' },
    { key: 'secret_access_key', labelKey: 'dns.config.secret_access_key', type: 'password' },
    { key: 'region', labelKey: 'dns.config.region', type: 'text' },
  ],
  googlecloud: [
    { key: 'client_id', labelKey: 'dns.config.client_id', type: 'text' },
    { key: 'email', labelKey: 'dns.config.email', type: 'text' },
    { key: 'password', labelKey: 'dns.config.password', type: 'password' },
  ],
  baiducloud: [
    { key: 'access_key_id', labelKey: 'dns.config.access_key_id', type: 'text' },
    { key: 'secret_access_key', labelKey: 'dns.config.secret_access_key', type: 'password' },
  ],
  jdcloud: [
    { key: 'access_key_id', labelKey: 'dns.config.access_key_id', type: 'text' },
    { key: 'access_key_secret', labelKey: 'dns.config.access_key_secret', type: 'password' },
    { key: 'region_id', labelKey: 'dns.config.region_id', type: 'text' },
  ],
  volcengine: [
    { key: 'access_key', labelKey: 'dns.config.access_key', type: 'text' },
    { key: 'secret_key', labelKey: 'dns.config.secret_key', type: 'password' },
    { key: 'region', labelKey: 'dns.config.region', type: 'text' },
  ],
  edgeone: [
    { key: 'secret_id', labelKey: 'dns.config.secret_id', type: 'text' },
    { key: 'secret_key', labelKey: 'dns.config.secret_key', type: 'password' },
    { key: 'region', labelKey: 'dns.config.region', type: 'text' },
  ],
  aliesa: [
    { key: 'api_key', labelKey: 'dns.config.api_key', type: 'text' },
    { key: 'secret_key', labelKey: 'dns.config.secret_key', type: 'password' },
    { key: 'region_id', labelKey: 'dns.config.region_id', type: 'text' },
  ],
  ucloud: [
    { key: 'public_key', labelKey: 'dns.config.public_key', type: 'text' },
    { key: 'private_key', labelKey: 'dns.config.private_key', type: 'password' },
    { key: 'region', labelKey: 'dns.config.region', type: 'text' },
  ],
  westcn: [
    { key: 'username', labelKey: 'dns.config.username', type: 'text' },
    { key: 'password', labelKey: 'dns.config.password', type: 'password' },
  ],
  com35: [
    { key: 'username', labelKey: 'dns.config.username', type: 'text' },
    { key: 'password', labelKey: 'dns.config.password', type: 'password' },
  ],
  rainyun: [{ key: 'api_key', labelKey: 'dns.config.api_key', type: 'text' }],
  todaynic: [
    { key: 'auth_user_id', labelKey: 'dns.config.auth_user_id', type: 'text' },
    { key: 'api_key', labelKey: 'dns.config.api_key', type: 'text' },
  ],
  dnsla: [
    { key: 'api_id', labelKey: 'dns.config.api_id', type: 'text' },
    { key: 'api_secret', labelKey: 'dns.config.api_secret', type: 'password' },
  ],
  dns51: [
    { key: 'api_key', labelKey: 'dns.config.api_key', type: 'text' },
    { key: 'api_secret', labelKey: 'dns.config.api_secret', type: 'password' },
  ],
  xinnet: [
    { key: 'secret', labelKey: 'dns.config.secret', type: 'password' },
    { key: 'agent_id', labelKey: 'dns.config.agent_id', type: 'text' },
  ],
  porkbun: [
    { key: 'api_key', labelKey: 'dns.config.api_key', type: 'text' },
    { key: 'secret_api_key', labelKey: 'dns.config.secret_api_key', type: 'password' },
  ],
  namecheap: [
    { key: 'api_user', labelKey: 'dns.config.api_user', type: 'text' },
    { key: 'api_key', labelKey: 'dns.config.api_key', type: 'text' },
    { key: 'client_ip', labelKey: 'dns.config.client_ip', type: 'text' },
  ],
  godaddy: [
    { key: 'api_key', labelKey: 'dns.config.api_key', type: 'text' },
    { key: 'api_secret', labelKey: 'dns.config.api_secret', type: 'password' },
  ],
  gandiv5: [
    {
      key: 'personal_access_token',
      labelKey: 'dns.config.personal_access_token',
      type: 'password',
    },
  ],
  dynadot: [
    { key: 'api_key', labelKey: 'dns.config.api_key', type: 'text' },
    { key: 'api_secret', labelKey: 'dns.config.api_secret', type: 'password' },
  ],
  azuredns: [
    { key: 'subscription_id', labelKey: 'dns.config.subscription_id', type: 'text' },
    { key: 'resource_group', labelKey: 'dns.config.resource_group', type: 'text' },
    { key: 'client_id', labelKey: 'dns.config.client_id', type: 'text' },
    { key: 'client_secret', labelKey: 'dns.config.client_secret', type: 'password' },
    { key: 'tenant_id', labelKey: 'dns.config.tenant_id', type: 'text' },
  ],
  digitalocean: [{ key: 'auth_token', labelKey: 'dns.config.auth_token', type: 'password' }],
  vultr: [{ key: 'api_key', labelKey: 'dns.config.api_key', type: 'password' }],
  hetzner: [{ key: 'api_token', labelKey: 'dns.config.api_token', type: 'password' }],
  linode: [{ key: 'token', labelKey: 'dns.config.token', type: 'password' }],
  ovh: [
    { key: 'application_key', labelKey: 'dns.config.application_key', type: 'text' },
    { key: 'application_secret', labelKey: 'dns.config.application_secret', type: 'password' },
    { key: 'consumer_key', labelKey: 'dns.config.consumer_key', type: 'password' },
  ],
  dnsimple: [{ key: 'access_token', labelKey: 'dns.config.access_token', type: 'password' }],
  ns1: [{ key: 'api_key', labelKey: 'dns.config.api_key', type: 'password' }],
}

const currentFields = computed(() => providerConfigSchema[formData.value.provider_type] || [])

const providerTypes = [
  { value: 'cloudflare', labelKey: 'dns.type.cloudflare' },
  { value: 'aliyun', labelKey: 'dns.type.aliyun' },
  { value: 'huawei', labelKey: 'dns.type.huawei' },
  { value: 'tencentcloud', labelKey: 'dns.type.tencentcloud' },
  { value: 'aws', labelKey: 'dns.type.aws' },
  { value: 'googlecloud', labelKey: 'dns.type.googlecloud' },
  { value: 'baiducloud', labelKey: 'dns.type.baiducloud' },
  { value: 'jdcloud', labelKey: 'dns.type.jdcloud' },
  { value: 'volcengine', labelKey: 'dns.type.volcengine' },
  { value: 'edgeone', labelKey: 'dns.type.edgeone' },
  { value: 'aliesa', labelKey: 'dns.type.aliesa' },
  { value: 'ucloud', labelKey: 'dns.type.ucloud' },
  { value: 'westcn', labelKey: 'dns.type.westcn' },
  { value: 'com35', labelKey: 'dns.type.com35' },
  { value: 'rainyun', labelKey: 'dns.type.rainyun' },
  { value: 'todaynic', labelKey: 'dns.type.todaynic' },
  { value: 'dnsla', labelKey: 'dns.type.dnsla' },
  { value: 'dns51', labelKey: 'dns.type.dns51' },
  { value: 'xinnet', labelKey: 'dns.type.xinnet' },
  { value: 'porkbun', labelKey: 'dns.type.porkbun' },
  { value: 'namecheap', labelKey: 'dns.type.namecheap' },
  { value: 'godaddy', labelKey: 'dns.type.godaddy' },
  { value: 'gandiv5', labelKey: 'dns.type.gandiv5' },
  { value: 'dynadot', labelKey: 'dns.type.dynadot' },
  { value: 'azuredns', labelKey: 'dns.type.azuredns' },
  { value: 'digitalocean', labelKey: 'dns.type.digitalocean' },
  { value: 'vultr', labelKey: 'dns.type.vultr' },
  { value: 'hetzner', labelKey: 'dns.type.hetzner' },
  { value: 'linode', labelKey: 'dns.type.linode' },
  { value: 'ovh', labelKey: 'dns.type.ovh' },
  { value: 'dnsimple', labelKey: 'dns.type.dnsimple' },
  { value: 'ns1', labelKey: 'dns.type.ns1' },
]

const providerTypeOptions = computed(() =>
  providerTypes.map((p) => ({
    label: t(p.labelKey),
    value: p.value,
  })),
)

// 将配置字段同步到 JSON
const syncConfigToMap = () => {
  const clean: Record<string, string> = {}
  for (const [k, v] of Object.entries(configFields)) {
    if (v) clean[k] = v
  }
  formData.value.config = clean
}

// 从 map 解析到配置字段
const parseConfigFromMap = (config: Record<string, string> | undefined) => {
  Object.assign(configFields, config || {})
}

onMounted(async () => {
  isLoading.value = true
  try {
    providers.value = (await DNSProviderService.ListDNSProviders()) ?? []
  } catch (e) {
    console.error(t('dns.loadFailed'), e)
  } finally {
    isLoading.value = false
  }
})

const openCreate = () => {
  editingId.value = null
  formData.value = {
    name: '',
    provider_type: 'cloudflare',
    config: {},
    is_active: true,
    comment: '',
  }
  const fields = providerConfigSchema['cloudflare'] || []
  const newConfig: Record<string, string> = {}
  fields.forEach((f) => {
    newConfig[f.key] = ''
  })
  Object.assign(configFields, newConfig)
  showModal.value = true
}

const openEdit = (p: (typeof providers.value)[0]) => {
  editingId.value = p.id
  formData.value = {
    name: p.name,
    provider_type: p.provider_type,
    config: (p.config ?? {}) as Record<string, string>,
    is_active: p.is_active,
    comment: p.comment,
  }
  Object.assign(configFields, p.config || {})
  showModal.value = true
}

const handleSave = async () => {
  try {
    syncConfigToMap()
    if (editingId.value) {
      await DNSProviderService.UpdateDNSProvider(editingId.value, formData.value)
    } else {
      await DNSProviderService.CreateDNSProvider(formData.value)
    }
    showModal.value = false
    providers.value = (await DNSProviderService.ListDNSProviders()) ?? []
    showMessage(t('dns.saveSuccess'), 'success')
  } catch (e) {
    showMessage(t('dns.saveFailed') + ' ' + e, 'error')
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
    await DNSProviderService.DeleteDNSProvider(id)
    providers.value = providers.value.filter((p) => p.id !== id)
    showMessage(t('dns.deleteSuccess'), 'success')
  } catch (e) {
    showMessage(t('dns.deleteProviderFailed') + ' ' + e, 'error')
  }
}

const getProviderLabel = (type: string) => {
  const pt = providerTypes.find((p) => p.value === type)
  return pt ? t(pt.labelKey) : type
}
</script>

<template>
  <div class="page mt-4">
    <n-card size="small">
      <n-spin :show="isLoading">
        <n-empty v-if="!isLoading && providers.length === 0" :description="t('dns.noProvider')" />

        <div v-else class="divide-y divide-neutral-200 dark:divide-neutral-700">
          <div
            v-for="p in providers"
            :key="p.id"
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
                    d="M21 12a9 9 0 01-9 9m9-9a9 9 0 00-9-9m9 9H3m9 9a9 9 0 01-9-9m9 9c1.657 0 3-4.03 3-9s-1.343-9-3-9m0 18c-1.657 0-3-4.03-3-9s1.343-9 3-9"
                  />
                </svg>
              </div>
              <div>
                <div class="flex items-center gap-2">
                  <h3 class="font-medium">{{ p.name }}</h3>
                  <n-tag size="small" :bordered="false">{{
                    getProviderLabel(p.provider_type)
                  }}</n-tag>
                  <n-tag v-if="!p.is_active" size="small" :bordered="false">{{
                    t('dns.disabled')
                  }}</n-tag>
                </div>
                <p v-if="p.comment" class="text-sm mt-1 opacity-50">{{ p.comment }}</p>
              </div>
            </div>
            <div class="flex items-center gap-1">
              <n-button
                quaternary
                circle
                size="small"
                @click="openEdit(p)"
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
                @click="openDeleteModal(p.id)"
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
      :title="editingId ? t('dns.editProvider') : t('dns.addProviderTitle')"
      style="max-width: 560px"
    >
      <n-form label-placement="top">
        <n-form-item :label="t('dns.name')">
          <n-input v-model:value="formData.name" :placeholder="t('dns.namePlaceholder')" />
        </n-form-item>
        <n-form-item :label="t('dns.providerType')">
          <n-select v-model:value="formData.provider_type" :options="providerTypeOptions" />
        </n-form-item>
        <!-- 动态配置字段 -->
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
