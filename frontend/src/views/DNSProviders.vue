<script setup lang="ts">
import { ref, reactive, onMounted, computed, watch } from 'vue'
import * as DNSProviderService from '@bindings/cnb.cool/dtapp/certflow/dnsproviderservicewrapper'
import type { DNSProviderListItem } from '@bindings/cnb.cool/dtapp/certflow/models'
import { useI18n } from '../stores/i18n'

const { t } = useI18n()

const providers = ref<DNSProviderListItem[]>([])
const isLoading = ref(false)
const showModal = ref(false)
const editingId = ref<number | null>(null)

const formData = ref<{ name: string; provider_type: string; config: Record<string, string>; is_default: boolean; is_active: boolean; comment: string }>({ name: '', provider_type: 'cloudflare', config: {}, is_default: false, is_active: true, comment: '' })

// 动态配置表单字段
const configFields = reactive<Record<string, string>>({})

// 各提供商的配置字段定义
const providerConfigSchema: Record<string, { key: string; labelKey: string; type: 'text' | 'password' }[]> = {
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
  rainyun: [
    { key: 'api_key', labelKey: 'dns.config.api_key', type: 'text' },
  ],
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
    { key: 'personal_access_token', labelKey: 'dns.config.personal_access_token', type: 'password' },
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
  digitalocean: [
    { key: 'auth_token', labelKey: 'dns.config.auth_token', type: 'password' },
  ],
  vultr: [
    { key: 'api_key', labelKey: 'dns.config.api_key', type: 'password' },
  ],
  hetzner: [
    { key: 'api_token', labelKey: 'dns.config.api_token', type: 'password' },
  ],
  linode: [
    { key: 'token', labelKey: 'dns.config.token', type: 'password' },
  ],
  ovh: [
    { key: 'application_key', labelKey: 'dns.config.application_key', type: 'text' },
    { key: 'application_secret', labelKey: 'dns.config.application_secret', type: 'password' },
    { key: 'consumer_key', labelKey: 'dns.config.consumer_key', type: 'password' },
  ],
  dnsimple: [
    { key: 'access_token', labelKey: 'dns.config.access_token', type: 'password' },
  ],
  ns1: [
    { key: 'api_key', labelKey: 'dns.config.api_key', type: 'password' },
  ],
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
  formData.value = { name: '', provider_type: 'cloudflare', config: {}, is_default: false, is_active: true, comment: '' }
  const fields = providerConfigSchema['cloudflare'] || []
  const newConfig: Record<string, string> = {}
  fields.forEach(f => { newConfig[f.key] = '' })
  Object.assign(configFields, newConfig)
  showModal.value = true
}

const openEdit = (p: typeof providers.value[0]) => {
  editingId.value = p.id
  formData.value = { name: p.name, provider_type: p.provider_type, config: (p.config ?? {}) as Record<string, string>, is_default: p.is_default, is_active: p.is_active, comment: p.comment }
  Object.assign(configFields, p.config || {})
  showModal.value = true
}

const handleSave = async () => {
  syncConfigToMap()
  if (editingId.value) {
    await DNSProviderService.UpdateDNSProvider(editingId.value, formData.value)
  } else {
    await DNSProviderService.CreateDNSProvider(formData.value)
  }
  showModal.value = false
  providers.value = (await DNSProviderService.ListDNSProviders()) ?? []
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
    providers.value = providers.value.filter(p => p.id !== id)
  } catch (e) {
    console.error('Failed to delete DNS provider', e)
  }
}

const handleSetDefault = async (id: number) => {
  await DNSProviderService.SetDefaultDNSProvider(id)
  providers.value = (await DNSProviderService.ListDNSProviders()) ?? []
}

const getProviderLabel = (type: string) => {
  const pt = providerTypes.find(p => p.value === type)
  return pt ? t(pt.labelKey) : type
}
</script>

