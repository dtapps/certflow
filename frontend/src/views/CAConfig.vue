<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { storeToRefs } from 'pinia'
import {
  NCard,
  NButton,
  NInput,
  NInputGroup,
  NSwitch,
  NSpin,
  NModal,
  NForm,
  NFormItem,
  NEmpty,
  NTag,
  NAlert,
  NCheckbox,
  useMessage,
} from 'naive-ui'
import * as CAService from '@bindings/cnb.cool/dtapp/certflow/caservicewrapper'
import * as BrowserService from '@bindings/cnb.cool/dtapp/certflow/browserservicewrapper'
import type { CAListItem } from '@bindings/cnb.cool/dtapp/certflow/models'
import { useI18nStore } from '../stores/i18n'
import { useThemeStore } from '../stores/theme'
import { initMessage, showMessage } from '../utils/message'
import CAIcon from '../components/CAIcon.vue'

const i18nStore = useI18nStore()
const { t } = i18nStore
const message = useMessage()
initMessage(message)

const { isDark } = storeToRefs(useThemeStore())

// 暗色适配：本项目 Tailwind 的 dark: 变体不生效，改用 isDark 控制卡片配色
const cardBorder = computed(() => (isDark.value ? 'rgba(255,255,255,0.08)' : 'rgba(0,0,0,0.06)'))
const iconBg = computed(() => (isDark.value ? 'rgba(255,255,255,0.08)' : 'rgba(0,0,0,0.04)'))
const selectedCardStyle = {
  borderColor: '#2080f0',
  boxShadow: '0 0 0 2px rgba(32,128,240,0.25)',
}

const cas = ref<CAListItem[]>([])
const isLoading = ref(false)
const showModal = ref(false)
const editingCA = ref<number | null>(null)
const editingIsBuiltin = ref(false)

const formData = ref({
  name: '',
  directory_url: '',
  account_email: '',
  eab_kid: '',
  eab_hmac: '',
})

// 仅当 CA 类型需要 EAB 时才显示 EAB 获取说明
const eabRequiredDirs = [
  'https://acme.zerossl.com/v2/DV90/directory',
  'https://acme.litessl.com/acme/v2/directory',
]
const showEabHint = computed(() => eabRequiredDirs.includes(formData.value.directory_url))

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
  editingIsBuiltin.value = false
  formData.value = {
    name: '',
    directory_url: '',
    account_email: '',
    eab_kid: '',
    eab_hmac: '',
  }
  showModal.value = true
}

const openEdit = (ca: (typeof cas.value)[0]) => {
  editingCA.value = ca.id
  editingIsBuiltin.value = ca.is_builtin
  formData.value = {
    name: ca.name,
    directory_url: ca.directory_url,
    account_email: ca.account_email,
    eab_kid: ca.eab_kid || '',
    eab_hmac: ca.eab_hmac || '',
  }
  showModal.value = true
}

const handleSave = async () => {
  // 表单必填与格式校验
  const name = formData.value.name.trim()
  const dirURL = formData.value.directory_url.trim()
  const email = formData.value.account_email.trim()
  if (!name) {
    showMessage(t('ca.nameRequired'), 'warning')
    return
  }
  if (!dirURL) {
    showMessage(t('ca.directoryURLRequired'), 'warning')
    return
  }
  if (!email) {
    showMessage(t('ca.emailRequired'), 'warning')
    return
  }
  if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)) {
    showMessage(t('ca.emailInvalid'), 'warning')
    return
  }
  try {
    if (editingCA.value) {
      await CAService.UpdateCA(editingCA.value, formData.value)
    } else {
      await CAService.CreateCA(formData.value)
    }
    showModal.value = false
    cas.value = (await CAService.ListCA()) ?? []
    showMessage(t('ca.saveSuccess'), 'success')
  } catch (e) {
    showMessage(t('ca.saveFailed') + ' ' + e, 'error')
  }
}

// 验证目录 URL 是否可访问（不依赖已有 CA 记录）
const isTesting = ref(false)
const handleTestDirectoryURL = async () => {
  const dirURL = formData.value.directory_url.trim()
  if (!dirURL) {
    showMessage(t('ca.directoryURLRequired'), 'warning')
    return
  }
  isTesting.value = true
  try {
    const msg = await CAService.CheckDirectoryURL(dirURL)
    showMessage(msg, 'success')
  } catch (e) {
    showMessage(String(e), 'error')
  } finally {
    isTesting.value = false
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
    await CAService.DeleteCA(id)
    cas.value = cas.value.filter((c) => c.id !== id)
    showMessage(t('ca.deleteSuccess'), 'success')
  } catch (e) {
    showMessage(t('ca.deleteFailed') + ' ' + e, 'error')
  }
}

