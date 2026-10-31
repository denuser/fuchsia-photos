<script setup lang="ts">
import { onMounted, ref, shallowRef } from 'vue'

import type { Photo } from '../api/client'
import PhotoGrid from '../components/PhotoGrid.vue'
import PhotoViewer from '../components/PhotoViewer.vue'
import { usePhotosStore } from '../stores/photos'
import { useUploadsStore } from '../stores/uploads'

const photos = usePhotosStore()
const uploads = useUploadsStore()

const opened = shallowRef<Photo | null>(null)
const dragging = ref(false)

onMounted(() => {
  photos.connect()
  void photos.reload()
})

function onDrop(event: DragEvent) {
  dragging.value = false
  const files = Array.from(event.dataTransfer?.files ?? [])
  if (files.length > 0) {
    uploads.add(files)
  }
}
</script>

<template>
  <div class="timeline" @dragover.prevent="dragging = true" @dragleave.self="dragging = false" @drop.prevent="onDrop">
    <div v-if="photos.loadedOnce && photos.photos.length === 0" class="empty">
      <v-icon icon="mdi-image-multiple-outline" size="64" class="mb-4" />
      <div class="text-h6">No photos yet</div>
      <div class="text-medium-emphasis">Use the Upload button, or drag photos here.</div>
    </div>
    <PhotoGrid v-else :photos="photos.photos" :has-more="photos.hasMore" @open="opened = $event" @load-more="photos.loadMore()" />

    <div v-if="dragging" class="drop-hint">Drop photos to upload</div>
    <PhotoViewer v-if="opened" :photos="photos.photos" :photo="opened" @close="opened = null" @navigate="opened = $event" />
  </div>
</template>

<style scoped>
.timeline {
  position: relative;
  height: calc(100vh - var(--v-layout-top, 0px));
}

.empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
}

.drop-hint {
  position: absolute;
  inset: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  border: 2px dashed rgb(var(--v-theme-primary));
  border-radius: 8px;
  background: rgba(var(--v-theme-surface), 0.85);
  font-size: 1.25rem;
  pointer-events: none;
}
</style>
