<script setup lang="ts">
import { onMounted, ref, useTemplateRef } from 'vue'

import { api } from '../api/client'
import AppPageHeader from '../components/AppPageHeader.vue'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import EmptyState from '../components/EmptyState.vue'
import PhotoBrowser from '../components/PhotoBrowser.vue'
import { usePhotosStore } from '../stores/photos'
import { t } from '../i18n'

const photos = usePhotosStore()
const browser = useTemplateRef('browser')

const confirmOpen = ref(false)
const emptying = ref(false)

onMounted(() => void photos.open({ trash: true }))

async function empty() {
  emptying.value = true
  const { response } = await api.POST('/photos/trash/empty')
  emptying.value = false
  browser.value?.clearSelection()
  browser.value?.say(response.ok ? t('trash.emptied') : t('trash.emptyFailed'), response.ok ? 'success' : 'error')
  await photos.reload()
}
</script>

<template>
  <PhotoBrowser ref="browser" mode="trash">
    <template #header>
      <AppPageHeader :title="t('nav.trash')" :description="t('trash.description')">
        <template #actions>
          <v-btn
            prepend-icon="fa-trash-can"
            :text="t('trash.empty')"
            color="error"
            variant="outlined"
            :disabled="photos.photos.length === 0"
            :loading="emptying"
            @click="confirmOpen = true"
          />
        </template>
      </AppPageHeader>
    </template>
    <template #empty>
      <EmptyState icon="fa-trash-can" :title="t('trash.emptied')" />
    </template>
  </PhotoBrowser>
  <ConfirmDialog
    v-model="confirmOpen"
    :title="t('trash.confirmTitle')"
    :text="t('trash.confirmText')"
    :action="t('trash.empty')"
    @confirm="empty"
  />
</template>
