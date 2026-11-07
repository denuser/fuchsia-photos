<script setup lang="ts">
import { computed, onMounted, ref, shallowRef, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useDisplay } from 'vuetify'

import { api, type NameCount } from '../api/client'
import AppAutocomplete from '../components/AppAutocomplete.vue'
import EmptyState from '../components/EmptyState.vue'
import PhotoBrowser from '../components/PhotoBrowser.vue'
import { useAuthStore } from '../stores/auth'
import { listFilters, usePhotosStore, type PhotoQuery } from '../stores/photos'
import { useTagsStore } from '../stores/tags'
import { useUploadsStore } from '../stores/uploads'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const photos = usePhotosStore()
const tags = useTagsStore()
const uploads = useUploadsStore()

const dragging = ref(false)
const browser = ref<InstanceType<typeof PhotoBrowser> | null>(null)

// Whether the timeline shows other users' photos the user can see next to the
// user's own. It is a setting of the user rather than a filter in the address,
// so that it stays as chosen on every device.
const withShared = computed(() => auth.user?.timelineIncludesShared ?? true)
const savingScope = ref(false)

async function setWithShared(value: boolean) {
  if (value === withShared.value) {
    return
  }
  savingScope.value = true
  const { data, error } = await api.PATCH('/me', { body: { timelineIncludesShared: value } })
  savingScope.value = false
  if (data) {
    auth.user = data
  } else {
    browser.value?.say(error?.error ?? 'The setting could not be saved')
  }
}

// On a phone the cameras, lenses and dates hide behind a button, so the filters take one row.
const { xs } = useDisplay()

// The cameras and lenses of the photos the other filters keep, with how many photos each would add.
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

// A chosen name the other filters leave no photos stays at the top of its list, so that it can be removed.
function withChosen(items: NameCount[], chosen: string[]) {
  const missing = chosen.filter((name) => !items.some((item) => item.name === name))
  if (missing.length === 0) {
    return items
  }
  return [...missing.map((name) => ({ name, count: 0 })), ...items]
}

const cameraFields = computed(() => [
  { key: 'camera', label: 'Any of these cameras', items: withChosen(cameraList.value, cameras.value), value: cameras.value },
  { key: 'lens', label: 'Any of these lenses', items: withChosen(lensList.value, lenses.value), value: lenses.value },
])

function photoCount(item: NameCount) {
  return { subtitle: item.count === 1 ? '1 photo' : `${item.count} photos` }
}

// Counts the request of the latest filters, so that an earlier answer arriving late is dropped.
let camerasRequest = 0

async function loadCameras(query: PhotoQuery) {
  const request = ++camerasRequest
  const { data } = await api.GET('/photos/cameras', { params: { query: { ...listFilters(query), from: query.from, to: query.to } } })
  if (data && request === camerasRequest) {
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
    withShared: withShared.value,
    tags: tagIds.value,
    cameras: cameras.value,
    lenses: lenses.value,
    from: fromDate.value ? dayStart(fromDate.value) : undefined,
    to: toDate.value ? dayStart(toDate.value, 1) : undefined,
  }
}

onMounted(() => {
  void tags.load()
  const query = currentQuery()
  void loadCameras(query)
  void photos.open(query)
})

watch(
  [() => route.query, withShared],
  () => {
    const query = currentQuery()
    void loadCameras(query)
    void photos.open(query)
  },
)

function onTagged() {
  if (tagIds.value.length > 0) {
    void loadCameras(currentQuery())
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
    <PhotoBrowser ref="browser" mode="library" @tagged="onTagged">
      <template #toolbar>
        <div class="filters">
          <v-btn-toggle
            :model-value="withShared ? 'all' : 'mine'"
            mandatory
            divided
            variant="outlined"
            density="compact"
            :disabled="savingScope"
            class="scope"
            @update:model-value="setWithShared($event === 'all')"
          >
            <v-btn v-if="xs" value="mine" icon="mdi-account-outline" aria-label="Mine" />
            <v-btn v-else value="mine" text="Mine" />
            <v-btn v-if="xs" value="all" icon="mdi-account-multiple-outline" aria-label="All" />
            <v-btn v-else value="all" text="All" />
          </v-btn-toggle>
          <AppAutocomplete
            :model-value="tagIds"
            :items="tags.tags"
            item-title="name"
            item-value="id"
            placeholder="Any of these tags"
            aria-label="Any of these tags"
            density="compact"
            hide-details
            multiple
            clearable
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
              <AppAutocomplete
                v-for="field in cameraFields"
                :key="field.key"
                :model-value="field.value"
                :items="field.items"
                item-title="name"
                item-value="name"
                :item-props="photoCount"
                :placeholder="field.label"
                :aria-label="field.label"
                density="compact"
                clearable
                hide-details
                multiple
                @update:model-value="setFilter({ [field.key]: $event })"
              />
              <v-text-field
                v-for="field in dateFields"
                :key="field.key"
                :model-value="field.value"
                type="date"
                :prefix="field.label"
                :aria-label="field.label"
                density="compact"
                hide-details
                @update:model-value="setFilter({ [field.key]: $event })"
              />
            </v-card>
          </v-menu>
          <template v-else>
            <AppAutocomplete
              v-for="field in cameraFields"
              :key="field.key"
              :model-value="field.value"
              :items="field.items"
              item-title="name"
              item-value="name"
              :item-props="photoCount"
              :placeholder="field.label"
              :aria-label="field.label"
              density="compact"
              hide-details
              multiple
              class="camera-filter"
              @update:model-value="setFilter({ [field.key]: $event })"
            />
            <v-text-field
              v-for="field in dateFields"
              :key="field.key"
              :model-value="field.value"
              type="date"
              :prefix="field.label"
              :aria-label="field.label"
              density="compact"
              hide-details
              class="date-filter"
              @update:model-value="setFilter({ [field.key]: $event })"
            />
          </template>
          <v-btn v-if="filtered && xs" icon="mdi-filter-remove-outline" variant="text" aria-label="Clear all filters" @click="router.replace({ query: {} })" />
          <v-btn v-else-if="filtered" text="Clear all filters" variant="text" @click="router.replace({ query: {} })" />
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
  height: 100%;
}

.filters {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  padding: 8px;
}

.scope {
  flex: none;
}

.tag-filter {
  flex: 1 1 200px;
  max-width: 480px;
}

.camera-filter {
  flex: 1 1 200px;
  max-width: 360px;
}

.date-filter {
  flex: 0 1 196px;
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