// 在列表中切换 CA 启用状态（独立接口，统一校验由后端完成）
const toggleActive = async (ca: (typeof cas.value)[0], val: boolean) => {
  try {
    await CAService.SetCAActive(ca.id, val)
    ca.is_active = val
    showMessage(val ? t('ca.enabledSuccess') : t('ca.disabledSuccess'), 'success')
  } catch (e) {
    showMessage(t('ca.toggleFailed') + ' ' + e, 'error')
  }
}

// 通过系统默认浏览器打开 FreeSSL（获取 EAB 凭据）
const openFreessl = () => {
  BrowserService.OpenURL('https://freessl.cn/').catch((e) =>
    console.error(t('ca.eabLinkFailed'), e),
  )
}

// 批量操作
const selectedIds = ref<number[]>([])
const isSelected = (id: number) => selectedIds.value.includes(id)
const isAllSelected = computed(
  () => cas.value.length > 0 && cas.value.every((c) => selectedIds.value.includes(c.id)),
)
const isIndeterminate = computed(() => {
  const n = cas.value.filter((c) => selectedIds.value.includes(c.id)).length
  return n > 0 && n < cas.value.length
})
const toggleSelect = (id: number) => {
  const i = selectedIds.value.indexOf(id)
  if (i >= 0) selectedIds.value.splice(i, 1)
  else selectedIds.value.push(id)
}
const toggleSelectAll = () => {
  selectedIds.value = isAllSelected.value ? [] : cas.value.map((c) => c.id)
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
      } catch (e) {
        showMessage(`${t('ca.batchFailed')} ${String(e)}`, 'error')
      }
    }),
  )
  if (ok > 0) {
    showMessage(t(successKey, { count: ok }), 'success')
  }
  clearSelection()
  await loadCAs()
}

async function loadCAs() {
  try {
    cas.value = (await CAService.ListCA()) ?? []
  } catch (e) {
    console.error(t('ca.loadFailed'), e)
  }
}

const batchEnable = () => runBatch((id) => CAService.SetCAActive(id, true), 'ca.batchEnabled')
const batchDisable = () => runBatch((id) => CAService.SetCAActive(id, false), 'ca.batchDisabled')

const showBatchDeleteModal = ref(false)
const batchDeleting = ref(false)

function batchDelete() {
  if (selectedIds.value.length === 0) return
  showBatchDeleteModal.value = true
}

