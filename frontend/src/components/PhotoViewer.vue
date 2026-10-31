<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, watch } from 'vue'

import { originalUrl, previewUrl, type Photo } from '../api/client'

// paused leaves the keyboard to a dialog opened over the viewer.
const props = defineProps<{ photos: Photo[]; photo: Photo; editable: boolean; paused: boolean }>()
const emit = defineEmits<{ close: []; navigate: [photo: Photo]; trash: [photo: Photo]; tags: [photo: Photo] }>()

const ready = computed(() => props.photos.filter((p) => p.status === 'ready'))
const index = computed(() => ready.value.findIndex((p) => p.id === props.photo.id))
const previous = computed(() => ready.value[index.value - 1])
const next = computed(() => ready.value[index.value + 1])

const details = computed(() => {
  const date = new Date(props.photo.takenAt ?? props.photo.createdAt)
  return [date.toLocaleString(), props.photo.width && `${props.photo.width} × ${props.photo.height}`]
    .filter(Boolean)
    .join(' · ')
})

// Loads the neighbours ahead of time, so that stepping through photos feels instant.
watch(
  [previous, next],
  (neighbours) => {
    for (const photo of neighbours) {
      if (photo) {
        new Image().src = previewUrl(photo.id, 'viewer')
      }
    }
  },
  { immediate: true },
)

function onKey(event: KeyboardEvent) {
  if (props.paused) {
    return
  }
  if (event.key === 'Escape') {
    emit('close')
  } else if (event.key === 'ArrowLeft' && previous.value) {
    emit('navigate', previous.value)
  } else if (event.key === 'ArrowRight' && next.value) {
    emit('navigate', next.value)
  }
}

onMounted(() => window.addEventListener('keydown', onKey))
onBeforeUnmount(() => window.removeEventListener('keydown', onKey))
</script>

<template>
  <div class="viewer" role="dialog" aria-modal="true" :aria-label="photo.name">
    <img :key="photo.id" :src="previewUrl(photo.id, 'viewer')" :alt="photo.name" class="image" />

    <div class="bar">
      <div class="info">
        <div class="name">{{ photo.name }}</div>
        <div class="details">{{ details }}</div>
      </div>
      <template v-if="editable">
        <v-btn icon="mdi-tag-outline" variant="text" aria-label="Tags" @click="emit('tags', photo)" />
        <v-btn icon="mdi-trash-can-outline" variant="text" aria-label="Move to trash" @click="emit('trash', photo)" />
      </template>
      <v-btn :href="originalUrl(photo.id)" icon="mdi-download" variant="text" aria-label="Download the original" />
      <v-btn icon="mdi-close" variant="text" aria-label="Close" @click="emit('close')" />
    </div>

    <v-btn
      v-if="previous"
      class="nav left"
      icon="mdi-chevron-left"
      variant="text"
      size="x-large"
      aria-label="Previous photo"
      @click="emit('navigate', previous)"
    />
    <v-btn
      v-if="next"
      class="nav right"
      icon="mdi-chevron-right"
      variant="text"
      size="x-large"
      aria-label="Next photo"
      @click="emit('navigate', next)"
    />
  </div>
</template>

<style scoped>
.viewer {
  position: fixed;
  inset: 0;
  z-index: 1200;
  display: flex;
  align-items: center;
  justify-content: center;
  background: #000;
  color: #fff;
}

.image {
  max-width: 100%;
  max-height: 100%;
  object-fit: contain;
}

.bar {
  position: absolute;
  top: 0;
  right: 0;
  left: 0;
  display: flex;
  align-items: center;
  padding: 8px 8px 24px 16px;
  background: linear-gradient(rgba(0, 0, 0, 0.7), transparent);
}

.info {
  flex: 1;
  min-width: 0;
}

.name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.details {
  font-size: 0.8rem;
  opacity: 0.75;
}

.nav {
  position: absolute;
  top: 50%;
  transform: translateY(-50%);
}

.left {
  left: 8px;
}

.right {
  right: 8px;
}
</style>
