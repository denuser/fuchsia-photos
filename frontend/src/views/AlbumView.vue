<script setup lang="ts">
import { computed, onMounted, ref, shallowRef, useTemplateRef } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import { api, type Album } from '../api/client'
import AppPageHeader from '../components/AppPageHeader.vue'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import EmptyState from '../components/EmptyState.vue'
import NameDialog from '../components/NameDialog.vue'
import PhotoBrowser from '../components/PhotoBrowser.vue'
import ShareDialog from '../components/ShareDialog.vue'
import { useAuthStore } from '../stores/auth'
import { inBatches, usePhotosStore } from '../stores/photos'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const photos = usePhotosStore()
const browser = useTemplateRef('browser')

const id = computed(() => Number(route.params.id))
const album = shallowRef<Album | null>(null)
const renameOpen = ref(false)
const deleteOpen = ref(false)
const shareOpen = ref(false)
// Another user's album is read-only here, even for an administrator.
const owned = computed(() => album.value?.ownerId === auth.user?.id)
const breadcrumbs = computed(() => [owned.value ? { title: 'Albums', to: '/albums' } : { title: 'Shared with me', to: '/shared' }])
const photoCount = computed(() => {
  const n = album.value?.photoCount ?? 0
  return n === 1 ? '1 photo' : `${n} photos`
})

onMounted(async () => {
  await loadAlbum()
  if (album.value) {
    await photos.open({ album: id.value })
  }
})

async function loadAlbum() {
  const { data } = await api.GET('/albums/{id}', { params: { path: { id: id.value } } })
  if (!data) {
    await router.replace('/albums')
    return
  }
  album.value = data
}

async function rename(name: string) {
  const { data, error } = await api.PATCH('/albums/{id}', { params: { path: { id: id.value } }, body: { name } })
  if (error) {
    return error.error
  }
  album.value = data
  return null
}

async function remove() {
  const { error } = await api.DELETE('/albums/{id}', { params: { path: { id: id.value } } })
  if (error) {
    browser.value?.say(error.error, 'error')
    return
  }
  await router.push('/albums')
}

async function removeFromAlbum(ids: number[]) {
  await inBatches(ids, (batch) => api.POST('/albums/{id}/photos/remove', { params: { path: { id: id.value } }, body: { ids: batch } }))
  photos.remove(ids)
  await loadAlbum()
}

async function setCover(photoId: number | null) {
  const { data, error } = await api.PUT('/albums/{id}/cover', { params: { path: { id: id.value } }, body: { photoId } })
  if (error) {
    throw new Error(error.error)
  }
  album.value = data
}

async function resetCover() {
  try {
    await setCover(null)
    browser.value?.say('The newest photo is the cover again')
  } catch (failure) {
    browser.value?.say((failure as Error).message, 'error')
  }
}
</script>

<template>
  <PhotoBrowser v-if="album" ref="browser" :mode="owned ? 'library' : 'shared'" @tagged="loadAlbum">
    <template #header>
      <AppPageHeader :title="album.name" :description="photoCount" :breadcrumbs="breadcrumbs">
        <template v-if="owned" #actions>
          <v-menu>
            <template #activator="{ props }">
              <v-btn v-bind="props" icon="mdi-dots-vertical" variant="text" aria-label="Album actions" />
            </template>
            <v-list density="compact">
              <v-list-item title="Rename" prepend-icon="mdi-pencil" @click="renameOpen = true" />
              <v-list-item title="Share" prepend-icon="mdi-share-variant-outline" @click="shareOpen = true" />
              <v-list-item v-if="album.coverChosen" title="Use the newest photo as the cover" prepend-icon="mdi-image-refresh" @click="resetCover" />
              <v-list-item title="Delete album" prepend-icon="mdi-delete" @click="deleteOpen = true" />
            </v-list>
          </v-menu>
        </template>
      </AppPageHeader>
    </template>
    <template #selection-actions="{ ids, run }">
      <v-btn v-if="ids.length === 1" prepend-icon="mdi-image-check" text="Set as cover" @click="run(() => setCover(ids[0]!), 'Cover changed')" />
      <v-btn
        prepend-icon="mdi-image-remove"
        text="Remove from album"
        @click="run(() => removeFromAlbum(ids), ids.length === 1 ? '1 photo removed from the album' : `${ids.length} photos removed from the album`)"
      />
    </template>
    <template #empty>
      <EmptyState v-if="owned" icon="mdi-image-album" title="This album is empty" text="Select photos in the timeline and add them here.">
        <template #actions>
          <v-btn text="Open the timeline" color="primary" to="/" />
        </template>
      </EmptyState>
      <EmptyState v-else icon="mdi-image-album" title="This album is empty" />
    </template>
  </PhotoBrowser>

  <ShareDialog v-model="shareOpen" :album-id="id" @done="loadAlbum" />
  <NameDialog v-model="renameOpen" title="Rename album" label="Name" action="Rename" :initial="album?.name" :save="rename" />
  <ConfirmDialog
    v-model="deleteOpen"
    title="Delete this album?"
    :text="`${album?.name} will be deleted. Its photos stay in your library.`"
    action="Delete"
    @confirm="remove"
  />
</template>
