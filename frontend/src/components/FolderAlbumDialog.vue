<script setup lang="ts">
// Links a folder of the user's WebDAV tree to one of their albums. The folder may be one a sync app
// has made or a new path, as an app creates its folder only on the first sync.
import { ref, shallowRef, watch } from 'vue'

import { api, type Album, type WebDAVAlbum } from '../api/client'
import AppAutocomplete from './AppAutocomplete.vue'
import AppDialog from './AppDialog.vue'
import AppFormGroup from './AppFormGroup.vue'

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
    error.value = failure?.error ?? 'The folder could not be linked'
    return
  }
  open.value = false
  emit('created', data)
}
</script>

<template>
  <AppDialog
    v-model="open"
    title="Link a folder to an album"
    action="Link"
    :error="error"
    :loading="saving"
    :disabled="!folder?.trim() || albumId === null"
    max-width="480"
    @submit="create"
  >
    <AppFormGroup v-slot="field" label="Folder" required>
      <v-combobox
        v-bind="field"
        v-model="folder"
        :items="folders"
        variant="outlined"
        autofocus
        hint="The server folder set in the sync app, such as /Camera. Folders inside it go to the same album unless linked themselves. / is the whole tree."
        persistent-hint
      />
    </AppFormGroup>
    <AppFormGroup v-slot="field" label="Album" required>
      <AppAutocomplete
        v-bind="field"
        v-model="albumId"
        :items="albums"
        item-title="name"
        item-value="id"
        no-data-text="No albums yet. Create one on the Albums page."
      />
    </AppFormGroup>
  </AppDialog>
</template>
