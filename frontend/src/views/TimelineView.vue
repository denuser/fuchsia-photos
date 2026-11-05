<script setup lang="ts">
import { computed, onMounted, ref, shallowRef, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useDisplay } from 'vuetify'

import { api, type NameCount } from '../api/client'
import EmptyState from '../components/EmptyState.vue'
import PhotoBrowser from '../components/PhotoBrowser.vue'
import { usePhotosStore, type PhotoQuery } from '../stores/photos'
import { useTagsStore } from '../stores/tags'
import { useUploadsStore } from '../stores/uploads'

const route = useRoute()
const router = useRouter()
const photos = usePhotosStore()
const tags = useTagsStore()
const uploads = useUploadsStore()

const dragging = ref(false)

// On a phone the cameras, lenses and dates hide behind a button, so the filters take one row.
const { xs } = useDisplay()

// The cameras and lenses of the user's photos, with how many photos each took.
const cameraList = shallowRef<NameCount[]>([])
const lensList = shallowRef<NameCount[]>([])

// The address holds the filters, so that they survive a reload and a tag page can link to them.
function queryList(key: string) {
  const value = route.query[key]
  const list = Array.isArray(value) ? value : value ? [value] : []
  return list.filter((item): item is string => typeof item === 'string')
}
const tagIds = computed(() => queryList('tag').map(Number).filter((id) => Number.isInteger(id)))
const cameras = computed(() => queryList('camera'))
const lenses = computed(() => queryList('lens'))
const fromDate = computed(() => (typeof route.query.from === 'string' ? route.query.from : ''))
const toDate = computed(() => (typeof route.query.to === 'string' ? route.query.to : ''))
const dateFields = computed(() => [
  { key: 'from', label: 'From', value: fromDate.value },
  { key: 'to', label: 'To', value: toDate.value },
])
// The filters a phone keeps behind the button.
const moreFiltered = computed(() => cameras.value.length > 0 || lenses.value.length > 0 || Boolean(fromDate.value || toDate.value))
const filtered = computed(() => tagIds.value.length > 0 || moreFiltered.value)

interface FilterChange {
  tag?: number[]
  camera?: string[]
  lens?: string[]
  from?: string | null
  to?: string | null
}

function setFilter(change: FilterChange) {
  const next = { tag: tagIds.value, camera: cameras.value, lens: lenses.value, from: fromDate.value, to: toDate.value, ...change }
  void router.replace({
    query: {
      ...(next.tag.length > 0 ? { tag: next.tag.map(String) } : {}),
      ...(next.camera.length > 0 ? { camera: next.camera } : {}),
      ...(next.lens.length > 0 ? { lens: next.lens } : {}),
      ...(next.from ? { from: next.from } : {}),
      ...(next.to ? { to: next.to } : {}),
    },
  })
}

const cameraFields = computed(() => [
  { key: 'camera', label: 'Any of these cameras', items: cameraList.value, value: cameras.value },
  { key: 'lens', label: 'Any of these lenses', items: lensList.value, value: lenses.value },
])

function photoCount(item: NameCount) {
  return { subtitle: item.count === 1 ? '1 photo' : `${item.count} photos` }
}

async function loadCameras() {
  const { data } = await api.GET('/photos/cameras')
  if (data) {
    cameraList.value = data.cameras
    lensList.value = data.lenses
  }
}

// The start of a day in the browser's time zone. The end of a range is exclusive, so it takes the next day.
function dayStart(date: string, addDays = 0) {
  const day = new Date(`${date}T00:00:00`)
  if (Number.isNaN(day.getTime())) {
    return undefined
  }
  day.setDate(day.getDate() + addDays)
  return day.toISOString()
}

function currentQuery(): PhotoQuery {
  return {
    tags: tagIds.value,
    cameras: cameras.value,
    lenses: lenses.value,
    from: fromDate.value ? dayStart(fromDate.value) : undefined,
    to: toDate.value ? dayStart(toDate.value, 1) : undefined,
  }
}

onMounted(() => {
  void tags.load()
  void loadCameras()
  void photos.open(currentQuery())
})

watch(() => route.query, () => void photos.open(currentQuery()))