<template>
  <div class="page">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-base-content">{{ t('dns.title') }}</h1>
        <p class="text-content-70 text-sm mt-1">{{ t('dns.subtitle') }}</p>
      </div>
      <button @click="openCreate" class="btn btn-primary">
        <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" /></svg>
        {{ t('dns.addProvider') }}
      </button>
    </div>

    <div class="glass-panel rounded-2xl overflow-hidden">
      <div v-if="isLoading" class="flex items-center justify-center py-20">
        <div class="spinner animate-spin"></div>
      </div>
      <div v-else-if="providers.length === 0" class="text-center py-20">
        <svg class="w-20 h-20 mx-auto text-content-50 mb-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M21 12a9 9 0 01-9 9m9-9a9 9 0 00-9-9m9 9H3m9 9a9 9 0 01-9-9m9 9c1.657 0 3-4.03 3-9s-1.343-9-3-9m0 18c-1.657 0-3-4.03-3-9s1.343-9 3-9" /></svg>
        <p class="text-content-70 text-lg">{{ t('dns.noProvider') }}</p>
      </div>
      <div v-else class="list-divider">
        <div v-for="p in providers" :key="p.id" class="list-item">
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-4">
              <div class="w-12 h-12 rounded-xl bg-success-soft flex items-center justify-center">
                <svg class="w-6 h-6 text-success" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 12a9 9 0 01-9 9m9-9a9 9 0 00-9-9m9 9H3m9 9a9 9 0 01-9-9m9 9c1.657 0 3-4.03 3-9s-1.343-9-3-9m0 18c-1.657 0-3-4.03-3-9s1.343-9 3-9" /></svg>
              </div>
              <div>
                <div class="flex items-center gap-2">
                  <h3 class="text-base-content font-medium">{{ p.name }}</h3>
                  <span class="badge-tag badge-tag-muted">{{ getProviderLabel(p.provider_type) }}</span>
                  <span v-if="p.is_default" class="badge-tag badge-tag-primary">{{ t('dns.default') }}</span>
                  <span v-if="!p.is_active" class="badge-tag badge-tag-muted">{{ t('dns.disabled') }}</span>
                </div>
                <p v-if="p.comment" class="text-content-50 text-sm mt-1">{{ p.comment }}</p>
              </div>
            </div>
            <div class="flex items-center gap-1">
              <button v-if="!p.is_default" @click="handleSetDefault(p.id)" class="icon-btn" :title="t('dns.setTitle')">
                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" /></svg>
              </button>
              <button @click="openEdit(p)" class="icon-btn" :title="t('dns.editTitle')">
                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" /></svg>
              </button>
              <button @click="openDeleteModal(p.id)" class="icon-btn icon-btn-danger" :title="t('dns.deleteTitle')">
                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" /></svg>
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Modal -->
    <div v-if="showModal" class="modal modal-open">
      <div class="modal-box">
        <h3 class="font-bold text-lg mb-4">{{ editingId ? t('dns.editProvider') : t('dns.addProviderTitle') }}</h3>
        <div class="space-y-4">
          <div>
            <label class="label"><span class="label-text">{{ t('dns.name') }}</span></label>
            <input v-model="formData.name" type="text" :placeholder="t('dns.namePlaceholder')" class="input input-bordered w-full" />
          </div>
          <div>
            <label class="label"><span class="label-text">{{ t('dns.providerType') }}</span></label>
            <select v-model="formData.provider_type" class="select select-bordered w-full">
              <option v-for="pt in providerTypes" :key="pt.value" :value="pt.value">{{ t(pt.labelKey) }}</option>
            </select>
          </div>
          <!-- 动态配置字段 -->
          <div v-if="currentFields.length > 0" class="space-y-3">
            <label class="label"><span class="label-text">{{ t('dns.config') }}</span></label>
            <div v-for="field in currentFields" :key="field.key">
              <label class="label"><span class="label-text text-xs">{{ t(field.labelKey) }}</span></label>
              <input
                v-model="configFields[field.key]"
                :type="field.type"
                :placeholder="t(field.labelKey)"
                class="input input-bordered w-full"
              />
            </div>
          </div>
          <div v-else>
            <label class="label"><span class="label-text">{{ t('dns.config') }}</span></label>
            <textarea :value="JSON.stringify(formData.config, null, 2)" @input="(e: Event) => { const v = (e.target as HTMLTextAreaElement).value; try { formData.config = JSON.parse(v) } catch {} }" :placeholder="t('dns.configPlaceholder')" class="textarea textarea-bordered w-full font-mono text-sm" rows="4"></textarea>
          </div>
          <div>
            <label class="label"><span class="label-text">{{ t('dns.comment') }}</span></label>
            <input v-model="formData.comment" type="text" :placeholder="t('dns.commentPlaceholder')" class="input input-bordered w-full" />
          </div>
          <div class="flex items-center gap-4">
            <label class="flex items-center gap-2 cursor-pointer">
              <input type="checkbox" class="toggle toggle-primary" v-model="formData.is_active" />
              <span class="text-sm">{{ t('dns.enabled') }}</span>
            </label>
            <label class="flex items-center gap-2 cursor-pointer">
              <input type="checkbox" class="toggle toggle-primary" v-model="formData.is_default" />
              <span class="text-sm">{{ t('dns.setAsDefault') }}</span>
            </label>
          </div>
        </div>
        <div class="modal-action">
          <button @click="showModal = false" class="btn">{{ t('dns.cancel') }}</button>
          <button @click="handleSave" class="btn btn-primary">{{ t('dns.save') }}</button>
        </div>
      </div>
    </div>

    <!-- 删除确认弹窗 -->
    <dialog v-if="showDeleteModal" class="modal modal-open">
      <div class="modal-box glass-panel">
        <h3 class="font-bold text-lg">{{ t('dns.deleteTitle') }}</h3>
        <p class="py-4">{{ t('dns.deleteConfirm') }}</p>
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
