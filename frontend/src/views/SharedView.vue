<script setup lang="ts">
import { computed, onMounted, ref, shallowRef, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useDisplay } from 'vuetify'

import { api, type Album } from '../api/client'
import AlbumCard from '../components/AlbumCard.vue'
import AppPageHeader from '../components/AppPageHeader.vue'
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
const { xs } = useDisplay()

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

// The switch shows in the header or, on a phone, above the photos.
const includesAlbums = computed(() => ({
  modelValue: auth.user?.sharedIncludesAlbums,
  disabled: saving.value,
  label: 'Include photos from shared albums',
  color: 'primary',
  density: 'compact' as const,
  hideDetails: true,
  'onUpdate:modelValue': setIncludesAlbums,
}))

function albumSubtitle(album: Album) {
  const owner = users.login(album.ownerId)
  return owner ? `${album.photoCount} photos · ${owner}` : `${album.photoCount} photos`
}
</script>

<template>
  <!-- Both tabs share the header and the tab row, so switching keeps them in
       place and only the content below changes. The switch for the photos
       tab sits in the header; on a phone it would not fit beside the title,
       so it heads the photos instead. -->
  <div class="shared">
    <AppPageHeader title="Shared with me">
      <template v-if="tab === 'photos' && !xs" #actions>
        <v-switch v-bind="includesAlbums" />
      </template>
    </AppPageHeader>
    <div class="px-4 px-sm-6 pb-2">
      <v-tabs :model-value="tab" @update:model-value="chooseTab">
        <v-tab value="albums" text="Albums" />
        <v-tab value="photos" text="Photos" />
      </v-tabs>
      <v-alert v-if="error" type="error" density="compact" class="mt-2" :title="error" />
    </div>

    <div v-if="tab === 'albums'" class="page page-section pt-2">
      <EmptyState v-if="albumsLoaded && albums.length === 0" icon="mdi-image-album" title="No albums are shared with you" />
      <div class="albums">
        <AlbumCard v-for="album in albums" :key="album.id" :album="album" :subtitle="albumSubtitle(album)" />
      </div>
    </div>

    <PhotoBrowser v-else mode="shared" class="photos">
      <template v-if="xs" #toolbar>
        <div class="px-4 pb-2">
          <v-switch v-bind="includesAlbums" />
        </div>
      </template>
      <template #empty>
        <EmptyState icon="mdi-account-multiple-outline" title="No photos are shared with you" />
      </template>
    </PhotoBrowser>
  </div>
</template>

<style scoped>
.shared {
  display: flex;
  flex-direction: column;
  height: 100%;
}

.shared > .page,
.shared > .photos {
  flex: 1;
  min-height: 0;
}

.albums {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
  gap: 16px;
}
</style>