function onTagged() {
  if (tagIds.value.length > 0) {
    void photos.reload()
  }
}

function onDrop(event: DragEvent) {
  dragging.value = false
  const files = Array.from(event.dataTransfer?.files ?? [])
  if (files.length > 0) {
    uploads.add(files)
  }
}
</script>

<template>
  <div class="timeline" @dragover.prevent="dragging = true" @dragleave.self="dragging = false" @drop.prevent="onDrop">
    <PhotoBrowser mode="library" @tagged="onTagged">
      <template #toolbar>
        <div class="filters">
          <v-autocomplete
            :model-value="tagIds"
            :items="tags.tags"
            item-title="name"
            item-value="id"
            label="Any of these tags"
            density="compact"
            hide-details
            multiple
            chips
            closable-chips
            class="tag-filter"
            @update:model-value="setFilter({ tag: $event })"
          />
          <v-menu v-if="xs" :close-on-content-click="false">
            <template #activator="{ props: menu }">
              <v-btn
                v-bind="menu"
                icon="mdi-tune-variant"
                :color="moreFiltered ? 'primary' : undefined"
                variant="text"
                aria-label="More filters"
              />
            </template>
            <v-card class="filter-menu">
              <v-autocomplete
                v-for="field in cameraFields"
                :key="field.key"
                :model-value="field.value"
                :items="field.items"
                item-title="name"
                item-value="name"
                :item-props="photoCount"
                :label="field.label"
                density="compact"
                hide-details
                multiple
                chips
                closable-chips
                @update:model-value="setFilter({ [field.key]: $event })"
              />
              <v-text-field
                v-for="field in dateFields"
                :key="field.key"
                :model-value="field.value"
                type="date"
                :label="field.label"
                density="compact"
                hide-details
                @update:model-value="setFilter({ [field.key]: $event })"
              />
            </v-card>
          </v-menu>
          <template v-else>
            <v-autocomplete
              v-for="field in cameraFields"
              :key="field.key"
              :model-value="field.value"
              :items="field.items"
              item-title="name"
              item-value="name"
              :item-props="photoCount"
              :label="field.label"
              density="compact"
              hide-details
              multiple
              chips
              closable-chips
              class="camera-filter"
              @update:model-value="setFilter({ [field.key]: $event })"
            />
            <v-text-field
              v-for="field in dateFields"
              :key="field.key"
              :model-value="field.value"
              type="date"
              :label="field.label"
              density="compact"
              hide-details
              class="date-filter"
              @update:model-value="setFilter({ [field.key]: $event })"
            />
          </template>
          <v-btn v-if="filtered && xs" icon="mdi-filter-remove-outline" variant="text" aria-label="Clear" @click="router.replace({ query: {} })" />
          <v-btn v-else-if="filtered" text="Clear" variant="text" @click="router.replace({ query: {} })" />
        </div>
      </template>
      <template #empty>
        <EmptyState v-if="filtered" icon="mdi-image-search-outline" title="No photos match these filters" />
        <EmptyState v-else icon="mdi-image-multiple-outline" title="No photos yet" text="Use the Upload button, or drag photos here." />
      </template>
    </PhotoBrowser>

    <div v-if="dragging" class="drop-hint">Drop photos to upload</div>
  </div>
</template>

<style scoped>
.timeline {
  position: relative;
}

.filters {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  padding: 8px;
}

.tag-filter {
  flex: 1 1 240px;
  max-width: 480px;
}

.camera-filter {
  flex: 1 1 200px;
  max-width: 360px;
}

.date-filter {
  flex: 0 1 170px;
}

.filter-menu {
  display: flex;
  flex-direction: column;
  gap: 8px;
  width: min(320px, calc(100vw - 24px));
  padding: 12px;
}

@media (max-width: 599.98px) {
  .filters {
    flex-wrap: nowrap;
    gap: 4px;
  }

  .tag-filter {
    flex-basis: 0;
    min-width: 0;
  }
}

.drop-hint {
  position: absolute;
  inset: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  border: 2px dashed rgb(var(--v-theme-primary));
  border-radius: 8px;
  background: rgba(var(--v-theme-surface), 0.85);
  font-size: 1.25rem;
  pointer-events: none;
}
</style>
