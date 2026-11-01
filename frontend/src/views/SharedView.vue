<script setup lang="ts">
import { computed, onMounted, ref, shallowRef, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import { api, type Album } from '../api/client'
import AlbumCard from '../components/AlbumCard.vue'
import EmptyState from '../components/EmptyState.vue'
import PhotoBrowser from '../components/PhotoBrowser.vue'
import { useAuthStore } from '../stores/auth'
import { usePhotosStore } from '../stores/photos'
import { useUsersStore } from '../stores/users'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const photos = usePhotosStore()
const users = useUsersStore()

// The address holds the tab, so that it survives a reload.
const tab = computed(() => (route.query.tab === 'photos' ? 'photos' : 'albums'))
const albums = shallowRef<Album[]>([])
const albumsLoaded = ref(false)
const saving = ref(false)
const error = ref('')

async function showTab() {
  if (tab.value === 'photos') {
    await photos.open({ shared: true })
  } else {
    const { data } = await api.GET('/albums/shared')
    albums.value = data ?? []
    albumsLoaded.value = true
  }
}

onMounted(() => {
  void users.load()
  void showTab()
})
watch(tab, () => void showTab())

function chooseTab(next: unknown) {
  void router.replace({ query: next === 'photos' ? { tab: 'photos' } : {} })
}

async function setIncludesAlbums(value: boolean | null) {
  saving.value = true
  error.value = ''
  const { data, error: failure } = await api.PATCH('/me', { body: { sharedIncludesAlbums: Boolean(value) } })
  saving.value = false
  if (!data) {
    error.value = failure?.error ?? 'The setting could not be saved'
    return
  }
  auth.user = data
  await photos.reload()
}

function albumSubtitle(album: Album) {
  const owner = users.login(album.ownerId)
  return owner ? `${album.photoCount} photos · ${owner}` : `${album.photoCount} photos`
}
</script>

<template>
  <div v-if="tab === 'albums'" class="page">
    <v-tabs :model-value="tab" density="compact" class="mb-4" @update:model-value="chooseTab">
      <v-tab value="albums" text="Albums" />
      <v-tab value="photos" text="Photos" />
    </v-tabs>
    <EmptyState v-if="albumsLoaded && albums.length === 0" icon="mdi-image-album" title="No albums are shared with you" />
    <div class="albums">
      <AlbumCard v-for="album in albums" :key="album.id" :album="album" :subtitle="albumSubtitle(album)" />
    </div>
  </div>

  <PhotoBrowser v-else mode="shared">
    <template #toolbar>
      <div class="d-flex flex-wrap align-center ga-4 pr-4">
        <v-tabs :model-value="tab" density="compact" class="flex-grow-1" @update:model-value="chooseTab">
          <v-tab value="albums" text="Albums" />
          <v-tab value="photos" text="Photos" />
        </v-tabs>
        <v-alert v-if="error" type="error" density="compact" :text="error" />
        <v-switch
          :model-value="auth.user?.sharedIncludesAlbums"
          :disabled="saving"
          label="Include photos from shared albums"
          color="primary"
          density="compact"
          hide-details
          class="flex-grow-0"
          @update:model-value="setIncludesAlbums"
        />
      </div>
    </template>
    <template #empty>
      <EmptyState icon="mdi-account-multiple-outline" title="No photos are shared with you" />
    </template>
  </PhotoBrowser>
</template>

<style scoped>
.page {
  height: calc(100vh - var(--v-layout-top, 0px));
  padding: 16px;
  overflow-y: auto;
}

.albums {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
  gap: 16px;
}
</style>
