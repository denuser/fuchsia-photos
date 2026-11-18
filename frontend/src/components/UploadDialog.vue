<script setup lang="ts">
// Chooses the album for an upload, then hands the choice to App, which opens
// the file picker within the same click, as browsers require.
import { ref, shallowRef, watch } from 'vue'

import { api } from '../api/client'
import { canPickFolders } from '../lib/uploadFiles'
import { useUploadsStore, type UploadAlbum } from '../stores/uploads'
import AppDialog from './AppDialog.vue'
import AppFormGroup from './AppFormGroup.vue'
import { t } from '../i18n'

// An album, the name of a new one, or null for the library only.
export type AlbumChoice = UploadAlbum | string | null

const emit = defineEmits<{ pick: [choice: AlbumChoice, folder: boolean] }>()

const folders = canPickFolders()

const uploads = useUploadsStore()
const albums = shallowRef<UploadAlbum[]>([])
// v-combobox gives the album when one is chosen, and the text when a name is typed.
const chosen = ref<UploadAlbum | string | null>(null)
// The dialog focuses the field as it opens, and a focused combobox opens its
// list when its items or value change, covering the buttons. So the field
// appears with both already in place.
const loaded = ref(false)

watch(
  () => uploads.dialogOpen,
  async (open) => {
    if (!open) {
      return
    }
    loaded.value = false
    const { data } = await api.GET('/albums')
    albums.value = (data ?? []).map(({ id, name }) => ({ id, name }))
    chosen.value = uploads.dialogAlbum
    loaded.value = true
  },
)

function choice(): AlbumChoice {
  const value = chosen.value
  if (value === null || value === undefined) {
    return null
  }
  if (typeof value !== 'string') {
    return value
  }
  const name = value.trim()
  if (!name) {
    return null
  }
  const existing = albums.value.find((a) => a.name === name)
  return existing ?? name
}

function pick(folder: boolean) {
  emit('pick', choice(), folder)
  uploads.dialogOpen = false
}
</script>

<template>
  <AppDialog v-model="uploads.dialogOpen" :title="t('uploadDialog.title')">
    <AppFormGroup v-slot="field" :label="t('uploadDialog.album')">
      <v-combobox
        v-if="loaded"
        v-bind="field"
        v-model="chosen"
        :items="albums"
        item-title="name"
        item-value="id"
        return-object
        clearable
        variant="outlined"
        :hint="t('uploadDialog.albumHint')"
        persistent-hint
      />
      <v-text-field v-else v-bind="field" loading disabled variant="outlined" :hint="t('uploadDialog.albumHint')" persistent-hint />
    </AppFormGroup>
    <div class="d-flex flex-wrap ga-2">
      <v-btn color="primary" variant="flat" prepend-icon="fa-file-image" :text="t('uploadDialog.chooseFiles')" @click="pick(false)" />
      <v-btn
        v-if="folders"
        color="primary"
        variant="outlined"
        prepend-icon="fa-folder-open"
        :text="t('uploadDialog.chooseFolder')"
        @click="pick(true)"
      />
    </div>
  </AppDialog>
</template>
