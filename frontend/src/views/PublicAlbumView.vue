<script setup lang="ts">
// What a visitor of a public link sees: the album's photos as previews, with a viewer, and a password
// form first when the link has one. It uses the public API only and needs no account.
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'

import logo from '../assets/logo.png'
import { api, type PublicAlbum, type PublicPhoto } from '../api/client'
import { apiError } from '../i18n/apiErrors'
import AppFormGroup from '../components/AppFormGroup.vue'
import { t } from '../i18n'

const route = useRoute()
const token = computed(() => String(route.params.token))
const base = computed(() => `/api/v1/public/${encodeURIComponent(token.value)}`)

const album = ref<PublicAlbum | null>(null)
const photos = ref<PublicPhoto[]>([])
const missing = ref(false)
const loading = ref(true)

const password = ref('')
const error = ref('')
const submitting = ref(false)

async function load() {
  loading.value = true
  const params = { params: { path: { token: token.value } } }
  const { data, response } = await api.GET('/public/{token}', params)
  if (!data) {
    missing.value = response.status === 404
    loading.value = false
    return
  }
  album.value = data
  if (data.unlocked) {
    const list = await api.GET('/public/{token}/photos', params)
    photos.value = list.data ?? []
  }
  loading.value = false
}

async function unlock() {
  submitting.value = true
  const { error: failure } = await api.POST('/public/{token}/unlock', { params: { path: { token: token.value } }, body: { password: password.value } })
  submitting.value = false
  if (failure) {
    error.value = apiError(failure)
    return
  }
  error.value = ''
  password.value = ''
  await load()
}

onMounted(load)
watch(token, load)
watch(
  () => album.value?.name,
  (name) => {
    document.title = name ? `${name} · Magellanica Photos` : 'Magellanica Photos'
  },
)

function preview(photo: PublicPhoto, kind: 'grid' | 'viewer') {
  return `${base.value}/photos/${photo.id}/previews/${kind}?v=${photo.previewVersion}`
}

// The viewer: one photo or video over the page, with the arrows and Escape.
const shown = ref<number | null>(null)
const current = computed(() => (shown.value === null ? null : (photos.value[shown.value] ?? null)))

function step(by: number) {
  if (shown.value !== null) {
    shown.value = Math.min(photos.value.length - 1, Math.max(0, shown.value + by))
  }
}

function onKey(event: KeyboardEvent) {
  if (shown.value === null) {
    return
  }
  if (event.key === 'Escape') {
    shown.value = null
  } else if (event.key === 'ArrowLeft') {
    step(-1)
  } else if (event.key === 'ArrowRight') {
    step(1)
  }
}

onMounted(() => window.addEventListener('keydown', onKey))
onBeforeUnmount(() => window.removeEventListener('keydown', onKey))
</script>

<template>
  <div class="publicAlbum">
    <header class="publicAlbum__bar">
      <img :src="logo" alt="" width="28" height="28" />
      <span class="publicAlbum__brand">Magellanica Photos</span>
    </header>

    <v-progress-linear v-if="loading" indeterminate />
    <v-container v-else-if="missing" class="publicAlbum__message" data-testid="public-missing">
      <h1 class="mb-2">{{ t('publicAlbum.missing') }}</h1>
      <p class="text-medium-emphasis">{{ t('publicAlbum.missingText') }}</p>
    </v-container>
    <v-container v-else-if="album && !album.unlocked" class="publicAlbum__message">
      <v-form @submit.prevent="unlock">
        <h1 class="mb-2">{{ t('publicAlbum.locked') }}</h1>
        <v-alert v-if="error" type="error" density="compact" class="mb-4" :title="error" />
        <AppFormGroup v-slot="field" :label="t('publicAlbum.password')" required>
          <v-text-field v-bind="field" v-model="password" type="password" autocomplete="off" autofocus data-testid="public-password" />
        </AppFormGroup>
        <v-btn type="submit" color="primary" block :loading="submitting" :disabled="!password" :text="t('publicAlbum.open')" />
      </v-form>
    </v-container>
    <template v-else-if="album">
      <v-container>
        <h1>{{ album.name }}</h1>
        <p class="text-medium-emphasis mb-4">{{ t('common.photos', photos.length) }}</p>
        <p v-if="photos.length === 0" class="text-medium-emphasis">{{ t('publicAlbum.empty') }}</p>
        <div class="publicAlbum__grid">
          <button
            v-for="(photo, index) in photos"
            :key="photo.id"
            type="button"
            class="publicAlbum__tile"
            data-testid="public-photo"
            :aria-label="t('publicAlbum.open')"
            @click="shown = index"
          >
            <img :src="preview(photo, 'grid')" alt="" loading="lazy" />
            <v-icon v-if="photo.kind === 'video'" icon="fa-play" class="publicAlbum__play" />
          </button>
        </div>
      </v-container>

      <div v-if="current" class="publicAlbum__viewer" role="dialog" aria-modal="true" data-testid="public-viewer" @click.self="shown = null">
        <video v-if="current.kind === 'video'" :key="current.id" :src="`${base}/photos/${current.id}/video`" :poster="preview(current, 'viewer')" controls autoplay playsinline />
        <img v-else :key="current.id" :src="preview(current, 'viewer')" alt="" />
        <v-btn class="publicAlbum__close" icon="fa-xmark" variant="text" color="white" :aria-label="t('common.close')" @click="shown = null" />
        <v-btn v-if="shown! > 0" class="publicAlbum__prev" icon="fa-angle-left" variant="text" color="white" :aria-label="t('publicAlbum.previous')" @click="step(-1)" />
        <v-btn v-if="shown! < photos.length - 1" class="publicAlbum__next" icon="fa-angle-right" variant="text" color="white" :aria-label="t('publicAlbum.next')" @click="step(1)" />
      </div>
    </template>
  </div>
</template>

<style scoped>
.publicAlbum {
  min-height: 100vh;
  background: rgb(var(--v-theme-background));
  color: rgb(var(--v-theme-on-background));
}

.publicAlbum__bar {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 16px;
  border-bottom: 1px solid rgb(var(--v-theme-on-surface), 0.12);
}

.publicAlbum__brand {
  font-weight: 500;
}

.publicAlbum__message {
  max-width: 420px;
  padding-top: 48px;
}

.publicAlbum__grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
  gap: 4px;
}

.publicAlbum__tile {
  position: relative;
  aspect-ratio: 1;
  padding: 0;
  border: 0;
  background: rgb(var(--v-theme-surface-variant), 0.2);
  cursor: pointer;
  overflow: hidden;
}

.publicAlbum__tile img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}

.publicAlbum__play {
  position: absolute;
  right: 8px;
  bottom: 8px;
  color: #fff;
  filter: drop-shadow(0 0 3px rgba(0, 0, 0, 0.7));
}

.publicAlbum__viewer {
  position: fixed;
  inset: 0;
  z-index: 2000;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(0, 0, 0, 0.92);
}

.publicAlbum__viewer img,
.publicAlbum__viewer video {
  max-width: 100%;
  max-height: 100%;
  object-fit: contain;
}

.publicAlbum__close,
.publicAlbum__prev,
.publicAlbum__next {
  position: absolute;
}

.publicAlbum__close {
  top: 8px;
  right: 8px;
}

.publicAlbum__prev,
.publicAlbum__next {
  top: 50%;
  transform: translateY(-50%);
}

.publicAlbum__prev {
  left: 8px;
}

.publicAlbum__next {
  right: 8px;
}
</style>