async function confirmBatchDelete() {
  // 内置 CA 不可删除，先过滤掉
  const ids = selectedIds.value.slice()
  const deletableIds = ids.filter((id) => {
    const ca = cas.value.find((c) => c.id === id)
    return ca ? !ca.is_builtin : true
  })
  const builtinSkipped = ids.length - deletableIds.length

  let ok = 0
  await Promise.all(
    deletableIds.map(async (id) => {
      try {
        await CAService.DeleteCA(id)
        ok++
      } catch (e) {
        showMessage(`${t('ca.batchFailed')} ${String(e)}`, 'error')
      }
    }),
  )
  if (ok > 0) {
    showMessage(t('ca.batchDeleted', { count: ok }), 'success')
  }
  if (builtinSkipped > 0) {
    showMessage(t('ca.batchBuiltinSkip', { count: builtinSkipped }), 'warning')
  }
  showBatchDeleteModal.value = false
  clearSelection()
  await loadCAs()
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

    <!-- 批量操作栏 -->
    <div v-if="cas.length > 0" class="flex items-center justify-between mb-3 px-1">
      <div class="flex items-center gap-2">
        <n-checkbox
          :checked="isAllSelected"
          :indeterminate="isIndeterminate"
          @update:checked="toggleSelectAll"
        >
          {{ t('ca.selectAll') }}
        </n-checkbox>
        <span v-if="selectedIds.length > 0" class="text-sm opacity-60">{{
          t('ca.selectedCount', { count: selectedIds.length })
        }}</span>
      </div>
      <div v-if="selectedIds.length > 0" class="flex items-center gap-2">
        <n-button size="small" type="primary" secondary @click="batchEnable">{{
          t('ca.batchEnable')
        }}</n-button>
        <n-button size="small" secondary @click="batchDisable">{{ t('ca.batchDisable') }}</n-button>
        <n-button size="small" type="error" secondary @click="batchDelete">{{
          t('ca.batchDelete')
        }}</n-button>
        <n-button size="small" quaternary @click="clearSelection">{{
          t('common.cancel')
        }}</n-button>
      </div>
    </div>

    <n-card size="small">
      <n-spin :show="isLoading">
        <n-empty v-if="!isLoading && cas.length === 0" :description="t('ca.noCA')">
          <template #extra>
            <p class="text-sm opacity-50">{{ t('ca.noCADesc') }}</p>
          </template>
        </n-empty>

        <div v-else>
          <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4 p-6">
            <n-card
              v-for="ca in cas"
              :key="ca.id"
              size="small"
              hoverable
              class="relative"
              :style="isSelected(ca.id) ? selectedCardStyle : undefined"
            >
              <!-- 选择框 -->
              <span class="absolute top-3 right-3 z-10" @click.stop>
                <n-checkbox
                  size="small"
                  :checked="isSelected(ca.id)"
                  @update:checked="toggleSelect(ca.id)"
                />
              </span>

              <!-- 头部：图标 + 名称 + 标签 -->
              <div class="flex items-start gap-3 pr-6">
                <div
                  class="w-10 h-10 rounded-lg flex items-center justify-center shrink-0"
                  :style="{ background: iconBg }"
                >
                  <CAIcon :directory-url="ca.directory_url" :name="ca.name" :size="22" />
                </div>
                <div class="min-w-0 flex-1">
                  <div class="flex flex-wrap items-center gap-2">
                    <h3 class="font-medium truncate">{{ ca.name }}</h3>
                    <n-tag v-if="ca.is_builtin" size="small" type="info" :bordered="false">{{
                      t('ca.builtin')
                    }}</n-tag>
                    <n-tag v-if="!ca.is_active" size="small" :bordered="false">{{
                      t('ca.disabled')
                    }}</n-tag>
                  </div>
                  <p class="text-sm mt-1 truncate opacity-50" :title="ca.directory_url">
                    {{ ca.directory_url }}
                  </p>
                  <p class="text-xs mt-0.5 truncate opacity-50">{{ ca.account_email }}</p>
                </div>
              </div>

              <!-- 操作区 -->
              <div
                class="flex items-center justify-end gap-1 mt-3 pt-3"
                :style="{ borderTop: `1px solid ${cardBorder}` }"
              >
                <n-switch
                  :value="ca.is_active"
                  @update:value="(v: boolean) => toggleActive(ca, v)"
                  :title="ca.is_active ? t('ca.disableTitle') : t('ca.enableTitle')"
                />
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
                  :disabled="ca.is_builtin"
                  :title="ca.is_builtin ? t('ca.builtinNoDelete') : t('ca.deleteTitle')"
                  @click="openDeleteModal(ca.id)"
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
            </n-card>
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
          <n-input
            v-model:value="formData.name"
            :placeholder="t('ca.namePlaceholder')"
            :disabled="editingIsBuiltin"
          />
        </n-form-item>
        <n-form-item :label="t('ca.directoryURL')">
          <n-input-group>
            <n-input
              v-model:value="formData.directory_url"
              :placeholder="t('ca.directoryURLPlaceholder')"
              :disabled="editingIsBuiltin"
            />
            <n-button type="primary" secondary :loading="isTesting" @click="handleTestDirectoryURL">
              {{ t('ca.testDirectoryURL') }}
            </n-button>
          </n-input-group>
        </n-form-item>
        <n-alert v-if="editingIsBuiltin" type="warning" :show-icon="true" class="mt-1 mb-2">
          {{ t('ca.builtinReadonlyHint') }}
        </n-alert>
        <n-form-item :label="t('ca.accountEmail')">
          <n-input
            v-model:value="formData.account_email"
            :placeholder="t('ca.accountEmailPlaceholder')"
            :input-props="{ type: 'email', autocomplete: 'email' }"
          />
        </n-form-item>
        <n-form-item :label="t('ca.eabKid')">
          <n-input v-model:value="formData.eab_kid" :placeholder="t('ca.eabKidPlaceholder')" />
        </n-form-item>
        <n-form-item :label="t('ca.eabHmac')">
          <n-input
            v-model:value="formData.eab_hmac"
            type="password"
            show-password-on="click"
            :placeholder="t('ca.eabHmacPlaceholder')"
          />
        </n-form-item>
        <n-alert v-if="showEabHint" type="info" :show-icon="true" class="mt-1 mb-2">
          <span>{{ t('ca.eabHint') }}</span>
          <a href="#" @click.prevent="openFreessl" class="underline">freessl.cn</a>
        </n-alert>
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

    <!-- 批量删除确认弹窗 -->
    <n-modal v-model:show="showBatchDeleteModal" preset="dialog" :title="t('ca.batchDelete')">
      <p>{{ t('ca.batchDeleteConfirm', { count: selectedIds.length }) }}</p>
      <p class="text-sm opacity-60 mt-2">{{ t('ca.batchDeleteConfirmDesc') }}</p>
      <template #action>
        <n-button @click="showBatchDeleteModal = false">{{ t('common.cancel') }}</n-button>
        <n-button type="error" :loading="batchDeleting" @click="confirmBatchDelete">{{
          t('common.confirm')
        }}</n-button>
      </template>
    </n-modal>
  </div>
</template>
