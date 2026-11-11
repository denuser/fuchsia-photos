<script setup lang="ts">
import { onMounted, ref, shallowRef } from 'vue'

import { api, type Tag } from '../api/client'
import AppPageHeader from '../components/AppPageHeader.vue'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import EmptyState from '../components/EmptyState.vue'
import { apiError } from '../i18n/apiErrors'
import NameDialog from '../components/NameDialog.vue'
import { useTagsStore } from '../stores/tags'
import { t } from '../i18n'

const tags = useTagsStore()

const loaded = ref(false)
const error = ref('')
const renaming = shallowRef<Tag | null>(null)
const renameOpen = ref(false)
const deleting = shallowRef<Tag | null>(null)
const deleteOpen = ref(false)

onMounted(async () => {
  await tags.load()
  loaded.value = true
})

async function rename(name: string) {
  const { error: failure } = await api.PATCH('/tags/{id}', { params: { path: { id: renaming.value!.id } }, body: { name } })
  if (failure) {
    return apiError(failure)
  }
  await tags.load()
  return null
}

async function remove() {
  const { error: failure } = await api.DELETE('/tags/{id}', { params: { path: { id: deleting.value!.id } } })
  error.value = failure ? apiError(failure) : ''
  await tags.load()
}

function startRename(tag: Tag) {
  renaming.value = tag
  renameOpen.value = true
}

function startDelete(tag: Tag) {
  deleting.value = tag
  deleteOpen.value = true
}
</script>

<template>
  <div class="page">
    <AppPageHeader :title="t('nav.tags')" :description="t('tags.description')" />
    <div class="page-section">
      <v-alert v-if="error" type="error" density="compact" class="mb-4" :title="error" />

      <EmptyState v-if="loaded && tags.tags.length === 0" icon="fa-tags" :title="t('tags.none')" :text="t('tags.noneText')">
        <template #actions>
          <v-btn :text="t('common.openTimeline')" color="primary" to="/" />
        </template>
      </EmptyState>

      <v-table v-else-if="loaded">
        <thead>
          <tr>
            <th>{{ t('common.name') }}</th>
            <th>{{ t('nav.photos') }}</th>
            <th />
          </tr>
        </thead>
        <tbody>
          <tr v-for="tag in tags.tags" :key="tag.id">
            <td>
              <router-link :to="{ path: '/', query: { tag: String(tag.id) } }">{{ tag.name }}</router-link>
            </td>
            <td>{{ tag.photoCount }}</td>
            <td class="text-right">
              <v-btn icon="fa-pen" size="small" variant="text" :aria-label="t('common.renameNamed', { name: tag.name })" @click="startRename(tag)" />
              <v-btn icon="fa-trash-can" size="small" variant="text" :aria-label="t('common.deleteNamed', { name: tag.name })" @click="startDelete(tag)" />
            </td>
          </tr>
        </tbody>
      </v-table>

      <NameDialog
        v-model="renameOpen"
        :title="t('tags.rename')"
        :label="t('common.name')"
        :action="t('common.rename')"
        :initial="renaming?.name" :save="rename" />
      <ConfirmDialog
        v-model="deleteOpen"
        :title="t('tags.deleteTitle')"
        :text="t('tags.deleteText', { name: deleting?.name })"
        :action="t('common.delete')"
        @confirm="remove"
      />
    </div>
  </div>
</template>
