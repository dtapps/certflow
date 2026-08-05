<script setup lang="ts">
import { ref } from 'vue'
import * as DeployCredentialService from '@bindings/cnb.cool/dtapp/certflow/deploycredentialservicewrapper'
import CredentialList from './CredentialList.vue'
import { useI18nStore } from '../stores/i18n'
import { deployProviderConfigSchema, deployProviderTypes } from '../utils/deployProviderConfig'

const i18nStore = useI18nStore()
const { t } = i18nStore

const credentialListRef = ref<InstanceType<typeof CredentialList>>()

const loadItems = async () => {
  const list = await DeployCredentialService.ListDeployCredentials()
  const items = list || []
  console.debug(
    t('log.deployCredentialLoad', {
      count: items.length,
      first: JSON.stringify(
        items[0]
          ? {
              name: items[0].name,
              provider_type: items[0].provider_type,
              is_active: items[0].is_active,
            }
          : null,
      ),
    }),
  )
  return items
}

const createItem = async (data: any) => {
  return await DeployCredentialService.CreateDeployCredential(data)
}

const updateItem = async (id: number, data: any) => {
  return await DeployCredentialService.UpdateDeployCredential(id, data)
}

const deleteItem = async (id: number) => {
  return await DeployCredentialService.DeleteDeployCredential(id)
}

const setActiveItem = async (id: number, active: boolean) => {
  await DeployCredentialService.SetActive(id, active)
}
</script>

<template>
  <CredentialList
    ref="credentialListRef"
    :title="t('deploy.credentialTitle')"
    :subtitle="t('deploy.credentialSubtitle')"
    :empty-text="t('deploy.credentialEmpty')"
    :create-text="t('deploy.credentialCreate')"
    :edit-text="t('deploy.credentialEdit')"
    :provider-types="deployProviderTypes"
    :config-schema="deployProviderConfigSchema"
    :load-items="loadItems"
    :create-item="createItem"
    :update-item="updateItem"
    :delete-item="deleteItem"
    :set-active-item="setActiveItem"
  />
</template>
