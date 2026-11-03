<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, watch } from 'vue'

import { originalUrl, previewUrl, type Camera, type Photo } from '../api/client'
import { useAuthStore } from '../stores/auth'
import { useUsersStore } from '../stores/users'

// paused leaves the keyboard to a dialog opened over the viewer.
const props = defineProps<{ photos: Photo[]; photo: Photo; canTag: boolean; canManage: boolean; paused: boolean }>()
const emit = defineEmits<{ close: []; navigate: [photo: Photo]; trash: [photo: Photo]; tags: [photo: Photo]; share: [photo: Photo] }>()

const auth = useAuthStore()
const users = useUsersStore()

const ready = computed(() => props.photos.filter((p) => p.status === 'ready'))
const index = computed(() => ready.value.findIndex((p) => p.id === props.photo.id))
const previous = computed(() => ready.value[index.value - 1])
const next = computed(() => ready.value[index.value + 1])

const details = computed(() => {
  const date = new Date(props.photo.takenAt ?? props.photo.createdAt)
  const owner = props.photo.ownerId === auth.user?.id ? null : users.login(props.photo.ownerId)
  return [owner && `Shared by ${owner}`, date.toLocaleString(), props.photo.width && `${props.photo.width} × ${props.photo.height}`]
    .filter(Boolean)
    .join(' · ')
})

const camera = computed(() => (props.photo.camera ? describeCamera(props.photo.camera) : ''))

const location = computed(() => {
  const l = props.photo.location
  if (!l) {
    return null
  }
  const lat = l.latitude.toFixed(5)
  const lon = l.longitude.toFixed(5)
  return { text: `${lat}, ${lon}`, url: `https://www.openstreetmap.org/?mlat=${lat}&mlon=${lon}#map=15/${lat}/${lon}` }
})

// Formats the shooting details the way cameras show them, such as
// "Canon EOS R5 · 35 mm · ƒ/2.8 · 1/250 s · ISO 100".
function describeCamera(c: Camera) {
  return [
    cameraName(c),
    c.lens,
    c.focalLength && focalLength(c.focalLength, c.focalLength35mm),
    c.fNumber && `ƒ/${round(c.fNumber)}`,
    c.exposureTime && exposureTime(c.exposureTime),
    c.iso && `ISO ${c.iso}`,
    c.exposureCompensation && `${c.exposureCompensation > 0 ? '+' : '−'}${round(Math.abs(c.exposureCompensation))} EV`,
    c.flash && 'Flash',
  ]
    .filter(Boolean)
    .join(' · ')
}

// Many cameras repeat the maker in the model, as in "Canon" and "Canon EOS R5".
function cameraName(c: Camera) {
  if (!c.make || !c.model) {
    return c.model ?? c.make
  }
  const maker = c.make.split(' ')[0].toLowerCase()
  return c.model.toLowerCase().startsWith(maker) ? c.model : `${c.make} ${c.model}`
}

function focalLength(mm: number, mm35?: number) {
  const text = `${round(mm)} mm`
  return mm35 && Math.abs(mm35 - mm) >= 1 ? `${text} (${Math.round(mm35)} mm equiv.)` : text
}

// Short exposures are fractions such as 1/250, longer ones decimals such as 0.8 or 2.
function exposureTime(seconds: number) {
  const inverse = 1 / seconds
  return seconds < 1 && Math.abs(inverse - Math.round(inverse)) < 0.05 ? `1/${Math.round(inverse)} s` : `${round(seconds)} s`
}

function round(n: number) {
  return String(Math.round(n * 10) / 10)
}

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
  <div class="viewer" data-testid="viewer" role="dialog" aria-modal="true" :aria-label="photo.name">
    <img :key="photo.id" :src="previewUrl(photo.id, 'viewer')" :alt="photo.name" class="image" />

    <div class="bar">
      <div class="info">
        <div class="name">{{ photo.name }}</div>
        <div class="details">{{ details }}</div>
        <div v-if="camera || location" class="details" data-testid="camera">
          {{ camera }}<template v-if="camera && location"> · </template>
          <a v-if="location" :href="location.url" target="_blank" rel="noopener noreferrer" class="location">{{
            location.text
          }}</a>
        </div>
      </div>
      <v-btn v-if="canTag" icon="mdi-tag-outline" variant="text" aria-label="Tags" @click="emit('tags', photo)" />
      <template v-if="canManage">
        <v-btn icon="mdi-share-variant-outline" variant="text" aria-label="Share" @click="emit('share', photo)" />
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

.location {
  color: inherit;
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
