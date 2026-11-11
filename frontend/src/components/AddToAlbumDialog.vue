<script setup lang="ts">
import { ref, shallowRef, watch } from 'vue'

import { api, type Album } from '../api/client'
import { inBatches } from '../stores/photos'
import { apiError } from '../i18n/apiErrors'
import AppDialog from './AppDialog.vue'
import AppFormGroup from './AppFormGroup.vue'
import { t } from '../i18n'

const open = defineModel<boolean>({ required: true })

const props = defineProps<{ photoIds: number[] }>()
const emit = defineEmits<{ done: [album: string] }>()

const albums = shallowRef<Album[]>([])
const newName = ref('')
const error = ref('')
const saving = ref(false)

watch(open, async (isOpen) => {
  if (!isOpen) {
    return
  }
  newName.value = ''
  error.value = ''
  const { data } = await api.GET('/albums')
  albums.value = data ?? []
})

async function addTo(album: Album) {
  saving.value = true
  error.value = ''
  try {
    await inBatches(props.photoIds, (batch) =>
      api.POST('/albums/{id}/photos', { params: { path: { id: album.id } }, body: { ids: batch } }),
    )
    open.value = false
    emit('done', album.name)
  } catch (failure) {
    error.value = apiError((failure as Error).message)
  } finally {
    saving.value = false
  }
}

async function createAndAdd() {
  saving.value = true
  const { data, error: failure } = await api.POST('/albums', { body: { name: newName.value } })
  saving.value = false
  if (failure) {
    error.value = apiError(failure)
    return
  }
  await addTo(data)
}
</script>

<template>
  <AppDialog v-model="open" :title="t('addToAlbum.title')" :error="error">
    <v-form class="d-flex align-end ga-2 mb-2" @submit.prevent="createAndAdd">
      <AppFormGroup v-slot="field" :label="t('addToAlbum.newAlbum')" class="flex-grow-1 mb-0">
        <v-text-field v-bind="field" v-model="newName" />
      </AppFormGroup>
      <v-btn type="submit" color="primary" :text="t('common.create')" :disabled="!newName.trim()" :loading="saving" />
    </v-form>
    <v-list v-if="albums.length > 0" density="compact" max-height="300" class="overflow-y-auto">
      <v-list-item
        v-for="album in albums"
        :key="album.id"
        :title="album.name"
        :subtitle="t('common.photos', album.photoCount)"
        :disabled="saving"
        prepend-icon="fa-book-open"
        @click="addTo(album)"
      />
    </v-list>
  </AppDialog>
</template>
