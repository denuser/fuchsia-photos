<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import { api } from '../api/client'
import AppPageHeader from '../components/AppPageHeader.vue'
import EmptyState from '../components/EmptyState.vue'
import PhotoBrowser from '../components/PhotoBrowser.vue'
import { usePhotosStore } from '../stores/photos'

const route = useRoute()
const router = useRouter()
const photos = usePhotosStore()

// The address holds the query or the photo, so that a search survives a
// reload and the back button returns to it.
const query = computed(() => (typeof route.query.q === 'string' ? route.query.q.trim() : ''))
const similarTo = computed(() => {
  const id = Number(route.query.similar)
  return Number.isInteger(id) && id > 0 ? id : undefined
})
const active = computed(() => similarTo.value !== undefined || query.value !== '')

const text = ref(query.value)
// Until the server answers, the field stays enabled.
const available = ref(true)

async function loadStatus() {
  const { data } = await api.GET('/search/status')
  available.value = data?.available ?? false
}

function open() {
  text.value = query.value
  if (similarTo.value !== undefined) {
    void photos.open({ similarTo: similarTo.value })
  } else if (query.value) {
    void photos.open({ search: query.value })
  }
}

onMounted(() => {
  void loadStatus()
  open()
})
watch([query, similarTo], open)

function submit() {
  const q = text.value.trim()
  if (q) {
    void router.push({ query: { q } })
  }
}

function clear() {
  text.value = ''
  void router.push({ query: {} })
}
</script>

<template>
  <div class="search">
    <PhotoBrowser v-if="active" mode="library">
      <template #header>
        <AppPageHeader
          v-if="similarTo !== undefined"
          title="Similar photos"
          description="Photos you can see that look like the chosen one, nearest first."
          :breadcrumbs="[{ title: 'Search', to: '/search' }]"
        />
        <AppPageHeader v-else title="Search" description="Photos you can see that match a description, best first." />
      </template>
      <template #toolbar>
        <form class="search-bar px-4 px-sm-6 pb-2" role="search" @submit.prevent="submit">
          <v-text-field
            v-model="text"
            placeholder="Describe a photo, such as a dog on the beach"
            aria-label="Search by description"
            prepend-inner-icon="fa-magnifying-glass"
            density="compact"
            hide-details
            clearable
            :disabled="!available"
            @click:clear="clear"
          />
        </form>
        <v-alert
          v-if="!available"
          type="info"
          variant="tonal"
          density="compact"
          class="mx-4 mx-sm-6 mb-2"
          title="Search by description is unavailable while the ML service is off"
        />
      </template>
      <template #empty>
        <EmptyState
          v-if="similarTo !== undefined"
          icon="fa-clone"
          title="No similar photos yet"
          text="Photos are compared once the ML service has processed them."
        />
        <EmptyState v-else icon="fa-magnifying-glass" title="Nothing found" text="Try other words." />
      </template>
    </PhotoBrowser>

    <div v-else class="page">
      <AppPageHeader title="Search" description="Photos you can see that match a description, best first." />
      <form class="search-bar px-4 px-sm-6 pb-2" role="search" @submit.prevent="submit">
        <v-text-field
          v-model="text"
          placeholder="Describe a photo, such as a dog on the beach"
          aria-label="Search by description"
          prepend-inner-icon="fa-magnifying-glass"
          density="compact"
          hide-details
          clearable
          autofocus
          :disabled="!available"
        />
      </form>
      <EmptyState
        v-if="available"
        icon="fa-magnifying-glass"
        title="Search by description"
        text="Describe what the photos show, in any language, and press Enter."
      />
      <EmptyState
        v-else
        icon="fa-magnifying-glass"
        title="Search by description is unavailable"
        text="It works while the ML service is running. Similar photos in the viewer still work."
      />
    </div>
  </div>
</template>

<style scoped>
.search {
  height: 100%;
}

.search-bar {
  max-width: 720px;
}
</style>
