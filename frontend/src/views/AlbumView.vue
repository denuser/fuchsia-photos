<script setup lang="ts">
import { computed, onMounted, ref, shallowRef, useTemplateRef } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import { api, type Album } from '../api/client'
import AppPageHeader from '../components/AppPageHeader.vue'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import EmptyState from '../components/EmptyState.vue'
import { apiError } from '../i18n/apiErrors'
import NameDialog from '../components/NameDialog.vue'
import PhotoBrowser from '../components/PhotoBrowser.vue'
import ShareDialog from '../components/ShareDialog.vue'
import { useAuthStore } from '../stores/auth'
import { inBatches, usePhotosStore } from '../stores/photos'
import { t } from '../i18n'

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
const breadcrumbs = computed(() => [owned.value ? { title: t('nav.albums'), to: '/albums' } : { title: t('nav.shared'), to: '/shared' }])
const photoCount = computed(() => t('common.photos', album.value?.photoCount ?? 0))

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
    return apiError(error)
  }
  album.value = data
  return null
}

async function remove() {
  const { error } = await api.DELETE('/albums/{id}', { params: { path: { id: id.value } } })
  if (error) {
    browser.value?.say(apiError(error), 'error')
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
    throw new Error(apiError(error))
  }
  album.value = data
}

async function resetCover() {
  try {
    await setCover(null)
    browser.value?.say(t('album.coverReset'))
  } catch (failure) {
    browser.value?.say(apiError((failure as Error).message), 'error')
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
              <v-btn v-bind="props" icon="fa-ellipsis-vertical" variant="text" :aria-label="t('albums.actions')" />
            </template>
            <v-list density="compact">
              <v-list-item :title="t('common.rename')" prepend-icon="fa-pen" @click="renameOpen = true" />
              <v-list-item :title="t('browser.share')" prepend-icon="fa-share-nodes" @click="shareOpen = true" />
              <v-list-item
                v-if="album.coverChosen"
                :title="t('album.resetCover')"
                prepend-icon="fa-arrow-rotate-left"
                @click="resetCover"
              />
              <v-list-item :title="t('album.delete')" prepend-icon="fa-trash-can" @click="deleteOpen = true" />
            </v-list>
          </v-menu>
        </template>
      </AppPageHeader>
    </template>
    <template #selection-actions="{ ids, run }">
      <v-btn
        v-if="ids.length === 1"
        prepend-icon="fa-image"
        :text="t('album.setCover')"
        @click="run(() => setCover(ids[0]!), t('album.coverChanged'))"
      />
      <v-btn
        prepend-icon="fa-circle-minus"
        :text="t('album.remove')"
        @click="run(() => removeFromAlbum(ids), t('album.removed', ids.length))"
      />
    </template>
    <template #empty>
      <EmptyState v-if="owned" icon="fa-book-open" :title="t('album.empty')" :text="t('album.emptyText')">
        <template #actions>
          <v-btn :text="t('common.openTimeline')" color="primary" to="/" />
        </template>
      </EmptyState>
      <EmptyState v-else icon="fa-book-open" :title="t('album.empty')" />
    </template>
  </PhotoBrowser>

  <ShareDialog v-model="shareOpen" :album-id="id" @done="loadAlbum" />
  <NameDialog
    v-model="renameOpen"
    :title="t('albums.rename')"
    :label="t('common.name')"
    :action="t('common.rename')"
    :initial="album?.name" :save="rename" />
  <ConfirmDialog
    v-model="deleteOpen"
    :title="t('albums.deleteTitle')"
    :text="t('albums.deleteText', { name: album?.name })"
    :action="t('common.delete')"
    @confirm="remove"
  />
</template>
