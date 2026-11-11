<script setup lang="ts">
// Links a folder of the user's WebDAV tree to one of their albums. The folder may be one a sync app
// has made or a new path, as an app creates its folder only on the first sync.
import { ref, shallowRef, watch } from 'vue'

import { api, type Album, type WebDAVAlbum } from '../api/client'
import { apiError } from '../i18n/apiErrors'
import AppAutocomplete from './AppAutocomplete.vue'
import AppDialog from './AppDialog.vue'
import AppFormGroup from './AppFormGroup.vue'
import { t } from '../i18n'

const open = defineModel<boolean>({ required: true })
const emit = defineEmits<{ created: [WebDAVAlbum] }>()

const folders = shallowRef<string[]>([])
const albums = shallowRef<Album[]>([])
const folder = ref<string | null>(null)
const albumId = ref<number | null>(null)
const error = ref('')
const saving = ref(false)

watch(open, async (isOpen) => {
  if (!isOpen) {
    return
  }
  folder.value = null
  albumId.value = null
  error.value = ''
  const [foldersRes, albumsRes] = await Promise.all([api.GET('/me/webdav-folders'), api.GET('/albums')])
  folders.value = foldersRes.data ?? []
  albums.value = albumsRes.data ?? []
})

async function create() {
  if (!folder.value?.trim() || albumId.value === null) {
    return
  }
  saving.value = true
  error.value = ''
  const { data, error: failure } = await api.POST('/me/webdav-albums', { body: { path: folder.value, albumId: albumId.value } })
  saving.value = false
  if (!data) {
    error.value = apiError(failure, t('folderAlbum.failed'))
    return
  }
  open.value = false
  emit('created', data)
}
</script>

<template>
  <AppDialog
    v-model="open"
    :title="t('folderAlbum.title')"
    :action="t('folderAlbum.link')"
    :error="error"
    :loading="saving"
    :disabled="!folder?.trim() || albumId === null"
    max-width="480"
    @submit="create"
  >
    <AppFormGroup v-slot="field" :label="t('folderAlbum.folder')" required>
      <v-combobox
        v-bind="field"
        v-model="folder"
        :items="folders"
        variant="outlined"
        autofocus
        :hint="t('folderAlbum.folderHint')"
        persistent-hint
      />
    </AppFormGroup>
    <AppFormGroup v-slot="field" :label="t('folderAlbum.album')" required>
      <AppAutocomplete
        v-bind="field"
        v-model="albumId"
        :items="albums"
        item-title="name"
        item-value="id"
        :no-data-text="t('folderAlbum.noAlbums')"
      />
    </AppFormGroup>
  </AppDialog>
</template>
