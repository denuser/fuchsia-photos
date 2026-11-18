<script setup lang="ts">
import { computed, onMounted, onUnmounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import AppAutocomplete from '../components/AppAutocomplete.vue'
import AppPageHeader from '../components/AppPageHeader.vue'
import EmptyState from '../components/EmptyState.vue'
import PhotoBrowser from '../components/PhotoBrowser.vue'
import { usePhotosStore } from '../stores/photos'
import { useUsersStore } from '../stores/users'
import { t } from '../i18n'

const route = useRoute()
const router = useRouter()
const photos = usePhotosStore()
const users = useUsersStore()

// The address holds the filter, so that it survives a reload.
const owner = computed(() => {
  const id = Number(route.query.owner)
  return Number.isInteger(id) && id > 0 ? id : undefined
})

// The months carry their counts, so the total of the chosen owner, or of
// everyone, is known before the photos load.
const description = computed(() => {
  const text = t('allPhotos.description')
  if (!photos.loadedOnce || photos.failedToOpen) {
    return text
  }
  const total = photos.sections.reduce((sum, s) => sum + s.count, 0)
  return `${text} ${t('allPhotos.count', total)}`
})

function open() {
  void photos.open({ everyone: true, owner: owner.value })
}

// Photo events carry only the administrator's own photos, so other users'
// photos being processed, as during an import, would show only after a reload.
// The page reads the month counts again while it is in view.
const REFRESH_MS = 10_000
let timer: ReturnType<typeof setInterval> | undefined

onMounted(() => {
  void users.load()
  open()
  timer = setInterval(() => {
    if (document.visibilityState === 'visible') {
      void photos.refreshMonths()
    }
  }, REFRESH_MS)
})
onUnmounted(() => clearInterval(timer))
watch(owner, open)

function chooseOwner(id: number | null) {
  void router.replace({ query: id ? { owner: String(id) } : {} })
}
</script>

<template>
  <PhotoBrowser mode="admin">
    <template #header>
      <AppPageHeader :title="t('nav.all')" :description="description" />
    </template>
    <template #toolbar>
      <div class="d-flex align-center px-4 px-sm-6 pb-2 ga-2">
        <AppAutocomplete
          :model-value="owner"
          :items="users.everyone"
          item-title="login"
          item-value="id"
          :placeholder="t('allPhotos.anyOwner')"
          :aria-label="t('allPhotos.owner')"
          density="compact"
          hide-details
          clearable
          class="owner-filter"
          @update:model-value="chooseOwner"
        />
      </div>
    </template>
    <template #empty>
      <EmptyState icon="fa-images" :title="t('allPhotos.none')" />
    </template>
  </PhotoBrowser>
</template>

<style scoped>
.owner-filter {
  max-width: 320px;
}
</style>
