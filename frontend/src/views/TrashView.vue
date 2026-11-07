<script setup lang="ts">
import { onMounted, ref, useTemplateRef } from 'vue'

import { api } from '../api/client'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import EmptyState from '../components/EmptyState.vue'
import PhotoBrowser from '../components/PhotoBrowser.vue'
import { usePhotosStore } from '../stores/photos'

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
  browser.value?.say(response.ok ? 'The trash is empty' : 'Some photos could not be deleted')
  await photos.reload()
}
</script>

<template>
  <PhotoBrowser ref="browser" mode="trash">
    <template #toolbar>
      <div class="d-flex align-center pa-2 ga-2">
        <div class="text-medium-emphasis flex-grow-1">Photos stay here until you restore or delete them.</div>
        <v-btn
          prepend-icon="mdi-delete-sweep"
          text="Empty trash"
          color="error"
          variant="text"
          :disabled="photos.photos.length === 0"
          :loading="emptying"
          @click="confirmOpen = true"
        />
      </div>
    </template>
    <template #empty>
      <EmptyState icon="mdi-trash-can-outline" title="The trash is empty" />
    </template>
  </PhotoBrowser>
  <ConfirmDialog
    v-model="confirmOpen"
    title="Empty the trash?"
    text="All photos in the trash and their original files will be deleted. This cannot be undone."
    action="Empty trash"
    @confirm="empty"
  />
</template>
