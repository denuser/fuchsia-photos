<script setup lang="ts">
import { onMounted, ref, shallowRef } from 'vue'

import { api, type Tag } from '../api/client'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import EmptyState from '../components/EmptyState.vue'
import NameDialog from '../components/NameDialog.vue'
import { useTagsStore } from '../stores/tags'

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
    return failure.error
  }
  await tags.load()
  return null
}

async function remove() {
  const { error: failure } = await api.DELETE('/tags/{id}', { params: { path: { id: deleting.value!.id } } })
  error.value = failure?.error ?? ''
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
    <div class="text-h6 mb-4">Tags</div>
    <v-alert v-if="error" type="error" density="compact" class="mb-4" :text="error" />

    <EmptyState v-if="loaded && tags.tags.length === 0" icon="mdi-tag-multiple-outline" title="No tags yet" text="Select photos in the timeline to tag them." />

    <v-table v-else-if="loaded">
      <thead>
        <tr>
          <th>Name</th>
          <th>Photos</th>
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
            <v-btn icon="mdi-pencil" size="small" variant="text" :aria-label="`Rename ${tag.name}`" @click="startRename(tag)" />
            <v-btn icon="mdi-delete" size="small" variant="text" :aria-label="`Delete ${tag.name}`" @click="startDelete(tag)" />
          </td>
        </tr>
      </tbody>
    </v-table>

    <NameDialog v-model="renameOpen" title="Rename tag" label="Name" action="Rename" :initial="renaming?.name" :save="rename" />
    <ConfirmDialog
      v-model="deleteOpen"
      title="Delete this tag?"
      :text="`${deleting?.name} will be removed from all photos. The photos stay in your library.`"
      action="Delete"
      @confirm="remove"
    />
  </div>
</template>

<style scoped>
.page {
  height: calc(100vh - var(--v-layout-top, 0px));
  padding: 16px;
  overflow-y: auto;
}
</style>
