<script setup lang="ts">
import { computed, ref, watch } from 'vue'

import { useUploadsStore } from '../stores/uploads'
import { formatBytes, t } from '../i18n'

const uploads = useUploadsStore()

const title = computed(() => {
  const finished = uploads.done + uploads.failures.length
  return uploads.running
    ? t('uploads.running', { done: finished, total: uploads.total })
    : t('uploads.finished', { done: uploads.done, total: uploads.total })
})

// One line each, so that none is cut off in the narrow panel.
const lines = computed(() => {
  const parts = []
  if (uploads.running) {
    parts.push(t('uploads.bytes', { sent: formatBytes(uploads.sentBytes), total: formatBytes(uploads.totalBytes) }))
  }
  if (uploads.album) {
    parts.push(t('uploads.into', { album: uploads.album.name }))
  }
  if (uploads.duplicates) {
    parts.push(t('uploads.duplicates', { n: uploads.duplicates }))
  }
  if (uploads.restored) {
    parts.push(t('uploads.restored', { n: uploads.restored }))
  }
  return parts
})

// The skipped files of a folder are listed on request, so that it is clear
// what was left out and why.
const showSkipped = ref(false)
watch(
  () => uploads.skipped.length === 0,
  (none) => none && (showSkipped.value = false),
)
</script>

<template>
  <v-card v-if="uploads.visible" class="upload-panel" elevation="8" width="360">
    <v-card-item :title="title">
      <div v-for="line in lines" :key="line" class="line text-medium-emphasis">{{ line }}</div>
      <div v-if="uploads.skipped.length > 0" class="line text-medium-emphasis">
        {{ t('uploads.skipped', uploads.skipped.length) }}
      </div>
      <div v-if="uploads.skipped.length > 0">
        <v-btn
          variant="text"
          color="primary"
          size="small"
          density="compact"
          class="toggle"
          :text="showSkipped ? t('uploads.hideSkipped') : t('uploads.showSkipped')"
          @click="showSkipped = !showSkipped"
        />
      </div>
      <template #append>
        <v-btn v-if="!uploads.running" icon="fa-xmark" size="small" variant="text" :aria-label="t('common.close')" @click="uploads.dismiss()" />
      </template>
    </v-card-item>
    <v-list v-if="showSkipped" density="compact" max-height="200" class="overflow-y-auto">
      <v-list-item v-for="path in uploads.skipped" :key="path" class="skipped" min-height="0" :title="path" />
    </v-list>
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
        <v-btn :text="t('uploads.retry')" color="primary" @click="uploads.retryFailed()" />
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

.line {
  font-size: 0.875rem;
  overflow-wrap: anywhere;
}

/* A text button sitting in the line, its text in line with the words before it. */
.toggle {
  margin-inline-start: -8px;
}

/* The end of a path names the file, so paths wrap rather than being cut. */
.skipped {
  padding-block: 4px;
}

.skipped :deep(.v-list-item-title) {
  white-space: normal;
  overflow-wrap: anywhere;
  font-size: 0.875rem;
}
</style>
