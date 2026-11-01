<script setup lang="ts">
import { computed, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import EmptyState from '../components/EmptyState.vue'
import PhotoBrowser from '../components/PhotoBrowser.vue'
import { usePhotosStore } from '../stores/photos'
import { useUsersStore } from '../stores/users'

const route = useRoute()
const router = useRouter()
const photos = usePhotosStore()
const users = useUsersStore()

// The address holds the filter, so that it survives a reload.
const owner = computed(() => {
  const id = Number(route.query.owner)
  return Number.isInteger(id) && id > 0 ? id : undefined
})

function open() {
  void photos.open({ everyone: true, owner: owner.value })
}

onMounted(() => {
  void users.load()
  open()
})
watch(owner, open)

function chooseOwner(id: number | null) {
  void router.replace({ query: id ? { owner: String(id) } : {} })
}
</script>

<template>
  <PhotoBrowser mode="admin">
    <template #toolbar>
      <div class="d-flex align-center pa-2 ga-2">
        <v-autocomplete
          :model-value="owner"
          :items="users.everyone"
          item-title="login"
          item-value="id"
          label="Owner"
          density="compact"
          hide-details
          clearable
          class="owner-filter"
          @update:model-value="chooseOwner"
        />
      </div>
    </template>
    <template #empty>
      <EmptyState icon="mdi-image-multiple-outline" title="No photos" />
    </template>
  </PhotoBrowser>
</template>

<style scoped>
.owner-filter {
  max-width: 320px;
}
</style>
