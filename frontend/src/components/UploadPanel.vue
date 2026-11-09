<script setup lang="ts">
import { computed } from 'vue'

import { useUploadsStore } from '../stores/uploads'

const uploads = useUploadsStore()

const title = computed(() => {
  const finished = uploads.done + uploads.failures.length
  return uploads.running ? `Uploading ${finished} of ${uploads.total}` : `Uploaded ${uploads.done} of ${uploads.total}`
})

const subtitle = computed(() => {
  const parts = []
  if (uploads.duplicates) {
    parts.push(`${uploads.duplicates} already in your library`)
  }
  if (uploads.restored) {
    parts.push(`${uploads.restored} restored from the trash`)
  }
  return parts.join(', ') || undefined
})
</script>

<template>
  <v-card v-if="uploads.visible" class="upload-panel" elevation="8" width="360">
    <v-card-item :title="title" :subtitle="subtitle">
      <template #append>
        <v-btn v-if="!uploads.running" icon="fa-xmark" size="small" variant="text" aria-label="Close" @click="uploads.dismiss()" />
      </template>
    </v-card-item>
    <v-progress-linear :model-value="uploads.progress" color="primary" />

    <template v-if="uploads.failures.length > 0">
      <v-list density="compact" max-height="200" class="overflow-y-auto">
        <v-list-item
          v-for="failure in uploads.failures"
          :key="failure.file.name + failure.file.size"
          :title="failure.file.name"
          :subtitle="failure.error"
          prepend-icon="fa-circle-exclamation"
        />
      </v-list>
      <v-card-actions v-if="!uploads.running">
        <v-spacer />
        <v-btn text="Retry failed" color="primary" @click="uploads.retryFailed()" />
      </v-card-actions>
    </template>
  </v-card>
</template>

<style scoped>
.upload-panel {
  position: fixed;
  right: 16px;
  bottom: 16px;
  z-index: 1100;
}
</style>
