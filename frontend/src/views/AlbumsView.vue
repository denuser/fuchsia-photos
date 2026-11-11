<script setup lang="ts">
import { onMounted, ref, shallowRef } from 'vue'

import { api, type Album } from '../api/client'
import AlbumCard from '../components/AlbumCard.vue'
import AppPageHeader from '../components/AppPageHeader.vue'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import EmptyState from '../components/EmptyState.vue'
import { apiError } from '../i18n/apiErrors'
import NameDialog from '../components/NameDialog.vue'
import ShareDialog from '../components/ShareDialog.vue'
import { t } from '../i18n'

const albums = shallowRef<Album[]>([])
const loaded = ref(false)
const error = ref('')

const createOpen = ref(false)
const renaming = shallowRef<Album | null>(null)
const renameOpen = ref(false)
const deleting = shallowRef<Album | null>(null)
const deleteOpen = ref(false)
const sharing = shallowRef<Album | null>(null)
const shareOpen = ref(false)

onMounted(load)

async function load() {
  const { data } = await api.GET('/albums')
  albums.value = data ?? []
  loaded.value = true
}

async function create(name: string) {
  const { error: failure } = await api.POST('/albums', { body: { name } })
  if (failure) {
    return apiError(failure)
  }
  await load()
  return null
}

async function rename(name: string) {
  const { error: failure } = await api.PATCH('/albums/{id}', { params: { path: { id: renaming.value!.id } }, body: { name } })
  if (failure) {
    return apiError(failure)
  }
  await load()
  return null
}

async function remove() {
  const { error: failure } = await api.DELETE('/albums/{id}', { params: { path: { id: deleting.value!.id } } })
  error.value = failure ? apiError(failure) : ''
  await load()
}

function startRename(album: Album) {
  renaming.value = album
  renameOpen.value = true
}

function startShare(album: Album) {
  sharing.value = album
  shareOpen.value = true
}

function startDelete(album: Album) {
  deleting.value = album
  deleteOpen.value = true
}
</script>

<template>
  <div class="page">
    <AppPageHeader :title="t('nav.albums')">
      <template #actions>
        <v-btn color="primary" prepend-icon="fa-plus" :text="t('albums.new')" @click="createOpen = true" />
      </template>
    </AppPageHeader>
    <div class="page-section">
      <v-alert v-if="error" type="error" density="compact" class="mb-4" :title="error" />

      <EmptyState
        v-if="loaded && albums.length === 0"
        icon="fa-book-open"
        :title="t('albums.none')"
        :text="t('albums.noneText')"
      >
        <template #actions>
          <v-btn :text="t('albums.create')" color="primary" prepend-icon="fa-plus" @click="createOpen = true" />
        </template>
      </EmptyState>

      <div class="albums">
        <AlbumCard v-for="album in albums" :key="album.id" :album="album" :subtitle="t('common.photos', album.photoCount)">
          <template #actions>
            <v-icon v-if="album.shared" icon="fa-users" size="small" class="mr-1" :aria-label="t('albums.shared')" />
            <v-menu>
              <template #activator="{ props }">
                <v-btn v-bind="props" icon="fa-ellipsis-vertical" size="small" variant="text" :aria-label="t('albums.actions')" @click.prevent />
              </template>
              <v-list density="compact">
                <v-list-item :title="t('common.rename')" prepend-icon="fa-pen" @click="startRename(album)" />
                <v-list-item :title="t('browser.share')" prepend-icon="fa-share-nodes" @click="startShare(album)" />
                <v-list-item :title="t('common.delete')" prepend-icon="fa-trash-can" @click="startDelete(album)" />
              </v-list>
            </v-menu>
          </template>
        </AlbumCard>
      </div>

      <NameDialog v-model="createOpen" :title="t('albums.new')" :label="t('common.name')" :action="t('common.create')" :save="create" />
      <NameDialog
        v-model="renameOpen"
        :title="t('albums.rename')"
        :label="t('common.name')"
        :action="t('common.rename')"
        :initial="renaming?.name" :save="rename" />
      <ShareDialog v-model="shareOpen" :album-id="sharing?.id" @done="load" />
      <ConfirmDialog
        v-model="deleteOpen"
        :title="t('albums.deleteTitle')"
        :text="t('albums.deleteText', { name: deleting?.name })"
        :action="t('common.delete')"
        @confirm="remove"
      />
    </div>
  </div>
</template>

<style scoped>
.albums {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
  gap: 16px;
}
</style>
