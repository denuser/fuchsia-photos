<script setup lang="ts">
import { computed, onMounted, ref, shallowRef, useTemplateRef } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import { api, type Album } from '../api/client'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import EmptyState from '../components/EmptyState.vue'
import NameDialog from '../components/NameDialog.vue'
import PhotoBrowser from '../components/PhotoBrowser.vue'
import { inBatches, usePhotosStore } from '../stores/photos'

const route = useRoute()
const router = useRouter()
const photos = usePhotosStore()
const browser = useTemplateRef('browser')

const id = computed(() => Number(route.params.id))
const album = shallowRef<Album | null>(null)
const renameOpen = ref(false)
const deleteOpen = ref(false)

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
    browser.value?.say(error.error)
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
    browser.value?.say((failure as Error).message)
  }
}
</script>

<template>
  <PhotoBrowser ref="browser" mode="library" @tagged="loadAlbum">
    <template #toolbar>
      <div class="d-flex align-center pa-2 ga-1">
        <v-btn to="/albums" icon="mdi-arrow-left" size="small" variant="text" aria-label="Albums" />
        <div class="text-subtitle-1 text-truncate">{{ album?.name }}</div>
        <div class="text-body-2 text-medium-emphasis ml-2 flex-grow-1">{{ album?.photoCount }} photos</div>
        <v-menu>
          <template #activator="{ props }">
            <v-btn v-bind="props" icon="mdi-dots-vertical" size="small" variant="text" aria-label="Album actions" />
          </template>
          <v-list density="compact">
            <v-list-item title="Rename" prepend-icon="mdi-pencil" @click="renameOpen = true" />
            <v-list-item v-if="album?.coverChosen" title="Use the newest photo as the cover" prepend-icon="mdi-image-refresh" @click="resetCover" />
            <v-list-item title="Delete album" prepend-icon="mdi-delete" @click="deleteOpen = true" />
          </v-list>
        </v-menu>
      </div>
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
      <EmptyState icon="mdi-image-album" title="This album is empty" text="Select photos in the timeline and add them here." />
    </template>
  </PhotoBrowser>

  <NameDialog v-model="renameOpen" title="Rename album" label="Name" action="Rename" :initial="album?.name" :save="rename" />
  <ConfirmDialog
    v-model="deleteOpen"
    title="Delete this album?"
    :text="`${album?.name} will be deleted. Its photos stay in your library.`"
    action="Delete"
    @confirm="remove"
  />
</template>
